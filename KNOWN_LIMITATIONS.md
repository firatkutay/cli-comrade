# Bilinen Kısıtlar / Known Limitations

Bu dosya, mevcut sürüm hattının (şu anda `v0.1.4`) dürüst "bilinen
sorunlar" listesidir ve her sürümle güncel tutulur. Hiçbir madde
gizlenmedi ya da hafifletilmedi.

This file is the current release line's (currently `v0.1.4`) honest
known-issues list, kept up to date with every release. Nothing here is
hidden or downplayed.

---

## Türkçe

### Platform çalışma zamanı — bakım ekibince gerçek donanımda henüz doğrulanmamış

- **Windows süreç-ağacı öldürme**: `internal/executor`'ın Windows dalı
  (timeout/Ctrl-C üzerine) tek süreci öldürür; torun süreçler (bir
  komutun başlattığı alt süreçlerin alt süreçleri) hayatta kalabilir.
  Unix tarafı `setpgid`/process-group kill ile bunu doğru yapar. Gerçek
  bir Windows ana bilgisayarda çalışma zamanı testi ile doğrulanması
  gerekiyor.
- **PowerShell shell hook'ları**: `comrade init powershell`'in ürettiği
  `$PROFILE` entegrasyonu golden testlerle doğrulandı, ancak bakım
  ekibince gerçek bir PowerShell oturumunda (gerçek
  `$?`/`$LASTEXITCODE`/`Get-History` yakalama) henüz çalıştırılmadı.
- **Gerçek OS keychain**: macOS Keychain, v0.1.3 sürüm QA'sında gerçek
  macOS'ta (Sequoia 15.7, arm64-emu QEMU VM) `comrade auth login` dahil
  uçtan uca canlı doğrulandı. Windows Credential Manager / Linux Secret
  Service ile gerçek entegrasyon bakım ekibince gerçek donanımda henüz
  doğrulanmadı (go-keyring mock'u + enjekte edilebilir okuyucu ile test
  edildi). Kullanıcı bu platformlarda `comrade auth login` ile bir kez
  denemeli.
- **SSH oturumu üzerinden keychain yazma (kozmetik)**: macOS'ta konsol
  olmayan bir SSH oturumu üzerinden `comrade auth login` çalıştırılırsa,
  keychain yazma işlemi ham `keychain set: exit status 36`
  (`errSecInteractionNotAllowed`) hatasıyla başarısız olur; kullanıcı
  dostu, yerelleştirilmiş bir ipucu yerine bu ham mesaj gösterilir
  (v0.1.3 QA'sında bulundu, minör/kozmetik). Geçici çözüm: komutu yerel/
  konsol bir oturumda (veya GUI ile kilidi açılmış bir keychain ile)
  çalıştırmak.
- **macOS/Windows uçtan uca senaryolar** (bkz. `docs/history/phases/FAZ-11.md`
  madde 1): brew hatası, dosya izin hatası (macOS); `ExecutionPolicy`
  hatası, winget kurulumu, PATH sorunu (Windows) — CI matrix'i bunları
  otomatik koşar, ancak `docs/history/phases/FAZ-11.md`'de her biri için
  ayrıca tam komut + beklenen davranış belgelendi. Kullanıcı ilgili
  platformlarda isteğe bağlı olarak bir kez manuel doğrulayabilir.

### Ağ gerektiren doğrulamalar

- **Gerçek LLM kabul koşuşturmaları**: "docker kur", "pyton --version"
  gibi senaryolar `httptest` mock sunucularla uçtan uca doğrulanır;
  gerçek bir API anahtarıyla gerçek sağlayıcıya karşı otomatik testlerde
  hiç çalıştırılmaz (kasıtlı — CI'da gerçek provider çağrısı yok).

### Yayın (release) kanalları — üçüncü taraf incelemesi bekleyenler

v0.1.0'dan v0.1.4'e beş sürüm gerçek GitHub Releases olarak yayınlandı;
Homebrew (`firatkutay/tap`) ve Scoop (`firatkutay/scoop-bucket`) kanalları
v0.1.2/v0.1.3'ten bu yana canlı ve her release'de otomatik güncelleniyor.
Kalan açık maddeler:

- **winget**: `microsoft/winget-pkgs`'e `cli.comrade` kimliğiyle
  gönderildi, moderatör incelemesi bekliyor (bkz. `docs/INSTALL.md`).
- **Snap**: paket hazır (`snap/snapcraft.yaml` + classic confinement)
  ama Snap Store kaydı ve classic onayı bekliyor (bkz. `docs/INSTALL.md`).
- **cosign imzalama** hâlâ etkin değil — `.goreleaser.yaml`'de yorum
  satırı halinde belgelendi (~232-241. satırlar), bir anahtar-sağlama
  kararı (keypair+secret vs. keyless OIDC) gerektiriyor.

### Tasarım gereği sınırlar (bilinçli seçimler, hata değil)

- **`anthropic`/`google` model listeleri statik bir anlık görüntüdür**
  (FAZ 8) — `ollama`/`openai_compat` gibi canlı `/models` sorgusu
  yapılmaz; dokümantasyon linkiyle birlikte sunulur.
- **i18n istisnaları**: cobra `Use` komut adları, `hook.go`'nun gizli
  `COMRADE_DEBUG` satırı, `promptui.go`'nun LLM prompt metni, ve ~40
  adet "işlem: %w" hata sarmalama zinciri — CLAUDE.md'nin kendi
  belgelediği, gerekçeli istisnalardır (bkz. `docs/history/phases/FAZ-09.md`).
  (`internal/tui/confirm.go`'nun onay harfleri — `[e]vet/[h]ayır/...` —
  daha önce burada listeliydi: sabit Türkçe idi ve `general.language`'ı
  takip etmiyordu. Düzeltildi — artık `internal/i18n` üzerinden, dile
  göre kesinlikle ayrık bir tuş kümesiyle çözülüyor: TR
  `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü`, EN
  `[y]es [n]o [e]dit [x]plain [a]ll`.)
- **`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>`
  bu sürümde desteklenmez**: `docs/history/phases/FAZ-11.md`'in vendorlanmış clipboard soğuk-başlangıç
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

### Platform runtime — not yet verified by the maintainer on real hardware

- **Windows process-tree kill**: `internal/executor`'s Windows branch
  (on timeout/Ctrl-C) kills only the direct child process; grandchild
  processes (children spawned by the command's own children) may
  survive. The Unix side does this correctly via `setpgid`/process-group
  kill. Needs verification with a runtime test on a real Windows host.
- **PowerShell shell hooks**: `comrade init powershell`'s `$PROFILE`
  integration is verified with golden tests, but has not yet been run
  by the maintainer in a real PowerShell session (real
  `$?`/`$LASTEXITCODE`/`Get-History` capture).
- **Real OS keychain**: macOS Keychain was live-verified end-to-end
  during v0.1.3 release QA on real macOS (Sequoia 15.7, arm64-emu QEMU
  VM), including `comrade auth login`. Windows Credential Manager /
  Linux Secret Service have not yet been verified by the maintainer on
  real hardware (verified instead with a go-keyring mock + an injectable
  reader). A user should try `comrade auth login` once on those
  platforms.
- **Keychain write over an SSH session (cosmetic)**: running `comrade
  auth login` over a non-console SSH session on macOS makes the keychain
  write fail with the raw `keychain set: exit status 36`
  (`errSecInteractionNotAllowed`) error instead of a friendly localized
  hint (found during v0.1.3 QA, minor/cosmetic). Workaround: run it in a
  local/console session (or with a GUI-unlocked keychain).
- **macOS/Windows end-to-end scenarios** (see `docs/history/phases/FAZ-11.md`
  item 1): a brew error, a file-permission error (macOS); an
  `ExecutionPolicy` error, a winget install, a PATH problem (Windows) —
  the CI matrix runs these automatically, and `docs/history/phases/FAZ-11.md`
  additionally documents the exact command + expected behavior for each.
  A user can optionally re-verify manually once on the matching
  platform.

### Verifications that need real network access

- **Real-LLM acceptance runs**: scenarios like "docker kur" and "pyton
  --version" are verified end-to-end against `httptest` mock servers;
  automated tests never call a real provider with a real API key
  (deliberate — no live provider calls in CI).

### Release channels — awaiting third-party review

Five releases (v0.1.0 through v0.1.4) have shipped as real GitHub
Releases. The Homebrew (`firatkutay/tap`) and Scoop
(`firatkutay/scoop-bucket`) channels have been live and auto-updated on
every release since v0.1.2/v0.1.3. Remaining open items:

- **winget**: submitted to `microsoft/winget-pkgs` under the id
  `cli.comrade`, awaiting moderator review (see `docs/INSTALL.md`).
- **Snap**: the package is prepared (`snap/snapcraft.yaml`, classic
  confinement) but awaiting Snap Store registration and classic-
  confinement approval (see `docs/INSTALL.md`).
- **cosign signing** is still not enabled — documented (commented out)
  in `.goreleaser.yaml` (~lines 232-241); it needs a key-provisioning
  decision (a committed keypair + secret vs. keyless OIDC).

### Limits by design (deliberate choices, not bugs)

- **`anthropic`/`google` model lists are a static snapshot** (FAZ 8) —
  unlike `ollama`/`openai_compat`, there is no live `/models` query;
  a docs link is shown alongside the snapshot instead.
- **i18n exceptions**: cobra `Use` command names, `hook.go`'s hidden
  `COMRADE_DEBUG` diagnostic line, `promptui.go`'s LLM prompt text, and
  ~40 "doing X: %w" error-wrap chains are CLAUDE.md's own documented,
  justified exceptions (see `docs/history/phases/FAZ-09.md`).
  (`internal/tui/confirm.go`'s confirmation-option letters —
  `[e]vet/[h]ayır/...` — used to be listed here too: hardcoded Turkish,
  ignoring `general.language`. Fixed — it now resolves through
  `internal/i18n` with a strictly per-language, disjoint key set: TR
  `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü`, EN
  `[y]es [n]o [e]dit [x]plain [a]ll`.)
- **`go install github.com/firatkutay/cli-comrade/cmd/comrade@<version>`
  is unsupported at this release**: `docs/history/phases/FAZ-11.md`'s vendored clipboard cold-start fix
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
