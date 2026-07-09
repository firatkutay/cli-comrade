# cli-comrade

> ⚠️ **Geliştirme aşamasında / Under active development.** cli-comrade henüz
> kullanıma hazır değil; API, komutlar ve davranışlar önceden haber
> verilmeden değişebilir. / cli-comrade is not yet ready for use; its API,
> commands, and behavior may change without notice.

Binary name: `comrade`

---

## Türkçe

### Vizyon

**cli-comrade**, terminal bilgisi olmayan veya terminalle uğraşmak istemeyen
kullanıcılara komut satırında yoldaşlık eden, cross-platform (Windows / macOS
/ Linux) bir yapay zeka CLI asistanıdır. Kullanıcı doğal dille (Türkçe veya
İngilizce) istekte bulunur; araç hatayı analiz eder, gerekli komutu üretir ve
ayarlanan davranış moduna göre çalıştırır, onay ister ya da sadece bilgi
verir.

Temel senaryolar:

- **Hata çözme:** `comrade fix` — son çalıştırılan komutu, exit code'unu ve
  hata çıktısını analiz eder, kök nedeni ve çözümü sunar.
- **Görev yaptırma:** `comrade docker kur` gibi doğal dil istekleri çok
  adımlı bir plana dönüştürülür ve moda göre yürütülür.
- **Açıklama:** `comrade explain "git rebase -i HEAD~5"` — komutu
  çalıştırmadan sade dille açıklar.
- **Sohbet:** `comrade chat` — bağlamı koruyan interaktif oturum.

### Davranış Modları

| Mod | Davranış |
|---|---|
| `auto` | Aracı devralır, komutları kendisi çalıştırır. Her adımda tek satırlık ne yaptığını yazar. |
| `ask` | Her komuttan önce kısa gerekçe + komutun kendisini gösterir, `[e]vet / [h]ayır / [d]üzenle / [a]çıkla / [t]ümünü onayla` sorar. **Varsayılan mod budur.** |
| `info` | Hiçbir şey çalıştırmaz. Sorunun nedenini ve çözüm adımlarını kopyalanabilir komutlarla açıklar. |

**Güvenlik istisnası (pazarlık edilemez):** `auto` modda bile risk sınıfı
`destructive` olan komutlar HER ZAMAN onay ister.

### Kurulum

```sh
# macOS/Linux — Homebrew
brew tap firatkutay/tap && brew install --cask comrade

# Windows — winget
winget install FiratKutay.comrade

# Windows — Scoop
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket && scoop install comrade

# macOS/Linux — kurulum script'i (checksum doğrulamalı)
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

Debian/Ubuntu (.deb), Fedora/RHEL (.rpm) ve kaynaktan derleme dahil tüm
seçenekler için: [docs/INSTALL.md](docs/INSTALL.md) (not: bir `go.mod`
`replace` direktifi nedeniyle `go install ...@sürüm` biçimi
desteklenmez — bkz. o dosyadaki "Kaynaktan derleme" bölümü).
Yapılandırma anahtarları için
[docs/CONFIGURATION.md](docs/CONFIGURATION.md),
güvenlik modeli için [docs/SECURITY.md](docs/SECURITY.md), yaygın
sorunlar için [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md).

### Durum

Proje şu anda **FAZ 0 — Proje İskeleti** aşamasındadır. Henüz gerçek bir
işlevsellik yoktur; komutlar iskelet/stub durumundadır.

---

## English

### Vision

**cli-comrade** is a cross-platform (Windows / macOS / Linux) AI CLI
companion for users who don't know — or don't want to deal with — the
terminal. The user makes a request in natural language (Turkish or English);
the tool analyzes any error, generates the necessary command, and either
executes it, asks for confirmation, or just explains it, depending on the
configured behavior mode.

Core scenarios:

- **Fixing errors:** `comrade fix` — analyzes the last run command, its exit
  code, and its error output, then presents the root cause and a fix.
- **Running tasks:** natural-language requests like `comrade install docker`
  are turned into a multi-step plan and executed according to the active mode.
- **Explaining:** `comrade explain "git rebase -i HEAD~5"` — explains a
  command in plain language without running it.
- **Chatting:** `comrade chat` — an interactive, context-preserving session.

### Behavior Modes

| Mode | Behavior |
|---|---|
| `auto` | The agent takes over and runs commands itself, printing a one-line status per step. |
| `ask` | Before every command, shows a short rationale plus the command itself, then asks `[y]es / [n]o / [e]dit / [x]plain / [a]ll`. **This is the default mode.** |
| `info` | Runs nothing. Explains the cause of the problem and the fix steps as copy-pasteable commands. |

**Non-negotiable safety exception:** even in `auto` mode, commands classified
as `destructive` ALWAYS require confirmation.

### Install

```sh
# macOS/Linux — Homebrew
brew tap firatkutay/tap && brew install --cask comrade

# Windows — winget
winget install FiratKutay.comrade

# Windows — Scoop
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket && scoop install comrade

# macOS/Linux — install script (checksum-verified)
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

See [docs/INSTALL.md](docs/INSTALL.md) for every option, including
Debian/Ubuntu (.deb), Fedora/RHEL (.rpm), and building from source
(note: `go install ...@version` isn't supported, due to a `go.mod`
`replace` directive — see that file's "Build from source" section).
Also see [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for every config key,
[docs/SECURITY.md](docs/SECURITY.md) for the security model, and
[docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) for common issues.

### Status

The project is currently at **FAZ 0 — Project Skeleton**. There is no real
functionality yet; commands are skeleton/stub implementations.

### Build

```sh
make build   # -> ./comrade
make test
make lint
make vet
make cross   # -> dist/comrade-<os>-<arch>[.exe]
```
