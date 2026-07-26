__comrade_last_cmd=""
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
if command -v comrade >/dev/null 2>&1 && whence compdef >/dev/null 2>&1; then
  # See bash.sh's matching guard: never source/eval comrade's completion
  # output unguarded -- a broken comrade prints its diagnostic to stderr
  # (discarded by 2>/dev/null) and leaves stdout empty, so only eval
  # when there is a non-empty script to eval.
  __comrade_completion="$(comrade completion zsh 2>/dev/null)"
  [ -n "$__comrade_completion" ] && eval "$__comrade_completion"
  unset __comrade_completion
fi
if [[ -o interactive ]] && zmodload zsh/zle 2>/dev/null; then
  typeset -g __comrade_hint_key="" __comrade_hint_text="" __comrade_hint_owns=0
  __comrade_hint_widget() {
    if [[ $BUFFER == comrade\ * && $BUFFER == *' ' ]]; then
      local key="${BUFFER%% }"
      if [[ $key != $__comrade_hint_key ]]; then
        __comrade_hint_key=$key
        # "${(@z)BUFFER}" (quoted, @-flag) rather than ${(z)BUFFER}
        # (bare): unquoted (z)-split words are glob-eligible under
        # 'setopt GLOB_SUBST', so a buffer word containing a glob
        # metacharacter can trigger "no matches found" leaking to the
        # terminal mid-redraw. The quoted "${(@z)}" form keeps (z)'s own
        # per-word tokenization (the "@" flag preserves the array split
        # across the double quotes) while suppressing further glob/
        # re-splitting of each resulting word.
        __comrade_hint_text=$(comrade __hint -- "${(@z)BUFFER}" 2>/dev/null)
      fi
      if [[ -n $__comrade_hint_text && ( -z $POSTDISPLAY || $__comrade_hint_owns -eq 1 ) ]]; then
        POSTDISPLAY=" $__comrade_hint_text"
        __comrade_hint_owns=1
        region_highlight=( ${region_highlight:#*memo=comrade} )
        region_highlight+=("$#BUFFER $(($#BUFFER + $#POSTDISPLAY)) fg=8 memo=comrade")
      fi
    elif (( __comrade_hint_owns )); then
      POSTDISPLAY=""
      __comrade_hint_owns=0
      __comrade_hint_key=""
      region_highlight=( ${region_highlight:#*memo=comrade} )
    fi
  }
  autoload -Uz add-zle-hook-widget 2>/dev/null && command -v comrade >/dev/null 2>&1 && add-zle-hook-widget line-pre-redraw __comrade_hint_widget
fi
