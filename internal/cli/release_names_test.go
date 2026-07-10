package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/update"
)

// TestReleaseArchiveNamingIsConsistentAcrossGoreleaserInstallScriptsAndUpdatePackage
// is docs/history/UYGULAMA_PLANI.md FAZ 10's Derive-or-Guard requirement: the release
// archive naming scheme lives, unavoidably, as FOUR independent copies —
// .goreleaser.yaml's archives[].name_template (what actually gets built
// and uploaded), scripts/install.sh's and scripts/install.ps1's own
// project-name-prefix + os/arch/extension-suffix pieces (what the install
// scripts use to pick out and parse the right line of the downloaded
// checksums.txt — the scripts themselves DERIVE the archive filename from
// that line rather than hardcoding it, but still hand-maintain those
// selection pieces), and internal/update's
// ArchiveName/BinaryName/ChecksumsFileName (what `comrade upgrade`
// downloads). Each copy is individually valid — every one of the four
// already has its own passing tests/syntax checks — so no single-file
// check catches one of them drifting from the other three. This test
// renders/greps all four and cross-checks them bidirectionally: it fails
// on ANY of the four changing alone, not just on the install scripts
// falling behind goreleaser.
func TestReleaseArchiveNamingIsConsistentAcrossGoreleaserInstallScriptsAndUpdatePackage(t *testing.T) {
	root := repoRoot(t)

	goreleaserYAML := readRepoFile(t, root, ".goreleaser.yaml")
	installSh := readRepoFile(t, root, "scripts", "install.sh")
	installPs1 := readRepoFile(t, root, "scripts", "install.ps1")

	projectName := extractOne(t, goreleaserYAML, `(?m)^project_name:\s*(\S+)\s*$`, ".goreleaser.yaml project_name")
	archiveTemplateRaw := extractOne(t, goreleaserYAML, `(?m)^\s*name_template:\s*>-\s*\n\s*(.+?)\s*$`, ".goreleaser.yaml archives[].name_template")
	checksumsName := extractOne(t, goreleaserYAML, `(?s)checksum:\s*\n\s*name_template:\s*"([^"]+)"`, ".goreleaser.yaml checksum.name_template")
	// Exactly 4 leading spaces distinguishes the archives-level default
	// "formats:" line from the more deeply nested (8 spaces)
	// format_overrides one matched separately below — both are
	// "formats: [...]" lines, so indentation is what disambiguates them
	// in this fixed, known YAML shape.
	defaultFormat := extractOne(t, goreleaserYAML, `(?m)^ {4}formats:\s*\[(\w[\w.]*)\]\s*$`, ".goreleaser.yaml archives[].formats (default)")
	windowsFormat := extractOne(t, goreleaserYAML, `(?s)format_overrides:\s*\n\s*-\s*goos:\s*windows\s*\n\s*formats:\s*\[(\w[\w.]*)\]`, ".goreleaser.yaml archives[].format_overrides (windows)")
	buildBinary := extractOne(t, goreleaserYAML, `(?m)^\s*binary:\s*(\S+)\s*$`, ".goreleaser.yaml builds[].binary")

	require.Equal(t, "comrade", projectName, "sanity: project_name must still be comrade")
	require.Equal(t, "comrade", buildBinary, "sanity: builds[].binary must still be comrade")
	require.Equal(t, "tar.gz", defaultFormat, "sanity: default archive format must still be tar.gz")
	require.Equal(t, "zip", windowsFormat, "sanity: windows archive format override must still be zip")
	require.Equal(t, update.ChecksumsFileName, checksumsName,
		"internal/update.ChecksumsFileName must match .goreleaser.yaml's checksum.name_template")

	tmpl, err := template.New("archive_name").Parse(archiveTemplateRaw)
	require.NoError(t, err, "goreleaser's archives[].name_template must be a valid Go template")

	renderArchiveBaseName := func(t *testing.T, goos, goarch string) string {
		t.Helper()
		var buf bytes.Buffer
		data := struct{ ProjectName, Version, Os, Arch string }{
			ProjectName: projectName, Version: "9.9.9", Os: goos, Arch: goarch,
		}
		require.NoError(t, tmpl.Execute(&buf, data))
		return buf.String()
	}

	// scripts/install.sh and scripts/install.ps1 (FAZ-11 rewrite) no
	// longer CONSTRUCT the archive filename by string interpolation —
	// they DERIVE it from the matching line of the downloaded
	// checksums.txt instead (avoids ever hand-duplicating goreleaser's
	// full name_template). That's a real, deliberate improvement: it
	// can't drift on the archive name as a whole. But both scripts still
	// hand-maintain two drift-sensitive pieces used to SELECT the right
	// checksums.txt line and to split the version back out of it: a
	// project-name prefix and an os/arch/extension suffix. If
	// goreleaser's name_template field order, os/arch values, or archive
	// extension ever changed, these suffixes would silently stop
	// matching any checksums.txt line — this guard reconstructs each
	// script's expected archive name from its own prefix+suffix pieces
	// and cross-checks it against goreleaser's own rendered template
	// below (per-case, in the cases loop), so it still fails on that
	// drift even though the scripts no longer hardcode the full name.

	// scripts/install.sh: BIN_NAME (project prefix) + the
	// archive_suffix="_${os}_${arch}.tar.gz" grep suffix used to pick
	// the checksums.txt line.
	shBinName := extractOne(t, installSh, `(?m)^BIN_NAME="([^"]+)"$`, "scripts/install.sh BIN_NAME")
	assert.Equal(t, projectName, shBinName, "install.sh's BIN_NAME must match goreleaser's project_name")

	shSuffixExpr := extractOne(t, installSh, `(?m)^\s*archive_suffix="(.+)"$`, "scripts/install.sh archive_suffix= line")
	shSuffixFields := parseShellSuffixFields(t, shSuffixExpr)
	assert.Equal(t, []string{"os", "arch"}, shSuffixFields.vars,
		"install.sh's archive_suffix var order must match goreleaser's trailing Os_Arch template fields")
	assert.Equal(t, defaultFormat, shSuffixFields.ext, "install.sh must grep for the same default extension goreleaser produces")
	assert.Contains(t, installSh, checksumsName, "install.sh must download the same checksums file name goreleaser produces")

	// scripts/install.ps1: the $archiveSuffix = "_windows_${arch}.zip"
	// grep suffix, and the literal "comrade_" project-name prefix baked
	// into the -replace pattern that strips the version number back out
	// of the matched filename.
	ps1SuffixExpr := extractOne(t, installPs1, `(?m)^\$archiveSuffix\s*=\s*"(.+)"$`, "scripts/install.ps1 $archiveSuffix= line")
	ps1SuffixFields := parsePowerShellSuffixFields(t, ps1SuffixExpr)
	assert.Equal(t, "windows", ps1SuffixFields.goos, "install.ps1 only ever builds the windows archive name")
	assert.Equal(t, windowsFormat, ps1SuffixFields.ext, "install.ps1 must grep for the same windows-override extension goreleaser uses")

	ps1ProjectPrefix := extractOne(t, installPs1, `\$archive -replace '\^(\w+)_'`, "scripts/install.ps1 version-strip project-name prefix")
	assert.Equal(t, projectName, ps1ProjectPrefix, "install.ps1's version-strip prefix must match goreleaser's project_name")
	assert.Contains(t, installPs1, checksumsName, "install.ps1 must download the same checksums file name goreleaser produces")

	cases := []struct {
		goos, goarch, format string
	}{
		{"linux", "amd64", defaultFormat},
		{"linux", "arm64", defaultFormat},
		{"darwin", "amd64", defaultFormat},
		{"darwin", "arm64", defaultFormat},
		{"windows", "amd64", windowsFormat},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			wantFromGoreleaser := renderArchiveBaseName(t, tc.goos, tc.goarch) + "." + tc.format
			gotFromUpdatePkg := update.ArchiveName(projectName, "9.9.9", tc.goos, tc.goarch)
			assert.Equal(t, wantFromGoreleaser, gotFromUpdatePkg,
				"internal/update.ArchiveName must render the exact same name as .goreleaser.yaml's own template")

			switch tc.goos {
			case "windows":
				// install.ps1 never constructs this string itself (it reads
				// the real filename out of checksums.txt) — this
				// reconstructs what it EXPECTS that filename to look like
				// from its own project-prefix + archiveSuffix pieces, and
				// checks that still matches goreleaser's real output.
				gotFromInstallPs1 := ps1ProjectPrefix + "_9.9.9_windows_" + tc.goarch + "." + ps1SuffixFields.ext
				assert.Equal(t, wantFromGoreleaser, gotFromInstallPs1,
					"install.ps1's project-prefix + archiveSuffix must reconstruct the exact same archive name goreleaser produces")
			default:
				// install.sh likewise never constructs the archive name
				// itself anymore — reconstruct what it EXPECTS via
				// BIN_NAME + archive_suffix and check it still matches.
				gotFromInstallSh := shBinName + "_9.9.9_" + tc.goos + "_" + tc.goarch + "." + shSuffixFields.ext
				assert.Equal(t, wantFromGoreleaser, gotFromInstallSh,
					"install.sh's BIN_NAME + archive_suffix must reconstruct the exact same archive name goreleaser produces")
			}
		})
	}

	// internal/update.BinaryName must agree with goreleaser's builds[].binary
	// (plus the universal ".exe" Windows convention, which goreleaser applies
	// implicitly and is not itself a config value to cross-check).
	assert.Equal(t, buildBinary, update.BinaryName("linux"))
	assert.Equal(t, buildBinary, update.BinaryName("darwin"))
	assert.Equal(t, buildBinary+".exe", update.BinaryName("windows"))
}

// readRepoFile reads root/parts... as a string, failing the test if it's
// missing. CRLF line endings are normalized to LF: .gitattributes forces
// `*.ps1 text eol=crlf`, so a fresh checkout materializes install.ps1
// with \r\n while a working tree that predates that attribute (or was
// never renormalized) still has \n — this guard's regexes anchor on `$`
// (end of line) and must match either way, not just whichever the
// checkout happens to have.
func readRepoFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// extractOne runs pattern (which must have exactly one capture group)
// against text and returns that group's value, failing the test with a
// descriptive message (naming what/where) if it doesn't match exactly
// once — this drift guard must fail loudly, not silently pass with an
// empty string, if any of the four files' shape ever changes underneath
// the regex.
func extractOne(t *testing.T, text, pattern, what string) string {
	t.Helper()
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(text, -1)
	require.Lenf(t, matches, 1, "%s: expected exactly one match for %s, found %d — has the file's shape changed?", what, pattern, len(matches))
	require.Len(t, matches[0], 2, "%s: pattern must have exactly one capture group", what)
	return matches[0][1]
}

// shellArchiveFields is parseShellSuffixFields's result: the ordered
// list of shell variable names interpolated into the archive-selection
// suffix, and the trailing literal file extension.
type shellArchiveFields struct {
	vars []string
	ext  string
}

// parseShellSuffixFields parses install.sh's archive_suffix expression
// (the checksums.txt line-selection suffix, of the form `_${A}_${B}.ext`)
// into its ordered variable names and trailing literal extension.
func parseShellSuffixFields(t *testing.T, expr string) shellArchiveFields {
	t.Helper()
	re := regexp.MustCompile(`^_\$\{(\w+)\}_\$\{(\w+)\}\.(.+)$`)
	m := re.FindStringSubmatch(expr)
	require.NotNil(t, m, "install.sh archive_suffix expression %q does not match the expected _${A}_${B}.ext shape", expr)
	return shellArchiveFields{vars: m[1:3], ext: m[3]}
}

// powerShellSuffixFields is parsePowerShellSuffixFields's result:
// install.ps1's literal goos segment plus its interpolated arch variable
// and trailing extension, from its own checksums.txt line-selection
// suffix.
type powerShellSuffixFields struct {
	goos string
	arch string
	ext  string
}

// parsePowerShellSuffixFields parses install.ps1's $archiveSuffix
// expression of the form `_windows_${arch}.zip` into its literal goos
// segment, interpolated arch variable name, and trailing extension.
func parsePowerShellSuffixFields(t *testing.T, expr string) powerShellSuffixFields {
	t.Helper()
	re := regexp.MustCompile(`^_(\w+)_\$\{(\w+)\}\.(.+)$`)
	m := re.FindStringSubmatch(expr)
	require.NotNil(t, m, "install.ps1 archiveSuffix expression %q does not match the expected _literal_${A}.ext shape", expr)
	return powerShellSuffixFields{goos: m[1], arch: m[2], ext: m[3]}
}
