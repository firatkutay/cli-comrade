package llm

import "time"

// UsageEvent is what a WithUsageObserver callback receives once per
// SUCCESSFUL Client.Complete attempt — see Client.fireUsage, the single
// call site, at the exact point Complete already measures latency and
// logs the attempt (logAttempt). Provider/Model attribute the event to a
// specific pricing.go row (see EstimateUSD); Usage is the connector's own
// reported token accounting, copied from CompletionResponse.Usage
// unchanged.
type UsageEvent struct {
	Provider string
	Model    string
	// BaseURL is the exact endpoint this attempt's connector talked to —
	// populated only for openai_compat (empty for every other provider,
	// which have no equivalent ambiguity); EstimateUSD's own base_url
	// gate is this field's one consumer (see pricing.go's doc comment on
	// why a same-named model served through a non-OpenAI gateway must
	// never reuse OpenAI's own price row).
	BaseURL string
	Usage   Usage
	Latency time.Duration
}

// WithUsageObserver registers fn to be invoked once per successful
// Complete attempt, symmetric with WithKeyResolver (client.go). A failed
// attempt never invokes fn — see Client.fireUsage — since it carries no
// usage to report.
//
// fn runs SYNCHRONOUSLY on Complete's own call path, before Complete
// returns to its caller: it MUST be cheap and non-blocking (e.g. append
// to an in-memory accumulator under a mutex, as internal/cli's
// usageTally does), never perform its own I/O — a slow or blocking
// observer directly adds to every completion's observed latency. A nil
// fn is ignored (mirrors WithKeyResolver's own nil handling), so a
// caller can pass a possibly-nil observer through without an extra
// branch.
func WithUsageObserver(fn func(UsageEvent)) Option {
	return func(o *clientOptions) {
		if fn != nil {
			o.usageObserver = fn
		}
	}
}
