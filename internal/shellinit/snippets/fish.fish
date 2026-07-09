set -g __comrade_last_cmd ""
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
