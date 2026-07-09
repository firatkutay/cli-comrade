//go:build !windows

package i18n

// SystemLocale returns "" on every platform except Windows. On
// Linux/macOS the OS locale mechanism IS the LANG/LC_ALL environment
// convention that ResolveLanguage already consults directly (step 3 of
// its precedence chain) — there is no separate, lower-level "OS locale
// API" to probe that could ever say anything LANG/LC_ALL didn't already
// have the chance to say, so a non-empty return here would only ever be
// a redundant duplicate, never new information. See locale_windows.go
// (built only on GOOS=windows) for the one platform where a real,
// distinct OS-level locale API exists and is used — and for why this
// pair of files is a documented runtime.GOOS exception: one of the
// codebase's two build-tag isolations, alongside internal/executor's
// process-group syscall pair (executor_unix.go/executor_windows.go).
func SystemLocale() string {
	return ""
}
