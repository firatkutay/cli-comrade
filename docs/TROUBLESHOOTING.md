# Sorun Giderme / Troubleshooting

## Türkçe

### "no API key found for provider ..."

Aktif `llm.provider` için ne keychain'de/dosya yedeğinde ne de ilgili
ortam değişkeninde bir anahtar bulunamadı. Çözüm:

```sh
comrade auth login <provider>   # örn: comrade auth login anthropic
```

veya ilgili ortam değişkenini ayarlayın (bkz. CONFIGURATION.md'nin
"API anahtarları" tablosu — örn. `ANTHROPIC_API_KEY`). Hangi
sağlayıcıların bir anahtarı olduğunu görmek için: `comrade auth
status`.

### Ollama çalışmıyor / bağlanamıyor

`llm.provider = "ollama"` iken `llm.ollama.base_url`'de (varsayılan
`http://localhost:11434`) bir Ollama sunucusu çalışıyor olmalı.
`ollama serve` ile başlatın veya `comrade config models` ile model
listesini çekmeyi deneyip bağlantıyı doğrulayın. Ollama kurulu değilse
[ollama.com](https://ollama.com)'dan kurun; comrade'in kendisi Ollama'yı
kurmaz.

### Shell kancası (hook) tetiklenmiyor / `comrade fix` her zaman yapıştırma moduna düşüyor

1. `comrade init` çalıştırıldı mı ve bir sonraki adımda **yeni bir
   shell açtınız mı** (kanca mevcut oturuma değil, yeni açılan
   kabuklara uygulanır)? `comrade init --print` ile hangi bloğun
   eklenmesi gerektiğini görebilirsiniz.
2. bash/zsh: kanca `PROMPT_COMMAND`/`precmd`'e eklenir; rc dosyanızda
   (`.bashrc`/`.zshrc`) başka bir araç `PROMPT_COMMAND`'ı komple
   ÜZERİNE YAZIYORSA (append değil) comrade'in kancası kaybolur —
   rc dosyasında comrade bloğunun EN SONDA olduğundan emin olun.
3. fish: `fish_postexec` event'i; başka bir eklenti aynı event'e
   bağlıysa ikisi de tetiklenmeli (fish, aynı event'e birden fazla
   handler'a izin verir) — yine de bir çakışma şüpheleniyorsanız
   `functions --details fish_postexec` ile kontrol edin.
4. PowerShell: kanca `$PROFILE`'a bir `prompt` fonksiyonu ekler; başka
   bir araç (örn. oh-my-posh, starship) `$PROFILE`'da comrade'den SONRA
   kendi `prompt` fonksiyonunu tanımlıyorsa comrade'inkini geçersiz
   kılar — comrade bloğunun diğer prompt-özelleştirme araçlarından
   SONRA gelmesi gerekir (`comrade init powershell` zaten dosyanın
   sonuna ekler; elle taşımayın).
5. Kanca hiç kurulamıyorsa (ör. betikli olmayan bir ortam): `comrade
   fix -- <komut>` ile komutu comrade'in kendisi çalıştırıp
   gözlemlemesini sağlayın, ya da hatayı doğrudan yapıştırın (paste
   modu her zaman çalışır, kanca gerektirmez).

### Windows: "çalıştırılamıyor çünkü bu sistemde betik çalıştırma devre dışı bırakılmış" (ExecutionPolicy)

`comrade init powershell` `$PROFILE`'a yazar, ama PowerShell'in
**betik çalıştırma politikası** varsayılan olarak (`Restricted`)
profil dosyalarının hiç yüklenmesine izin vermeyebilir — bu comrade'e
özgü değil, her PowerShell profili için geçerli bir Windows
davranışıdır. Çözüm (yönetici olmayan, kullanıcı kapsamında):

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

### `comrade: command not found` / kurulum sonrası PATH'e eklenmiyor

`install.sh`/`install.ps1` kurulum dizinini PATH'te bulamazsa artık
**otomatik olarak** kalıcı hale getirir: `install.ps1`
`[Environment]::SetEnvironmentVariable` ile User PATH'ine ekler (yeni
bir terminal gerekir); `install.sh` kabuğunuza uygun rc dosyasına
(bash → `~/.bashrc`, zsh → `~/.zshrc`, fish →
`~/.config/fish/config.fish`, diğerleri → `~/.profile`) bir PATH
export satırı ekler ve ardından kabuğunuzu yeniden başlatmanızı ya da
ekrana yazdırılan `export ...` komutunu doğrudan çalıştırmanızı ister.

Yine de `comrade` bulunamıyorsa, olası nedenler:

- **`COMRADE_NO_MODIFY_PATH` ayarlıydı** — bu durumda `install.sh`
  rc dosyanızı hiç düzenlemez, sadece bir not basar; PATH'e elle
  eklemeniz gerekir.
- **rc dosyası yazılamadı** (dizin yoktu/izin yoktu) — script bu
  durumda da sessizce eski uyarı-yazdırma davranışına döner.
- **Kabuğu yeniden başlatmadınız / rc dosyasını `source` etmediniz** —
  export satırı eklenmiş olsa bile mevcut oturuma etki etmez.

Herhangi bir durumda, elle eklemek için:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

ve kalıcı olması için bu satırı ilgili rc dosyanıza ekleyip yeni bir
kabuk açın (ya da `source ~/.bashrc` gibi dosyayı yeniden yükleyin).

### `comrade upgrade` "bu bir geliştirme (dev) derlemesi" diyor

`go build`/`go install` ile (bir `-ldflags -X main.version=...`
olmadan) yaptığınız yerel bir derlemeyi çalıştırıyorsunuz — bu
derlemeler `dev` sürüm dizesini taşır ve karşılaştırılacak bir release
etiketi yoktur. Yalnızca yukarıdaki resmi paket
yöneticilerinden/scriptlerinden kurulan sürümler `comrade upgrade`'i
destekler.

### Checksum doğrulaması başarısız oluyor (kurulum veya `comrade upgrade`)

Bu ASLA yok sayılmaması gereken bir güvenlik sinyalidir — kurulum/
güncelleme otomatik olarak iptal edilir. Genellikle geçici bir ağ/CDN
sorunudur (yarım inen dosya); tekrar deneyin. Israrla tekrarlıyorsa
[Releases](https://github.com/firatkutay/cli-comrade/releases)
sayfasından o sürümün `checksums.txt`'ini elle indirip karşılaştırın
ve bir issue açın.

`comrade upgrade` ayrıca checksum'dan önce bir cosign imza doğrulaması
yapar; imza geçersizse güncelleme (checksum'a bakılmaksızın) durdurulur
— bu da kasıtlı, atlanamayan bir güvenlik davranışıdır. Ayrıntılar için
bkz. [`docs/SECURITY.md`](SECURITY.md) ve
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md).

---

## English

### "no API key found for provider ..."

No credential was found for the active `llm.provider` — not in the
keychain/file fallback, not in the matching environment variable. Fix:

```sh
comrade auth login <provider>   # e.g. comrade auth login anthropic
```

or set the matching environment variable (see CONFIGURATION.md's "API
keys" table — e.g. `ANTHROPIC_API_KEY`). See which providers already
have a key with `comrade auth status`.

### Ollama isn't running / can't connect

With `llm.provider = "ollama"`, an Ollama server must actually be
running at `llm.ollama.base_url` (default `http://localhost:11434`).
Start it with `ollama serve`, or use `comrade config models` to fetch
the model list as a connectivity check. If Ollama isn't installed at
all, get it from [ollama.com](https://ollama.com) — comrade itself
does not install Ollama for you.

### The shell hook never fires / `comrade fix` always falls back to paste mode

1. Did you run `comrade init`, and did you **open a new shell**
   afterward (the hook applies to newly started shells, not the
   current session)? `comrade init --print` shows exactly what block
   should be installed.
2. bash/zsh: the hook is added to `PROMPT_COMMAND`/`precmd`; if
   another tool in your rc file (`.bashrc`/`.zshrc`) OVERWRITES
   `PROMPT_COMMAND` entirely (instead of appending) AFTER comrade's
   block, comrade's hook is lost — make sure comrade's block is the
   LAST thing that touches it.
3. fish: uses the `fish_postexec` event; fish allows multiple handlers
   on the same event, so another plugin using it should not conflict —
   if you suspect it does anyway, check with `functions --details
   fish_postexec`.
4. PowerShell: the hook adds a `prompt` function to `$PROFILE`; if
   another tool (e.g. oh-my-posh, starship) defines its OWN `prompt`
   function AFTER comrade's in `$PROFILE`, it overrides comrade's —
   comrade's block must come AFTER other prompt-customization tools
   (`comrade init powershell` already appends to the end of the file;
   don't manually reorder it).
5. If the hook can't be installed at all (e.g. a non-interactive
   environment): use `comrade fix -- <command>` to have comrade itself
   run and observe the command, or just paste the error directly
   (paste mode always works, no hook required).

### Windows: "cannot be loaded because running scripts is disabled on this system" (ExecutionPolicy)

`comrade init powershell` writes to `$PROFILE`, but PowerShell's
**script execution policy** defaults to `Restricted`, which can
prevent profile files from loading at all — this is standard Windows
behavior for any PowerShell profile, not specific to comrade. Fix
(no admin rights needed, user-scoped):

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

### `comrade: command not found` / PATH isn't updated after installing

If `install.sh`/`install.ps1` can't find the install directory on
PATH, it now handles it **automatically**: `install.ps1` adds it to
your User PATH via `[Environment]::SetEnvironmentVariable` (a new
terminal is required to pick it up); `install.sh` appends a
shell-appropriate PATH export line to your rc file (bash →
`~/.bashrc`, zsh → `~/.zshrc`, fish → `~/.config/fish/config.fish`,
anything else → `~/.profile`), then tells you to restart your shell or
run the printed `export ...` command directly.

If `comrade` still isn't found, likely causes:

- **`COMRADE_NO_MODIFY_PATH` was set** — `install.sh` then never
  edits your rc file automatically; it only prints a note, and you
  need to add the install directory to PATH yourself.
- **The rc file couldn't be written** (missing directory / no write
  permission) — the script silently falls back to the same
  print-only-note behavior in that case.
- **You haven't restarted your shell / sourced the rc file** — the
  export line may already be there but not yet loaded into your
  current session.

Either way, you can add it by hand:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

and add that line to your shell's rc file for it to persist, then open
a new shell (or `source` the rc file, e.g. `source ~/.bashrc`).

### `comrade upgrade` says "this is a dev build"

You're running a local build made with `go build`/`go install`
(without a `-ldflags -X main.version=...`) — these builds carry the
literal version string `dev` and have no release tag to compare
against. Only versions installed via the official package
managers/scripts above support `comrade upgrade`.

### Checksum verification fails (during install or `comrade upgrade`)

This is a security signal that must never be bypassed — install/
upgrade aborts automatically. It's usually a transient network/CDN
issue (a partially downloaded file); retry. If it persists, manually
download that release's `checksums.txt` from the
[Releases page](https://github.com/firatkutay/cli-comrade/releases),
compare by hand, and open an issue.

`comrade upgrade` also verifies a cosign signature before it even gets
to the checksum — if the signature doesn't verify, the upgrade is
aborted regardless of the checksum, by design and non-bypassable. See
[`docs/SECURITY.md`](SECURITY.md) and
[`docs/UPDATE_SIGNING.md`](UPDATE_SIGNING.md) for details.
