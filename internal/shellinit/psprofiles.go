package shellinit

import (
	stdctx "context"
	"errors"
)

// PSVariant identifies one PowerShell binary/profile pair "comrade init
// powershell" can target on GOOS=windows, where two independent
// PowerShell installations commonly coexist.
type PSVariant string

const (
	// PSVariantWindowsPowerShell is Windows PowerShell 5.1 (the "powershell"
	// binary) — present on every Windows machine out of the box, never
	// present on any other GOOS.
	PSVariantWindowsPowerShell PSVariant = "powershell"
	// PSVariantPwsh is PowerShell 7+ (the "pwsh" binary) — an optional,
	// separately-installed edition on Windows; the only PowerShell edition
	// on non-Windows GOOS.
	PSVariantPwsh PSVariant = "pwsh"
)

// Label renders v as the human-readable product name shown in "comrade
// init powershell"'s per-profile report lines (MsgInitPSVariant*). This
// is deliberately NOT routed through internal/i18n: "Windows PowerShell
// 5.1" and "PowerShell 7" are Microsoft's own product names, not prose —
// the same category as a risk-class name or a credential-source name
// (see i18n.MsgAuthStatusSet's doc comment for the precedent), left
// untranslated by this project's established convention.
func (v PSVariant) Label() string {
	switch v {
	case PSVariantWindowsPowerShell:
		return "Windows PowerShell 5.1"
	case PSVariantPwsh:
		return "PowerShell 7"
	default:
		return string(v)
	}
}

// PSProfile is one PowerShell variant's resolved (or failed-to-resolve)
// profile path — RCPath's single-profile (path, ok, note) shape, plus
// which variant it belongs to, so a caller handling several variants at
// once (ResolvePowerShellProfiles) can report and act on each
// independently.
type PSProfile struct {
	Variant PSVariant
	Path    string
	OK      bool
	Note    string
}

// ErrNoPowerShellFound is ResolvePowerShellProfiles' error return when
// NOT A SINGLE candidate variant's binary could be found on PATH at
// all — the one case "comrade init powershell" cannot proceed with at
// all. It is a sentinel, not a formatted message: the user-facing text
// belongs in internal/i18n's catalog (internal/cli/init.go wraps it via
// i18n.MsgInitPowerShellNoneFoundError) — this package has no i18n
// dependency by design, exactly like every other note string RCPath
// itself already returns (see rcpath.go).
var ErrNoPowerShellFound = errors.New("shellinit: no PowerShell installation found on PATH")

// ResolvePowerShellProfiles finds every PowerShell variant installed on
// this machine and resolves each one's own $PROFILE path independently,
// so "comrade init powershell" can install/upgrade/remove its hook in
// EVERY variant's profile on GOOS=windows — not just whichever one
// resolvePowerShellProfile's single goos-keyed guess would have picked
// (the "pwsh gap": see docs/history/PROGRESS.md's Tamamlandı note for the bug
// this closes).
//
// Candidate selection:
//   - non-Windows goos: PSVariantPwsh only — Windows PowerShell does not
//     exist off Windows. This function is not actually used on
//     non-Windows by internal/cli/init.go (which keeps calling RCPath's
//     original single-profile PowerShell branch there — see init.go's
//     dispatch comment), but behaves this way for symmetry and so it is
//     table-testable from any host GOOS.
//   - GOOS=windows: PSVariantWindowsPowerShell AND PSVariantPwsh, both
//     probed via lookPath.
//
// A candidate variant whose BINARY is not found on PATH is simply absent
// from the returned slice — "not installed" is not a failure for THAT
// variant, only a reason to skip it; a machine with only one of the two
// Windows variants installed (the common case before PowerShell 7
// adoption, or a locked-down image without Windows PowerShell) is not an
// error. A variant whose binary IS found but whose own `$PROFILE` query
// then fails is still included, as a PSProfile with OK=false and an
// explanatory Note — one variant's resolution failure never hides
// another variant's success; the caller reports and skips that one
// PSProfile and proceeds with the rest.
//
// err is non-nil (ErrNoPowerShellFound) ONLY when the returned slice
// would otherwise be empty — no candidate variant's binary was found on
// PATH at all.
func ResolvePowerShellProfiles(ctx stdctx.Context, goos string, lookPath func(string) (string, error), run CommandRunner) ([]PSProfile, error) {
	candidates := []PSVariant{PSVariantPwsh}
	if goos == "windows" {
		candidates = []PSVariant{PSVariantWindowsPowerShell, PSVariantPwsh}
	}

	var profiles []PSProfile
	for _, variant := range candidates {
		bin := string(variant)
		if lookPath == nil {
			continue
		}
		if _, err := lookPath(bin); err != nil {
			continue
		}
		if run == nil {
			profiles = append(profiles, PSProfile{
				Variant: variant,
				OK:      false,
				Note:    "cannot resolve PowerShell profile path: no way to query " + bin,
			})
			continue
		}
		path, ok, note := queryProfilePath(ctx, bin, run)
		profiles = append(profiles, PSProfile{Variant: variant, Path: path, OK: ok, Note: note})
	}

	if len(profiles) == 0 {
		return nil, ErrNoPowerShellFound
	}
	return profiles, nil
}
