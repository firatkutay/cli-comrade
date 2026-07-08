package context

// packageManagerNames is the fixed, stable scan order DetectPackageManagers
// uses — CLAUDE.md's "Paket yöneticisi tespiti" list.
var packageManagerNames = []string{
	"apt", "dnf", "pacman", "zypper", "brew", "port", "winget", "scoop", "choco",
}

// DetectPackageManagers scans PATH (via lookPath, typically exec.LookPath)
// for each known package manager binary and returns the ones found, in
// packageManagerNames' fixed order — never the scan order, so the result
// is stable across runs regardless of which PATH directories a manager
// happens to live in.
func DetectPackageManagers(lookPath func(string) (string, error)) []string {
	if lookPath == nil {
		return nil
	}
	found := make([]string, 0, len(packageManagerNames))
	for _, name := range packageManagerNames {
		if _, err := lookPath(name); err == nil {
			found = append(found, name)
		}
	}
	return found
}
