// Package update implements cli-comrade's FAZ 10 "comrade upgrade" and
// passive version-notification features: resolving the latest published
// GitHub release, comparing it against the binary's own build-time
// version, downloading and checksum-verifying the matching platform
// archive, extracting the binary from it, and atomically replacing the
// currently running executable (including the Windows
// can't-overwrite-a-running-exe rename dance).
//
// Every network-facing piece is an injectable interface/struct field
// (ReleaseFetcher, AssetDownloader), never a bare call to net/http from
// inside internal/cli — exactly like internal/llm's connectors take an
// injectable *http.Client and internal/context.RunCommand is an
// injectable CommandRunner. Tests exercise the whole flow against fake
// implementations and never touch the real network or GitHub API.
//
// This package is a leaf: it imports only the standard library, so it
// can be depended on by internal/cli without any import-cycle risk —
// the same shape as internal/i18n and internal/redact.
package update
