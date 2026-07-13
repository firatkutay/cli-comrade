<div align="center">
<img width="1536" height="1024" alt="image" src="https://github.com/user-attachments/assets/809b8c42-081e-432d-82b3-cf7ae697d218" />

# cli-comrade

**A cross-platform AI CLI companion for people who don't want to fight the terminal.**
**Terminalle uğraşmak istemeyenler için cross-platform bir yapay zeka CLI yoldaşı.**

[![Release](https://img.shields.io/github/v/release/firatkutay/cli-comrade)](https://github.com/firatkutay/cli-comrade/releases)
[![CI](https://github.com/firatkutay/cli-comrade/actions/workflows/ci.yml/badge.svg)](https://github.com/firatkutay/cli-comrade/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/firatkutay/cli-comrade)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/firatkutay/cli-comrade)](go.mod)
![Platforms](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-informational)

Binary: `comrade`

**English** · [Türkçe](#türkçe)

</div>

---

## English

### What it is

**cli-comrade** is a cross-platform (Windows / macOS / Linux) AI CLI
companion for people who don't know, or don't want to deal with, the
terminal. You describe what you want in natural language; comrade analyzes
the situation, generates the shell command(s), and either runs them, asks
for confirmation, or just explains — depending on the active behavior mode.

Natural-language requests work in whatever language the configured LLM
understands — there's no language gate on input. What *is* fixed to
Turkish/English is the product's own surface: UI strings (`internal/i18n`)
and the language comrade instructs the LLM to answer in, resolved from
config `general.language` → `COMRADE_LANG` → `LANG`/`LC_ALL` → (Windows
only) the system locale → English fallback (`internal/i18n/lang.go`).

### See it in action

```text
$ comrade "free up disk space in /var/log"

  comrade: I'll compress log files older than 7 days, then remove ones
  older than 30 days that are already compressed.

  → find /var/log -name "*.log" -mtime +7 -exec gzip {} \;
  [write]  [y]es [n]o [e]dit [x]plain [a]ll: y
  ✓ compressed 14 files

  → find /var/log -name "*.log.gz" -mtime +30 -delete
  [destructive]  [y]es [n]o [e]dit [x]plain [a]ll: y
  ✓ removed 6 files, freed 212 MB
```

Note the `[destructive]` step still stopped for confirmation — that never
changes in `ask` mode, and even in `auto` mode it's the one thing that
always asks (see [Safety](#how-it-stays-safe) below).

### Core scenarios

| Command | What it does |
|---|---|
| `comrade fix` | Diagnoses the last failed command (captured via shell hook, or pass one explicitly) — its exit code and stderr — and proposes a fix. |
| `comrade "install docker"` (or `comrade do "..."`) | Turns a free-text request into a multi-step plan and runs it per the active mode. |
| `comrade explain "git rebase -i HEAD~5"` | Explains a command flag-by-flag without running it. |
| `comrade chat` | Interactive, context-preserving chat session. |

Plus setup/utility commands: `comrade auth` (login/logout/status),
`comrade config` (get/set), `comrade init` (shell integration),
`comrade history`, `comrade upgrade`.

### Behavior modes

| Mode | Behavior |
|---|---|
| `auto` | comrade runs each step itself, printing a one-line status per step. |
| `ask` | Before every command: a short rationale + the command itself, then `[y]es / [n]o / [e]dit / [x]plain / [a]ll`. **Default mode.** |
| `info` | Runs nothing — explains the cause and the fix as copy-pasteable commands. |

> The prompt and its accepted keys follow the interface language — a
> Turkish interface shows `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü` instead.

**Non-negotiable safety exception:** even in `auto` mode, any step classified
`destructive` always requires confirmation. This can only be waived by
setting `safety.confirm_destructive=false` in config *and* passing `--yolo`
together — and doing so prints a loud warning on every use.

### How it stays safe

- **Risk classification** — every generated step is labeled by the LLM as
  `read` / `write` / `network` / `elevated` / `destructive`.
- **Local rule engine + denylist** (`internal/safety`) — a regex/AST-based
  second check that never trusts the LLM's own label; hard-blocks known
  catastrophic patterns (`rm -rf /`, `mkfs`, `dd of=/dev/...`, `diskpart
  clean`, fork bombs, etc.) regardless of mode.
- **Redaction** (`internal/redact`) — every payload sent to the LLM is
  scrubbed of API-key-shaped strings, `password=`/`token=`, bearer headers,
  etc. before it leaves the machine.
- **Audit log** (`internal/audit`) — every executed command is recorded:
  timestamp, mode, command, risk class, exit code.
- **Keychain-backed secrets** — API keys go through the OS keychain
  (macOS Keychain, Windows Credential Manager, Linux Secret Service) with
  an explicit, opt-in 0600 file fallback; never plaintext in config.

Full model: [docs/SECURITY.md](docs/SECURITY.md).

### LLM providers

| Provider | Connector | Notes |
|---|---|---|
| Anthropic | `anthropic` | Native Messages API |
| OpenAI-compatible | `openai_compat` | One connector, `base_url`-driven — covers OpenAI, OpenRouter, Groq, Mistral, and other OpenAI-compatible endpoints |
| Google | `google` | Gemini API |
| Ollama | `ollama` | Local, `http://localhost:11434`, live model discovery |

A config-driven fallback chain tries providers in order if one errors or
times out.

### Install

| Channel | Command | Status |
|---|---|---|
| Install script (macOS/Linux) | `curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh \| sh` | ✅ live |
| Install script (Windows) | `irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 \| iex` | ✅ live |
| Homebrew | `brew install firatkutay/tap/comrade` | ✅ live |
| Scoop | `scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket`<br>`scoop install comrade` | ✅ live |
| .deb / .rpm / raw archives | [GitHub Releases](https://github.com/firatkutay/cli-comrade/releases) | ✅ live |
| winget | `winget install cli.comrade` | ⏳ pending — PR open against `microsoft/winget-pkgs`, awaiting moderator review |
| Snap | `sudo snap install cli-comrade --classic` | ⏳ pending — awaiting Snap Store registration + classic-confinement approval |

The install scripts download the matching release archive via GitHub's
no-API `releases/latest/download` redirect (or a tag-scoped URL when
pinned), verify it against that release's `checksums.txt` (`sha256sum -c` /
`Get-FileHash`) **before** installing anything, and print a `comrade init
<shell>` hint when done. Set `COMRADE_VERSION` (env var, or `-Version` on
Windows) to pin an exact release instead of installing the latest one.

Full details, env-var reference, and per-channel maintainer notes:
[docs/INSTALL.md](docs/INSTALL.md) and [docs/PACKAGING.md](docs/PACKAGING.md).

**Building from source** (Go developers):

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
make build          # -> ./comrade
```

`go install github.com/firatkutay/cli-comrade/cmd/comrade@<version>` is
**not supported** — `go.mod` has a local-path `replace` directive for a
vendored cold-start fix, and Go's own toolchain rejects `@version` installs
against a module whose `go.mod` contains `replace`/`exclude`. `git clone` +
`make build` sidesteps this because the checkout itself becomes the main
module — and it's exactly how every binary package above is built, so none
of them are affected. Details: [docs/INSTALL.md](docs/INSTALL.md).

### Quick start

```sh
comrade auth login anthropic  # store an API key for a provider (keychain, or file fallback)
comrade init                  # install the shell hook + Tab-completion (bash/zsh/fish/PowerShell)
comrade "install docker"      # or: comrade do "install docker"
comrade fix                   # after a failed command
comrade explain "git rebase -i HEAD~5"
comrade chat
```

### Shell completion

`comrade init <shell>` installs Tab-completion automatically alongside
the shell hook — no separate step. Once installed, pressing space also
triggers a live next-word hint on the shells that support it, sourced
from the exact same command tree as Tab-completion (`comrade __hint`,
hidden, ~4ms, silent on any error) so it can never drift from what Tab
would offer:

```text
comrade ▍ [auth|chat|config|do|explain|fix|help|history|init|upgrade]
```

| Shell | How suggestions appear |
|---|---|
| zsh | Space shows a dim inline ghost hint (e.g. `comrade auth login ` → `[anthropic\|openai_compat\|google]`); Tab-completion menu also works |
| PowerShell | Space auto-opens the Tab-completion list below the line; Tab also works |
| fish | Suggestions appear as you type (fish's native as-you-type completion) + Tab |
| bash | Tab / double-Tab only — readline has no ghost-text mechanism, and rebinding space would break magic-space and paste |

`comrade <Tab>` lists every visible command; `comrade auth <Tab>` lists
`login`/`logout`/`status`; `comrade auth login <Tab>` lists the known
providers; `comrade config get <Tab>` lists every real config key;
`comrade init <Tab>` lists the supported shells.

**Already have `comrade init` installed?** Completions and the space
hint are new content added on top of the existing hook — re-run
`comrade init <shell>` once to pick them up (it's idempotent: your
existing hook is left untouched, the new content is simply added
alongside it). Details:
[docs/TECHNICAL.md §9](docs/TECHNICAL.md#9-shell-integration).

### Docs

- [docs/INSTALL.md](docs/INSTALL.md) — every install channel, in detail.
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) — every config key.
- [docs/SECURITY.md](docs/SECURITY.md) — the full safety/security model.
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — common issues.
- [docs/TECHNICAL.md](docs/TECHNICAL.md) — technical documentation (EN).
- [docs/TECHNICAL.tr.md](docs/TECHNICAL.tr.md) — teknik dokümantasyon (TR).
- [docs/PACKAGING.md](docs/PACKAGING.md) — maintainer-facing package-channel activation.
- [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md) — honest known-issues list.
- [docs/history/](docs/history/) — development-history records (implementation plan, phase logs, progress notes).

### Build & develop

```sh
make build             # -> ./comrade
make test              # go test ./...
make lint              # golangci-lint (auto-installs the pinned version)
make vet               # go vet ./...
make cross             # -> dist/comrade-<os>-<arch>[.exe], all platforms
make release-check     # validate .goreleaser.yaml, no build
make release-snapshot  # full local dry-run of every release artifact
```

### License

MIT — see [LICENSE](LICENSE).

---

## Türkçe

### Nedir

**cli-comrade**, terminal bilgisi olmayan veya terminalle uğraşmak istemeyen
kullanıcılara komut satırında yoldaşlık eden, cross-platform (Windows /
macOS / Linux) bir yapay zeka CLI asistanıdır. İsteğinizi doğal dille
tarif edersiniz; comrade durumu analiz eder, gerekli shell komutlarını
üretir ve etkin davranış moduna göre ya çalıştırır, ya onay ister ya da
sadece açıklar.

Doğal dil istekleri, kullanılan LLM'in anladığı her dilde çalışır — girdi
tarafında bir dil kısıtlaması yoktur. Sabit olan TR/İngilizce çift, ürünün
kendi yüzeyidir: arayüz metinleri (`internal/i18n`) ve comrade'ın LLM'e
hangi dilde yanıt vermesini söylediği — bu da config'teki
`general.language` → `COMRADE_LANG` → `LANG`/`LC_ALL` → (yalnızca
Windows'ta) sistem yerel ayarı → İngilizce varsayılanı sırasıyla
belirlenir (`internal/i18n/lang.go`).

### Aksiyonda görün

```text
$ comrade "/var/log altında disk alanı boşalt"

  comrade: 7 günden eski log dosyalarını sıkıştıracağım, ardından
  30 günden eski ve zaten sıkıştırılmış olanları sileceğim.

  → find /var/log -name "*.log" -mtime +7 -exec gzip {} \;
  [write]  [e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü: e
  ✓ 14 dosya sıkıştırıldı

  → find /var/log -name "*.log.gz" -mtime +30 -delete
  [destructive]  [e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü: e
  ✓ 6 dosya silindi, 212 MB boşaldı
```

`[destructive]` adımının yine de onay için durduğuna dikkat edin — bu
`ask` modunda hiç değişmez, `auto` modda bile her zaman onay isteyen tek
şey budur (aşağıdaki [Güvenlik](#güvenliği-nasıl-sağlıyor) bölümüne bakın).

### Temel senaryolar

| Komut | Ne yapar |
|---|---|
| `comrade fix` | Son başarısız komutu (shell kancasıyla yakalanır, ya da elle verilir) — exit code'unu ve stderr'ini — teşhis eder ve bir çözüm önerir. |
| `comrade "docker kur"` (ya da `comrade do "..."`) | Serbest metin isteği çok adımlı bir plana çevirir ve etkin moda göre çalıştırır. |
| `comrade explain "git rebase -i HEAD~5"` | Bir komutu çalıştırmadan, bayrak bayrak açıklar. |
| `comrade chat` | Bağlamı koruyan interaktif sohbet oturumu. |

Ayrıca kurulum/yardımcı komutlar: `comrade auth` (login/logout/status),
`comrade config` (get/set), `comrade init` (shell entegrasyonu),
`comrade history`, `comrade upgrade`.

### Davranış modları

| Mod | Davranış |
|---|---|
| `auto` | comrade her adımı kendisi çalıştırır, her adımda tek satırlık durum yazar. |
| `ask` | Her komuttan önce kısa gerekçe + komutun kendisi gösterilir, ardından `[e]vet / [h]ayır / [d]üzenle / [a]çıkla / [t]ümü` sorulur. **Varsayılan mod budur.** |
| `info` | Hiçbir şey çalıştırmaz — nedeni ve çözüm adımlarını kopyalanabilir komutlarla açıklar. |

> Prompt ve kabul edilen tuşlar arayüz diline göre değişir — İngilizce
> arayüzde bunun yerine `[y]es [n]o [e]dit [x]plain [a]ll` gösterilir.

**Pazarlık edilemez güvenlik istisnası:** `auto` modda bile risk sınıfı
`destructive` olan her adım daima onay ister. Bu yalnızca config'te
`safety.confirm_destructive=false` **ve** `--yolo` bayrağı birlikte
verilerek kapatılabilir — bu durumda her kullanımda gürültülü bir uyarı
basılır.

### Güvenliği nasıl sağlıyor

- **Risk sınıflandırması** — üretilen her adım LLM tarafından `read` /
  `write` / `network` / `elevated` / `destructive` olarak etiketlenir.
- **Yerel kural motoru + denylist** (`internal/safety`) — LLM'in kendi
  etiketine hiç güvenmeyen, regex/AST tabanlı ikinci bir kontrol; bilinen
  yıkıcı kalıpları (`rm -rf /`, `mkfs`, `dd of=/dev/...`, `diskpart clean`,
  fork bomb vb.) mod ne olursa olsun sert biçimde engeller.
- **Redaction** (`internal/redact`) — LLM'e giden her payload, makineden
  çıkmadan önce API-key benzeri dizeler, `password=`/`token=`, bearer
  başlıkları vb. için temizlenir.
- **Audit log** (`internal/audit`) — çalıştırılan her komut kaydedilir:
  zaman damgası, mod, komut, risk sınıfı, exit code.
- **Keychain destekli sırlar** — API anahtarları işletim sistemi
  keychain'inden (macOS Keychain, Windows Credential Manager, Linux Secret
  Service) geçer; açık, opt-in bir 0600 dosya fallback'i vardır, config
  dosyasına asla düz metin yazılmaz.

Tam model: [docs/SECURITY.md](docs/SECURITY.md).

### LLM sağlayıcıları

| Sağlayıcı | Connector | Not |
|---|---|---|
| Anthropic | `anthropic` | Native Messages API |
| OpenAI uyumlu | `openai_compat` | `base_url` ile tek connector — OpenAI, OpenRouter, Groq, Mistral ve diğer OpenAI-uyumlu uç noktaları kapsar |
| Google | `google` | Gemini API |
| Ollama | `ollama` | Yerel, `http://localhost:11434`, canlı model keşfi |

Config'te tanımlı sıralı bir fallback zinciri, bir sağlayıcı hata verir
veya zaman aşımına uğrarsa sıradakine geçer.

### Kurulum

| Kanal | Komut | Durum |
|---|---|---|
| Kurulum script'i (macOS/Linux) | `curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh \| sh` | ✅ canlı |
| Kurulum script'i (Windows) | `irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 \| iex` | ✅ canlı |
| Homebrew | `brew install firatkutay/tap/comrade` | ✅ canlı |
| Scoop | `scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket`<br>`scoop install comrade` | ✅ canlı |
| .deb / .rpm / ham arşivler | [GitHub Releases](https://github.com/firatkutay/cli-comrade/releases) | ✅ canlı |
| winget | `winget install cli.comrade` | ⏳ beklemede — `microsoft/winget-pkgs`'e açılan PR, moderatör incelemesi bekliyor |
| Snap | `sudo snap install cli-comrade --classic` | ⏳ beklemede — Snap Store kaydı + classic-confinement onayı bekliyor |

Kurulum script'leri, eşleşen release arşivini GitHub'ın API gerektirmeyen
`releases/latest/download` yönlendirmesi (ya da sabitlenmişse tag'e özel
bir URL) üzerinden indirir, herhangi bir şey kurmadan **önce** o release'in
`checksums.txt`'ine karşı doğrular (`sha256sum -c` / `Get-FileHash`) ve
bitince bir `comrade init <shell>` ipucu basar. Belirli bir sürümü
sabitlemek için `COMRADE_VERSION` ortam değişkenini (Windows'ta `-Version`
parametresini) kullanın.

Tüm ayrıntılar, ortam değişkeni referansı ve kanal başına bakım notları:
[docs/INSTALL.md](docs/INSTALL.md) ve [docs/PACKAGING.md](docs/PACKAGING.md).

**Kaynaktan derleme** (Go geliştiricileri için):

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
make build          # -> ./comrade
```

`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>` biçimi
**desteklenmez** — `go.mod`'da vendorlanmış bir soğuk-başlangıç düzeltmesi
için yerel bir `replace` direktifi var ve Go'nun kendi araç zinciri,
`go.mod`'unda `replace`/`exclude` bulunan bir modüle karşı `@sürüm`
kurulumunu reddediyor. `git clone` + `make build` bunu aşar, çünkü
checkout'un kendisi ana modül haline gelir — yukarıdaki her ikili paket de
tam olarak bu şekilde derlendiği için hiçbiri bundan etkilenmez. Detaylar:
[docs/INSTALL.md](docs/INSTALL.md).

### Hızlı başlangıç

```sh
comrade auth login anthropic  # bir sağlayıcı için API anahtarı sakla (keychain, ya da dosya fallback'i)
comrade init                  # shell kancasını + Tab-tamamlamayı kur (bash/zsh/fish/PowerShell)
comrade "docker kur"          # ya da: comrade do "docker kur"
comrade fix                   # başarısız bir komuttan sonra
comrade explain "git rebase -i HEAD~5"
comrade chat
```

### Kabuk (shell) tamamlama

`comrade init <shell>`, shell kancasıyla birlikte Tab-tamamlamayı da
otomatik olarak kurar — ayrı bir adım yok. Kurulduktan sonra, bunu
destekleyen shell'lerde boşluk tuşu da canlı bir sonraki-kelime ipucu
tetikler; Tab-tamamlama ile tamamen aynı komut ağacından beslenir
(`comrade __hint`, gizli, ~4ms, herhangi bir hatada sessiz), bu yüzden
Tab'ın sunacağından asla sapamaz:

```text
comrade ▍ [auth|chat|config|do|explain|fix|help|history|init|upgrade]
```

| Shell | Öneriler nasıl görünür |
|---|---|
| zsh | Boşluk soluk bir satır-içi hayalet ipucu gösterir (ör. `comrade auth login ` → `[anthropic\|openai_compat\|google]`); Tab-tamamlama menüsü de çalışır |
| PowerShell | Boşluk, Tab-tamamlama listesini satırın altında otomatik açar; Tab de çalışır |
| fish | Öneriler yazarken kendiliğinden görünür (fish'in doğal yazarken-tamamlama özelliği) + Tab |
| bash | Yalnızca Tab / çift-Tab — readline'da hayalet-metin mekanizması yok, ve boşluğu yeniden bağlamak magic-space ve yapıştırmayı bozardı |

`comrade <Tab>` görünür her komutu listeler; `comrade auth <Tab>`
`login`/`logout`/`status`'u listeler; `comrade auth login <Tab>` bilinen
sağlayıcıları listeler; `comrade config get <Tab>` her gerçek config
anahtarını listeler; `comrade init <Tab>` desteklenen shell'leri listeler.

**`comrade init` zaten kurulu mu?** Tamamlamalar ve boşluk ipucu, mevcut
hook'un üzerine eklenen yeni içeriktir — bunları almak için `comrade
init <shell>`'i bir kez yeniden çalıştırın (idempotenttir: mevcut
hook'unuza dokunulmaz, yeni içerik yalnızca onun yanına eklenir).
Ayrıntılar:
[docs/TECHNICAL.tr.md §9](docs/TECHNICAL.tr.md#9-shell-entegrasyonu).

### Dokümanlar

- [docs/INSTALL.md](docs/INSTALL.md) — tüm kurulum kanalları, ayrıntılı.
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) — her config anahtarı.
- [docs/SECURITY.md](docs/SECURITY.md) — tam güvenlik modeli.
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — yaygın sorunlar.
- [docs/TECHNICAL.md](docs/TECHNICAL.md) — technical documentation (EN).
- [docs/TECHNICAL.tr.md](docs/TECHNICAL.tr.md) — teknik dokümantasyon (TR).
- [docs/PACKAGING.md](docs/PACKAGING.md) — bakımcıya yönelik paket kanalı etkinleştirme.
- [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md) — dürüst bilinen sorunlar listesi.
- [docs/history/](docs/history/) — geliştirme geçmişi kayıtları (uygulama planı, faz günlükleri, ilerleme notları).

### Derleme & geliştirme

```sh
make build             # -> ./comrade
make test              # go test ./...
make lint              # golangci-lint (pinlenmiş sürümü otomatik kurar)
make vet               # go vet ./...
make cross             # -> dist/comrade-<os>-<arch>[.exe], tüm platformlar
make release-check     # .goreleaser.yaml'ı doğrula, derleme yapmadan
make release-snapshot  # her release artifact'inin tam yerel deneme derlemesi
```

### Lisans

MIT — bkz. [LICENSE](LICENSE).
