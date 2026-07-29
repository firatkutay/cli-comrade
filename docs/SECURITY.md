# Güvenlik / Security

## Türkçe

### Davranış modları ve onay modeli

| Mod | Davranış |
|---|---|
| `auto` | Komutları kendisi çalıştırır, her adımı tek satırda özetler. |
| `ask` | *(varsayılan)* Her komuttan önce gerekçe + komutu gösterir, `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü` sorar. |
| `info` | Hiçbir şey çalıştırmaz; sadece açıklar. |

**Pazarlık edilemez kural:** `auto` modda bile risk sınıfı
`destructive` olan bir komut HER ZAMAN onay ister. Bu davranış ancak
config'de **hem** `safety.confirm_destructive=false` **hem de**
`--yolo` flag'i birlikte verildiğinde kapanır — ve kapandığında her
kullanımda kırmızı bir uyarı basılır (`--yolo` kırmızı uyarısı, config
tarafındaki bypass koşulları o çalıştırmada gerçekten bir şeyi
atlatıp atlatmadığından bağımsız olarak, her `--yolo` kullanımında
basılır).

### Risk sınıflandırma

Her üretilen komut LLM tarafından sınıflandırılır, ardından yerel bir
kural motoru (regex/AST tabanlı, LLM'e güvenmeden) ikinci kez kontrol
eder — beş sınıf, artan risk sırasıyla: `read` → `write` → `network` →
`elevated` → `destructive`.

Yerel **denylist** (mod ne olursa olsun, LLM ne önerirse önersin,
HER ZAMAN bloklar):

- `rm -rf /` (veya `~`/`$HOME` kök silme)
- `mkfs` (dosya sistemi formatlama)
- `dd of=/dev/<disk>` (ham disk üzerine yazma)
- `diskpart clean` (disk bölüm tablosunu siler)
- PowerShell `Remove-Item`/`ri`/`rd`/`rmdir`/`del`/`erase`/`rm`
  takma adlarıyla `-Recurse <sürücü kökü>`
- `format <sürücü>:` (Windows format)
- fork bomb (`:(){:|:&};:` ve eşdeğerleri)
- `> /dev/<disk>` (gerçek bir disk aygıtına yönlendirme)

### Sertleştirilmiş yıkıcı komut tespiti (v0.3.0)

Yerel kural motoru (`internal/safety`), LLM'in bildirdiği risk sınıfını asla
nihai kabul etmez; imza tabanlı bir denylist/escalation kümesiyle bağımsızca
yeniden değerlendirir. v0.3.0 itibarıyla eskiden kaçırılan şu kalıplar da
yakalanır (denylist eşleşmesi → `Block`, escalation eşleşmesi → `Confirm`):

- `find ... -delete` (rm dışı toplu silme)
- disk'i doğrudan yok eden araç ailesi: `mke2fs`/`mkswap`/`mkdosfs`/
  `mkntfs`/`newfs` (her zaman); ve gerçek bir `/dev/<disk>` aygıtına
  yönlendirildiğinde `wipefs`, `blkdiscard`, `sgdisk`, silme
  bayraklarıyla `sfdisk`, `badblocks -w`, `cryptsetup luksFormat`/`reencrypt`/`erase`
- kök veya ev dizinini hedefleyen `chmod -R`/`chown -R` (yalnızca `777` değil,
  hangi mod olursa olsun)
- `mv ... /dev/null` (taşıyarak silme), `shred -u`, `truncate -s 0`
- Windows depolama cmdlet'leri: `Format-Volume`, `Clear-Disk`,
  `Initialize-Disk`, `Remove-Partition`
- getir-ve-çalıştır kalıpları: `curl ... | sh`, `bash <(curl ...)`,
  `bash -c "$(curl ...)"`, base64 decode + pipe, çıplak `eval`
- `reg delete ... /f`, `diskpart /s <script>`, HKLM:/HKCU: registry silme

Ayrıca: tüm eşleştirmeler büyük/küçük harften bağımsızdır (`rm -Rf /` da
yakalanır), `$(...)` komut ikamesi eşleştirmeden önce düzleştirilir
(`$(rm -rf /)` çıplak `rm -rf /` ile aynı şekilde görülür), ve
`safety.Engine.Evaluate`'den hiç geçmemiş bir adım sessizce `Allow`
sayılmaz — yürütmeden önce yeniden değerlendirmeye zorlanır (kapalı-hata /
fail-closed).

### Bağlam gönderiminden önce redaction (zorunlu, atlanamaz)

LLM'e gönderilen HER payload, gönderilmeden önce `internal/redact`'ten
geçer. Zorunlu desen aileleri her zaman aktiftir (kapatılamaz): API key
biçimleri — `sk-`, `ghp_`/`gho_`, `AKIA...`, Slack `xox[baprs]-`, Google
`AIza...`, GitHub `github_pat_...`/`ghs_...`, GitLab `glpat-...`, Stripe
`sk_live_`/`sk_test_`, Google OAuth `GOCSPX-...`, SendGrid `SG....`, npm
`npm_...`, GCP OAuth `ya29....`, Slack incoming-webhook URL'leri — JWT'ler,
PEM özel anahtar blokları, `key=value`/`key: value` biçimli kimlik bilgisi
çiftleri (bileşik/önekli adlar dahil: `DB_PASSWORD=`,
`AWS_SECRET_ACCESS_KEY=`), `scheme://kullanıcı:parola@` bağlantı dizeleri,
Azure `AccountKey=...`, ve `Authorization: Bearer ...` / `Authorization: Basic ...` başlıkları. İki opsiyonel aile config ile açılır:
`privacy.redact_emails`, `privacy.redact_ips`. Env var İÇERİKLERİ
ASLA gönderilmez — yalnızca isimleri, o da `context.send_env_names`
ile opt-in edilirse.

### API anahtarı saklama: keychain birincil, dosya yedeği

`comrade auth login <sağlayıcı>` ile kaydedilen anahtarlar önce OS
keychain'e (macOS Keychain / Windows Credential Manager / Linux Secret
Service, `zalando/go-keyring` ile) yazılmaya çalışılır. Bir keychain
arka ucu bulunamazsa (örn. başsız/headless bir Linux makinesi), 0600
izinli bir dosyaya **base64 ile gizlenmiş** (şifrelenmiş DEĞİL) olarak
düşülür — bu geçiş her seferinde stderr'e açık bir uyarı basar.
API anahtarları HİÇBİR ZAMAN config dosyasına düz metin yazılmaz.
Gizli anahtarlar deposu tek bir arka uç kullanır — erişilebilir ise OS
keychain'i, aksi takdirde 0600 dosyasını — ve ortam değişkenleri
(`COMRADE_<SAĞLAYICI>_API_KEY`, sonra sağlayıcının kendi değişkeni)
yalnızca depo hiçbir anahtara sahip değilse sorgulanır (bkz.
CONFIGURATION.md).

### Sağlayıcı uç noktası doğrulama (base_url)

`llm.openai_compat.base_url` ve `llm.ollama.base_url` artık doğrulanır
(`internal/config/validate.go`) — API anahtarının hangi ana bilgisayara
gönderileceği kontrolsüz bırakılmaz:

- **Reddedilir** (`comrade config set` hata verir): şema `http`/`https`
  değilse, host boşsa, veya host bir bulut-metadata/link-local adrese işaret
  ediyorsa (`169.254.0.0/16` — AWS/GCP/Azure metadata uç noktasını
  `169.254.169.254` da kapsar — ya da IPv6 `fe80::/10`).
- **Uyarılır ama izin verilir**: şema `http` ve host loopback değilse
  (`localhost`/`127.0.0.0/8`/`::1` dışında) — API anahtarının ağ üzerinde
  şifrelenmemiş gönderileceği uyarısı basılır. Özel ağ aralıkları (`10/8`,
  `192.168/16`, `172.16/12`) kendi barındırılan LLM kurulumları için meşru
  sayılır ve reddedilmez.
- Config her yüklendiğinde (her `comrade` çalıştırmasında) yalnızca **etkin
  sağlayıcının** (`llm.provider`) base_url'ü için aynı kontrol tekrar
  çalışır — ama asla sert hata vermez, yalnızca uyarır (aksi halde bozuk bir
  değer `comrade config set` ile onarma yolunu bile kilitlerdi).
- Gerçek LLM istemcisi kurulurken (`do`/`fix`/`chat`/`explain` çalıştığında)
  etkin sağlayıcının base_url'ü tekrar kontrol edilir ve bu kez
  **reddedilirse istemci hiç oluşturulmaz** — API anahtarı tehlikeli bir
  hosta asla gönderilmez. `comrade config set/get/edit` gibi onarım
  komutları bu sert kontrolden geçmez, her zaman kullanılabilir kalır.

Tam anahtar/varsayılan tablosu için bkz. CONFIGURATION.md.

### Denetim kaydı (audit log)

`audit.enabled=true` (varsayılan) iken her çalıştırılan komut
`$XDG_STATE_HOME/cli-comrade/audit.jsonl`'a (Windows:
`%LOCALAPPDATA%\cli-comrade\audit.jsonl`) tek satırlık bir JSON kaydı
olarak eklenir: zaman damgası, orijinal istek, çalıştırılan komut,
risk sınıfı, mod, exit code, süre. `comrade history` bu kaydı okur.
`audit.retention_days` (varsayılan 90) kadar eski kayıtlar periyodik
olarak temizlenir.

### Telemetri: varsayılan kapalı

`privacy.telemetry` varsayılan olarak `false`'tur. Açılsa bile
gönderilen tek şey anonim kullanım sayaçlarıdır — asla komut içeriği,
asla kişisel veri.

### Kendi kendini güncelleme imza doğrulaması (cosign)

`comrade upgrade`, indirdiği `checksums.txt`'i güvenilir saymadan önce
ikili içine gömülü bir cosign genel anahtarına (`internal/update/cosign.pub`,
ECDSA P-256/SHA-256, saf Go ile doğrulanır) karşı imzasını doğrular —
tamamen çevrimdışı, Rekor/şeffaflık-kaydı sorgusu OLMADAN
(`--tlog-upload=false`). Sıra: imza → checksum → çıkarma/ikiliyi
değiştirme. Yayınlar CI'da goreleaser'ın `signs` bloğu + cosign ile
imzalanır. v0.3.0 itibarıyla gerçek anahtar gömülüdür ve imzalama
zorunludur: imzasız ya da geçersiz imzalı bir sürüm yükseltmeyi durdurur
(kapalı-hata/fail-closed). Ayrıntılar için bkz.
[UPDATE_SIGNING.md](UPDATE_SIGNING.md).

### `curl | sh` bootstrap yolunun güven modeli (GitHub issue #28)

v0.4.x'e kadar `scripts/install.sh`, indirdiği `checksums.txt`'i **hiç
doğrulamadan** güvenilir sayıyordu — arşivin kendisi o dosyaya karşı
`sha256sum`/`shasum`/`openssl dgst` ile doğrulanıyordu, ama
`checksums.txt`'in KENDİSİNİ kimin yazdığı hiç kontrol edilmiyordu. Bu,
`comrade upgrade`'in zaten kapattığı tam olarak aynı boşluktu: bir checksum
yalnızca arşivin manifestoyla eşleştiğini kanıtlar, manifestoyu kimin
yazdığını kanıtlamaz. `install.sh` — projeyle stranger bir makinede,
incelemeden çalışan tek script — bu boşluğa sahipti; `comrade upgrade`
sahip değildi.

Bu artık kapatıldı: `install.sh`, `checksums.txt`'i `comrade upgrade` ile
**birebir aynı mekanizmayla** doğrular — `internal/update/cosign.pub`'daki
gerçek anahtarın birebir aynı baytları script içine PEM literal olarak
gömülüdür (`COSIGN_PUB` değişkeni; drift'e karşı
`internal/update/install_sh_mirror_test.go` ile korunur), ve
`checksums.txt.sig`, `openssl dgst -sha256 -verify` ile (saf openssl,
cosign CLI'sine gerek yok) doğrulanır. Sıra `comrade upgrade` ile aynı:
imza → checksum → çıkarma/kurulum.

**openssl bulunamazsa veya bir release `checksums.txt.sig` yayınlamamışsa**
(imza hiç KONTROL EDİLEMEZse — geçersiz bir imzadan farklı bir durum),
davranış varsayılan olarak **kapalı-hata**dır: kurulum durur. Bu bilinçli
bir seçimdir — sessizce checksum-only doğrulamaya geri dönmek, bu
özelliğin kapatmaya çalıştığı tam olarak aynı zayıf garantiyi yeniden
açardı. Openssl'siz minimal bir imaj gibi gerçek bir alternatifi olmayan
kullanıcılar için tek çıkış yolu `COMRADE_INSTALL_ALLOW_UNSIGNED=1`
ortam değişkenidir — her kullanımda yüksek sesle bir uyarı basar
(`--yolo` ile aynı "asla sessiz değil" ilkesi). Bir imza gerçekten
DOĞRULANAMAZSA (indirilen `checksums.txt` gerçekten imzasıyla eşleşmiyorsa)
bu override GEÇERLİ DEĞİLDİR — bu her zaman koşulsuz olarak durur.

**`scripts/install.ps1` (Windows) artık aynı korumaya sahip** (GitHub issue
#43, bu bölümün daha önce takip ettiği boşluğu kapatır): `checksums.txt`'i
aynı gömülü cosign genel anahtarına karşı doğrular (`internal/update/cosign.pub`
ile birebir aynı, `internal/update/install_sh_mirror_test.go`'daki
`TestInstallPs1EmbedsExactCosignPub` ile korunur), aynı kapalı-hata
varsayılanı ve `COMRADE_INSTALL_ALLOW_UNSIGNED` override'ı ile. Ancak
`install.sh`'ın `openssl dgst -sha256 -verify` yaklaşımını birebir
kopyalayamaz: openssl Windows'ta standart değildir, ve ECDSA doğrulama
API'si Windows PowerShell 5.1 (.NET Framework) ile PowerShell 7 (.NET Core)
arasında farklıdır — projenin belgelenmiş destek matrisi (bkz. INSTALL.md)
ikisini de gerektirir. Sürüme göre dallanmak yerine
`System.Security.Cryptography.ECDsaCng` + `CngKey.Import`'u, elle
oluşturulmuş bir CNG genel-anahtar blob'u ile kullanır: `ECDsaCng`,
.NET Framework 4.6.1'den beri bu şekilde çalışır ve Windows'ta PowerShell 7
altında da aynı davranır, çünkü ikisi de aynı temel Windows CNG API'sini
sarmalar. Tam mekanizma için `scripts/install.ps1`'in kendi "checksums.txt
cosign-signature verification" yorum bloğuna, imza testleri için
`scripts/install_test.ps1`'e bakın (Windows CI'da hem `pwsh` hem
`powershell` altında `internal/cli/scripts_test.go`'daki
`TestInstallPs1ChecksumsSignatureVerification` ile çalıştırılır).

### CI güvenlik taraması: secret scanning + SBOM/SCA (GitHub issue #47)

`.github/workflows/ci.yml`'deki `gitleaks` ve `sbom-scan` işleri:

- **gitleaks** (`gitleaks dir`, v8.30.1) her push/PR'da ÇALIŞMA AĞACINI
  tarar — tam git geçmişini değil. Bu depoya özgü gerekçe: değişiklikler
  doğrudan push/self-merge ile gelir (trunk-based), bu yüzden "bir sırrı
  ekleyip merge'den önce sil" tehdidi burada gerçekçi değil; ayrıca bu
  deponun bilinen iki gerçek-sır-benzeri olayı (cosign özel anahtarı —
  yalnızca bir GitHub Actions secret'ı olarak tutulur; sohbet üzerinden
  sızan ve iptal edilen bir npm token'ı) hiçbiri git'e hiç commit
  edilmedi, dolayısıyla herhangi bir tarama derinliği bunları zaten
  yakalayamazdı. Gate'i kablolamadan ÖNCE, bir defalık tam geçmiş taraması
  (`gitleaks git --redact --log-opts="--all"`, tüm commit'ler/tüm ref'ler)
  yerel olarak çalıştırıldı ve gerçek hiçbir kimlik bilgisi bulunamadı —
  yalnızca `.gitleaks.toml`'da kural+dosya yolu bazında allowlist'lenen,
  `internal/redact`'in test fixture'larındaki bilinçli sahte değerler
  (AWS'nin kendi örnek anahtarı, jwt.io'nun örnek JWT'si, vb.). v0.4.10'da
  (PR #57) bu allowlist, gitleaks'in yerleşik `slack-webhook-url` ve
  `slack-app-token` kurallarını da kapsayacak şekilde genişledi — ikisi de
  yine `internal/redact/redact_test.go`'a özel, aynı kural-ID + dosya-yolu
  prensibiyle (`.gitleaks.toml`): gitleaks'in yerleşik Slack kuralları,
  bu dosyanın bilinçli olarak gerçekçi webhook/workflow/trigger URL'leri
  ve `xapp-1-...` fixture'ıyla eşleşiyor. Bu tam
  geçmiş taraması periyodik olarak (örn. yıllık) veya bir dış katkıcı
  onboard edilmeden önce elle tekrarlanmalı — CI gate'ine dahil değil.
- **sbom-scan**: `syft` her push/PR'da bir CycloneDX kaynak SBOM'u üretir
  (CI artifact'i olarak yüklenir — `release.yml`'in kendi SBOM adımı
  bugün üretilen dosyayı hiçbir yere yüklemediği için, bu depo şu anda
  bunun dışında erişilebilir bir SBOM'a sahip değil). `grype` bu SBOM'u
  tarar ama BİLGİLENDİRİCİ modda çalışır (`fail-build: false`) — merge'i
  bloke etmez. Neden: bu depoda tek bağımlılık manifestosu (`go.mod`/
  `go.sum`) için `govulncheck` (yukarıdaki `vulncheck` işi) zaten
  call-graph tabanlı, ulaşılabilirlik-farkında bir tarama yapıyor; grype
  ise yalnızca sürüm eşleştirmesi yapar ve ulaşılabilirlik analizi yoktur.
  Ampirik kanıt: bu iş yazılırken grype tam olarak GO-2026-5970'i (High,
  `golang.org/x/text` v0.28.0) buldu — govulncheck aynı ID'yi zaten
  biliyor ve kodun o sembollere hiç erişmediğini kanıtlayarak güvenli
  sayıyor. `grype`'ı `--fail-on high` ile bloke edici yapmak, zaten
  triyaj edilmiş bu bulguda her PR'ı kırardı — tam olarak bu işin
  kaçınmaya çalıştığı "yok sayılan gate" durumu.

### GitHub-native secret scanning ve CodeQL triyaj (2026-07-29)

Yukarıdaki bölüm yalnızca **gitleaks** CI işini (çalışma ağacı taraması)
belgeler — GitHub'ın kendi secret scanning'i bugüne kadar hiç açık
değildi (`secret_scanning: disabled`,
`secret_scanning_push_protection: disabled`). 2026-07-29 itibarıyla
ikisi de `enabled`. `secret_scanning_validity_checks` ise **açılamadı**
ve `disabled` kalmaya devam ediyor — bu dürüstçe not edilsin diye burada
kayıtlı.

**Push protection kanıtını çoktan verdi.** v0.4.10 geliştirmesi
sırasında, yeni bir redaction test fixture'ı gerçek bir Slack token'ına
benzediği için push protection bir push'u BLOKLADI; geliştirici değeri
açıkça `TEST` etiketli, sahte bir değerle değiştirdi. Bu, kontrolün
gerçekten çalıştığının somut kanıtı — ve `internal/redact/redact_test.go`
içindeki fixture'ların (`xoxb-fake_TEST_9Zk1`, `xapp-1-TESTfake9Zk1` gibi)
neden bilinçli olarak sahte göründüğünü gelecekteki katkıcılara açıklıyor.

**Geçmiş üzerinde bir secret-scanning uyarısı — `used_in_tests` olarak
çözüldü.** Uyarı #1, tip "Google API Key", konum
`internal/redact/redact_test.go:152`. Redaction test tablosundaki
sentetik bir fixture: test, değerin `[REDACTED:api_key]`'e maskelendiğini
ve ham dizenin çıktıda bulunmadığını doğruluyor. Yalnızca `:152`'de
(girdi) ve `:154`'te (bulunmamalı doğrulaması) geçtiği, ağacın veya git
geçmişinin başka hiçbir yerinde geçmediği doğrulandı; `used_in_tests`
nedeniyle çözüldü. Ayrıca ayrı bir kontrolle doğrulandı: deponun commit
geçmişinin tamamında yapılan taramada **sıfır** gerçek sızmış kimlik
bilgisi bulundu (yukarıdaki gitleaks tam-geçmiş taramasıyla aynı sonuç).

**İki CodeQL uyarısı yanlış pozitif olarak kapatıldı — asıl kayda
geçirilmesi gereken bu.** Uyarı **#1** ve **#3**, ikisi de kural
`go/regex/missing-regexp-anchor` (CWE-20, `security_severity_level: high`),
ikisi de `internal/redact/redact.go`'daki Slack incoming-webhook
dedektörü üzerinde. #3, #1'in yeniden yükselmiş hali: v0.4.10 (#57 PR'ı)
o satırı GERÇEKTEN düzenledi — eski
`https://hooks\.slack\.com/services/[A-Za-z0-9/]+` deseni yeni
`(?i)(?:https?://)?hooks\.slack\.com/[A-Za-z0-9_+/-]+` ile değiştirildi
(commit `8915480`) — bu yalnızca bir satır-numarası kayması değil,
deseni yeniden yazan gerçek bir düzenleme, dolayısıyla CodeQL aynı satırı
yeni bir uyarı numarasıyla (#3) yeniden bildirdi.

Kayda geçirilecek gerekçe (her iddia kodla karşılaştırılarak doğrulandı):

- Bu bir **dedektör**, doğrulayıcı değil. Tek tüketicisi
  `internal/redact/redact.go`'daki `apiKeyPatterns` maskeleme döngüsü
  (satır 247-249) — `ReplaceAllString` ile `[REDACTED:api_key]`'e ikame
  yapıyor. Depoda hiçbir kod yolu bunu bir boolean'a, dallanmaya, ya da
  erişim kararına çevirmiyor. Paket yalnızca `Redactor`, `New`, ve
  `Apply`'ı export ediyor (`redact.go:10,18,238`); desenler export
  edilmiyor ve hiç geri döndürülmüyor. Tek importer `internal/llm/client.go`'daki
  `Client.redactPayload` (`client.go:432`, import `client.go:12`).
- Kuralın önermesi — "bu bir URL üzerinde regex olarak kullanıldığında"
  — burada geçerli değil: desen bir URL'yi *bulmak için* bir haystack'e
  uygulanıyor, bir URL'yi *doğrulamak için* asla.
- **Risk yönü ters.** Fazla eşleşme yalnızca modele biraz fazladan bağlam
  maliyeti getirir; eksik eşleşme canlı bir sırrı sızdırır. Bu yüzden
  ankajsız (unanchored) form tasarım gereği zorunludur ve ankajlamak bir
  güvenlik düzeltmesi değil, bir GERİLEME olurdu — ampirik olarak
  doğrulandı: ankajlı bir varyant (`^...$`), bir komut satırına gömülü
  bir webhook'u eşleştiremiyor
  (`curl -X POST https://hooks.slack.com/services/... -d @x.json`) —
  yani gerçekçi her girdide.
- Bu satır işaretlendi, aynı blokta eşit derecede ankajsız olan diğer 25
  kardeşi işaretlenmedi — TEK NEDENİ bu bloktaki tek bir literal TLD/host
  adı (`hooks\.slack\.com`) içeren desen olması; kuralın hostname
  sezgiseli buna göre çalışıyor, ankajlama olarak değil
  (`internal/redact/redact.go`'daki 26 `regexp.MustCompile` çağrısının
  tamamı doğrulandı — hiçbiri ankajlı değil, yalnızca satır 173 literal
  bir host adı içeriyor).
- **Bunun tekrar edeceği beklenmeli**: o satıra yapılacak herhangi bir
  gelecekteki düzenleme yeni bir numarayla yeni bir uyarı doğuracaktır
  (#1→#3 geçişi bunu zaten kanıtladı). Doğru tepki bu gerekçeyle YENİDEN
  kapatmaktır, regex'i ankajlamak değil.

### Yeniden üretilebilir (reproducible) release binary'leri

İmza doğrulaması yalnızca `checksums.txt`'in bütünlüğünü kanıtlar; kimin
derlediğini kanıtlamaz. Bu boşluğu kapatmak için release binary'leri
`-trimpath` (`.goreleaser.yaml`, `Makefile`) ile derlenir — bu, derlemenin
mutlak kaynak yolunu ikili dosyaya gömülmekten çıkarır. Sonuç: aynı
commit'ten aynı Go araç zinciriyle yapılan **herhangi bir** temiz
checkout derlemesi, hangi mutlak yolda çalıştırıldığından bağımsız olarak
resmi release'deki binary'yle byte-byte aynıdır (iki farklı mutlak yolda
yapılan derlemeyle doğrudan doğrulandı — bkz. `docs/INSTALL.md`'nin
"Yeniden üretilebilir derlemeler" bölümü). Bu özellik, `install.sh`'ın
cosign imza doğrulamasıyla birlikte, elle yapılan bir derlemenin
(örneğin npm paketinin ilk kez yayınlanması için kullanılan tek seferlik
elle derleme) imzalı release'e karşı bağımsızca doğrulanabilmesini sağlar
— aksi halde npm sürümleri değişmez olduğundan böyle bir derleme hiçbir
zaman doğrulanamaz kalırdı.

### `--yolo` flag'i

Her kullanımda kırmızı bir uyarı basar (CLAUDE.md güvenlik kuralı #6).
Yalnızca config'de `safety.confirm_destructive=false` **ve**
`safety.confirm_elevated=false` ayarları da varsa gerçek bir etkisi
olur; aksi halde uyarı basılır ama destructive/elevated onayları yine
istenir.

---

## English

### Behavior modes and the confirmation model

| Mode | Behavior |
|---|---|
| `auto` | Runs commands itself, printing a one-line summary per step. |
| `ask` | *(default)* Shows the rationale + command before each step, prompts `[y]es [n]o [e]dit [x]plain [a]ll`. |
| `info` | Runs nothing; only explains. |

**Non-negotiable rule:** even in `auto` mode, a command classified
`destructive` ALWAYS requires confirmation. This is bypassed only when
config has BOTH `safety.confirm_destructive=false` AND `--yolo` is
given — and every `--yolo` use prints a red warning regardless of
whether the config-side bypass actually changes anything for that
particular run.

### Risk classification

Every generated command is classified by the LLM, then independently
re-checked by a local rule engine (regex/AST-based, never trusting the
LLM) — five classes, in increasing risk order: `read` → `write` →
`network` → `elevated` → `destructive`.

The local **denylist** (blocks regardless of mode, regardless of what
the LLM suggested):

- `rm -rf /` (or a `~`/`$HOME` root delete)
- `mkfs` (filesystem format)
- `dd of=/dev/<disk>` (raw disk overwrite)
- `diskpart clean` (wipes a disk's partition table)
- PowerShell `Remove-Item`/`ri`/`rd`/`rmdir`/`del`/`erase`/`rm` alias
  with `-Recurse <drive root>`
- `format <drive>:` (Windows format)
- a fork bomb (`:(){:|:&};:` and equivalents)
- `> /dev/<disk>` (redirect into a real disk device)

### Hardened destructive-command detection (v0.3.0)

The local rule engine (`internal/safety`) never treats the LLM's declared
risk class as final — it independently re-checks every command against a
signature-based denylist/escalation set. As of v0.3.0 this additionally
catches patterns that previously slipped through (a denylist match →
`Block`, an escalation match → `Confirm`):

- `find ... -delete` (mass, non-`rm` deletion)
- the disk-destroying tool family: `mke2fs`/`mkswap`/`mkdosfs`/`mkntfs`/
  `newfs` (always); and, when pointed at a real `/dev/<disk>` device,
  `wipefs`, `blkdiscard`, `sgdisk`, `sfdisk` with destructive flags,
  `badblocks -w`, `cryptsetup luksFormat`/`reencrypt`/`erase`
- `chmod -R`/`chown -R` targeting a root or home directory (any mode, not
  just `777`)
- `mv ... /dev/null` (discard via move), `shred -u`, `truncate -s 0`
- Windows storage cmdlets: `Format-Volume`, `Clear-Disk`, `Initialize-Disk`,
  `Remove-Partition`
- fetch-and-execute shapes: `curl ... | sh`, `bash <(curl ...)`,
  `bash -c "$(curl ...)"`, base64 decode piped into an interpreter, a bare
  `eval`
- `reg delete ... /f`, `diskpart /s <script>`, HKLM:/HKCU: registry
  deletion

Also: every match is case-insensitive (`rm -Rf /` is caught too), `$(...)`
command substitution is flattened before matching (`$(rm -rf /)` is seen
exactly like a bare `rm -rf /`), and a step that never actually passed
through `safety.Engine.Evaluate` is never silently treated as `Allow` — it
is forced through re-evaluation before it can run (fail-closed).

### Redaction before any context is sent (mandatory, cannot be bypassed)

EVERY payload sent to the LLM passes through `internal/redact` first.
Mandatory pattern families are always active (cannot be turned off): API
key formats — `sk-`, `ghp_`/`gho_`, `AKIA...`, Slack `xox[baprs]-`, Google
`AIza...`, GitHub `github_pat_...`/`ghs_...`, GitLab `glpat-...`, Stripe
`sk_live_`/`sk_test_`, Google OAuth `GOCSPX-...`, SendGrid `SG....`, npm
`npm_...`, GCP OAuth `ya29....`, Slack incoming-webhook URLs — JWTs, full
PEM private-key blocks, `key=value`/`key: value`-shaped credential pairs
(including compound/prefixed names like `DB_PASSWORD=`,
`AWS_SECRET_ACCESS_KEY=`), `scheme://user:pass@` connection strings, Azure
`AccountKey=...`, and `Authorization: Bearer ...` / `Authorization: Basic ...` headers. Two optional families are config-gated: `privacy.redact_emails`,
`privacy.redact_ips`. Environment variable CONTENTS are never sent —
only their names, and only when `context.send_env_names` is opted in.

### API key storage: keychain primary, file fallback

Keys saved via `comrade auth login <provider>` are written to the OS
keychain first (macOS Keychain / Windows Credential Manager / Linux
Secret Service, via `zalando/go-keyring`). When no keychain backend is
reachable (e.g. a headless Linux machine), they fall back to a 0600
file, **base64-obfuscated** (NOT encrypted) — this fallback prints an
explicit stderr warning every time it's used. API keys are NEVER
written to the config file in plaintext. The secrets store uses a
single active backend — the OS keychain when available, otherwise a
0600 credentials file — and environment variables (`COMRADE_<PROVIDER>_API_KEY`,
then the provider's own vendor variable) are consulted only when the
store has no key (see CONFIGURATION.md).

### Provider endpoint validation (base_url)

`llm.openai_compat.base_url` and `llm.ollama.base_url` are now validated
(`internal/config/validate.go`) — the API key's destination host is no
longer left unchecked:

- **Rejected** (`comrade config set` errors out): the scheme isn't
  `http`/`https`, the host is empty, or the host is a literal
  cloud-metadata / link-local address (`169.254.0.0/16` — which covers the
  `169.254.169.254` metadata endpoint used by AWS/GCP/Azure — or IPv6
  `fe80::/10`).
- **Warned but allowed**: the scheme is `http` and the host is not loopback
  (anything other than `localhost`/`127.0.0.0/8`/`::1`) — a warning that the
  API key will be sent unencrypted is printed. Private network ranges
  (`10/8`, `192.168/16`, `172.16/12`) are treated as legitimate for
  self-hosted LLM setups and are never rejected.
- Every time config loads (every `comrade` invocation), the same check
  re-runs for the **active provider's** (`llm.provider`) base_url only —
  but it never hard-fails, only warns (a hard fail here could brick the
  repair commands themselves).
- When the real LLM client is actually built (running `do`/`fix`/`chat`/
  `explain`), the active provider's base_url is checked again, and this
  time a reject-class value means **the client is never constructed** — the
  API key is never handed to a dangerous host. Repair commands
  (`comrade config set/get/edit`) don't go through this hard check and stay
  usable regardless.

See CONFIGURATION.md for the full key/default table.

### Audit log

While `audit.enabled=true` (the default), every executed command is
appended as one JSON line to
`$XDG_STATE_HOME/cli-comrade/audit.jsonl` (Windows:
`%LOCALAPPDATA%\cli-comrade\audit.jsonl`): timestamp, the original
request, the command actually run, risk class, mode, exit code,
duration. `comrade history` reads this log. Entries older than
`audit.retention_days` (default 90) are periodically cleaned up.

### Telemetry: off by default

`privacy.telemetry` defaults to `false`. Even when enabled, the only
thing ever sent is anonymous usage counters — never command content,
never personal data.

### Self-update signature verification (cosign)

`comrade upgrade` verifies the signature of the `checksums.txt` it
downloads against a cosign public key embedded in the binary
(`internal/update/cosign.pub`, ECDSA P-256/SHA-256, verified in pure Go)
before trusting it — fully offline, with NO Rekor/transparency-log lookup
(`--tlog-upload=false`). Order: signature → checksum → extract/replace the
binary. Releases are signed in CI via goreleaser's `signs` block + cosign.
As of v0.3.0 a real key is embedded and signing is enforced: a missing or
invalid signature aborts the upgrade (fail-closed). See
[UPDATE_SIGNING.md](UPDATE_SIGNING.md) for details.

### Trust model of the `curl | sh` bootstrap path (GitHub issue #28)

Through v0.4.x, `scripts/install.sh` trusted the `checksums.txt` it
downloaded outright — the archive itself was verified against that file
(`sha256sum`/`shasum`/`openssl dgst`), but nothing verified WHO WROTE
`checksums.txt` in the first place. That was exactly the gap
`comrade upgrade` had already closed: a checksum only proves the archive
matches the manifest, never who authored the manifest. `install.sh` — the
one script guaranteed to run, unreviewed, on a stranger's machine over a
public one-liner — had this weaker guarantee; `comrade upgrade` didn't.

This is now closed: `install.sh` authenticates `checksums.txt` with
**the exact same mechanism** `comrade upgrade` uses. The project's real
cosign public key — byte-identical to `internal/update/cosign.pub` — is
embedded as a literal PEM block in the script itself (the `COSIGN_PUB`
variable; guarded against drift by
`internal/update/install_sh_mirror_test.go`), and `checksums.txt.sig` is
verified with `openssl dgst -sha256 -verify` (plain openssl, no cosign
CLI required). Order matches `comrade upgrade`: signature → checksum →
extract/install.

**If openssl is missing, or a release has no published
`checksums.txt.sig`** (the signature can't be CHECKED at all — distinct
from an actual signature mismatch), the default is **fail-closed**: the
install aborts. This is deliberate — silently falling back to
checksum-only verification would quietly reopen exactly the weaker
guarantee this feature exists to close. The one escape hatch, for users
with no real alternative (e.g. a minimal image with no openssl), is the
`COMRADE_INSTALL_ALLOW_UNSIGNED=1` environment variable — it prints a
loud warning on every use (the same never-silent principle already
applied to `--yolo`). This override does NOT apply when a signature is
actually checked and fails to verify (a real mismatch) — that always
aborts unconditionally, with no override.

**`scripts/install.ps1` (Windows) has the same protection** (GitHub issue
#43, closing the gap this section originally tracked): it authenticates
`checksums.txt` against the same embedded cosign public key (byte-identical
to `internal/update/cosign.pub`, guarded by
`internal/update/install_sh_mirror_test.go`'s
`TestInstallPs1EmbedsExactCosignPub`), with the identical fail-closed
default and `COMRADE_INSTALL_ALLOW_UNSIGNED` override. It cannot reuse
`install.sh`'s `openssl dgst -sha256 -verify` approach, though: openssl
isn't standard on Windows, and the ECDSA-verification API differs between
Windows PowerShell 5.1 (.NET Framework) and PowerShell 7 (.NET Core) — the
repo's own documented support matrix (see docs/INSTALL.md) requires both.
Rather than branching per runtime, it uses
`System.Security.Cryptography.ECDsaCng` + `CngKey.Import` with a hand-built
CNG public-key blob: `ECDsaCng` has worked this way since .NET Framework
4.6.1 and behaves identically under PowerShell 7 on Windows, since both
wrap the same underlying Windows CNG API. See `scripts/install.ps1`'s own
"checksums.txt cosign-signature verification" comment block for the full
mechanism, and `scripts/install_test.ps1` for the signature tests (run via
`internal/cli/scripts_test.go`'s `TestInstallPs1ChecksumsSignatureVerification`
on Windows CI, against both `pwsh` and `powershell`).

### CI security scanning: secret scanning + SBOM/SCA (GitHub issue #47)

The `gitleaks` and `sbom-scan` jobs in `.github/workflows/ci.yml`:

- **gitleaks** (`gitleaks dir`, v8.30.1) scans the WORKING TREE on every
  push/PR — not full git history. Rationale specific to this repo:
  changes land via direct push/self-merge (trunk-based), so the "add a
  secret then remove it before merge" threat a history scan defends
  against doesn't really apply; and neither of this repo's two known
  real-secret-adjacent incidents (the cosign signing key, held only as a
  GitHub Actions secret; an npm token leaked via chat and revoked) was
  ever committed to git, so no scan depth would have caught either. A
  one-off FULL history scan (`gitleaks git --redact --log-opts="--all"`,
  every commit / every ref) was run locally before wiring the gate, per
  issue #47's mandatory baseline, and found zero real credentials — only
  the golden-test-fixture false positives allowlisted per rule + exact
  file path in `.gitleaks.toml` (deliberately fake values in
  `internal/redact`'s own test fixtures — AWS's own example key, jwt.io's
  example JWT, etc.). In v0.4.10 (PR #57) this allowlist grew to also
  cover gitleaks' built-in `slack-webhook-url` and `slack-app-token`
  rules — both scoped to the same `internal/redact/redact_test.go` path
  under the same rule-ID + file-path principle (`.gitleaks.toml`):
  gitleaks' built-in Slack rules match this file's deliberately realistic
  webhook/workflow/trigger URL fixtures and its `xapp-1-...` fixture.
  Re-run that full-history scan manually on a
  standing cadence (e.g. annually) or before onboarding an external
  contributor — it is not wired into the per-PR gate.
- **sbom-scan**: `syft` generates a CycloneDX source SBOM on every
  push/PR (uploaded as a CI artifact — `release.yml`'s own SBOM step
  currently generates a file it never uploads anywhere, so this repo has
  no other retrievable SBOM today). `grype` scans that SBOM but runs
  INFORMATIONAL-only (`fail-build: false`) — it does not block merge.
  Why: this repo's one dependency manifest (`go.mod`/`go.sum`) is already
  covered by `govulncheck` (the `vulncheck` job above), which is
  call-graph-aware and reachability-checked; grype only matches versions
  and has no reachability analysis. Empirical evidence: when this job was
  written, grype found exactly GO-2026-5970 (High, `golang.org/x/text`
  v0.28.0) — a vulnerability govulncheck already knows about and already
  excludes, having proven the code never calls the affected symbols.
  Making grype blocking at `--fail-on high` would fail every PR on this
  one already-triaged finding — exactly the "gate people learn to ignore"
  outcome this job is designed to avoid.

### GitHub-native secret scanning and CodeQL triage (2026-07-29)

The section above only documents the **gitleaks** CI job (a working-tree
scan) — GitHub's own secret scanning had never been enabled at all
(`secret_scanning: disabled`,
`secret_scanning_push_protection: disabled`). As of 2026-07-29 both are
`enabled`. `secret_scanning_validity_checks` **could not be enabled** and
remains `disabled` — noted here honestly rather than glossed over.

**Push protection has already proven itself.** During v0.4.10
development, push protection BLOCKED a push because a new redaction test
fixture looked like a real Slack token; the developer replaced it with
an obviously-fake, `TEST`-marked value. That's concrete evidence the
control actually works — and it explains to future contributors why
fixtures in `internal/redact/redact_test.go` (e.g.
`xoxb-fake_TEST_9Zk1`, `xapp-1-TESTfake9Zk1`) look deliberately fake.

**One secret-scanning alert on history — resolved as
`used_in_tests`.** Alert #1, type "Google API Key", located at
`internal/redact/redact_test.go:152`. It's a synthetic fixture in the
redaction test table: the test asserts the value is masked to
`[REDACTED:api_key]` and that the raw string is absent from the output.
Verified it appears only at `:152` (input) and `:154` (must-not-appear
assertion), nowhere else in the tree or in git history; resolved with
reason `used_in_tests`. Also verified separately: a full-history search
found **zero** real leaked credentials across the repo's commits (the
same conclusion as the gitleaks full-history scan above).

**Two CodeQL alerts dismissed as false positives — this is the main
thing to record.** Alerts **#1** and **#3**, both rule
`go/regex/missing-regexp-anchor` (CWE-20, `security_severity_level: high`),
both on the Slack incoming-webhook detector in
`internal/redact/redact.go`. #3 is #1 re-raised: v0.4.10 (PR #57)
actually edited that line — the old
`https://hooks\.slack\.com/services/[A-Za-z0-9/]+` pattern was replaced
with the new `(?i)(?:https?://)?hooks\.slack\.com/[A-Za-z0-9_+/-]+`
(commit `8915480`) — this isn't merely a line-number shift, it's a real
edit that rewrote the pattern, so CodeQL re-reported the same line under
a new alert number (#3).

The reasoning to record (every claim verified against the code):

- The regex is a **detector**, not a validator. Its only consumer is
  the `apiKeyPatterns` masking loop in `internal/redact/redact.go`
  (lines 247-249) — it calls `ReplaceAllString` to substitute
  `[REDACTED:api_key]`. No code path anywhere in the repo turns it into
  a boolean, a branch, or an access decision. The package exports only
  `Redactor`, `New`, and `Apply` (`redact.go:10,18,238`); the patterns
  are unexported and never returned. The sole importer is
  `Client.redactPayload` in `internal/llm/client.go`
  (`client.go:432`, imported at `client.go:12`).
- The rule's premise — "when this is used as a regular expression on a
  URL" — does not hold here: the pattern is applied *to a haystack to
  find a URL*, never *to a URL to validate it*.
- **The risk direction is inverted.** Over-matching only costs the model
  some context; under-matching leaks a live secret. So the unanchored
  form is required by design, and anchoring would be a security
  regression, not a fix — verified empirically: an anchored variant
  (`^...$`) fails to match a webhook embedded in a command line
  (`curl -X POST https://hooks.slack.com/services/... -d @x.json`) —
  i.e. every realistic input.
- This line was flagged and its 25 equally-unanchored siblings in the
  same block were not, solely because it is the only pattern in that
  block containing a literal TLD/hostname (`hooks\.slack\.com`) — which
  is what the query's hostname heuristic keys on, not anchoring as such
  (verified: all 26 `regexp.MustCompile` calls in
  `internal/redact/redact.go` — none are anchored, and only line 173
  contains a literal hostname).
- **Expect this to recur**: any future edit to that line will raise a
  new alert with a new number (the #1 → #3 transition already proves
  this). The correct response is to re-dismiss with this same reasoning,
  not to anchor the regex.

### Reproducible release binaries

Signature verification only proves `checksums.txt`'s integrity, not who
built the artifacts it describes. To close that gap, release binaries are
built with `-trimpath` (`.goreleaser.yaml`, `Makefile`) — this strips the
build's absolute source path from being embedded in the binary. The
result: any clean-checkout build of the same commit with the same Go
toolchain is byte-identical to the binary inside the official release,
regardless of the absolute path it was built at (directly verified with
two builds at two different absolute paths — see docs/INSTALL.md's
"Reproducible builds" section). Combined with `install.sh`'s cosign
signature check, this means a manually built binary — such as the
one-time manual build used to first publish the npm package — can be
independently verified against the signed release, which would otherwise
be permanently unverifiable once published, since npm package versions are
immutable.

### The `--yolo` flag

Prints a red warning on every use (CLAUDE.md security rule #6). It
only has a real effect when config ALSO has both
`safety.confirm_destructive=false` AND `safety.confirm_elevated=false`
— otherwise the warning still prints, but destructive/elevated
confirmations are still required.
