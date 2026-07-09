//go:build windows

package i18n

import (
	"syscall"
	"unsafe"
)

// localeNameMaxLength is LOCALE_NAME_MAX_LENGTH (winnt.h): the maximum
// number of UTF-16 code units a Windows locale name can occupy,
// including its terminating null character — 85, per
// https://learn.microsoft.com/windows/win32/intl/locale-name-constants
// (verified against current Microsoft Learn docs, not recalled from
// memory). Hardcoded here rather than derived from any Go stdlib
// constant (none exists) because GetUserDefaultLocaleName's own
// contract requires exactly this buffer size.
const localeNameMaxLength = 85

// modKernel32/procGetUserDefaultLocaleName are resolved lazily —
// syscall.NewLazyDLL/NewProc never touch the DLL until the first Call —
// though kernel32.dll is in practice already loaded into every Windows
// process by the OS loader regardless, so this costs nothing beyond the
// one Call SystemLocale itself performs.
var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultLocaleName = modKernel32.NewProc("GetUserDefaultLocaleName")
)

// SystemLocale returns the current user's Windows locale name — a BCP-47
// tag such as "tr-TR" or "en-US" — via the kernel32 GetUserDefaultLocaleName
// API (https://learn.microsoft.com/windows/win32/api/winnls/nf-winnls-getuserdefaultlocalename:
// "int GetUserDefaultLocaleName([out] LPWSTR lpLocaleName, [in] int
// cchLocaleName)", returning the written length including the null
// terminator on success or 0 on failure), or "" if the syscall fails for
// any reason (ResolveLanguage's caller then falls through to English,
// exactly as if this probe didn't exist).
//
// This file (and locale_other.go, built on every other GOOS) is a
// justified, documented exception to CLAUDE.md's "platform dallanmaları
// build tag ile DEĞİL, runtime runtime.GOOS + internal/executor
// soyutlamasıyla" preference — one of the codebase's two build-tag
// isolations, alongside internal/executor's process-group syscall pair
// (executor_unix.go/executor_windows.go): both exist for the identical
// reason, a raw platform syscall cannot compile on the other GOOS
// (kernel32.dll and the syscall.LazyDLL machinery for it do not exist on
// non-Windows; syscall.SysProcAttr's Setpgid field does not exist on
// Windows), so a build tag is the only way to isolate either one — there
// is no runtime.GOOS branch that would let a function using either API
// live in one cross-platform file. The runtime.GOOS rule's actual intent
// (three-OS testability from one binary, per CLAUDE.md's "üç OS testi
// kolaylaşır" rationale) is preserved anyway: every DECISION this
// package makes — ResolveLanguage, parseLocaleLikeValue, the whole
// precedence chain — stays pure, platform-independent Go, fully
// unit-tested on every GOOS via ResolveLanguage's injectable
// systemLocale parameter (see lang_test.go). Only this one, unavoidably
// Windows-specific raw syscall — which does nothing but return a
// string, no decision logic at all — needed a build-tagged home.
func SystemLocale() string {
	buf := make([]uint16, localeNameMaxLength)
	ret, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])), //nolint:gosec // required by GetUserDefaultLocaleName's own LPWSTR-out-parameter contract; buf is a same-function-local slice sized exactly localeNameMaxLength, never reused or freed concurrently.
		uintptr(len(buf)),
	)
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
