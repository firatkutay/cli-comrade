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
| `general.show_usage` | `false` | Her çalıştırma sonrası token/maliyet özet satırı yaz (bkz. `--usage`) | `COMRADE_GENERAL_SHOW_USAGE` |
| `general.profile` | *(boş)* | Etkin config profili adı (bkz. aşağıdaki "Config profilleri") | `COMRADE_PROFILE` |
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

### Yerel LLM (Ollama)

comrade'i hiçbir API anahtarı gerektirmeden, tamamen çevrimdışı, yerel
bir modele karşı çalıştırın:

```sh
ollama pull llama3.1                     # önce modeli Ollama ile indirin
comrade config set llm.provider ollama
comrade config set llm.model llama3.1    # opsiyonel — boş bırakılırsa kurulu bir model otomatik seçilir
comrade "docker kur"
```

Anahtar gerekmez — `ollama`, `comrade auth login`/kimlik bilgisi
deposu üzerinden çözülecek hiçbir şeyi olmayan tek sağlayıcıdır.

`comrade config models`, etkin sağlayıcı için o an kullanılabilir
modelleri listeler ve interaktif olarak birini seçmenizi sağlar (seçim
`llm.model`'e kaydedilir): `ollama` için `llm.ollama.base_url`'in canlı
model listesini sorgular; `openai_compat` için aynı şekilde o uç
noktanın model listesini sorgular.

**Uzak Ollama sunucusu** — `llm.ollama.base_url`'i yerel varsayılan
yerine erişilebilir herhangi bir Ollama sunucusuna yönlendirin:

```sh
comrade config set llm.ollama.base_url http://<host>:11434
```

**Fallback zinciri** — `llm.fallback` (`KindStringSlice`), birincil
sağlayıcı hata verir veya zaman aşımına uğrarsa sırayla denenecek
`<sağlayıcı>/<model>` girdilerinin virgülle ayrılmış bir listesidir.
Diğer her anahtar gibi `comrade config set` ile ayarlanır:

```sh
comrade config set llm.fallback ollama/llama3.1,openai_compat/gpt-4o-mini
```

### OpenAI-uyumlu sağlayıcılar (Qwen, Groq, Mistral, OpenRouter, LM Studio, ...)

`openai_compat`, her OpenAI-uyumlu uç nokta tarafından paylaşılan tek bir
connector'dur, ama `llm.model` varsayılan olarak yalnızca OpenAI'nin
kendisinde var olan `gpt-5.4-mini`'ye ayarlıdır.
`llm.openai_compat.base_url`'i başka bir sağlayıcıya yönlendirip
`llm.model`'i o sağlayıcının gerçekten sunduğu bir modele
ayarlamazsanız, istek zamanında şöyle bir hatayla başarısız olur:

```
openai_compat: http 404: The model 'gpt-5.4-mini' does not exist
```

Çözüm — hem `base_url`'i hem `llm.model`'i ayarlayın. Qwen/DashScope
örneği:

```sh
comrade config set llm.provider openai_compat
comrade config set llm.openai_compat.base_url https://dashscope-intl.aliyuncs.com/compatible-mode/v1
comrade config set llm.model qwen-plus     # ya da qwen-turbo / qwen-max
```

`comrade auth login openai_compat`, API anahtarını okuduktan sonra:
hâlâ gönderilmiş OpenAI varsayılanındaysanız `base_url`'i sorar (boş
geçmek OpenAI'de kalır); ardından `base_url` artık OpenAI-DIŞI **ve**
`llm.model` boşsa, model adını da sorar (ör. `qwen-plus` — boş
geçebilirsiniz, sonra `comrade config set llm.model` ile ayarlarsınız).
Anahtarı test eden ping'in sonucu "model bulunamadı" (gövdesinde
"model" geçen bir 404) derse, anahtar yine de kaydedilir ve size
`comrade config models` çalıştırıp ardından `comrade config set
llm.model <model>` demenizi söyler — yani leftover-default bir 404,
belirsiz bir "ağ sorunu" değil, doğrudan bir çözüm yönergesi verir.

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

### Config profilleri

Adlandırılmış profiller (ör. `work`/`personal`, bulut/yerel), config
dosyasının İÇİNDE `[profiles.<ad>]` tabloları olarak saklanır — ayrı
dosyalar DEĞİL. Hiçbir profil tanımlanmadıysa tamamen etkisizdir; mevcut
config dosyalarında hiçbir şey değişmez.

**Etkin değer önceliği** (yeni bir katman, ortam değişkeni her zaman en
üstte kalır):

```
varsayılanlar < dosya [general]/[llm]/... < dosya [profiles.<etkin>] < COMRADE_* ortam değişkenleri
```

**Etkin profil önceliği:** `--profile` bayrağı > `COMRADE_PROFILE` ortam
değişkeni > dosyadaki `general.profile` değeri > hiçbiri.

**Komutlar:**

| Komut | Ne yapar |
|---|---|
| `comrade config profile list` | Her profili, aktif olanı ve anahtar sayısını listeler |
| `comrade config profile show [<ad>]` | Bir profilin kendi anahtar/değerlerini yazdırır (varsayılan: etkin profil) |
| `comrade config profile use <ad>` | Profili etkinleştirir (`general.profile`'ı ayarlar) |
| `comrade config profile add <ad> [--from-current]` | Yeni (boş veya `--from-current` ile mevcut `[llm]` değerleriyle doldurulmuş) bir profil oluşturur |
| `comrade config profile remove <ad>` | Profili siler; etkin profil oysa `general.profile`'ı temizler |
| `comrade config profile set <ad> <anahtar> <değer>` | Bir profil içinde tek bir anahtarı doğrulayıp kaydeder |

**Sınırlamalar:**

- Kimlik bilgileri (API anahtarları) sağlayıcı bazlıdır, PROFİL bazlı
  DEĞİLDİR — aynı sağlayıcıyı kullanan iki profil aynı saklanmış anahtarı
  paylaşır.
- Bir profil `general.profile`'ı KENDİ İÇİNDE ayarlayamaz (bir profilin
  başka bir profili etkinleştirmesi sınırsız özyinelemeye yol açardı) —
  `comrade config profile set <ad> general.profile ...` reddedilir.
- Bir profil `safety.*` anahtarlarını GEÇERSİZ KILABİLİR, ama
  `profile use`/`profile show` bunu yaptığında VURGULU bir uyarı basar.
  Çalışma zamanındaki destructive/elevated onay kapısı bundan etkilenmez
  — yalnızca `--yolo` + `safety.confirm_destructive=false` birlikte bunu
  atlatabilir (bkz. yukarıdaki güvenlik istisnası).
- Tanımsız bir etkin profil veya bir profil içindeki bilinmeyen bir
  anahtar asla config yüklemesini BAŞARISIZ KILMAZ — stderr'e (İngilizce,
  diğer paket-içi uyarılar gibi) bir uyarı yazılır ve yok sayılır.

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
| `general.show_usage` | `false` | Print a per-run token/cost summary line (see `--usage`) | `COMRADE_GENERAL_SHOW_USAGE` |
| `general.profile` | *(empty)* | Active config profile name (see "Config profiles" below) | `COMRADE_PROFILE` |
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

### Local LLM (Ollama)

Run comrade entirely offline against a locally-served model:

```sh
ollama pull llama3.1                     # pull the model with Ollama first
comrade config set llm.provider ollama
comrade config set llm.model llama3.1    # optional — empty auto-picks a pulled model
comrade "install docker"
```

No API key is needed — `ollama` is the one provider with nothing to
resolve through `comrade auth login`/the credential store.

`comrade config models` lists the models currently available for the
active provider and lets you pick one interactively (persisting the
choice to `llm.model`): for `ollama` it queries `llm.ollama.base_url`'s
live model list; for `openai_compat` it queries that endpoint's model
list the same way.

**Remote Ollama host** — point `llm.ollama.base_url` at any reachable
Ollama server instead of the local default:

```sh
comrade config set llm.ollama.base_url http://<host>:11434
```

**Fallback chain** — `llm.fallback` (`KindStringSlice`) is a
comma-separated list of `<provider>/<model>` entries, tried in order if
the primary provider errors or times out. It's set like any other key,
via `comrade config set`:

```sh
comrade config set llm.fallback ollama/llama3.1,openai_compat/gpt-4o-mini
```

### OpenAI-compatible providers (Qwen, Groq, Mistral, OpenRouter, LM Studio, ...)

`openai_compat` is one connector shared by every OpenAI-compatible
endpoint, but `llm.model` defaults to `gpt-5.4-mini` — a model that only
exists on OpenAI itself. Pointing `llm.openai_compat.base_url` at a
different provider without also setting `llm.model` to a model that
provider actually serves fails at request time with an error like:

```
openai_compat: http 404: The model 'gpt-5.4-mini' does not exist
```

Fix — set both `base_url` and `llm.model`. Qwen/DashScope example:

```sh
comrade config set llm.provider openai_compat
comrade config set llm.openai_compat.base_url https://dashscope-intl.aliyuncs.com/compatible-mode/v1
comrade config set llm.model qwen-plus     # or qwen-turbo / qwen-max
```

`comrade auth login openai_compat`, after reading the API key: if
`base_url` is still pointed at the shipped OpenAI default, it prompts
for the provider's address (bare Enter keeps OpenAI); then, if
`base_url` is now non-OpenAI **and** `llm.model` is empty, it also
prompts for the model name (e.g. `qwen-plus` — you can leave it blank
and set it later with `comrade config set llm.model`). If the
verification ping that follows comes back as "model not found" (a 404
whose body mentions "model"), the key is still saved and you're told to
run `comrade config models` and then `comrade config set llm.model
<model>` — so a leftover-default 404 gives you a directive fix instead
of a vague "network issue" message.

### Language resolution order

An explicit `general.language` of `tr`/`en` wins outright; `auto` (the
default) falls through to `COMRADE_LANG` > `LANG`/`LC_ALL` > **Windows
system locale** (Windows only; read via `GetUserDefaultLocaleName`, e.g.
`tr-TR`) > English. `LANG`/`LC_ALL` are Unix conventions and are
typically unset on Windows — without this step, `auto` would always
fall back to English there; Linux/macOS behavior is unchanged (the real
OS locale mechanism there already IS `LANG`/`LC_ALL`, so this step
always returns empty on those platforms).

### Config profiles

Named profiles (e.g. `work`/`personal`, cloud/local) are stored as
`[profiles.<name>]` tables INSIDE the config file — not separate files.
Inert until a profile is defined: an existing config file is completely
unaffected.

**Effective-value precedence** (one new layer, environment stays king):

```
defaults < file [general]/[llm]/... < file [profiles.<active>] < COMRADE_* env
```

**Active-profile precedence:** `--profile` flag > `COMRADE_PROFILE` env >
the file's own `general.profile` value > none.

**Commands:**

| Command | What it does |
|---|---|
| `comrade config profile list` | Lists every profile, which is active, and its key count |
| `comrade config profile show [<name>]` | Prints a profile's own key/value pairs (defaults to the active profile) |
| `comrade config profile use <name>` | Activates a profile (sets `general.profile`) |
| `comrade config profile add <name> [--from-current]` | Creates a new profile (empty, or seeded from the current `[llm]` section with `--from-current`) |
| `comrade config profile remove <name>` | Deletes a profile; clears `general.profile` if it pointed there |
| `comrade config profile set <name> <key> <value>` | Validates and persists a single key inside a profile |

**Boundaries:**

- Credentials (API keys) are per-PROVIDER, not per-profile — two profiles
  using the same provider share one stored key.
- A profile cannot set `general.profile` INSIDE ITSELF (a profile
  activating another profile would be unbounded recursion) —
  `comrade config profile set <name> general.profile ...` is rejected.
- A profile MAY override `safety.*` keys, but `profile use`/`profile
  show` print a HIGHLIGHTED warning whenever it does. The runtime
  destructive/elevated confirmation gate itself is untouched by this —
  only `--yolo` plus `safety.confirm_destructive=false` together can ever
  bypass it (see the security exception above).
- An undefined active profile, or an unknown key inside a defined
  profile, never FAILS a config load — it prints a warning to stderr
  (in English, like every other config-package-level warning) and is
  ignored.
