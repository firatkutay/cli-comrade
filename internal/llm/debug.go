package llm

import (
	"fmt"
	"os"
	"time"
)

// debugEnabled reports whether COMRADE_DEBUG=1 is set, gating the
// fallback-chain attempt log required by docs/history/UYGULAMA_PLANI.md FAZ 2 item 4.
func debugEnabled() bool {
	return os.Getenv("COMRADE_DEBUG") == "1"
}

// logAttempt writes one line to stderr per fallback-chain attempt when
// COMRADE_DEBUG=1: provider name, model (blank when the attempt failed
// before a model could be attributed, e.g. a key-resolution failure),
// the error classification (or "ok"), and latency. This is the only
// place in this package that writes to stderr.
func logAttempt(provider, model, class string, latency time.Duration) {
	if !debugEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "[llm] provider=%s model=%s result=%s latency=%s\n",
		provider, model, class, latency.Round(time.Millisecond))
}
