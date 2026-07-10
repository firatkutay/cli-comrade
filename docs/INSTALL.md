# Kurulum / Installation

Binary name: `comrade`

---

## Türkçe

v0.1.x için **birincil kurulum yolu** aşağıdaki `install.sh`/`install.ps1`
tek satırlık komutlarıdır. Tüm kurulum yöntemleri her release'de aynı
imzalanmış/checksum'lı arşivlerden ve paketlerden üretilir (bkz.
`.goreleaser.yaml`). Hiçbiri `sudo curl | bash` gibi bir "kör" script
çalıştırmaz; kurulum script'lerinin kendisi bile indirdiği arşivi
`checksums.txt`'e karşı doğrular (aşağıya bakın).

### Kurulum script'i (macOS / Linux) — önerilen yöntem

```sh
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

Bu script:

1. `curl` veya `wget`'ten hangisi varsa onu kullanır (ikisi de yoksa
   anlaşılır bir hatayla durur);
2. `COMRADE_VERSION` verilmemişse, sürümü `api.github.com`'daki
   rate-limit'li (kimliksiz istekte saatte 60) "latest release" REST
   uç noktasını **hiç çağırmadan** çözer: doğrudan GitHub'ın
   `releases/latest/download/checksums.txt` yönlendirmesini indirir, o
   dosyadan işletim sistemi/mimarinize uyan satırı bulur ve gerçek
   arşiv dosya adını (sürüm numarası dahil) oradan okur;
3. indirilen arşivi aynı `checksums.txt` satırına karşı `sha256sum -c`
   ile doğrular — doğrulama başarısız olursa kurulum iptal edilir;
4. `$HOME/.local/bin`'e (yazılamıyorsa `/usr/local/bin`'e, o da
   yazılamıyorsa `sudo` ile) kurar;
5. `comrade init <shell>` çalıştırmanızı önerir.

Ortam değişkenleri: `COMRADE_VERSION` (belirli bir sürümü, örn. `v0.1.4`,
sabitler — bu durumda script o tag'e özel `checksums.txt`'i kullanır),
`COMRADE_INSTALL_DIR` (kurulum dizinini değiştirir).

### Kurulum script'i (Windows PowerShell) — önerilen yöntem

```powershell
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

Aynı mantığı izler: sürümü `api.github.com` yerine
`releases/latest/download/checksums.txt`'ten çözer, `Get-FileHash` ile
checksum doğrular, `%LOCALAPPDATA%\Programs\cli-comrade`'e kurar ve
kullanıcı `PATH`'ine ekler. Belirli bir sürümü sabitlemek için
`$env:COMRADE_VERSION` (veya `-Version` parametresi) kullanın. Windows
PowerShell 5.1 ve PowerShell 7 (`pwsh`) ile test edilmiştir.

### Debian/Ubuntu — .deb

```sh
curl -fsSL -o comrade.deb \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade_<VERSION>_amd64.deb
sudo dpkg -i comrade.deb
```

`<VERSION>` yerine indirmek istediğiniz sürümü ("v" olmadan, örn.
`0.1.0`) yazın; [Releases](https://github.com/firatkutay/cli-comrade/releases)
sayfasından tam dosya adını kopyalayabilirsiniz.

### Fedora/RHEL — .rpm

```sh
curl -fsSL -o comrade.rpm \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade-<VERSION>-1.x86_64.rpm
sudo rpm -i comrade.rpm
```

### Homebrew — canlı

```sh
brew tap firatkutay/tap
brew install comrade
```

### Scoop (Windows) — canlı

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

### winget (Windows) — beklemede

```powershell
winget install cli.comrade
```

Paket, `microsoft/winget-pkgs`'e `cli.comrade` kimliğiyle gönderildi ve
moderatör incelemesi bekliyor; onaylanana kadar yukarıdaki komut
çalışmaz. Bu arada Scoop veya `install.ps1` script'ini kullanın.

### Snap (Linux) — beklemede

```sh
sudo snap install cli-comrade --classic
```

Snap paketi hazır (`snap/snapcraft.yaml` + `.github/workflows/snap.yml`,
`classic` confinement ile) ancak Snap Store kaydı ve classic confinement
onayı bekleniyor; onaylanana kadar yukarıdaki komut çalışmaz. Bu arada
`install.sh` script'ini veya `.deb`/`.rpm` paketlerini kullanın.

### Kaynaktan derleme (Go geliştiricileri için)

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
go build -o comrade ./cmd/comrade   # ya da: go install ./cmd/comrade
```

**`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>`
biçimi (modülü doğrudan bir proxy'den, bir ana-modül bağlamı OLMADAN
kuran `@sürüm` biçimi) bu sürümde DESTEKLENMEZ.** Sebep keyfi değil, Go
araç zincirinin kendi, belgelenmiş kısıtlaması: `go.mod`'umuzda bir
soğuk-başlangıç performans düzeltmesi için yerel-dosya-yolu bir
`replace` direktifi var (`replace github.com/atotto/clipboard =>
./third_party/atotto-clipboard` — bkz. `docs/history/phases/FAZ-11.md`), ve
Go'nun kendi kuralı gereği "`@sürüm` argümanlarını içeren komut
satırındaki paketleri barındıran modülün `go.mod` dosyası, ana modül
olsaydı farklı yorumlanmasına neden olacak direktifler (`replace` ve
`exclude`) içermemelidir" (go.dev/ref/mod). Bunu ihlal ederek
denendiğinde Go **sessizce yok saymaz, sert bir hatayla reddeder** (bu
davranış doğrudan doğrulandı: `go install .../cmd/foo@v0.0.1` verilen
bir go.mod'da yerel bir `replace` varken, Go tam olarak şu hatayı
basıyor: *"The go.mod file for the module providing named packages
contains one or more replace directives. It must not contain
directives that would cause it to be interpreted differently than if
it were the main module."*). Yukarıdaki `git clone` + `go build`/`go
install ./cmd/comrade` yöntemi bunun yerine çalışır, çünkü checkout'un
kendisi o an ana modül olur ve `replace` direktifi normal şekilde
uygulanır — soğuk başlangıç düzeltmesini de doğru şekilde alırsınız
(goreleaser'ın kendi derleme adımı da aynı sebeple bu yöntemi kullanır
ve etkilenmez). Bu yöntem checksum doğrulaması yapmaz; üretim
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

The **primary install path for v0.1.x** is the `install.sh`/`install.ps1`
one-liners below. Every install method is built from the exact same
signed/checksummed archives and packages on every release (see
`.goreleaser.yaml`). None of them is a blind `curl | sudo bash` — even
the install scripts themselves verify the downloaded archive against
that release's own `checksums.txt` before installing anything (see
below).

### Install script (macOS / Linux) — recommended

```sh
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

This script:

1. uses whichever of `curl`/`wget` is available (and fails with a clear
   message if neither is present);
2. resolves the version to install **without ever calling** the
   rate-limited (60 req/hr unauthenticated) `api.github.com` "latest
   release" REST endpoint, unless `COMRADE_VERSION` is set: it fetches
   GitHub's `releases/latest/download/checksums.txt` redirect directly,
   finds the line matching your OS/arch, and reads the real archive
   filename (version number included) out of that;
3. verifies the downloaded archive against that same `checksums.txt`
   line via `sha256sum -c` — installation is aborted if verification
   fails;
4. installs to `$HOME/.local/bin` (falling back to `/usr/local/bin`,
   then to `sudo` if neither is writable);
5. suggests running `comrade init <shell>`.

Env overrides: `COMRADE_VERSION` (pin an exact version, e.g. `v0.1.4` —
this switches the script to that tag's own `checksums.txt` instead of
`latest`), `COMRADE_INSTALL_DIR` (override the install directory).

### Install script (Windows PowerShell) — recommended

```powershell
irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
```

Same approach: resolves the version from
`releases/latest/download/checksums.txt` instead of `api.github.com`,
verifies with `Get-FileHash`, installs to
`%LOCALAPPDATA%\Programs\cli-comrade`, and adds it to your user `PATH`.
Pin a version with `$env:COMRADE_VERSION` (or the `-Version` parameter).
Tested on both Windows PowerShell 5.1 and PowerShell 7 (`pwsh`).

### Debian/Ubuntu — .deb

```sh
curl -fsSL -o comrade.deb \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade_<VERSION>_amd64.deb
sudo dpkg -i comrade.deb
```

Replace `<VERSION>` with the release you want (no leading "v", e.g.
`0.1.0`) — copy the exact filename from the
[Releases page](https://github.com/firatkutay/cli-comrade/releases).

### Fedora/RHEL — .rpm

```sh
curl -fsSL -o comrade.rpm \
  https://github.com/firatkutay/cli-comrade/releases/latest/download/comrade-<VERSION>-1.x86_64.rpm
sudo rpm -i comrade.rpm
```

### Homebrew — live

```sh
brew tap firatkutay/tap
brew install comrade
```

### Scoop (Windows) — live

```powershell
scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket
scoop install comrade
```

### winget (Windows) — pending

```powershell
winget install cli.comrade
```

The package was submitted to `microsoft/winget-pkgs` under the id
`cli.comrade` and is awaiting moderator review; the command above won't
work until it's merged. Use Scoop or the `install.ps1` script in the
meantime.

### Snap (Linux) — pending

```sh
sudo snap install cli-comrade --classic
```

The snap package is prepared (`snap/snapcraft.yaml` +
`.github/workflows/snap.yml`, `classic` confinement) but is awaiting
Snap Store registration and classic-confinement approval; the command
above won't work until that clears. Use the `install.sh` script or the
`.deb`/`.rpm` packages in the meantime.

### Build from source (for Go developers)

```sh
git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade
go build -o comrade ./cmd/comrade   # or: go install ./cmd/comrade
```

**The `go install github.com/firatkutay/cli-comrade/cmd/comrade@<version>`
form (installing the module directly from a proxy, with no main-module
context) is NOT supported at this release.** The reason isn't
arbitrary — it's the Go toolchain's own, documented constraint: our
`go.mod` carries a local-filesystem `replace` directive for a
cold-start performance fix (`replace github.com/atotto/clipboard =>
./third_party/atotto-clipboard` — see `docs/history/phases/FAZ-11.md`), and per
Go's own rule, "if the module containing packages named on the command
line has a go.mod file, it must not contain directives (`replace` and
`exclude`) that would cause it to be interpreted differently if it
were the main module" (go.dev/ref/mod). Attempting it does **not**
silently drop the replace — Go hard-errors (verified directly: running
`go install .../cmd/foo@v0.0.1` against a go.mod with a local replace
produces exactly: *"The go.mod file for the module providing named
packages contains one or more replace directives. It must not contain
directives that would cause it to be interpreted differently than if
it were the main module."*). The `git clone` + `go build`/`go install
./cmd/comrade` method above works instead, because the checkout itself
becomes the main module and the `replace` directive is honored
normally — you get the cold-start fix correctly too (goreleaser's own
build step uses this same method for the same reason, and is
unaffected). This method does not checksum-verify; prefer one of the
package managers above for production use.

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
