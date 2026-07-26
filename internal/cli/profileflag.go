package cli

import (
	"errors"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// errProfileFlagMissingValue is extractProfileFlag's sentinel for a
// trailing, valueless "--profile" token — see its own doc comment.
var errProfileFlagMissingValue = errors.New("--profile: missing value")

// extractProfileFlag hand-parses a "--profile <value>" / "--profile=<value>"
// token pair out of args — wherever it appears, not just at the front —
// for the three leaf commands (`config set`, `config profile set`,
// `explain`) that set
// DisableFlagParsing: true and therefore never let cobra's own flag
// parser see root's persistent --profile flag at all (see issue #27 —
// cobra's Command.ParseFlags returns immediately, a no-op, whenever
// DisableFlagParsing is set, which skips inherited persistent flags too,
// not just the leaf's own). Without this, --profile arrives in args as a
// literal, unconsumed string token: config set/config profile set fail
// their own fixed-arity check on the leaked tokens, and explain silently
// folds them into the text it explains.
//
// Scanning stops at the first literal "--" separator: everything at or
// after it (INCLUDING the "--" itself) is copied into rest verbatim and
// never inspected for "--profile" — this is what lets `comrade explain
// -- --profile weird` keep explaining the literal string "--profile
// weird" instead of this helper swallowing it, exactly mirroring
// explain.go's own pre-existing "--" escape hatch (and giving config
// set/config profile set the same escape hatch for a value that must
// legitimately BE the string "--profile", which they didn't have
// before). Every other "-"-prefixed token (a negative number, a raw flag
// value config.Validate exists to reject with a clear message) is left
// completely untouched, matching every DisableFlagParsing command's own
// documented reason for turning cobra's flag parsing off in the first
// place.
//
// A "--profile" with no following token (nothing left to consume as its
// value) is reported via err (errProfileFlagMissingValue) rather than
// silently dropped or left as a stray positional argument for the
// caller's own arity check to (mis)report.
func extractProfileFlag(args []string) (profile string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i:]...)
			return profile, rest, nil
		}
		if arg == "--profile" {
			if i+1 >= len(args) {
				return "", nil, errProfileFlagMissingValue
			}
			profile = args[i+1]
			i++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--profile="); ok {
			profile = value
			continue
		}
		rest = append(rest, arg)
	}
	return profile, rest, nil
}

// loaderWithProfileOverride returns newLoader unchanged when profile is
// empty (the common case: no --profile token was present in this
// invocation's raw args). When profile is non-empty, it returns a fresh
// loaderFactory that threads profile into config.NewLoaderWithProfile as
// the active-profile override instead — the same override root.go's own
// newLoader would have applied had cobra been allowed to parse --profile
// normally (see extractProfileFlag's doc comment for why it wasn't).
//
// Building a brand-new Loader with path "" here (rather than somehow
// recovering the path the injected newLoader itself resolved) is safe:
// both root.go's real newLoader and every test's newTestLoaderFactory
// (auth_test.go) always call config.NewLoaderWithProfile with an empty
// path, letting it resolve the platform-default location itself — there
// is no second, distinct path this package ever threads through
// loaderFactory.
func loaderWithProfileOverride(newLoader loaderFactory, profile string) loaderFactory {
	if profile == "" {
		return newLoader
	}
	return func() (*config.Loader, error) {
		return config.NewLoaderWithProfile("", profile)
	}
}

// stripFirstDoubleDash removes the first literal "--" element from args,
// if present, leaving every other token (including one that legitimately
// starts with "-") completely untouched — config set/config profile
// set's OWN "--" escape hatch, used only by those two commands, NOT
// explain (which keeps its own pre-existing, leading-only "--" handling
// unchanged — see newExplainCmd's doc comment for why that one can't be
// unified with this one: explain's "--" always marks "everything after
// this is one literal joined string", so it must run BEFORE explain's
// own --usage-stripping loop can misparse a literal "--usage" that only
// looks like the real flag because a leading "--" shielded it).
//
// config set/config profile set have no such free-text tail: their
// positional arguments are a small, FIXED count (2, respectively 3), so
// a lone "--" can appear ANYWHERE — not just leading — and still
// unambiguously act as a zero-width "the next token is literal, not one
// of my own flags" marker. This is what lets `comrade config set
// some.key -- --profile` set some.key to the literal value "--profile"
// (extractProfileFlag's own "--" boundary already protects the
// "--profile" token itself from being extracted; this is the second
// half — removing the marker so the arity check still sees exactly the
// expected number of positional arguments).
func stripFirstDoubleDash(args []string) []string {
	for i, arg := range args {
		if arg != "--" {
			continue
		}
		rest := make([]string, 0, len(args)-1)
		rest = append(rest, args[:i]...)
		rest = append(rest, args[i+1:]...)
		return rest
	}
	return args
}
