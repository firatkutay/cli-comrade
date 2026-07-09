module github.com/firatkutay/cli-comrade

go 1.25.0

toolchain go1.26.5

require (
	charm.land/bubbles/v2 v2.1.1
	charm.land/bubbletea/v2 v2.0.8
	charm.land/lipgloss/v2 v2.0.5
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/term v0.45.0
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260703014108-f5a850f9c2b7 // indirect
	github.com/charmbracelet/x/ansi v0.11.7 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// FAZ 11 cold-start hardening: github.com/atotto/clipboard v0.1.4's Unix
// build (clipboard_unix.go) runs up to five sequential exec.LookPath PATH
// scans unconditionally in a package-level init() — paid by every
// process that imports charm.land/bubbles/v2/textinput (internal/tui and
// internal/cli's chat model both do, for the ask-mode confirm prompt and
// `comrade chat`), i.e. every comrade invocation, including --version and
// --help, whether or not it ever touches the clipboard. On a PATH with
// many entries (observed: 100+ on a WSL2 shell, most 9p/DrvFs-mounted)
// this cost hundreds of milliseconds on every single command. This
// replace points at a locally vendored, behavior-preserving copy
// (third_party/atotto-clipboard) whose only change is deferring that
// same probe from init() to a sync.Once triggered by first actual
// clipboard use — see its clipboard_unix.go doc comment and
// docs/phases/FAZ-11.md / KNOWN_LIMITATIONS.md for the full rationale.
// No newer upstream release exists (v0.1.4 is latest) to pick up a fix
// instead.
replace github.com/atotto/clipboard => ./third_party/atotto-clipboard
