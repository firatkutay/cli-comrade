# Yapılandırma / Configuration

## Türkçe

### Dosya konumu

| Platform | Yol |
|---|---|
| Linux/macOS | `$XDG_CONFIG_HOME/cli-comrade/config.toml` (yoksa `~/.config/cli-comrade/config.toml`) |
| Windows | `%APPDATA%\cli-comrade\config.toml` |

Dosya ilk çalıştırmada varsayılan değerlerle otomatik oluşturulur.
Yolu görmek için: `comrade config path`. Görüntülemek için:
`comrade config list`. Tek bir anahtarı okumak/yazmak için:
`comrade config get <anahtar>` / `comrade config set <anahtar> <değer>`.

### Etkin değer önceliği

Her anahtar şu sırayla çözülür (yüksekten düşüğe): **ortam değişkeni**
(`COMRADE_...`) > **dosyadaki değer** > **yerleşik varsayılan**.
`comrade config list` her anahtarın kaynağını (`env`/`file`/`default`)
gösterir.

### Tüm anahtarlar

| Anahtar | Varsayılan | Açıklama | Ortam değişkeni |
|---|---|---|---|
| `general.mode` | `ask` | Davranış modu: `auto`/`ask`/`info` | `COMRADE_MODE` |
| `general.language` | `auto` | Arayüz dili: `auto`/`tr`/`en` | `COMRADE_GENERAL_LANGUAGE` (ayrıca `COMRADE_LANG`/`LANG`/`LC_ALL`, bkz. aşağı) |
| `general.color` | `true` | Renkli/lipgloss çıktı | `COMRADE_GENERAL_COLOR` |
| `general.update_check` | `true` | GitHub Releases'ı haftada en fazla bir kez kontrol et | `COMRADE_GENERAL_UPDATE_CHECK` |
| `llm.provider` | `anthropic` | `anthropic`/`openai_compat`/`google`/`ollama` | `COMRADE_PROVIDER` |
| `llm.model` | *(boş)* | Boşsa sağlayıcının kendi varsayılanı | `COMRADE_MODEL` |
| `llm.fallback` | `[]` | Yedek sağlayıcı/model listesi (virgülle ayrılmış) | `COMRADE_LLM_FALLBACK` |
| `llm.timeout_seconds` | `60` | Tek bir LLM isteği için zaman aşımı | `COMRADE_LLM_TIMEOUT_SECONDS` |
| `llm.idle_timeout_seconds` | `0` | Akış (stream) modunda iki chunk arası azami boşluk; `0` = kapalı (yalnızca `timeout_seconds` uygulanır) | `COMRADE_LLM_IDLE_TIMEOUT_SECONDS` |
| `llm.max_tokens` | `2048` | Yanıt başına azami token | `COMRADE_LLM_MAX_TOKENS` |
| `llm.openai_compat.base_url` | `https://api.openai.com/v1` | OpenAI-uyumlu uç nokta (Mistral/Groq/GLM/Qwen/Kimi/OpenRouter/LM Studio) | `COMRADE_LLM_OPENAI_COMPAT_BASE_URL` |
| `llm.ollama.base_url` | `http://localhost:11434` | Yerel Ollama sunucusu | `COMRADE_LLM_OLLAMA_BASE_URL` |
| `safety.confirm_destructive` | `true` | `destructive` komutlar her zaman onay ister (auto modda bile) | `COMRADE_SAFETY_CONFIRM_DESTRUCTIVE` |
| `safety.confirm_elevated` | `true` | sudo/admin gerektiren komutlar onay ister | `COMRADE_SAFETY_CONFIRM_ELEVATED` |
| `safety.denylist_extra` | `[]` | Kullanıcı tanımlı ek denylist regex'leri (virgülle ayrılmış) | `COMRADE_SAFETY_DENYLIST_EXTRA` |
| `safety.max_auto_steps` | `10` | auto modda istek başına azami adım | `COMRADE_SAFETY_MAX_AUTO_STEPS` |
| `context.send_history` | `false` | Son N komut geçmişini LLM'e gönder (opt-in) | `COMRADE_CONTEXT_SEND_HISTORY` |
| `context.history_depth` | `5` | `send_history=true` ise kaç komut gönderilsin | `COMRADE_CONTEXT_HISTORY_DEPTH` |
| `context.send_env_names` | `false` | Env var İSİMLERİni (değerlerini asla) gönder | `COMRADE_CONTEXT_SEND_ENV_NAMES` |
| `privacy.redact_emails` | `false` | E-posta adreslerini de maskele | `COMRADE_PRIVACY_REDACT_EMAILS` |
| `privacy.redact_ips` | `false` | IP adreslerini de maskele | `COMRADE_PRIVACY_REDACT_IPS` |
| `privacy.telemetry` | `false` | Anonim kullanım sayaçları (asla komut içeriği) | `COMRADE_PRIVACY_TELEMETRY` |
| `audit.enabled` | `true` | Her çalıştırılan komutu denetim kaydına yaz | `COMRADE_AUDIT_ENABLED` |
| `audit.retention_days` | `90` | Denetim kaydı saklama süresi | `COMRADE_AUDIT_RETENTION_DAYS` |
| `executor.step_timeout_seconds` | `300` | Tek bir çalıştırılan adımın azami süresi | `COMRADE_EXECUTOR_STEP_TIMEOUT_SECONDS` |

Genel kural: bir TOML anahtarı `bölüm.alt_alt` biçimindeyse, üreyen
ortam değişkeni `COMRADE_BÖLÜM_ALT_ALT` şeklindedir (nokta yerine alt
çizgi, büyük harf). `general.mode`/`llm.provider`/`llm.model` için
ayrıca kısa takma adlar da vardır (yukarıdaki tabloda "Ortam
değişkeni" sütununda ilk sırada gösterilenler); ikisi de çalışır.

### `base_url` doğrulama kuralları

`llm.openai_compat.base_url` ve `llm.ollama.base_url` düz metin string
değildir — `comrade config set` (ve config dosyası her yüklendiğinde)
`internal/config/validate.go`'daki aynı kurala göre doğrulanır, çünkü bu
değer API anahtarının `Authorization: Bearer` başlığıyla gönderileceği
hostu belirler:

- **Reddedilir** (`comrade config set` hata döndürür, değer kaydedilmez):
  şema `http`/`https` değilse, host boşsa, veya host bir bulut-metadata/
  link-local adrese işaret ediyorsa — `169.254.0.0/16` (AWS/GCP/Azure'un
  `169.254.169.254` metadata uç noktasını da kapsar) ya da IPv6
  `fe80::/10`.
- **Uyarılır ama kabul edilir**: şema `http` ve host loopback değilse
  (`localhost`, `127.0.0.0/8`, `::1` dışında) — API anahtarının ağ
  üzerinde şifrelenmemiş gönderileceğine dair bir uyarı basılır, değer
  yine de kaydedilir. Özel ağ aralıkları (`10/8`, `192.168/16`,
  `172.16/12`) kendi barındırılan Ollama/LM Studio gibi kurulumlar için
  meşru kabul edilir ve asla **reddedilmez** — ama diğer loopback-olmayan
  host'lar gibi, bu aralıklardan birine işaret eden bir `http` base_url
  yine aynı şifresiz-iletim uyarısını tetikler.
- Config her yüklendiğinde yalnızca **etkin sağlayıcının**
  (`llm.provider`) base_url'ü için aynı kontrol tekrar çalışır, ama
  yükleme asla bu yüzden başarısız olmaz — sadece uyarı basar (aksi halde
  hatalı bir değer `comrade config set` ile onarma yolunu da
  kilitleyebilirdi).
- Gerçek bir LLM istemcisi kurulurken (`do`/`fix`/`chat`/`explain`
  çalıştığında) etkin sağlayıcının base_url'ü tekrar kontrol edilir ve bu
  kez reddedilirse istemci hiç oluşturulmaz — `comrade config
  set`/`get`/`edit` gibi onarım komutları bu sert kontrolden geçmediği
  için her zaman kullanılabilir kalır. Ayrıntılar için bkz.
  [SECURITY.md](SECURITY.md).

### API anahtarları — ayrı bir mekanizma

API anahtarları YUKARIDAKİ config anahtarlarından değil, `comrade auth
login <provider>` ile ayrı bir kimlik bilgisi deposundan yönetilir
(bkz. SECURITY.md). Sağlayıcı bazlı ortam değişkeni takma adları da
vardır (öncelik sırasıyla):

| Sağlayıcı | Ortam değişkenleri |
|---|---|
| `anthropic` | `COMRADE_ANTHROPIC_API_KEY`, `ANTHROPIC_API_KEY` |
| `openai_compat` | `COMRADE_OPENAI_COMPAT_API_KEY`, `OPENAI_API_KEY` |
| `google` | `COMRADE_GOOGLE_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY` |
| `ollama` | *(anahtar gerekmez)* |

Çözümleme sırası: OS keychain > 0600 dosya yedeği > yukarıdaki ortam
değişkenleri.

### Dil çözümleme sırası

`general.language` `tr`/`en` olarak açıkça ayarlanmışsa kazanır; `auto`
(varsayılan) ise sırayla `COMRADE_LANG` > `LANG`/`LC_ALL` > **Windows
sistem yereli** (yalnızca Windows'ta; `GetUserDefaultLocaleName` ile
okunur, ör. `tr-TR`) > İngilizce'ye bakar. `LANG`/`LC_ALL` Unix
kuralları olduğundan Windows'ta genelde ayarlı değildir — bu adım
olmadan Windows'ta `auto` her zaman İngilizce'ye düşerdi; Linux/macOS'ta
davranış birebir aynı kalır (oradaki gerçek işletim sistemi yerel
mekanizması zaten `LANG`/`LC_ALL`'ın kendisidir, bu adım o platformlarda
her zaman boş döner).

---

## English

### File location

| Platform | Path |
|---|---|
| Linux/macOS | `$XDG_CONFIG_HOME/cli-comrade/config.toml` (falling back to `~/.config/cli-comrade/config.toml`) |
| Windows | `%APPDATA%\cli-comrade\config.toml` |

The file is created automatically, with defaults, on first run. See
its path with `comrade config path`; view it with `comrade config
list`; read/write a single key with `comrade config get <key>` /
`comrade config set <key> <value>`.

### Effective-value precedence

Every key resolves in this order (highest to lowest): **environment
variable** (`COMRADE_...`) > **value in the file** > **built-in
default**. `comrade config list` shows each key's actual source
(`env`/`file`/`default`).

### All keys

| Key | Default | Description | Env override |
|---|---|---|---|
| `general.mode` | `ask` | Behavior mode: `auto`/`ask`/`info` | `COMRADE_MODE` |
| `general.language` | `auto` | UI language: `auto`/`tr`/`en` | `COMRADE_GENERAL_LANGUAGE` (also `COMRADE_LANG`/`LANG`/`LC_ALL`, see below) |
| `general.color` | `true` | Colored/lipgloss output | `COMRADE_GENERAL_COLOR` |
| `general.update_check` | `true` | Check GitHub Releases at most once/week | `COMRADE_GENERAL_UPDATE_CHECK` |
| `llm.provider` | `anthropic` | `anthropic`/`openai_compat`/`google`/`ollama` | `COMRADE_PROVIDER` |
| `llm.model` | *(empty)* | Empty means the provider's own default | `COMRADE_MODEL` |
| `llm.fallback` | `[]` | Fallback provider/model chain (comma-separated) | `COMRADE_LLM_FALLBACK` |
| `llm.timeout_seconds` | `60` | Timeout for a single LLM request | `COMRADE_LLM_TIMEOUT_SECONDS` |
| `llm.idle_timeout_seconds` | `0` | Max gap between two chunks while streaming; `0` = disabled (only `timeout_seconds` applies) | `COMRADE_LLM_IDLE_TIMEOUT_SECONDS` |
| `llm.max_tokens` | `2048` | Max tokens per response | `COMRADE_LLM_MAX_TOKENS` |
| `llm.openai_compat.base_url` | `https://api.openai.com/v1` | OpenAI-compatible endpoint (Mistral/Groq/GLM/Qwen/Kimi/OpenRouter/LM Studio) | `COMRADE_LLM_OPENAI_COMPAT_BASE_URL` |
| `llm.ollama.base_url` | `http://localhost:11434` | Local Ollama server | `COMRADE_LLM_OLLAMA_BASE_URL` |
| `safety.confirm_destructive` | `true` | `destructive` commands always confirm (even in auto mode) | `COMRADE_SAFETY_CONFIRM_DESTRUCTIVE` |
| `safety.confirm_elevated` | `true` | sudo/admin-requiring commands confirm | `COMRADE_SAFETY_CONFIRM_ELEVATED` |
| `safety.denylist_extra` | `[]` | User-supplied extra denylist regexes (comma-separated) | `COMRADE_SAFETY_DENYLIST_EXTRA` |
| `safety.max_auto_steps` | `10` | Max steps per request in auto mode | `COMRADE_SAFETY_MAX_AUTO_STEPS` |
| `context.send_history` | `false` | Send recent command history to the LLM (opt-in) | `COMRADE_CONTEXT_SEND_HISTORY` |
| `context.history_depth` | `5` | How many commands to send when `send_history=true` | `COMRADE_CONTEXT_HISTORY_DEPTH` |
| `context.send_env_names` | `false` | Send env var NAMES (never values) | `COMRADE_CONTEXT_SEND_ENV_NAMES` |
| `privacy.redact_emails` | `false` | Also mask email addresses | `COMRADE_PRIVACY_REDACT_EMAILS` |
| `privacy.redact_ips` | `false` | Also mask IP addresses | `COMRADE_PRIVACY_REDACT_IPS` |
| `privacy.telemetry` | `false` | Anonymous usage counters (never command content) | `COMRADE_PRIVACY_TELEMETRY` |
| `audit.enabled` | `true` | Log every executed command to the audit log | `COMRADE_AUDIT_ENABLED` |
| `audit.retention_days` | `90` | Audit log retention window | `COMRADE_AUDIT_RETENTION_DAYS` |
| `executor.step_timeout_seconds` | `300` | Max seconds a single executed step may run | `COMRADE_EXECUTOR_STEP_TIMEOUT_SECONDS` |

General rule: a `section.sub_key` TOML key's generic env override is
`COMRADE_SECTION_SUB_KEY` (dots become underscores, upper-cased).
`general.mode`/`llm.provider`/`llm.model` additionally have short
aliases (shown first in the "Env override" column above); either form
works.

### `base_url` validation rules

`llm.openai_compat.base_url` and `llm.ollama.base_url` are not plain
strings — `comrade config set` (and every config load) validates them
against the same rule in `internal/config/validate.go`, because this
value decides which host receives the API key via an `Authorization:
Bearer` header:

- **Rejected** (`comrade config set` errors, the value is not saved): the
  scheme isn't `http`/`https`, the host is empty, or the host is a
  literal cloud-metadata / link-local address — `169.254.0.0/16` (which
  covers the `169.254.169.254` metadata endpoint used by AWS/GCP/Azure)
  or IPv6 `fe80::/10`.
- **Warned but accepted**: the scheme is `http` and the host is not
  loopback (anything other than `localhost`, `127.0.0.0/8`, `::1`) — a
  warning that the API key will be sent unencrypted is printed, but the
  value is still saved. Private network ranges (`10/8`, `192.168/16`,
  `172.16/12`) are treated as legitimate for self-hosted setups like
  Ollama/LM Studio and are never **rejected** — but like any other
  non-loopback host, an `http` base_url pointed at one still triggers the
  same cleartext warning.
- Every time config loads, the same check re-runs for the **active
  provider's** (`llm.provider`) base_url only, but loading never fails
  because of it — it only warns (a hard failure here could brick the
  repair path through `comrade config set` itself).
- When a real LLM client is actually built (running `do`/`fix`/`chat`/
  `explain`), the active provider's base_url is checked again, and a
  reject-class value this time means the client is never constructed —
  repair commands (`comrade config set`/`get`/`edit`) don't go through
  this hard check and stay usable regardless. See
  [SECURITY.md](SECURITY.md) for details.

### API keys — a separate mechanism

API keys are NOT one of the config keys above — they're managed
through a separate credential store via `comrade auth login
<provider>` (see SECURITY.md). Provider-specific env var aliases also
exist (checked in this priority order):

| Provider | Env vars |
|---|---|
| `anthropic` | `COMRADE_ANTHROPIC_API_KEY`, `ANTHROPIC_API_KEY` |
| `openai_compat` | `COMRADE_OPENAI_COMPAT_API_KEY`, `OPENAI_API_KEY` |
| `google` | `COMRADE_GOOGLE_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY` |
| `ollama` | *(no key needed)* |

Resolution order: OS keychain > 0600 file fallback > the env vars above.

### Language resolution order

An explicit `general.language` of `tr`/`en` wins outright; `auto` (the
default) falls through to `COMRADE_LANG` > `LANG`/`LC_ALL` > **Windows
system locale** (Windows only; read via `GetUserDefaultLocaleName`, e.g.
`tr-TR`) > English. `LANG`/`LC_ALL` are Unix conventions and are
typically unset on Windows — without this step, `auto` would always
fall back to English there; Linux/macOS behavior is unchanged (the real
OS locale mechanism there already IS `LANG`/`LC_ALL`, so this step
always returns empty on those platforms).
