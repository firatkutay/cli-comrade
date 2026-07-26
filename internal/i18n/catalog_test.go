package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// formatVerbRe matches one fmt-style format verb (%s, %d, %q, %v, ...).
// Applied AFTER stripping literal "%%" escapes (fmt's own rule for a
// literal percent sign, which introduces no interpolation argument), so
// countFormatVerbs never miscounts a literal "%" as a verb.
var formatVerbRe = regexp.MustCompile(`%[a-zA-Z]`)

// countFormatVerbs returns how many fmt format verbs appear in s.
func countFormatVerbs(s string) int {
	return len(formatVerbRe.FindAllString(strings.ReplaceAll(s, "%%", ""), -1))
}

// TestCatalogsCoverIdenticalKeys is the bidirectional drift guard
// docs/history/UYGULAMA_PLANI.md FAZ 9 calls for: catalogEN and catalogTR must define
// exactly the same MessageID set, checked in BOTH directions, so a
// MessageID added to only one catalog (in either direction) fails CI
// instead of silently falling back at runtime — see Translator.T's
// fallback behavior, which would otherwise mask the gap.
func TestCatalogsCoverIdenticalKeys(t *testing.T) {
	for id := range catalogEN {
		if _, ok := catalogTR[id]; !ok {
			t.Errorf("MessageID %q is defined in catalogEN but missing from catalogTR", id)
		}
	}
	for id := range catalogTR {
		if _, ok := catalogEN[id]; !ok {
			t.Errorf("MessageID %q is defined in catalogTR but missing from catalogEN", id)
		}
	}
}

// TestCatalogsHaveNoEmptyValues guards against a copy-paste placeholder
// (an accidentally empty translation) in either catalog.
func TestCatalogsHaveNoEmptyValues(t *testing.T) {
	for id, v := range catalogEN {
		if v == "" {
			t.Errorf("catalogEN[%q] is empty", id)
		}
	}
	for id, v := range catalogTR {
		if v == "" {
			t.Errorf("catalogTR[%q] is empty", id)
		}
	}
}

// TestCatalogsHaveMatchingFormatVerbCounts is the format-verb parity
// guard TestCatalogsCoverIdenticalKeys/TestCatalogsHaveNoEmptyValues
// don't cover: for every MessageID present in both catalogs, EN and TR
// must use the SAME NUMBER of fmt format verbs (%s/%d/%q/%v/...). Only
// key-presence and non-emptiness were guarded before this test — an
// arg-count mismatch between languages (a translation that drops or
// adds an interpolation) shipped silently and would only surface as a
// "%!s(MISSING)"/"%!(EXTRA ...)" artifact in exactly ONE language, at
// render time, in production.
func TestCatalogsHaveMatchingFormatVerbCounts(t *testing.T) {
	for id, en := range catalogEN {
		tr, ok := catalogTR[id]
		if !ok {
			continue // TestCatalogsCoverIdenticalKeys already reports this gap
		}
		enCount := countFormatVerbs(en)
		trCount := countFormatVerbs(tr)
		if enCount != trCount {
			t.Errorf("MessageID %q: catalogEN has %d format verb(s) (%q) but catalogTR has %d (%q)", id, enCount, en, trCount, tr)
		}
	}
}

// declaredMessageIDs source-parses catalog.go's own MessageID const block
// (go/ast, since Go has no reflection over a const block — the set of
// declared MessageID values isn't enumerable at runtime the way
// catalogEN/catalogTR's map keys are) and returns every MessageID it
// declares, by its literal string value.
//
// This is issue #38's chosen fix (option 1 from the issue's own
// tradeoff writeup): a go/ast parse over catalog.go rather than a
// //go:generate'd list. Parsing needs no separate generation step for a
// contributor to remember to run — the exact gap this issue exists to
// close (a hand-maintained mirror nobody remembers to update) — at the
// cost of coupling this test to catalog.go's current, simple shape: one
// `const ( ... )` block, no iota, exactly one name and one string
// literal per ValueSpec (verified once, cheaply, by requireNonEmpty
// below: if a future reorganization changes that shape, this returns
// zero IDs and the test fails loudly instead of silently passing).
func declaredMessageIDs(t *testing.T) []MessageID {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed to resolve this test file's own path")
	}
	catalogPath := filepath.Join(filepath.Dir(thisFile), "catalog.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, catalogPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", catalogPath, err)
	}

	var ids []MessageID
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeIdent, ok := valueSpec.Type.(*ast.Ident)
			if !ok || typeIdent.Name != "MessageID" {
				continue // not a "Msg... MessageID = ..." declaration
			}
			for i := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s literal %s: %v", valueSpec.Names[i].Name, lit.Value, err)
				}
				ids = append(ids, MessageID(value))
			}
		}
	}
	return ids
}

// TestEveryDeclaredMessageIDIsInBothCatalogs is issue #38's core guard:
// TestCatalogsCoverIdenticalKeys/TestCatalogsHaveNoEmptyValues/
// TestCatalogsHaveMatchingFormatVerbCounts above all range over catalog
// MAP KEYS only, so a MessageID declared in catalog.go's const block but
// added to NEITHER catalogEN nor catalogTR is invisible to every one of
// them — Translator.T silently falls back to the bare MessageID string
// (translator.go's own documented fallback) for that ID, in either
// language, and CI stayed green. This test enumerates the const block
// itself (declaredMessageIDs, above) and checks each one against both
// catalogs directly, closing that gap.
//
// Verified failing (issue #38's own "prove it" requirement): temporarily
// adding `MsgTestGapCanary MessageID = "test_gap_canary"` to catalog.go's
// const block, with no matching catalogEN/catalogTR entry, turns this
// red — "MessageID \"test_gap_canary\" is declared in catalog.go's const
// block but missing from catalogEN" (and the catalogTR line right after
// it) — then passes again once the constant is reverted. See this PR's
// own description for the exact command output; the canary is not left
// in the tree.
func TestEveryDeclaredMessageIDIsInBothCatalogs(t *testing.T) {
	ids := declaredMessageIDs(t)
	if len(ids) == 0 {
		t.Fatal("parsed zero MessageID constants out of catalog.go — the go/ast parser's shape assumptions (one const block, no iota, one name/value per ValueSpec) are stale; see declaredMessageIDs' own doc comment")
	}

	for _, id := range ids {
		_, enOK := catalogEN[id]
		_, trOK := catalogTR[id]
		if !enOK {
			t.Errorf("MessageID %q is declared in catalog.go's const block but missing from catalogEN", id)
		}
		if !trOK {
			t.Errorf("MessageID %q is declared in catalog.go's const block but missing from catalogTR", id)
		}
	}
}
