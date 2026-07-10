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

## 1. Homebrew (`brew install comrade`)

**Target repo:** `firatkutay/homebrew-tap` (already created, empty except
a README).

1. Create a fine-grained-scope PAT: GitHub → Settings → Developer
   settings → **Fine-grained tokens** → New token.
   - Repository access: **Only** `firatkutay/homebrew-tap`.
   - Permissions: **Contents: Read and write** (this is all
     `homebrew_casks` needs — it commits `Casks/comrade.rb` directly to
     the tap's default branch, no PR).
2. Set it as the `HOMEBREW_TAP_TOKEN` secret on `firatkutay/cli-comrade`.
3. **Lead time:** instant on the next tagged release once the secret
   exists — no external review, it's your own repo.

**End-user install command (once live):**
```sh
brew tap firatkutay/tap
brew install comrade
```

---

## 2. Scoop (`scoop install comrade`)

**Target repo:** `firatkutay/scoop-bucket` (already created, empty except
a README).

1. Same PAT shape as Homebrew: fine-grained token scoped to
   **only** `firatkutay/scoop-bucket`, **Contents: Read and write**.
2. Set it as the `SCOOP_BUCKET_TOKEN` secret on `firatkutay/cli-comrade`.
3. **Lead time:** instant on the next tagged release once the secret
   exists.

**End-user install command (once live):**
```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

---

## 3. winget (`winget install cli.comrade`)

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

**End-user install command (once live):**
```powershell
winget install cli.comrade
```

---

## 4. Snap (`snap install cli-comrade --classic`)

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

| Channel | Secret name | What it needs first | Lead time once secret is set |
|---|---|---|---|
| Homebrew | `HOMEBREW_TAP_TOKEN` | Nothing (tap repo already exists) | Instant, next tag |
| Scoop | `SCOOP_BUCKET_TOKEN` | Nothing (bucket repo already exists) | Instant, next tag |
| winget | `WINGET_TOKEN` | Fork `microsoft/winget-pkgs` to your account | Hours–days (MS moderator merges the auto-opened PR) |
| Snap | `SNAPCRAFT_STORE_CREDENTIALS` | `snapcraft register cli-comrade` + a passed classic-confinement `store-requests` forum review | Multi-week (Canonical manual review), then instant per-release after |

None of these are required for `firatkutay/cli-comrade`'s next tagged
release to succeed — every one degrades to "skip this channel" when its
secret is absent, verified by running `goreleaser check` and `goreleaser
release --snapshot --clean --skip=publish` with none of the four secrets
set (see the release-engineering handoff notes for that run's output).
