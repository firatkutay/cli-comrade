// Package engine turns a natural-language request plus a collected
// internal/context.Context into a risk-labeled, step-by-step Plan
// (CLAUDE.md "Temel Kullanım Senaryoları" #2). Planner.GeneratePlan builds
// the go:embed'd system prompt (prompts/plan_system.txt, plus
// prompts/plan_lang_tr.txt when the resolved language is Turkish),
// requests a single structured-JSON completion through a Completer (the
// package's own minimal consumer-side interface — any *llm.Client
// satisfies it), and runs every returned step through
// internal/safety.Engine so the LLM's declared risk label is never the
// last word. This package performs no execution: that is FAZ 6's
// internal/executor. Import direction is one-way — engine depends on
// llm/safety/context/config; none of those import engine back.
package engine
