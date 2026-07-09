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

### Tamamlandı
- Stream goroutine sızıntısı kapatıldı (FAZ 6 sertleştirme kalemi): anthropic/google/ollama/openai_compat connector'larının kanal gönderimleri artık `sendChunk` (internal/llm/client.go) ile `ctx.Done()` select'i üzerinden korumalı; `Client.Stream`'in `releaseOnClose` forward goroutine'i de aynı deseni kullanıyor. Tüketici kanalı erken terk edip (Ctrl-C) ctx iptal edildiğinde üretici goroutine artık sonsuza kadar bloklanmıyor. Regresyon testleri (`-race` altında, `runtime.NumGoroutine()` baseline'a döndüğünü doğrular): `TestAnthropicStreamGoroutineExitsWhenContextCancelledWithoutDraining`, `TestGoogleStream...`, `TestOllamaStream...`, `TestOpenAICompatStream...`, `TestClientStreamGoroutineExitsWhenContextCancelledWithoutDraining` (internal/llm). Fix olmadan tüm 5 test 3sn timeout'ta fail ediyor — doğrulandı.
- Idle-timeout değerlendirildi ve uygulandı: yeni `llm.idle_timeout_seconds` config anahtarı (varsayılan `0` = kapalı, mevcut davranışla birebir aynı). Tek chunk arası boşluğu `Client.Stream`'in `releaseOnClose`'unda merkezi bir timer ile sınırlıyor — connector'ların kendi okuma döngülerine dokunmadan (temiz, katmanlı ek). `time.Timer` sıfırlama Go 1.25 (bu modülün sürümü) hedefiyle bilinçli olarak Stop()+drain YAPMIYOR: Go 1.23'ten beri Timer kanalı senkron (buffersız) ve çıplak `Reset` bayat değer sızdırmaz (`go doc time.Timer.Reset`); pre-1.23 Stop+drain deyimi burada TERSİNE tehlikeli olurdu — select aynı anda hem chunk hem timer ateşini görüp chunk dalını seçerse, Stop() false döner ama `<-idleTimer.C` üzerinde asla gelmeyecek bir değeri beklerdi (tam da bu değişikliğin kapatmaya çalıştığı sınıftan bir goroutine sızıntısı). Bu, review turunda gerçek bir bulgu olarak yakalandı ve düzeltildi; ilgili kod satırında kalıcı açıklayıcı yorum var (client.go). Yeni sentinel: `ErrIdleTimeout`. Tablo tabanlı testler: `TestIdleTimeoutDurationDisabledForNonPositive`, `TestClientStreamIdleTimeoutAbortsWhenGapBetweenChunksExceeded`, `TestClientStreamIdleTimeoutDisabledByDefaultAllowsSlowGaps` (internal/llm); config şema/validate testleri (internal/config). Belgelendi: docs/CONFIGURATION.md (TR+EN). Kullanıcı görünür yeni metin yok (internal/llm paketinin mevcut i18n istisnası kapsamında — Stream henüz hiçbir CLI/engine akışına bağlanmadığından hata TUI'ye yansımıyor).
- FAZ 10 kalemi (install.sh/install.ps1 arşiv adı drift testi): tamamlandı — `internal/cli/release_names_test.go` mevcut.
- FAZ 10 kalemi (wget fallback + önkoşul kontrolü): tamamlandı — `scripts/install.sh`'ta `require_downloader` (curl/wget sırayla arar, ikisi de yoksa açıklayıcı hata) + `fetch_url`/`fetch_url_to_file` (tek kod yolu, hangisi bulunduysa onu kullanır).
- Bulgu (bağımsız review) çözüldü: ask-modu onay satırı artık `general.language`'ı takip ediyor. `internal/tui/confirm.go`, `PromptChoice`/legend/edit-başlığı metnini `internal/i18n.Translator` üzerinden çözüyor (yeni `MsgConfirmLegend`/`MsgConfirmEditHeader` katalog anahtarları, TR+EN); `mapKey` artık `lang` parametresi alıyor ve kabul edilen tuş kümesi dile göre **kesinlikle ayrık** — birleşim (union) değil: TR `e`=evet vs EN `e`=edit, TR `a`=açıkla vs EN `a`=tümü çakışmasını önlemek için. `Confirm`/`newConfirmModel` imzalarına `tr i18n.Translator` eklendi; `internal/cli`'de `tuiPromptUI` (do.go/fix.go/chat.go, promptui.go) aynı `newTranslator(cfg)` zincirini enjekte ediyor — ayrı bir dil çözümü yok. `internal/cli/catalog_coverage_test.go`'daki i18n linter'ı `WriteString(...)` çağrılarını da taramaya genişletildi (`findRawWriteStringLiterals`) — önceden confirm.go'nun `strings.Builder.WriteString` kullanımı bu linter için kör noktaydı (yalnızca `fmt.Print*/Fprint*` görüyordu); genişletilmiş tarayıcı, eski (sabit-Türkçe) confirm.go içeriğine karşı çalıştırılıp 2 sabit literal yakaladığı, düzeltilmiş dosyaya karşı 0 bulduğu doğrulandı. Testler (internal/tui/confirm_test.go): `TestMapKeyTRLetters`, `TestMapKeyENLetters` (regresyon bekçileri: EN `e`→Edit [Yes DEĞİL], EN `a`→All [Explain DEĞİL]), `TestConfirmModelUpdateENPressEEntersEditNotYes`, `TestConfirmModelUpdateENPressAQuitsWithAllNotExplain`, `TestConfirmModelUpdateENTRLettersDoNothing`, `TestConfirmModelViewShowsTRLegend`/`ENLegend`, `TestConfirmModelViewShowsEditPromptWhileEditingTR`/`EN`, `TestConfirmRunsHeadlessProgramENPressAIsAllNotExplain` (tam bubbletea Program üzerinden uçtan uca). Gate: `go vet ./...`, `golangci-lint run`, `go test ./... -count=1`, `go test -race ./internal/tui/... ./internal/engine/...` temiz.

### Açık / bilgilendirici
- golangci-lint GPL-3.0 lisanslıdır; yalnızca ayrı süreç (CI/dev aracı) olarak çağrılır, koda gömülmez/vendorlanmaz.
- Manuel doğrulama: PowerShell hook'unun çalışma zamanı testi Windows ortamı gerektirir (golden testler mevcut); kullanıcı Windows'ta `comrade init powershell` ile doğrulamalı.
- Manuel doğrulama: FAZ 5 planlayıcının gerçek LLM ile "docker kur" kabul senaryosu API key gerektirir (httptest mock ile uçtan uca doğrulandı); kullanıcı gerçek key ile bir kez denemeli.
- Karar (kullanıcı onaylı): TUI için bubbletea v2 seçildi; Go tabanı 1.23 → 1.25'e yükseltildi (FAZ 0 kararı bilinçli tersine çevrildi). CLAUDE.md/go.mod güncellendi.
- Manuel doğrulama (Windows): internal/executor Windows dalında süreç-ağacı öldürme tek-süreç (torun süreçler kalabilir); pwsh yok, çalışma zamanı testi Windows'ta yapılmalı — FAZ 11 sertleştirme kalemi.
- Manuel doğrulama: comrade fix gerçek LLM ile "pyton --version" senaryosu API key gerektirir (mock ile uçtan uca doğrulandı); kullanıcı gerçek key ile bir kez denemeli.
- Manuel doğrulama: gerçek OS keychain (macOS Keychain / Windows Credential Manager / Linux Secret Service) ve gerçek TTY no-echo parola girişi bu ortamda test edilemedi (go-keyring mock + injectable reader ile doğrulandı); kullanıcı bir platformda comrade auth login ile doğrulamalı.
- i18n istisnaları (gerekçeli, docs/phases/FAZ-09.md'de belgeli — confirm.go artık bu listede DEĞİL, yukarıdaki "Tamamlandı" notuna bakın): cobra Use komut adları, hook.go gizli komut/debug satırı, promptui.go LLM prompt'u, ~40 "işlem: %w" hata sarmalama zinciri.
- FAZ 10 gerçek yayın için hazır ama henüz tag atılmadı: goreleaser snapshot tüm artefaktları üretiyor; gerçek release öncesi homebrew-tap / scoop-bucket / winget-pkgs hedef repoları ve tap PAT secret'ı oluşturulmalı.
- cosign imzalama .goreleaser.yaml'de yorumlu (etkinleştirme notu); anahtar/keyless kurulumu gerektirir.
- Manuel doğrulama: install.sh/install.ps1 ve comrade upgrade gerçek ağ yolu (GitHub Releases) canlı test edilmedi (mock/snapshot ile doğrulandı); ilk gerçek release'te doğrulanmalı.
- v0.1.0-rc1 hazır (tag atılmadı — kullanıcı kararı; tag push'u gerçek release pipeline'ını tetikler ve tap repoları/PAT gerekir). Soğuk başlangıç ~5ms, tüm gate + -race + govulncheck temiz. Bilinen kısıtlar KNOWN_LIMITATIONS.md'de.
