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
is a wholly separate GitHub Actions workflow (`.github/workflows/
snap.yml`) that no-ops when its secret is absent, so it can never even
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

## Summary table

| Channel | Status | Secret name | What it needs first | Lead time once secret is set |
|---|---|---|---|---|
| Homebrew | ✅ shipped (since v0.1.2) | `HOMEBREW_TAP_TOKEN` | Nothing (tap repo already exists) | Instant, next tag |
| Scoop | ✅ shipped (since v0.1.3) | `SCOOP_BUCKET_TOKEN` | Nothing (bucket repo already exists) | Instant, next tag |
| winget | ⏳ pending | `WINGET_TOKEN` | Fork `microsoft/winget-pkgs` to your account | Hours–days (MS moderator merges the auto-opened PR; commit now passes the CLA gate, see above) |
| Snap | ⏳ pending | `SNAPCRAFT_STORE_CREDENTIALS` | `snapcraft register cli-comrade` + a passed classic-confinement `store-requests` forum review | Multi-week (Canonical manual review), then instant per-release after |
| Cosign signing | ✅ shipped (v0.3.0) | `COSIGN_PRIVATE_KEY` + `COSIGN_PASSWORD` | Key pair generated, public half committed at `internal/update/cosign.pub` (already done) | Instant, next tag — but **fails the whole release** if the secrets are missing, unlike the four channels above |

The four package-manager channels above are **not** required for
`firatkutay/cli-comrade`'s next tagged release to succeed — each
degrades to "skip this channel" when its secret is absent, verified by
running `goreleaser check` and `goreleaser release --snapshot --clean
--skip=publish` with none of the four secrets set (see the
release-engineering handoff notes for that run's output). Cosign
signing is the one exception to that pattern — see "Supply-chain
signing (cosign)" above.
