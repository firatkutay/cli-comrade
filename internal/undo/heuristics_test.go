package undo

import "testing"

// deriveCase is one Derive table-test case: name, the Recorded input, and
// the expected outcome — mirroring internal/safety's own evalCase table-
// test discipline (denylist_test.go). wantOK=false cases assert only
// that Derive refuses the shape (wantCommands/wantCaveat/wantRelative are
// ignored in that case) — a deliberate near-miss/negative proof, exactly
// like safety's own "near-miss is not a match" cases.
type deriveCase struct {
	name         string
	recorded     Recorded
	wantOK       bool
	wantCommands []string
	wantCaveat   string
	wantRelative bool
}

func runDeriveCases(t *testing.T, cases []deriveCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Derive(c.recorded)
			if ok != c.wantOK {
				t.Fatalf("Derive(%+v) ok = %v, want %v (got %+v)", c.recorded, ok, c.wantOK, got)
			}
			if !c.wantOK {
				return
			}
			if len(got.Commands) != len(c.wantCommands) {
				t.Fatalf("Derive(%+v).Commands = %v, want %v", c.recorded, got.Commands, c.wantCommands)
			}
			for i, want := range c.wantCommands {
				if got.Commands[i] != want {
					t.Errorf("Derive(%+v).Commands[%d] = %q, want %q", c.recorded, i, got.Commands[i], want)
				}
			}
			if got.Caveat != c.wantCaveat {
				t.Errorf("Derive(%+v).Caveat = %q, want %q", c.recorded, got.Caveat, c.wantCaveat)
			}
			if got.UsesRelativePath != c.wantRelative {
				t.Errorf("Derive(%+v).UsesRelativePath = %v, want %v", c.recorded, got.UsesRelativePath, c.wantRelative)
			}
		})
	}
}

func TestDeriveEmptyCommandNeverMatches(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{name: "empty command", recorded: Recorded{Command: "", GOOS: "linux"}, wantOK: false},
		{name: "whitespace-only command", recorded: Recorded{Command: "   ", GOOS: "linux"}, wantOK: false},
	})
}

func TestDeriveMkdir(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "mkdir relative path",
			recorded:     Recorded{Command: "mkdir foo", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"rmdir foo"},
			wantRelative: true,
		},
		{
			name:         "mkdir absolute path",
			recorded:     Recorded{Command: "mkdir /tmp/x", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"rmdir /tmp/x"},
			wantRelative: false,
		},
		{
			name:         "sudo mkdir preserves elevation prefix",
			recorded:     Recorded{Command: "sudo mkdir /etc/myapp", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"sudo rmdir /etc/myapp"},
			wantRelative: false,
		},
		{
			name:         "darwin mkdir behaves like linux",
			recorded:     Recorded{Command: "mkdir /Users/me/x", GOOS: "darwin"},
			wantOK:       true,
			wantCommands: []string{"rmdir /Users/me/x"},
			wantRelative: false,
		},
		{name: "mkdir -p is rejected (flag present)", recorded: Recorded{Command: "mkdir -p foo", GOOS: "linux"}, wantOK: false},
		{name: "mkdir -m mode is rejected (flag present)", recorded: Recorded{Command: "mkdir -m 0755 foo", GOOS: "linux"}, wantOK: false},
		{name: "mkdir with two paths is rejected", recorded: Recorded{Command: "mkdir foo bar", GOOS: "linux"}, wantOK: false},
		{name: "mkdir with no argument is rejected", recorded: Recorded{Command: "mkdir", GOOS: "linux"}, wantOK: false},
		{name: "mkdirp near-miss word boundary", recorded: Recorded{Command: "mkdirp foo", GOOS: "linux"}, wantOK: false},
	})
}

func TestDeriveMv(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "mv two relative paths",
			recorded:     Recorded{Command: "mv a b", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"mv b a"},
			wantRelative: true,
		},
		{
			name:         "mv two absolute paths",
			recorded:     Recorded{Command: "mv /etc/a /etc/b", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"mv /etc/b /etc/a"},
			wantRelative: false,
		},
		{
			name:         "sudo mv preserves elevation prefix",
			recorded:     Recorded{Command: "sudo mv /etc/a /etc/b", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"sudo mv /etc/b /etc/a"},
			wantRelative: false,
		},
		{
			name:         "mv one relative one absolute is still relative overall",
			recorded:     Recorded{Command: "mv a /etc/b", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"mv /etc/b a"},
			wantRelative: true,
		},
		{name: "mv with a flag is rejected", recorded: Recorded{Command: "mv -f a b", GOOS: "linux"}, wantOK: false},
		{name: "mv with three sources is rejected", recorded: Recorded{Command: "mv a b c", GOOS: "linux"}, wantOK: false},
		{name: "mv with wildcard source is rejected", recorded: Recorded{Command: "mv a*.txt b", GOOS: "linux"}, wantOK: false},
		{name: "mv with wildcard dest is rejected", recorded: Recorded{Command: "mv a b*", GOOS: "linux"}, wantOK: false},
		{name: "mv with too few args is rejected", recorded: Recorded{Command: "mv a", GOOS: "linux"}, wantOK: false},
	})
}

func TestDeriveUnixPackageInstall(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "apt install",
			recorded:     Recorded{Command: "apt install nginx", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"apt remove nginx"},
		},
		{
			name:         "sudo apt-get install with flag and package",
			recorded:     Recorded{Command: "sudo apt-get install -y docker.io", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"sudo apt-get remove -y docker.io"},
		},
		{
			name:         "dnf install",
			recorded:     Recorded{Command: "dnf install httpd", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"dnf remove httpd"},
		},
		{
			name:         "sudo dnf install",
			recorded:     Recorded{Command: "sudo dnf install httpd", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"sudo dnf remove httpd"},
		},
		{
			name:         "brew install",
			recorded:     Recorded{Command: "brew install wget", GOOS: "darwin"},
			wantOK:       true,
			wantCommands: []string{"brew uninstall wget"},
		},
		{
			name:         "apt install multiple packages preserves all args",
			recorded:     Recorded{Command: "apt install -y curl wget", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"apt remove -y curl wget"},
		},
		{name: "apt update is not install", recorded: Recorded{Command: "apt update", GOOS: "linux"}, wantOK: false},
		{name: "apt-get purge is not install", recorded: Recorded{Command: "apt-get purge nginx", GOOS: "linux"}, wantOK: false},
		{name: "brew upgrade is not install", recorded: Recorded{Command: "brew upgrade wget", GOOS: "darwin"}, wantOK: false},
		{name: "apt install with no package is rejected", recorded: Recorded{Command: "apt install", GOOS: "linux"}, wantOK: false},
		{name: "unrecognized manager is not matched", recorded: Recorded{Command: "yum install httpd", GOOS: "linux"}, wantOK: false},
	})
}

func TestDeriveSystemctl(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "systemctl enable",
			recorded:     Recorded{Command: "systemctl enable docker", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"systemctl disable docker"},
		},
		{
			name:         "systemctl start",
			recorded:     Recorded{Command: "systemctl start nginx", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"systemctl stop nginx"},
		},
		{
			name:         "sudo systemctl enable --now preserves flag and elevation",
			recorded:     Recorded{Command: "sudo systemctl enable --now docker", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"sudo systemctl disable --now docker"},
		},
		{
			name:         "systemctl start with --no-block flag",
			recorded:     Recorded{Command: "systemctl start --no-block nginx", GOOS: "linux"},
			wantOK:       true,
			wantCommands: []string{"systemctl stop --no-block nginx"},
		},
		{name: "systemctl status is not enable/start", recorded: Recorded{Command: "systemctl status docker", GOOS: "linux"}, wantOK: false},
		{name: "systemctl reload is not enable/start", recorded: Recorded{Command: "systemctl reload nginx", GOOS: "linux"}, wantOK: false},
		{name: "systemctl enable with no unit is rejected", recorded: Recorded{Command: "systemctl enable", GOOS: "linux"}, wantOK: false},
		{name: "systemctl enable with two units is rejected", recorded: Recorded{Command: "systemctl enable docker nginx", GOOS: "linux"}, wantOK: false},
	})
}

func TestDeriveWindowsNewItemDirectory(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "New-Item -ItemType Directory -Path",
			recorded:     Recorded{Command: `New-Item -ItemType Directory -Path C:\temp\x`, GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{`Remove-Item C:\temp\x`},
			wantCaveat:   "only removes the directory if it is still empty",
		},
		{
			name:         "New-Item -ItemType Directory positional path",
			recorded:     Recorded{Command: `New-Item -ItemType Directory C:\temp\x`, GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{`Remove-Item C:\temp\x`},
			wantCaveat:   "only removes the directory if it is still empty",
		},
		{
			name:         "New-Item flags reordered (-Path before -ItemType)",
			recorded:     Recorded{Command: `New-Item -Path C:\temp\x -ItemType Directory`, GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{`Remove-Item C:\temp\x`},
			wantCaveat:   "only removes the directory if it is still empty",
		},
		{
			name:         "New-Item case-insensitive flags and cmdlet name",
			recorded:     Recorded{Command: `new-item -itemtype directory -path C:\temp\x`, GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{`Remove-Item C:\temp\x`},
			wantCaveat:   "only removes the directory if it is still empty",
		},
		{
			name:         "New-Item relative positional path",
			recorded:     Recorded{Command: `New-Item -ItemType Directory sub\folder`, GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{`Remove-Item sub\folder`},
			wantCaveat:   "only removes the directory if it is still empty",
			wantRelative: true,
		},
		{name: "New-Item -ItemType File is not a directory", recorded: Recorded{Command: `New-Item -ItemType File -Path C:\temp\x.txt`, GOOS: "windows"}, wantOK: false},
		{name: "New-Item with an unrecognized flag is rejected", recorded: Recorded{Command: `New-Item -ItemType Directory -Path C:\temp\x -Force`, GOOS: "windows"}, wantOK: false},
		{name: "New-Item with no path at all is rejected", recorded: Recorded{Command: `New-Item -ItemType Directory`, GOOS: "windows"}, wantOK: false},
		{name: "New-Item with duplicate -Path is rejected", recorded: Recorded{Command: `New-Item -ItemType Directory -Path C:\a -Path C:\b`, GOOS: "windows"}, wantOK: false},
	})
}

func TestDeriveWindowsPackageInstall(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{
			name:         "winget install",
			recorded:     Recorded{Command: "winget install Git.Git", GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{"winget uninstall Git.Git"},
		},
		{
			name:         "scoop install",
			recorded:     Recorded{Command: "scoop install nvm", GOOS: "windows"},
			wantOK:       true,
			wantCommands: []string{"scoop uninstall nvm"},
		},
		{name: "winget upgrade is not install", recorded: Recorded{Command: "winget upgrade Git.Git", GOOS: "windows"}, wantOK: false},
		{name: "scoop update is not install", recorded: Recorded{Command: "scoop update nvm", GOOS: "windows"}, wantOK: false},
	})
}

// TestDeriveIsGOOSGated proves the per-GOOS table gating itself: a
// Windows-shaped command recorded under a non-Windows GOOS (and vice
// versa) is never matched — the two dialects' rules never bleed into
// each other, exactly like comrade's own executor never runs a
// PowerShell-shaped command through a POSIX shell or vice versa.
func TestDeriveIsGOOSGated(t *testing.T) {
	runDeriveCases(t, []deriveCase{
		{name: "New-Item under linux GOOS is not matched", recorded: Recorded{Command: `New-Item -ItemType Directory -Path C:\temp\x`, GOOS: "linux"}, wantOK: false},
		{name: "winget install under linux GOOS is not matched", recorded: Recorded{Command: "winget install Git.Git", GOOS: "linux"}, wantOK: false},
		{name: "mkdir under windows GOOS is not matched (New-Item is the Windows verb)", recorded: Recorded{Command: "mkdir foo", GOOS: "windows"}, wantOK: false},
		{name: "apt install under windows GOOS is not matched", recorded: Recorded{Command: "apt install nginx", GOOS: "windows"}, wantOK: false},
	})
}
