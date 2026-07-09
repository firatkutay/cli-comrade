# Bilinen Kısıtlar / Known Limitations — v0.1.0-rc1

Bu dosya, `v0.1.0-rc1` sürüm adayının dürüst "bilinen sorunlar" listesidir.
Hiçbir madde gizlenmedi ya da hafifletilmedi.

This file is `v0.1.0-rc1`'s honest known-issues list. Nothing here is
hidden or downplayed.

---

## Türkçe

### Platform çalışma zamanı — bu ortamda doğrulanamayan

- **Windows süreç-ağacı öldürme**: `internal/executor`'ın Windows dalı
  (timeout/Ctrl-C üzerine) tek süreci öldürür; torun süreçler (bir
  komutun başlattığı alt süreçlerin alt süreçleri) hayatta kalabilir.
  Unix tarafı `setpgid`/process-group kill ile bunu doğru yapar. Gerçek
  bir Windows ana bilgisayarda çalışma zamanı testi gerekiyor (bu sandbox
  Linux'tur, gerçek bir Windows süreç ağacı üzerinde test edilemedi).
- **PowerShell shell hook'ları**: `comrade init powershell`'in ürettiği
  `$PROFILE` entegrasyonu golden testlerle doğrulandı, ancak gerçek bir
  PowerShell oturumunda (gerçek `$?`/`$LASTEXITCODE`/`Get-History`
  yakalama) hiç çalıştırılmadı.
- **Gerçek OS keychain**: macOS Keychain / Windows Credential Manager /
  Linux Secret Service ile gerçek entegrasyon bu sandbox'ta test
  edilemedi (go-keyring mock'u + enjekte edilebilir okuyucu ile
  doğrulandı). Kullanıcı bir platformda `comrade auth login` ile bir kez
  denemeli.
- **macOS/Windows uçtan uca senaryolar** (FAZ 11 madde 1): brew hatası,
  dosya izin hatası (macOS); `ExecutionPolicy` hatası, winget kurulumu,
  PATH sorunu (Windows) — bu Linux sandbox'ta çalıştırılamaz;
  `docs/phases/FAZ-11.md`'de her biri için tam komut + beklenen davranış
  belgelendi. Kullanıcı ilgili platformlarda bir kez manuel doğrulamalı.

### Ağ gerektiren doğrulamalar

- **Gerçek LLM kabul koşuşturmaları**: "docker kur" (FAZ 5), "pyton
  --version" (FAZ 7) gibi senaryolar `httptest` mock sunucularla uçtan
  uca doğrulandı; gerçek bir API anahtarıyla gerçek sağlayıcıya karşı
  hiç çalıştırılmadı.
- **`install.sh`/`install.ps1` ve `comrade upgrade`'in canlı ağ yolu**:
  gerçek bir GitHub Releases yayını hiç yapılmadığından (henüz tag
  atılmadı), yalnızca sahte/snapshot artefaktlarla doğrulandı. İlk
  gerçek release'te bir kez canlı doğrulanmalı.

### Yayın (release) hazırlığı — kullanıcı kararı bekliyor

- **Git tag atılmadı**: `v0.1.0-rc1` bilinçli olarak etiketlenmedi (tag
  atmak gerçek release pipeline'ını tetikler ve aşağıdaki hedef
  repo'lar henüz yok).
- **`homebrew-tap`/`scoop-bucket`/`winget-pkgs` hedef repo'ları henüz
  oluşturulmadı** — `goreleaser release --snapshot` bunlara ihtiyaç
  duymaz, ama gerçek bir `goreleaser release` bunlar olmadan brew/scoop/
  winget yayın adımlarında başarısız olur.
- **cosign imzalama** `.goreleaser.yaml`'de yorum satırı halinde
  belgelendi, etkinleştirilmedi — bir anahtar-sağlama kararı
  (keypair+secret vs. keyless OIDC) gerektiriyor.

### Tasarım gereği sınırlar (bilinçli seçimler, hata değil)

- **`anthropic`/`google` model listeleri statik bir anlık görüntüdür**
  (FAZ 8) — `ollama`/`openai_compat` gibi canlı `/models` sorgusu
  yapılmaz; dokümantasyon linkiyle birlikte sunulur.
- **i18n istisnaları**: cobra `Use` komut adları, `hook.go`'nun gizli
  `COMRADE_DEBUG` satırı, `promptui.go`'nun LLM prompt metni, ve ~40
  adet "işlem: %w" hata sarmalama zinciri — CLAUDE.md'nin kendi
  belgelediği, gerekçeli istisnalardır (bkz. `docs/phases/FAZ-09.md`).
  (`internal/tui/confirm.go`'nun onay harfleri — `[e]vet/[h]ayır/...` —
  daha önce burada listeliydi: sabit Türkçe idi ve `general.language`'ı
  takip etmiyordu. Düzeltildi — artık `internal/i18n` üzerinden, dile
  göre kesinlikle ayrık bir tuş kümesiyle çözülüyor: TR
  `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü`, EN
  `[y]es [n]o [e]dit [x]plain [a]ll`.)
- **`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>`
  bu RC'de desteklenmez**: FAZ 11'in vendorlanmış clipboard soğuk-başlangıç
  düzeltmesi (`go.mod`'daki yerel-dosya-yolu `replace` direktifi) Go'nun
  kendi kısıtlaması nedeniyle `@sürüm` biçiminde sert bir hatayla
  reddedilir (bir ana-modül bağlamı olmadan `replace` direktifleri
  yok sayılamaz/uygulanamaz — bkz. `docs/INSTALL.md`'nin "Kaynaktan
  derleme" bölümü, doğrulanmış tam hata metniyle birlikte). Bunun yerine
  bir kaynak checkout'undan kurun (`git clone` + `go build`/`go install
  ./cmd/comrade`) ya da ikili paketlerden birini kullanın (brew/scoop/
  winget/.deb/.rpm/`install.sh`/`install.ps1`) — bu paketler goreleaser
  ile checkout içinden derlendiği için `replace` direktifi normal
  şekilde uygulanır ve etkilenmezler.

---

## English

### Platform runtime — unverifiable in this sandbox

- **Windows process-tree kill**: `internal/executor`'s Windows branch
  (on timeout/Ctrl-C) kills only the direct child process; grandchild
  processes (children spawned by the command's own children) may
  survive. The Unix side does this correctly via `setpgid`/process-group
  kill. A real Windows host is needed for a runtime test (this sandbox
  is Linux; no real Windows process tree was available to test against).
- **PowerShell shell hooks**: `comrade init powershell`'s `$PROFILE`
  integration is verified with golden tests, but has never actually run
  in a real PowerShell session (real `$?`/`$LASTEXITCODE`/`Get-History`
  capture).
- **Real OS keychain**: real integration with macOS Keychain / Windows
  Credential Manager / Linux Secret Service could not be tested in this
  sandbox (verified instead with a go-keyring mock + an injectable
  reader). A user should try `comrade auth login` once on a real
  platform.
- **macOS/Windows end-to-end scenarios** (FAZ 11 item 1): a brew error,
  a file-permission error (macOS); an `ExecutionPolicy` error, a winget
  install, a PATH problem (Windows) — cannot run in this Linux sandbox;
  `docs/phases/FAZ-11.md` documents the exact command + expected
  behavior for each. A user should manually verify each once on the
  matching platform.

### Verifications that need real network access

- **Real-LLM acceptance runs**: scenarios like "docker kur" (FAZ 5) and
  "pyton --version" (FAZ 7) are verified end-to-end against `httptest`
  mock servers; never run against a real provider with a real API key.
- **`install.sh`/`install.ps1` and `comrade upgrade`'s live network
  path**: since no real GitHub Releases publish has ever happened yet
  (no tag cut), these are only verified against fake/snapshot artifacts.
  Should be verified live once against the first real release.

### Release preparation — awaiting a user decision

- **No git tag was created**: `v0.1.0-rc1` is deliberately not tagged
  (tagging triggers the real release pipeline, and the target repos
  below don't exist yet).
- **`homebrew-tap`/`scoop-bucket`/`winget-pkgs` target repos don't exist
  yet** — `goreleaser release --snapshot` doesn't need them, but a real
  `goreleaser release` will fail at the brew/scoop/winget publish steps
  without them.
- **cosign signing** is documented (commented out) in
  `.goreleaser.yaml`, not enabled — it needs a key-provisioning decision
  (a committed keypair + secret vs. keyless OIDC).

### Limits by design (deliberate choices, not bugs)

- **`anthropic`/`google` model lists are a static snapshot** (FAZ 8) —
  unlike `ollama`/`openai_compat`, there is no live `/models` query;
  a docs link is shown alongside the snapshot instead.
- **i18n exceptions**: cobra `Use` command names, `hook.go`'s hidden
  `COMRADE_DEBUG` diagnostic line, `promptui.go`'s LLM prompt text, and
  ~40 "doing X: %w" error-wrap chains are CLAUDE.md's own documented,
  justified exceptions (see `docs/phases/FAZ-09.md`).
  (`internal/tui/confirm.go`'s confirmation-option letters —
  `[e]vet/[h]ayır/...` — used to be listed here too: hardcoded Turkish,
  ignoring `general.language`. Fixed — it now resolves through
  `internal/i18n` with a strictly per-language, disjoint key set: TR
  `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü`, EN
  `[y]es [n]o [e]dit [x]plain [a]ll`.)
- **`go install github.com/firatkutay/cli-comrade/cmd/comrade@<version>`
  is unsupported at this RC**: FAZ 11's vendored clipboard cold-start fix
  (a local-filesystem `replace` directive in `go.mod`) is hard-rejected
  by Go's own `@version` install constraint (a `replace` directive
  cannot be honored/ignored without a main-module context — see
  `docs/INSTALL.md`'s "Build from source" section for the exact,
  verified error text). Install from a source checkout instead (`git
  clone` + `go build`/`go install ./cmd/comrade`), or use one of the
  binary packages (brew/scoop/winget/.deb/.rpm/`install.sh`/
  `install.ps1`) — those are built by goreleaser from within the
  checkout, so the `replace` directive is honored normally and they are
  unaffected.
