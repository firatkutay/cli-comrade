__comrade_last_cmd=""
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
if command -v comrade >/dev/null 2>&1; then
  # Never `source <(comrade completion bash)` unguarded: if comrade is on
  # PATH but broken (e.g. an npm install that landed the dispatcher
  # without its platform binary), its diagnostic goes to stderr, which
  # this 2>/dev/null discards, leaving stdout empty. Only eval when
  # there is something non-empty to eval, so a broken comrade never
  # spams -- or breaks -- every new shell.
  __comrade_completion="$(comrade completion bash 2>/dev/null)"
  [ -n "$__comrade_completion" ] && eval "$__comrade_completion"
  unset __comrade_completion
fi
# bash's readline has no ghost-text/auto-list primitive comrade can hook
# without rebinding the space key itself (which would break magic-space,
# multiline editing, and paste) — unlike zsh/PowerShell above, there is
# no space-triggered hint here. Press Tab twice after "comrade " (or any
# subcommand) for the same next-word list via the completion loaded
# above.
