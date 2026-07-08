// Package context collects the operating environment context (OS,
// shell, working directory, last failed command, package managers, and
// opt-in history/env-var names) sent to the LLM as grounding for its
// plans (CLAUDE.md "Bağlam Toplama"). Every OS/exec/env dependency a
// Collector needs is injected (see Collector's fields), so every branch
// — including the windows-only ones — is testable regardless of the OS
// the test binary actually runs on.
//
// Every file in this package that needs the stdlib "context" package
// (this one is itself named "context") imports it under the alias
// stdctx, to keep "context.Foo" in code unambiguous between the two.
package context
