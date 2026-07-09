# PROGRESS
module_path: github.com/firatkutay/cli-comrade
current_phase: 5
status: in_progress

## Tamamlanan Fazlar
- [x] FAZ 0 — commit: 33c92ec
- [x] FAZ 1 — commit: 18e6baa
- [x] FAZ 2 — commit: 105a726
- [x] FAZ 3 — commit: b50852b
- [x] FAZ 4 — commit: c31f71a

## Notlar / Ertelenen İşler
- golangci-lint GPL-3.0 lisanslıdır; yalnızca ayrı süreç (CI/dev aracı) olarak çağrılır, koda gömülmez/vendorlanmaz.
- FAZ 6 için sertleştirme notu: stream connector'larında kanal gönderimlerine ctx.Done() select'i eklenmeli (tüketici kanalı erken terk ederse goroutine sızıntısı — Ctrl-C senaryosu); reviewer tavsiyesi, FAZ 2'de bloklamadı.
- Stream zaman aşımı tüm akış içindir (timeout_seconds); uzun yanıtlar için idle-timeout iyileştirmesi ileride değerlendirilebilir.
- FAZ 10 kalemi: install.sh/install.ps1 arşiv adları .goreleaser.yaml name_template'inin korumasız el-kopyası — FAZ 10'da render-and-diff drift testi eklenecek (reviewer bulgusu).
- FAZ 10 kalemi: install.sh yalnızca curl kullanıyor; wget fallback + önkoşul kontrolü eklenecek.
- Manuel doğrulama: PowerShell hook'unun çalışma zamanı testi Windows ortamı gerektirir (golden testler mevcut); kullanıcı Windows'ta `comrade init powershell` ile doğrulamalı.
