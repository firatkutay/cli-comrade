package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
)

const rmRfExplanationJSON = `{
  "summary": "Deletes the node_modules directory and everything inside it, without asking for confirmation.",
  "parts": [
    {"token": "rm", "meaning": "Removes files or directories."},
    {"token": "-r", "meaning": "Removes directories and everything inside them, recursively."},
    {"token": "-f", "meaning": "Never asks for confirmation and ignores missing files."},
    {"token": "node_modules", "meaning": "The directory this command deletes."}
  ],
  "risk_note": "This permanently deletes node_modules and everything in it; it cannot be undone, though it is usually safe to regenerate by reinstalling dependencies."
}`

func TestExplainerHappyPathParsesPartsAndSendsCommandInUserMessage(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: rmRfExplanationJSON}}}
	explainer := NewExplainer(fake, config.Default())

	explanation, err := explainer.Explain(context.Background(), "rm -rf node_modules")
	require.NoError(t, err)

	assert.Contains(t, explanation.Summary, "Deletes the node_modules directory")
	require.Len(t, explanation.Parts, 4)
	assert.Equal(t, ExplanationPart{Token: "rm", Meaning: "Removes files or directories."}, explanation.Parts[0])
	assert.Equal(t, ExplanationPart{Token: "-r", Meaning: "Removes directories and everything inside them, recursively."}, explanation.Parts[1])
	assert.Contains(t, explanation.RiskNote, "permanently deletes")

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].Messages[0].Content, "rm -rf node_modules")
	assert.Equal(t, []string{"summary"}, fake.calls[0].RequiredFields)
}

func TestExplainerNeverCallsAnythingButComplete(t *testing.T) {
	// Explainer's only dependency is the Completer interface — there is no
	// executor, no safety.Engine field on Explainer at all, so it is
	// structurally impossible for Explain to run the command it was asked
	// to explain. This test documents that guarantee by construction: if a
	// future change ever added an execution path, it would have to change
	// Explainer's fields (reviewable), not just its Explain method body.
	fake := &fakeCompleter{responses: []fakeResponse{{text: rmRfExplanationJSON}}}
	explainer := NewExplainer(fake, config.Default())

	_, err := explainer.Explain(context.Background(), "rm -rf /")
	require.NoError(t, err)
	assert.Len(t, fake.calls, 1, "Explain must make exactly one LLM call and touch nothing else")
}

func TestExplainerRequestsTurkishInstructionBlockWhenConfigured(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: rmRfExplanationJSON}}}
	cfg := config.Default()
	cfg.General.Language = "tr"
	explainer := NewExplainer(fake, cfg)

	_, err := explainer.Explain(context.Background(), "rm -rf node_modules")
	require.NoError(t, err)

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].System, "TÜRKÇE",
		"a tr-resolved language must append the Turkish instruction block to the system prompt")
}

func TestExplainerOmitsEmptyPartsEntries(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{"summary": "does nothing notable", "parts": [{"token":"","meaning":""}], "risk_note": ""}`}}}
	explainer := NewExplainer(fake, config.Default())

	explanation, err := explainer.Explain(context.Background(), "true")
	require.NoError(t, err)
	assert.Empty(t, explanation.Parts)
}

func TestExplainerPropagatesCompleterError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{err: assert.AnError}}}
	explainer := NewExplainer(fake, config.Default())

	_, err := explainer.Explain(context.Background(), "ls")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}
