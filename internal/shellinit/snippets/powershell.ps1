if (Get-Command comrade -ErrorAction SilentlyContinue) {
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
    comrade completion powershell | Out-String | Invoke-Expression
    try {
        $existingSpacebarHandler = Get-PSReadLineKeyHandler -Chord Spacebar -ErrorAction SilentlyContinue | Where-Object { $_.Function -ne 'SelfInsert' -and $_.Function }
        if ($null -eq $existingSpacebarHandler) {
            Set-PSReadLineKeyHandler -Chord Spacebar -BriefDescription 'comrade hint' -ScriptBlock {
                param($key, $arg)
                [Microsoft.PowerShell.PSConsoleReadLine]::Insert(' ')
                $line = $null
                $cursor = 0
                [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line, [ref]$cursor)
                if ($line -match '^\s*comrade(\.exe)?(\s+[\w-]+)*\s$') {
                    [Microsoft.PowerShell.PSConsoleReadLine]::PossibleCompletions()
                }
            }
        }
    } catch {
    }
}
