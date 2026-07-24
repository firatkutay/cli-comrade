# Self-Update Signature Verification

`comrade upgrade` downloads a new release and replaces the running binary. To
stop a compromised or spoofed release from installing a malicious binary, each
release's `checksums.txt` is signed with **cosign** (key-based) and verified
against a public key **baked into the binary** before anything is installed —
fully offline, with no network or transparency-log lookup at upgrade time.

## Verification flow (built in)

1. `comrade upgrade` downloads `checksums.txt` **and** `checksums.txt.sig` from the release.
2. It verifies the signature against the embedded `internal/update/cosign.pub`
   (ECDSA P-256 / SHA-256, verified with the Go standard library — no cosign
   binary required on the user's machine).
3. Only if the signature is valid does it verify the archive's SHA-256 against
   `checksums.txt`, then extract and replace the binary.

The signature anchors `checksums.txt` to your key; the checksum anchors the
archive to `checksums.txt`. A release-channel compromise therefore cannot forge
an installable update without your private key.

### Rollout behavior

If no real key is embedded (the shipped placeholder in
`internal/update/cosign.pub`), signature verification is **skipped with a
warning** and only the checksum is checked — so upgrades keep working until the
one-time setup below is done. Once a real key is embedded **and** releases are
signed, verification is enforced: a missing or invalid signature **aborts**
the upgrade.

**Current status: done.** As of v0.3.0, `internal/update/cosign.pub` holds the
project's real ECDSA P-256 public key (not the placeholder), and every release
from v0.3.0 onward is signed in CI — so `comrade upgrade` enforces the
signature unconditionally for those releases. The one-time setup below is kept
for reference and for rotating the key (see "Rotating the key").

## One-time setup (maintainer) — already completed

> **⚠️ Before cutting a release without this configured:** the release
> workflow's cosign step **requires** the `COSIGN_PRIVATE_KEY` and
> `COSIGN_PASSWORD` secrets — unlike the Homebrew/Scoop/winget publish steps,
> signing has **no graceful skip**, so a release cut without these secrets set
> will **fail**. These secrets and the embedded key below are already
> configured for this project; the steps are kept here for the record and for
> setting up a fork or rotating the key.

Activating signing requires generating a key pair and configuring CI secrets.

1. **Generate a cosign key pair** (prompts for a password):
   ```bash
   cosign generate-key-pair
   ```
   This writes `cosign.key` (encrypted private key) and `cosign.pub` (public key).

2. **Add the private key + password as GitHub Actions secrets:**
   ```bash
   gh secret set COSIGN_PRIVATE_KEY < cosign.key
   gh secret set COSIGN_PASSWORD        # paste the password you chose
   ```

3. **Embed the public key:** replace the placeholder `internal/update/cosign.pub`
   with the contents of your `cosign.pub`, and commit it:
   ```bash
   cp cosign.pub internal/update/cosign.pub
   git add internal/update/cosign.pub
   git commit -m "chore: embed cosign release public key"
   ```

4. **Never commit `cosign.key`.** Keep it and the password safe — losing them
   means you must rotate to a new key in a future release.

After the next release (CI signs with the pinned cosign `v2.6.3`),
`checksums.txt.sig` is published and `comrade upgrade` enforces the signature.

## Rotating the key

Generate a new pair, update the two secrets and `internal/update/cosign.pub`,
and ship a release. Clients that upgrade *through* the release carrying the new
embedded public key will then require signatures from the new key.
