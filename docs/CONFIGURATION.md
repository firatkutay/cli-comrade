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
(varsayılan) ise sırayla `COMRADE_LANG` > `LANG`/`LC_ALL` > İngilizce'ye
bakar.

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
default) falls through to `COMRADE_LANG` > `LANG`/`LC_ALL` > English.
