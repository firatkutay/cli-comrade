package safety

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// evalCase is one Engine.Evaluate table-driven case. wantRule, when
// non-empty, must be a substring of the returned Decision.MatchedRule
// (exact rule names are implementation detail; the substring anchors the
// test to *which* rule fired without hardcoding its full wording).
type evalCase struct {
	name     string
	command  string
	declared RiskClass
	want     Action
	wantRisk RiskClass
	wantRule string // substring of MatchedRule, "" to skip the check
}

func runEvalCases(t *testing.T, engine *Engine, cases []evalCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := engine.Evaluate(tc.command, tc.declared)
			assert.Equal(t, tc.want, got.Action, "command %q", tc.command)
			assert.Equal(t, tc.wantRisk, got.EffectiveRisk, "command %q", tc.command)
			if tc.wantRule != "" {
				assert.Contains(t, got.MatchedRule, tc.wantRule, "command %q", tc.command)
			}
			if tc.want == Block {
				assert.NotEmpty(t, got.Reason)
			}
		})
	}
}

// --- Denylist: hits (must Block), unix + PowerShell, spacing/case variants ---

func TestEvaluateDenylistBlocksRmRootDeleteVariants(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"rm -rf /", "rm -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -fr /", "rm -fr /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf /*", "rm -rf /*", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -r -f /", "rm -r -f /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -f -r /", "rm -f -r /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm --recursive --force /", "rm --recursive --force /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm --force --recursive /", "rm --force --recursive /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf ~", "rm -rf ~", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf $HOME", "rm -rf $HOME", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf ${HOME}", "rm -rf ${HOME}", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf --no-preserve-root /", "rm -rf --no-preserve-root /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"sudo rm -rf /", "sudo rm -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{
			"quoted echo false positive is accepted (fail-closed)",
			`echo "rm -rf /"`, RiskRead, Block, RiskDestructive, "rm -rf",
		},
		{"extra whitespace between flags and target", "rm  -rf   /", RiskRead, Block, RiskDestructive, "rm -rf"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistDoesNotBlockRmNearMisses(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		// Recursive+force but NOT targeting root/home: escalated to
		// destructive by the rm -r/-f escalation rule, but never Blocked.
		{"rm -rf ./build", "rm -rf ./build", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf /tmp/x", "rm -rf /tmp/x", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf /home/user/project", "rm -rf /home/user/project", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm without recursive/force flags", "rm ./file.txt", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksMkfs(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"mkfs bare", "mkfs /dev/sda1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkfs.ext4", "mkfs.ext4 /dev/sdb1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkfs.xfs with sudo", "sudo mkfs.xfs /dev/sdc1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkfs with flags", "mkfs -t ext4 /dev/sdd1", RiskRead, Block, RiskDestructive, "mkfs"},
		{"mkfsutils near-miss is not mkfs", "mkfsutils --help", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksDdToDiskDevice(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"dd of=/dev/sda", "dd if=/dev/zero of=/dev/sda bs=1M", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/nvme disk", "dd if=/dev/zero of=/dev/nvme0n1", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/hda", "dd if=/dev/urandom of=/dev/hda", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{
			// MEDIUM 5: CLAUDE.md's `dd of=/dev/` denylist entry names no
			// specific device family — a loopback device is still a real
			// block device and must Block too, not merely escalate.
			"dd of= a loop device now blocks too (broadened device family, MEDIUM 5)",
			"dd if=/dev/zero of=/dev/loop0", RiskRead, Block, RiskDestructive, "dd of=/dev",
		},
		{"dd targeting a regular file is benign", "dd if=/dev/zero of=/home/user/image.iso", RiskRead, Allow, RiskRead, ""},
		{"dd of=/dev/null is a safe pseudo-device, not blocked", "dd if=/dev/zero of=/dev/null", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksDiskpartClean(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"diskpart clean lowercase", "diskpart clean", RiskRead, Block, RiskDestructive, "diskpart"},
		{"Diskpart Clean mixed case", "Diskpart Clean", RiskRead, Block, RiskDestructive, "diskpart"},
		{"DISKPART ... CLEAN uppercase", "DISKPART select disk 0 CLEAN", RiskRead, Block, RiskDestructive, "diskpart"},
		{"diskpart without clean is not blocked", "diskpart list disk", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksRemoveItemRecurseDriveRoot(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"Remove-Item -Recurse C:\\", `Remove-Item -Recurse C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"Remove-Item -Recurse -Force C:\\", `Remove-Item -Recurse -Force C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"lowercase remove-item -recurse c:\\", `remove-item -recurse c:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{
			"Remove-Item -Recurse on a subdirectory is a near-miss, not blocked",
			`Remove-Item -Recurse C:\Users\foo`, RiskRead, Confirm, RiskDestructive, "Remove-Item",
		},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksFormatDrive(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"format C:", "format C:", RiskRead, Block, RiskDestructive, "format"},
		{"format lowercase c:", "format c:", RiskRead, Block, RiskDestructive, "format"},
		{"format D: with fs flag", "format /FS:NTFS D:", RiskRead, Block, RiskDestructive, "format"},
		{
			"format targeting a path, not a drive, is a near-miss",
			`format D:\data`, RiskRead, Allow, RiskRead, "",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- BLOCKER 3: PowerShell Remove-Item alias family (ri/rd/rmdir/del/erase/rm) ---

func TestEvaluateDenylistBlocksRemoveItemAliasesAtDriveRoot(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"ri -r -fo C:\\ (abbreviated flags)", `ri -r -fo C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"rd /s /q C:\\ (cmd.exe legacy flags)", `rd /s /q C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"rmdir /s C:\\", `rmdir /s C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"del -Recurse C:\\", `del -Recurse C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"erase -Recurse C:\\", `erase -Recurse C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"rm -Recurse -Force C:\\ (PS rm alias at drive root, MAJOR 4c)", `rm -Recurse -Force C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{"case-insensitive RI -REC C:\\", `RI -REC C:\`, RiskRead, Block, RiskDestructive, "Remove-Item"},
		{
			"ri without any recurse-ish flag on a subdirectory is benign",
			`ri .\build`, RiskRead, Allow, RiskRead, "",
		},
		{
			"rd on a drive root without any recurse-ish flag is benign",
			`rd C:\`, RiskRead, Allow, RiskRead, "",
		},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationRemoveItemAliasAnyTarget(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"ri -Recurse on a subdirectory escalates, not blocked", `ri -Recurse .\build`, RiskRead, Confirm, RiskDestructive, "Remove-Item"},
		{"del -Force on a file escalates", `del -Force C:\temp\file.txt`, RiskRead, Confirm, RiskDestructive, "Remove-Item"},
		{"rd /s on a subdirectory escalates", `rd /s C:\temp\build`, RiskRead, Confirm, RiskDestructive, "Remove-Item"},
		{
			"ri without any recurse/force flag is not escalated at all",
			`ri .\build`, RiskRead, Allow, RiskRead, "",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- MAJOR 4: root-delete near-equivalents must still Block, not merely escalate ---

func TestEvaluateDenylistBlocksRmRootNearEquivalents(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"rm -rf // (double slash)", "rm -rf //", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf /. (trailing dot)", "rm -rf /.", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf ~/ (trailing slash on home)", "rm -rf ~/", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"rm -rf $HOME/ (trailing slash)", "rm -rf $HOME/", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"/bin/rm -rf / (absolute path to rm)", "/bin/rm -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
		{"/usr/bin/rm -rf / (absolute path to rm)", "/usr/bin/rm -rf /", RiskRead, Block, RiskDestructive, "rm -rf"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateRmRootNearMissesStillOnlyEscalate(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"rm -rf ./build stays a near-miss", "rm -rf ./build", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf /tmp/x stays a near-miss", "rm -rf /tmp/x", RiskRead, Confirm, RiskDestructive, "rm -r"},
		{"rm -rf ~/project stays a near-miss", "rm -rf ~/project", RiskRead, Confirm, RiskDestructive, "rm -r"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksForkBomb(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"canonical fork bomb", ":(){ :|:& };:", RiskRead, Block, RiskDestructive, "fork bomb"},
		{"fork bomb extra whitespace", ": ( ) { : | : & } ; :", RiskRead, Block, RiskDestructive, "fork bomb"},
		{"fork bomb no spaces", ":(){:|:&};:", RiskRead, Block, RiskDestructive, "fork bomb"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksRedirectIntoDiskDevice(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"echo redirected to /dev/sda", "echo hello > /dev/sda", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"cat redirected to /dev/sdb1", "cat payload > /dev/sdb1", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"redirect to a regular file is benign", "echo hello > /tmp/out.txt", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/null is a safe pseudo-device", "echo hello > /dev/null", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

// --- BLOCKER 2 / MEDIUM 5: disk-device family broadened beyond /dev/sd* ---

func TestEvaluateDenylistBlocksBroadDiskDeviceFamilyOnRedirect(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"redirect to /dev/nvme0n1", "echo hello > /dev/nvme0n1", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"redirect to /dev/vda", "echo hello > /dev/vda", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"redirect to /dev/xvda", "echo hello > /dev/xvda", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"redirect to /dev/mmcblk0", "echo hello > /dev/mmcblk0", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"redirect to /dev/disk0 (macOS)", "echo hello > /dev/disk0", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateDenylistBlocksBroadDiskDeviceFamilyOnDdOf(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"dd of=/dev/nvme0n1", "dd if=/dev/zero of=/dev/nvme0n1", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/vda", "dd if=/dev/zero of=/dev/vda", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/xvda", "dd if=/dev/zero of=/dev/xvda", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/mmcblk0", "dd if=/dev/zero of=/dev/mmcblk0", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of=/dev/disk0 (macOS)", "dd if=/dev/zero of=/dev/disk0", RiskRead, Block, RiskDestructive, "dd of=/dev"},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateSafePseudoDevicesAreNeverTreatedAsDisks(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"redirect to /dev/null", "echo hello > /dev/null", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/urandom", "echo hello > /dev/urandom", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/zero", "echo hello > /dev/zero", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/tty", "echo hello > /dev/tty", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/tty1 (numbered tty)", "echo hello > /dev/tty1", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/fd/3", "echo hello > /dev/fd/3", RiskRead, Allow, RiskRead, ""},
		{"redirect to /dev/pts/0", "echo hello > /dev/pts/0", RiskRead, Allow, RiskRead, ""},
		{"dd of=/dev/urandom", "dd if=/dev/zero of=/dev/urandom", RiskRead, Allow, RiskRead, ""},
		{"dd of=/dev/null", "dd if=/dev/zero of=/dev/null", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

// --- BLOCKER 1: quote-fragility fixed by normalizing before every match ---

func TestEvaluateQuoteFragilityIsFixedByNormalization(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"dd of= single-quoted disk device", "dd if=/dev/zero of='/dev/sda'", RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"dd of= double-quoted disk device", `dd if=/dev/zero of="/dev/sda"`, RiskRead, Block, RiskDestructive, "dd of=/dev"},
		{"mkfs.ext4 single-quoted device", "mkfs.ext4 '/dev/sdb'", RiskRead, Block, RiskDestructive, "mkfs"},
		{"redirect to a single-quoted disk device", "echo hello > '/dev/sda'", RiskRead, Block, RiskDestructive, "/dev/<disk>"},
		{"rm -rf single-quoted root", "rm -rf '/'", RiskRead, Block, RiskDestructive, "rm -rf"},
		{
			"a single quote must not defeat the safe-pseudo-device allowlist either",
			"dd if=/dev/zero of='/dev/null'", RiskRead, Allow, RiskRead, "",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- Escalation rules: hit + non-hit for every rule ---

func TestEvaluateEscalationChmodRecursive777(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"chmod -R 777 then path", "chmod -R 777 /var/www", RiskRead, Confirm, RiskDestructive, "chmod"},
		{"chmod 777 -R order swapped", "chmod 777 -R /var/www", RiskRead, Confirm, RiskDestructive, "chmod"},
		{"chmod 644 is not escalated", "chmod 644 file.txt", RiskRead, Allow, RiskRead, ""},
		{"chmod -R without 777 is not escalated", "chmod -R 755 /var/www", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationRegistryRemove(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"Remove-Item HKLM", `Remove-Item HKLM:\Software\Foo`, RiskRead, Confirm, RiskDestructive, "registry"},
		{
			"Remove-ItemProperty HKCU", `Remove-ItemProperty -Path HKCU:\Software\Foo -Name Bar`,
			RiskRead, Confirm, RiskDestructive, "registry",
		},
		{"Get-ItemProperty is not a remove, not escalated", `Get-ItemProperty HKLM:\Software\Foo`, RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationKillallTaskkill(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"killall", "killall node", RiskRead, Confirm, RiskElevated, "killall"},
		{"taskkill /F", "taskkill /F /IM node.exe", RiskRead, Confirm, RiskElevated, "killall"},
		{"taskkill without /F is not escalated", "taskkill /IM node.exe", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationIptablesNetsh(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"iptables -F", "iptables -F", RiskRead, Confirm, RiskElevated, "iptables"},
		{"netsh advfirewall reset", "netsh advfirewall reset", RiskRead, Confirm, RiskElevated, "iptables"},
		{"Netsh AdvFirewall Reset case-insensitive", "Netsh AdvFirewall Reset", RiskRead, Confirm, RiskElevated, "iptables"},
		{"iptables -L is not escalated", "iptables -L", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationGitPushForce(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"git push --force", "git push --force origin main", RiskRead, Confirm, RiskDestructive, "git push"},
		{"git push -f", "git push -f origin main", RiskRead, Confirm, RiskDestructive, "git push"},
		{"git push without force is not escalated", "git push origin main", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationSudoRunas(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"sudo", "sudo apt-get install docker-ce", RiskRead, Confirm, RiskElevated, "sudo"},
		{"runas", "runas /user:Administrator cmd", RiskRead, Confirm, RiskElevated, "sudo"},
		{"Start-Process -Verb RunAs", "Start-Process cmd -Verb RunAs", RiskRead, Confirm, RiskElevated, "sudo"},
		{"sudoku near-miss word boundary", "sudoku-solver --fast", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationPackageInstall(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"apt install", "apt install docker.io", RiskRead, Allow, RiskWrite, "package manager install"},
		{"winget install", "winget install Docker.DockerDesktop", RiskRead, Allow, RiskWrite, "package manager install"},
		{"brew install", "brew install jq", RiskRead, Allow, RiskWrite, "package manager install"},
		{"apt list --installed is not an install", "apt list --installed", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEscalationNetworkVerbs(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{"curl", "curl -O https://example.com/file", RiskRead, Allow, RiskNetwork, "network access"},
		{"wget", "wget https://example.com/file", RiskRead, Allow, RiskNetwork, "network access"},
		{"Invoke-WebRequest", "Invoke-WebRequest -Uri https://example.com", RiskRead, Allow, RiskNetwork, "network access"},
		{"apt update", "apt update", RiskRead, Allow, RiskNetwork, "network access"},
		{"apt-get upgrade", "apt-get upgrade", RiskRead, Allow, RiskNetwork, "network access"},
		{"apt list is not a network verb", "apt list", RiskRead, Allow, RiskRead, ""},
	}
	runEvalCases(t, engine, cases)
}

// --- Upward-only escalation property: declared risk is a floor, never lowered ---

func TestEvaluateNeverLowersDeclaredRisk(t *testing.T) {
	engine := NewEngine(config.Default())
	cases := []evalCase{
		{
			"declared destructive stays destructive on a totally benign command",
			"ls -la", RiskDestructive, Confirm, RiskDestructive, "",
		},
		{
			"declared elevated stays elevated on a benign read command",
			"cat /etc/hostname", RiskElevated, Confirm, RiskElevated, "",
		},
		{
			"declared network stays network (below Confirm threshold) when nothing escalates it",
			"ls -la", RiskNetwork, Allow, RiskNetwork, "",
		},
		{
			"declared read escalates upward to destructive via rm -r",
			"rm -r /tmp/x", RiskRead, Confirm, RiskDestructive, "rm -r",
		},
		{
			"declared write escalates upward to elevated via sudo",
			"sudo systemctl restart nginx", RiskWrite, Confirm, RiskElevated, "sudo",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- Combined multi-rule commands (highest match wins) ---

func TestEvaluateMultipleEscalationsResolveToHighestRisk(t *testing.T) {
	engine := NewEngine(config.Default())
	got := engine.Evaluate("sudo apt-get install docker-ce", RiskRead)
	assert.Equal(t, Confirm, got.Action)
	assert.Equal(t, RiskElevated, got.EffectiveRisk, "sudo (elevated) must win over package-install (write)")
	assert.Contains(t, got.MatchedRule, "sudo")
}

// --- User denylist_extra ---

func TestEvaluateUserDenylistExtraBlocksCustomPattern(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.DenylistExtra = []string{`\bmy-dangerous-tool\b`}
	engine := NewEngine(cfg)

	got := engine.Evaluate("my-dangerous-tool --run", RiskRead)
	assert.Equal(t, Block, got.Action)
	assert.Equal(t, RiskDestructive, got.EffectiveRisk)
	assert.Contains(t, got.MatchedRule, "denylist_extra")
	assert.Contains(t, got.MatchedRule, "my-dangerous-tool")

	// A command that doesn't match the custom pattern is unaffected.
	benign := engine.Evaluate("ls -la", RiskRead)
	assert.Equal(t, Allow, benign.Action)
}

func TestNewEngineSkipsInvalidUserDenylistPatternWithOneStderrWarning(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.DenylistExtra = []string{"(unclosed", `\bmy-dangerous-tool\b`}

	var engine *Engine
	out := captureStderr(t, func() {
		engine = NewEngine(cfg)
	})

	require.Contains(t, out, "denylist_extra")
	require.Contains(t, out, "(unclosed")
	assert.Equal(t, 1, strings.Count(out, "\n"), "expected exactly one warning line for the one invalid pattern, got: %q", out)

	// The valid pattern alongside the invalid one still works.
	got := engine.Evaluate("my-dangerous-tool --run", RiskRead)
	assert.Equal(t, Block, got.Action)

	// Evaluate never re-validates or re-warns after construction.
	out2 := captureStderr(t, func() {
		engine.Evaluate("my-dangerous-tool --run", RiskRead)
	})
	assert.Empty(t, out2)
}

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stderr
	os.Stderr = w
	fn()
	require.NoError(t, w.Close())
	os.Stderr = orig

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestActionString(t *testing.T) {
	assert.Equal(t, "allow", Allow.String())
	assert.Equal(t, "confirm", Confirm.String())
	assert.Equal(t, "block", Block.String())
	assert.Equal(t, "unknown", Action(99).String())
}
