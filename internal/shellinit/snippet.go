package shellinit

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed snippets/bash.sh
var bashSnippet string

//go:embed snippets/zsh.sh
var zshSnippet string

//go:embed snippets/fish.fish
var fishSnippet string

//go:embed snippets/powershell.ps1
var powershellSnippet string

// MarkerBegin and MarkerEnd delimit cli-comrade's installed block inside
// a shell rc/profile file. PowerShell also accepts "#"-prefixed
// comments, so the same two literal lines are used for every shell —
// per UYGULAMA_PLANI.md FAZ 4, one marker pair, not four.
const (
	MarkerBegin = "# >>> cli-comrade init >>>"
	MarkerEnd   = "# <<< cli-comrade init <<<"
)

// Snippet returns the raw hook body embedded for shell — the file
// contents under internal/shellinit/snippets/, unmodified except for a
// single trailing-newline trim performed by Block below. Golden tests
// pin this exact text so an edit to any snippets/*.{sh,fish,ps1} file
// can only ship intentionally.
func Snippet(shell Shell) (string, error) {
	switch shell {
	case Bash:
		return bashSnippet, nil
	case Zsh:
		return zshSnippet, nil
	case Fish:
		return fishSnippet, nil
	case PowerShell:
		return powershellSnippet, nil
	default:
		return "", fmt.Errorf("shellinit: no snippet for shell %q", shell)
	}
}

// Block wraps shell's Snippet in the MarkerBegin/MarkerEnd delimiter
// lines, ready to be inserted into (or matched against) an rc file by
// ApplyBlock/RemoveBlock. The returned string never has a leading or
// trailing newline of its own; callers decide how to join it with
// surrounding file content.
func Block(shell Shell) (string, error) {
	body, err := Snippet(shell)
	if err != nil {
		return "", err
	}
	body = strings.TrimRight(body, "\n")
	return MarkerBegin + "\n" + body + "\n" + MarkerEnd, nil
}
