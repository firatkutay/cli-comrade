# Kurulum / Installation

Binary name: `comrade`

---

## Türkçe

Tüm kurulum yöntemleri her release'de aynı imzalanmış/checksum'lı
arşivlerden ve paketlerden üretilir (bkz. `.goreleaser.yaml`). Hiçbiri
`sudo curl | bash` gibi bir "kör" script çalıştırmaz; kurulum
script'lerinin kendisi bile indirdiği arşivi `checksums.txt`'e karşı
doğrular (aşağıya bakın).

### macOS / Linux — Homebrew

```sh
brew tap firatkutay/tap
brew install --cask comrade
```

`comrade` artık bir Homebrew **Cask**'tir (eski "Formula" biçimi
goreleaser v2.16'dan itibaren kullanımdan kaldırılmıştır); kurulum ve
güncelleme komutları kullanıcı açısından aynıdır.

### Windows — winget

```powershell
winget install FiratKutay.comrade
```

### Windows — Scoop

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

### Debian/Ubuntu — .deb

```sh
curl -fsSL -o comrade.deb \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade_<VERSION>_amd64.deb
sudo dpkg -i comrade.deb
```

`<VERSION>` yerine indirmek istediğiniz sürümü ("~" olmadan, örn.
`0.2.0`) yazın; [Releases](https://github.com/firatkutay/cli-comrade/releases)
sayfasından tam dosya adını kopyalayabilirsiniz.

### Fedora/RHEL — .rpm

```sh
curl -fsSL -o comrade.rpm \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade-<VERSION>-1.x86_64.rpm
sudo rpm -i comrade.rpm
```

### Kurulum script'i (macOS / Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

Bu script:

1. `curl` veya `wget`'ten hangisi varsa onu kullanır (ikisi de yoksa
   anlaşılır bir hatayla durur);
2. en son (veya `COMRADE_VERSION` ile sabitlenmiş) release'i indirir;
3. indirilen arşivi aynı release'in `checksums.txt` dosyasına karşı
   `sha256sum -c` ile doğrular — doğrulama başarısız olursa kurulum
   iptal edilir;
4. `$HOME/.local/bin`'e (yazılamıyorsa `/usr/local/bin`'e, o da
   yazılamıyorsa `sudo` ile) kurar;
5. `comrade init` çalıştırmanızı önerir.

Ortam değişkenleri: `COMRADE_VERSION` (belirli bir sürümü sabitler),
`COMRADE_INSTALL_DIR` (kurulum dizinini değiştirir).

### Kurulum script'i (Windows PowerShell)

```powershell
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

Aynı doğrulama adımlarını (`Get-FileHash` ile checksum kontrolü) yapar,
`%LOCALAPPDATA%\Programs\cli-comrade`'e kurar ve kullanıcı `PATH`'ine
ekler.

### `go install` (Go geliştiricileri için)

```sh
go install github.com/firatkutay/cli-comrade/cmd/comrade@latest
```

Bu yöntem checksum doğrulaması yapmaz (Go module proxy'sinin kendi
bütünlük doğrulamasına — `go.sum`/GONOSUMCHECK — güvenir); üretim
ortamlarında yukarıdaki paket yöneticilerinden birini tercih edin.

### Kurulumdan sonra

Her yöntemde kurulumdan sonra shell entegrasyonunu kurun:

```sh
comrade init
```

Bu, kabuğunuzu (bash/zsh/fish/PowerShell) otomatik tespit eder ve son
komut/exit code/hata çıktısını yakalayan kancayı ekler — `comrade fix`
bunsuz da çalışır (yapıştırma moduna düşer) ama kancayla çok daha
kullanışlıdır.

### Güncelleme

```sh
comrade upgrade --check   # sadece daha yeni bir sürüm var mı bildirir
comrade upgrade           # indirir, checksum doğrular, kendini günceller
```

`comrade`, en fazla haftada bir kez, herhangi bir komutun sonunda daha
yeni bir sürüm olduğunu sessizce bildirir (`general.update_check =
false` ile kapatılabilir — bkz. CONFIGURATION.md).

---

## English

Every install method is built from the exact same signed/checksummed
archives and packages on every release (see `.goreleaser.yaml`). None
of them is a blind `curl | sudo bash` — even the install scripts
themselves verify the downloaded archive against that release's own
`checksums.txt` before installing anything (see below).

### macOS / Linux — Homebrew

```sh
brew tap firatkutay/tap
brew install --cask comrade
```

`comrade` is published as a Homebrew **Cask** (the older "Formula"
shape was deprecated by goreleaser as of v2.16); install/upgrade
commands are the same either way from the user's side.

### Windows — winget

```powershell
winget install FiratKutay.comrade
```

### Windows — Scoop

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

### Debian/Ubuntu — .deb

```sh
curl -fsSL -o comrade.deb \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade_<VERSION>_amd64.deb
sudo dpkg -i comrade.deb
```

Replace `<VERSION>` with the release you want (no leading "v", e.g.
`0.2.0`) — copy the exact filename from the
[Releases page](https://github.com/firatkutay/cli-comrade/releases).

### Fedora/RHEL — .rpm

```sh
curl -fsSL -o comrade.rpm \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade-<VERSION>-1.x86_64.rpm
sudo rpm -i comrade.rpm
```

### Install script (macOS / Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

This script:

1. uses whichever of `curl`/`wget` is available (and fails with a clear
   message if neither is present);
2. downloads the latest (or `COMRADE_VERSION`-pinned) release;
3. verifies the downloaded archive against that same release's
   `checksums.txt` via `sha256sum -c` — installation is aborted if
   verification fails;
4. installs to `$HOME/.local/bin` (falling back to `/usr/local/bin`,
   then to `sudo` if neither is writable);
5. suggests running `comrade init`.

Env overrides: `COMRADE_VERSION` (pin an exact version),
`COMRADE_INSTALL_DIR` (override the install directory).

### Install script (Windows PowerShell)

```powershell
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

Performs the same verification (a `Get-FileHash` checksum check),
installs to `%LOCALAPPDATA%\Programs\cli-comrade`, and adds it to your
user `PATH`.

### `go install` (for Go developers)

```sh
go install github.com/firatkutay/cli-comrade/cmd/comrade@latest
```

This method does not checksum-verify (it relies on the Go module
proxy's own integrity checking — `go.sum`/GONOSUMCHECK); prefer one of
the package managers above for production use.

### After installing

Whichever method you used, set up shell integration next:

```sh
comrade init
```

This auto-detects your shell (bash/zsh/fish/PowerShell) and installs
the hook that captures the last command/exit code/error output —
`comrade fix` still works without it (it falls back to paste mode) but
is far more useful with it.

### Upgrading

```sh
comrade upgrade --check   # only report whether a newer version exists
comrade upgrade           # download, checksum-verify, and self-update
```

`comrade` also prints a single, silent, at-most-once-a-week notice at
the end of any command when a newer version is available (disable with
`general.update_check = false` — see CONFIGURATION.md).
