# Güvenlik / Security

## Türkçe

### Davranış modları ve onay modeli

| Mod | Davranış |
|---|---|
| `auto` | Komutları kendisi çalıştırır, her adımı tek satırda özetler. |
| `ask` | *(varsayılan)* Her komuttan önce gerekçe + komutu gösterir, `[e]vet/[h]ayır/[d]üzenle/[a]çıkla/[t]ümünü onayla` sorar. |
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

### Bağlam gönderiminden önce redaction (zorunlu, atlanamaz)

LLM'e gönderilen HER payload, gönderilmeden önce `internal/redact`'ten
geçer. Beş zorunlu desen ailesi her zaman aktiftir (kapatılamaz):
API key biçimleri (`sk-`, `ghp-`, `AKIA...` vb.), JWT'ler, PEM özel
anahtar blokları, `key=value`/`key: value` biçimli kimlik bilgisi
çiftleri (password/token/secret...), `Authorization: Bearer ...`
başlıkları. İki opsiyonel aile config ile açılır:
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
| `ask` | *(default)* Shows the rationale + command before each step, prompts `[y]es/[n]o/[e]dit/[a]sk-to-explain/[a]ll-approve`. |
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

### Redaction before any context is sent (mandatory, cannot be bypassed)

EVERY payload sent to the LLM passes through `internal/redact` first.
Five mandatory pattern families are always active (cannot be turned
off): API key formats (`sk-`, `ghp-`, `AKIA...`, etc.), JWTs, full PEM
private-key blocks, `key=value`/`key: value`-shaped credential pairs
(password/token/secret/...), `Authorization: Bearer ...` headers. Two
optional families are config-gated: `privacy.redact_emails`,
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

### The `--yolo` flag

Prints a red warning on every use (CLAUDE.md security rule #6). It
only has a real effect when config ALSO has both
`safety.confirm_destructive=false` AND `safety.confirm_elevated=false`
— otherwise the warning still prints, but destructive/elevated
confirmations are still required.
