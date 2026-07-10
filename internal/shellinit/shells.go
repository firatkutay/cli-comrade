package shellinit

import "fmt"

// Shell identifies one of the four shells cli-comrade installs a hook
// for. It is a defined string type (not a bare string) so ParseShell is
// the only supported way to obtain one from user input.
type Shell string

// The four shells "comrade init" supports, matching CLAUDE.md's "Shell
// Entegrasyonu" section and docs/history/UYGULAMA_PLANI.md FAZ 4 exactly.
const (
	Bash       Shell = "bash"
	Zsh        Shell = "zsh"
	Fish       Shell = "fish"
	PowerShell Shell = "powershell"
)

// All lists every supported shell, in the fixed display order used by
// error messages below.
var All = []Shell{Bash, Zsh, Fish, PowerShell}

// ParseShell validates name against the supported shell set and returns
// the corresponding Shell. An empty or unrecognized name is an error
// naming every supported shell, so the caller (comrade init's error
// path) never has to duplicate that list itself.
func ParseShell(name string) (Shell, error) {
	for _, s := range All {
		if string(s) == name {
			return s, nil
		}
	}
	return "", fmt.Errorf("unsupported shell %q (expected one of: bash, zsh, fish, powershell)", name)
}
