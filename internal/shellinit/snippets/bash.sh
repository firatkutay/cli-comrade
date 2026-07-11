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
command -v comrade >/dev/null 2>&1 && source <(comrade completion bash)
# bash's readline has no ghost-text/auto-list primitive comrade can hook
# without rebinding the space key itself (which would break magic-space,
# multiline editing, and paste) — unlike zsh/PowerShell above, there is
# no space-triggered hint here. Press Tab twice after "comrade " (or any
# subcommand) for the same next-word list via the completion sourced on
# the line above.
