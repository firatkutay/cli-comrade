package update

import (
	"fmt"
	"strconv"
	"strings"
)

// IsDevBuild reports whether version is the placeholder cmd/comrade/main.go
// bakes in for local, non-release builds (its "var version = \"dev\""
// default) — the case `comrade upgrade` must always refuse, since a dev
// build has no comparable release tag at all.
func IsDevBuild(version string) bool {
	return version == "" || version == "dev"
}

// IsNewer reports whether latest names a strictly greater release than
// current. Both are compared as dotted, "v"-prefix-optional numeric
// version strings (e.g. "v0.2.0", "0.2.0"): each is stripped of a
// leading "v", any "-"-delimited pre-release/build suffix (e.g. a
// goreleaser snapshot's "-SNAPSHOT-<hash>") is discarded for ordering
// purposes, then compared component-by-component as integers, with a
// missing trailing component treated as 0 (so "1.2" == "1.2.0").
//
// This is deliberately a minimal, hand-rolled comparison — not a full
// SemVer 2.0 precedence implementation (it does not order pre-release
// identifiers against each other, e.g. "1.0.0-alpha" vs "1.0.0-beta") —
// sufficient for cli-comrade's own goreleaser-produced "vX.Y.Z" release
// tags, and avoids pulling in a new dependency (golang.org/x/mod/semver)
// for a comparison this narrow.
func IsNewer(current, latest string) (bool, error) {
	curParts, err := parseVersionCore(current)
	if err != nil {
		return false, fmt.Errorf("update: parse current version %q: %w", current, err)
	}
	latestParts, err := parseVersionCore(latest)
	if err != nil {
		return false, fmt.Errorf("update: parse latest version %q: %w", latest, err)
	}

	for i := 0; i < len(curParts) || i < len(latestParts); i++ {
		var c, l int
		if i < len(curParts) {
			c = curParts[i]
		}
		if i < len(latestParts) {
			l = latestParts[i]
		}
		if l != c {
			return l > c, nil
		}
	}
	return false, nil
}

// parseVersionCore strips a leading "v" and any "-"-delimited suffix from
// version, then splits the remaining dotted numeric core into ints.
func parseVersionCore(version string) ([]int, error) {
	core := StripVersionPrefix(version)
	if idx := strings.IndexByte(core, '-'); idx >= 0 {
		core = core[:idx]
	}
	if core == "" {
		return nil, fmt.Errorf("empty version core")
	}

	fields := strings.Split(core, ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("non-numeric version component %q: %w", f, err)
		}
		parts = append(parts, n)
	}
	return parts, nil
}
