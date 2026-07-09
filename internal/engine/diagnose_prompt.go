package engine

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed prompts/diagnose_system.txt
var diagnoseSystemPromptEN string

//go:embed prompts/diagnose_lang_tr.txt
var diagnoseLangTR string

//go:embed prompts/diagnose_fewshot.txt
var diagnoseFewshotExamples string

// diagnoseUserMessage is the fixed user-turn content sent with every
// diagnose request — everything the model actually needs (instructions,
// few-shot grounding, the failing command's captured context) lives in
// the system prompt buildDiagnoseSystemPrompt assembles, exactly like
// plan generation's buildSystemPrompt; the user message only states the
// request itself.
const diagnoseUserMessage = "Diagnose the failing command described in the system context above and produce the JSON response."

// buildDiagnoseSystemPrompt assembles the full system prompt sent with
// every diagnose request: the English core instruction (JSON schema,
// root-cause/explanation quality bar, OS/shell targeting, risk labeling),
// the Turkish language instruction block appended only when lang == "tr"
// (resolveLanguage, shared with plan generation, is defined in
// prompt.go), the few-shot examples (diagnose_fewshot.txt — grounding
// only, always included in English regardless of lang, since the model
// reads it, not the user), and finally the failing command's captured
// context plus the system context, via serializeErrorContext.
func buildDiagnoseSystemPrompt(lang string, errCtx ErrorContext) string {
	var b strings.Builder
	b.WriteString(diagnoseSystemPromptEN)
	if lang == "tr" {
		b.WriteString("\n\n")
		b.WriteString(diagnoseLangTR)
	}
	b.WriteString("\n\n")
	b.WriteString(diagnoseFewshotExamples)
	b.WriteString("\n\n")
	b.WriteString(serializeErrorContext(errCtx))
	return b.String()
}

// serializeErrorContext renders errCtx as the grounding block appended to
// the diagnose system prompt: the failing command itself, its exit code
// (or "unknown" — see ErrorContext's doc comment on ExitCode's -1
// sentinel, used by internal/cli's paste-mode fallback, where no real
// exit code was ever observed), its captured stderr/stdout tails, and the
// same OS/shell/package-manager/admin system context block plan
// generation's serializeContext (prompt.go) renders — reused here
// verbatim rather than duplicated, so the two prompts can never drift on
// how that shared grounding is described.
func serializeErrorContext(errCtx ErrorContext) string {
	var b strings.Builder
	b.WriteString("Failing command context:\n")
	fmt.Fprintf(&b, "- Command: %s\n", orUnknown(errCtx.Command))
	if errCtx.ExitCode >= 0 {
		fmt.Fprintf(&b, "- Exit code: %d\n", errCtx.ExitCode)
	} else {
		b.WriteString("- Exit code: unknown\n")
	}
	b.WriteString("- Stderr (tail):\n")
	b.WriteString(orEmptyBlock(errCtx.Stderr))
	b.WriteString("- Stdout (tail):\n")
	b.WriteString(orEmptyBlock(errCtx.Stdout))
	b.WriteString("\n")
	b.WriteString(serializeContext(errCtx.System))
	return b.String()
}

// orEmptyBlock renders s followed by a trailing newline, or the literal
// "(empty)\n" placeholder when s is blank — used for the stderr/stdout
// tails, which are legitimately empty in paste mode when the user only
// supplied a command with no captured output.
func orEmptyBlock(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(empty)\n"
	}
	return s + "\n"
}
