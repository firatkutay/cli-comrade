package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractJSON extracts the single top-level JSON object from raw model
// output. It strips a leading/trailing markdown code fence (``` or
// ```json, with or without a language tag) if present, tolerates leading
// prose before the object, and locates exactly one balanced top-level
// JSON object in what remains. A second top-level object appearing
// anywhere after the first is rejected as ambiguous, per
// docs/history/UYGULAMA_PLANI.md FAZ 2 item 3.
func ExtractJSON(raw string) (json.RawMessage, error) {
	text := stripFence(raw)

	start := strings.IndexByte(text, '{')
	if start == -1 {
		return nil, fmt.Errorf("llm: no JSON object found in response")
	}

	end, err := matchBrace(text, start)
	if err != nil {
		return nil, err
	}
	candidate := text[start : end+1]

	if strings.IndexByte(text[end+1:], '{') != -1 {
		return nil, fmt.Errorf("llm: multiple top-level JSON objects found in response")
	}

	var probe any
	if err := json.Unmarshal([]byte(candidate), &probe); err != nil {
		return nil, fmt.Errorf("llm: invalid JSON in response: %w", err)
	}

	return json.RawMessage(candidate), nil
}

// stripFence removes a single wrapping markdown code fence from raw, if
// present: an opening ``` (optionally followed by a language tag such as
// "json" on the same line) and, if found, a matching trailing ``` fence.
// raw is returned trimmed but otherwise unchanged when it does not start
// with a fence.
func stripFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	firstNewline := strings.IndexByte(text, '\n')
	if firstNewline == -1 {
		// A lone "```..." line with nothing after it: nothing to extract.
		return text
	}
	text = strings.TrimSpace(text[firstNewline+1:])

	if idx := strings.LastIndex(text, "```"); idx != -1 {
		text = strings.TrimSpace(text[:idx])
	}
	return text
}

// matchBrace returns the index of the '}' that closes the '{' at s[start],
// respecting JSON string literals (so a brace inside a quoted string
// never affects depth). It errors if the object is never closed.
func matchBrace(s string, start int) (int, error) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("llm: unterminated JSON object in response")
}

// ValidateInto extracts the single top-level JSON object from raw (via
// ExtractJSON), optionally unmarshals it into target (skipped when target
// is nil), and verifies that every entry in requiredFields names a
// top-level JSON key present in the object with a non-empty/non-zero
// value. requiredFields uses the JSON object's own key names, not Go
// struct field names; dotted/nested paths are not supported. It returns
// the extracted json.RawMessage on success.
func ValidateInto(raw string, requiredFields []string, target any) (json.RawMessage, error) {
	doc, err := ExtractJSON(raw)
	if err != nil {
		return nil, err
	}

	if target != nil {
		if err := json.Unmarshal(doc, target); err != nil {
			return nil, fmt.Errorf("llm: response JSON does not match expected shape: %w", err)
		}
	}

	if len(requiredFields) == 0 {
		return doc, nil
	}

	var generic map[string]any
	if err := json.Unmarshal(doc, &generic); err != nil {
		return nil, fmt.Errorf("llm: response is not a JSON object: %w", err)
	}

	for _, field := range requiredFields {
		value, ok := generic[field]
		if !ok || isEmptyJSONValue(value) {
			return nil, fmt.Errorf("llm: response missing required field %q", field)
		}
	}

	return doc, nil
}

// isEmptyJSONValue reports whether a json.Unmarshal-decoded value (into
// map[string]any) counts as "empty" for ValidateInto's required-field
// check: nil (JSON null or absent), an empty string, or an empty
// array/object. Numbers and booleans (including false and zero) are never
// considered empty — their presence is what's being checked, not their
// truthiness.
func isEmptyJSONValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}
