package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// TestNormalizeCommand pins normalizeCommand's exact contract: strip every
// quote character anywhere in the string (not just at token edges — see
// the embedded-quote case below) and collapse whitespace runs to single
// spaces.
func TestNormalizeCommand(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"no quotes, untouched", "rm -rf /tmp/x", "rm -rf /tmp/x"},
		{"double-quoted argument", `echo "rm -rf /"`, "echo rm -rf /"},
		{"single-quoted argument", "dd if=/dev/zero of='/dev/sda'", "dd if=/dev/zero of=/dev/sda"},
		{"embedded quote mid-token defeats naive edge-trim, not this", `of="/dev/sda"`, "of=/dev/sda"},
		{"backtick stripped too", "echo `rm -rf /`", "echo rm -rf /"},
		{"collapses extra whitespace", "rm   -rf    /tmp/x", "rm -rf /tmp/x"},
		{"leading/trailing whitespace trimmed", "  rm -rf /tmp/x  ", "rm -rf /tmp/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeCommand(tc.input))
		})
	}
}

// TestNormalizeRootTarget pins the exact canonicalization MAJOR 4 relies
// on: a "/"-rooted target is canonicalized with path.Clean (collapsing
// repeated slashes, ".", and ".." segments that can't climb above root),
// while "~"/"$HOME"/"${HOME}" get their trailing-slash/trailing-"/."
// normalized by hand (path.Clean has no notion of them) — and neither
// path ever touches a genuine near-miss.
func TestNormalizeRootTarget(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"bare root unchanged", "/", "/"},
		{"double slash collapses to root", "//", "/"},
		{"triple slash collapses to root", "///", "/"},
		{"trailing /. resolves to root", "/.", "/"},
		{"embedded /./ resolves to root", "/./", "/"},
		{"embedded /.// resolves to root", "/.//", "/"},
		{"trailing .. cannot climb above root", "/..", "/"},
		{"repeated .. cannot climb above root", "/../..", "/"},
		{"trailing slash on home strips to bare home", "~/", "~"},
		{"trailing /. on home strips to bare home", "~/.", "~"},
		{"trailing slash on $HOME strips to bare $HOME", "$HOME/", "$HOME"},
		{"near-miss path is untouched", "/tmp/x", "/tmp/x"},
		{"near-miss with trailing /. only strips the /.", "/tmp/.", "/tmp"},
		{"near-miss under home is untouched", "~/project", "~/project"},
		{"glob root unchanged", "/*", "/*"},
		{"glob under home unchanged", "~/*", "~/*"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeRootTarget(tc.input))
		})
	}
}

// TestEvaluateDenylistBlocksRmRootDotDotVariants is the Evaluate-level
// regression for the residual a re-verification pass found after the
// first hardening round: `rm -rf /..`, `rm -rf /./`, `rm -rf /.//`, and
// `rm -rf /../..` all resolve to the filesystem root once path.Clean
// canonicalizes them, and must Block exactly like `rm -rf /` — reaching
// only Confirm here would let a root delete slip through under
// auto+--yolo+confirm_destructive=false (FAZ 6's territory, but the
// safety engine's Block verdict is the one thing standing between that
// combination and actually deleting the root).
func TestEvaluateDenylistBlocksRmRootDotDotVariants(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"rm -rf /..", "rm -rf /..", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf /./", "rm -rf /./", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf /.//", "rm -rf /.//", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf /../..", "rm -rf /../..", RiskRead, Block, RiskDestructive, "rm -rf"},
		// Re-asserted alongside the new cases: the near-miss set must
		// still only escalate, never Block, after this fix.
		{"rm -rf ./build stays a near-miss", "rm -rf ./build", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf /tmp/x stays a near-miss", "rm -rf /tmp/x", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf ~/project stays a near-miss", "rm -rf ~/project", RiskRead, Confirm, RiskDestructive, "rm -r"},
	}
	runEvalCases(t, engine, cases)
}

// TestHasRealDiskDeviceReference pins the device-family allowlist/
// denylist boundary BLOCKER 2 and MEDIUM 5 depend on.
func TestHasRealDiskDeviceReference(t *testing.T) {
	realDiskCases := []string{
		"/dev/sda", "/dev/sdb1", "/dev/hda", "/dev/nvme0n1",
		"/dev/vda", "/dev/xvda", "/dev/mmcblk0", "/dev/disk0", "/dev/loop0",
	}
	for _, path := range realDiskCases {
		t.Run("real disk: "+path, func(t *testing.T) {
			assert.True(t, hasRealDiskDeviceReference(path), "expected %q to be recognized as a real disk device", path)
		})
	}

	safeCases := []string{
		"/dev/null", "/dev/zero", "/dev/tty", "/dev/tty1", "/dev/tty42",
		"/dev/random", "/dev/urandom", "/dev/full",
		"/dev/stdin", "/dev/stdout", "/dev/stderr",
		"/dev/fd/3", "/dev/pts/0",
	}
	for _, path := range safeCases {
		t.Run("safe pseudo-device: "+path, func(t *testing.T) {
			assert.False(t, hasRealDiskDeviceReference(path), "expected %q to be recognized as a safe pseudo-device", path)
		})
	}

	assert.False(t, hasRealDiskDeviceReference("/home/user/image.iso"), "a non-/dev/ path is never a disk reference")
}

// TestIsRemoveItemAliasWord pins exactly which command words this
// package's Remove-Item-equivalent rules recognize.
func TestIsRemoveItemAliasWord(t *testing.T) {
	aliases := []string{"Remove-Item", "remove-item", "REMOVE-ITEM", "Remove-ItemProperty", "ri", "RI", "rd", "rmdir", "del", "erase", "rm", "/bin/rm"}
	for _, word := range aliases {
		t.Run(word, func(t *testing.T) {
			assert.True(t, isRemoveItemAliasWord(word), "expected %q to be recognized as a Remove-Item alias", word)
		})
	}

	notAliases := []string{"ls", "cat", "mv", "cp", "echo", "Get-Item"}
	for _, word := range notAliases {
		t.Run("not an alias: "+word, func(t *testing.T) {
			assert.False(t, isRemoveItemAliasWord(word))
		})
	}
}

// TestHasAbbreviatedFlag pins the PowerShell parameter-abbreviation
// acceptance BLOCKER 3 relies on: an unambiguous case-insensitive prefix
// of the full flag name, and nothing that merely happens to share a
// substring with it.
func TestHasAbbreviatedFlag(t *testing.T) {
	acceptedForRecurse := []string{"-r", "-R", "-rec", "-Rec", "-recurse", "-Recurse", "--recurse", "--Recurse"}
	for _, flag := range acceptedForRecurse {
		t.Run("accepted recurse abbreviation: "+flag, func(t *testing.T) {
			assert.True(t, hasAbbreviatedFlag([]string{flag}, "recurse"))
		})
	}

	acceptedForForce := []string{"-f", "-fo", "-Fo", "-force", "-Force", "--force"}
	for _, flag := range acceptedForForce {
		t.Run("accepted force abbreviation: "+flag, func(t *testing.T) {
			assert.True(t, hasAbbreviatedFlag([]string{flag}, "force"))
		})
	}

	rejected := []string{"-x", "-whatif", "-recklessly", "C:\\"}
	for _, flag := range rejected {
		t.Run("rejected for recurse: "+flag, func(t *testing.T) {
			assert.False(t, hasAbbreviatedFlag([]string{flag}, "recurse"))
		})
	}
}
