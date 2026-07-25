package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNpmPlatformMatrixIsConsistentAcrossGoreleaserAndNpmPackaging is the
// Derive-or-Guard bidirectional check for the npm-distribution platform
// matrix -- hand-maintained in FOUR places: .goreleaser.yaml's builds[]
// goos/goarch/ignore (what actually gets cross-compiled), npm/main/bin/
// platform-map.js's PLATFORM_PACKAGES (what the dispatcher will resolve),
// scripts/build-npm-packages.sh's PLATFORMS[] (what gets assembled into an
// npm package), and that same script's KNOWN_PKG_DIR_NAMES (what the
// assembly script's own out-dir safety check recognizes as "one of ours").
//
// Unlike TestReleaseArchiveNamingIsConsistentAcrossGoreleaserInstallScripts
// AndUpdatePackage above, a goreleaser-only addition to this matrix was
// previously INVISIBLE: the binary gets built, no npm package is ever
// assembled for it, and a user on that platform just sees "not part of the
// npm distribution matrix" -- nothing red in CI. This test fails on ANY of
// the four drifting from the others, in either direction.
func TestNpmPlatformMatrixIsConsistentAcrossGoreleaserAndNpmPackaging(t *testing.T) {
	root := repoRoot(t)

	goreleaserYAML := readRepoFile(t, root, ".goreleaser.yaml")
	platformMapJS := readRepoFile(t, root, "npm", "main", "bin", "platform-map.js")
	buildScript := readRepoFile(t, root, "scripts", "build-npm-packages.sh")

	// --- 1. .goreleaser.yaml's effective goos x goarch matrix -----------

	goosList := extractYAMLScalarList(t, goreleaserYAML, "goos")
	goarchList := extractYAMLScalarList(t, goreleaserYAML, "goarch")
	ignoredPairs := extractGoreleaserIgnoredPairs(t, goreleaserYAML)

	var goreleaserPairs []string
	for _, goos := range goosList {
		for _, goarch := range goarchList {
			pair := goos + "/" + goarch
			if ignoredPairs[pair] {
				continue
			}
			goreleaserPairs = append(goreleaserPairs, pair)
		}
	}
	require.NotEmpty(t, goreleaserPairs, "sanity: .goreleaser.yaml must resolve to at least one non-ignored goos/goarch pair")

	// --- 2. scripts/build-npm-packages.sh's PLATFORMS[] -----------------

	platformsLines := extractBashQuotedArray(t, buildScript, "PLATFORMS")
	scriptGoPairs := make([]string, 0, len(platformsLines))
	scriptNpmKeys := make([]string, 0, len(platformsLines))
	for _, line := range platformsLines {
		fields := strings.Split(line, ":")
		require.Lenf(t, fields, 5,
			"PLATFORMS entry %q must have exactly 5 colon-separated fields (goos:goarch:npm_os:npm_cpu:binary)", line)
		goos, goarch, npmOS, npmCPU := fields[0], fields[1], fields[2], fields[3]
		scriptGoPairs = append(scriptGoPairs, goos+"/"+goarch)
		scriptNpmKeys = append(scriptNpmKeys, npmOS+"-"+npmCPU)
	}
	require.NotEmpty(t, scriptGoPairs, "sanity: scripts/build-npm-packages.sh's PLATFORMS[] must not be empty")

	// --- 3. npm/main/bin/platform-map.js's PLATFORM_PACKAGES keys -------

	platformMapKeys := extractPlatformMapKeys(t, platformMapJS)
	require.NotEmpty(t, platformMapKeys, "sanity: npm/main/bin/platform-map.js's PLATFORM_PACKAGES must not be empty")

	// --- 4. scripts/build-npm-packages.sh's KNOWN_PKG_DIR_NAMES ---------

	knownPkgDirNames := extractBashWordArray(t, buildScript, "KNOWN_PKG_DIR_NAMES")

	// --- cross-checks: bidirectional set equality at every hop ----------
	// assert.ElementsMatch ignores order but fails if either side has an
	// element the other lacks -- exactly "set equality", in both
	// directions, at once.

	assert.ElementsMatchf(t, goreleaserPairs, scriptGoPairs,
		".goreleaser.yaml's effective goos x goarch matrix (minus ignore) must exactly match "+
			"scripts/build-npm-packages.sh's PLATFORMS[] goos:goarch pairs -- a target added to "+
			"EITHER file without the other must fail here.\ngoreleaser: %v\nPLATFORMS[]: %v",
		goreleaserPairs, scriptGoPairs)

	assert.ElementsMatchf(t, scriptNpmKeys, platformMapKeys,
		"scripts/build-npm-packages.sh's PLATFORMS[] npm_os-npm_cpu keys must exactly match "+
			"npm/main/bin/platform-map.js's PLATFORM_PACKAGES keys.\nPLATFORMS[]: %v\nplatform-map.js: %v",
		scriptNpmKeys, platformMapKeys)

	expectedPkgDirNames := append([]string{"cli-comrade"}, prefixEach(scriptNpmKeys, "comrade-")...)
	assert.ElementsMatchf(t, expectedPkgDirNames, knownPkgDirNames,
		"scripts/build-npm-packages.sh's KNOWN_PKG_DIR_NAMES must be exactly {cli-comrade} plus "+
			"comrade-<npm_os>-<npm_cpu> for every PLATFORMS[] entry.\nexpected: %v\nKNOWN_PKG_DIR_NAMES: %v",
		expectedPkgDirNames, knownPkgDirNames)
}

func prefixEach(items []string, prefix string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = prefix + item
	}
	return out
}

// extractYAMLScalarList extracts a simple YAML block-list's scalar values,
// e.g. for `key`:
//
//	key:
//	  - a
//	  - b
//
// returns ["a", "b"]. Anchored on the key being followed immediately by a
// newline (no inline value), so it does not also match a `key: value`
// occurrence elsewhere in the file (e.g. inside an `ignore:` entry).
func extractYAMLScalarList(t *testing.T, yamlText, key string) []string {
	t.Helper()
	blockRe := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `:\s*\n((?:\s*-\s*\S+\s*\n)+)`)
	m := blockRe.FindStringSubmatch(yamlText)
	require.NotNilf(t, m, ".goreleaser.yaml: expected a block-list %q:\\n  - ...", key)

	itemRe := regexp.MustCompile(`(?m)^\s*-\s*(\S+)\s*$`)
	items := itemRe.FindAllStringSubmatch(m[1], -1)
	require.NotEmptyf(t, items, ".goreleaser.yaml: %q block matched but contained no list items", key)

	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item[1]
	}
	return out
}

// extractGoreleaserIgnoredPairs extracts .goreleaser.yaml's builds[].ignore
// entries (each a `- goos: X` / `goarch: Y` pair) as a set of "X/Y" strings.
func extractGoreleaserIgnoredPairs(t *testing.T, yamlText string) map[string]bool {
	t.Helper()
	blockRe := regexp.MustCompile(`(?ms)^\s*ignore:\s*\n((?:\s*-?\s*(?:goos|goarch):\s*\S+\s*\n?)+)`)
	block := blockRe.FindStringSubmatch(yamlText)
	require.NotNil(t, block, ".goreleaser.yaml: expected a builds[].ignore: block")

	pairRe := regexp.MustCompile(`(?m)-\s*goos:\s*(\S+)\s*\n\s*goarch:\s*(\S+)\s*$`)
	pairs := pairRe.FindAllStringSubmatch(block[1], -1)
	require.NotEmptyf(t, pairs, ".goreleaser.yaml: ignore: block matched but no goos/goarch pair inside it")

	out := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		out[p[1]+"/"+p[2]] = true
	}
	return out
}

// extractBashQuotedArray extracts the double-quoted string literals inside
// a bash array assignment of the form:
//
//	NAME=(
//	  "a"
//	  "b"
//	)
func extractBashQuotedArray(t *testing.T, shText, varName string) []string {
	t.Helper()
	blockRe := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(varName) + `=\(\n(.*?)\n\)`)
	m := blockRe.FindStringSubmatch(shText)
	require.NotNilf(t, m, "expected a bash array %s=(\\n  \"...\"\\n)", varName)

	itemRe := regexp.MustCompile(`"([^"]+)"`)
	items := itemRe.FindAllStringSubmatch(m[1], -1)
	require.NotEmptyf(t, items, "%s=(...) matched but contained no quoted entries", varName)

	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item[1]
	}
	return out
}

// extractBashWordArray extracts the bare, space-separated words inside a
// single-line bash array assignment: NAME=(a b c).
func extractBashWordArray(t *testing.T, shText, varName string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(varName) + `=\(([^)]+)\)\s*$`)
	m := re.FindStringSubmatch(shText)
	require.NotNilf(t, m, "expected a single-line bash array %s=(...)", varName)
	return strings.Fields(m[1])
}

// extractPlatformMapKeys extracts the "os-cpu" keys of platform-map.js's
// PLATFORM_PACKAGES object literal, e.g. 'linux-x64': Object.freeze({ pkg: ... }).
func extractPlatformMapKeys(t *testing.T, jsText string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*'([a-z0-9]+-[a-z0-9]+)':\s*Object\.freeze\(\{\s*pkg:`)
	matches := re.FindAllStringSubmatch(jsText, -1)
	require.NotEmpty(t, matches, "platform-map.js: found no PLATFORM_PACKAGES entries matching the expected shape")

	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m[1]
	}
	return out
}
