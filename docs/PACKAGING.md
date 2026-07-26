# PACKAGING.md — package-manager channel activation (maintainer-facing)

This is the checklist for turning on each package-manager channel wired
into `.goreleaser.yaml` / `.github/workflows/`. It is **not** user-facing
install instructions — those live in `docs/INSTALL.md` / `README.md`
(owned separately; this file is only for the person operating the
release pipeline).

## How the "never break a release" gate works

Every channel below is wired so that **a missing credential degrades
that one channel to "skip, log why, keep going" — it never fails the
whole `goreleaser release` run.** homebrew_casks/scoops/winget each carry
their own `skip_upload: "{{ not (isEnvSet \"...\") }}"`; the Snap channel
is a wholly separate GitHub Actions workflow (`.github/workflows/snap.yml`) that no-ops when its secret is absent, so it can never even
touch the main release job. See the comments in `.goreleaser.yaml` and
`.github/workflows/release.yml` for the exact mechanism if you're
debugging why a channel didn't publish — it is almost always "the
secret named below isn't set on the repo yet," which is the expected,
safe default state.

Each secret goes in **Settings → Secrets and variables → Actions →
New repository secret** on `firatkutay/cli-comrade` (the secret is read
by `.github/workflows/release.yml` / `snap.yml` running in that repo —
it does not need to exist anywhere else).

---

## Supply-chain signing (cosign) — ✅ SHIPPED (v0.3.0)

Every release's `checksums.txt` (which transitively covers every other
release artifact, since it lists their digests) is signed with
**cosign**, key-based and fully offline (`--tlog-upload=false` — no
Rekor transparency-log entry, no network dependency at sign time). The
real public key is already committed at `internal/update/cosign.pub`,
so `comrade upgrade` verifies every downloaded release's signature
before trusting its checksum or replacing the running binary — see
[docs/UPDATE_SIGNING.md](UPDATE_SIGNING.md) for the verification flow,
and `.goreleaser.yaml`'s `signs:` block for the exact `cosign sign-blob`
invocation.

This channel is **not** gated by the same `skip_upload`/missing-secret
pattern as Homebrew/Scoop/winget below: per
[docs/UPDATE_SIGNING.md](UPDATE_SIGNING.md), once the public key is
embedded, the release workflow's cosign step has **no graceful skip** —
a release cut without `COSIGN_PRIVATE_KEY`/`COSIGN_PASSWORD` set as
repository secrets will **fail** the whole release, by design (an
unsigned release with an embedded verification key would silently
downgrade every future `comrade upgrade` to a hard failure instead).
Rotating the key means generating a new pair, updating both secrets,
and re-committing `internal/update/cosign.pub`.

---

## 1. Homebrew (`brew install comrade`) — ✅ SHIPPED (live since v0.1.2)

**Target repo:** `firatkutay/homebrew-tap`. Holds a live, auto-updated
`Casks/comrade.rb`, committed directly by `homebrew_casks` on every
tagged release.

1. Create a fine-grained-scope PAT: GitHub → Settings → Developer
   settings → **Fine-grained tokens** → New token.
   - Repository access: **Only** `firatkutay/homebrew-tap`.
   - Permissions: **Contents: Read and write** (this is all
     `homebrew_casks` needs — it commits `Casks/comrade.rb` directly to
     the tap's default branch, no PR).
2. Set it as the `HOMEBREW_TAP_TOKEN` secret on `firatkutay/cli-comrade`.
3. **Lead time:** instant on the next tagged release once the secret
   exists — no external review, it's your own repo.

**End-user install command:**
```sh
brew tap firatkutay/tap
brew install comrade
```

---

## 2. Scoop (`scoop install comrade`) — ✅ SHIPPED (live since v0.1.3)

**Target repo:** `firatkutay/scoop-bucket`. Holds a live, auto-updated
bucket manifest, committed directly by `scoops` on every tagged
release.

1. Same PAT shape as Homebrew: fine-grained token scoped to
   **only** `firatkutay/scoop-bucket`, **Contents: Read and write**.
2. Set it as the `SCOOP_BUCKET_TOKEN` secret on `firatkutay/cli-comrade`.
3. **Lead time:** instant on the next tagged release once the secret
   exists.

**End-user install command:**
```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

---

## 3. winget (`winget install cli.comrade`) — ⏳ PENDING (PR open, moderator review)

**Target repo:** the real `microsoft/winget-pkgs` community repo, via a
PR opened from a fork.

1. **Fork `microsoft/winget-pkgs` into your own account** (this is a
   manual, one-time GitHub UI/CLI action on your own account — the
   pipeline cannot do this for you):
   ```sh
   gh repo fork microsoft/winget-pkgs --clone=false
   ```
   This creates `firatkutay/winget-pkgs`, matching what
   `.goreleaser.yaml`'s `winget.repository` already points at.
2. Create a classic PAT (fine-grained tokens do not yet reliably cover
   cross-repo PR creation against a fork's upstream — verify current
   support before switching) with the `public_repo` scope (or `repo` if
   you keep the fork private, though winget-pkgs itself is public).
3. Set it as the `WINGET_TOKEN` secret on `firatkutay/cli-comrade`.
4. **Lead time:** goreleaser pushes a `comrade-{{ .Version }}` branch to
   your fork and auto-opens a PR against `microsoft/winget-pkgs:master`
   on every tagged release once the secret exists. A Microsoft moderator
   / automated validation pipeline reviews and merges — typically hours
   to a few days, entirely outside this repo's control.

**CLA fix (v0.3.0):** `.goreleaser.yaml`'s `winget.commit_author` now
authors the winget-pkgs commit as the maintainer's own CLA-signed
identity (name + noreply email) instead of goreleaser's default
`goreleaserbot` author. winget-pkgs' automated CLA check is keyed on the
commit author, so a bot-authored commit previously tripped its
`Needs-CLA` gate on every release PR before a human reviewer ever saw
it — this fix is what lets the auto-opened PR reach actual moderator
review instead of being auto-rejected at the CLA gate.

**End-user install command (once live):**
```powershell
winget install cli.comrade
```

---

## 4. Snap (`snap install cli-comrade --classic`) — ⏳ PENDING (awaiting Canonical review)

**This channel needs the most lead time — start it early.** Snap is
wired as its own workflow, `.github/workflows/snap.yml`, driven by
`snap/snapcraft.yaml` — it is intentionally decoupled from
`release.yml` because a snap cannot be built inside the container/job
that runs the rest of the release (see the comments in both files).

1. Register the snap name (one-time, requires a Snap Store / Ubuntu SSO
   account):
   ```sh
   sudo snap install snapcraft --classic
   snapcraft login
   snapcraft register cli-comrade
   ```
2. `comrade` needs **classic confinement** (it runs arbitrary
   user-approved shell commands and reads/writes outside its own sandbox
   — strict confinement would defeat the tool's purpose). Classic
   confinement is not self-service: file a request in the Snap Store
   forum's `store-requests` category —
   <https://forum.snapcraft.io/c/store-requests/16> — following the
   template there (name, why classic is required, a link to this repo).
   **This is a manual human review by Canonical and commonly takes
   multiple weeks.** Do not expect this to be fast; start it as soon as
   the name is registered, independent of when you set up the other
   three channels.
3. Once you're ready to let CI publish, get the upload credentials and
   set them as a single secret:
   ```sh
   snapcraft export-login --snaps=cli-comrade \
     --acls package_access,package_push,package_update,package_release \
     exported.txt
   ```
   Set the **contents of `exported.txt`** as the `SNAPCRAFT_STORE_CREDENTIALS`
   secret on `firatkutay/cli-comrade`, then delete the local file — it's
   a bearer credential.
4. Until both (a) the name is registered and (b) the classic-confinement
   review has passed, leave `SNAPCRAFT_STORE_CREDENTIALS` unset —
   `snap.yml` will keep running green and doing nothing (see its final
   step's `::notice::`). Setting the secret before the review passes is
   harmless but pointless: uploads to a channel that isn't approved for
   classic confinement will be rejected by the Store, not by this repo's
   CI.
5. `snap.yml` publishes to the `edge` channel only (see the workflow's
   comment). Promote `edge` → `candidate`/`stable` by hand once you're
   confident in a given revision:
   ```sh
   snapcraft release cli-comrade <revision> stable
   ```

**Lead time:** multi-week (Canonical's manual classic-confinement
review is the bottleneck, not anything in this repo).

**End-user install command (once live):**
```sh
snap install cli-comrade --classic
```

---

## 5. npm (`npm install -g cli-comrade`) — ✅ SHIPPED (automated via Trusted Publishing, OIDC)

**Published to the real npm registry** — `npm view cli-comrade dist-tags`
resolves to a real, installable version. This is an **alternative**
channel for developers who already have Node.js; it is not the primary
install path (see `README.md`/`docs/INSTALL.md`) — the target audience
for this tool generally doesn't have Node installed, and npm installs
can't self-update via `comrade upgrade` (see below).

What exists: `npm/` (the main dispatcher package template + the shared
platform-package template) and `scripts/build-npm-packages.sh`, which
assembles the 6 publishable package directories (1 main + 5 platform:
linux-x64/arm64, darwin-x64/arm64, win32-x64 -- mirrors
`.goreleaser.yaml`'s build matrix exactly, guarded bidirectionally by
`internal/cli/npm_platform_matrix_test.go`) from goreleaser's `dist/`
output.

**Package names:** main `cli-comrade`; platform packages
`@firatkutay/comrade-linux-x64`, `-linux-arm64`, `-darwin-x64`,
`-darwin-arm64`, `-win32-x64`, scoped under the `@firatkutay` npm org/user.

**History:** the first published version (v0.4.4) went out as a
**one-time manual bootstrap publish** (`npm publish` run by hand against
`scripts/build-npm-packages.sh`'s output), before this automation existed.
`release.yml` now wires automated publishing into the same job as
goreleaser, so every release **from here on** publishes to npm without
a manual step -- the paragraphs below describe that wiring, which is
what makes the bootstrap a one-off rather than the standing process.

**Wired into `release.yml`** as the final two steps of the `goreleaser`
job, run in the SAME job (not a downstream job), after goreleaser has
created the GitHub Release and its SBOM has been attached -- so the npm
packages are assembled from the exact `dist/` this job's own goreleaser
step just built and cosign-signed, never a separately rebuilt copy, and
an npm publish failure can never leave a half-finished GitHub Release
behind (goreleaser's release and its SBOM are already complete,
irreversible, in-repo steps by the time npm -- whose published versions
are themselves immutable -- is even attempted).

**Auth mechanism: npm Trusted Publishing (OIDC) -- no token, ever.**
No `NPM_TOKEN`, no secret of any kind for this channel. The workflow's
`permissions:` block grants `id-token: write`; npm CLI >=11.5.1 detects
it is running in GitHub Actions with that permission and exchanges the
job's short-lived OIDC token for a one-off, per-publish npm registry
credential (docs.npmjs.com/trusted-publishers/, verified 2026-07-26).
Each of the 6 packages must have a **Trusted Publisher** configured on
npmjs.com first (one-time, per package, done by hand below) -- npm
refuses the publish with a 404 if that configuration is missing or
doesn't match this repo + this exact workflow filename.

**A real `NODE_AUTH_TOKEN` gotcha (hit on the v0.4.6 release, run
30210611779):** `actions/setup-node`'s `registry-url` input -- needed so
`.npmrc` points at the real npm registry -- makes the action write a
*placeholder* `NODE_AUTH_TOKEN` (`XXXXX-XXXXX-XXXXX-XXXXX`) for every
later step whenever no real token is supplied, on every setup-node
release through v6.5.0 (fixed only in v7.0.0, too new for this repo's
>=15-day version-selection floor as of this writing). `release.yml`'s
"Publish npm packages" step overrides that placeholder to an explicit
empty string for itself; see that step's inline comment for the full
citation trail. This is why you may see `NODE_AUTH_TOKEN: ""` in that
step even though this channel is tokenless -- it is not a credential,
it is a guard against the third-party action's own fallback.

**Owner click-path -- do this once, for EACH of the 6 packages
(organization/user `firatkutay`, repository `cli-comrade`, workflow
filename `release.yml`, environment left blank, allowed action scoped
to `npm publish` only):**

1. Sign in to <https://www.npmjs.com/> as an owner/maintainer of the
   package (the `firatkutay` user for `cli-comrade`; the `firatkutay`
   org for the 5 scoped `@firatkutay/comrade-*` packages).
2. Go to `https://www.npmjs.com/package/<name>/access` -- e.g.
   `https://www.npmjs.com/package/cli-comrade/access` and
   `https://www.npmjs.com/package/@firatkutay/comrade-linux-x64/access`
   (repeat for `comrade-linux-arm64`, `comrade-darwin-x64`,
   `comrade-darwin-arm64`, `comrade-win32-x64`).
3. Find the **Trusted Publisher** section and click **Add trusted publisher**
   (wording may read "GitHub Actions").
4. Fill in exactly:
   - **Organization or user:** `firatkutay`
   - **Repository:** `cli-comrade`
   - **Workflow filename:** `release.yml` (filename only, no path, no
     leading `.github/workflows/` -- this MUST byte-match the file in
     this repo)
   - **Environment name:** leave blank (this workflow does not use a
     GitHub Environment)
   - **Allowed actions / events:** select `npm publish` (not `npm stage publish`)
5. Save. npm does **not** validate this configuration when you save it
   -- a typo'd repo or workflow filename only surfaces as a failed
   publish on the next tagged release. Double-check all 6 before
   trusting it.
6. Repeat for all 6 packages. There is no bulk/org-wide toggle; each
   package's Trusted Publisher is configured independently.

Once all 6 are configured, every future `git push --tags` (or the
existing `task`/`make`-driven tagging flow) publishes to npm
automatically as part of the same release run that already handles
Homebrew/Scoop/winget/Snap/cosign -- no further manual step.

**`comrade upgrade` under an npm-managed install:** already handled
(v0.4.3, GitHub issue #37/PR #37) -- `comrade upgrade` refuses under a
Node-package-manager-managed install and points at that package
manager's own update command instead of attempting to replace a binary
`npm`/`pnpm`/`yarn`/`bun` itself owns the lifecycle of. See
`internal/cli`'s upgrade-guard tests and CHANGELOG's `[0.4.3]` entry --
nothing left to decide here.

**Idempotent on re-run:** the publish step checks `npm view <name>@<version>`
before every one of the 6 publishes and skips (logging a notice) any
package/version already on the registry, rather than failing the whole
job -- a re-run of an already-released tag (e.g. a transient failure in
an earlier step) does not need the tag moved or the version bumped to
retry cleanly. A genuine publish failure (auth, network, registry error)
still fails the job.

**Reproducibility note:** the v0.4.4 bootstrap publish's binaries were
built by hand, not by this repo's own CI runner. They are still
verifiable against the signed release because release builds now use
`-trimpath` (see "Supply-chain signing (cosign)" above and
`docs/SECURITY.md`'s "Reproducible release binaries" section) --
without that flag, a manually built binary published to an immutable
npm version would have been permanently unverifiable. Every release
from here on is built and published by this same CI job, so this note
is history, not a standing caveat.

**End-user install command:**
```sh
npm install -g cli-comrade
```

---

## Summary table

| Channel | Status | Secret name | What it needs first | Lead time once secret is set |
|---|---|---|---|---|
| Homebrew | ✅ shipped (since v0.1.2) | `HOMEBREW_TAP_TOKEN` | Nothing (tap repo already exists) | Instant, next tag |
| Scoop | ✅ shipped (since v0.1.3) | `SCOOP_BUCKET_TOKEN` | Nothing (bucket repo already exists) | Instant, next tag |
| winget | ⏳ pending | `WINGET_TOKEN` | Fork `microsoft/winget-pkgs` to your account | Hours–days (MS moderator merges the auto-opened PR; commit now passes the CLA gate, see above) |
| Snap | ⏳ pending | `SNAPCRAFT_STORE_CREDENTIALS` | `snapcraft register cli-comrade` + a passed classic-confinement `store-requests` forum review | Multi-week (Canonical manual review), then instant per-release after |
| npm | ✅ shipped (wired into `release.yml`, automated via Trusted Publishing) | none -- Trusted Publishing (OIDC), no token | A "Trusted Publisher" configured per package on npmjs.com (see "5. npm" above for the exact click-path) | Instant, next tag, once all 6 packages' Trusted Publisher config is saved |
| Cosign signing | ✅ shipped (v0.3.0) | `COSIGN_PRIVATE_KEY` + `COSIGN_PASSWORD` | Key pair generated, public half committed at `internal/update/cosign.pub` (already done) | Instant, next tag — but **fails the whole release** if the secrets are missing, unlike the four goreleaser-native channels above (Homebrew/Scoop/winget/Snap) |
| Reproducible builds (`-trimpath`) | ✅ shipped | none (build flag, not a secret) | `.goreleaser.yaml`'s `builds:` entry + `Makefile`'s `GOBUILDFLAGS` (already done) | Instant — applies to every build, no per-release action needed |

The four goreleaser-native package-manager channels (Homebrew, Scoop,
winget, Snap) are **not** required for `firatkutay/cli-comrade`'s next
tagged release to succeed — each degrades to "skip this channel" when its
secret is absent, verified by running `goreleaser check` and
`goreleaser release --snapshot --clean --skip=publish` with none of the
four secrets set (see the release-engineering handoff notes for that run's
output). Cosign signing is the one exception to that pattern — see
"Supply-chain signing (cosign)" above. npm is a *third* pattern: it needs
no repo secret at all (Trusted Publishing/OIDC), but it DOES need
per-package configuration done by hand on npmjs.com before it can succeed
-- until that's done for all 6 packages, the npm publish step will fail
loudly (by design; there is no silent-skip for this channel, since it
carries no missing-secret signal to gate on).
