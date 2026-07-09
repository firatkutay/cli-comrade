package llm

import (
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// disableKeepAlives turns off HTTP keep-alives on client's transport, so a
// completed request's TCP connection — and the persistConn
// readLoop/writeLoop goroutines net/http keeps running for a pooled,
// reusable connection — tears down and exits promptly instead of
// lingering for reuse. Without this, a goroutine-count-based leak
// assertion would see those legitimate, unrelated keep-alive goroutines
// as a false leak. Test-only: production connectors always keep
// connections alive for reuse.
func disableKeepAlives(t *testing.T, client *http.Client) {
	t.Helper()
	tr, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "httptest.Server.Client() must return an *http.Transport")
	tr.DisableKeepAlives = true
}

// assertGoroutinesReturnToBaseline polls runtime.NumGoroutine() until it
// drops back to at most baseline, within a generous deadline, failing the
// test if it never does. It is the stdlib-only (no goleak dependency)
// regression check shared by every "abandoned stream channel must not leak
// its producer goroutine" test in this package — see the ctx.Done() guard
// in sendChunk and releaseOnClose (client.go) that this proves.
//
// A short poll loop rather than a single snapshot comparison is required
// because goroutine teardown (the transport noticing ctx cancellation,
// closing the connection, the producer's deferred close(ch)/Body.Close())
// is asynchronous — it does not complete in the same instant cancel() is
// called.
func assertGoroutinesReturnToBaseline(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last int
	for {
		last = runtime.NumGoroutine()
		if last <= baseline {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: NumGoroutine()=%d, want <= baseline %d", last, baseline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
