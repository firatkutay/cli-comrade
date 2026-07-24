package cli

import (
	"context"
	"time"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// pingProviderWithKey sends a minimal completion through a Client scoped
// to exactly one attempt — provider, using key directly (bypassing the
// stored-credential resolver entirely) — reusing cfg's other effective
// settings (base_url, timeout) so e.g. an openai_compat ping against a
// non-OpenAI endpoint pings the right place.
//
// This is `comrade auth login`'s own ping (see pingProvider, auth.go)
// with its config-loading/*cobra.Command dependency stripped out, so
// `comrade doctor --live` (internal/doctor.Deps.LivePing, wired by
// internal/cli/doctor.go) can share this EXACT same hardened path instead
// of a second, parallel implementation — internal/doctor itself never
// imports internal/cli (that would be a cycle: this package already
// imports internal/doctor for the check registry), so it can only ever
// receive this as an injected func value, never call it directly.
func pingProviderWithKey(ctx context.Context, cfg config.Config, provider, key string) (llm.CompletionResponse, time.Duration, error) {
	cfg.LLM.Fallback = nil
	if cfg.LLM.Provider != provider {
		cfg.LLM.Provider = provider
		cfg.LLM.Model = ""
	}

	client, err := llm.New(cfg, llm.WithKeyResolver(func(string) (string, error) { return key, nil }))
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}

	start := time.Now()
	resp, err := client.Complete(ctx, llm.CompletionRequest{
		Messages:  []llm.Message{{Role: "user", Content: "ping"}},
		MaxTokens: 16,
	})
	return resp, time.Since(start), err
}
