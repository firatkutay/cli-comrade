package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractJSONPlainObject(t *testing.T) {
	doc, err := ExtractJSON(`{"a":1,"b":"two"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1,"b":"two"}`, string(doc))
}

func TestExtractJSONFencedWithLanguageTag(t *testing.T) {
	doc, err := ExtractJSON("```json\n{\"a\":1}\n```")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(doc))
}

func TestExtractJSONFencedWithoutLanguageTag(t *testing.T) {
	doc, err := ExtractJSON("```\n{\"a\":1}\n```")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(doc))
}

func TestExtractJSONWithLeadingProse(t *testing.T) {
	doc, err := ExtractJSON("Sure, here is the plan:\n{\"a\":1}")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(doc))
}

func TestExtractJSONWithLeadingProseAndTrailingFence(t *testing.T) {
	doc, err := ExtractJSON("```json\nHere you go:\n{\"a\":1}\n```")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(doc))
}

func TestExtractJSONNestedBracesAndStrings(t *testing.T) {
	raw := `{"a": {"nested": "value with } brace and \"quote\""}, "b": [1,2,3]}`
	doc, err := ExtractJSON(raw)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(doc))
}

func TestExtractJSONTwoObjectsErrors(t *testing.T) {
	_, err := ExtractJSON(`{"a":1} {"b":2}`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "multiple top-level JSON objects")
}

func TestExtractJSONBrokenJSONErrors(t *testing.T) {
	_, err := ExtractJSON(`{"a": }`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid JSON")
}

func TestExtractJSONNoObjectErrors(t *testing.T) {
	_, err := ExtractJSON("just some prose, no JSON here")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no JSON object found")
}

func TestExtractJSONUnterminatedErrors(t *testing.T) {
	_, err := ExtractJSON(`{"a": 1`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unterminated JSON object")
}

func TestValidateIntoAllRequiredFieldsPresent(t *testing.T) {
	var target struct {
		Command string `json:"command"`
		Risk    string `json:"risk"`
	}
	doc, err := ValidateInto(`{"command":"ls -la","risk":"read"}`, []string{"command", "risk"}, &target)
	require.NoError(t, err)
	assert.JSONEq(t, `{"command":"ls -la","risk":"read"}`, string(doc))
	assert.Equal(t, "ls -la", target.Command)
	assert.Equal(t, "read", target.Risk)
}

func TestValidateIntoMissingRequiredFieldErrors(t *testing.T) {
	_, err := ValidateInto(`{"command":"ls -la"}`, []string{"command", "risk"}, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, `missing required field "risk"`)
}

func TestValidateIntoEmptyStringRequiredFieldErrors(t *testing.T) {
	_, err := ValidateInto(`{"command":"","risk":"read"}`, []string{"command"}, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, `missing required field "command"`)
}

func TestValidateIntoNullRequiredFieldErrors(t *testing.T) {
	_, err := ValidateInto(`{"command":null}`, []string{"command"}, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, `missing required field "command"`)
}

func TestValidateIntoFalseBooleanFieldIsNotEmpty(t *testing.T) {
	_, err := ValidateInto(`{"destructive":false}`, []string{"destructive"}, nil)
	require.NoError(t, err, "false is a present value, not an empty one")
}

func TestValidateIntoZeroNumberFieldIsNotEmpty(t *testing.T) {
	_, err := ValidateInto(`{"count":0}`, []string{"count"}, nil)
	require.NoError(t, err, "0 is a present value, not an empty one")
}

func TestValidateIntoNoRequiredFieldsSkipsCheck(t *testing.T) {
	doc, err := ValidateInto(`{"anything":"goes"}`, nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"anything":"goes"}`, string(doc))
}

func TestValidateIntoPropagatesExtractionFailure(t *testing.T) {
	_, err := ValidateInto("no json here", []string{"command"}, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "no JSON object found")
}
