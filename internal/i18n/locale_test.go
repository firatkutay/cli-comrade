package i18n

import (
	"runtime"
	"testing"
)

// TestSystemLocaleOnNonWindowsIsAlwaysEmpty pins locale_other.go's
// contract on this test binary's host GOOS: SystemLocale must return ""
// unconditionally. On an actual `GOOS=windows` build this file still
// compiles and runs (it makes no windows-only calls itself), but the
// assertion is skipped there since locale_windows.go's real
// GetUserDefaultLocaleName result is environment-dependent and cannot be
// pinned to a fixed value in CI — locale_windows.go's own doc comment
// covers that file's contract instead, and ResolveLanguage's injectable
// systemLocale parameter (lang_test.go) is what makes the Windows
// decision path itself testable without a real Windows host.
func TestSystemLocaleOnNonWindowsIsAlwaysEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SystemLocale is expected to return a real, environment-dependent value on windows — see locale_windows.go")
	}
	if got := SystemLocale(); got != "" {
		t.Fatalf("SystemLocale() = %q on GOOS=%s, want \"\"", got, runtime.GOOS)
	}
}
