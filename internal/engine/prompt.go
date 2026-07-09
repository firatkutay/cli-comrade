package engine

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
)

//go:embed prompts/plan_system.txt
var planSystemPromptEN string

//go:embed prompts/plan_lang_tr.txt
var planLangTR string

// buildSystemPrompt assembles the full system prompt sent with every plan
// request: the English core instruction (JSON schema, OS/shell targeting,
// risk labeling, chaining/splitting rule, non-interactive-flag rule), the
// Turkish language instruction block appended only when lang == "tr", and
// finally the serialized system context — grounding information only, in
// English regardless of lang, since it is read by the model, not shown to
// the user.
func buildSystemPrompt(lang string, sysCtx contextpkg.Context) string {
	var b strings.Builder
	b.WriteString(planSystemPromptEN)
	if lang == "tr" {
		b.WriteString("\n\n")
		b.WriteString(planLangTR)
	}
	b.WriteString("\n\n")
	b.WriteString(serializeContext(sysCtx))
	return b.String()
}

// serializeContext renders sysCtx as the grounding block appended to the
// plan-generation system prompt: OS, CPU architecture (runtime.GOARCH —
// not part of contextpkg.Context, read directly since it is a build-time
// constant, not something that needs OS-process collection), shell,
// working directory, detected package managers, and admin/root status.
// It deliberately never includes sysCtx.History or sysCtx.EnvNames (only
// collected when the user opts in, and env *values* are never collected
// at all — CLAUDE.md's redaction rule) or sysCtx.LastCommand (irrelevant
// to plan generation; that is FAZ 7's `comrade fix` prompt's concern).
func serializeContext(sysCtx contextpkg.Context) string {
	var b strings.Builder
	b.WriteString("System context:\n")
	fmt.Fprintf(&b, "- OS: %s\n", orUnknown(sysCtx.OS))
	fmt.Fprintf(&b, "- Architecture: %s\n", runtime.GOARCH)
	fmt.Fprintf(&b, "- Shell: %s\n", orUnknown(sysCtx.Shell))
	fmt.Fprintf(&b, "- Working directory: %s\n", orUnknown(sysCtx.WorkingDir))

	if len(sysCtx.PackageManagers) > 0 {
		fmt.Fprintf(&b, "- Detected package managers: %s\n", strings.Join(sysCtx.PackageManagers, ", "))
	} else {
		b.WriteString("- Detected package managers: none detected\n")
	}

	switch {
	case sysCtx.AdminKnown && sysCtx.IsAdmin:
		b.WriteString("- Admin/root privileges: yes\n")
	case sysCtx.AdminKnown && !sysCtx.IsAdmin:
		b.WriteString("- Admin/root privileges: no\n")
	default:
		b.WriteString("- Admin/root privileges: unknown\n")
	}

	return b.String()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
