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
// is UYGULAMA_PLANI.md FAZ 10's Derive-or-Guard requirement: the release
// archive naming scheme lives, unavoidably, as FOUR independent copies —
// .goreleaser.yaml's archives[].name_template (what actually gets built
// and uploaded), scripts/install.sh's and scripts/install.ps1's own
// archive-name construction (what the install scripts download), and
// internal/update's ArchiveName/BinaryName/ChecksumsFileName (what
// `comrade upgrade` downloads). Each copy is individually valid — every
// one of the four already has its own passing tests/syntax checks — so
// no single-file check catches one of them drifting from the other
// three. This test renders/greps all four and cross-checks them
// bidirectionally: it fails on ANY of the four changing alone, not just
// on the install scripts falling behind goreleaser.
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
		})
	}

	// scripts/install.sh: BIN_NAME + the archive="${BIN_NAME}_${version_number}_${os}_${arch}.tar.gz" line.
	shBinName := extractOne(t, installSh, `(?m)^BIN_NAME="([^"]+)"$`, "scripts/install.sh BIN_NAME")
	assert.Equal(t, projectName, shBinName, "install.sh's BIN_NAME must match goreleaser's project_name")

	shArchiveExpr := extractOne(t, installSh, `(?m)^\s*archive="(.+)"$`, "scripts/install.sh archive= line")
	shFields := parseShellInterpolationFields(t, shArchiveExpr)
	assert.Equal(t, []string{"BIN_NAME", "version_number", "os", "arch"}, shFields.vars,
		"install.sh's archive name field order/count must match goreleaser's ProjectName_Version_Os_Arch template")
	assert.Equal(t, defaultFormat, shFields.ext, "install.sh must build the same default extension goreleaser uses")
	assert.Contains(t, installSh, checksumsName, "install.sh must download the same checksums file name goreleaser produces")

	// scripts/install.ps1: $archive = "comrade_${versionNumber}_windows_${arch}.zip"
	ps1ArchiveExpr := extractOne(t, installPs1, `(?m)^\$archive\s*=\s*"(.+)"$`, "scripts/install.ps1 $archive= line")
	ps1Literal, ps1Fields := parsePowerShellInterpolationFields(t, ps1ArchiveExpr)
	assert.Equal(t, projectName, ps1Literal.project, "install.ps1 must build the archive name from goreleaser's own project_name")
	assert.Equal(t, "windows", ps1Literal.goos, "install.ps1 only ever builds the windows archive name")
	assert.Equal(t, windowsFormat, ps1Fields.ext, "install.ps1 must build the same windows-override extension goreleaser uses")
	assert.Equal(t, []string{"versionNumber", "arch"}, ps1Fields.vars,
		"install.ps1's archive name field order/count (version, arch) must match goreleaser's template")
	assert.Contains(t, installPs1, checksumsName, "install.ps1 must download the same checksums file name goreleaser produces")

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

// shellArchiveFields is parseShellInterpolationFields's result: the
// ordered list of shell variable names interpolated into the archive
// name, and the trailing literal file extension.
type shellArchiveFields struct {
	vars []string
	ext  string
}

// parseShellInterpolationFields parses an sh archive-name expression of
// the form `${A}_${B}_${C}_${D}.ext` into its ordered variable names and
// trailing extension.
func parseShellInterpolationFields(t *testing.T, expr string) shellArchiveFields {
	t.Helper()
	re := regexp.MustCompile(`^\$\{(\w+)\}_\$\{(\w+)\}_\$\{(\w+)\}_\$\{(\w+)\}\.(.+)$`)
	m := re.FindStringSubmatch(expr)
	require.NotNil(t, m, "install.sh archive expression %q does not match the expected ${A}_${B}_${C}_${D}.ext shape", expr)
	return shellArchiveFields{vars: m[1:5], ext: m[5]}
}

// powerShellArchiveLiteral is the literal (non-interpolated) segments of
// install.ps1's archive-name expression.
type powerShellArchiveLiteral struct {
	project string
	goos    string
}

// parsePowerShellInterpolationFields parses a PowerShell double-quoted
// archive-name expression of the form
// `comrade_${versionNumber}_windows_${arch}.zip` into its literal
// project/goos segments and its interpolated variable names + extension.
func parsePowerShellInterpolationFields(t *testing.T, expr string) (powerShellArchiveLiteral, shellArchiveFields) {
	t.Helper()
	re := regexp.MustCompile(`^(\w+)_\$\{(\w+)\}_(\w+)_\$\{(\w+)\}\.(.+)$`)
	m := re.FindStringSubmatch(expr)
	require.NotNil(t, m, "install.ps1 archive expression %q does not match the expected literal_${A}_literal_${B}.ext shape", expr)
	return powerShellArchiveLiteral{project: m[1], goos: m[3]},
		shellArchiveFields{vars: []string{m[2], m[4]}, ext: m[5]}
}
