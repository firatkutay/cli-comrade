# Bilinen Kısıtlar / Known Limitations

Bu dosya, mevcut sürüm hattının (şu anda `v0.3.0`) dürüst "bilinen
sorunlar" listesidir ve her sürümle güncel tutulur. Hiçbir madde
gizlenmedi ya da hafifletilmedi.

This file is the current release line's (currently `v0.3.0`) honest
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
- **Boşluk-tetiklemeli komut ipuçlarının görsel render'ı**: zsh hayalet-
  metin render'ı gerçek macOS 15.7.7 (zsh 5.9) üzerinde v0.2.0 QA'sında
  canlı doğrulandı — gerçek bir etkileşimli terminalde `comrade ` + boşluk
  tuşunun `line-pre-redraw` kancasını tetikleyip soluk (SGR 90 / fg=8)
  POSTDISPLAY ipucunu ekrana çizdiği, ham PTY byte yakalaması ve ekran
  görüntüsüyle kanıtlandı. PowerShell tamamlama-listesi render'ı gerçek
  5.1/7.6 üzerinde parse+kayıt+koruma testleriyle doğrulandı ama
  etkileşimli oturumda ekranda henüz görsel olarak doğrulanmadı; ayrıca
  PSReadLine 2.0 (stok 5.1) sessiz-geri çekilme dalı test edilmedi (test
  makinesinde 2.4.5 vardı).
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

v0.1.0'dan mevcut v0.3.0'a kadar her sürüm gerçek GitHub Releases olarak yayınlandı;
Homebrew (`firatkutay/tap`) ve Scoop (`firatkutay/scoop-bucket`) kanalları
v0.1.2/v0.1.3'ten bu yana canlı ve her release'de otomatik güncelleniyor.
Kalan açık maddeler:

- **winget**: `microsoft/winget-pkgs`'e `cli.comrade` kimliğiyle
  gönderildi, moderatör incelemesi bekliyor (bkz. `docs/INSTALL.md`).
- **Snap**: paket hazır (`snap/snapcraft.yaml` + classic confinement)
  ama Snap Store kaydı ve classic onayı bekliyor (bkz. `docs/INSTALL.md`).

### Güvenlik sertleştirmesi — bilinen kalan boşluklar (v0.3.0)

v0.3.0, kendi-kendini-güncelleme imza doğrulamasını, `base_url`
doğrulamasını, redaction kapsamını ve yıkıcı-komut sınıflandırıcısını
sertleştirdi (bkz. `docs/SECURITY.md`). Dürüstçe kalan boşluklar:

- **Yıkıcı komut sınıflandırıcısı imza tabanlıdır**, niyet tabanlı değil —
  bu yüzden tanınmayan bir getir aracı (httpie'nin `http` komutu, BSD
  `fetch`) `internal/safety/escalation.go`'daki fetch kalıplarından
  kasıtlı olarak hariç tutulur (her ikisi de sıradan kelimelerle/URL
  şema alt dizgileriyle çakışıp yanlış pozitif üretir), ve kabuk
  değişkeni dolaylaması (`R=rm; $R -rf /`) hiç yakalanmaz —
  `internal/safety/tokenize.go`'nun `normalizeCommand`'ı kasıtlı olarak
  değişken genişletmesi yapmaz. Uzun vadeli düzeltme imza listesini
  genişletmek değil, **niyet tabanlı** (komutun ne yapacağını
  yorumlayan) bir sınıflandırmaya geçmektir.
- **`base_url` alternatif kodlama (decimal/hex IP) reddetmez, yalnızca
  uyarır** — `internal/config/validate.go`'nun metadata/link-local
  kontrolü yalnızca `net.ParseIP`'nin ayrıştırdığı gerçek IP
  literallerini tanır; `169.254.169.254`'ün decimal/hex kodlanmış bir
  biçimi bu kontrolü es geçip yalnızca http+non-loopback uyarısını
  tetikler. Go'nun standart kütüphane çözümleyicisi böyle bir host
  adını zaten reddettiğinden bu pratikte istismar edilemez, ama kural
  kendi başına "reddet" değil "uyar" sınıfındadır.
- **Redaction, kaçış karakteri içermeyen `/` veya `@` taşıyan bozuk bir
  bağlantı dizesi parolasını kaçırabilir** — `internal/redact/redact.go`
  içindeki `connStringPattern`, parola sınıfını `[^@\s/]+` olarak
  tanımlar; kasıtlı olarak URL-kodlanmamış bir `/` ya da `@` içeren
  (yani zaten hatalı biçimlendirilmiş) bir DSN parolası, `@`'ye kadar
  eşleşmediği için maskelenmeden kalabilir. Standart biçimli DSN'ler
  etkilenmez.
- **STRICT konumdaki `~kullanıcı` çözümlemesi artık `destructive` değil
  `elevated` seviyesine çıkıyor** — `internal/safety/effect_bash.go`'nun
  `wordHasLeadingUnescapedTilde` kapısı, gerçek `os/user.Lookup` host
  çağrısını (saf-fonksiyon iddiasını bozan bir yan etki) STRICT
  (komut-sözcüğü/atama-değeri) konumundan tamamen kaldırdı — artık
  çözülemez (`indeterminate`) sayılıp `RiskElevated`'e düşüyor, önceden
  gerçek `os/user.Lookup` sonucuna göre `RiskDestructive`'e kadar
  çıkabiliyordu (örn. `R=~root/bin/rm; $R -rf /`). Nihai `Action`
  (`confirm`) değişmiyor, ama `RiskElevated` ve `RiskDestructive`
  `internal/engine/runner.go`'da FARKLI `--yolo` bypass bayraklarıyla
  (`safety.confirm_elevated=false` / `safety.confirm_destructive=false`)
  kapatılıyor — yani sadece `confirm_elevated=false` + `--yolo` açık bir
  kurulum, önceden main'de `confirm_destructive=false` GEREKTİREN bu dar
  kalıbı artık onaysız atlatabilir. Kabul edilen, kasıtlı bir değiş tokuş:
  host'a bağımlı gerçek bir syscall'ı kaldırmanın dürüst sonucu budur, bir
  gözden kaçırma değil.
- **May-not-execute gövdelerdeki (invalidation) çözümler artık `destructive`
  değil `elevated` tavanına takılıyor** — `internal/safety/effect_bash.go`'nun
  `resolveMayNotExecute`'u, bir `while`/`for` gövdesi, atlanan bir `elif`,
  ya da bir `if`/`case` dalı (hepsi "çalışabilir de çalışmayabilir de")
  bir değişkeni yeniden atadığında, o değişkeni ÇÖZÜLEMEZ olarak işaretler
  (eski değeri sessizce korumak yerine) — bu, gövde hiç çalışmasa bile eski
  tehlikeli değerin sessizce ezilmesini önleyen (kritik bir güvenlik açığını
  kapatan) sağlam davranıştır, ama dürüst maliyeti şudur: önceden gövde
  hiç MODELLENMEDİĞİ için (main, `if`/`while`/`for`/`case`'i hiç anlamıyordu)
  değişken ÖNCEKİ atamasından `RiskDestructive`'e kadar çözülebiliyorken,
  artık aynı komut için üst sınır her zaman `RiskElevated`'dir (örn.
  `R=rm; while false; do R=echo; done; $R -rf /` main'de `destructive`,
  artık `elevated`). Nihai `Action` (`confirm`) değişmiyor, ama tilde
  bulgusundaki AYNI `--yolo` etkileşimi burada da geçerli — ve kapsamı çok
  daha geniş (auditor'un korpuslarında 83 vaka). Kabul edilen, kasıtlı bir
  değiş tokuş: bir CRITICAL false-Allow'u kapatmanın dürüst sonucu budur.
- ~~**Bir döngü gövdesi TEK GEÇİŞTE çözülür**~~ — **DÜZELTİLDİ (issue #33)**:
  `internal/safety/effect_bash.go`'nun `resolveLoopBody`'si artık bir
  `for`/`while`/`until` gövdesini TEK bir sıralı geçiş yerine bir
  SABİT NOKTAYA (fixpoint) kadar çözer — gövdeyi kendi önceki sonucuna
  tekrar tekrar uygular (`maxLoopFixpointIterations` ile sınırlı, aynı
  paylaşılan `resolverBudget`/`maxScopeForks`/`maxEnvSize` koruması
  altında), ve bu zincir boyunca herhangi bir noktada değişen HER ismi
  geçersiz kılar. `X=echo; R=echo; for i in 1 2; do X=$R; R=rm; done; $X -rf /` artık `X`i ÇÖZÜLEMEZ olarak işaretleyip `Confirm`'e düşüyor
  (önceden main'de `Allow` idi). Detaylar için `resolveLoopBody`'nin kendi
  doc yorumuna ve `internal/safety/effect_loop_fixpoint_test.go`'daki
  regresyon testlerine bakın.

### CLI bayrağı — `--profile`, ham-argümanlı komutlarda çalışmıyor (issue #27)

- **`--profile <ad>`, `config set`/`config profile set`/`explain` üzerinde
  görmezden gelinir ya da hataya yol açar**: `--profile` kalıcı (persistent)
  bir bayraktır (`internal/cli/root.go:105-106`), ama üç leaf komut kendi
  konumsal argümanlarının (`-` ile başlayabilen) olduğu gibi geçmesi için
  `DisableFlagParsing: true` ayarlar — `config set`
  (`internal/cli/config.go:172`), `config profile set`
  (`internal/cli/configprofile.go:342`), ve `explain`
  (`internal/cli/explain.go:52`). cobra'nın `DisableFlagParsing: true`'su
  yalnızca o leaf'in kendi bayraklarını değil, root'un kalıcı bayrak
  ayrıştırmasını da atlar; bu üç komutta `--profile` pflag'e hiç ulaşmaz.
  `config set`/`config profile set`'te `--profile`/`<ad>` token'ları ham
  argüman olarak kalır, arity kontrolü başarısız olur ve komut kendi
  kullanım hatasıyla döner. `explain`'de arity kontrolü yoktur, bu yüzden
  bayrak SESSİZCE açıklanan komut metnine karışır ve istek varsayılan
  profille sessizce çalışır — hiçbir hata/uyarı yoktur. Geçici çözüm: bu
  üç komut için `--profile` yerine `COMRADE_PROFILE=<ad>` kullanın. Bkz.
  [issue #27](https://github.com/firatkutay/cli-comrade/issues/27).

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
  bu sürümde desteklenmez**: `docs/history/phases/FAZ-11.md`'in
  vendorlanmış clipboard soğuk-başlangıç düzeltmesi (`go.mod`'daki
  yerel-dosya-yolu `replace` direktifi) Go'nun kendi kısıtlaması
  nedeniyle `@sürüm` biçiminde sert bir hatayla reddedilir (bir
  ana-modül bağlamı olmadan `replace` direktifleri yok
  sayılamaz/uygulanamaz — bkz. `docs/INSTALL.md`'nin "Kaynaktan derleme"
  bölümü, doğrulanmış tam hata metniyle birlikte). Bunun yerine bir
  kaynak checkout'undan kurun (`git clone` +
  `go build`/`go install ./cmd/comrade`) ya da ikili paketlerden birini
  kullanın (brew/scoop/ winget/.deb/.rpm/`install.sh`/`install.ps1`) —
  bu paketler goreleaser ile checkout içinden derlendiği için `replace`
  direktifi normal şekilde uygulanır ve etkilenmezler.

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
- **Space-triggered command hint rendering**: zsh ghost-text rendering
  was live-verified on real macOS 15.7.7 (zsh 5.9) during v0.2.0 QA — in
  a real interactive terminal, the `comrade ` + space keypress fires the
  `line-pre-redraw` hook and paints the dimmed (SGR 90 / fg=8) POSTDISPLAY
  hint on screen, proven by raw PTY byte capture and an on-screen
  screenshot. PowerShell completion-list rendering is verified via
  parse+registration+guard tests on real 5.1/7.6 but not yet visually
  verified on screen in an interactive session; the PSReadLine 2.0 (stock
  5.1) silent-degradation branch is also untested (the test machine had
  2.4.5).
- **Real OS keychain**: macOS Keychain was live-verified end-to-end
  during v0.1.3 release QA on real macOS (Sequoia 15.7, arm64-emu QEMU
  VM), including `comrade auth login`. Windows Credential Manager /
  Linux Secret Service have not yet been verified by the maintainer on
  real hardware (verified instead with a go-keyring mock + an injectable
  reader). A user should try `comrade auth login` once on those
  platforms.
- **Keychain write over an SSH session (cosmetic)**: running
  `comrade auth login` over a non-console SSH session on macOS makes
  the keychain write fail with the raw `keychain set: exit status 36`
  (`errSecInteractionNotAllowed`) error instead of a friendly
  localized hint (found during v0.1.3 QA, minor/cosmetic). Workaround:
  run it in a local/console session (or with a GUI-unlocked keychain).
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

Every release from v0.1.0 through the current v0.3.0 has shipped as real GitHub
Releases. The Homebrew (`firatkutay/tap`) and Scoop
(`firatkutay/scoop-bucket`) channels have been live and auto-updated on
every release since v0.1.2/v0.1.3. Remaining open items:

- **winget**: submitted to `microsoft/winget-pkgs` under the id
  `cli.comrade`, awaiting moderator review (see `docs/INSTALL.md`).
- **Snap**: the package is prepared (`snap/snapcraft.yaml`, classic
  confinement) but awaiting Snap Store registration and classic-
  confinement approval (see `docs/INSTALL.md`).

### Security hardening — known residual gaps (v0.3.0)

v0.3.0 hardened self-update signature verification, `base_url`
validation, redaction coverage, and the destructive-command classifier
(see `docs/SECURITY.md`). The honest gaps that remain:

- **The destructive-command classifier is signature-based, not
  intent-based** — an unrecognized fetch tool (httpie's `http`
  command, BSD `fetch`) is deliberately excluded from
  `internal/safety/escalation.go`'s fetch patterns (both collide with
  ordinary English words / the `http(s)://` URL-scheme substring and
  would false-positive too broadly), and shell-variable indirection
  (`R=rm; $R -rf /`) is never caught at all —
  `internal/safety/tokenize.go`'s `normalizeCommand` deliberately does
  no variable expansion. The long-term fix is not a bigger signature
  allowlist but moving to **intent-based** classification
  (interpreting what the command will actually do).
- **`base_url` alt-encoding (decimal/hex IP) warns, it does not reject**
  — `internal/config/validate.go`'s metadata/link-local check only
  recognizes a literal IP address parsed by `net.ParseIP`; a
  decimal/hex-encoded form of `169.254.169.254` slips past that check
  and only trips the http+non-loopback warning. Go's standard-library
  resolver already refuses to treat such a hostname as a literal IP, so
  this is not exploitable in practice — but the rule itself is a "warn,"
  not a "reject."
- **Redaction can miss a malformed connection-string password containing
  an unencoded `/` or `@`** — `internal/redact/redact.go`'s
  `connStringPattern` defines the password class as `[^@\s/]+`; a DSN
  password that itself contains an unescaped `/` or `@` (already a
  malformed DSN) won't match through to the terminating `@` and can be
  left unmasked. Standard-shaped DSNs are unaffected.
- **A STRICT-position `~username` resolution now tops out at `elevated`,
  not `destructive`** — `internal/safety/effect_bash.go`'s
  `wordHasLeadingUnescapedTilde` gate removes the real `os/user.Lookup`
  host call (a side effect that broke the analyzer's pure-function claim)
  from STRICT (command-word/assignment-value) position entirely; such a
  word is now treated as unresolved (`indeterminate`) and caps out at
  `RiskElevated`, where it previously could reach `RiskDestructive` via
  the real Lookup result (e.g. `R=~root/bin/rm; $R -rf /`). The resulting
  `Action` (`confirm`) is unchanged, but `RiskElevated` and
  `RiskDestructive` are gated by DIFFERENT `--yolo` bypass flags in
  `internal/engine/runner.go` (`safety.confirm_elevated=false` vs.
  `safety.confirm_destructive=false`) — so a setup with only
  `confirm_elevated=false` + `--yolo` can now bypass unprompted a narrow
  shape that previously required `confirm_destructive=false` on main.
  Accepted as a deliberate trade-off: this is the honest consequence of
  removing a real, host-dependent syscall from this analyzer's resolution
  path, not an oversight.
- **A may-not-execute body's invalidated resolution now tops out at
  `elevated`, not `destructive`** — `internal/safety/effect_bash.go`'s
  `resolveMayNotExecute` marks a variable a `while`/`for` body, a skipped
  `elif`, or an `if`/`case` branch (all "may or may not actually run")
  reassigns as UNRESOLVABLE rather than silently keeping its prior value —
  the sound fix for a CRITICAL false-Allow (a body that never runs could
  otherwise overwrite an already-dangerous value with a benign one). The
  honest cost: since the body used to be completely unmodeled (main never
  understood `if`/`while`/`for`/`case` at all), the variable's PRIOR
  assignment could resolve all the way to `RiskDestructive`; now the same
  command caps at `RiskElevated` (e.g. `R=rm; while false; do R=echo; done; $R -rf /` was `destructive` on main, is now `elevated`). The
  resulting `Action` (`confirm`) is unchanged, but the SAME `--yolo`
  interaction documented for the tilde entry above applies here too — and
  this class is far larger (83 cases across the audit's corpora, vs. the
  tilde entry's narrower scope). Accepted as a deliberate trade-off: this
  is the honest consequence of closing a CRITICAL false-Allow, not an
  oversight.
- ~~**A loop body is resolved in a SINGLE PASS**~~ — **FIXED (issue #33)**:
  `internal/safety/effect_bash.go`'s `resolveLoopBody` now resolves a
  `for`/`while`/`until` body to a FIXPOINT instead of a single pass — it
  repeatedly re-applies the body to its own prior result (bounded by
  `maxLoopFixpointIterations`, under the same shared
  `resolverBudget`/`maxScopeForks`/`maxEnvSize` guard), and invalidates
  every name that ever changes anywhere along that chain.
  `X=echo; R=echo; for i in 1 2; do X=$R; R=rm; done; $X -rf /` now
  correctly invalidates `X` and falls to `Confirm` (previously `Allow` on
  main). See `resolveLoopBody`'s own doc comment for the full mechanism
  and `internal/safety/effect_loop_fixpoint_test.go` for the regression
  suite (the exact repro, a 3-iteration relay chain, a while-loop
  equivalent, a two-variable swap, a nested loop, and over-invalidation
  negative controls).

### CLI flag — `--profile` doesn't work on raw-arg commands (issue #27)

- **`--profile <name>` is ignored or errors on three raw-arg commands**:
  `--profile` is a persistent flag (`internal/cli/root.go:105-106`), but
  `config set` (`internal/cli/config.go:172`),
  `config profile set` (`internal/cli/configprofile.go:342`), and
  `explain` (`internal/cli/explain.go:52`) each set
  `DisableFlagParsing: true` so their own positional arguments (which may
  legitimately start with `-`) pass through untouched. cobra's
  `DisableFlagParsing: true` disables flag parsing for the whole
  invocation, not just that leaf's own flags — it skips root's
  persistent-flag parsing too, so `--profile` never reaches pflag on
  these three commands. On `config set` and `config profile set`, the
  literal `--profile`/`<name>` tokens land in `args`, the arity check
  fails, and the command errors with its own usage message. On
  `explain`, there is no arity check, so the flag is silently folded
  into the text being explained and the request silently runs under the
  default profile instead — no error, no indication the flag did
  nothing. Workaround: use `COMRADE_PROFILE=<name>` instead of
  `--profile` for these three commands. See
  [issue #27](https://github.com/firatkutay/cli-comrade/issues/27).

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
  `docs/INSTALL.md`'s "Build from source" section for the exact, verified
  error text). Install from a source checkout instead (`git clone` +
  `go build`/`go install ./cmd/comrade`), or use one of the
  binary packages (brew/scoop/winget/.deb/.rpm/`install.sh`/
  `install.ps1`) — those are built by goreleaser from within the
  checkout, so the `replace` directive is honored normally and they are
  unaffected.
