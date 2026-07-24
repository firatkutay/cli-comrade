# Güvenlik / Security

## Türkçe

### Davranış modları ve onay modeli

| Mod | Davranış |
|---|---|
| `auto` | Komutları kendisi çalıştırır, her adımı tek satırda özetler. |
| `ask` | *(varsayılan)* Her komuttan önce gerekçe + komutu gösterir, `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü` sorar. |
| `info` | Hiçbir şey çalıştırmaz; sadece açıklar. |

**Pazarlık edilemez kural:** `auto` modda bile risk sınıfı
`destructive` olan bir komut HER ZAMAN onay ister. Bu davranış ancak
config'de **hem** `safety.confirm_destructive=false` **hem de**
`--yolo` flag'i birlikte verildiğinde kapanır — ve kapandığında her
kullanımda kırmızı bir uyarı basılır (`--yolo` kırmızı uyarısı, config
tarafındaki bypass koşulları o çalıştırmada gerçekten bir şeyi
atlatıp atlatmadığından bağımsız olarak, her `--yolo` kullanımında
basılır).

### Risk sınıflandırma

Her üretilen komut LLM tarafından sınıflandırılır, ardından yerel bir
kural motoru (regex/AST tabanlı, LLM'e güvenmeden) ikinci kez kontrol
eder — beş sınıf, artan risk sırasıyla: `read` → `write` → `network` →
`elevated` → `destructive`.

Yerel **denylist** (mod ne olursa olsun, LLM ne önerirse önersin,
HER ZAMAN bloklar):

- `rm -rf /` (veya `~`/`$HOME` kök silme)
- `mkfs` (dosya sistemi formatlama)
- `dd of=/dev/<disk>` (ham disk üzerine yazma)
- `diskpart clean` (disk bölüm tablosunu siler)
- PowerShell `Remove-Item`/`ri`/`rd`/`rmdir`/`del`/`erase`/`rm`
  takma adlarıyla `-Recurse <sürücü kökü>`
- `format <sürücü>:` (Windows format)
- fork bomb (`:(){:|:&};:` ve eşdeğerleri)
- `> /dev/<disk>` (gerçek bir disk aygıtına yönlendirme)

### Sertleştirilmiş yıkıcı komut tespiti (v0.3.0)

Yerel kural motoru (`internal/safety`), LLM'in bildirdiği risk sınıfını asla
nihai kabul etmez; imza tabanlı bir denylist/escalation kümesiyle bağımsızca
yeniden değerlendirir. v0.3.0 itibarıyla eskiden kaçırılan şu kalıplar da
yakalanır (denylist eşleşmesi → `Block`, escalation eşleşmesi → `Confirm`):

- `find ... -delete` (rm dışı toplu silme)
- disk'i doğrudan yok eden araç ailesi: `mke2fs`/`mkswap`/`mkdosfs`/
  `mkntfs`/`newfs` (her zaman); ve gerçek bir `/dev/<disk>` aygıtına
  yönlendirildiğinde `wipefs`, `blkdiscard`, `sgdisk`, silme
  bayraklarıyla `sfdisk`, `badblocks -w`, `cryptsetup
  luksFormat`/`reencrypt`/`erase`
- kök veya ev dizinini hedefleyen `chmod -R`/`chown -R` (yalnızca `777` değil,
  hangi mod olursa olsun)
- `mv ... /dev/null` (taşıyarak silme), `shred -u`, `truncate -s 0`
- Windows depolama cmdlet'leri: `Format-Volume`, `Clear-Disk`,
  `Initialize-Disk`, `Remove-Partition`
- getir-ve-çalıştır kalıpları: `curl ... | sh`, `bash <(curl ...)`,
  `bash -c "$(curl ...)"`, base64 decode + pipe, çıplak `eval`
- `reg delete ... /f`, `diskpart /s <script>`, HKLM:/HKCU: registry silme

Ayrıca: tüm eşleştirmeler büyük/küçük harften bağımsızdır (`rm -Rf /` da
yakalanır), `$(...)` komut ikamesi eşleştirmeden önce düzleştirilir
(`$(rm -rf /)` çıplak `rm -rf /` ile aynı şekilde görülür), ve
`safety.Engine.Evaluate`'den hiç geçmemiş bir adım sessizce `Allow`
sayılmaz — yürütmeden önce yeniden değerlendirmeye zorlanır (kapalı-hata /
fail-closed).

### Bağlam gönderiminden önce redaction (zorunlu, atlanamaz)

LLM'e gönderilen HER payload, gönderilmeden önce `internal/redact`'ten
geçer. Zorunlu desen aileleri her zaman aktiftir (kapatılamaz): API key
biçimleri — `sk-`, `ghp_`/`gho_`, `AKIA...`, Slack `xox[baprs]-`, Google
`AIza...`, GitHub `github_pat_...`/`ghs_...`, GitLab `glpat-...`, Stripe
`sk_live_`/`sk_test_`, Google OAuth `GOCSPX-...`, SendGrid `SG....`, npm
`npm_...`, GCP OAuth `ya29....`, Slack incoming-webhook URL'leri — JWT'ler,
PEM özel anahtar blokları, `key=value`/`key: value` biçimli kimlik bilgisi
çiftleri (bileşik/önekli adlar dahil: `DB_PASSWORD=`,
`AWS_SECRET_ACCESS_KEY=`), `scheme://kullanıcı:parola@` bağlantı dizeleri,
Azure `AccountKey=...`, ve `Authorization: Bearer ...` / `Authorization:
Basic ...` başlıkları. İki opsiyonel aile config ile açılır:
`privacy.redact_emails`, `privacy.redact_ips`. Env var İÇERİKLERİ
ASLA gönderilmez — yalnızca isimleri, o da `context.send_env_names`
ile opt-in edilirse.

### API anahtarı saklama: keychain birincil, dosya yedeği

`comrade auth login <sağlayıcı>` ile kaydedilen anahtarlar önce OS
keychain'e (macOS Keychain / Windows Credential Manager / Linux Secret
Service, `zalando/go-keyring` ile) yazılmaya çalışılır. Bir keychain
arka ucu bulunamazsa (örn. başsız/headless bir Linux makinesi), 0600
izinli bir dosyaya **base64 ile gizlenmiş** (şifrelenmiş DEĞİL) olarak
düşülür — bu geçiş her seferinde stderr'e açık bir uyarı basar.
API anahtarları HİÇBİR ZAMAN config dosyasına düz metin yazılmaz.
Gizli anahtarlar deposu tek bir arka uç kullanır — erişilebilir ise OS
keychain'i, aksi takdirde 0600 dosyasını — ve ortam değişkenleri
(`COMRADE_<SAĞLAYICI>_API_KEY`, sonra sağlayıcının kendi değişkeni)
yalnızca depo hiçbir anahtara sahip değilse sorgulanır (bkz.
CONFIGURATION.md).

### Sağlayıcı uç noktası doğrulama (base_url)

`llm.openai_compat.base_url` ve `llm.ollama.base_url` artık doğrulanır
(`internal/config/validate.go`) — API anahtarının hangi ana bilgisayara
gönderileceği kontrolsüz bırakılmaz:

- **Reddedilir** (`comrade config set` hata verir): şema `http`/`https`
  değilse, host boşsa, veya host bir bulut-metadata/link-local adrese işaret
  ediyorsa (`169.254.0.0/16` — AWS/GCP/Azure metadata uç noktasını
  `169.254.169.254` da kapsar — ya da IPv6 `fe80::/10`).
- **Uyarılır ama izin verilir**: şema `http` ve host loopback değilse
  (`localhost`/`127.0.0.0/8`/`::1` dışında) — API anahtarının ağ üzerinde
  şifrelenmemiş gönderileceği uyarısı basılır. Özel ağ aralıkları (`10/8`,
  `192.168/16`, `172.16/12`) kendi barındırılan LLM kurulumları için meşru
  sayılır ve reddedilmez.
- Config her yüklendiğinde (her `comrade` çalıştırmasında) yalnızca **etkin
  sağlayıcının** (`llm.provider`) base_url'ü için aynı kontrol tekrar
  çalışır — ama asla sert hata vermez, yalnızca uyarır (aksi halde bozuk bir
  değer `comrade config set` ile onarma yolunu bile kilitlerdi).
- Gerçek LLM istemcisi kurulurken (`do`/`fix`/`chat`/`explain` çalıştığında)
  etkin sağlayıcının base_url'ü tekrar kontrol edilir ve bu kez
  **reddedilirse istemci hiç oluşturulmaz** — API anahtarı tehlikeli bir
  hosta asla gönderilmez. `comrade config set/get/edit` gibi onarım
  komutları bu sert kontrolden geçmez, her zaman kullanılabilir kalır.

Tam anahtar/varsayılan tablosu için bkz. CONFIGURATION.md.

### Denetim kaydı (audit log)

`audit.enabled=true` (varsayılan) iken her çalıştırılan komut
`$XDG_STATE_HOME/cli-comrade/audit.jsonl`'a (Windows:
`%LOCALAPPDATA%\cli-comrade\audit.jsonl`) tek satırlık bir JSON kaydı
olarak eklenir: zaman damgası, orijinal istek, çalıştırılan komut,
risk sınıfı, mod, exit code, süre. `comrade history` bu kaydı okur.
`audit.retention_days` (varsayılan 90) kadar eski kayıtlar periyodik
olarak temizlenir.

### Telemetri: varsayılan kapalı

`privacy.telemetry` varsayılan olarak `false`'tur. Açılsa bile
gönderilen tek şey anonim kullanım sayaçlarıdır — asla komut içeriği,
asla kişisel veri.

### Kendi kendini güncelleme imza doğrulaması (cosign)

`comrade upgrade`, indirdiği `checksums.txt`'i güvenilir saymadan önce
ikili içine gömülü bir cosign genel anahtarına (`internal/update/cosign.pub`,
ECDSA P-256/SHA-256, saf Go ile doğrulanır) karşı imzasını doğrular —
tamamen çevrimdışı, Rekor/şeffaflık-kaydı sorgusu OLMADAN
(`--tlog-upload=false`). Sıra: imza → checksum → çıkarma/ikiliyi
değiştirme. Yayınlar CI'da goreleaser'ın `signs` bloğu + cosign ile
imzalanır. v0.3.0 itibarıyla gerçek anahtar gömülüdür ve imzalama
zorunludur: imzasız ya da geçersiz imzalı bir sürüm yükseltmeyi durdurur
(kapalı-hata/fail-closed). Ayrıntılar için bkz.
[UPDATE_SIGNING.md](UPDATE_SIGNING.md).

### `--yolo` flag'i

Her kullanımda kırmızı bir uyarı basar (CLAUDE.md güvenlik kuralı #6).
Yalnızca config'de `safety.confirm_destructive=false` **ve**
`safety.confirm_elevated=false` ayarları da varsa gerçek bir etkisi
olur; aksi halde uyarı basılır ama destructive/elevated onayları yine
istenir.

---

## English

### Behavior modes and the confirmation model

| Mode | Behavior |
|---|---|
| `auto` | Runs commands itself, printing a one-line summary per step. |
| `ask` | *(default)* Shows the rationale + command before each step, prompts `[y]es [n]o [e]dit [x]plain [a]ll`. |
| `info` | Runs nothing; only explains. |

**Non-negotiable rule:** even in `auto` mode, a command classified
`destructive` ALWAYS requires confirmation. This is bypassed only when
config has BOTH `safety.confirm_destructive=false` AND `--yolo` is
given — and every `--yolo` use prints a red warning regardless of
whether the config-side bypass actually changes anything for that
particular run.

### Risk classification

Every generated command is classified by the LLM, then independently
re-checked by a local rule engine (regex/AST-based, never trusting the
LLM) — five classes, in increasing risk order: `read` → `write` →
`network` → `elevated` → `destructive`.

The local **denylist** (blocks regardless of mode, regardless of what
the LLM suggested):

- `rm -rf /` (or a `~`/`$HOME` root delete)
- `mkfs` (filesystem format)
- `dd of=/dev/<disk>` (raw disk overwrite)
- `diskpart clean` (wipes a disk's partition table)
- PowerShell `Remove-Item`/`ri`/`rd`/`rmdir`/`del`/`erase`/`rm` alias
  with `-Recurse <drive root>`
- `format <drive>:` (Windows format)
- a fork bomb (`:(){:|:&};:` and equivalents)
- `> /dev/<disk>` (redirect into a real disk device)

### Hardened destructive-command detection (v0.3.0)

The local rule engine (`internal/safety`) never treats the LLM's declared
risk class as final — it independently re-checks every command against a
signature-based denylist/escalation set. As of v0.3.0 this additionally
catches patterns that previously slipped through (a denylist match →
`Block`, an escalation match → `Confirm`):

- `find ... -delete` (mass, non-`rm` deletion)
- the disk-destroying tool family: `mke2fs`/`mkswap`/`mkdosfs`/`mkntfs`/
  `newfs` (always); and, when pointed at a real `/dev/<disk>` device,
  `wipefs`, `blkdiscard`, `sgdisk`, `sfdisk` with destructive flags,
  `badblocks -w`, `cryptsetup luksFormat`/`reencrypt`/`erase`
- `chmod -R`/`chown -R` targeting a root or home directory (any mode, not
  just `777`)
- `mv ... /dev/null` (discard via move), `shred -u`, `truncate -s 0`
- Windows storage cmdlets: `Format-Volume`, `Clear-Disk`, `Initialize-Disk`,
  `Remove-Partition`
- fetch-and-execute shapes: `curl ... | sh`, `bash <(curl ...)`,
  `bash -c "$(curl ...)"`, base64 decode piped into an interpreter, a bare
  `eval`
- `reg delete ... /f`, `diskpart /s <script>`, HKLM:/HKCU: registry
  deletion

Also: every match is case-insensitive (`rm -Rf /` is caught too), `$(...)`
command substitution is flattened before matching (`$(rm -rf /)` is seen
exactly like a bare `rm -rf /`), and a step that never actually passed
through `safety.Engine.Evaluate` is never silently treated as `Allow` — it
is forced through re-evaluation before it can run (fail-closed).

### Redaction before any context is sent (mandatory, cannot be bypassed)

EVERY payload sent to the LLM passes through `internal/redact` first.
Mandatory pattern families are always active (cannot be turned off): API
key formats — `sk-`, `ghp_`/`gho_`, `AKIA...`, Slack `xox[baprs]-`, Google
`AIza...`, GitHub `github_pat_...`/`ghs_...`, GitLab `glpat-...`, Stripe
`sk_live_`/`sk_test_`, Google OAuth `GOCSPX-...`, SendGrid `SG....`, npm
`npm_...`, GCP OAuth `ya29....`, Slack incoming-webhook URLs — JWTs, full
PEM private-key blocks, `key=value`/`key: value`-shaped credential pairs
(including compound/prefixed names like `DB_PASSWORD=`,
`AWS_SECRET_ACCESS_KEY=`), `scheme://user:pass@` connection strings, Azure
`AccountKey=...`, and `Authorization: Bearer ...` / `Authorization: Basic
...` headers. Two optional families are config-gated: `privacy.redact_emails`,
`privacy.redact_ips`. Environment variable CONTENTS are never sent —
only their names, and only when `context.send_env_names` is opted in.

### API key storage: keychain primary, file fallback

Keys saved via `comrade auth login <provider>` are written to the OS
keychain first (macOS Keychain / Windows Credential Manager / Linux
Secret Service, via `zalando/go-keyring`). When no keychain backend is
reachable (e.g. a headless Linux machine), they fall back to a 0600
file, **base64-obfuscated** (NOT encrypted) — this fallback prints an
explicit stderr warning every time it's used. API keys are NEVER
written to the config file in plaintext. The secrets store uses a
single active backend — the OS keychain when available, otherwise a
0600 credentials file — and environment variables (`COMRADE_<PROVIDER>_API_KEY`,
then the provider's own vendor variable) are consulted only when the
store has no key (see CONFIGURATION.md).

### Provider endpoint validation (base_url)

`llm.openai_compat.base_url` and `llm.ollama.base_url` are now validated
(`internal/config/validate.go`) — the API key's destination host is no
longer left unchecked:

- **Rejected** (`comrade config set` errors out): the scheme isn't
  `http`/`https`, the host is empty, or the host is a literal
  cloud-metadata / link-local address (`169.254.0.0/16` — which covers the
  `169.254.169.254` metadata endpoint used by AWS/GCP/Azure — or IPv6
  `fe80::/10`).
- **Warned but allowed**: the scheme is `http` and the host is not loopback
  (anything other than `localhost`/`127.0.0.0/8`/`::1`) — a warning that the
  API key will be sent unencrypted is printed. Private network ranges
  (`10/8`, `192.168/16`, `172.16/12`) are treated as legitimate for
  self-hosted LLM setups and are never rejected.
- Every time config loads (every `comrade` invocation), the same check
  re-runs for the **active provider's** (`llm.provider`) base_url only —
  but it never hard-fails, only warns (a hard fail here could brick the
  repair commands themselves).
- When the real LLM client is actually built (running `do`/`fix`/`chat`/
  `explain`), the active provider's base_url is checked again, and this
  time a reject-class value means **the client is never constructed** — the
  API key is never handed to a dangerous host. Repair commands
  (`comrade config set/get/edit`) don't go through this hard check and stay
  usable regardless.

See CONFIGURATION.md for the full key/default table.

### Audit log

While `audit.enabled=true` (the default), every executed command is
appended as one JSON line to
`$XDG_STATE_HOME/cli-comrade/audit.jsonl` (Windows:
`%LOCALAPPDATA%\cli-comrade\audit.jsonl`): timestamp, the original
request, the command actually run, risk class, mode, exit code,
duration. `comrade history` reads this log. Entries older than
`audit.retention_days` (default 90) are periodically cleaned up.

### Telemetry: off by default

`privacy.telemetry` defaults to `false`. Even when enabled, the only
thing ever sent is anonymous usage counters — never command content,
never personal data.

### Self-update signature verification (cosign)

`comrade upgrade` verifies the signature of the `checksums.txt` it
downloads against a cosign public key embedded in the binary
(`internal/update/cosign.pub`, ECDSA P-256/SHA-256, verified in pure Go)
before trusting it — fully offline, with NO Rekor/transparency-log lookup
(`--tlog-upload=false`). Order: signature → checksum → extract/replace the
binary. Releases are signed in CI via goreleaser's `signs` block + cosign.
As of v0.3.0 a real key is embedded and signing is enforced: a missing or
invalid signature aborts the upgrade (fail-closed). See
[UPDATE_SIGNING.md](UPDATE_SIGNING.md) for details.

### The `--yolo` flag

Prints a red warning on every use (CLAUDE.md security rule #6). It
only has a real effect when config ALSO has both
`safety.confirm_destructive=false` AND `safety.confirm_elevated=false`
— otherwise the warning still prints, but destructive/elevated
confirmations are still required.
