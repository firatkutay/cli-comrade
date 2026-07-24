package doctor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverityStringIsStableLowercaseVocabulary(t *testing.T) {
	assert.Equal(t, "ok", SeverityOK.String())
	assert.Equal(t, "warn", SeverityWarn.String())
	assert.Equal(t, "fail", SeverityFail.String())
	assert.Equal(t, "skip", SeveritySkip.String())
	assert.Equal(t, "unknown", Severity(99).String())
}

func TestNewRunnerRegistersEveryCheckInCanonicalOrder(t *testing.T) {
	runner := NewRunner()
	ids := make([]string, 0, len(runner.Checks))
	for _, c := range runner.Checks {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"version", "path", "shellhook", "key", "reach", "baseurl", "config"}, ids)
}

// TestRunnerRunPreservesRegistryOrderRegardlessOfCompletionOrder proves
// Run's own documented guarantee: results come back in r.Checks' fixed
// registry order, never completion order — a deliberately slow FIRST
// check and a deliberately instant LAST check must not reorder the
// output.
func TestRunnerRunPreservesRegistryOrderRegardlessOfCompletionOrder(t *testing.T) {
	runner := Runner{Checks: []Check{
		{ID: "slow", Run: func(context.Context, Deps) Result {
			time.Sleep(20 * time.Millisecond)
			return Result{Severity: SeverityOK}
		}},
		{ID: "fast", Run: func(context.Context, Deps) Result {
			return Result{Severity: SeverityOK}
		}},
	}}

	results := runner.Run(context.Background(), Deps{})

	require.Len(t, results, 2)
	assert.Equal(t, "slow", results[0].ID)
	assert.Equal(t, "fast", results[1].ID)
}

// TestRunnerRunSetsResultIDFromCheckRegardlessOfWhatCheckItselfSet proves
// Run is the single source of truth for Result.ID — an individual
// Check.Run function never needs to (and, per every check_*.go file in
// this package, never does) set its own Result.ID.
func TestRunnerRunSetsResultIDFromCheckRegardlessOfWhatCheckItselfSet(t *testing.T) {
	runner := Runner{Checks: []Check{
		{ID: "real-id", Run: func(context.Context, Deps) Result {
			return Result{ID: "wrong-id-the-check-should-not-set", Severity: SeverityOK}
		}},
	}}

	results := runner.Run(context.Background(), Deps{})

	require.Len(t, results, 1)
	assert.Equal(t, "real-id", results[0].ID)
}

// TestRunnerRunBoundsEachCheckWithATimeoutIndependentOfCallerContext
// proves a check that ignores cancellation forever does not hang the
// whole Run call forever — each check gets its own checkTimeout-bounded
// context, derived from (but not identical to) the caller's ctx.
func TestRunnerRunBoundsEachCheckWithATimeoutIndependentOfCallerContext(t *testing.T) {
	runner := Runner{Checks: []Check{
		{ID: "respects-timeout", Run: func(ctx context.Context, _ Deps) Result {
			<-ctx.Done()
			return Result{Severity: SeverityFail}
		}},
	}}

	done := make(chan []Result, 1)
	go func() { done <- runner.Run(context.Background(), Deps{}) }()

	select {
	case results := <-done:
		require.Len(t, results, 1)
		assert.Equal(t, SeverityFail, results[0].Severity)
	case <-time.After(checkTimeout + 2*time.Second):
		t.Fatal("Run did not return within checkTimeout + slack; a check's own timeout context is not being honored")
	}
}
