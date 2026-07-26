# Kurulum / Installation

Binary name: `comrade`

---

## Türkçe

**Birincil kurulum yolu** aşağıdaki `install.sh`/`install.ps1`
tek satırlık komutlarıdır (Homebrew, Scoop ve doğrudan `.deb`/`.rpm`
paketleri de eşit derecede canlı ve desteklenen kanallardır — npm ise
zaten Node.js kurulu olan geliştiriciler için bir **alternatiftir**, bkz.
aşağıdaki "npm" bölümü). Tüm kurulum yöntemleri her release'de aynı
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

**`install.ps1` (Windows) artık aynı korumaya sahip** (GitHub issue #43):
`checksums.txt`'i, `internal/update/cosign.pub`'daki gerçek anahtarla
birebir aynı, script içine gömülü bir cosign genel anahtarına karşı
doğrular — aynı kapalı-hata varsayılanı ve `COMRADE_INSTALL_ALLOW_UNSIGNED`
override'ı ile. Windows PowerShell 5.1 ile PowerShell 7 arasındaki ECDSA
API farklılıkları nedeniyle `install.sh`'ın openssl yaklaşımının birebir
kopyası değildir — bkz. SECURITY.md'nin "curl | sh bootstrap yolunun güven
modeli" bölümü.

### Yeniden üretilebilir (reproducible) derlemeler

Release binary'leri `-trimpath` ile derlenir (bkz. `.goreleaser.yaml`),
bu yüzden aynı commit'ten aynı Go araç zinciriyle **herhangi bir** temiz
checkout'ta yapılan bir derleme, resmi release arşivinin içindeki
binary'yle byte-byte aynıdır — hangi mutlak dosya yolunda derlendiğinden
bağımsız olarak. `checksums.txt` arşivlerin (binary'nin kendisinin değil)
SHA-256'sını listeler, bu yüzden karşılaştırma indirilen arşivi açıp
yapılmalıdır:

```sh
# indirilen resmi arşivden binary'yi çıkarın (örn. comrade_<sürüm>_linux_amd64.tar.gz)
tar -xzf comrade_<sürüm>_linux_amd64.tar.gz comrade
sha256sum comrade > /tmp/released.sha256

git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade && git checkout v<sürüm>
make build   # -> ./comrade, aynı -trimpath bayrağıyla
sha256sum comrade   # /tmp/released.sha256 ile karşılaştırın — aynı olmalı
```

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

### npm — canlı (alternatif kanal)

```sh
npm install -g cli-comrade
```

Bu, terminalle uğraşmak istemeyen ana kitlemiz için değil, zaten Node.js
kurulu geliştiriciler için bir **alternatif** kurulum yoludur — yukarıdaki
`install.sh`/`install.ps1`/Homebrew/Scoop kanalları birincil kalır.

`cli-comrade` paketi, Node.js/npm zaten kurulu ortamlar için önceden
derlenmiş `comrade` binary'sini indirir — herhangi bir Go/derleme araç
zinciri gerekmez; platforma özgü binary, 5 `@firatkutay/comrade-<os>-<cpu>`
paketinden (`linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`,
`win32-x64`) biri, `optionalDependencies` üzerinden otomatik seçilir. Bu
nedenle `npm ci --ignore-scripts` de sorunsuz çalışır (postinstall
script'i yoktur).

**Önemli sınır: `comrade upgrade` bir npm kurulumunda kendi kendini
güncellemeyi reddeder** (aynı davranış pnpm/yarn/bun'la kurulduğunda da
geçerlidir — hepsi aynı dispatcher'ı çalıştırır) — ikili dosyayı yerinde
değiştirmek, paket yöneticisinin kendi kayıtlı sürümünü diskteki gerçek
durumdan koparır. Bunun yerine paket yöneticinizle güncelleyin:

```sh
npm update -g cli-comrade
```

`comrade upgrade --check` bu durumda reddedilmez — hiçbir şey indirmez
veya değiştirmez, yalnızca daha yeni bir sürüm olup olmadığını bildirir.
Ayrıntılar için bkz. [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md).

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

npm/pnpm/yarn/bun ile kurduysanız `comrade upgrade` kendi kendini
güncellemeyi reddeder — yukarıdaki "npm" bölümüne ve
[docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md)'a bakın.

`comrade upgrade`, indirdiği sürümü kurmadan önce artık bir cosign
imzasını (offline, ağ erişimi olmadan) doğrular; imza doğrulanamazsa
kurulum iptal edilir. Ayrıntılar için bkz.
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md) ve
[`docs/SECURITY.md`](SECURITY.md).

### Kaldırma (Uninstall)

Binary'i silmek uygulamayı **tam olarak** kaldırmaz — comrade aşağıda
listelenen birkaç yerde durum bırakır. Önce API anahtarlarını temizleyin
(güvenlik açısından en önemli adım), shell kancasını binary hâlâ PATH'teyken
kaldırın, sonra binary'nin kendisini silin.

#### Kanal başına kaldırma komutu

| Kanal | Kaldırma komutu |
|---|---|
| `install.sh` (macOS/Linux) | `rm -f "$(command -v comrade)"` — varsayılan olarak `$HOME/.local/bin/comrade`, o yazılamıyorsa `/usr/local/bin/comrade`'dır (kurulumda `COMRADE_INSTALL_DIR` verildiyse onu kullanın) |
| `install.ps1` (Windows) | `Remove-Item -Recurse -Force "$env:LOCALAPPDATA\Programs\cli-comrade"` (kurulumda `-InstallDir`/`$env:COMRADE_INSTALL_DIR` verildiyse onu kullanın) |
| Homebrew | `brew uninstall comrade` — isterseniz ayrıca `brew untap firatkutay/tap` |
| Scoop | `scoop uninstall comrade` — isterseniz ayrıca `scoop bucket rm firatkutay` |
| winget | `winget uninstall cli.comrade` — paket henüz moderatör onayı beklediği için bu komut da yukarıdaki kurulum komutu gibi henüz çalışmaz |
| .deb | `sudo dpkg -r comrade` |
| .rpm | `sudo rpm -e comrade` |
| npm | `npm uninstall -g cli-comrade` — pnpm/yarn/bun ile kurduysanız o paket yöneticisinin kendi `remove -g`/`uninstall -g` komutunu kullanın; 5 platforma özgü `@firatkutay/comrade-*` paketi `optionalDependencies` üzerinden otomatik kaldırılır |

`install.ps1`'in kalıcı olarak kullanıcı `PATH`'ine eklediği kaydı da
temizlemek isterseniz (script `$InstallDir`'i, zaten yoksa, doğrudan
`Environment.SetEnvironmentVariable` ile ekler — bir rc dosyası değil):

```powershell
$p = [Environment]::GetEnvironmentVariable("Path", "User")
$dir = "$env:LOCALAPPDATA\Programs\cli-comrade"
[Environment]::SetEnvironmentVariable("Path", (($p -split ';') | Where-Object { $_ -ne $dir }) -join ';', "User")
```

#### Kalan veriler

Binary'yi (ve varsa paket yöneticisi kaydını) kaldırmak yukarıdakilerin
hiçbirini silmez — comrade'ın bıraktığı her şey ayrı ayrı temizlenmelidir.
Aşağıdaki liste `internal/config`, `internal/context`, `internal/update`,
`internal/audit`, `internal/secrets` ve `internal/undo`'nun kodunda
bulunan her kalıcı durumu kapsar (`internal/undo` diske hiçbir şey
yazmaz — bellek içi bir tersine-çevirme kural motorudur, kaldırılacak
ayrı bir dosyası yoktur). **Önce API anahtarlarıyla başlayın** — güvenlik
sonucu olan tek adım budur. Son madde (config + state dizinlerinin
tamamen silinmesi) **GERİ ALINAMAZ**.

1. **API anahtarları (önce bu — güvenlik önemli).** comrade hâlâ
   çalışıyorsa kendi komutuyla kaldırın — keychain'de mi dosya
   fallback'inde mi saklandığını bilmenize gerek yok, `comrade auth logout`
   doğru backend'i kendisi bulur (`internal/secrets/store.go`):

   ```sh
   comrade auth logout anthropic
   comrade auth logout openai_compat
   comrade auth logout google
   ```

   comrade artık kurulu değilse: OS keychain'inde `cli-comrade` servis adı
   altında kayıtlı girdileri elle arayıp silin (macOS Keychain Access,
   Windows Credential Manager, veya Linux'ta Secret Service — `secret-tool`
   ya da Seahorse gibi bir GUI üzerinden; `internal/secrets/store.go`'daki
   `serviceName = "cli-comrade"`, `github.com/zalando/go-keyring` v0.2.8
   üzerinden). Hiçbir OS keychain'i yoksa anahtarlar bunun yerine 0600
   izinli düz bir dosyadaydı (base64 ile gizlenmiş, **şifreli değil**):
   `~/.config/cli-comrade/credentials` (ya da
   `$XDG_CONFIG_HOME/cli-comrade/credentials`), Windows'ta
   `%APPDATA%\cli-comrade\credentials` — bu dosya, aşağıdaki adım 4'teki
   config dizini silmenin bir parçası olarak da temizlenir.

2. **Shell entegrasyonu — binary'yi silmeden ÖNCE yapın.**
   `comrade init <shell> --remove` (PowerShell için
   `comrade init powershell --remove`) kancayı ilgili rc dosyasından
   (`~/.bashrc`, `~/.zshrc`, fish config, PowerShell `$PROFILE`) kaldırır
   ve fish kullanıyorsanız tamamlama dosyasını da siler. Binary hâlâ
   PATH'teyken çalıştırılmalıdır — önce binary'yi silerseniz kancayı elle
   rc dosyasından çıkarmanız gerekir.

3. **`install.sh`'ın eklediği PATH satırı.** Yukarıdaki PowerShell
   komutunu kullanmadıysanız ve `install.sh`, `COMRADE_NO_MODIFY_PATH`
   ayarlanmamışken rc dosyanızı düzenlediyse, şu işaretle başlayan iki
   satırı bulup silin:

   ```
   # Added by the cli-comrade installer — https://github.com/firatkutay/cli-comrade
   export PATH="...:$PATH"
   ```

   Hangi dosyada olduğunu bulmak için:

   ```sh
   grep -n "Added by the cli-comrade installer" ~/.bashrc ~/.zshrc ~/.config/fish/config.fish ~/.profile 2>/dev/null
   ```

4. **Config dosyası** — ayarlar ve config profilleri (profiller ayrı bir
   dosyada değil, bu dosyanın içinde `[profiles.*]` tabloları olarak
   tutulur): `~/.config/cli-comrade/config.toml` (ya da
   `$XDG_CONFIG_HOME/cli-comrade/config.toml`), Windows'ta
   `%APPDATA%\cli-comrade\config.toml`.

5. **State — denetim kaydı, son komut ve güncelleme-kontrolü önbelleği**,
   hepsi aynı dizinde: `audit.jsonl` (her çalıştırılan komutun zaman
   damgası, mod, komut, risk sınıfı, exit code kaydı — CLAUDE.md güvenlik
   kuralı #4), `last_command.json` (shell kancasının yakaladığı son
   komut/exit code/hata çıktısı), `update_check.json`
   (`comrade upgrade`'in kendi kendini güncelleme önbelleği). Dizin:
   `~/.local/state/cli-comrade/` (ya da `$XDG_STATE_HOME/cli-comrade/`),
   Windows'ta `%LOCALAPPDATA%\cli-comrade\`.

**Hepsini tek seferde silmek** (**GERİ ALINAMAZ** — config profillerinizi,
denetim geçmişinizi ve dosya-fallback kullanıyorsanız API anahtarlarınızı
kalıcı olarak siler; yukarıdaki adım 1'i önce yaptığınızdan emin olun):

```sh
rm -rf ~/.config/cli-comrade ~/.local/state/cli-comrade
```

Windows:

```powershell
Remove-Item -Recurse -Force "$env:APPDATA\cli-comrade", "$env:LOCALAPPDATA\cli-comrade"
```

---

## English

The **primary install path** is the `install.sh`/`install.ps1` one-liners
below (Homebrew, Scoop, and the direct `.deb`/`.rpm` packages are equally
live, supported channels too — npm is an **alternative** for developers who
already have Node.js installed, see the "npm" section below). Every install
method is built from the exact same signed/checksummed archives and
packages on every release (see `.goreleaser.yaml`). None of them is a blind
`curl | sudo bash` — even the install scripts themselves verify the
downloaded archive against that release's own `checksums.txt` before
installing anything (see below).

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

**`install.ps1` (Windows) now has the same protection** (GitHub issue #43):
it authenticates `checksums.txt` against a cosign public key embedded in
the script, byte-identical to `internal/update/cosign.pub` — the same
fail-closed default and `COMRADE_INSTALL_ALLOW_UNSIGNED` override. It isn't
a straight port of `install.sh`'s openssl approach, though, because of the
ECDSA API differences between Windows PowerShell 5.1 and PowerShell 7 — see
SECURITY.md's "Trust model of the `curl | sh` bootstrap path" section.

### Reproducible builds

Release binaries are built with `-trimpath` (see `.goreleaser.yaml`), so a
build from a clean checkout of the same commit, with the same Go toolchain,
is byte-identical to the binary inside the official release archive —
regardless of the absolute path it was built at. `checksums.txt` hashes the
archives, not the raw binary, so verify by extracting first:

```sh
# extract the binary from the downloaded official archive (e.g. comrade_<version>_linux_amd64.tar.gz)
tar -xzf comrade_<version>_linux_amd64.tar.gz comrade
sha256sum comrade > /tmp/released.sha256

git clone https://github.com/firatkutay/cli-comrade.git
cd cli-comrade && git checkout v<version>
make build   # -> ./comrade, with the same -trimpath flag
sha256sum comrade   # compare against /tmp/released.sha256 — should match
```

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

### npm — live (alternative channel)

```sh
npm install -g cli-comrade
```

This is an **alternative** install path for developers who already have
Node.js installed — not our primary channel for the terminal-averse
audience this project targets. The `install.sh`/`install.ps1`/Homebrew/
Scoop channels above remain primary.

The `cli-comrade` package installs a prebuilt `comrade` binary for
environments that already have Node.js/npm — no Go/build toolchain
required. The right platform-specific binary is selected automatically via
`optionalDependencies`, resolving to one of 5 scoped packages —
`@firatkutay/comrade-linux-x64`, `-linux-arm64`, `-darwin-x64`,
`-darwin-arm64`, `-win32-x64` — so `npm ci --ignore-scripts` works fine too
(there is no postinstall script).

**Important limitation: `comrade upgrade` refuses to self-update on an npm
install** (the same holds for pnpm/yarn/bun installs — all of them run the
same distribution dispatcher) — replacing the binary in place would desync
the package manager's own recorded version from what's actually on disk.
Update it with your package manager instead:

```sh
npm update -g cli-comrade
```

`comrade upgrade --check` is not refused in this case — it downloads and
changes nothing, and still reports whether a newer version is available.
See [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md) for details.

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

If you installed via npm/pnpm/yarn/bun, `comrade upgrade` refuses to
self-update — see the "npm" section above and
[docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md).

`comrade upgrade` now verifies a cosign signature (offline, no network
lookup) on the downloaded release before installing it — the upgrade
aborts if the signature doesn't check out. See
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md) and
[`docs/SECURITY.md`](SECURITY.md) for details.

### Uninstalling

Deleting the binary does **not** fully remove the app — comrade leaves
state in several places listed below. Clear your API keys first (the
one step with security consequences), remove the shell hook while the
binary is still on PATH, then delete the binary itself.

#### Per-channel uninstall command

| Channel | Uninstall command |
|---|---|
| `install.sh` (macOS/Linux) | `rm -f "$(command -v comrade)"` — defaults to `$HOME/.local/bin/comrade`, falling back to `/usr/local/bin/comrade` (use whatever `COMRADE_INSTALL_DIR` was set to at install time) |
| `install.ps1` (Windows) | `Remove-Item -Recurse -Force "$env:LOCALAPPDATA\Programs\cli-comrade"` (use whatever `-InstallDir`/`$env:COMRADE_INSTALL_DIR` was set to at install time) |
| Homebrew | `brew uninstall comrade` — optionally also `brew untap firatkutay/tap` |
| Scoop | `scoop uninstall comrade` — optionally also `scoop bucket rm firatkutay` |
| winget | `winget uninstall cli.comrade` — the package is still awaiting moderator review, so this command doesn't work yet either, same as the install command |
| .deb | `sudo dpkg -r comrade` |
| .rpm | `sudo rpm -e comrade` |
| npm | `npm uninstall -g cli-comrade` — if you installed with pnpm/yarn/bun, use that package manager's own `remove -g`/`uninstall -g` command instead; the 5 platform-specific `@firatkutay/comrade-*` packages are removed automatically via `optionalDependencies` |

To also clear the entry `install.ps1` permanently added to your user
`PATH` (the script adds `$InstallDir` directly via
`Environment.SetEnvironmentVariable` when it's missing — not an rc file):

```powershell
$p = [Environment]::GetEnvironmentVariable("Path", "User")
$dir = "$env:LOCALAPPDATA\Programs\cli-comrade"
[Environment]::SetEnvironmentVariable("Path", (($p -split ';') | Where-Object { $_ -ne $dir }) -join ';', "User")
```

#### Leftover data

Removing the binary (and any package-manager registration) deletes none
of the following — everything comrade leaves behind must be cleaned up
separately. This list covers every persistent location found in the
code of `internal/config`, `internal/context`, `internal/update`,
`internal/audit`, `internal/secrets`, and `internal/undo`
(`internal/undo` writes nothing to disk at all — it's an in-memory
reversal-rule engine with no file of its own to remove). **Start with
the API keys** — that's the one step with security consequences. The
last item (deleting the config + state directories entirely) is
**IRREVERSIBLE**.

1. **API keys (do this first — security matters).** If comrade is still
   installed, remove them with its own command — you don't need to know
   whether they're in the keychain or the file fallback,
   `comrade auth logout` finds the right backend itself
   (`internal/secrets/store.go`):

   ```sh
   comrade auth logout anthropic
   comrade auth logout openai_compat
   comrade auth logout google
   ```

   If comrade is already gone: manually find and delete the entries
   stored under the `cli-comrade` service name in your OS keychain
   (macOS Keychain Access, Windows Credential Manager, or Linux Secret
   Service via `secret-tool` or a GUI like Seahorse; see
   `internal/secrets/store.go`'s `serviceName = "cli-comrade"`, via
   `github.com/zalando/go-keyring` v0.2.8). If no OS keychain was
   available, the keys instead lived in a plain 0600-permission file
   (base64-obfuscated, **not encrypted**):
   `~/.config/cli-comrade/credentials` (or
   `$XDG_CONFIG_HOME/cli-comrade/credentials`), on Windows
   `%APPDATA%\cli-comrade\credentials` — this file is also cleared as
   part of deleting the config directory in step 4 below.

2. **Shell integration — do this BEFORE deleting the binary.**
   `comrade init <shell> --remove` (`comrade init powershell --remove`
   on Windows) removes the hook from the relevant rc file
   (`~/.bashrc`, `~/.zshrc`, fish config, PowerShell `$PROFILE`) and
   also deletes the fish completions file if you use fish. It must be
   run while the binary is still on PATH — if you delete the binary
   first, you'll need to remove the hook from the rc file by hand.

3. **The PATH line `install.sh` added.** If you didn't use the
   PowerShell command above, and `install.sh` edited your rc file
   (i.e. `COMRADE_NO_MODIFY_PATH` wasn't set), find and delete the two
   lines starting with this marker:

   ```
   # Added by the cli-comrade installer — https://github.com/firatkutay/cli-comrade
   export PATH="...:$PATH"
   ```

   To find which file has it:

   ```sh
   grep -n "Added by the cli-comrade installer" ~/.bashrc ~/.zshrc ~/.config/fish/config.fish ~/.profile 2>/dev/null
   ```

4. **The config file** — settings and config profiles (profiles aren't
   a separate file; they live as `[profiles.*]` tables inside this
   same file): `~/.config/cli-comrade/config.toml` (or
   `$XDG_CONFIG_HOME/cli-comrade/config.toml`), on Windows
   `%APPDATA%\cli-comrade\config.toml`.

5. **State — the audit log, last command, and update-check cache**,
   all in the same directory: `audit.jsonl` (a record of every
   executed command's timestamp, mode, command, risk class, and exit
   code — CLAUDE.md security rule #4), `last_command.json` (the last
   command/exit code/error output the shell hook captured),
   `update_check.json` (`comrade upgrade`'s own self-update cache).
   Directory: `~/.local/state/cli-comrade/` (or
   `$XDG_STATE_HOME/cli-comrade/`), on Windows
   `%LOCALAPPDATA%\cli-comrade\`.

**Deleting everything in one shot** (**IRREVERSIBLE** — permanently
deletes your config profiles, your audit history, and, if you were on
the file fallback, your API keys; make sure you did step 1 first):

```sh
rm -rf ~/.config/cli-comrade ~/.local/state/cli-comrade
```

Windows:

```powershell
Remove-Item -Recurse -Force "$env:APPDATA\cli-comrade", "$env:LOCALAPPDATA\cli-comrade"
```
