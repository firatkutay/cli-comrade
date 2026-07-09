# PROGRESS
module_path: github.com/firatkutay/cli-comrade
current_phase: 11
status: done

## Tamamlanan Fazlar
- [x] FAZ 0 — commit: 33c92ec
- [x] FAZ 1 — commit: 18e6baa
- [x] FAZ 2 — commit: 105a726
- [x] FAZ 3 — commit: b50852b
- [x] FAZ 4 — commit: c31f71a
- [x] FAZ 5 — commit: 6f4b1c3
- [x] FAZ 6 — commit: dd15f48
- [x] FAZ 7 — commit: f6b89ba
- [x] FAZ 8 — commit: a90d74e
- [x] FAZ 9 — commit: 5b8fca4
- [x] FAZ 10 — commit: 04d52c8
- [x] FAZ 11 — commit: afc31ae

## Notlar / Ertelenen İşler
- golangci-lint GPL-3.0 lisanslıdır; yalnızca ayrı süreç (CI/dev aracı) olarak çağrılır, koda gömülmez/vendorlanmaz.
- FAZ 6 için sertleştirme notu: stream connector'larında kanal gönderimlerine ctx.Done() select'i eklenmeli (tüketici kanalı erken terk ederse goroutine sızıntısı — Ctrl-C senaryosu); reviewer tavsiyesi, FAZ 2'de bloklamadı.
- Stream zaman aşımı tüm akış içindir (timeout_seconds); uzun yanıtlar için idle-timeout iyileştirmesi ileride değerlendirilebilir.
- FAZ 10 kalemi: install.sh/install.ps1 arşiv adları .goreleaser.yaml name_template'inin korumasız el-kopyası — FAZ 10'da render-and-diff drift testi eklenecek (reviewer bulgusu).
- FAZ 10 kalemi: install.sh yalnızca curl kullanıyor; wget fallback + önkoşul kontrolü eklenecek.
- Manuel doğrulama: PowerShell hook'unun çalışma zamanı testi Windows ortamı gerektirir (golden testler mevcut); kullanıcı Windows'ta `comrade init powershell` ile doğrulamalı.
- Manuel doğrulama: FAZ 5 planlayıcının gerçek LLM ile "docker kur" kabul senaryosu API key gerektirir (httptest mock ile uçtan uca doğrulandı); kullanıcı gerçek key ile bir kez denemeli.
- Karar (kullanıcı onaylı): TUI için bubbletea v2 seçildi; Go tabanı 1.23 → 1.25'e yükseltildi (FAZ 0 kararı bilinçli tersine çevrildi). CLAUDE.md/go.mod güncellendi.
- Manuel doğrulama (Windows): internal/executor Windows dalında süreç-ağacı öldürme tek-süreç (torun süreçler kalabilir); pwsh yok, çalışma zamanı testi Windows'ta yapılmalı — FAZ 11 sertleştirme kalemi.
- Manuel doğrulama: comrade fix gerçek LLM ile "pyton --version" senaryosu API key gerektirir (mock ile uçtan uca doğrulandı); kullanıcı gerçek key ile bir kez denemeli.
- Manuel doğrulama: gerçek OS keychain (macOS Keychain / Windows Credential Manager / Linux Secret Service) ve gerçek TTY no-echo parola girişi bu ortamda test edilemedi (go-keyring mock + injectable reader ile doğrulandı); kullanıcı bir platformda comrade auth login ile doğrulamalı.
- i18n istisnaları (gerekçeli, docs/phases/FAZ-09.md'de belgeli): confirm.go seçenek harfleri (CLAUDE.md), cobra Use komut adları, hook.go gizli komut/debug satırı, promptui.go LLM prompt'u, ~40 "işlem: %w" hata sarmalama zinciri.
- FAZ 10 gerçek yayın için hazır ama henüz tag atılmadı: goreleaser snapshot tüm artefaktları üretiyor; gerçek release öncesi homebrew-tap / scoop-bucket / winget-pkgs hedef repoları ve tap PAT secret'ı oluşturulmalı.
- cosign imzalama .goreleaser.yaml'de yorumlu (etkinleştirme notu); anahtar/keyless kurulumu gerektirir.
- Manuel doğrulama: install.sh/install.ps1 ve comrade upgrade gerçek ağ yolu (GitHub Releases) canlı test edilmedi (mock/snapshot ile doğrulandı); ilk gerçek release'te doğrulanmalı.
- v0.1.0-rc1 hazır (tag atılmadı — kullanıcı kararı; tag push'u gerçek release pipeline'ını tetikler ve tap repoları/PAT gerekir). Soğuk başlangıç ~5ms, tüm gate + -race + govulncheck temiz. Bilinen kısıtlar KNOWN_LIMITATIONS.md'de.
