package shellinit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// These are golden copies of internal/shellinit/snippets/*. Any edit to
// a snippet file must update its matching literal here — that is the
// point: an accidental or unreviewed change to hook behavior fails this
// test instead of silently shipping.

const wantBashSnippet = `__comrade_last_cmd=""
__comrade_hook() {
  local ec=$?
  command -v comrade >/dev/null 2>&1 || return $ec
  local raw
  raw=$(HISTTIMEFORMAT= history 1 2>/dev/null)
  local cmd
  cmd=$(printf '%s' "$raw" | sed -E 's/^[[:space:]]*[0-9]+[[:space:]]+//')
  if [ -n "$cmd" ] && [ "$cmd" != "$__comrade_last_cmd" ]; then
    __comrade_last_cmd="$cmd"
    comrade hook record --shell bash --exit "$ec" --command "$cmd" >/dev/null 2>&1 || true
  fi
  return $ec
}
case ";${PROMPT_COMMAND:-};" in
  *";__comrade_hook;"*) ;;
  *) PROMPT_COMMAND="__comrade_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
`

const wantZshSnippet = `__comrade_last_cmd=""
__comrade_precmd() {
  local ec=$?
  command -v comrade >/dev/null 2>&1 || return $ec
  local cmd
  cmd=$(fc -ln -1 2>/dev/null)
  cmd="${cmd#"${cmd%%[![:space:]]*}"}"
  if [ -n "$cmd" ] && [ "$cmd" != "$__comrade_last_cmd" ]; then
    __comrade_last_cmd="$cmd"
    comrade hook record --shell zsh --exit "$ec" --command "$cmd" >/dev/null 2>&1 || true
  fi
  return $ec
}
if ! { autoload -Uz add-zsh-hook && add-zsh-hook precmd __comrade_precmd; } 2>/dev/null; then
  precmd() { __comrade_precmd; }
fi
`

const wantFishSnippet = `set -g __comrade_last_cmd ""
function __comrade_postexec --on-event fish_postexec
    set -l ec $status
    if not command -v comrade >/dev/null 2>&1
        return
    end
    set -l cmd $argv[1]
    if test -n "$cmd"; and test "$cmd" != "$__comrade_last_cmd"
        set -g __comrade_last_cmd "$cmd"
        comrade hook record --shell fish --exit $ec --command "$cmd" >/dev/null 2>&1
    end
end
`

const wantPowerShellSnippet = `if (Get-Command comrade -ErrorAction SilentlyContinue) {
    $global:__ComradeOriginalPrompt = $function:prompt
    $global:__ComradeLastCommand = $null
    function global:prompt {
        # $? MUST be read as the literal first statement: it reflects
        # whether the previous command succeeded, but ANY statement that
        # runs first -- even a plain assignment -- resets it to $true
        # before we ever get to look at it.
        $success = $?
        $lastExitCode = $global:LASTEXITCODE
        # $LASTEXITCODE only ever reflects a native (external) program's
        # own exit code -- it is untouched by a failed cmdlet, a
        # CommandNotFoundException ("pyton" typo'd for "python"), or any
        # other non-native error, and it is NOT reset on success either,
        # so a stale nonzero value can leak from an earlier native
        # failure into a later, genuinely successful command. $? is the
        # one signal that is always correct for "did the last command
        # succeed", for both native and non-native commands alike; when
        # it says success, $ec is always 0 regardless of $LASTEXITCODE's
        # staleness. When it says failure, prefer $LASTEXITCODE's own
        # nonzero value if this failure really was a native command's
        # (so the real exit code is preserved), otherwise fall back to a
        # generic 1 (e.g. command-not-found, where no native exit code
        # exists at all).
        $ec = 0
        if (-not $success) {
            if ($null -ne $lastExitCode -and $lastExitCode -ne 0) {
                $ec = $lastExitCode
            } else {
                $ec = 1
            }
        }
        try {
            $last = Get-History -Count 1
            if ($last) {
                $cmd = $last.CommandLine
                if ($cmd -and $cmd -ne $global:__ComradeLastCommand) {
                    $global:__ComradeLastCommand = $cmd
                    comrade hook record --shell powershell --exit $ec --command $cmd 2>$null | Out-Null
                }
            }
        } catch {
        }
        if ($global:__ComradeOriginalPrompt) {
            & $global:__ComradeOriginalPrompt
        } else {
            "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
        }
    }
}
`

func TestSnippetGoldenContent(t *testing.T) {
	cases := []struct {
		shell shellinit.Shell
		want  string
	}{
		{shellinit.Bash, wantBashSnippet},
		{shellinit.Zsh, wantZshSnippet},
		{shellinit.Fish, wantFishSnippet},
		{shellinit.PowerShell, wantPowerShellSnippet},
	}
	for _, tc := range cases {
		t.Run(string(tc.shell), func(t *testing.T) {
			got, err := shellinit.Snippet(tc.shell)
			require.NoError(t, err)
			// snippets/powershell.ps1 is forced to CRLF line endings by
			// .gitattributes (the native/correct convention for a file
			// written into a Windows $PROFILE) — a fresh checkout embeds
			// it with \r\n, while the golden literal above is an LF-only
			// Go string literal. Normalize \r\n -> \n on both sides
			// before comparing so this test pins actual content, not the
			// checkout's line-ending state; it is a no-op for the other
			// three shells, which are never CRLF.
			assert.Equal(t, normalizeCRLF(tc.want), normalizeCRLF(got))
		})
	}
}

// normalizeCRLF strips \r immediately before \n so golden comparisons are
// insensitive to whether the source file was checked out with LF or
// CRLF line endings (see .gitattributes' `*.ps1 text eol=crlf`).
func normalizeCRLF(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func TestBlockWrapsSnippetInExactMarkers(t *testing.T) {
	got, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)

	want := shellinit.MarkerBegin + "\n" +
		"__comrade_last_cmd=\"\"\n" +
		"__comrade_hook() {\n" +
		"  local ec=$?\n" +
		"  command -v comrade >/dev/null 2>&1 || return $ec\n" +
		"  local raw\n" +
		"  raw=$(HISTTIMEFORMAT= history 1 2>/dev/null)\n" +
		"  local cmd\n" +
		"  cmd=$(printf '%s' \"$raw\" | sed -E 's/^[[:space:]]*[0-9]+[[:space:]]+//')\n" +
		"  if [ -n \"$cmd\" ] && [ \"$cmd\" != \"$__comrade_last_cmd\" ]; then\n" +
		"    __comrade_last_cmd=\"$cmd\"\n" +
		"    comrade hook record --shell bash --exit \"$ec\" --command \"$cmd\" >/dev/null 2>&1 || true\n" +
		"  fi\n" +
		"  return $ec\n" +
		"}\n" +
		"case \";${PROMPT_COMMAND:-};\" in\n" +
		"  *\";__comrade_hook;\"*) ;;\n" +
		"  *) PROMPT_COMMAND=\"__comrade_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}\" ;;\n" +
		"esac\n" +
		shellinit.MarkerEnd
	assert.Equal(t, want, got)
	assert.False(t, strings.HasSuffix(got, "\n"), "Block must not end with a trailing newline")
}

func TestSnippetUnsupportedShellErrors(t *testing.T) {
	_, err := shellinit.Snippet(shellinit.Shell("tcsh"))
	assert.ErrorContains(t, err, "tcsh")
}

func TestBlockUnsupportedShellErrors(t *testing.T) {
	_, err := shellinit.Block(shellinit.Shell("tcsh"))
	assert.ErrorContains(t, err, "tcsh")
}
