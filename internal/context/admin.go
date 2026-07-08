package context

// IsAdmin reports whether the current process runs with elevated
// privileges.
//
// On unix this is exact: geteuid()==0 means root, and known is always
// true.
//
// On windows, reliably detecting admin/UAC elevation needs the Windows
// token API (OpenProcessToken + GetTokenInformation via
// golang.org/x/sys/windows or similar), which this project does not
// depend on — CLAUDE.md's stack list has no such dependency, and adding
// one just for this best-effort signal is not warranted. So on windows
// this always returns (false, false): known=false means "not checked",
// never "confirmed non-admin". Callers (including FAZ 5/6's engine and
// safety packages) must not treat a windows IsAdmin()==(false, false)
// result as a safety signal — the safety engine's own elevated/
// destructive risk classification is the actual gate, independent of
// this flag.
func IsAdmin(goos string, geteuid func() int) (isAdmin bool, known bool) {
	if goos == "windows" {
		return false, false
	}
	if geteuid == nil {
		return false, false
	}
	return geteuid() == 0, true
}
