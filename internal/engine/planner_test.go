package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// fakeResponse is one queued reply for fakeCompleter: either a raw model
// text (which fakeCompleter runs through llm.ValidateInto exactly like
// the real llm.Client does, so JSON extraction/validation behaves
// identically to production) or a hard error to return instead.
type fakeResponse struct {
	text string
	err  error
}

// fakeCompleter is engine.Completer's test double: a fixed, ordered queue
// of responses, one per call, recording every CompletionRequest it saw so
// tests can assert on the exact messages/system prompt Planner sent.
type fakeCompleter struct {
	responses []fakeResponse
	calls     []llm.CompletionRequest
}

func (f *fakeCompleter) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, req)
	if idx >= len(f.responses) {
		return llm.CompletionResponse{}, fmt.Errorf("fakeCompleter: no response queued for call #%d", idx)
	}

	r := f.responses[idx]
	if r.err != nil {
		return llm.CompletionResponse{}, r.err
	}

	doc, err := llm.ValidateInto(r.text, req.RequiredFields, nil)
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	return llm.CompletionResponse{Text: r.text, JSON: doc, Model: "fake-model"}, nil
}

func testSysCtx() contextpkg.Context {
	return contextpkg.Context{
		OS:              "linux",
		Shell:           "bash",
		WorkingDir:      "/home/user/project",
		PackageManagers: []string{"apt"},
		IsAdmin:         false,
		AdminKnown:      true,
	}
}

const validThreeStepPlanJSON = `{
  "summary": "Install docker and start it.",
  "steps": [
    {"command": "sudo apt-get install -y docker.io", "rationale": "Installs the docker package.", "risk": "elevated", "reversible": false},
    {"command": "sudo systemctl enable --now docker", "rationale": "Starts the docker service.", "risk": "elevated", "reversible": true},
    {"command": "rm -rf /", "rationale": "A dangerous decoy step the LLM should never emit.", "risk": "destructive", "reversible": false}
  ]
}`

func TestGeneratePlanHappyPathMapsStepsAndRunsSafetyEngine(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: validThreeStepPlanJSON}}}
	planner := NewPlanner(fake, config.Default())

	plan, err := planner.GeneratePlan(context.Background(), "docker kur", testSysCtx())
	require.NoError(t, err)

	assert.Equal(t, "Install docker and start it.", plan.Summary)
	require.Len(t, plan.Steps, 3)

	assert.Equal(t, "sudo apt-get install -y docker.io", plan.Steps[0].Command)
	assert.Equal(t, safety.RiskElevated, plan.Steps[0].Risk)
	assert.Equal(t, safety.Confirm, plan.Steps[0].Decision.Action, "elevated steps must Confirm")

	assert.Equal(t, safety.Confirm, plan.Steps[1].Decision.Action)

	// The third step is a denylisted rm -rf / decoy: the safety engine
	// must Block it regardless of what risk label the LLM attached.
	assert.Equal(t, safety.Block, plan.Steps[2].Decision.Action)
	assert.NotEmpty(t, plan.Steps[2].Decision.Reason)

	require.Len(t, fake.calls, 1)
	assert.Equal(t, []string{"summary"}, fake.calls[0].RequiredFields)
}

func TestGeneratePlanEmptyStepsTriggersOneCorrectiveReprompt(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{
		{text: `{"summary": "nothing to do", "steps": []}`},
		{text: `{"summary": "ok now", "steps": [{"command": "ls", "rationale": "lists files", "risk": "read", "reversible": true}]}`},
	}}
	planner := NewPlanner(fake, config.Default())

	plan, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
	require.NoError(t, err)
	assert.Equal(t, "ok now", plan.Summary)
	require.Len(t, plan.Steps, 1)

	require.Len(t, fake.calls, 2)
	require.Len(t, fake.calls[1].Messages, 2)
	assert.Equal(t, emptyStepsCorrection, fake.calls[1].Messages[1].Content)
}

func TestGeneratePlanEmptyStepsTwiceIsAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{
		{text: `{"summary": "nothing", "steps": []}`},
		{text: `{"summary": "still nothing", "steps": []}`},
	}}
	planner := NewPlanner(fake, config.Default())

	_, err := planner.GeneratePlan(context.Background(), "do something impossible", testSysCtx())
	assert.Error(t, err)
	assert.Len(t, fake.calls, 2)
}

func TestGeneratePlanTruncatesAfterFailedConsolidateReprompt(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.MaxAutoSteps = 2

	tooMany := `{"summary": "many steps", "steps": [
		{"command": "echo one", "rationale": "r1", "risk": "read", "reversible": true},
		{"command": "echo two", "rationale": "r2", "risk": "read", "reversible": true},
		{"command": "echo three", "rationale": "r3", "risk": "read", "reversible": true}
	]}`
	fake := &fakeCompleter{responses: []fakeResponse{
		{text: tooMany},
		{err: fmt.Errorf("boom: consolidate retry failed")},
	}}
	planner := NewPlanner(fake, cfg)

	plan, err := planner.GeneratePlan(context.Background(), "do three things", testSysCtx())
	require.NoError(t, err)
	require.Len(t, plan.Steps, 2, "must hard-truncate to max_auto_steps after the retry itself failed")
	assert.Contains(t, plan.Summary, "truncated")

	require.Len(t, fake.calls, 2)
	assert.Contains(t, fake.calls[1].Messages[1].Content, "2 steps")
}

func TestGeneratePlanConsolidateRepromptSucceeding(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.MaxAutoSteps = 2

	tooMany := `{"summary": "many steps", "steps": [
		{"command": "echo one", "rationale": "r1", "risk": "read", "reversible": true},
		{"command": "echo two", "rationale": "r2", "risk": "read", "reversible": true},
		{"command": "echo three", "rationale": "r3", "risk": "read", "reversible": true}
	]}`
	consolidated := `{"summary": "two steps now", "steps": [
		{"command": "echo one", "rationale": "r1", "risk": "read", "reversible": true},
		{"command": "echo two-three", "rationale": "r2+r3", "risk": "read", "reversible": true}
	]}`
	fake := &fakeCompleter{responses: []fakeResponse{{text: tooMany}, {text: consolidated}}}
	planner := NewPlanner(fake, cfg)

	plan, err := planner.GeneratePlan(context.Background(), "do three things", testSysCtx())
	require.NoError(t, err)
	require.Len(t, plan.Steps, 2)
	assert.Equal(t, "two steps now", plan.Summary, "must not carry a truncation marker when the retry itself already fit")
}

func TestGeneratePlanUnknownRiskLabelFailsClosedToDestructive(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{
		{text: `{"summary": "does a thing", "steps": [{"command": "ls", "rationale": "lists files", "risk": "catastrophic", "reversible": true}]}`},
	}}
	planner := NewPlanner(fake, config.Default())

	plan, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
	require.NoError(t, err)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, safety.RiskDestructive, plan.Steps[0].Risk)
	assert.Contains(t, plan.Summary, "unrecognized risk label")
}

func TestGeneratePlanEmptyCommandIsAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{
		{text: `{"summary": "does a thing", "steps": [{"command": "", "rationale": "oops", "risk": "read", "reversible": true}]}`},
	}}
	planner := NewPlanner(fake, config.Default())

	_, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
	assert.Error(t, err)
}

func TestGeneratePlanPropagatesCompleteError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{err: fmt.Errorf("network unreachable")}}}
	planner := NewPlanner(fake, config.Default())

	_, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network unreachable")
}

func TestGeneratePlanSystemPromptLanguage(t *testing.T) {
	oneStep := `{"summary": "ok", "steps": [{"command": "ls", "rationale": "r", "risk": "read", "reversible": true}]}`

	t.Run("explicit tr", func(t *testing.T) {
		cfg := config.Default()
		cfg.General.Language = "tr"
		fake := &fakeCompleter{responses: []fakeResponse{{text: oneStep}}}
		planner := NewPlanner(fake, cfg)

		_, err := planner.GeneratePlan(context.Background(), "dosyaları listele", testSysCtx())
		require.NoError(t, err)
		assert.Contains(t, fake.calls[0].System, "TÜRKÇE")
	})

	t.Run("explicit en", func(t *testing.T) {
		cfg := config.Default()
		cfg.General.Language = "en"
		fake := &fakeCompleter{responses: []fakeResponse{{text: oneStep}}}
		planner := NewPlanner(fake, cfg)

		_, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
		require.NoError(t, err)
		assert.NotContains(t, fake.calls[0].System, "TÜRKÇE")
	})

	t.Run("auto resolves to tr via LANG", func(t *testing.T) {
		cfg := config.Default() // general.language defaults to "auto"
		fake := &fakeCompleter{responses: []fakeResponse{{text: oneStep}}}
		planner := NewPlanner(fake, cfg)
		planner.getenv = func(name string) string {
			if name == "LANG" {
				return "tr_TR.UTF-8"
			}
			return ""
		}

		_, err := planner.GeneratePlan(context.Background(), "dosyaları listele", testSysCtx())
		require.NoError(t, err)
		assert.Contains(t, fake.calls[0].System, "TÜRKÇE")
	})

	t.Run("auto resolves to en without a tr LANG", func(t *testing.T) {
		cfg := config.Default()
		fake := &fakeCompleter{responses: []fakeResponse{{text: oneStep}}}
		planner := NewPlanner(fake, cfg)
		planner.getenv = func(string) string { return "" }

		_, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
		require.NoError(t, err)
		assert.NotContains(t, fake.calls[0].System, "TÜRKÇE")
	})
}

func TestGeneratePlanSystemPromptCarriesSystemContext(t *testing.T) {
	oneStep := `{"summary": "ok", "steps": [{"command": "ls", "rationale": "r", "risk": "read", "reversible": true}]}`
	fake := &fakeCompleter{responses: []fakeResponse{{text: oneStep}}}
	planner := NewPlanner(fake, config.Default())

	_, err := planner.GeneratePlan(context.Background(), "list files", testSysCtx())
	require.NoError(t, err)

	sys := fake.calls[0].System
	assert.Contains(t, sys, "OS: linux")
	assert.Contains(t, sys, "Shell: bash")
	assert.Contains(t, sys, "/home/user/project")
	assert.Contains(t, sys, "apt")
	assert.NotContains(t, sys, "SECRET_ENV_VALUE", "the context block must never carry env var values")
}

// Language-resolution precedence itself is now internal/i18n's own,
// consolidated concern (i18n.ResolveLanguage — see
// internal/i18n/lang_test.go's TestResolveLanguagePrecedence for the full
// precedence table, including config/COMRADE_LANG/LANG/LC_ALL). What
// remains here (TestBuildSystemPromptIncludesGroundingContext above) is
// GeneratePlan's own concern: that the resolved lang value actually
// selects the right prompt block.
