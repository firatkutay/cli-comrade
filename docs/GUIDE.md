# Kullanım Kılavuzu / User Guide

Kurulumdan günlük kullanıma, baştan sona, terminalle arası iyi olmayanlar
için. / From install to daily use, start to finish — for people who
aren't comfortable with a terminal.

---

## Türkçe

### comrade nedir?

**comrade**, terminalde ne yazacağınızı bilmenize gerek kalmadan, düz
Türkçe veya İngilizce ile isteğinizi yazdığınızda sizin yerinize doğru
komutu üreten bir yapay zeka yoldaşıdır. Siz "docker kur" ya da "8080
portunu kim kullanıyor bul" dersiniz; comrade durumu (işletim sisteminiz,
son hatanız, çalışma dizininiz gibi) toplar, bir yapay zeka sağlayıcısına
sorar, adım adım bir plan üretir ve seçtiğiniz **davranış moduna** göre bu
planı ya çalıştırır, ya her adımda size sorar ya da sadece açıklar. Hiçbir
komut sizin bilginiz/onayınız olmadan sinsice çalışmaz — bkz. aşağıdaki
[3 davranış modu](#3-davranış-modu) ve [güvenlik](#güvenlik-sade-anlatım).

Terim: **"sağlayıcı" (provider)** — isteğinizi anlayıp yanıtlayan yapay
zeka servisi (Anthropic, Google, yerel Ollama gibi). comrade bir sağlayıcı
olmadan çalışamaz; ilk kurulumdan sonraki tek zorunlu adım budur.

### Kurulum

En hızlı yol, işletim sisteminize göre tek satırlık bir script'tir:

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

Homebrew (macOS/Linux) ve Scoop (Windows) kullananlar için:

```sh
brew install firatkutay/tap/comrade
```

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

Script'ler indirdiği paketi kurmadan **önce** checksum'a karşı doğrular;
kurulum dizini `PATH`'inizde değilse otomatik olarak ekler (yeni bir
terminal gerekir). Diğer kanallar (.deb/.rpm, ham arşivler, kaynaktan
derleme) ve ortam değişkeni referansı için: [docs/INSTALL.md](INSTALL.md).

Kurulumun bittiğini doğrulamak için yeni bir terminal açıp:

```sh
comrade --version
```

### İlk çalıştırma — bir sağlayıcı ayarlayın

comrade'i kurar kurmaz hiçbir yapay zeka komutu (`do`/`fix`/`explain`/
`chat`) çalışmaz — çünkü henüz bir sağlayıcı anahtarı yok. İki yol var:

**A) Bulut sağlayıcı (API anahtarı ile) — en hızlı yol**

```sh
comrade auth login anthropic
```

Bu, sizden API anahtarınızı ister, saklamadan **önce** küçük bir test
isteği gönderir ve anahtarı işletim sistemi keychain'inde saklar. `google`
(Gemini) için de aynı şekilde çalışır: `comrade auth login google`.

OpenAI, Qwen, Groq, Mistral, GLM/Zhipu, Kimi/Moonshot, OpenRouter, LM
Studio gibi **OpenAI-uyumlu** bir sağlayıcı kullanacaksanız:

```sh
comrade auth login openai_compat
```

Akış şöyle işler:

1. API anahtarınızı sorar.
2. Hâlâ varsayılan OpenAI adresindeyseniz, sağlayıcının adresini
   (`base_url`) sorar — sadece Enter'a basarsanız OpenAI'de kalırsınız.
3. Girdiğiniz adres OpenAI'den farklıysa **ve** henüz bir model
   seçilmediyse, kullanmak istediğiniz modelin adını sorar (ör.
   `qwen-plus`) — boş bırakabilirsiniz, sonra `comrade config set
   llm.model <model>` ile ayarlarsınız.
4. Anahtarı test eder: anahtar reddedilirse (401/403) hiçbir şey
   kaydedilmez; "model bulunamadı" (404) hatası alırsa anahtar yine de
   kaydedilir ve size `comrade config models` çalıştırıp doğru modeli
   seçmenizi söyler; başka bir hata olursa yine kaydeder ama
   doğrulayamadığını belirtir.

Girişten sonra, giriş yaptığınız sağlayıcı **etkin sağlayıcınız** olur
(`comrade "..."` artık onu kullanır ve girdiğiniz model ona uygulanır).

Örnek — Qwen/DashScope:

```sh
comrade auth login openai_compat
# API key: <anahtarınız>
# Provider address (base_url) [Enter = OpenAI]: https://dashscope-intl.aliyuncs.com/compatible-mode/v1
# Model [Enter = skip]: qwen-plus
```

**B) Yerel / çevrimdışı (Ollama) — anahtar gerekmez**

Kendi makinenizde çalışan bir modeli kullanmak isterseniz:

```sh
ollama pull llama3.1                     # önce modeli Ollama ile indirin
comrade config set llm.provider ollama
comrade config set llm.model llama3.1    # opsiyonel — boş bırakırsanız kurulu bir model otomatik seçilir
```

Ollama kurulu değilse [ollama.com](https://ollama.com)'dan kurun —
comrade bunu sizin için kurmaz. `comrade auth login ollama` diye bir şey
YOKTUR ve reddedilir; bu sağlayıcının hiç anahtara ihtiyacı yoktur.

**Kontrol edin:**

```sh
comrade auth status   # hangi sağlayıcılarda anahtar var, gösterir (değerleri asla yazdırmaz)
comrade doctor         # yapılandırmanızı uçtan uca kontrol eder
comrade doctor --live  # + gerçek, doğrulanmış bir istek gönderir (bir token harcar)
```

Ayrıntılı sağlayıcı/model referansı için: [docs/CONFIGURATION.md](CONFIGURATION.md).

### 3 davranış modu

comrade'in ne kadar "kendi başına" davranacağını belirleyen üç mod var:

| Mod | Ne yapar | Ne zaman kullanılır |
|---|---|---|
| `ask` (**varsayılan**) | Her adımdan önce kısa bir gerekçe + komutun kendisini gösterir, onay ister: `[e]vet / [h]ayır / [d]üzenle / [a]çıkla / [t]ümü`. "Tümü"nden sonra kalan write/network adımları sormadan çalışır, ama elevated/destructive adımlar yine tek tek sorar. | Günlük kullanım, yeni başlayanlar |
| `auto` | Her adımı kendisi çalıştırır, tek satırlık durum yazar. | Güvendiğiniz, tekrarlayan görevler |
| `info` | Hiçbir şey çalıştırmaz — sadece nedeni ve kopyalanabilir çözüm komutlarını açıklar. | Sadece ne olduğunu öğrenmek istediğinizde |

**`auto` modda bile** yıkıcı (`destructive`) bir adım HER ZAMAN onay
ister — bu, config'te `safety.confirm_destructive=false` **ve**
`--yolo` bayrağı birlikte kullanılmadıkça değişmez, ve o zaman bile her
kullanımda kırmızı bir uyarı basılır.

Modu değiştirme yolları:

```sh
comrade "docker kur" --auto      # sadece bu çalıştırma için
comrade config set general.mode auto   # kalıcı varsayılan
```

### Ana işler

**`comrade fix`** — son başarısız komutunuzu teşhis eder ve bir düzeltme
önerir. Bunun otomatik çalışması için önce shell entegrasyonunu kurmanız
GEREKİR:

```sh
comrade init        # shell'inizi otomatik algılar (bash/zsh/fish/PowerShell)
```

...ve ardından **terminalinizi yeniden başlatın** — kanca (hook) yeni
açılan oturumlara uygulanır, mevcut oturuma değil. Kurulmadıysa veya yeni
bir terminal açmadıysanız, `comrade fix` hiçbir hatayı otomatik yakalamaz;
bunun yerine:

```sh
comrade fix -- npm run build   # comrade komutu kendisi çalıştırıp gözlemler
```

ya da hatayı doğrudan terminale yapıştırmanız istenir (yapıştırma modu her
zaman çalışır, kancaya ihtiyaç duymaz).

**`comrade "<düz metin istek>"`** — varsayılan komuttur, ayrı bir alt
komut yazmanıza gerek yoktur (`comrade do "..."` ile aynı):

```sh
comrade "8080 portunu kim kullanıyor bul ve kapat"
comrade docker kur
```

**`comrade explain "<komut>"`** — bir komutu ÇALIŞTIRMADAN, bayrak bayrak
açıklar. Korkutucu bir komutu anlamanın güvenli yolu:

```sh
comrade explain "git rebase -i HEAD~5"
```

Komut tire (`-`) ile başlıyorsa: `comrade explain -- <komut>`.

**`comrade chat`** — bağlamı koruyan interaktif bir sohbet oturumu (gerçek
bir terminal/TTY gerektirir). Oturum içi komutlar:

| Komut | Ne yapar |
|---|---|
| `/do <istek>` | Gerçek, güvenlik-kontrollü comrade işlem hattını çalıştırır |
| `/mode auto\|ask\|info` | Bu oturumun modunu değiştirir |
| `/clear` | Sohbet geçmişini sıfırlar |
| `/save <dosya>` | Dökümü dosyaya kaydeder (oturum başka hiçbir şekilde diske yazılmaz — gizlilik) |
| `/help` | Yardımı gösterir |
| `/exit` | Oturumu sonlandırır |

### Günlük yardımcılar

- **`comrade undo`** — geri alınabilir son işlemi tersine çevirir, ya da
  elle yapılacak adımları gösterir. Her zaman `ask` modunda çalışır — bir
  `--yolo` yolu YOKTUR, çünkü bir geri alma orijinal işlemden daha az
  değil, daha fazla dikkat ister. `comrade undo --list` son geri alınabilir
  işlemleri listeler; `audit.retention_days` (varsayılan 90 gün) ile
  sınırlıdır.
- **`comrade history`** — denetim kaydından (audit log) son çalıştırılan
  komutları gösterir (`--limit 50` gibi).
- **`comrade config`** — `get <anahtar>` / `set <anahtar> <değer>` /
  `list` (her anahtarın kaynağını da gösterir) / `models` (etkin
  sağlayıcı için modelleri listeler, seçip `llm.model`'e kaydeder) /
  `edit` (`$EDITOR`'da açar, yoksa vi/Windows'ta notepad) / `path`
  (dosya yolunu yazdırır).
- **Profiller** — iş/kişisel gibi ayrı yapılandırmalar için:
  `comrade config profile add work --from-current`, `comrade config
  profile use work`, ya da tek seferlik `--profile work`. Dikkat: `config
  set` dosya-seviyesi değeri değiştirir, AKTİF PROFİLİ değil — bir profil
  İÇİNDEKİ bir anahtarı değiştirmek için `comrade config profile set
  work llm.provider openai_compat` kullanın.
- **Plan önizlemesi** — `general.plan_review=ask` ayarlayın (ya da
  `--review` verin) ki adımlar çalışmadan önce yeniden sıralayabileceğiniz/
  atlayabileceğiniz/düzenleyebileceğiniz interaktif bir önizleme ekranı
  görün (`ask` modda, çok adımlı planlarda); `--no-review` bunu her
  zaman kapatır.
- **Maliyet görünürlüğü** — herhangi bir çalıştırmaya `--usage` ekleyin,
  ya da kalıcı olarak `comrade config set general.show_usage true` yapın;
  her çalıştırma sonunda token/maliyet özeti yazılır.
- **`comrade doctor`** — salt-okunur, uçtan uca bir öz-tanı; `--live` ile
  gerçek bir doğrulanmış istek gönderir (bir token harcar).
- **`comrade upgrade`** — yeni bir sürümü kontrol eder/kurar; `--check`
  sadece rapor verir, kurmaz. Yalnızca resmi paket kanallarından
  kurulmuş sürümlerde çalışır (kaynaktan `go build` ile yapılan yerel
  derlemelerde değil).

### Güvenlik, sade anlatım

- Her üretilen komut bir risk sınıfına etiketlenir: `read` (okuma) <
  `write` (yazma) < `network` (ağ) < `elevated` (sudo/admin) <
  `destructive` (geri alınamaz silme/format). comrade'in kendi yerel
  kural motoru bu etikete asla körü körüne güvenmez — sadece
  YÜKSELTEBİLİR, asla düşüremez.
- `elevated` ve `destructive` adımlar **her modda** onay ister — `auto`
  modda bile. Bu, `safety.confirm_destructive=false`/`safety.
  confirm_elevated=false` **ve** `--yolo` birlikte olmadıkça değişmez;
  o zaman bile her kullanımda kırmızı bir uyarı basılır.
- Sabit bir **kara liste (denylist)** vardır: `rm -rf /`, `mkfs`, `dd
  of=/dev/...`, `diskpart clean`, drive-root `Remove-Item -Recurse`,
  fork bomb gibi kalıplar hiçbir modda, hiçbir bayrakla çalışmaz —
  engellenir.
- Tam güvenlik modeli: [docs/SECURITY.md](SECURITY.md).

### Shell entegrasyonu ve öneriler

`comrade init [bash|zsh|fish|powershell]` (argümansız çağrılırsa
`$SHELL`'den otomatik algılar) hem `fix`'in ihtiyaç duyduğu kancayı hem de
Tab-tamamlamayı tek seferde kurar. Kurulumdan sonra **yeni bir terminal
açın** — değişiklik mevcut oturuma değil, yeni açılan kabuklara uygulanır.

`comrade ` yazıp bir boşluk bıraktığınızda öneriler nasıl görünür, shell'e
göre değişir:

| Shell | Boşlukta ne olur |
|---|---|
| zsh | Soluk gri satır-içi bir "hayalet" ipucu belirir (ör. `comrade auth login ` → `[anthropic\|openai_compat\|google]`) |
| PowerShell | Tab-tamamlama listesi satırın altında otomatik açılır |
| fish | fish'in kendi yerleşik yazarken-tamamlama özelliği önerileri gösterir |
| bash | Hayalet metin YOKTUR (readline'ın bir sınırı) — bunun yerine `comrade ` sonrası **Tab'a iki kez basın** |

### Dosyalar nerede duruyor?

| Ne | Linux/macOS | Windows |
|---|---|---|
| Config | `~/.config/cli-comrade/config.toml` (ya da `$XDG_CONFIG_HOME`) | `%APPDATA%\cli-comrade\config.toml` |
| Denetim kaydı + son komut | `~/.local/state/cli-comrade/` (ya da `$XDG_STATE_HOME`) | `%LOCALAPPDATA%\cli-comrade\` |
| API anahtarları | İşletim sistemi keychain'i (yoksa 0600 izinli dosya) — asla config'e düz metin yazılmaz | Aynı |

Bir sorunla karşılaşırsanız (Ollama bağlanmıyor, hayalet ipucu çıkmıyor,
PATH bulunamıyor, checksum hatası vb.): [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md).

---

## English

### What comrade is

**comrade** is an AI companion that lets you type a plain-language
request — in Turkish or English — instead of knowing what to type at the
terminal. You say "install docker" or "find and kill whatever's using
port 8080"; comrade gathers context (your OS, your last error, your
working directory), asks an AI provider, produces a step-by-step plan,
and — depending on your chosen **behavior mode** — either runs the plan,
asks you at every step, or just explains it. Nothing runs behind your
back — see [the 3 behavior modes](#3-behavior-modes) and
[safety](#safety-in-plain-words) below.

Term: a **"provider"** is the AI service that understands and answers
your request (Anthropic, Google, or a local Ollama, for example). comrade
can't do anything without one configured — that's the one required step
after install.

### Install

The fastest path is a one-line script for your OS:

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

If you use Homebrew (macOS/Linux) or Scoop (Windows):

```sh
brew install firatkutay/tap/comrade
```

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

The scripts verify the downloaded package against its checksum **before**
installing anything, and add the install directory to your `PATH`
automatically if it's missing (a new terminal is required). For other
channels (.deb/.rpm, raw archives, building from source) and the full
env-var reference: [docs/INSTALL.md](INSTALL.md).

Confirm it worked by opening a new terminal and running:

```sh
comrade --version
```

### First run — set up a provider

Right after install, no AI command (`do`/`fix`/`explain`/`chat`) will
work yet — there's no provider key configured. Two paths:

**A) Cloud provider (API key) — fastest**

```sh
comrade auth login anthropic
```

This asks for your API key, sends a small test request **before**
storing it, and saves it in your OS keychain. `google` (Gemini) works
the same way: `comrade auth login google`.

For an **OpenAI-compatible** provider — OpenAI, Qwen, Groq, Mistral,
GLM/Zhipu, Kimi/Moonshot, OpenRouter, LM Studio:

```sh
comrade auth login openai_compat
```

The flow:

1. Asks for your API key.
2. If you're still pointed at the shipped OpenAI default, asks for the
   provider's address (`base_url`) — press Enter to stay on OpenAI.
3. If the address you entered is non-OpenAI **and** no model is set yet,
   asks for the model name you want (e.g. `qwen-plus`) — you can leave it
   blank and set it later with `comrade config set llm.model <model>`.
4. Tests the key: a rejected key (401/403) is never saved; a "model not
   found" (404) still saves the key and tells you to run `comrade config
   models` to pick the right one; any other failure saves the key too,
   just flagging that it couldn't verify.

After login, the provider you logged into becomes your **active
provider** (`comrade "..."` now uses it, and the model you entered
applies to it).

Example — Qwen/DashScope:

```sh
comrade auth login openai_compat
# API key: <your key>
# Provider address (base_url) [Enter = OpenAI]: https://dashscope-intl.aliyuncs.com/compatible-mode/v1
# Model [Enter = skip]: qwen-plus
```

**B) Local / offline (Ollama) — no key needed**

To run a model on your own machine:

```sh
ollama pull llama3.1                     # pull the model with Ollama first
comrade config set llm.provider ollama
comrade config set llm.model llama3.1    # optional — leave unset to auto-pick a pulled model
```

Install Ollama from [ollama.com](https://ollama.com) if you haven't —
comrade does not install it for you. There is no `comrade auth login
ollama`; it's rejected outright, because this provider needs no key at
all.

**Verify:**

```sh
comrade auth status   # shows which providers have a key (never prints values)
comrade doctor         # checks your setup end-to-end
comrade doctor --live  # + sends a real, authenticated request (spends one token)
```

Full provider/model reference: [docs/CONFIGURATION.md](CONFIGURATION.md).

### 3 behavior modes

Three modes control how much comrade does on its own:

| Mode | What it does | When to use it |
|---|---|---|
| `ask` (**default**) | Before every step: a short rationale + the command itself, then confirms: `[y]es / [n]o / [e]dit / [x]plain / [a]ll`. After "all", remaining write/network steps run without asking, but elevated/destructive steps still ask individually. | Everyday use, beginners |
| `auto` | Runs each step itself, printing a one-line status. | Repeated tasks you already trust |
| `info` | Runs nothing — just explains the cause and copy-pasteable fix commands. | Just want to understand |

**Even in `auto` mode**, a `destructive` step ALWAYS asks for
confirmation — this only changes if `safety.confirm_destructive=false`
in config **and** `--yolo` are used together, and even then a red
warning prints on every use.

Switching mode:

```sh
comrade "install docker" --auto      # this run only
comrade config set general.mode auto   # persistent default
```

### The main things you'll do

**`comrade fix`** — diagnoses your last failed command and proposes a
fix. For it to auto-capture that error, you MUST set up shell
integration first:

```sh
comrade init        # auto-detects your shell (bash/zsh/fish/PowerShell)
```

...then **restart your terminal** — the hook applies to newly opened
shells, not your current session. Without it (or before restarting),
`comrade fix` never captures an error automatically; instead:

```sh
comrade fix -- npm run build   # comrade runs and observes the command itself
```

or you'll be prompted to paste the error directly (paste mode always
works, no hook required).

**`comrade "<plain-language request>"`** — the default command, no
subcommand needed (same as `comrade do "..."`):

```sh
comrade "find and kill whatever's using port 8080"
comrade install docker
```

**`comrade explain "<command>"`** — explains a command flag-by-flag
WITHOUT running it. The safe way to understand a scary command:

```sh
comrade explain "git rebase -i HEAD~5"
```

If the command starts with a dash: `comrade explain -- <command>`.

**`comrade chat`** — an interactive, context-preserving chat session
(requires a real terminal/TTY). In-session slash commands:

| Command | What it does |
|---|---|
| `/do <request>` | Runs the real, safety-gated comrade pipeline |
| `/mode auto\|ask\|info` | Changes this session's mode |
| `/clear` | Resets the conversation history |
| `/save <file>` | Exports the transcript to a file (the only way a session is ever written to disk — privacy) |
| `/help` | Shows help |
| `/exit` | Ends the session |

### Everyday helpers

- **`comrade undo`** — reverses the last reversible action, or shows the
  manual steps. Always runs in `ask` mode — there is NO `--yolo` path
  here, since a reversal deserves more scrutiny than the original
  action, not less. `comrade undo --list` lists recent reversible
  actions; bounded by `audit.retention_days` (default 90 days).
- **`comrade history`** — shows recently executed commands from the
  audit log (e.g. `--limit 50`).
- **`comrade config`** — `get <key>` / `set <key> <value>` / `list`
  (also shows each key's source) / `models` (lists models for the
  active provider, picks and persists to `llm.model`) / `edit` (opens
  `$EDITOR`, or vi, or notepad on Windows) / `path` (prints the file
  path).
- **Profiles** — for separate setups like work/personal:
  `comrade config profile add work --from-current`, `comrade config
  profile use work`, or a one-off `--profile work`. Note: `config set`
  writes the file-level value, NOT the active profile — to change a key
  INSIDE a profile use `comrade config profile set work llm.provider
  openai_compat`.
- **Plan preview** — set `general.plan_review=ask` (or pass `--review`)
  to see an interactive preview where you can reorder/skip/edit steps
  before they run (ask mode, multi-step plans); `--no-review` always
  forces it off.
- **Cost visibility** — add `--usage` to any run, or make it permanent
  with `comrade config set general.show_usage true`; a token/cost
  summary prints at the end of every run.
- **`comrade doctor`** — a read-only, end-to-end self-diagnostic;
  `--live` sends a real authenticated request (spends one token).
- **`comrade upgrade`** — checks for / installs a newer version;
  `--check` only reports, never installs. Only works on versions
  installed via the official package channels (not a local `go build`
  from source).

### Safety, in plain words

- Every generated command is labeled with a risk class: `read` <
  `write` < `network` < `elevated` (sudo/admin) < `destructive`
  (irreversible delete/format). comrade's own local rule engine never
  blindly trusts that label — it can only RAISE it, never lower it.
- `elevated` and `destructive` steps ask for confirmation **in every
  mode** — even `auto`. This only changes with
  `safety.confirm_destructive=false`/`safety.confirm_elevated=false`
  **and** `--yolo` together, and even then a red warning prints on
  every use.
- There's a fixed **denylist**: patterns like `rm -rf /`, `mkfs`, `dd
  of=/dev/...`, `diskpart clean`, drive-root `Remove-Item -Recurse`, and
  fork bombs never run in any mode, with any flag — they're blocked.
- Full safety model: [docs/SECURITY.md](SECURITY.md).

### Shell integration and suggestions

`comrade init [bash|zsh|fish|powershell]` (auto-detects from `$SHELL`
if you omit the shell) installs both the hook `fix` needs and Tab
completion in one step. Afterward, **open a new terminal** — the change
applies to newly opened shells, not your current session.

What happens when you type `comrade ` and hit space varies by shell:

| Shell | What happens on space |
|---|---|
| zsh | A dim inline "ghost" hint appears (e.g. `comrade auth login ` → `[anthropic\|openai_compat\|google]`) |
| PowerShell | The Tab-completion list auto-opens below the line |
| fish | fish's own built-in as-you-type completion shows suggestions |
| bash | No ghost text (a readline limitation) — press **Tab twice** after `comrade ` instead |

### Where things live

| What | Linux/macOS | Windows |
|---|---|---|
| Config | `~/.config/cli-comrade/config.toml` (or `$XDG_CONFIG_HOME`) | `%APPDATA%\cli-comrade\config.toml` |
| Audit log + last command | `~/.local/state/cli-comrade/` (or `$XDG_STATE_HOME`) | `%LOCALAPPDATA%\cli-comrade\` |
| API keys | OS keychain (falls back to a 0600-permission file) — never plaintext in config | Same |

If something goes wrong (Ollama won't connect, no ghost hint, PATH not
found, a checksum error, ...): [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md).
