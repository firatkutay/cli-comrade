# CLAUDE.md — cli-comrade

## Proje Tanımı

**cli-comrade**, terminal bilgisi olmayan veya uğraşmak istemeyen kullanıcılara komut satırında yoldaşlık eden, cross-platform (Windows/macOS/Linux) bir AI CLI asistanıdır. Kullanıcı doğal dille (Türkçe veya İngilizce) istekte bulunur; araç hatayı analiz eder, komutu üretir ve ayarlanan davranış moduna göre çalıştırır, onay ister veya sadece bilgi verir.

Binary adı: `comrade`

## Temel Kullanım Senaryoları

1. **Hata çözme:** Kullanıcı bir komut çalıştırdı, hata aldı. `comrade fix` (veya `comrade bu hatayı çöz`) der. Araç son komutu, exit code'u ve stderr'i shell entegrasyonu üzerinden yakalar, kök nedeni analiz eder ve moda göre davranır.
2. **Görev yaptırma:** `comrade docker kur`, `comrade şu klasörü yedekle`, `comrade 8080 portunu kim kullanıyor bul ve kapat` gibi doğal dil istekleri → çok adımlı plan → moda göre yürütme.
3. **Açıklama:** `comrade explain "git rebase -i HEAD~5"` → komutun ne yaptığını sade dille açıklar.
4. **Sohbet:** `comrade chat` → bağlamı koruyan interaktif oturum.

## Davranış Modları (kritik tasarım)

Global ayar + komut bazında override flag'leri:

| Mod | Davranış |
|---|---|
| `auto` | Aracı devralır, komutları kendisi çalıştırır. Her adımda tek satırlık ne yaptığını yazar. |
| `ask` | Her komuttan önce kısa gerekçe + komutun kendisini gösterir, `[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü` sorar. **Varsayılan mod budur.** |
| `info` | Hiçbir şey çalıştırmaz. Sorunun nedenini ve çözüm adımlarını kopyalanabilir komutlarla açıklar. |

**Güvenlik istisnası (pazarlık edilemez):** `auto` modda bile risk sınıfı `destructive` olan komutlar HER ZAMAN onay ister. Bu davranış ancak config'de `safety.confirm_destructive=false` + `--yolo` flag'i birlikte varsa kapanır ve kapatıldığında uyarı basılır.

## Komut Risk Sınıflandırması

Her üretilen komut LLM tarafından şu sınıflardan biriyle etiketlenir ve yerel bir kural motoru (regex/AST tabanlı, LLM'e güvenmeden) ikinci kontrol yapar:

- `read` — sadece okuma (ls, cat, Get-ChildItem, df)
- `write` — dosya/ayar değişikliği (mkdir, kurulum, chmod)
- `network` — ağ erişimi (curl, apt update, Invoke-WebRequest)
- `elevated` — sudo / admin yükseltme gerektiren
- `destructive` — geri alınamaz silme/format/registry/disk işlemleri

Yerel kural motoru denylist içerir: `rm -rf /`, `rm -rf ~`, `mkfs`, `dd of=/dev/`, `diskpart clean`, `Remove-Item -Recurse C:\`, `format`, `:(){:|:&};:` vb. Denylist eşleşmesi → mod ne olursa olsun blok + açıklama.

## Teknoloji Stack'i

- **Dil:** Go 1.25+ (tek statik binary, cross-compile)
- **CLI framework:** spf13/cobra
- **Config:** spf13/viper (TOML: `~/.config/cli-comrade/config.toml`; Windows: `%APPDATA%\cli-comrade\config.toml`)
- **TUI/etkileşim:** charmbracelet/bubbletea + lipgloss (onay promptları, spinner, diff görünümü)
- **Keychain:** zalando/go-keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service; yoksa dosya fallback + 0600 izin)
- **HTTP:** stdlib net/http; provider SDK'sı YOK, ham REST istemcileri yazılır (bağımlılığı azaltmak için)
- **Release:** goreleaser (brew tap, scoop bucket, winget manifest, .deb/.rpm, curl install script)
- **Test:** stdlib testing + testify; LLM çağrıları httptest mock'ları ile

## LLM Provider Mimarisi

`internal/llm` altında tek interface:

```go
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
    Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error)
    Name() string
}
```

Connector'lar:
- `anthropic` — Anthropic Messages API
- `openai_compat` — **tek connector, base_url parametreli.** OpenAI, Mistral, Groq, GLM/Zhipu, Qwen, Kimi/Moonshot, OpenRouter, LM Studio bu connector'ı kullanır.
- `google` — Gemini API
- `ollama` — yerel, `http://localhost:11434`, model listesi çekme + otomatik keşif

Model fallback zinciri: config'de sıralı liste; ilk provider hata/timeout verirse sıradakine geçilir.

Yapılandırılmış çıktı: provider'dan HER ZAMAN JSON istenir (plan/komut/risk/açıklama şeması). Markdown backtick temizliği + şema doğrulaması `internal/llm/parse.go`'da tek yerden yapılır.

## Bağlam Toplama (Context Collector)

LLM'e gönderilen bağlam — hepsi `internal/context` altında:

- OS, mimari, shell tipi + versiyonu, çalışma dizini
- Son komut + exit code + stderr/stdout kuyruğu (shell entegrasyonu varsa)
- Opt-in: son N komut geçmişi (varsayılan kapalı)
- Paket yöneticisi tespiti (apt/dnf/pacman/brew/winget/scoop/choco)

**Redaction (zorunlu):** Gönderilmeden önce `internal/redact` şu pattern'leri maskeler: API key formatları (sk-, ghp-, AKIA...), `password=`, `token=`, Bearer başlıkları, e-posta (opsiyonel), IP (opsiyonel). Env var içerikleri ASLA gönderilmez, sadece isimleri (opt-in).

## Shell Entegrasyonu

`comrade init [bash|zsh|fish|powershell]` ilgili hook'u kurar:

- **bash/zsh:** `PROMPT_COMMAND` / `precmd` hook → son komut, exit code ve stderr'i (`exec 2> >(tee ...)` yerine güvenli tmp dosya yaklaşımı) `$XDG_STATE_HOME/cli-comrade/last_command.json`'a yazar
- **fish:** `fish_postexec` event
- **PowerShell:** `$PROFILE`'a prompt fonksiyonu + `$?`/`$LASTEXITCODE` + `Get-History` yakalama
- Hook kurulu değilse `comrade fix` fallback: kullanıcıdan hatayı yapıştırmasını ister veya `comrade fix -- <komut>` ile komutu kendi çalıştırıp gözlemler

## Dizin Yapısı

```
cmd/comrade/            # main, cobra root
internal/
  cli/                  # alt komutlar: fix, do (default), explain, chat, config, init, history
  config/               # viper yükleme, şema, migration
  secrets/              # API key saklama: keychain (birincil) + 0600 dosya fallback
  llm/                  # provider interface + connector'lar + parse
  context/              # bağlam toplayıcı, OS/shell tespiti
  redact/               # gizli bilgi maskeleme
  engine/               # plan üretimi, risk sınıflandırma, yürütme döngüsü
  executor/             # platform-özel komut çalıştırma (sh -c / powershell -Command)
  safety/               # kural motoru, denylist, risk override
  update/               # self-update: release indirme, checksum + gömülü-key cosign imza doğrulama, atomik replace
  shellinit/            # shell entegrasyonu: bash/zsh/fish/PowerShell hook kurulumu
  audit/                # audit log (JSONL)
  i18n/                 # TR/EN mesaj katalogları
  tui/                  # bubbletea bileşenleri
scripts/                # install.sh, install.ps1
docs/
```

## Kod Kuralları

- Go idiomatik: küçük paketler, interface'ler tüketici tarafında tanımlanır, `context.Context` her I/O fonksiyonunun ilk parametresi
- Hata sarmalama: `fmt.Errorf("...: %w", err)`; kullanıcıya gösterilen hatalar i18n katalogundan
- Platform dallanmaları build tag ile DEĞİL, runtime `runtime.GOOS` + `internal/executor` soyutlamasıyla (tek binary'de üç OS testi kolaylaşır)
- Global state yok; her şey dependency injection ile
- Her faz sonunda: `go vet`, `golangci-lint run`, `go test ./...` temiz geçmeli
- Commit mesajları İngilizce, conventional commits (`feat:`, `fix:`, `chore:`)

## i18n Kuralları

- Kullanıcıya görünen TÜM metinler `internal/i18n` kataloglarından (TR + EN)
- Dil seçimi: config > `LANG` env > EN fallback
- LLM'den gelen açıklamalar da kullanıcı dilinde istenir (system prompt'a dil talimatı eklenir)

## Güvenlik Kuralları (pazarlık edilemez)

1. Destructive komutlar auto modda bile onaysız çalışmaz
2. API key'ler asla config dosyasına plaintext yazılmaz (keychain birincil; dosya fallback'i kullanıcı onayı + 0600 ile)
3. Redaction pipeline'ı bypass edilemez; LLM'e giden her payload redact'ten geçer
4. Audit log her yürütülen komutu kaydeder: timestamp, mod, komut, risk sınıfı, exit code
5. Telemetri varsayılan KAPALI; açılırsa sadece anonim kullanım sayaçları, asla komut içeriği değil
6. `--yolo` flag'i her kullanımda kırmızı uyarı basar
7. Self-update (`comrade upgrade`) yalnızca, binary'e gömülü cosign public key'e karşı imzası doğrulanan release'i kurar (saf-Go, offline — Rekor/ağ yok); imza yoksa/doğrulanamıyorsa yükseltme reddedilir (fail-closed). Release'ler CI'da cosign ile imzalanır (`checksums.txt.sig`).

## Test Stratejisi

- Birim: risk sınıflandırıcı ve denylist için tablo tabanlı testler (en az 50 vaka, üç OS komut seti)
- LLM parse: bozuk JSON, markdown sarılı JSON, eksik alan vakaları
- Redaction: bilinen key formatları için golden testler
- Entegrasyon: httptest ile sahte provider; executor için `echo`/`Get-Date` gibi zararsız komutlarla gerçek çalıştırma
- CI: GitHub Actions matrix (ubuntu, macos, windows)

## Otonom Yürütme Protokolü (kritik)

*(Protokol tamamlandı — v0.1.4 itibarıyla tüm fazlar bitti; kayıtlar `docs/history/` altında arşivlendi.)*

Bu proje tek bir master prompt ile başlatılır ve fazlar KULLANICIDAN YENİ PROMPT BEKLENMEDEN sırayla uygulanır. Kurallar:

1. **Kaynak plan:** Tüm faz spesifikasyonları `docs/history/UYGULAMA_PLANI.md`'dedir. Bu dosya asla değiştirilmez.
2. **İlerleme durumu:** `docs/history/PROGRESS.md` tek doğruluk kaynağıdır. Format:

```markdown
# PROGRESS
module_path: github.com/<kullanici>/cli-comrade
current_phase: 3
status: in_progress   # in_progress | done | blocked
## Tamamlanan Fazlar
- [x] FAZ 0 — commit: <hash>
- [x] FAZ 1 — commit: <hash>
- [x] FAZ 2 — commit: <hash>
## Notlar / Ertelenen İşler
- ...
```

3. **Faz döngüsü:** Her faz için sırayla: (a) `docs/history/PROGRESS.md` ve ilgili faz spesifikasyonunu oku → (b) uygula → (c) `go vet` + `golangci-lint run` + `go test ./...` yeşil olana kadar düzelt → (d) `docs/history/phases/FAZ-XX.md` özetini yaz → (e) CHANGELOG girdisi ekle → (f) `git commit` at (conventional commits) → (g) PROGRESS.md'yi güncelle → (h) sonraki faza geç.
4. **Durma koşulları (sadece bunlarda kullanıcıya sor):**
   - Bir kabul kriteri 3 denemede sağlanamıyorsa → durumu `blocked` yap, sorunu ve seçenekleri özetle, dur
   - Önceki fazın public API'sini kırmak gerekiyorsa → gerekçe yaz, onay iste
   - Gerçek API key gerektiren manuel doğrulama adımları → adımı `docs/history/PROGRESS.md` "Notlar"a yaz, otomatik testlerle devam et, kullanıcıyı durdurmadan bilgilendir
5. **Oturum kopması / context daralması:** Kod dışı hiçbir bağlam oturuma emanet edilmez. Yeni oturumda "devam" komutu geldiğinde: CLAUDE.md → docs/history/PROGRESS.md → son faz özeti oku, kaldığın yerden sür. Context daralıyorsa mevcut fazı bitir, commit + PROGRESS güncelle, sonra kullanıcıya `/compact` önerip bekle.
6. Faz atlanmaz, sıra değiştirilmez, kapsam genişletilmez. "İyi olur" fikirleri koda değil PROGRESS.md notlarına yazılır.
