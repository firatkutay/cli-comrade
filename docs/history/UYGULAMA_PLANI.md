# UYGULAMA_PLANI.md — cli-comrade

> Bu dosya projenin tam uygulama planıdır. Claude Code bu planı FAZ 0'dan başlayarak sırayla, otonom olarak uygular. Yürütme protokolü CLAUDE.md'de tanımlıdır. Bu dosya DEĞİŞTİRİLMEZ; ilerleme durumu `docs/PROGRESS.md`'de tutulur.

---

## FAZ 0 — Proje İskeleti

**Kapsam:** iskelet ve altyapı. Kod mantığı yok.

1. **Go modülü:** `go mod init` — modül yolu `docs/PROGRESS.md` başlığındaki `module_path` değerinden alınır (master prompt'ta kullanıcı belirtir).
2. **Dizin yapısı:** CLAUDE.md'deki dizin yapısını aynen oluştur; her internal paket için doc.go içine tek paragraflık paket açıklaması yaz.
3. **cobra root command:** `comrade` çalıştığında versiyon + kısa yardım gösterecek minimal iskelet. Alt komutları (fix, explain, chat, config, init, history) stub olarak kaydet — her biri "bu özellik henüz hazır değil" mesajı bassın (şimdilik hardcoded EN; Faz 9'da i18n'e taşınacak).
4. **Makefile:** build, test, lint, cross-compile (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) hedefleri.
5. **golangci-lint:** makul bir .golangci.yml.
6. **GitHub Actions:** üç OS matrix'inde build + test + lint çalıştıran ci.yml.
7. **goreleaser:** temel .goreleaser.yaml (arşivler + checksums; brew/scoop/winget Faz 10'da).
8. **README.md:** proje vizyonu, üç davranış modu tablosu, "geliştirme aşamasında" uyarısı — TR ve EN iki bölüm.
9. **.gitignore, LICENSE (MIT), CHANGELOG.md** başlangıç dosyaları.

**Kabul:** `make build` üç ana platform için binary üretir; `./comrade --version` ve `--help` çalışır; `go vet` + `golangci-lint run` temiz.

---

## FAZ 1 — Config Sistemi

**Kapsam:** yapılandırma altyapısı.

1. `internal/config`: viper ile TOML config yükleme. Konum: Linux/macOS `~/.config/cli-comrade/config.toml`, Windows `%APPDATA%\cli-comrade\config.toml`. `XDG_CONFIG_HOME` desteği.
2. Config şeması (Go struct + varsayılanlar):

```toml
[general]
mode = "ask"              # auto | ask | info
language = "auto"         # auto | tr | en
color = true

[llm]
provider = "anthropic"    # anthropic | openai_compat | google | ollama
model = ""                # boşsa provider varsayılanı
fallback = []             # ["ollama/llama3.1", "openai_compat/gpt-4o-mini"] gibi
timeout_seconds = 60
max_tokens = 2048

[llm.openai_compat]
base_url = "https://api.openai.com/v1"

[llm.ollama]
base_url = "http://localhost:11434"

[safety]
confirm_destructive = true   # auto modda bile destructive onayı
confirm_elevated = true      # sudo/admin komutları için onay
denylist_extra = []          # kullanıcı ek denylist regex'leri
max_auto_steps = 10          # auto modda tek istekte en fazla adım

[context]
send_history = false
history_depth = 5
send_env_names = false

[privacy]
redact_emails = false
redact_ips = false
telemetry = false

[audit]
enabled = true
retention_days = 90
```

3. `comrade config` alt komutları: `get <key>`, `set <key> <value>`, `list`, `edit` (EDITOR ile aç), `path`. `set` işlemi tip doğrulaması yapar (mode'a "hızlı" yazılamaz vb.).
4. İlk çalıştırmada config yoksa: varsayılanlarla oluştur + tek satır bilgi mesajı.
5. Env override: `COMRADE_MODE`, `COMRADE_PROVIDER`, `COMRADE_MODEL` gibi `COMRADE_` prefix'li değişkenler config'i ezer.
6. Tablo tabanlı testler: geçersiz değerler, kısmi config, env override öncelik sırası.

Kabul: `comrade config list` düzgün tablo basar; `comrade config set general.mode auto` kalıcı olur; testler yeşil.

---

## FAZ 2 — LLM Provider Katmanı

**Kapsam:** provider soyutlaması ve dört connector.

1. `internal/llm`: CLAUDE.md'deki `Provider` interface'i. `CompletionRequest` system prompt, mesaj listesi, JSON şema beklentisi ve max_tokens taşır.
2. Connector'lar (SDK'sız, ham REST):
   - `anthropic`: Messages API, `x-api-key` header
   - `openai_compat`: `/chat/completions`, `base_url` parametreli — OpenAI, Mistral, Groq, GLM, Qwen, Kimi, OpenRouter, LM Studio hepsi bundan geçer
   - `google`: Gemini `generateContent`
   - `ollama`: `/api/chat`; ayrıca `ListModels()` (`/api/tags`) — Faz 8'de `comrade config` model seçiminde kullanılacak
3. `internal/llm/parse.go`: yanıtı JSON'a çevir — markdown fence temizliği, tek geçerli JSON objesi çıkarımı, şema doğrulama, anlamlı hata mesajları.
4. Fallback zinciri: `llm.fallback` listesi; timeout/5xx/parse hatasında sıradaki provider. Her denemede debug log.
5. API key çözümleme sırası (Faz 7'de keychain gelecek, şimdilik): `COMRADE_<PROVIDER>_API_KEY` env > `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` gibi bilinen env'ler > hata + kullanıcıya nasıl ayarlanacağını anlatan mesaj.
6. `comrade config test-llm` gizli komutu: konfigüre provider'a "ping" isteği atar, gecikme + model adı basar.
7. Testler: httptest sahte sunucularla dört connector, fallback senaryosu, parse edge case'leri (fence'li JSON, çift JSON, bozuk JSON).

Kabul: `comrade config test-llm` gerçek bir key ile çalışır; testler yeşil; hiçbir connector diğerinin koduna bağımlı değil.

---

## FAZ 3 — Context Collector + Redaction

**Kapsam:** bağlam toplama ve gizli bilgi maskeleme.

1. `internal/context`:
   - OS/arch/shell tespiti (shell: `SHELL` env, Windows'ta parent process veya `PSModulePath` varlığı ile powershell/cmd ayrımı)
   - Çalışma dizini, home, kullanıcının admin/root olup olmadığı
   - Paket yöneticisi tespiti: PATH'te apt/dnf/pacman/zypper/brew/port/winget/scoop/choco taraması, bulunanların listesi
   - `last_command.json` okuma (Faz 4'te shell hook yazacak; şimdilik dosya formatını tanımla ve oku): `{command, exit_code, stderr_tail, stdout_tail, timestamp, shell}`
   - Opt-in geçmiş: `context.send_history=true` ise shell history dosyasından son N komut
2. `internal/redact`:
   - Pattern'ler: `sk-[A-Za-z0-9]{20,}`, `ghp_...`, `gho_...`, `AKIA[0-9A-Z]{16}`, `xox[baprs]-...`, JWT (`eyJ...`), `-----BEGIN ... PRIVATE KEY-----` blokları, `password=`, `passwd=`, `token=`, `secret=`, `api_key=` değerleri, `Authorization: Bearer ...`
   - Opsiyonel: e-posta ve IP maskeleme (config'e bağlı)
   - Maske formatı: `[REDACTED:api_key]` gibi tip etiketli — LLM neyin gizlendiğini bilsin
   - Redaction, LLM katmanına giden TEK giriş noktasında zorunlu middleware olarak uygulanır (`llm.Client.Complete` sarmalayıcısı); connector'lar doğrudan çağrılamaz (paket görünürlüğüyle zorla)
3. Golden testler: her pattern için maskele/maskeleme vakaları; false-positive kontrolü (örn. "risk-free" içindeki "sk-" tetiklememeli — kelime sınırı kuralları).

Kabul: redact bypass edilemez (derleme düzeyinde); context collector üç OS'te derlenir; testler yeşil.

---

## FAZ 4 — Shell Entegrasyonu (`comrade init`)

**Kapsam:** hata yakalama hook'ları.

1. `comrade init [bash|zsh|fish|powershell]`:
   - Parametresizse shell'i tespit et, kullanıcıya ne ekleyeceğini göster, onayla rc dosyasına ekle (idempotent — marker yorum satırları ile, ikinci çalıştırma dupliket üretmez)
   - `--print` flag'i: sadece snippet'i bas (kullanıcı kendisi eklesin)
   - `comrade init --remove`: marker'lar arasını temizle
2. Hook davranışı (her shell için):
   - Her komut sonrası: komut metni, exit code, timestamp, shell adını `$XDG_STATE_HOME/cli-comrade/last_command.json`'a yaz (Windows: `%LOCALAPPDATA%\cli-comrade\`)
   - stderr yakalama: bash/zsh'de güvenilir global stderr tee riskli — bunun yerine exit code != 0 ise kullanıcı `comrade fix` dediğinde aracın komutu `comrade fix --rerun` ile kontrollü tekrar çalıştırıp stderr'i kendisinin yakalaması birincil strateji olsun. Hook sadece komut + exit code kaydeder. Bu kararı docs/phases'e gerekçesiyle yaz.
   - PowerShell: `$PROFILE`'a prompt fonksiyonu; `Get-History -Count 1` + `$LASTEXITCODE`
3. Fallback zinciri (`comrade fix` için, Faz 6'da kullanılacak):
   a. `last_command.json` taze ise (< 10 dk) onu kullan
   b. Değilse `comrade fix -- <komut>`: komutu çalıştır, çıktıyı yakala, analiz et
   c. O da yoksa: "hatayı yapıştır" interaktif modu
4. `scripts/install.sh` ve `scripts/install.ps1`: binary indirme + PATH'e ekleme + `comrade init` çağrısı öneren kurulum scriptleri (goreleaser URL şablonlu).
5. Testler: snippet üretimi golden testleri; idempotency testi (iki kez init → tek blok); last_command.json yazma/okuma round-trip.

Kabul: temiz bir bash ve powershell ortamında `comrade init` sonrası başarısız bir komutun kaydı json'a düşer.

---

## FAZ 5 — Engine: Plan Üretimi + Risk Sınıflandırma

**Kapsam:** aracın beyni.

1. `internal/engine`:
   - Giriş: kullanıcı isteği (doğal dil) + context. Çıkış: `Plan{Steps []Step, Summary string}`; `Step{Command, Rationale, Risk, Reversible bool}`
   - LLM system prompt'u: JSON şema zorunlu; OS/shell'e uygun komut üret; kullanıcı dilinde rationale yaz; her adıma risk etiketi ata; geri alınabilirlik bilgisi ver; asla zincirleme tehlikeli tek-satırlık üretme (adımlara böl)
   - System prompt'ları Go embed ile `internal/engine/prompts/` altında ayrı dosyalarda tut (TR talimat bloğu dahil)
2. `internal/safety` kural motoru (LLM'den bağımsız ikinci kontrol):
   - Denylist: CLAUDE.md'deki liste + config `denylist_extra`
   - Risk yükseltme kuralları: `rm -r`, `Remove-Item -Recurse`, `chmod -R 777`, `> /dev/sd`, registry `Remove-*`, `killall`, `iptables -F`, `git push --force` → en az `destructive` veya `elevated`'a yükselt (LLM ne derse desin)
   - Sonuç: `Allow | Confirm | Block(reason)`
   - Tablo tabanlı test: en az 60 vaka, üç OS komut seti, hem Unix hem PowerShell varyantları
3. Plan doğrulama: boş plan, tek adımda birden çok bağımsız iş, max_auto_steps aşımı → yeniden istem veya kırpma.
4. Bu fazda YÜRÜTME YOK — `comrade do "istek" --dry-run` gizli komutu planı tablo halinde basar (adım, komut, risk, gerekçe).

Kabul: `--dry-run` ile "docker kur" isteği makul, OS'e uygun, risk-etiketli bir plan basar; safety testleri yeşil.

---

## FAZ 6 — Executor + Üç Mod (auto / ask / info)

**Kapsam:** yürütme döngüsü. Bu, ürünün kalbi — özenli ol.

1. `internal/executor`:
   - Unix: `sh -c` (kullanıcı shell'i fish olsa bile üretilen komutlar POSIX sh hedefler — LLM prompt'una bunu ekle); Windows: `powershell -NoProfile -Command`
   - stdout/stderr canlı akış + yakalama, exit code, timeout (adım başına config'den), iptal (Ctrl-C → çalışan adımı öldür, sonrakileri atla, özet bas)
   - Elevated adımlar: komutu SUDO İLE ÇALIŞTIRMA; kullanıcıya "bu adım yönetici yetkisi istiyor" de ve `ask` moduna düşür (auto modda bile)
2. Mod davranışları:
   - **info:** planı numaralı, kopyalanabilir komutlarla ve kısa nedenlerle bas. Hiçbir şey çalıştırma.
   - **ask:** her adımda bubbletea prompt: komut + tek satır gerekçe + risk rozeti; seçenekler `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü`. `düzenle` → satır içi edit sonrası safety'den TEKRAR geçir. `açıkla` → LLM'den o adım için detaylı açıklama iste. `tümü` → kalan `read`/`write` adımlarını onayla; `destructive`/`elevated` yine tek tek sorar.
   - **auto:** adımları sırayla çalıştır, her adımda tek satır durum yaz. `destructive`/`elevated` → onay promptuna düş. Bir adım başarısız olursa: LLM'e hata çıktısını ver, revize plan iste (en fazla 3 self-correction denemesi — BI projendeki pattern), 3'te de olmazsa özet + öneriyle dur.
   - Mod önceliği: `--auto/--ask/--info` flag > `COMRADE_MODE` env > config.
3. `comrade do` yerine kök komut davranışı: `comrade <serbest metin>` bilinen alt komut değilse `do` olarak yorumlanır (`comrade docker kur` çalışsın). cobra'da `RunE` fallback ile çöz.
4. `internal/audit`: her yürütülen adım JSONL olarak `state` dizinine: timestamp, istek, komut, risk, mod, exit code, süre. `comrade history` son kayıtları tablo basar; `--json` flag'i ham çıktı verir. retention_days temizliği açılışta lazy çalışır.
5. Testler: mod davranışları için executor'ı sahte (echo tabanlı) komutlarla süren entegrasyon testleri; self-correction döngüsü mock LLM ile; Ctrl-C senaryosu.

Kabul: üç modun üçü de uçtan uca çalışır; destructive adım auto modda onay ister; audit log dolu ve `comrade history` okunur.

---

## FAZ 7 — `comrade fix` (Hata Çözme Akışı)

**Kapsam:** ana kullanım senaryosu.

1. `comrade fix` akışı:
   a. Faz 4'teki fallback zinciriyle hata bağlamını topla (last_command.json → `--rerun` → yapıştırma modu)
   b. LLM'e özel "diagnose" prompt'u: kök neden analizi + düzeltme planı iste; çıktı şeması `{root_cause, explanation, plan}` — explanation kullanıcı dilinde, teknik olmayan birinin anlayacağı sadelikte
   c. Moda göre: info → neden + çözüm adımları; ask/auto → Faz 6 yürütme döngüsüne planı ver
2. `comrade fix --rerun`: son komutu executor ile tekrar çalıştır, stderr/stdout yakala, analize ver. Komut kendisi destructive sınıfındaysa rerun etme, kullanıcıya sor.
3. Sık hata örüntüleri için system prompt'a few-shot örnekleri ekle (embed dosyaları): command not found (+ paket önerisi: tespit edilen paket yöneticisine göre), permission denied, port already in use, ENOENT, ModuleNotFoundError, git merge conflict, DNS/proxy hataları, PowerShell ExecutionPolicy. Türkçe ve İngilizce ikişer örnek.
4. Çözüm sonrası doğrulama: plan bittiğinde orijinal komutu (destructive değilse) tekrar deneme öner — ask modunda sor, auto modda çalıştır, info modunda öneri olarak yaz.
5. Testler: mock LLM ile uçtan uca fix akışı; taze olmayan last_command reddi; destructive rerun engeli.

Kabul: bash'te `pyton --version` gibi bir hata sonrası `comrade fix` doğru teşhis + moda uygun davranış üretir (mock ve gerçek LLM ile birer manuel senaryo docs'a yazılır).

---

## FAZ 8 — Keychain, `comrade auth` ve Model Yönetimi

**Kapsam:** kimlik bilgisi ve model UX'i.

1. `internal/secrets`: zalando/go-keyring sarmalayıcısı. Keychain yoksa (headless Linux) fallback: `~/.config/cli-comrade/credentials` dosyası, 0600, base64 (şifreleme değil — dosyada bunu belirten yorum satırı) + ilk kullanımda kullanıcıya uyarı.
2. `comrade auth` alt komutları:
   - `login <provider>`: key'i güvenli prompt ile al (echo'suz), keychain'e yaz, test isteği at, sonucu bildir
   - `logout <provider>`, `status` (hangi provider'larda key var, kaynağı: keychain/env)
3. Key çözümleme sırası güncelle: keychain > `COMRADE_*` env > bilinen env'ler.
4. `comrade config models`: aktif provider'dan model listesi çek (Ollama `/api/tags`; openai_compat `/models`; anthropic/google statik bilinen liste + docs linki), numaralı seçim menüsü ile `llm.model`'i güncelle.
5. Ollama otomatik keşif: provider ollama seçili ve erişilemiyor ise anlaşılır mesaj ("Ollama çalışmıyor görünüyor; `ollama serve` ..."), localhost dışı base_url desteği.
6. Testler: keyring mock'u ile login/logout/status; fallback dosya izin kontrolü (Unix'te 0600 doğrulaması).

Kabul: `comrade auth login anthropic` → key keychain'de, config dosyasında key YOK; `comrade config models` Ollama ile canlı liste getirir.

---

## FAZ 9 — i18n (TR/EN) + `comrade explain` + `comrade chat`

**Kapsam:** i18n, explain ve chat komutları.

1. `internal/i18n`: basit katalog (map tabanlı veya go-i18n), TR + EN. Faz 0'dan beri hardcoded tüm kullanıcı metinlerini kataloğa taşı — grep ile tara, hiçbir hardcoded kullanıcı mesajı kalmasın (test: katalog dışı string linter'ı basit bir script ile).
2. Dil çözümleme: config `general.language` > `LANG`/`LC_ALL` (tr_TR → tr) > en. LLM system prompt'larına aktif dil talimatı enjekte edilir.
3. `comrade explain <komut>`: komutu ÇALIŞTIRMADAN parça parça açıklar (flag flag), risk değerlendirmesi ekler ("bu komut şunu siler, dikkat"). Kullanıcı dili.
4. `comrade chat`: bubbletea tabanlı interaktif oturum; bağlam korunur (mesaj geçmişi bellek içi); kullanıcı chat içinde "yap" derse aktif moda göre plan+yürütme tetiklenir; `/mode auto|ask|info` oturum içi geçiş; `/clear`, `/exit`. Oturum geçmişi diske YAZILMAZ (gizlilik) — `--save <dosya>` ile isteğe bağlı dışa aktarım.
5. Testler: dil çözümleme öncelik tablosu; explain'in destructive komutta uyarı ürettiği mock testi.

Kabul: `COMRADE_LANG=tr comrade explain "rm -rf node_modules"` Türkçe, uyarılı açıklama üretir; chat oturumu mod değiştirip komut çalıştırabilir.

---

## FAZ 10 — Paketleme ve Dağıtım

**Kapsam:** paketleme ve dağıtım.

1. goreleaser tamamla: brew tap formülü, scoop bucket manifesti, winget manifesti, .deb/.rpm (nfpm), checksums + (mümkünse) cosign imza adımı notu.
2. `scripts/install.sh` / `install.ps1` son hali: en son release'i indir, PATH kur, `comrade init` öner. README'ye tek satır kurulum komutları.
3. `comrade upgrade`: GitHub Releases'tan yeni sürüm kontrolü + kendi kendini güncelleme (üç OS; Windows'ta çalışan exe rename hilesi). `--check` sadece bildirir.
4. Sürüm bildirimi: haftada en fazla bir kez, komut sonunda tek satır "yeni sürüm var" (config ile kapatılabilir: `general.update_check=false`).
5. `docs/`: INSTALL.md (üç OS), CONFIGURATION.md (tüm anahtarlar tablosu), SECURITY.md (redaction, keychain, audit, telemetri politikası), TROUBLESHOOTING.md. TR/EN.
6. CI'a release workflow: tag push → goreleaser.

Kabul: `git tag v0.1.0` sonrası release pipeline'ı kuru çalıştırmada (`goreleaser release --snapshot`) tüm artefaktları üretir.

---

## FAZ 11 — Sertleştirme ve Sürüm Adayı

**Kapsam:** kalite geçişi.

1. Uçtan uca senaryo testleri (docs/phases/FAZ-11.md'ye senaryo + sonuç tablosu):
   - Ubuntu: apt hatası fix, port çakışması fix, "nginx kur ve başlat" auto modda
   - macOS: brew hatası, dosya izin hatası
   - Windows: ExecutionPolicy hatası, winget ile kurulum, PATH sorunu
2. Fuzz/edge: çok uzun stderr (kuyruk kırpma), UTF-8 dışı çıktı, ağ yokken davranış (anlaşılır offline mesajı; Ollama varsa ona düşme önerisi), LLM'in denylist komut önermesi (block edildiğini doğrula).
3. Performans: soğuk başlangıç < 100ms hedefi (LLM çağrısı hariç); gereksiz init'leri lazy yap.
4. `golangci-lint` sıkı profil + `govulncheck` CI'a ekle.
5. CHANGELOG'u derle, sürümü v0.1.0-rc1 olarak etiketlemeye hazırla, bilinen kısıtlar bölümü yaz.

Kabul: tüm test paketi üç OS CI'ında yeşil; senaryo tablosu eksiksiz; bilinen kısıtlar dürüstçe belgelenmiş.
