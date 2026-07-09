# cli-comrade

> ⚠️ **Feature-complete release candidate, pre-release.** All planned phases
> (FAZ 0–11) are implemented and `v0.1.0-rc1` is ready — cold start ~5ms, full
> test/lint/vet/`-race`/`govulncheck` gate green — but **no tag has been
> published yet**, so no binaries exist on GitHub Releases and the
> Homebrew/winget/Scoop channels are not live. Build from source for now (see
> [Install](#install)). Commands and behavior may still change before the
> tag goes out. See [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md) for the
> honest list of what's unverified — mainly Windows runtime behavior not yet
> verified on a real Windows machine.
>
> ⚠️ **Geliştirme aşamasında sürüm adayı, henüz yayınlanmadı.** Tüm fazlar
> (FAZ 0–11) tamamlandı ve `v0.1.0-rc1` hazır, ama henüz bir tag
> yayınlanmadı — indirilebilir bir binary yok. Şimdilik kaynaktan derleyin
> (aşağıdaki [Kurulum](#kurulum) bölümüne bakın). Bilinen kısıtlar için
> [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md)'e bakın.

Binary name: `comrade`.

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
config `general.language` → `COMRADE_LANG` → `LANG`/`LC_ALL` → English
fallback (`internal/i18n/lang.go`).

### Core scenarios

| Command | What it does |
|---|---|
| `comrade fix` | Diagnoses the last failed command (captured via shell hook, or pass one explicitly) — its exit code and stderr — and proposes a fix. |
| `comrade "install docker"` (or `comrade do "..."`) | Turns a free-text request into a multi-step plan and runs it per the active mode. |
| `comrade explain "git rebase -i HEAD~5"` | Explains a command flag-by-flag without running it. |
| `comrade chat` | Interactive, context-preserving chat session. |

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
- **Keychain-backed secrets** (`internal/secrets`) — API keys go through
  the OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret
  Service) with an explicit, opt-in file fallback; never plaintext in config.

Full model: [docs/SECURITY.md](docs/SECURITY.md).

### Install

Nothing is published yet — build from source:

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
make build          # -> ./comrade
```

`go install github.com/firatkutay/cli-comrade/cmd/comrade@<version>` is
**not supported** — `go.mod` has a local-path `replace` directive for a
cold-start fix, and Go's own toolchain rejects `@version` installs against a
module whose `go.mod` contains `replace`/`exclude`. `git clone` + `make
build` sidesteps this because the checkout itself becomes the main module.
Details: [docs/INSTALL.md](docs/INSTALL.md).

Once a release is tagged, Homebrew (cask), winget, Scoop, `.deb`/`.rpm`, and
signed/checksum-verified install scripts will all build from the same
goreleaser pipeline (`.goreleaser.yaml`) — see docs/INSTALL.md for the exact
commands each channel will use.

### Quick start

```sh
comrade auth login anthropic  # store an API key for a provider (keychain, or file fallback)
comrade init                # install the shell hook (bash/zsh/fish/PowerShell)
comrade "install docker"    # or: comrade do "install docker"
comrade fix                 # after a failed command
comrade explain "git rebase -i HEAD~5"
comrade chat
```

### Docs

- [docs/INSTALL.md](docs/INSTALL.md) — every install channel, in detail.
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) — every config key.
- [docs/SECURITY.md](docs/SECURITY.md) — the full safety/security model.
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — common issues.
- [docs/TECHNICAL.md](docs/TECHNICAL.md) — technical documentation (EN).
- [docs/TECHNICAL.tr.md](docs/TECHNICAL.tr.md) — teknik dokümantasyon (TR).
- [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md) — honest known-issues list for `v0.1.0-rc1`.

### Build & develop

```sh
make build             # -> ./comrade
make test              # go test ./...
make lint              # golangci-lint (auto-installs the pinned version)
make vet                # go vet ./...
make cross              # -> dist/comrade-<os>-<arch>[.exe], all platforms
make release-check      # validate .goreleaser.yaml, no build
make release-snapshot   # full local dry-run of every release artifact
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
`general.language` → `COMRADE_LANG` → `LANG`/`LC_ALL` → İngilizce
varsayılanı sırasıyla belirlenir (`internal/i18n/lang.go`).

### Temel senaryolar

| Komut | Ne yapar |
|---|---|
| `comrade fix` | Son başarısız komutu (shell kancasıyla yakalanır, ya da elle verilir) — exit code'unu ve stderr'ini — teşhis eder ve bir çözüm önerir. |
| `comrade "docker kur"` (ya da `comrade do "..."`) | Serbest metin isteği çok adımlı bir plana çevirir ve etkin moda göre çalıştırır. |
| `comrade explain "git rebase -i HEAD~5"` | Bir komutu çalıştırmadan, bayrak bayrak açıklar. |
| `comrade chat` | Bağlamı koruyan interaktif sohbet oturumu. |

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
- **Keychain destekli sırlar** (`internal/secrets`) — API anahtarları
  işletim sistemi keychain'inden (macOS Keychain, Windows Credential
  Manager, Linux Secret Service) geçer; açık, opt-in bir dosya fallback'i
  vardır, config dosyasına asla düz metin yazılmaz.

Tam model: [docs/SECURITY.md](docs/SECURITY.md).

### Kurulum

Henüz hiçbir şey yayınlanmadı — kaynaktan derleyin:

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
make build          # -> ./comrade
```

`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>` biçimi
**desteklenmez** — `go.mod`'da soğuk-başlangıç düzeltmesi için yerel bir
`replace` direktifi var ve Go'nun kendi araç zinciri, `go.mod`'unda
`replace`/`exclude` bulunan bir modüle karşı `@sürüm` kurulumunu reddediyor.
`git clone` + `make build` bunu aşar, çünkü checkout'un kendisi ana modül
haline gelir. Detaylar: [docs/INSTALL.md](docs/INSTALL.md).

Bir sürüm etiketlendiğinde, Homebrew (cask), winget, Scoop, `.deb`/`.rpm` ve
imzalı/checksum doğrulamalı kurulum script'lerinin hepsi aynı goreleaser
pipeline'ından (`.goreleaser.yaml`) üretilecek — her kanalın kullanacağı tam
komutlar için docs/INSTALL.md'ye bakın.

### Hızlı başlangıç

```sh
comrade auth login anthropic  # bir sağlayıcı için API anahtarı sakla (keychain, ya da dosya fallback'i)
comrade init                # shell kancasını kur (bash/zsh/fish/PowerShell)
comrade "docker kur"        # ya da: comrade do "docker kur"
comrade fix                 # başarısız bir komuttan sonra
comrade explain "git rebase -i HEAD~5"
comrade chat
```

### Dokümanlar

- [docs/INSTALL.md](docs/INSTALL.md) — tüm kurulum kanalları, ayrıntılı.
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) — her config anahtarı.
- [docs/SECURITY.md](docs/SECURITY.md) — tam güvenlik modeli.
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — yaygın sorunlar.
- [docs/TECHNICAL.md](docs/TECHNICAL.md) — technical documentation (EN).
- [docs/TECHNICAL.tr.md](docs/TECHNICAL.tr.md) — teknik dokümantasyon (TR).
- [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md) — `v0.1.0-rc1` için dürüst bilinen sorunlar listesi.

### Derleme & geliştirme

```sh
make build             # -> ./comrade
make test              # go test ./...
make lint              # golangci-lint (pinlenmiş sürümü otomatik kurar)
make vet                # go vet ./...
make cross              # -> dist/comrade-<os>-<arch>[.exe], tüm platformlar
make release-check      # .goreleaser.yaml'ı doğrula, derleme yapmadan
make release-snapshot   # her release artifact'inin tam yerel deneme derlemesi
```

### Lisans

MIT — bkz. [LICENSE](LICENSE).
