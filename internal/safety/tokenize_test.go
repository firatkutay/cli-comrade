package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasFlagIsCaseInsensitiveForShortFlags pins MEDIUM finding #5's fix:
// hasFlag's short-flag scan must recognize a short flag regardless of the
// case it was spelled in — "-Rf", "-RF", "-rF" all register both 'r' and
// 'f', exactly like the already-lowercase "-rf" did before the fix.
func TestHasFlagIsCaseInsensitiveForShortFlags(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		short byte
		long  string
		want  bool
	}{
		{"lowercase -rf contains r", []string{"-rf"}, 'r', "", true},
		{"lowercase -rf contains f", []string{"-rf"}, 'f', "", true},
		{"capital -Rf contains r (case-insensitive)", []string{"-Rf"}, 'r', "", true},
		{"capital -Rf contains f", []string{"-Rf"}, 'f', "", true},
		{"all-caps -RF contains r", []string{"-RF"}, 'r', "", true},
		{"all-caps -RF contains f", []string{"-RF"}, 'f', "", true},
		{"mixed -rF contains r", []string{"-rF"}, 'r', "", true},
		{"mixed -rF contains f", []string{"-rF"}, 'f', "", true},
		{"separate -R and -f both register", []string{"-R", "-f"}, 'r', "", true},
		{"neither flag present", []string{"-v"}, 'r', "", false},
		{"short check disabled (short=0) still matches via long flag", []string{"--recursive"}, 0, "--recursive", true},
		{"short check disabled (short=0) does not match a short spelling", []string{"-r"}, 0, "--recursive", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasFlag(tc.args, tc.short, tc.long)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestUnwrapCommandSubstitution pins the exact contract MEDIUM finding #5's
// `$(...)` fix relies on: a `$(...)` boundary is unwrapped (nesting-aware,
// including a bare `$` left untouched when not followed by `(`), while a
// bare, non-substitution parenthesis — like the fork-bomb denylist
// signature's literal `()` — survives completely unmodified.
func TestUnwrapCommandSubstitution(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"no substitution, untouched", "rm -rf /tmp/x", "rm -rf /tmp/x"},
		{"simple substitution unwrapped", "$(rm -rf /)", "rm -rf /"},
		{"substitution with surrounding text", "echo $(rm -rf /) done", "echo rm -rf / done"},
		{"bare $HOME left untouched (not followed by paren)", "rm -rf $HOME", "rm -rf $HOME"},
		{"bare ${HOME} left untouched", "rm -rf ${HOME}", "rm -rf ${HOME}"},
		{"nested substitution both layers unwrapped", "$(echo $(rm -rf /))", "echo rm -rf /"},
		{
			"bare, non-substitution parens survive untouched (fork bomb signature)",
			":(){ :|:& };:", ":(){ :|:& };:",
		},
		{
			"substitution alongside a bare paren: only the substitution unwraps",
			"$(rm -rf /tmp) && echo (ok)", "rm -rf /tmp && echo (ok)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, unwrapCommandSubstitution(tc.input))
		})
	}
}

// TestNormalizeCommandUnwrapsCommandSubstitution is the normalizeCommand-
// level (not just the helper-level) regression for the same fix, run
// through the exact entry point Engine.Evaluate uses.
func TestNormalizeCommandUnwrapsCommandSubstitution(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"command substitution flattens like a bare invocation", "$(rm -rf /)", "rm -rf /"},
		{"quotes and substitution both strip", `$(rm -rf "/")`, "rm -rf /"},
		{"fork bomb signature is not touched by substitution unwrapping", ":(){ :|:& };:", ":(){ :|:& };:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeCommand(tc.input))
		})
	}
}

// TestNormalizeCommandIsIdempotent asserts normalizeCommand's documented
// idempotency: applying it a second time to its own output is a no-op.
func TestNormalizeCommandIsIdempotent(t *testing.T) {
	inputs := []string{
		"rm -rf /tmp/x",
		`echo "rm -rf /"`,
		"$(rm -rf /)",
		"echo $(rm -rf /) done",
		":(){ :|:& };:",
		"dd if=/dev/zero of='/dev/sda'",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			once := normalizeCommand(in)
			twice := normalizeCommand(once)
			assert.Equal(t, once, twice, "normalizeCommand must be idempotent")
		})
	}
}
