# Kurulum / Installation

Binary name: `comrade`

---

## Türkçe

v0.3.x için **birincil kurulum yolu** aşağıdaki `install.sh`/`install.ps1`
tek satırlık komutlarıdır. Tüm kurulum yöntemleri her release'de aynı
imzalanmış/checksum'lı arşivlerden ve paketlerden üretilir (bkz.
`.goreleaser.yaml`). Hiçbiri `sudo curl | bash` gibi bir "kör" script
çalıştırmaz; kurulum script'lerinin kendisi bile indirdiği arşivi
`checksums.txt`'e karşı doğrular (aşağıya bakın).

### Kurulum script'i (macOS / Linux) — önerilen yöntem

```sh
curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
```

`curl` yoksa, script `wget` ile de aynı şekilde çalışır:

```sh
wget -qO- https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
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
3. `checksums.txt`'in **kendisini**, indirilen `checksums.txt.sig`'e karşı
   bir cosign imzası olarak doğrular — `comrade upgrade`'in kullandığı
   birebir aynı gömülü genel anahtar ve mekanizmayla (bkz. aşağıdaki
   "Güven modeli"); yalnızca bu doğrulama geçtikten sonra dosyanın
   içeriğine güvenilir;
4. indirilen arşivi aynı `checksums.txt` satırına karşı `sha256sum -c`
   ile doğrular — doğrulama başarısız olursa kurulum iptal edilir;
5. `$HOME/.local/bin`'e (yazılamıyorsa `/usr/local/bin`'e, o da
   yazılamıyorsa `sudo` ile) kurar;
6. kurulum dizini PATH'inizde değilse, kabuğunuza uygun bir PATH export
   satırını rc dosyanıza (bash → `~/.bashrc`, zsh → `~/.zshrc`, fish →
   `~/.config/fish/config.fish`, diğerleri → `~/.profile`) **otomatik
   olarak** ve idempotent şekilde ekler (script'i tekrar çalıştırmak
   satırı ikinci kez eklemez), ardından kabuğunuzu yeniden başlatmanızı
   veya ekrana yazdırılan `export ...` komutunu doğrudan çalıştırmanızı
   söyler;
7. `comrade init <shell>` çalıştırmanızı önerir.

Ortam değişkenleri: `COMRADE_VERSION` (belirli bir sürümü, örn. `v0.1.4`,
sabitler — bu durumda script o tag'e özel `checksums.txt`'i kullanır),
`COMRADE_INSTALL_DIR` (kurulum dizinini değiştirir), `COMRADE_NO_MODIFY_PATH`
(herhangi bir değere ayarlanırsa, script rc dosyanızı OTOMATİK
DÜZENLEMEZ — bunun yerine eski davranışa döner: sadece PATH'e elle
eklemeniz gerektiğini bildiren bir not basar), `COMRADE_INSTALL_ALLOW_UNSIGNED`
(openssl yoksa veya bir release imza yayınlamamışsa checksum-only
doğrulamaya geri dönmeyi açıkça kabul eder — her kullanımda yüksek sesle
uyarır; bkz. [SECURITY.md](SECURITY.md)).

### Güven modeli

`checksums.txt`, arşivle **aynı kanaldan** indirilir — bu yüzden bir
SHA-256 checksum tek başına yalnızca arşivin manifestoyla eşleştiğini
kanıtlar, manifestoyu kimin yazdığını KANITLAMAZ. Bu yüzden script artık
`checksums.txt`'in kendisini, `internal/update/cosign.pub`'daki gerçek
anahtarla birebir aynı, script içine gömülü bir cosign genel anahtarına
karşı doğrular — `comrade upgrade`'in Go tarafında zaten yaptığı tam
olarak aynı ECDSA P-256/SHA-256 doğrulaması, burada saf `openssl` ile.
openssl yoksa veya bir release `checksums.txt.sig` yayınlamamışsa,
davranış varsayılan olarak **kapalı-hata**dır (kurulum durur) —
`COMRADE_INSTALL_ALLOW_UNSIGNED=1` ile açıkça atlatılabilir. Gerçek bir
imza UYUŞMAZLIĞI bu override'a asla tabi değildir, her zaman koşulsuz
durur. Ayrıntılar için bkz. [SECURITY.md](SECURITY.md).

**`install.ps1` (Windows) henüz aynı korumaya sahip değil** —
[GitHub issue #43](https://github.com/firatkutay/cli-comrade/issues/43)
olarak takip ediliyor (bkz. SECURITY.md).

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

### npm — beklemede

```sh
npm install -g cli-comrade
```

`cli-comrade` paketi Node.js/npm zaten kurulu ortamlar için önceden
derlenmiş `comrade` binary'sini indirir — herhangi bir Go/derleme araç
zinciri gerekmez; platforma özgü binary, `optionalDependencies` üzerinden
otomatik seçilir (linux/macOS amd64+arm64, Windows amd64). Bu nedenle
`npm ci --ignore-scripts` de sorunsuz çalışır (postinstall script'i
yoktur). Paket adı şu an doğrulanmış şekilde müsait (npm'de bir
rezervasyon mekanizması yoktur — bu sadece "henüz kimse almamış" demektir,
bir garanti değil) ancak ilk yayın henüz yapılmadı; onaylanana kadar
yukarıdaki diğer kurulum yöntemlerini kullanın.

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

**`go install github.com/firatkutay/cli-comrade/cmd/comrade@<sürüm>` biçimi
(modülü doğrudan bir proxy'den, bir ana-modül bağlamı OLMADAN kuran `@sürüm`
biçimi) bu sürümde DESTEKLENMEZ.** Sebep keyfi değil, Go araç zincirinin
kendi, belgelenmiş kısıtlaması: `go.mod`'umuzda bir soğuk-başlangıç
performans düzeltmesi için yerel-dosya-yolu bir `replace` direktifi var
(`replace github.com/atotto/clipboard => ./third_party/atotto-clipboard` —
bkz. `docs/history/phases/FAZ-11.md`), ve Go'nun kendi kuralı gereği
"`@sürüm` argümanlarını içeren komut satırındaki paketleri barındıran
modülün `go.mod` dosyası, ana modül olsaydı farklı yorumlanmasına neden
olacak direktifler (`replace` ve `exclude`) içermemelidir" (go.dev/ref/mod).
Bunu ihlal ederek denendiğinde Go **sessizce yok saymaz, sert bir hatayla
reddeder** (bu davranış doğrudan doğrulandı: `go install .../cmd/foo@v0.0.1`
verilen bir go.mod'da yerel bir `replace` varken, Go tam olarak şu hatayı
basıyor: *"The go.mod file for the module providing named packages contains
one or more replace directives. It must not contain directives that would
cause it to be interpreted differently than if it were the main module."*).
Yukarıdaki `git clone` + `go build`/`go install ./cmd/comrade` yöntemi bunun
yerine çalışır, çünkü checkout'un kendisi o an ana modül olur ve `replace`
direktifi normal şekilde uygulanır — soğuk başlangıç düzeltmesini de doğru
şekilde alırsınız (goreleaser'ın kendi derleme adımı da aynı sebeple bu
yöntemi kullanır ve etkilenmez). Bu yöntem checksum doğrulaması yapmaz;
üretim ortamlarında yukarıdaki paket yöneticilerinden birini tercih edin.

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
yeni bir sürüm olduğunu sessizce bildirir (`general.update_check = false`
ile kapatılabilir — bkz. CONFIGURATION.md).

`comrade upgrade`, indirdiği sürümü kurmadan önce artık bir cosign
imzasını (offline, ağ erişimi olmadan) doğrular; imza doğrulanamazsa
kurulum iptal edilir. Ayrıntılar için bkz.
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md) ve
[`docs/SECURITY.md`](SECURITY.md).

---

## English

The **primary install path for v0.3.x** is the `install.sh`/`install.ps1`
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

No `curl`? The script works identically with `wget`:

```sh
wget -qO- https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
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
3. authenticates `checksums.txt` **itself** against the downloaded
   `checksums.txt.sig`, as a cosign signature — the exact same embedded
   public key and mechanism `comrade upgrade` uses (see "Trust model"
   below) — before any of that file's content is trusted;
4. verifies the downloaded archive against that same `checksums.txt`
   line via `sha256sum -c` — installation is aborted if verification
   fails;
5. installs to `$HOME/.local/bin` (falling back to `/usr/local/bin`,
   then to `sudo` if neither is writable);
6. if the install directory isn't already on your PATH, **automatically**
   appends a shell-appropriate PATH export line to your rc file (bash →
   `~/.bashrc`, zsh → `~/.zshrc`, fish → `~/.config/fish/config.fish`,
   anything else → `~/.profile`), idempotently (re-running the script
   never appends it twice), then tells you to restart your shell or run
   the printed `export ...` command directly;
7. suggests running `comrade init <shell>`.

Env overrides: `COMRADE_VERSION` (pin an exact version, e.g. `v0.1.4` —
this switches the script to that tag's own `checksums.txt` instead of
`latest`), `COMRADE_INSTALL_DIR` (override the install directory),
`COMRADE_NO_MODIFY_PATH` (set to any non-empty value to stop the script
from auto-editing your rc file — it falls back to the old behavior of
just printing a note that you need to add the install directory to
PATH yourself), `COMRADE_INSTALL_ALLOW_UNSIGNED` (explicitly accepts
falling back to checksum-only verification when openssl is missing or a
release published no signature — prints a loud warning every time; see
[SECURITY.md](SECURITY.md)).

### Trust model

`checksums.txt` is downloaded over the **same channel** as the archive —
so a bare SHA-256 checksum only proves the archive matches the manifest,
it never proves who WROTE the manifest. The script therefore now
authenticates `checksums.txt` itself against a cosign public key embedded
in the script, byte-identical to `internal/update/cosign.pub` — the exact
same ECDSA-P256/SHA-256 verification `comrade upgrade` already does in
Go, done here with plain `openssl`. If openssl is missing, or a release
published no `checksums.txt.sig`, the default is **fail-closed** (the
install aborts) — override explicitly with
`COMRADE_INSTALL_ALLOW_UNSIGNED=1`. An actual signature MISMATCH is never
subject to that override; it always aborts unconditionally. See
[SECURITY.md](SECURITY.md) for the full writeup.

**`install.ps1` (Windows) does not have this protection yet** — tracked as
[GitHub issue #43](https://github.com/firatkutay/cli-comrade/issues/43)
(see SECURITY.md).

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

### npm — pending

```sh
npm install -g cli-comrade
```

The `cli-comrade` package installs a prebuilt `comrade` binary for
environments that already have Node.js/npm — no Go/build toolchain
required. The right platform-specific binary (linux/macOS amd64+arm64,
Windows amd64) is selected automatically via `optionalDependencies`, so
`npm ci --ignore-scripts` works fine too (there is no postinstall
script). The package name is currently verified available (npm has no
reservation mechanism -- this only means "nobody has claimed it yet," not
a guarantee) but not yet published; use one of the other install methods
above until it is.

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
context) is NOT supported at this release.** The reason isn't arbitrary —
it's the Go toolchain's own, documented constraint: our `go.mod` carries a
local-filesystem `replace` directive for a cold-start performance fix
(`replace github.com/atotto/clipboard => ./third_party/atotto-clipboard` —
see `docs/history/phases/FAZ-11.md`), and per Go's own rule, "if the
module containing packages named on the command line has a go.mod file, it
must not contain directives (`replace` and `exclude`) that would cause it
to be interpreted differently if it were the main module"
(go.dev/ref/mod). Attempting it does **not** silently drop the replace —
Go hard-errors (verified directly: running `go install .../cmd/foo@v0.0.1`
against a go.mod with a local replace produces exactly: *"The go.mod file
for the module providing named packages contains one or more replace
directives. It must not contain directives that would cause it to be
interpreted differently than if it were the main module."*). The
`git clone` + `go build`/`go install ./cmd/comrade` method above works
instead, because the checkout itself becomes the main module and the
`replace` directive is honored normally — you get the cold-start fix
correctly too (goreleaser's own build step uses this same method for the
same reason, and is unaffected). This method does not checksum-verify;
prefer one of the package managers above for production use.

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

`comrade upgrade` now verifies a cosign signature (offline, no network
lookup) on the downloaded release before installing it — the upgrade
aborts if the signature doesn't check out. See
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md) and
[`docs/SECURITY.md`](SECURITY.md) for details.
