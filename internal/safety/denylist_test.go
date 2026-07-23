package safety

import (
	"testing"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// --- Finding 1 hardening: denylist-rule coverage for the filesystem-
// format family beyond "mkfs" and the generic real-disk-device rule.
// Every case here was measured as Allow before this fix.

func TestEvaluateDenylistBlocksBroadenedMkfsFamily(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"mke2fs (Finding 1 proof)", "mke2fs /dev/sda1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkswap (Finding 1 proof)", "mkswap /dev/sda1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"newfs (Finding 1 proof)", "newfs /dev/disk0", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkdosfs", "mkdosfs /dev/sdb1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkntfs", "mkntfs /dev/sdb1", RiskRead, Block, RiskDestructive, "mkfs"},
		{
			"mkswaputils near-miss is not mkswap (word boundary)",
			"mkswaputils --help", RiskRead, Allow, RiskRead, "",
		},
		{
			"newfsx near-miss is not newfs (word boundary)",
			"newfsx --help", RiskRead, Allow, RiskRead, "",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateDenylistBlocksDestructiveDiskToolOnRealDevice pins the
// TARGETED denylist rule: Block only fires when a recognized destructive
// disk tool (isDestructiveDiskTool) co-occurs with a real (non-pseudo)
// /dev/<disk> reference — not for every command that merely mentions a
// real disk device (that broader form was rejected on review for
// hard-blocking legitimate read-only disk access; see
// TestEvaluateEscalationRealDiskDeviceFallbackConfirmsUnrecognizedTools
// in escalation_test.go for what happens to those instead).
func TestEvaluateDenylistBlocksDestructiveDiskToolOnRealDevice(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"wipefs -a /dev/sda (Finding 1 proof)", "wipefs -a /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"blkdiscard /dev/sda (Finding 1 proof)", "blkdiscard /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"sgdisk --zap-all /dev/sda (Finding 1 proof)", "sgdisk --zap-all /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"sfdisk --delete /dev/sda", "sfdisk --delete /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"badblocks -w /dev/sda (write mode)", "badblocks -w /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"cryptsetup luksFormat /dev/sda", "cryptsetup luksFormat /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"shred -uvz /dev/sda (Finding 1 proof)", "shred -uvz /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"tee /dev/sda", "echo payload | tee /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{
			"a safe pseudo-device is never blocked, even with a destructive tool",
			"wipefs -a /dev/null", RiskRead, Allow, RiskRead, "",
		},
		{
			"a destructive tool with no /dev/ reference at all is unaffected",
			"wipefs --help", RiskRead, Allow, RiskRead, "",
		},
		{
			"sfdisk without a destructive flag is not blocked (dump/list is read-only)",
			"sfdisk -l /dev/sda", RiskRead, Confirm, RiskDestructive, "real /dev/<disk>",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateDenylistDestructiveDiskToolAdjacencyScoped pins the S1 fix:
// the destructive-disk-tool Block rule must only fire when the real
// /dev/<disk> reference appears among the DESTRUCTIVE TOOL'S OWN
// arguments, not merely somewhere else on the same command/pipeline. A
// whole-command-string co-occurrence check previously false-Blocked safe
// pipelines like "read the disk, tee the output to a FILE" purely because
// a destructive tool word (tee) and a disk reference (elsewhere in the
// pipeline) both appeared on the line.
func TestEvaluateDenylistDestructiveDiskToolAdjacencyScoped(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{
			"cat /dev/sda | tee backup.img: tee's OWN target is a file, not the disk -- must NOT block",
			"cat /dev/sda | tee backup.img", RiskRead, Confirm, RiskDestructive, "real /dev/<disk>",
		},
		{
			"dd if=/dev/sda bs=4M | tee disk.img: imaging pipeline, tee's target is a file -- must NOT block",
			"dd if=/dev/sda bs=4M | tee disk.img", RiskRead, Confirm, RiskDestructive, "real /dev/<disk>",
		},
		{
			"lsblk | tee /tmp/o ; blkid /dev/sda: two read-only commands, tee's target is a file -- must NOT block",
			"lsblk | tee /tmp/o ; blkid /dev/sda", RiskRead, Confirm, RiskDestructive, "real /dev/<disk>",
		},
		{
			"echo x | tee /dev/sda: tee's OWN target IS the disk -- still blocks",
			"echo x | tee /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool",
		},
		{
			"shred -uvz /dev/sda: shred's OWN target IS the disk -- still blocks",
			"shred -uvz /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool",
		},
		{
			"wipefs -a /dev/sda: wipefs's OWN target IS the disk -- still blocks",
			"wipefs -a /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateDenylistCaseInsensitiveCommandNames pins the S2 fix:
// destructive command-NAME matching must be case-insensitive, not just
// flag matching (MEDIUM finding #5 already lower-cased flags). On a
// case-insensitive filesystem (macOS/Windows, both supported platforms)
// `RM -rf /` and `rm -rf /` name the same executable and must be
// classified identically.
func TestEvaluateDenylistCaseInsensitiveCommandNames(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"RM -rf / (uppercase command name) blocks exactly like lowercase", "RM -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf / (lowercase control, must still block)", "rm -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"WIPEFS -a /dev/sda (uppercase tool name) blocks exactly like lowercase", "WIPEFS -a /dev/sda", RiskRead, Block, RiskDestructive, "destructive disk tool"},
		{"mixed-case Rm -RF / also blocks", "Rm -RF /", RiskRead, Block, RiskDestructive, "rm -rf"},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateDenylistSpecificDiskRulesStillWinTheirOwnName confirms the
// new generic "/dev/<disk> reference" rule does not shadow the more
// specific dd-of=/redirect rules' MatchedRule text for the commands they
// already covered before this fix -- both rules would Block, but the
// specific one still decides the reported name (see denylist.go's comment
// on the generic rule's placement).
func TestEvaluateDenylistSpecificDiskRulesStillWinTheirOwnName(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"dd of=/dev/sda still reports the dd-specific rule name", "dd if=/dev/zero of=/dev/sda", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"redirect into /dev/sda still reports the redirect-specific rule name", "echo hello > /dev/sda", RiskRead, Block, RiskDestructive, "/dev/<disk> (redirect"},
	}
	runEvalCases(t, engine, cases)
}
