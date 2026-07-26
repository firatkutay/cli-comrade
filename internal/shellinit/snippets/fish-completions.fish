if command -v comrade >/dev/null 2>&1
    # Never pipe comrade's completion output straight into `source`
    # unguarded: if comrade is on PATH but broken (e.g. an npm install
    # that landed the dispatcher without its platform binary), its
    # diagnostic goes to stderr (discarded below) and stdout is empty.
    # Capture first, and only source a non-empty script -- `count`
    # avoids the list-in-double-quotes splitting pitfall a bare
    # `test -n "$var"` would hit on the normal multi-line case.
    set -l __comrade_completion (comrade completion fish 2>/dev/null)
    if test (count $__comrade_completion) -gt 0
        string join \n -- $__comrade_completion | source
    end
end
