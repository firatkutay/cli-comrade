package i18n

// MessageID names one catalog entry. Every user-facing string cli-comrade
// prints through a Translator is keyed by one of these constants — never
// a raw string literal — so catalogEN/catalogTR (and the coverage test in
// internal/cli) can enumerate every message cli-comrade actually shows a
// user.
type MessageID string

const (
	// MsgVersionBanner is the one-line version banner printed by a bare
	// `comrade` invocation (no subcommand), before cobra's own help
	// output. One %s: the version string.
	MsgVersionBanner MessageID = "version_banner"

	// MsgFirstRunNotice is printed the first time cli-comrade creates a
	// default config file for the user (internal/cli's shared
	// ensureLoaded/setupCLIRuntime path). One %s: the config file path.
	MsgFirstRunNotice MessageID = "first_run_notice"

	// MsgYoloWarning is CLAUDE.md security rule #6's mandatory red
	// warning, printed on every --yolo use regardless of whether the
	// config-side bypass conditions actually let it do anything.
	MsgYoloWarning MessageID = "yolo_warning"

	// MsgBlockedStep renders a plan step internal/safety.Engine
	// evaluated to Block: never run, in any mode. Two args: the block
	// reason, then the command text.
	MsgBlockedStep MessageID = "blocked_step"

	// MsgBlockedStepEdit renders a step re-evaluated to Block after an
	// ask-mode inline edit ([d]üzenle) — the same shape as MsgBlockedStep
	// minus the leading step number, since this print happens mid-prompt,
	// not in the numbered step-by-step list. Two args: the block reason,
	// then the command text.
	MsgBlockedStepEdit MessageID = "blocked_step_edit"

	// MsgYoloBypass is the mandatory red warning auto mode prints each
	// time the config+--yolo escape hatch actually bypasses a
	// destructive/elevated confirmation for one step. Two args: the
	// effective risk class name, then the command text.
	MsgYoloBypass MessageID = "yolo_bypass"

	// MsgAbortCanceled is engine.RunSummary.AbortReason's value when a
	// run stops because ctx was canceled (Ctrl-C).
	MsgAbortCanceled MessageID = "abort_canceled"

	// MsgAbortStepBlocked is engine.RunSummary.AbortReason's value when a
	// Blocked step aborts the remaining plan (auto mode's
	// abort-on-block). Two args: the 1-based step number, then the block
	// reason.
	MsgAbortStepBlocked MessageID = "abort_step_blocked"

	// MsgAbortStepFailed is engine.RunSummary.AbortReason's value when a
	// step fails with no self-correction attempted. Args: 1-based step
	// number, exit code, the command text, the retry suggestion.
	MsgAbortStepFailed MessageID = "abort_step_failed"

	// MsgAbortStepFailedAfterCorrection is engine.RunSummary.AbortReason's
	// value when a step fails after self-correction attempts were
	// exhausted. Args: 1-based step number, attempts made, exit code, the
	// command text, the retry suggestion.
	MsgAbortStepFailedAfterCorrection MessageID = "abort_step_failed_after_correction"

	// MsgRetrySuggestion is the trailing suggestion appended to both
	// MsgAbortStepFailed and MsgAbortStepFailedAfterCorrection.
	MsgRetrySuggestion MessageID = "retry_suggestion"

	// -- explain (comrade explain <command>) --------------------------

	// MsgExplainSafetyWarning prefixes `comrade explain`'s output when the
	// local safety pass finds the command destructive or denylisted. Two
	// args: the effective risk class name, then the block/confirm reason
	// (empty string when there is none).
	MsgExplainSafetyWarning MessageID = "explain_safety_warning"

	// MsgExplainSummaryHeading labels the LLM's one-paragraph summary in
	// `comrade explain`'s rendered output.
	MsgExplainSummaryHeading MessageID = "explain_summary_heading"

	// MsgExplainPartsHeading labels the flag-by-flag breakdown list in
	// `comrade explain`'s rendered output.
	MsgExplainPartsHeading MessageID = "explain_parts_heading"

	// MsgExplainRiskHeading labels the LLM's own risk note in `comrade
	// explain`'s rendered output (secondary to the safety warning above
	// it — see docs/history/phases/FAZ-09.md).
	MsgExplainRiskHeading MessageID = "explain_risk_heading"

	// MsgExplainUsageError is printed when `comrade explain` is given no
	// command text at all (QA D1: explain.go sets DisableFlagParsing so
	// it can accept a command starting with a flag, e.g. "explain -rf",
	// which also means cobra's own automatic -h/--help interception and
	// its own "requires at least 1 arg(s)" MinimumNArgs message never
	// fire — explain.go's RunE now checks for both cases itself, and
	// this is the no-args one's own translated usage error instead of
	// silently treating "" as a command or forwarding to the LLM).
	MsgExplainUsageError MessageID = "explain_usage_error"

	// -- chat (comrade chat) ------------------------------------------

	// MsgChatWelcome is the one-line banner chat prints when the session
	// starts.
	MsgChatWelcome MessageID = "chat_welcome"

	// MsgChatRequiresTTY is `comrade chat`'s error when stdin is not an
	// interactive terminal (QA MINOR-5's non-TTY guard, extended to
	// chat: bubbletea itself needs a real TTY and otherwise hangs
	// rather than failing cleanly — see runChat's doc comment). No
	// args.
	MsgChatRequiresTTY MessageID = "chat_requires_tty"

	// MsgChatHelp is the multi-line help text `/help` prints, listing
	// every slash command chat understands.
	MsgChatHelp MessageID = "chat_help"

	// MsgChatModeUsage is printed when `/mode` is given with a missing or
	// invalid argument.
	MsgChatModeUsage MessageID = "chat_mode_usage"

	// MsgChatModeChanged confirms a successful `/mode` switch. One arg:
	// the new mode name.
	MsgChatModeChanged MessageID = "chat_mode_changed"

	// MsgChatCleared confirms `/clear` reset the in-memory history.
	MsgChatCleared MessageID = "chat_cleared"

	// MsgChatSaveUsage is printed when `/save` is given with no filename.
	MsgChatSaveUsage MessageID = "chat_save_usage"

	// MsgChatSaved confirms `/save <file>` wrote the transcript to disk —
	// chat's one, explicit, opt-in exception to "session history is never
	// written to disk" (CLAUDE.md's privacy rule, see
	// docs/history/phases/FAZ-09.md). One arg: the file path.
	MsgChatSaved MessageID = "chat_saved"

	// MsgChatSaveFailed reports a `/save` write failure. Two args: the
	// file path, then the underlying error.
	MsgChatSaveFailed MessageID = "chat_save_failed"

	// MsgChatDoUsage is printed when `/do` is given no request text.
	MsgChatDoUsage MessageID = "chat_do_usage"

	// MsgChatUnknownCommand reports an unrecognized `/xyz` slash command.
	// One arg: the raw command text the user typed.
	MsgChatUnknownCommand MessageID = "chat_unknown_command"

	// MsgChatExiting is printed just before `/exit` ends the session.
	MsgChatExiting MessageID = "chat_exiting"

	// MsgChatLLMError reports a failed chat-turn completion request. One
	// arg: the underlying error.
	MsgChatLLMError MessageID = "chat_llm_error"

	// MsgChatDoSummary prefixes the run summary chat prints after a `/do`
	// request finishes (executed/skipped/blocked counts appended by the
	// caller — see MsgRunSummaryCounts, reused verbatim from `comrade
	// do`'s printRunSummary).
	MsgChatDoSummary MessageID = "chat_do_summary"

	// MsgTestLLMResult is `comrade config test-llm`'s (hidden diagnostic
	// command) one-line result. Three args: provider name, model name,
	// rounded latency duration string.
	MsgTestLLMResult MessageID = "test_llm_result"

	// MsgConfigListHeader is `comrade config list`'s table header row.
	MsgConfigListHeader MessageID = "config_list_header"

	// MsgConfigSetUsageError is printed when `comrade config set` is
	// given a number of arguments other than exactly 2 (QA D2:
	// newConfigSetCmd sets DisableFlagParsing so a value starting with
	// "-" is never misread as one of comrade's own flags, which also
	// means cobra's own automatic -h/--help interception and its
	// cobra.ExactArgs(2) validator never run — "comrade config set
	// --help" used to fail with cobra's raw English "accepts 2 arg(s),
	// received 1" instead of ever showing help. RunE now handles both
	// -h/--help and the wrong-arg-count case itself.)
	MsgConfigSetUsageError MessageID = "config_set_usage_error"

	// -- config.Validate/Loader error re-rendering (QA D4a) --------------
	//
	// internal/config.Validate/Loader.Get/Source/Set return structured
	// errors (config.UnknownKeyError/config.InvalidValueError) rather
	// than plain fmt.Errorf specifically so internal/cli's config.go can
	// re-render them through these MessageIDs via errors.As, instead of
	// surfacing config's own English-only Error() text verbatim (a QA-
	// found gap: `comrade config set`'s validation errors were the one
	// user-facing error class in this tree that bypassed i18n entirely).
	// Every EN value below is byte-identical to config.Validate's own
	// previous hardcoded English text, so no existing English-language
	// assertion changes.

	// MsgConfigUnknownKey is config.UnknownKeyError's translated
	// rendering. Two args: the unrecognized key, the comma-joined list of
	// valid keys.
	MsgConfigUnknownKey MessageID = "config_unknown_key"
	// MsgConfigInvalidEnum is config.InvalidValueError{Reason:
	// ReasonInvalidEnum}'s translated rendering. Three args: the rejected
	// raw value, the key, the comma-joined list of allowed values.
	MsgConfigInvalidEnum MessageID = "config_invalid_enum"
	// MsgConfigInvalidBool is config.InvalidValueError{Reason:
	// ReasonNotBoolean}'s translated rendering. Two args: the rejected
	// raw value, the key.
	MsgConfigInvalidBool MessageID = "config_invalid_bool"
	// MsgConfigInvalidInt is config.InvalidValueError{Reason:
	// ReasonNotInteger}'s translated rendering. Two args: the rejected
	// raw value, the key.
	MsgConfigInvalidInt MessageID = "config_invalid_int"
	// MsgConfigNotPositive is config.InvalidValueError{Reason:
	// ReasonNotPositive}'s translated rendering. Two args: the rejected
	// raw value, the key.
	MsgConfigNotPositive MessageID = "config_not_positive"
	// MsgConfigNotNonNegative is config.InvalidValueError{Reason:
	// ReasonNotNonNegative}'s translated rendering. Two args: the
	// rejected raw value, the key.
	MsgConfigNotNonNegative MessageID = "config_not_non_negative"
	// MsgConfigInvalidURL is config.InvalidValueError{Reason:
	// ReasonNotURL}'s translated rendering (SAST finding #3's base_url
	// validation — internal/config/validate.go's checkBaseURL). Two args:
	// the rejected raw value, the key.
	MsgConfigInvalidURL MessageID = "config_invalid_url"
	// MsgConfigMetadataBlocked is config.InvalidValueError{Reason:
	// ReasonMetadataOrLinkLocal}'s translated rendering (SAST finding #3).
	// Two args: the rejected raw value, the key.
	MsgConfigMetadataBlocked MessageID = "config_metadata_blocked"

	// MsgModelsDocsNote is printed after `comrade config models`'s numbered
	// list for a provider with only a static model snapshot (anthropic/
	// google). One arg: the docs URL to check for the current list.
	MsgModelsDocsNote MessageID = "models_docs_note"

	// MsgModelsSelectPrompt is `comrade config models`'s selection prompt.
	MsgModelsSelectPrompt MessageID = "models_select_prompt"

	// MsgModelsSetConfirm confirms `comrade config models`'s persisted
	// selection. One arg: the selected model name.
	MsgModelsSetConfirm MessageID = "models_set_confirm"

	// -- comrade do (plan table + run summary) -------------------------

	// MsgAuditRetentionFailed reports a non-fatal audit-log retention-
	// cleanup failure. One arg: the underlying error.
	MsgAuditRetentionFailed MessageID = "audit_retention_failed"

	// MsgPlanTableHeader is renderPlan's table header row — `comrade do`'s
	// primary output (docs/history/UYGULAMA_PLANI.md FAZ 5 item 4).
	MsgPlanTableHeader MessageID = "plan_table_header"

	// MsgPlanBlockedCell renders a Blocked step's RISK column cell. One
	// arg: the block reason.
	MsgPlanBlockedCell MessageID = "plan_blocked_cell"

	// MsgPlanConfirmCell renders a Confirm-escalated step's RISK column
	// cell. One arg: the effective risk class name.
	MsgPlanConfirmCell MessageID = "plan_confirm_cell"

	// MsgRunSummaryCounts is the "N executed, M skipped, K blocked" line
	// printed after ask/auto mode finishes (`comrade do`/`comrade fix`)
	// and reused verbatim by `comrade chat`'s "/do" summary. Three args:
	// executed count, skipped count, blocked count.
	MsgRunSummaryCounts MessageID = "run_summary_counts"

	// MsgRunSummaryAbortedLine reports why a run aborted, appended after
	// MsgRunSummaryCounts. One arg: the (already-localized, from
	// engine.RunDeps.Translator) abort reason text.
	MsgRunSummaryAbortedLine MessageID = "run_summary_aborted_line"

	// -- comrade fix ----------------------------------------------------

	// MsgFixStaleNotice explains why a stale (>10 minutes old)
	// last_command.json entry was ignored, falling through to paste mode.
	MsgFixStaleNotice MessageID = "fix_stale_notice"

	// MsgFixExitZeroNotice explains why a successful (exit 0)
	// last_command.json entry was ignored, falling through to paste mode.
	MsgFixExitZeroNotice MessageID = "fix_exit_zero_notice"

	// MsgFixRefusalNotice explains why `comrade fix --rerun`/`-- <cmd>`
	// refused to re-run a command the local safety.Engine classifies as
	// destructive/denylisted, falling through to paste mode. Two args:
	// the command text, then its classification (MsgFixBlockedClassification
	// or a plain risk-class name).
	MsgFixRefusalNotice MessageID = "fix_refusal_notice"

	// MsgFixBlockedClassification is the classification word substituted
	// into MsgFixRefusalNotice's second %s when the command matched the
	// denylist outright (Block), rather than merely being classified
	// RiskDestructive.
	MsgFixBlockedClassification MessageID = "fix_blocked_classification"

	// MsgFixPasteIntro is paste mode's opening instruction.
	MsgFixPasteIntro MessageID = "fix_paste_intro"

	// MsgFixPasteCommandPrompt is paste mode's "Command: " prompt.
	MsgFixPasteCommandPrompt MessageID = "fix_paste_command_prompt"

	// MsgFixPasteErrorPrompt is paste mode's error-output prompt.
	MsgFixPasteErrorPrompt MessageID = "fix_paste_error_prompt"

	// MsgFixRootCauseHeading labels `comrade fix`'s printed root cause,
	// matching `comrade explain`'s heading style.
	MsgFixRootCauseHeading MessageID = "fix_root_cause_heading"

	// MsgFixExplanationHeading labels `comrade fix`'s printed plain-
	// language explanation, matching `comrade explain`'s heading style.
	MsgFixExplanationHeading MessageID = "fix_explanation_heading"

	// MsgVerificationSuggestion is internal/engine.OfferVerification's
	// info-mode rendering of the offered post-fix verification command
	// (QA D6: this was a raw hardcoded English "Suggested verification:
	// %s" literal, the one stray-English label QA found in an otherwise
	// fully-translated `comrade fix` TR run — not LLM-generated text,
	// just a Go format string that had never been routed through i18n
	// like every other engine-printed string RunDeps.Translator already
	// covers). One arg: the command being suggested.
	MsgVerificationSuggestion MessageID = "verification_suggestion"

	// MsgVerificationSucceeded is ask/auto mode's one-line report after
	// actually re-running the verification command and it exiting 0
	// (same QA D6 sweep — found alongside MsgVerificationSuggestion,
	// same file, same previously-unrouted pattern). One arg: the command.
	MsgVerificationSucceeded MessageID = "verification_succeeded"

	// MsgVerificationStillFails is ask/auto mode's one-line report when
	// the re-run verification command still fails. Two args: the
	// command, its exit code.
	MsgVerificationStillFails MessageID = "verification_still_fails"

	// -- comrade auth ----------------------------------------------------

	// MsgAuthEnterKeyPrompt is `comrade auth login`'s no-echo key prompt.
	// One arg: the provider name.
	MsgAuthEnterKeyPrompt MessageID = "auth_enter_key_prompt"

	// MsgAuthStoredKeyPingFailed reports a stored key whose live test
	// request failed for a reason OTHER than the provider rejecting the
	// key itself (network/timeout/5xx/parse — see MsgAuthKeyRejected for
	// the 401/403 case, and MsgAuthModelNotFound for the 404-model case,
	// neither of which use this message) — the key is still stored, since
	// this class of failure says nothing about whether the key itself is
	// actually good (QA MAJOR-2). One arg: the ping error.
	MsgAuthStoredKeyPingFailed MessageID = "auth_stored_key_ping_failed"

	// MsgAuthKeyRejected reports `comrade auth login`'s live test request
	// coming back 401/403 (llm.ErrAuthRejected) — a DEFINITIVE rejection,
	// unlike every other ping-failure class MsgAuthStoredKeyPingFailed
	// covers. auth.go removes the just-stored key before rendering this
	// (QA MAJOR-2: a rejected key must never be left stored, silently
	// waiting to fail again on the next real LLM call) and returns it as
	// a genuine command error (nonzero exit), not a printed notice. Three
	// args: provider name, the ping error, provider name again (for the
	// suggested retry command).
	MsgAuthKeyRejected MessageID = "auth_key_rejected"

	// MsgAuthStoredKeyPingSucceeded reports a stored key whose live test
	// request succeeded. Three args: provider name, model name, rounded
	// latency duration string.
	MsgAuthStoredKeyPingSucceeded MessageID = "auth_stored_key_ping_succeeded"

	// MsgAuthStoredKeyBaseURLUnsafe reports `comrade auth login` storing a
	// key while SKIPPING the live test because the active provider's
	// base_url is reject-class (isBaseURLRejection — the SAME
	// errors.As+Reason check translateBaseURLRejectedError uses for
	// do/fix/explain/chat's own client-build-time refusal; runtime.go).
	// Unlike MsgAuthStoredKeyPingFailed (a network/timeout/5xx/parse
	// failure that says nothing about whether the key itself is good),
	// this is a DEFINITIVE, known cause: the endpoint itself was refused
	// before any request was attempted, so the message names that cause
	// directly instead of the misleading "could not verify it right now."
	// The key is still stored — buildProvider/pingProvider refuse before
	// any network call, so the key was never transmitted, and storing it
	// locally (0600, never sent anywhere) is harmless; only the ping was
	// skipped. Four args: provider name, the offending config key, the
	// rejected raw base_url value, the offending config key again (for
	// the suggested repair command).
	MsgAuthStoredKeyBaseURLUnsafe MessageID = "auth_stored_key_base_url_unsafe"

	// MsgAuthNoStoredKey reports `comrade auth logout` found nothing to
	// remove. One arg: the provider name.
	MsgAuthNoStoredKey MessageID = "auth_no_stored_key"

	// MsgAuthRemovedStoredKey confirms `comrade auth logout` removed a
	// key. One arg: the provider name.
	MsgAuthRemovedStoredKey MessageID = "auth_removed_stored_key"

	// MsgAuthStatusHeader is `comrade auth status`'s table header row.
	MsgAuthStatusHeader MessageID = "auth_status_header"

	// MsgAuthStatusOllamaRow is `comrade auth status`'s fixed ollama row
	// (ollama needs no credential at all).
	MsgAuthStatusOllamaRow MessageID = "auth_status_ollama_row"

	// MsgAuthStatusSet renders a provider with a stored key. One arg: the
	// credential source name (keychain/file — left untranslated, like a
	// risk-class name; it is Store's own internal vocabulary, not prose).
	MsgAuthStatusSet MessageID = "auth_status_set"

	// MsgAuthStatusSetEnv renders a provider whose key came from an
	// environment variable. One arg: the variable name.
	MsgAuthStatusSetEnv MessageID = "auth_status_set_env"

	// MsgAuthStatusNotSet renders a provider with no key at all.
	MsgAuthStatusNotSet MessageID = "auth_status_not_set"

	// MsgAuthOpenAICompatBaseURLPrompt is `comrade auth login
	// openai_compat`'s interactive prompt, shown only when
	// llm.openai_compat.base_url's effective value still equals the
	// shipped default (config.Default()) — openai_compat is a
	// single connector shared by every OpenAI-compatible provider
	// (Mistral, Groq, GLM/Zhipu, Qwen, Kimi/Moonshot, OpenRouter, LM
	// Studio; see CLAUDE.md's LLM Provider Mimarisi), so logging in with a
	// non-OpenAI key while base_url still points at api.openai.com
	// silently pinged the wrong provider and failed with a 401 from
	// OpenAI itself, not the user's actual provider. Pressing Enter with
	// no input keeps the current default untouched. One arg: the current
	// default base_url value.
	MsgAuthOpenAICompatBaseURLPrompt MessageID = "auth_openai_compat_base_url_prompt"

	// MsgAuthOpenAICompatBaseURLSaved confirms
	// MsgAuthOpenAICompatBaseURLPrompt's entered value was persisted to
	// llm.openai_compat.base_url (via Loader.SetAndSave) before the live
	// ping runs. One arg: the saved value.
	MsgAuthOpenAICompatBaseURLSaved MessageID = "auth_openai_compat_base_url_saved"

	// MsgAuthOpenAICompatModelPrompt is `comrade auth login
	// openai_compat`'s interactive model prompt — shown right after
	// MsgAuthOpenAICompatBaseURLPrompt, only when the base_url now in
	// effect is no longer OpenAI's own default AND llm.model is still
	// empty. Without this, buildProvider (internal/llm/client.go) falls
	// back to defaultOpenAICompatModel (an OpenAI-specific model name,
	// e.g. "gpt-5.4-mini") against a provider that has never heard of it,
	// failing with a confusing 404. Pressing Enter with no input leaves
	// llm.model empty, same as MsgAuthOpenAICompatBaseURLPrompt's own
	// empty-line behavior. No args.
	MsgAuthOpenAICompatModelPrompt MessageID = "auth_openai_compat_model_prompt"

	// MsgAuthModelNotFound reports `comrade auth login`'s live test
	// request coming back HTTP 404 for what looks like an unknown-model
	// error (llm.StatusError with StatusCode 404 whose Message mentions
	// "model") — a DEFINITIVE, known cause distinct from
	// MsgAuthStoredKeyPingFailed's generic network/timeout/5xx framing.
	// Non-fatal: the key is still stored (it was never the problem) and
	// this prints as a notice, not a command error, pointing the user at
	// `comrade config models` / `comrade config set llm.model`. One arg:
	// the effective model name that was pinged.
	MsgAuthModelNotFound MessageID = "auth_model_not_found"

	// MsgSecretsFileFallbackWarning is printed once, the first time any
	// stored-credential operation actually runs against the 0600 file
	// fallback (no OS keychain reachable on this machine) — QA MINOR-4's
	// softened wording. It keeps the one load-bearing security fact (the
	// file is base64-encoded, NOT encrypted) in a single calm sentence,
	// dropping the earlier, more alarming "no OS keychain available...
	// NOT encrypted" phrasing. No args.
	MsgSecretsFileFallbackWarning MessageID = "secrets_file_fallback_warning"

	// -- comrade history --------------------------------------------------

	// MsgHistoryTableHeader is `comrade history`'s table header row.
	MsgHistoryTableHeader MessageID = "history_table_header"

	// MsgHistoryEmpty is printed instead of an empty table when the audit
	// log has no entries at all.
	MsgHistoryEmpty MessageID = "history_empty"

	// -- comrade init -----------------------------------------------------

	// MsgInitPowerShellManualFallback is printed when comrade init cannot
	// automatically locate a profile file to edit (PowerShell's $PROFILE
	// could not be resolved). Two args: the shell snippet block, then the
	// resolution-failure note.
	MsgInitPowerShellManualFallback MessageID = "init_powershell_manual_fallback"

	// MsgInitAlreadyInstalled reports the shell integration block is
	// already present. One arg: the rc/profile file path.
	MsgInitAlreadyInstalled MessageID = "init_already_installed"

	// MsgInitPreview previews the block that will be added before asking
	// for confirmation. Two args: the rc/profile file path, then the
	// block text itself.
	MsgInitPreview MessageID = "init_preview"

	// MsgInitConfirmPrompt is the y/N install confirmation prompt. One
	// arg: the rc/profile file path.
	MsgInitConfirmPrompt MessageID = "init_confirm_prompt"

	// MsgInitAborted is printed when the user declines MsgInitConfirmPrompt.
	MsgInitAborted MessageID = "init_aborted"

	// MsgInitInstalled confirms the shell integration was installed. One
	// arg: the rc/profile file path.
	MsgInitInstalled MessageID = "init_installed"

	// MsgInitRemoveNoProfile is printed when --remove cannot locate a
	// profile file at all (nothing to remove). One arg: the resolution-
	// failure note.
	MsgInitRemoveNoProfile MessageID = "init_remove_no_profile"

	// MsgInitNotInstalled reports --remove found no installed block. One
	// arg: the rc/profile file path.
	MsgInitNotInstalled MessageID = "init_not_installed"

	// MsgInitRemoved confirms the shell integration was removed. One arg:
	// the rc/profile file path.
	MsgInitRemoved MessageID = "init_removed"

	// MsgInitFishCompletionsInstalled confirms "comrade init fish" wrote
	// its shell-completions script (shellinit.FishCompletionsScript) to
	// fish's native lazy-load location — a separate artifact from the
	// hook block above, printed in addition to (never instead of)
	// MsgInitInstalled/MsgInitAlreadyInstalled. One arg: the completions
	// file path.
	MsgInitFishCompletionsInstalled MessageID = "init_fish_completions_installed"

	// MsgInitFishCompletionsRemoved confirms "comrade init fish --remove"
	// deleted a previously-installed completions file. One arg: the
	// completions file path.
	MsgInitFishCompletionsRemoved MessageID = "init_fish_completions_removed"

	// -- comrade init powershell (multi-variant: Windows PowerShell 5.1 +
	// PowerShell 7) --------------------------------------------------------
	//
	// On GOOS=windows, "comrade init powershell" targets EVERY installed
	// PowerShell variant's own profile independently (shellinit.
	// ResolvePowerShellProfiles) rather than guessing one from goos — the
	// "pwsh gap" fix (docs/history/PROGRESS.md). Every message below takes the
	// variant's product-name Label() (shellinit.PSVariant.Label — itself
	// deliberately untranslated, see that method's own doc comment) as
	// its first arg, then behaves exactly like this same message's
	// single-profile MsgInitXxx counterpart above.

	// MsgInitPSVariantAlreadyInstalled is one variant's line in a
	// multi-profile install report when that variant's profile already
	// has the current block. Two args: the variant label, then the
	// profile path.
	MsgInitPSVariantAlreadyInstalled MessageID = "init_ps_variant_already_installed"

	// MsgInitPSVariantInstalled confirms one variant's profile was
	// installed or upgraded. Two args: the variant label, then the
	// profile path.
	MsgInitPSVariantInstalled MessageID = "init_ps_variant_installed"

	// MsgInitPSVariantNotInstalled reports one variant's --remove found
	// no installed block in that variant's profile. Two args: the
	// variant label, then the profile path.
	MsgInitPSVariantNotInstalled MessageID = "init_ps_variant_not_installed"

	// MsgInitPSVariantRemoved confirms one variant's profile had the
	// block removed. Two args: the variant label, then the profile path.
	MsgInitPSVariantRemoved MessageID = "init_ps_variant_removed"

	// MsgInitPSVariantUnresolved reports one variant whose binary was
	// found but whose own $PROFILE could not be resolved — every OTHER
	// variant is still processed normally (see shellinit.
	// ResolvePowerShellProfiles' doc comment). Two args: the variant
	// label, then the underlying (untranslated, English) resolution-
	// failure note.
	MsgInitPSVariantUnresolved MessageID = "init_ps_variant_unresolved"

	// MsgInitConfirmPromptMulti is the y/N install confirmation prompt
	// for a multi-profile PowerShell install, asked once for every
	// pending profile shown in the preceding MsgInitPreview line(s). No
	// args — unlike MsgInitConfirmPrompt, it does not repeat the
	// path(s); those were already printed just above it.
	MsgInitConfirmPromptMulti MessageID = "init_confirm_prompt_multi"

	// -- cobra --help Short text (one per command) -----------------------
	//
	// cobra reads a command's Short field directly (both for its own
	// "<command> --help" and for a parent's "Available Commands" listing
	// of every child's Short), which is a plain string set once at
	// command-construction time — before any per-invocation config is
	// ever loaded. internal/cli/help.go's applyTranslatedHelp overwrites
	// every command's Short from this catalog, keyed by CommandPath(),
	// immediately before cobra actually renders help/usage, so --help
	// output is localized exactly like every other command's output.

	// Every MsgHelpShortXxx constant below is the same shape — one line
	// of --help/usage text for one command in the tree, keyed by
	// internal/cli/help.go's helpShortByPath map — so each gets a short,
	// uniform doc comment naming that command rather than repeating the
	// mechanism explained above.

	// MsgHelpShortRoot is the root `comrade` command's --help text.
	MsgHelpShortRoot MessageID = "help_short_root"
	// MsgHelpShortDo is `comrade do`'s --help text.
	MsgHelpShortDo MessageID = "help_short_do"
	// MsgHelpShortFix is `comrade fix`'s --help text.
	MsgHelpShortFix MessageID = "help_short_fix"
	// MsgHelpShortExplain is `comrade explain`'s --help text.
	MsgHelpShortExplain MessageID = "help_short_explain"
	// MsgHelpShortChat is `comrade chat`'s --help text.
	MsgHelpShortChat MessageID = "help_short_chat"
	// MsgHelpShortConfig is `comrade config`'s --help text.
	MsgHelpShortConfig MessageID = "help_short_config"
	// MsgHelpShortConfigGet is `comrade config get`'s --help text.
	MsgHelpShortConfigGet MessageID = "help_short_config_get"
	// MsgHelpShortConfigSet is `comrade config set`'s --help text.
	MsgHelpShortConfigSet MessageID = "help_short_config_set"
	// MsgHelpShortConfigList is `comrade config list`'s --help text.
	MsgHelpShortConfigList MessageID = "help_short_config_list"
	// MsgHelpShortConfigEdit is `comrade config edit`'s --help text.
	MsgHelpShortConfigEdit MessageID = "help_short_config_edit"
	// MsgHelpShortConfigPath is `comrade config path`'s --help text.
	MsgHelpShortConfigPath MessageID = "help_short_config_path"
	// MsgHelpShortConfigTestLLM is `comrade config test-llm`'s --help text.
	MsgHelpShortConfigTestLLM MessageID = "help_short_config_test_llm"
	// MsgHelpShortConfigModels is `comrade config models`'s --help text.
	MsgHelpShortConfigModels MessageID = "help_short_config_models"
	// MsgHelpShortAuth is `comrade auth`'s --help text.
	MsgHelpShortAuth MessageID = "help_short_auth"
	// MsgHelpShortAuthLogin is `comrade auth login`'s --help text.
	MsgHelpShortAuthLogin MessageID = "help_short_auth_login"
	// MsgHelpShortAuthLogout is `comrade auth logout`'s --help text.
	MsgHelpShortAuthLogout MessageID = "help_short_auth_logout"
	// MsgHelpShortAuthStatus is `comrade auth status`'s --help text.
	MsgHelpShortAuthStatus MessageID = "help_short_auth_status"
	// MsgHelpShortInit is `comrade init`'s --help text.
	MsgHelpShortInit MessageID = "help_short_init"
	// MsgHelpShortHistory is `comrade history`'s --help text.
	MsgHelpShortHistory MessageID = "help_short_history"
	// MsgHelpShortHook is `comrade hook`'s --help text.
	MsgHelpShortHook MessageID = "help_short_hook"
	// MsgHelpShortHookRecord is `comrade hook record`'s --help text.
	MsgHelpShortHookRecord MessageID = "help_short_hook_record"
	// MsgHelpShortUpgrade is `comrade upgrade`'s --help text.
	MsgHelpShortUpgrade MessageID = "help_short_upgrade"

	// -- root --help command-group titles and Examples section -----------
	//
	// Same render-time-override mechanism as the Short/flag text above:
	// internal/cli/help.go's applyTranslatedHelp also overwrites every
	// registered *cobra.Group's Title (groupTitleByID) and root's own
	// Example field from these, immediately before cobra renders
	// --help/usage — cobra's default usage template renders both verbatim
	// once Groups are registered (see root.go), so both need the same
	// lazy, per-invocation-language override as everything else here.

	// MsgHelpGroupCore titles the "Core" command group (do/fix/explain/chat).
	MsgHelpGroupCore MessageID = "help_group_core"
	// MsgHelpGroupSetup titles the "Setup" command group (auth/init/config).
	MsgHelpGroupSetup MessageID = "help_group_setup"
	// MsgHelpGroupInfo titles the "Info" command group (history/upgrade).
	MsgHelpGroupInfo MessageID = "help_group_info"
	// MsgHelpExamplesRoot is root's --help "Examples:" section body — each
	// line pre-indented by two spaces to match cobra's own template
	// convention for that section (see cobra's defaultUsageTemplate:
	// ".Example" is rendered verbatim, with no added indentation).
	MsgHelpExamplesRoot MessageID = "help_examples_root"

	// -- --help/usage structural section labels (QA D4b) ------------------
	//
	// cobra's own defaultUsageTemplate hardcodes these eight section
	// labels as literal English text with no per-command override point
	// — internal/cli/help.go's usageTemplateFor builds a full
	// SetUsageTemplate() replacement from these MessageIDs, structurally
	// IDENTICAL to cobra's own template (same fields, same control flow,
	// copied verbatim from spf13/cobra v1.10.2's command.go
	// defaultUsageTemplate — see usageTemplateFor's own doc comment for
	// the version-drift caveat this implies) with only the label text
	// itself swapped for these. colorizeHelpText's header-recognizer maps
	// (help.go) key off the CURRENT resolved Translator's rendering of
	// these same IDs, so both stay in sync by construction rather than by
	// two separately-maintained literal-string lists.

	// MsgHelpLabelUsage labels the "Usage:" section.
	MsgHelpLabelUsage MessageID = "help_label_usage"
	// MsgHelpLabelAliases labels a command's Aliases: block. No command
	// in this tree currently HAS an alias, so this is unreached today —
	// included for completeness/correctness, not because it currently
	// renders anywhere.
	MsgHelpLabelAliases MessageID = "help_label_aliases"
	// MsgHelpLabelExamples labels the "Examples:" section.
	MsgHelpLabelExamples MessageID = "help_label_examples"
	// MsgHelpLabelAvailableCommands labels the "Available Commands:"
	// section — rendered only for a command tree with no cobra Groups
	// registered (this tree's root DOES register Groups, so this is
	// reached by every non-root command with subcommands, e.g. `comrade
	// config --help`, not by root itself).
	MsgHelpLabelAvailableCommands MessageID = "help_label_available_commands"
	// MsgHelpLabelAdditionalCommands labels the "Additional Commands:"
	// section (any subcommand not assigned to one of root's own Groups
	// — "help" itself, plus any future ungrouped addition).
	MsgHelpLabelAdditionalCommands MessageID = "help_label_additional_commands"
	// MsgHelpLabelFlags labels the "Flags:" (local flags) section.
	MsgHelpLabelFlags MessageID = "help_label_flags"
	// MsgHelpLabelGlobalFlags labels the "Global Flags:" (inherited
	// persistent flags) section.
	MsgHelpLabelGlobalFlags MessageID = "help_label_global_flags"
	// MsgHelpLabelAdditionalHelpTopics labels cobra's "additional help
	// topic" pseudo-commands (a command with no Run/RunE and no
	// subcommands of its own, used for a documentation-only entry) — this
	// tree defines none, so also unreached today; included for the same
	// completeness reason as MsgHelpLabelAliases.
	MsgHelpLabelAdditionalHelpTopics MessageID = "help_label_additional_help_topics"
	// MsgHelpMoreInfo is the trailing "Use "<path> [command] --help" for
	// more information about a command." line — used as raw TEMPLATE
	// SOURCE (usageTemplateFor, help.go), not ordinary rendered text: its
	// catalog value embeds cobra's own literal "{{.CommandPath}}" template
	// syntax and MUST be resolved via a zero-arg Translator.T(id) call
	// (Translator.T's own contract: a zero-arg call returns the catalog
	// string completely unchanged, never run through fmt.Sprintf) so that
	// literal "{{.CommandPath}}" survives intact into the built template
	// string for cobra's own template engine to substitute per-command,
	// per-render — every command's help uses the SAME one template
	// (root.SetUsageTemplate, inherited tree-wide), so this cannot be a
	// pre-filled %s value the way every OTHER MessageID's dynamic args
	// are; it has to stay a live template reference.
	MsgHelpMoreInfo MessageID = "help_more_info"

	// -- per-flag --help descriptions ------------------------------------
	//
	// Exactly like Short text above, a flag's description (pflag's
	// "usage" string) is a plain string baked in at flag-registration
	// time (addExecutionFlags/newFixCmd/newHistoryCmd/newInitCmd), before
	// any per-invocation config is loaded. internal/cli/help.go's
	// applyTranslatedHelp ALSO walks every command's own (non-inherited)
	// flags and overwrites each one's Usage from this catalog, keyed by
	// flag name (the description text is identical everywhere a given
	// flag name appears in this tree — e.g. --auto/--ask/--info/--yolo/
	// --dry-run are registered identically on root, `do`, and `fix`).
	// `comrade hook record`'s three flags (--shell/--exit/--command) are
	// deliberately NOT covered here — see catalogCoverageAllowlist's
	// hook.go entry: internal-only, invoked by generated shell snippets,
	// never read by an end user via --help.

	// MsgFlagDryRun is --dry-run's --help description.
	MsgFlagDryRun MessageID = "flag_dry_run"
	// MsgFlagAuto is --auto's --help description.
	MsgFlagAuto MessageID = "flag_auto"
	// MsgFlagAsk is --ask's --help description.
	MsgFlagAsk MessageID = "flag_ask"
	// MsgFlagInfo is --info's --help description.
	MsgFlagInfo MessageID = "flag_info"
	// MsgFlagYolo is --yolo's --help description.
	MsgFlagYolo MessageID = "flag_yolo"
	// MsgFlagRerun is --rerun's --help description.
	MsgFlagRerun MessageID = "flag_rerun"
	// MsgFlagJSON is --json's --help description.
	MsgFlagJSON MessageID = "flag_json"
	// MsgFlagLimit is --limit's --help description.
	MsgFlagLimit MessageID = "flag_limit"
	// MsgFlagPrint is --print's --help description.
	MsgFlagPrint MessageID = "flag_print"
	// MsgFlagRemove is --remove's --help description.
	MsgFlagRemove MessageID = "flag_remove"
	// MsgFlagYes is --yes's --help description.
	MsgFlagYes MessageID = "flag_yes"

	// -- full-sentence user-facing fmt.Errorf/errors.New messages --------
	//
	// These are the standalone, complete-sentence error messages a user
	// reads as the terminal "Error: ..." line itself (cmd/comrade/main.go
	// prints whatever Execute() returns via %v) — as opposed to the
	// project's dozens of "doing X: %w" wrap-chain errors (CLAUDE.md's
	// own established convention, used identically throughout this
	// codebase to add call-site context to an inner error), which are
	// deliberately NOT migrated here — see docs/history/phases/FAZ-09.md's exact
	// rule and the full accounting of what was/wasn't migrated.

	// MsgAuthOllamaNoKeyError is `comrade auth login ollama`'s refusal
	// message (ollama needs no API key).
	MsgAuthOllamaNoKeyError MessageID = "auth_ollama_no_key_error"
	// MsgAuthUnknownProviderError is `comrade auth login <unknown>`'s
	// error message. One arg: the list of known provider names.
	MsgAuthUnknownProviderError MessageID = "auth_unknown_provider_error"
	// MsgAuthNoKeyEnteredError is `comrade auth login`'s error when the
	// interactive prompt received an empty key.
	MsgAuthNoKeyEnteredError MessageID = "auth_no_key_entered_error"
	// MsgAuthLoginRequiresTTY is `comrade auth login`'s error when
	// stdin is not an interactive terminal (QA MINOR-5) — replaces the
	// raw errno x/term.ReadPassword returns in that situation (e.g.
	// "inappropriate ioctl for device" on Unix), which named no cause a
	// non-expert user could act on. No args.
	MsgAuthLoginRequiresTTY MessageID = "auth_login_requires_tty"
	// MsgLLMNoKeyError replaces the raw internal wrap-chain
	// ("llm: all providers failed: anthropic: no API key found for
	// provider \"anthropic\"; set one of: ...") every LLM-reaching
	// command (do/fix/explain/chat) used to surface verbatim, English-
	// only, when *llm.KeyMissingError bubbles out of a blocking
	// Complete/Stream call (QA MAJOR-1) — the single most common
	// first-run failure mode, not a bug, so it gets a friendly,
	// actionable message instead of an internal error dump. Two args:
	// the provider name (prose), the provider name again (the suggested
	// command's own argument).
	MsgLLMNoKeyError MessageID = "llm_no_key_error"
	// MsgLLMBaseURLRejected replaces the raw internal wrap-chain
	// (fmt.Errorf("llm: %s: %w", providerName, *config.InvalidValueError))
	// internal/llm/client.go's buildProvider returns when the ACTIVE
	// provider's base_url is reject-class per SAST finding #3 (non-
	// http(s) scheme, no host, or a cloud-metadata/link-local host) —
	// every LLM-reaching command (do/fix/explain/chat) refuses to build a
	// client rather than hand the API key to that host. Unlike
	// MsgConfigInvalidURL/MsgConfigMetadataBlocked (the SAME underlying
	// InvalidValueError, rendered for a rejected `comrade config set`),
	// this points the user at the fix: `comrade config set` remains
	// reachable because config subcommands never build an LLM client.
	// Three args: the offending key, the rejected raw value, the
	// offending key again (repeated in the suggested command).
	MsgLLMBaseURLRejected MessageID = "llm_base_url_rejected"
	// MsgFixRerunNoLastCommandError is `comrade fix --rerun`'s error when
	// there is no recorded last command to re-run.
	MsgFixRerunNoLastCommandError MessageID = "fix_rerun_no_last_command_error"
	// MsgFlagsModeExclusiveError is the error when more than one of
	// --auto/--ask/--info is given.
	MsgFlagsModeExclusiveError MessageID = "flags_mode_exclusive_error"
	// MsgInitPrintRemoveExclusiveError is `comrade init`'s error when
	// --print and --remove are both given.
	MsgInitPrintRemoveExclusiveError MessageID = "init_print_remove_exclusive_error"
	// MsgInitShellUndetectedError is `comrade init`'s error when no shell
	// argument was given and none could be auto-detected.
	MsgInitShellUndetectedError MessageID = "init_shell_undetected_error"
	// MsgInitShellUnsupportedError is `comrade init`'s error when the
	// detected/given shell name isn't one this project supports.
	MsgInitShellUnsupportedError MessageID = "init_shell_unsupported_error"
	// MsgInitPowerShellNoneFoundError is `comrade init powershell`'s
	// error on GOOS=windows when NEITHER Windows PowerShell 5.1 nor
	// PowerShell 7 could be found on PATH (shellinit.
	// ErrNoPowerShellFound) — the one case the multi-variant install
	// cannot proceed with at all; finding only one of the two is not an
	// error (see shellinit.ResolvePowerShellProfiles).
	MsgInitPowerShellNoneFoundError MessageID = "init_powershell_none_found_error"
	// MsgModelsNoModelsError is `comrade config models`'s error when the
	// active provider returned an empty model list. One arg: the
	// provider name.
	MsgModelsNoModelsError MessageID = "models_no_models_error"
	// MsgModelsUnknownProviderError is `comrade config models`'s error
	// for an unrecognized provider name. One arg: the provider name.
	MsgModelsUnknownProviderError MessageID = "models_unknown_provider_error"
	// MsgModelsChoiceNotANumber is `comrade config models`'s error when
	// the picker's input isn't a number.
	MsgModelsChoiceNotANumber MessageID = "models_choice_not_a_number"
	// MsgModelsChoiceOutOfRange is `comrade config models`'s error when
	// the picker's number is outside the listed range.
	MsgModelsChoiceOutOfRange MessageID = "models_choice_out_of_range"

	// -- translated cobra.PositionalArgs arg-count usage errors -----------
	//
	// Every message below replaces a raw, English-only cobra Args-
	// validator failure ("accepts N arg(s), received M", "unknown
	// command %q for %q") with a friendly, i18n'd usage error — see
	// internal/cli/argvalidation.go's translatedExactArgs/translatedMinArgs/
	// translatedMaxArgs/translatedNoArgs, the one shared implementation
	// behind every leaf command's Args field this applies to.

	// MsgAuthLoginUsageError is `comrade auth login`'s error when given
	// zero or 2+ arguments (it takes exactly one: the provider name).
	// One arg: the comma-joined list of secrets.KnownProviders.
	MsgAuthLoginUsageError MessageID = "auth_login_usage_error"
	// MsgAuthLogoutUsageError is `comrade auth logout`'s error when given
	// zero or 2+ arguments. One arg: the comma-joined list of
	// secrets.KnownProviders.
	MsgAuthLogoutUsageError MessageID = "auth_logout_usage_error"
	// MsgDoUsageError is `comrade do`'s error when given zero arguments.
	// No args — the example request text in the message is the same
	// literal example every language's MsgHelpExamplesRoot already uses
	// for its own free-text-request example line.
	MsgDoUsageError MessageID = "do_usage_error"
	// MsgInitUsageError is `comrade init`'s error when given 2+
	// arguments (it takes at most one: the shell name).
	MsgInitUsageError MessageID = "init_usage_error"
	// MsgConfigGetUsageError is `comrade config get`'s error when given
	// zero or 2+ arguments (it takes exactly one: the key).
	MsgConfigGetUsageError MessageID = "config_get_usage_error"
	// MsgUsageNoArgsError is the SHARED error for every leaf command that
	// takes no positional arguments at all (chat, history, config
	// list/edit/path/models/test-llm, upgrade, auth status, hook, hook
	// record) when given one or more stray arguments — one MessageID
	// covering every such command rather than a dedicated one apiece,
	// since there is nothing command-specific left to say once the
	// command's own full path is in the message. One arg: the resolved
	// command's own CommandPath (e.g. "comrade chat"), filled in at
	// validation time, not baked in at construction.
	MsgUsageNoArgsError MessageID = "usage_no_args_error"
	// MsgUnknownSubcommandError is the SHARED error for a parent command
	// with real, visible subcommands (auth, config) given a first
	// positional argument that matches none of them (translatedUnknownSubcommand,
	// argvalidation.go) — e.g. "comrade auth bogus" — instead of cobra's
	// raw, untranslated "unknown command %q for %q". Three args: the
	// unmatched subcommand name, the parent command's own CommandPath,
	// and its comma-joined list of non-Hidden child command names.
	MsgUnknownSubcommandError MessageID = "unknown_subcommand_error"

	// -- comrade upgrade (FAZ 10) -----------------------------------------

	// MsgUpgradeDevBuildError refuses to run `comrade upgrade` (--check or
	// not) against a "dev" (un-versioned local) build, which has no
	// released tag to compare against.
	MsgUpgradeDevBuildError MessageID = "upgrade_dev_build_error"

	// MsgUpgradeUpToDate reports the running version is already the
	// latest published release. One arg: the current version.
	MsgUpgradeUpToDate MessageID = "upgrade_up_to_date"

	// MsgUpgradeNewerAvailable reports a newer release exists. Three
	// args: the latest version, the current version, the release URL.
	MsgUpgradeNewerAvailable MessageID = "upgrade_newer_available"

	// MsgUpgradeDownloading is printed just before downloading the
	// matching platform archive. One arg: the target version.
	MsgUpgradeDownloading MessageID = "upgrade_downloading"

	// MsgUpgradeInstalled confirms a completed self-update. One arg: the
	// newly installed version.
	MsgUpgradeInstalled MessageID = "upgrade_installed"

	// MsgUpgradeNoReleaseFound is printed for update.ErrReleaseNotFound
	// (QA D3): GitHub's "latest release" endpoint 404s when this
	// repository has no published release yet. No args — replaces what
	// used to be GitHub's own raw, English 404 JSON response body
	// dumped straight to stderr.
	MsgUpgradeNoReleaseFound MessageID = "upgrade_no_release_found"

	// MsgUpgradeFetchFailed is printed for any OTHER update.ErrFetchFailed
	// (QA D3's "other HTTP errors" case: network unreachable, a non-200/
	// non-404 status, a malformed response body) — a concise, i18n'd
	// wrapper; no args, deliberately: the underlying detail (status code,
	// truncated response body) never reaches this message, only
	// COMRADE_DEBUG-gated stderr output (see upgrade.go).
	MsgUpgradeFetchFailed MessageID = "upgrade_fetch_failed"

	// MsgUpgradeSymlinkResolveWarning is a non-fatal warning printed to
	// stderr when `comrade upgrade` cannot resolve the running
	// executable's path through filepath.EvalSymlinks (LOW#8) — the
	// upgrade still proceeds, replacing the original, unresolved path.
	// Two args: the original executable path, and the underlying
	// resolution error.
	MsgUpgradeSymlinkResolveWarning MessageID = "upgrade_symlink_resolve_warning"

	// MsgUpgradeSignatureNotConfigured is a non-fatal, informational
	// warning printed to stderr (MEDIUM#4) when the embedded cosign.pub
	// is still the build-time placeholder: `comrade upgrade` falls back
	// to checksum-only verification instead of verifying checksums.txt's
	// signature. No args.
	MsgUpgradeSignatureNotConfigured MessageID = "upgrade_signature_not_configured"

	// MsgUpgradeSignatureMissing is printed for update.ErrMissingSignatureAsset
	// (MEDIUM#4): a real signing key IS configured, but the release being
	// installed published no checksums.txt.sig asset — `comrade upgrade`
	// refuses to install it. No args.
	MsgUpgradeSignatureMissing MessageID = "upgrade_signature_missing"

	// MsgUpgradeSignatureInvalid is printed for update.ErrSignatureInvalid
	// (MEDIUM#4): a real signing key is configured and the release
	// published a checksums.txt.sig, but it did not verify against the
	// embedded public key — `comrade upgrade` refuses to install it. No
	// args.
	MsgUpgradeSignatureInvalid MessageID = "upgrade_signature_invalid"

	// -- per-flag --help descriptions (comrade upgrade) -------------------

	// MsgFlagCheck is --check's --help description.
	MsgFlagCheck MessageID = "flag_check"

	// -- passive version-update notification (FAZ 10 item 4) -------------

	// MsgUpdateAvailableNotice is the single line printed at the end of a
	// command, at most once per week, when a background check found a
	// newer release than the running build. Two args: the latest
	// version, the current version.
	MsgUpdateAvailableNotice MessageID = "update_available_notice"

	// -- internal/tui ask-mode confirm prompt ----------------------------
	//
	// internal/tui/confirm.go's confirmModel.View() renders these through
	// its injected Translator instead of a raw literal — the fix for the
	// bug this pair of keys exists to close: the prompt used to be a
	// hardcoded-Turkish literal regardless of general.language. The
	// accepted keys per language are NOT a union (see mapKey in
	// confirm.go): TR's "e"=Yes and EN's "e"=Edit would otherwise collide
	// dangerously (same for TR "a"=Explain vs EN "a"=All).

	// MsgConfirmLegend is the ask-mode confirm prompt's trailing options
	// legend, rendered after the command/rationale/risk badge. No args —
	// each language's full legend (letters and all) is one atomic catalog
	// string, not assembled from per-choice fragments, because the
	// letter-to-choice mapping itself changes per language (see mapKey).
	MsgConfirmLegend MessageID = "confirm_legend"

	// MsgConfirmEditHeader is the header line shown while editing a
	// command inline ([d]üzenle in TR / [e]dit in EN), before the
	// textinput itself.
	MsgConfirmEditHeader MessageID = "confirm_edit_header"

	// MsgSpinnerThinking is the label internal/cli's waitSpinner (Part
	// 2(d) — animated braille spinner shown to stderr while do/fix/
	// explain/chat's "/do" waits on a blocking LLM call) renders next to
	// its animated frame. No args — this is the whole label, every time.
	MsgSpinnerThinking MessageID = "spinner_thinking"

	// --- token usage ---

	// MsgFlagUsage is --usage's --help description (do/fix/chat's
	// cobra-registered flag; `comrade explain --usage` handles the flag
	// manually — DisableFlagParsing — so it never reaches cobra's own
	// --help rendering at all, see explain.go).
	MsgFlagUsage MessageID = "flag_usage"

	// MsgUsageSummary is the base per-run/per-turn token-usage line
	// internal/cli/usage.go's formatUsageLine renders (do/fix/explain
	// after the run; chat per turn and at session-total time) — the cost
	// segment (MsgUsageCostEstimate/MsgUsageCostLocal) is appended after
	// it, never interpolated into it, so a translation can reorder/omit
	// the cost segment independently of this line's own word order. Five
	// args: formatted input-token count, formatted output-token count,
	// request count, provider name, model name.
	MsgUsageSummary MessageID = "usage_summary"

	// MsgUsageCostEstimate is appended to MsgUsageSummary when
	// llm.EstimateUSD found a priced table row for the run's most recent
	// provider/model. One arg: the formatted dollar amount (e.g.
	// "$0.0021").
	MsgUsageCostEstimate MessageID = "usage_cost_estimate"

	// MsgUsageCostLocal is appended to MsgUsageSummary instead of
	// MsgUsageCostEstimate when the run's most recent provider is
	// ollama — a local runtime has no real USD cost to estimate. No
	// args.
	MsgUsageCostLocal MessageID = "usage_cost_local"

	// MsgChatUsageSessionTotal prefixes the cumulative session-total
	// usage line `comrade chat` appends to its "/exit"/"/quit" reply
	// (chatdispatch.go's appendSessionTotal) — as opposed to the
	// unprefixed per-turn line appendTurnUsage appends after every
	// ordinary turn. One arg: formatUsageLine's own rendered line (the
	// same MsgUsageSummary + cost-segment shape used everywhere else),
	// embedded here as an already-fully-translated fragment, not raw
	// prose.
	MsgChatUsageSessionTotal MessageID = "chat_usage_session_total"

	// --- doctor ---
	//
	// `comrade doctor` (internal/doctor + internal/cli/doctor.go): every
	// check's per-check title, its outcome-specific one-line summaries,
	// and the two small rendering labels ("fix:"/"detail:") plus the
	// command's own failed-check exit summary and its --live flag
	// description. internal/doctor.Result never holds rendered text
	// itself (see that package's own doc comment) — only a MessageID and
	// args, resolved here.

	// MsgDoctorVersionTitle is the "version" check's row title.
	MsgDoctorVersionTitle MessageID = "doctor_version_title"
	// MsgDoctorPathTitle is the "path" check's row title.
	MsgDoctorPathTitle MessageID = "doctor_path_title"
	// MsgDoctorShellHookTitle is the "shellhook" check's row title.
	MsgDoctorShellHookTitle MessageID = "doctor_shellhook_title"
	// MsgDoctorKeyTitle is the "key" check's row title.
	MsgDoctorKeyTitle MessageID = "doctor_key_title"
	// MsgDoctorReachTitle is the "reach" check's row title.
	MsgDoctorReachTitle MessageID = "doctor_reach_title"
	// MsgDoctorBaseURLTitle is the "baseurl" check's row title.
	MsgDoctorBaseURLTitle MessageID = "doctor_baseurl_title"
	// MsgDoctorConfigTitle is the "config" check's row title.
	MsgDoctorConfigTitle MessageID = "doctor_config_title"

	// MsgDoctorVersionDevSkip fires for a dev build (update.IsDevBuild),
	// which has no comparable release tag — the version check is
	// skipped. No args.
	MsgDoctorVersionDevSkip MessageID = "doctor_version_dev_skip"
	// MsgDoctorVersionFetchError reports that update.Updater.Check itself
	// failed (network/GitHub reachability) — a Warn, not a Fail, since
	// this says nothing about whether the installed version is actually
	// fine. No args (the raw fetch error goes in Result.Detail instead —
	// see doctor.Result's own doc comment on why Detail is never
	// interpolated into a translated Summary).
	MsgDoctorVersionFetchError MessageID = "doctor_version_fetch_error"
	// MsgDoctorVersionBehind reports that a newer release is published.
	// Two args: the latest version, the current (running) version.
	MsgDoctorVersionBehind MessageID = "doctor_version_behind"
	// MsgDoctorVersionUpToDate reports that the running version is
	// already the latest published release. One arg: the current
	// version.
	MsgDoctorVersionUpToDate MessageID = "doctor_version_up_to_date"

	// MsgDoctorPathNotFound reports that the comrade binary is not on
	// PATH at all. One arg: the binary name looked up ("comrade" or
	// "comrade.exe").
	MsgDoctorPathNotFound MessageID = "doctor_path_not_found"
	// MsgDoctorPathStale reports that PATH resolves to a comrade binary
	// other than the one currently running this diagnostic. One arg: the
	// resolved path.
	MsgDoctorPathStale MessageID = "doctor_path_stale"
	// MsgDoctorPathOK reports that PATH resolves to the running binary
	// itself. One arg: the resolved path.
	MsgDoctorPathOK MessageID = "doctor_path_ok"

	// MsgDoctorShellHookUndetected reports that the current shell could
	// not be detected from the environment at all. No args.
	MsgDoctorShellHookUndetected MessageID = "doctor_shellhook_undetected"
	// MsgDoctorShellHookUnsupported reports that the detected shell is
	// not one `comrade init` integrates with (e.g. cmd.exe). One arg: the
	// detected shell's name.
	MsgDoctorShellHookUnsupported MessageID = "doctor_shellhook_unsupported"
	// MsgDoctorShellHookUnresolved reports that the shell IS one comrade
	// supports, but its rc/profile file path could not be resolved (e.g.
	// HOME unset, or no PowerShell binary on PATH). One arg: the shell
	// name.
	MsgDoctorShellHookUnresolved MessageID = "doctor_shellhook_unresolved"
	// MsgDoctorShellHookMissing reports that the rc/profile file was
	// resolved, but comrade's block is absent or outdated in it. One arg:
	// the shell name.
	MsgDoctorShellHookMissing MessageID = "doctor_shellhook_missing"
	// MsgDoctorShellHookOK reports that comrade's current block is
	// already installed. One arg: the shell name.
	MsgDoctorShellHookOK MessageID = "doctor_shellhook_ok"

	// MsgDoctorKeySkipOllama fires when the active provider is ollama,
	// which needs no API key. No args.
	MsgDoctorKeySkipOllama MessageID = "doctor_key_skip_ollama"
	// MsgDoctorKeyFound reports that a credential was found for the
	// active provider. Two args: the provider name, the source it came
	// from ("keychain"/"file"/an env var name — left untranslated,
	// matching MsgAuthStatusSet/MsgAuthStatusSetEnv's own established
	// precedent).
	MsgDoctorKeyFound MessageID = "doctor_key_found"
	// MsgDoctorKeyMissing reports that no credential was found anywhere
	// for the active provider. One arg: the provider name.
	MsgDoctorKeyMissing MessageID = "doctor_key_missing"

	// MsgDoctorReachSkip fires when the active provider name is not one
	// this package recognizes (llm.HealthEndpoint returned ok=false) —
	// e.g. ConfigErr left Cfg.LLM.Provider empty. One arg: the provider
	// name (may be empty).
	MsgDoctorReachSkip MessageID = "doctor_reach_skip"
	// MsgDoctorReachFail reports a transport-level failure
	// (dial/TLS/timeout) — the provider's host could not be reached at
	// all. One arg: the provider name.
	MsgDoctorReachFail MessageID = "doctor_reach_fail"
	// MsgDoctorReachOllamaNoModels reports that ollama answered but
	// /api/tags lists zero locally-pulled models. No args.
	MsgDoctorReachOllamaNoModels MessageID = "doctor_reach_ollama_no_models"
	// MsgDoctorReachOK reports that the provider's host responded to the
	// keyless probe (any HTTP status — see llm.HealthEndpoint's own doc
	// comment on this package's "any status = reachable" rule). One arg:
	// the provider name.
	MsgDoctorReachOK MessageID = "doctor_reach_ok"
	// MsgDoctorReachLiveOK reports that --live's authenticated ping
	// succeeded. Two args: the provider name, the round-trip latency.
	MsgDoctorReachLiveOK MessageID = "doctor_reach_live_ok"
	// MsgDoctorReachLiveRejected reports that --live's authenticated ping
	// got a 401/403 (llm.ErrAuthRejected) — the configured key is wrong.
	// One arg: the provider name.
	MsgDoctorReachLiveRejected MessageID = "doctor_reach_live_rejected"
	// MsgDoctorReachLiveFailed reports that --live's authenticated ping
	// failed for any OTHER reason (network/timeout/5xx/parse) — the key
	// might still be fine. One arg: the provider name.
	MsgDoctorReachLiveFailed MessageID = "doctor_reach_live_failed"

	// MsgDoctorBaseURLSkip fires when the active provider is not
	// openai_compat, so this check does not apply. No args.
	MsgDoctorBaseURLSkip MessageID = "doctor_baseurl_skip"
	// MsgDoctorBaseURLOK fires when either llm.openai_compat.base_url was
	// already customized away from the shipped default, or the
	// stored/env key's prefix doesn't match any known non-OpenAI vendor.
	// No args.
	MsgDoctorBaseURLOK MessageID = "doctor_baseurl_ok"
	// MsgDoctorBaseURLSuspectedVendor reports that base_url is still the
	// shipped OpenAI default, but the resolved key's prefix looks like a
	// different vendor's key format. One arg: the suspected vendor name
	// (e.g. "Groq") — see internal/doctor's own key-prefix sniff table.
	MsgDoctorBaseURLSuspectedVendor MessageID = "doctor_baseurl_suspected_vendor"

	// MsgDoctorConfigLoadError reports that internal/cli/doctor.go's own
	// config load failed before any check ran. No args (the raw error
	// goes in Result.Detail).
	MsgDoctorConfigLoadError MessageID = "doctor_config_load_error"
	// MsgDoctorConfigFileFallback reports that config loaded fine, but no
	// OS keychain is reachable on this machine — credentials use the
	// 0600 file fallback instead. No args.
	MsgDoctorConfigFileFallback MessageID = "doctor_config_file_fallback"
	// MsgDoctorConfigOK reports that config loaded fine and an OS
	// keychain is reachable. No args.
	MsgDoctorConfigOK MessageID = "doctor_config_ok"

	// MsgDoctorFixLabel is the small "fix:"-style label printed before
	// every non-OK/non-Skip result's Result.Fix line. One arg: the fix
	// text itself (a plain string — see doctor.Result.Fix's own doc
	// comment on why it is never a MessageID).
	MsgDoctorFixLabel MessageID = "doctor_fix_label"
	// MsgDoctorDetailLabel is the small "detail:"-style label printed
	// before a result's Result.Detail line (COMRADE_DEBUG-gated in table
	// output; unconditional in --json — see newDoctorCmd's own doc
	// comment). One arg: the detail text itself.
	MsgDoctorDetailLabel MessageID = "doctor_detail_label"
	// MsgDoctorFailedSummary is `comrade doctor`'s own final error when
	// at least one check is SeverityFail (P-1's exit-code rule — see
	// doctorFailedError, internal/cli/doctor.go). One arg: how many
	// checks failed.
	MsgDoctorFailedSummary MessageID = "doctor_failed_summary"

	// MsgFlagLive is --live's --help description.
	MsgFlagLive MessageID = "flag_live"

	// --- undo ---
	//
	// `comrade undo` (internal/undo + internal/engine.Undoer +
	// internal/cli/undo.go): its two flag descriptions, its own usage
	// errors (no target run found, --run named an unrecognized id,
	// nothing in the target run was reversible at all), --list's table
	// header/empty-log message, and the notes buildUndoPlan attaches to a
	// step it skipped or downgraded to the LLM/manual tier — every one of
	// these in a single, clearly-marked block per the task's own request.

	// MsgFlagUndoRun is --run's --help description.
	MsgFlagUndoRun MessageID = "flag_undo_run"
	// MsgFlagUndoList is --list's --help description.
	MsgFlagUndoList MessageID = "flag_undo_list"

	// MsgUndoRunNotFoundError is `comrade undo --run <id>`'s error when no
	// recorded run matches the given id. One arg: the given id.
	MsgUndoRunNotFoundError MessageID = "undo_run_not_found_error"
	// MsgUndoNoTargetError is `comrade undo`'s error when no eligible
	// (non-legacy, not already undone) run exists in the audit log at
	// all.
	MsgUndoNoTargetError MessageID = "undo_no_target_error"
	// MsgUndoNothingReversibleError is `comrade undo`'s error when the
	// selected run's every step either failed (never took effect) or
	// otherwise has nothing left to reverse.
	MsgUndoNothingReversibleError MessageID = "undo_nothing_reversible_error"

	// MsgUndoListHeader is `comrade undo --list`'s table header row.
	MsgUndoListHeader MessageID = "undo_list_header"
	// MsgUndoListEmpty is printed instead of an empty table when the
	// audit log has no recorded runs at all.
	MsgUndoListEmpty MessageID = "undo_list_empty"

	// MsgUndoStepSkippedNote reports one step buildUndoPlan skipped
	// because its recorded exit code was nonzero (it never took effect).
	// Two args: the 1-based step number (within the run, oldest-first),
	// the original command text.
	MsgUndoStepSkippedNote MessageID = "undo_step_skipped_note"
	// MsgUndoStepDowngradedNote reports one step whose heuristic-derived
	// undo command was NOT trusted, because it uses a relative path and
	// the step's own recorded working directory differs from the
	// directory `comrade undo` is running in now — see internal/undo.
	// Derived.UsesRelativePath's own doc comment for why this is never
	// silently rewritten. Three args: the original command text, the
	// step's recorded working directory, the current working directory.
	MsgUndoStepDowngradedNote MessageID = "undo_step_downgraded_note"
	// MsgUndoLLMFallbackNote reports that at least one step in the target
	// run could not be resolved by the local heuristic table at all (or
	// was downgraded — see MsgUndoStepDowngradedNote), so the WHOLE
	// undo plan is being asked of the LLM tier instead of the
	// deterministic one, per internal/cli's own documented
	// all-or-nothing-per-run design (see runUndo/buildUndoPlan's doc
	// comment for why a partial heuristic/LLM merge was deliberately not
	// attempted).
	MsgUndoLLMFallbackNote MessageID = "undo_llm_fallback_note"

	// MsgUndoHeuristicRationale is the rationale text attached to a
	// heuristic-derived undo step (internal/undo.Derive found a
	// deterministic reversal, with no LLM call at all). One arg: the
	// original command text it reverses.
	MsgUndoHeuristicRationale MessageID = "undo_heuristic_rationale"
	// MsgUndoPlanSummary is the Summary text for a purely heuristic-
	// derived undo Plan (every eligible step in the run was resolved by
	// internal/undo.Derive — no LLM tier needed at all). Two args: how
	// many steps the derived plan has, the target run's own RunID.
	MsgUndoPlanSummary MessageID = "undo_plan_summary"

	// --- config profiles ---

	// MsgFlagProfile is the persistent root --profile flag's --help
	// description (root.go) — selects the active config profile for this
	// one invocation, taking precedence over COMRADE_PROFILE and the
	// file's own general.profile (see config.ResolveActiveProfile).
	MsgFlagProfile MessageID = "flag_profile"

	// MsgFlagProfileFromCurrent is `comrade config profile add`'s
	// --from-current flag's --help description.
	MsgFlagProfileFromCurrent MessageID = "flag_profile_from_current"

	// MsgConfigProfileListHeader is `comrade config profile list`'s table
	// header row.
	MsgConfigProfileListHeader MessageID = "config_profile_list_header"

	// MsgConfigProfileUsageError is the shared wrong-arity usage error for
	// every `comrade config profile <subcommand>` that takes a fixed
	// positional-argument shape (use/add/remove/set/show) — mirrors
	// MsgUsageNoArgsError/MsgUnknownSubcommandError's own "one shared,
	// parameterized message" pattern (argvalidation.go) instead of one
	// dedicated MessageID per subcommand. Two args: the resolved
	// cmd.CommandPath() (e.g. "comrade config profile use"), then a
	// literal argument-hint string built in Go (e.g. "<name>") — never a
	// raw fmt.Print literal itself, so this needs no catalogCoverageAllowlist
	// entry.
	MsgConfigProfileUsageError MessageID = "config_profile_usage_error"

	// MsgConfigProfileNotFound is config.ProfileNotFoundError's translated
	// rendering — `show`/`use`/`remove`/`set` all reject a name that
	// isn't a defined profile with this. One arg: the profile name.
	MsgConfigProfileNotFound MessageID = "config_profile_not_found"

	// MsgConfigProfileAlreadyExists is config.ProfileExistsError's
	// translated rendering — `add` never silently overwrites an existing
	// profile. One arg: the profile name.
	MsgConfigProfileAlreadyExists MessageID = "config_profile_already_exists"

	// MsgConfigProfileInvalidName is config.InvalidProfileNameError's
	// translated rendering — `add`/`use` reject a name that doesn't match
	// the required lowercase-letters/digits/-/_ shape. One arg: the
	// rejected name.
	MsgConfigProfileInvalidName MessageID = "config_profile_invalid_name"

	// MsgConfigProfileKeyNotAllowed is config.ProfileKeyNotAllowedError's
	// translated rendering — `set <name> general.profile ...` is rejected
	// (a profile activating another profile would be unbounded
	// recursion). One arg: the rejected key ("general.profile").
	MsgConfigProfileKeyNotAllowed MessageID = "config_profile_key_not_allowed"

	// MsgConfigProfileSafetyOverrideWarning is P-5's mandatory HIGHLIGHTED
	// warning: `profile use`/`profile show` print this whenever the
	// target profile overrides any safety.* key (config.ProfileSafetyOverrides).
	// This is a visibility warning only — the runtime destructive/elevated
	// confirmation gate itself is untouched by a profile switch; only
	// config's safety.confirm_destructive/confirm_elevated=false PLUS
	// --yolo together ever bypass it (CLAUDE.md's security exception).
	// Two args: the profile name, then the comma-joined list of
	// overridden safety.* keys.
	MsgConfigProfileSafetyOverrideWarning MessageID = "config_profile_safety_override_warning"

	// MsgConfigProfileActivated confirms `comrade config profile use
	// <name>` succeeded. One arg: the now-active profile name.
	MsgConfigProfileActivated MessageID = "config_profile_activated"

	// MsgConfigProfileAdded confirms `comrade config profile add <name>`
	// succeeded. One arg: the new profile name.
	MsgConfigProfileAdded MessageID = "config_profile_added"

	// MsgConfigProfileRemoved confirms `comrade config profile remove
	// <name>` succeeded. One arg: the removed profile name.
	MsgConfigProfileRemoved MessageID = "config_profile_removed"

	// MsgConfigProfileShowActive is `comrade config profile show
	// <name>`'s heading line when name is the currently-active profile.
	// One arg: the profile name.
	MsgConfigProfileShowActive MessageID = "config_profile_show_active"

	// MsgConfigProfileShowInactive is `comrade config profile show
	// <name>`'s heading line when name is NOT the currently-active
	// profile. One arg: the profile name.
	MsgConfigProfileShowInactive MessageID = "config_profile_show_inactive"

	// --- plan review ---
	//
	// The interactive plan-preview/edit screen (internal/tui/planreview.go
	// + internal/cli/planreview.go): its --review flag description, the
	// screen's own heading/legend/per-row markers, and reuses
	// MsgConfirmEditHeader/MsgAbortCanceled for its edit-mode header and
	// whole-review-canceled abort text respectively, rather than
	// duplicating either.

	// MsgFlagReview is --review's --help description.
	MsgFlagReview MessageID = "flag_review"
	// MsgFlagNoReview is --no-review's --help description.
	MsgFlagNoReview MessageID = "flag_no_review"

	// MsgPlanReviewHeader is the one-line heading printed above the
	// numbered step list.
	MsgPlanReviewHeader MessageID = "plan_review_header"
	// MsgPlanReviewLegend is the trailing line listing every available
	// action's key, in the active language.
	MsgPlanReviewLegend MessageID = "plan_review_legend"
	// MsgPlanReviewSkippedMarker is appended after a row's command text
	// once the user has toggled it to be skipped.
	MsgPlanReviewSkippedMarker MessageID = "plan_review_skipped_marker"
	// MsgPlanReviewBlockedMarker replaces a Blocked row's risk badge. One
	// arg: the block reason.
	MsgPlanReviewBlockedMarker MessageID = "plan_review_blocked_marker"
)

// catalogEN is the English catalog — also the fallback catalog every
// Translator falls back to for a key missing from the active language
// (see Translator.T). Every MsgXxx constant above MUST have an entry
// here; TestCatalogsCoverIdenticalKeys enforces that bidirectionally
// against catalogTR.
var catalogEN = Catalog{ // #nosec G101 -- this is a user-facing UI-text catalog (prompts/labels that mention "password"/"key" as words), not a literal credential
	MsgVersionBanner:  "comrade version %s\n\n",
	MsgFirstRunNotice: "Created default config at %s\n",
	MsgYoloWarning:    "--yolo is set: destructive/elevated steps may run WITHOUT confirmation in auto mode, if safety.confirm_destructive/confirm_elevated is also disabled in config.",

	MsgBlockedStep:     "%d. BLOCKED(%s): %s\n",
	MsgBlockedStepEdit: "BLOCKED(%s): %s\n",
	MsgYoloBypass:      "--yolo bypass: running %s step without confirmation: %s",

	MsgAbortCanceled:                  "canceled",
	MsgAbortStepBlocked:               "step %d is blocked: %s",
	MsgAbortStepFailed:                "step %d failed (exit %d): %s — %s",
	MsgAbortStepFailedAfterCorrection: "step %d failed after %d self-correction attempt(s) (exit %d): %s — %s",
	MsgRetrySuggestion:                "review the command and retry manually, or adjust the request",

	MsgExplainSafetyWarning:  "⚠ this command is classified %s by cli-comrade's local safety check%s — it may delete, overwrite, or otherwise irreversibly change your system.",
	MsgExplainSummaryHeading: "Summary:",
	MsgExplainPartsHeading:   "Breakdown:",
	MsgExplainRiskHeading:    "Risk note:",
	MsgExplainUsageError:     `usage: comrade explain <command...> (to explain a command that starts with a flag, e.g. --help, use "comrade explain -- <command>")`,

	MsgTestLLMResult:       "provider=%s model=%s latency=%s\n",
	MsgConfigListHeader:    "KEY\tVALUE\tSOURCE",
	MsgConfigSetUsageError: "usage: comrade config set <key> <value>",

	MsgConfigUnknownKey:      "unknown config key %q; valid keys are: %s",
	MsgConfigInvalidEnum:     "invalid value %q for %s; must be one of: %s",
	MsgConfigInvalidBool:     "invalid value %q for %s: must be a boolean (true/false)",
	MsgConfigInvalidInt:      "invalid value %q for %s: must be an integer",
	MsgConfigNotPositive:     "invalid value %q for %s: must be greater than 0",
	MsgConfigNotNonNegative:  "invalid value %q for %s: must be 0 or greater",
	MsgConfigInvalidURL:      "invalid value %q for %s: must be a valid http:// or https:// URL with a host",
	MsgConfigMetadataBlocked: "invalid value %q for %s: cloud metadata / link-local address not allowed",

	MsgModelsDocsNote:     "(static snapshot — see %s for the current list)\n",
	MsgModelsSelectPrompt: "Select a model number: ",
	MsgModelsSetConfirm:   "llm.model = %s\n",

	MsgChatWelcome:     "cli-comrade chat — type a message, or /help for commands.",
	MsgChatRequiresTTY: "comrade chat needs an interactive terminal (stdin is not a TTY) — run it directly in a terminal, not piped or redirected.",
	MsgChatHelp: "Slash commands:\n" +
		"  /mode auto|ask|info   switch this session's active mode\n" +
		"  /do <request>         run a request through the plan+execute pipeline (safety-gated, per the active mode)\n" +
		"  /clear                reset the conversation history\n" +
		"  /save <file>          export the transcript to <file> (the only way this session is ever written to disk)\n" +
		"  /help                 show this help\n" +
		"  /exit                 end the session",
	MsgChatModeUsage:      "usage: /mode auto|ask|info",
	MsgChatModeChanged:    "mode set to %s",
	MsgChatCleared:        "conversation history cleared",
	MsgChatSaveUsage:      "usage: /save <file>",
	MsgChatSaved:          "transcript saved to %s",
	MsgChatSaveFailed:     "save %s: %v",
	MsgChatDoUsage:        "usage: /do <request>",
	MsgChatUnknownCommand: "unknown command: %s (try /help)",
	MsgChatExiting:        "goodbye.",
	MsgChatLLMError:       "chat request failed: %v",
	MsgChatDoSummary:      "do:",

	MsgAuditRetentionFailed:  "audit: retention cleanup failed: %v\n",
	MsgPlanTableHeader:       "STEP\tCOMMAND\tRISK\tREVERSIBLE\tRATIONALE",
	MsgPlanBlockedCell:       "BLOCKED(%s)",
	MsgPlanConfirmCell:       "CONFIRM(%s)",
	MsgRunSummaryCounts:      "%d executed, %d skipped, %d blocked",
	MsgRunSummaryAbortedLine: "aborted: %s",

	MsgFixStaleNotice:           "the last recorded command is more than 10 minutes old; ignoring it and asking you to paste the error instead.",
	MsgFixExitZeroNotice:        "the last recorded command exited successfully (nothing to fix); asking you to paste the error instead.",
	MsgFixRefusalNotice:         "refusing to re-run %q: it is classified %s; paste the command and its error output instead.\n",
	MsgFixBlockedClassification: "blocked (denylisted)",
	MsgFixPasteIntro:            "No recent failed command available. Paste the failing command, then its error output (end with a blank line):",
	MsgFixPasteCommandPrompt:    "Command: ",
	MsgFixPasteErrorPrompt:      "Error output (end with a blank line):",
	MsgFixRootCauseHeading:      "Root cause:",
	MsgFixExplanationHeading:    "Explanation:",
	MsgVerificationSuggestion:   "\nSuggested verification: %s\n",
	MsgVerificationSucceeded:    "verification: %s succeeded",
	MsgVerificationStillFails:   "verification: %s still fails (exit %d)",

	MsgAuthEnterKeyPrompt:         "Enter API key for %s: ",
	MsgAuthStoredKeyPingFailed:    "Key saved ✓  Couldn't verify it right now (%v) — likely a network issue, not a bad key.\n",
	MsgAuthKeyRejected:            "The provider rejected this key for %s (%v) — it was not saved. Double-check the key and try \"comrade auth login %s\" again.\n",
	MsgAuthStoredKeyPingSucceeded: "Stored key for %s. Test request succeeded (model=%s, latency=%s).\n",
	MsgAuthStoredKeyBaseURLUnsafe: "Stored key for %s. Skipped the live test — %s (currently %q) is not a safe endpoint, so your key was never sent there; fix it with: comrade config set %s <valid-url>\n",
	MsgAuthNoStoredKey:            "No stored key for %s.\n",
	MsgAuthRemovedStoredKey:       "Removed stored key for %s.\n",
	MsgAuthStatusHeader:           "PROVIDER\tSTATUS",
	MsgAuthStatusOllamaRow:        "ollama\t(no key required)",
	MsgAuthStatusSet:              "set (%s)",
	MsgAuthStatusSetEnv:           "set (env: %s)",
	MsgAuthStatusNotSet:           "not set",

	MsgAuthOpenAICompatBaseURLPrompt: "Provider address (base_url) [current: %s]\n› Enter another provider's URL (e.g. Qwen → https://dashscope-intl.aliyuncs.com/compatible-mode/v1), or press Enter to keep it: ",
	MsgAuthOpenAICompatBaseURLSaved:  "Saved llm.openai_compat.base_url = %s\n",
	MsgAuthOpenAICompatModelPrompt:   "Model — enter this provider's model name (e.g. qwen-plus); leave empty to set it later with 'comrade config set llm.model': ",
	MsgAuthModelNotFound:             "Key saved ✓  But model '%s' doesn't exist on this provider.\n› Pick a model:  comrade config models   then:  comrade config set llm.model <model>\n",

	MsgSecretsFileFallbackWarning: "cli-comrade: no system keychain found, so API keys are being saved to a local file instead (base64-encoded, not encrypted — see the file's own header for details).\n",

	MsgHistoryTableHeader: "TIME\tMODE\tRISK\tEXIT\tCOMMAND",
	MsgHistoryEmpty:       "No commands recorded yet.",

	MsgInitPowerShellManualFallback: "%s\n\nCould not automatically locate a profile file to edit (%s).\nAdd the block above to your PowerShell profile manually.\n",
	MsgInitAlreadyInstalled:         "cli-comrade shell integration is already installed in %s\n",
	MsgInitPreview:                  "The following will be added to %s:\n\n%s\n\n",
	MsgInitConfirmPrompt:            "Add cli-comrade shell integration to %s? [y/N] ",
	MsgInitAborted:                  "Aborted; no changes made.",
	MsgInitInstalled:                "Installed cli-comrade shell integration in %s\n",
	MsgInitRemoveNoProfile:          "Nothing to remove: could not locate a profile file (%s).\n",
	MsgInitNotInstalled:             "cli-comrade shell integration is not installed in %s; nothing to do.\n",
	MsgInitRemoved:                  "Removed cli-comrade shell integration from %s\n",
	MsgInitFishCompletionsInstalled: "Installed shell completions for fish: %s\n",
	MsgInitFishCompletionsRemoved:   "Removed shell completions for fish: %s\n",

	MsgInitPSVariantAlreadyInstalled: "%s: cli-comrade shell integration is already installed in %s\n",
	MsgInitPSVariantInstalled:        "%s: Installed cli-comrade shell integration in %s\n",
	MsgInitPSVariantNotInstalled:     "%s: cli-comrade shell integration is not installed in %s; nothing to do.\n",
	MsgInitPSVariantRemoved:          "%s: Removed cli-comrade shell integration from %s\n",
	MsgInitPSVariantUnresolved:       "%s: could not resolve profile path (%s)\n",
	MsgInitConfirmPromptMulti:        "Add cli-comrade shell integration to the profile(s) above? [y/N] ",

	MsgHelpShortRoot:          "comrade is a cross-platform AI CLI companion for the terminal",
	MsgHelpShortDo:            "Generate a plan for a free-text request and run it per the active mode",
	MsgHelpShortFix:           "Diagnose the last failed command (or a given one) and fix it",
	MsgHelpShortExplain:       "Explain what a command does, flag by flag, without running it",
	MsgHelpShortChat:          "Start an interactive, context-preserving chat session",
	MsgHelpShortConfig:        "View and edit cli-comrade configuration",
	MsgHelpShortConfigGet:     "Print the effective value of a config key",
	MsgHelpShortConfigSet:     "Validate and persist a config key's value",
	MsgHelpShortConfigList:    "List every config key, its effective value, and its source",
	MsgHelpShortConfigEdit:    "Open the config file in $EDITOR",
	MsgHelpShortConfigPath:    "Print the config file path",
	MsgHelpShortConfigTestLLM: "Send a tiny test completion through the configured LLM provider chain",
	MsgHelpShortConfigModels:  "List models available for the active provider and select one",
	MsgHelpShortAuth:          "Manage stored LLM provider API keys (keychain, with a file fallback)",
	MsgHelpShortAuthLogin:     "Store an API key for a provider, then send a small test request",
	MsgHelpShortAuthLogout:    "Remove a stored API key",
	MsgHelpShortAuthStatus:    "Show which providers have a stored or environment API key",
	MsgHelpShortInit:          "Install shell integration hooks",
	MsgHelpShortHistory:       "Show recently executed commands from the audit log",
	MsgHelpShortHook:          "Internal hooks invoked by shell integration (not for direct use)",
	MsgHelpShortHookRecord:    "Record the last executed shell command (invoked by shell hooks)",

	MsgFlagDryRun: "print the generated plan without executing it",
	MsgFlagAuto:   "run in auto mode for this invocation (overrides COMRADE_MODE/config)",
	MsgFlagAsk:    "run in ask mode for this invocation (overrides COMRADE_MODE/config)",
	MsgFlagInfo:   "print the plan and explain it without executing anything",
	MsgFlagYolo:   "DANGEROUS: bypass destructive/elevated confirmation in auto mode when safety.confirm_destructive/confirm_elevated is also disabled in config",
	MsgFlagRerun:  "re-run the last recorded command to capture fresh output before diagnosing it",
	MsgFlagJSON:   "print each entry as a JSON object, one per line, instead of a table",
	MsgFlagLimit:  "maximum number of most-recent entries to show",
	MsgFlagPrint:  "Print the shell snippet only; make no file changes",
	MsgFlagRemove: "Remove the cli-comrade block from the shell rc/profile file",
	MsgFlagYes:    "Assume yes and skip the confirmation prompt",

	MsgAuthOllamaNoKeyError:          "auth login: ollama needs no API key — it talks to a local server directly; set llm.ollama.base_url instead",
	MsgAuthUnknownProviderError:      "auth login: unknown provider %q (expected one of: %s)",
	MsgAuthNoKeyEnteredError:         "auth login: no key entered",
	MsgAuthLoginRequiresTTY:          "auth login needs an interactive terminal (stdin is not a TTY) — run it directly in a terminal, not piped or redirected.",
	MsgLLMNoKeyError:                 "no API key configured for %s yet — run \"comrade auth login %s\" to set one up (or export its env var directly; see \"comrade auth login --help\")",
	MsgLLMBaseURLRejected:            "refusing to send your API key to %s (currently %q) — it is not a safe endpoint; fix it with: comrade config set %s <valid-url>",
	MsgFixRerunNoLastCommandError:    "--rerun: no recorded last command found; run a command with shell integration installed first, or use `comrade fix -- <command>`",
	MsgFlagsModeExclusiveError:       "only one of --auto, --ask, or --info may be given",
	MsgInitPrintRemoveExclusiveError: "init: --print and --remove are mutually exclusive",
	MsgInitShellUndetectedError:      "init: could not detect your shell; run e.g. \"comrade init bash\" explicitly",
	MsgInitShellUnsupportedError:     "init: detected shell %q is not supported; run \"comrade init bash|zsh|fish|powershell\" explicitly",
	MsgInitPowerShellNoneFoundError:  "init: no PowerShell installation found on this machine (neither \"powershell\" nor \"pwsh\" was found on PATH)",
	MsgModelsNoModelsError:           "config models: provider %q returned no models",
	MsgModelsUnknownProviderError:    "unknown provider %q",
	MsgModelsChoiceNotANumber:        "%q is not a number (expected 1-%d)",
	MsgModelsChoiceOutOfRange:        "%d is out of range (expected 1-%d)",

	MsgAuthLoginUsageError:    "usage: comrade auth login <provider> (expected one of: %s)",
	MsgAuthLogoutUsageError:   "usage: comrade auth logout <provider> (expected one of: %s)",
	MsgDoUsageError:           `usage: comrade do <request...> (e.g. comrade do "install docker")`,
	MsgInitUsageError:         "usage: comrade init [bash|zsh|fish|powershell]",
	MsgConfigGetUsageError:    "usage: comrade config get <key>",
	MsgUsageNoArgsError:       "%s does not take any arguments",
	MsgUnknownSubcommandError: "unknown subcommand %q for %s (expected one of: %s)",

	MsgUpgradeDevBuildError:          "upgrade: this is a dev build (no version was embedded at build time); install a released build to use `comrade upgrade`",
	MsgUpgradeUpToDate:               "you're already on the latest version (%s)\n",
	MsgUpgradeNewerAvailable:         "a newer version is available: %s (you have %s) — %s\n",
	MsgUpgradeDownloading:            "downloading comrade %s...\n",
	MsgUpgradeInstalled:              "updated to %s. Restart any running comrade session to pick it up.\n",
	MsgUpgradeNoReleaseFound:         "no published release of comrade is available yet — check back later",
	MsgUpgradeFetchFailed:            "could not reach GitHub to check for a newer version — try again later",
	MsgUpgradeSymlinkResolveWarning:  "cli-comrade: warning: could not resolve symlinks for %s (%v); replacing it directly\n",
	MsgUpgradeSignatureNotConfigured: "cli-comrade: release signature verification is not configured; proceeding with checksum-only verification\n",
	MsgUpgradeSignatureMissing:       "this release does not include a signature file; refusing to install it",
	MsgUpgradeSignatureInvalid:       "this release's signature could not be verified; refusing to install it",

	MsgHelpShortUpgrade: "Check for or install a newer released version of comrade",
	MsgFlagCheck:        "only report whether a newer version is available; do not download or install it",

	MsgHelpGroupCore:  "Core:",
	MsgHelpGroupSetup: "Setup:",
	MsgHelpGroupInfo:  "Info:",
	MsgHelpExamplesRoot: "  comrade install docker           # free-text request -> do mode\n" +
		"  comrade fix                      # diagnose the last failed command\n" +
		"  comrade explain \"git rebase -i HEAD~5\"\n" +
		"  comrade chat                     # start an interactive session",

	MsgHelpLabelUsage:                "Usage:",
	MsgHelpLabelAliases:              "Aliases:",
	MsgHelpLabelExamples:             "Examples:",
	MsgHelpLabelAvailableCommands:    "Available Commands:",
	MsgHelpLabelAdditionalCommands:   "Additional Commands:",
	MsgHelpLabelFlags:                "Flags:",
	MsgHelpLabelGlobalFlags:          "Global Flags:",
	MsgHelpLabelAdditionalHelpTopics: "Additional help topics:",
	MsgHelpMoreInfo:                  `Use "{{.CommandPath}} [command] --help" for more information about a command.`,

	MsgUpdateAvailableNotice: "\ncomrade: a new version is available: %s (you have %s). Run `comrade upgrade` to update.\n",

	MsgConfirmLegend:     "[y]es [n]o [e]dit [x]plain [a]ll: ",
	MsgConfirmEditHeader: "Edit command (enter to confirm, esc to cancel):\n",
	MsgSpinnerThinking:   "thinking…",

	// --- token usage ---
	MsgFlagUsage:             "show a per-request token usage and estimated cost summary",
	MsgUsageSummary:          "tokens: %s in / %s out across %d requests (%s/%s)",
	MsgUsageCostEstimate:     " · est. %s",
	MsgUsageCostLocal:        " · local",
	MsgChatUsageSessionTotal: "session total — %s",

	MsgDoctorVersionTitle:   "version",
	MsgDoctorPathTitle:      "PATH",
	MsgDoctorShellHookTitle: "shell integration",
	MsgDoctorKeyTitle:       "API key",
	MsgDoctorReachTitle:     "provider reachability",
	MsgDoctorBaseURLTitle:   "base_url sanity",
	MsgDoctorConfigTitle:    "config & keychain",

	MsgDoctorVersionDevSkip:    "dev build; version check skipped",
	MsgDoctorVersionFetchError: "could not check for a newer version",
	MsgDoctorVersionBehind:     "a newer version is available: %s (you have %s)",
	MsgDoctorVersionUpToDate:   "up to date (%s)",

	MsgDoctorPathNotFound: "%q was not found on PATH",
	MsgDoctorPathStale:    "PATH resolves to a different comrade binary than the one currently running (%s)",
	MsgDoctorPathOK:       "found on PATH (%s)",

	MsgDoctorShellHookUndetected:  "could not detect the current shell",
	MsgDoctorShellHookUnsupported: "shell %q is not one comrade integrates with",
	MsgDoctorShellHookUnresolved:  "could not resolve a profile/rc file for %s",
	MsgDoctorShellHookMissing:     "shell integration is not installed (or is outdated) for %s",
	MsgDoctorShellHookOK:          "shell integration installed for %s",

	MsgDoctorKeySkipOllama: "ollama needs no API key",
	MsgDoctorKeyFound:      "API key found for %s (%s)",
	MsgDoctorKeyMissing:    "no API key configured for %s",

	MsgDoctorReachSkip:           "%s: unknown provider; skipping reachability check",
	MsgDoctorReachFail:           "could not reach %s",
	MsgDoctorReachOllamaNoModels: "ollama is reachable but has no models pulled",
	MsgDoctorReachOK:             "%s is reachable",
	MsgDoctorReachLiveOK:         "%s is reachable; live ping succeeded (%s)",
	MsgDoctorReachLiveRejected:   "%s rejected the configured API key",
	MsgDoctorReachLiveFailed:     "live ping to %s failed (the key may still be valid)",

	MsgDoctorBaseURLSkip:            "active provider is not openai_compat; skipping",
	MsgDoctorBaseURLOK:              "base_url looks fine",
	MsgDoctorBaseURLSuspectedVendor: "llm.openai_compat.base_url is still OpenAI's default, but the configured key looks like a %s key",

	MsgDoctorConfigLoadError:    "config failed to load",
	MsgDoctorConfigFileFallback: "no OS keychain available; credentials are stored in a 0600 file instead",
	MsgDoctorConfigOK:           "config loaded and an OS keychain is available",

	MsgDoctorFixLabel:      "    fix: %s\n",
	MsgDoctorDetailLabel:   "    detail: %s\n",
	MsgDoctorFailedSummary: "comrade doctor: %d check(s) failed",

	MsgFlagLive: "send a real, minimal authenticated request to the active provider (spends a token; never on by default)",

	MsgFlagUndoRun:  "undo a specific recorded run by id, instead of the newest eligible one",
	MsgFlagUndoList: "list recent recorded runs instead of undoing one",

	MsgUndoRunNotFoundError:       "No recorded run matches --run %q.",
	MsgUndoNoTargetError:          "No reversible run was found in the audit log yet (or every recorded run has already been undone).",
	MsgUndoNothingReversibleError: "Every step in that run either failed or never took effect; there is nothing to undo.",

	MsgUndoListHeader: "RUN ID\tTIME\tSTEPS\tREQUEST",
	MsgUndoListEmpty:  "No recorded runs yet.",

	MsgUndoStepSkippedNote:    "step %d (%s): skipped — it exited with a nonzero status and never took effect",
	MsgUndoStepDowngradedNote: "step (%s): its automatic undo command uses a relative path, but it was recorded in %s while this undo is running in %s — asking the model instead of guessing",
	MsgUndoLLMFallbackNote:    "one or more steps could not be reversed with a built-in rule; asking the model for the whole undo plan instead",

	MsgUndoHeuristicRationale: "Reverses: %s",
	MsgUndoPlanSummary:        "Reverses %d step(s) from run %s, newest first.",

	// --- config profiles ---
	MsgFlagProfile:            "use this named config profile for this invocation (overrides COMRADE_PROFILE and general.profile)",
	MsgFlagProfileFromCurrent: "seed the new profile with the current file-level [llm] section's values",

	MsgConfigProfileListHeader: "PROFILE\tACTIVE\tKEYS",
	MsgConfigProfileUsageError: "usage: %s %s",

	MsgConfigProfileNotFound:      "profile %q is not defined; run \"comrade config profile list\" to see defined profiles",
	MsgConfigProfileAlreadyExists: "profile %q already exists",
	MsgConfigProfileInvalidName:   "invalid profile name %q: must start with a lowercase letter or digit and contain only lowercase letters, digits, - or _ (max 32 characters)",
	MsgConfigProfileKeyNotAllowed: "config key %q cannot be set inside a profile (it selects the active profile itself)",

	MsgConfigProfileSafetyOverrideWarning: "⚠ profile %q overrides safety setting(s): %s — these apply automatically whenever this profile is active",

	MsgConfigProfileActivated: "activated profile %q\n",
	MsgConfigProfileAdded:     "created profile %q\n",
	MsgConfigProfileRemoved:   "removed profile %q\n",

	MsgConfigProfileShowActive:   "profile %q (active)\n",
	MsgConfigProfileShowInactive: "profile %q\n",

	// --- plan review ---
	MsgFlagReview:   "show the full plan for review/edit before running it (reorder, skip, edit, or delete steps)",
	MsgFlagNoReview: "never show the plan review/edit screen for this run, even if general.plan_review is \"ask\"",

	MsgPlanReviewHeader:        "Review the plan below, then approve:\n",
	MsgPlanReviewLegend:        "[u]p [d]own [e]dit [r]emove [space] skip [a]pprove all [esc] cancel: ",
	MsgPlanReviewSkippedMarker: "[skipped]",
	MsgPlanReviewBlockedMarker: "BLOCKED(%s)",
}

// catalogTR is the Turkish catalog. Every message here is a natural,
// idiomatic Turkish translation — not a literal machine translation — of
// its catalogEN counterpart; see docs/history/phases/FAZ-09.md's translation
// notes for terminology choices (e.g. "yürütme" for "execution", "geri
// alınamaz" for "irreversible").
var catalogTR = Catalog{ // #nosec G101 -- this is a user-facing UI-text catalog (prompts/labels that mention "password"/"key" as words), not a literal credential
	MsgVersionBanner:  "comrade sürüm %s\n\n",
	MsgFirstRunNotice: "Varsayılan ayar dosyası oluşturuldu: %s\n",
	MsgYoloWarning:    "--yolo etkin: config'de safety.confirm_destructive/confirm_elevated de kapalıysa, auto modda destructive/elevated adımlar ONAY ALINMADAN çalışabilir.",

	MsgBlockedStep:     "%d. ENGELLENDİ(%s): %s\n",
	MsgBlockedStepEdit: "ENGELLENDİ(%s): %s\n",
	MsgYoloBypass:      "--yolo bypass: %s riskli adım onay alınmadan çalıştırılıyor: %s",

	MsgAbortCanceled:                  "iptal edildi",
	MsgAbortStepBlocked:               "%d. adım engellendi: %s",
	MsgAbortStepFailed:                "%d. adım başarısız oldu (çıkış kodu %d): %s — %s",
	MsgAbortStepFailedAfterCorrection: "%d. adım %d kendi kendini düzeltme denemesinden sonra başarısız oldu (çıkış kodu %d): %s — %s",
	MsgRetrySuggestion:                "komutu gözden geçirip elle tekrar deneyin, ya da isteği değiştirin",

	MsgExplainSafetyWarning:  "⚠ bu komut cli-comrade'in yerel güvenlik kontrolüne göre %s sınıfında%s — sisteminizde geri alınamaz silme/üzerine yazma veya başka kalıcı bir değişikliğe yol açabilir.",
	MsgExplainSummaryHeading: "Özet:",
	MsgExplainPartsHeading:   "Parça parça açıklama:",
	MsgExplainRiskHeading:    "Risk notu:",
	MsgExplainUsageError:     `kullanım: comrade explain <komut...> (--help gibi bayrakla başlayan bir komutu açıklamak için "comrade explain -- <komut>" kullanın)`,

	MsgTestLLMResult:       "sağlayıcı=%s model=%s gecikme=%s\n",
	MsgConfigListHeader:    "ANAHTAR\tDEĞER\tKAYNAK",
	MsgConfigSetUsageError: "kullanım: comrade config set <anahtar> <değer>",

	MsgConfigUnknownKey:      "bilinmeyen config anahtarı %q; geçerli anahtarlar: %s",
	MsgConfigInvalidEnum:     "%q değeri %s için geçersiz; şunlardan biri olmalı: %s",
	MsgConfigInvalidBool:     "%q değeri %s için geçersiz: bir boolean olmalı (true/false)",
	MsgConfigInvalidInt:      "%q değeri %s için geçersiz: bir tam sayı olmalı",
	MsgConfigNotPositive:     "%q değeri %s için geçersiz: 0'dan büyük olmalı",
	MsgConfigNotNonNegative:  "%q değeri %s için geçersiz: 0 veya daha büyük olmalı",
	MsgConfigInvalidURL:      "%q değeri %s için geçersiz: geçerli bir http:// veya https:// URL'si olmalı ve bir host içermeli",
	MsgConfigMetadataBlocked: "%q değeri %s için geçersiz: bulut metadata / link-local adresine izin verilmiyor",

	MsgModelsDocsNote:     "(sabit bir liste — güncel liste için: %s)\n",
	MsgModelsSelectPrompt: "Bir model numarası seçin: ",
	MsgModelsSetConfirm:   "llm.model = %s\n",

	MsgChatWelcome:     "cli-comrade sohbet — bir mesaj yazın, ya da komutlar için /help yazın.",
	MsgChatRequiresTTY: "comrade chat, etkileşimli bir terminal gerektirir (stdin bir TTY değil) — doğrudan bir terminalde çalıştırın, boru hattına yönlendirmeyin.",
	MsgChatHelp: "Slash komutları:\n" +
		"  /mode auto|ask|info   bu oturumun aktif modunu değiştirir\n" +
		"  /do <istek>           isteği plan+yürütme hattından geçirir (aktif moda göre güvenlik kontrollü)\n" +
		"  /clear                konuşma geçmişini sıfırlar\n" +
		"  /save <dosya>         dökümü <dosya>'ya aktarır (bu oturumun diske yazıldığı TEK yol budur)\n" +
		"  /help                 bu yardımı gösterir\n" +
		"  /exit                 oturumu sonlandırır",
	MsgChatModeUsage:      "kullanım: /mode auto|ask|info",
	MsgChatModeChanged:    "mod %s olarak ayarlandı",
	MsgChatCleared:        "konuşma geçmişi temizlendi",
	MsgChatSaveUsage:      "kullanım: /save <dosya>",
	MsgChatSaved:          "döküm şuraya kaydedildi: %s",
	MsgChatSaveFailed:     "kaydetme hatası %s: %v",
	MsgChatDoUsage:        "kullanım: /do <istek>",
	MsgChatUnknownCommand: "bilinmeyen komut: %s (yardım için /help)",
	MsgChatExiting:        "hoşça kalın.",
	MsgChatLLMError:       "sohbet isteği başarısız oldu: %v",
	MsgChatDoSummary:      "yap:",

	MsgAuditRetentionFailed:  "audit: saklama temizliği başarısız oldu: %v\n",
	MsgPlanTableHeader:       "ADIM\tKOMUT\tRİSK\tGERİ ALINABİLİR\tGEREKÇE",
	MsgPlanBlockedCell:       "ENGELLENDİ(%s)",
	MsgPlanConfirmCell:       "ONAY(%s)",
	MsgRunSummaryCounts:      "%d çalıştırıldı, %d atlandı, %d engellendi",
	MsgRunSummaryAbortedLine: "durduruldu: %s",

	MsgFixStaleNotice:           "son kaydedilen komut 10 dakikadan daha eski; yok sayılıyor ve hatayı yapıştırmanız isteniyor.",
	MsgFixExitZeroNotice:        "son kaydedilen komut başarıyla tamamlanmış (düzeltilecek bir şey yok); hatayı yapıştırmanız isteniyor.",
	MsgFixRefusalNotice:         "%q yeniden çalıştırılması reddediliyor: %s olarak sınıflandırıldı; bunun yerine komutu ve hata çıktısını yapıştırın.\n",
	MsgFixBlockedClassification: "engellendi (kara listede)",
	MsgFixPasteIntro:            "Yakın zamanda başarısız olmuş bir komut yok. Başarısız olan komutu, ardından hata çıktısını yapıştırın (boş bir satırla bitirin):",
	MsgFixPasteCommandPrompt:    "Komut: ",
	MsgFixPasteErrorPrompt:      "Hata çıktısı (boş bir satırla bitirin):",
	MsgFixRootCauseHeading:      "Kök neden:",
	MsgFixExplanationHeading:    "Açıklama:",
	MsgVerificationSuggestion:   "\nÖnerilen doğrulama: %s\n",
	MsgVerificationSucceeded:    "doğrulama: %s başarılı oldu",
	MsgVerificationStillFails:   "doğrulama: %s hâlâ başarısız (çıkış %d)",

	MsgAuthEnterKeyPrompt:         "%s için API anahtarını girin: ",
	MsgAuthStoredKeyPingFailed:    "Anahtar kaydedildi ✓  Şimdi doğrulanamadı (%v) — ağ/bağlantı olabilir, anahtarın yanlış olduğu anlamına gelmez.\n",
	MsgAuthKeyRejected:            "%s için bu anahtar sağlayıcı tarafından reddedildi (%v) — kaydedilmedi. Anahtarı kontrol edip \"comrade auth login %s\" komutunu tekrar deneyin.\n",
	MsgAuthStoredKeyPingSucceeded: "%s için anahtar kaydedildi. Test isteği başarılı oldu (model=%s, gecikme=%s).\n",
	MsgAuthStoredKeyBaseURLUnsafe: "%s için anahtar kaydedildi. Canlı test atlandı — %s (şu an %q) güvenli bir uç nokta değil, bu yüzden anahtarınız oraya hiç gönderilmedi; düzeltmek için: comrade config set %s <geçerli-url>\n",
	MsgAuthNoStoredKey:            "%s için kayıtlı anahtar yok.\n",
	MsgAuthRemovedStoredKey:       "%s için kayıtlı anahtar kaldırıldı.\n",
	MsgAuthStatusHeader:           "SAĞLAYICI\tDURUM",
	MsgAuthStatusOllamaRow:        "ollama\t(anahtar gerekmez)",
	MsgAuthStatusSet:              "kayıtlı (%s)",
	MsgAuthStatusSetEnv:           "kayıtlı (ortam değişkeni: %s)",
	MsgAuthStatusNotSet:           "kayıtlı değil",

	MsgAuthOpenAICompatBaseURLPrompt: "Sağlayıcı adresi (base_url) [şu an: %s]\n› Farklı sağlayıcı için adresini gir (ör. Qwen → https://dashscope-intl.aliyuncs.com/compatible-mode/v1), yoksa Enter: ",
	MsgAuthOpenAICompatBaseURLSaved:  "llm.openai_compat.base_url = %s olarak kaydedildi\n",
	MsgAuthOpenAICompatModelPrompt:   "Model — bu sağlayıcının model adını gir (ör. qwen-plus); boş bırakırsan sonra 'comrade config set llm.model' ile ayarla: ",
	MsgAuthModelNotFound:             "Anahtar kaydedildi ✓  Ama '%s' modeli bu sağlayıcıda yok.\n› Modeli seç:  comrade config models   sonra:  comrade config set llm.model <model>\n",

	MsgSecretsFileFallbackWarning: "cli-comrade: sistem anahtarlığı bulunamadı, bu yüzden API anahtarları yerel bir dosyaya kaydediliyor (base64 ile kodlanmış, şifrelenmemiş — ayrıntılar için dosyanın kendi başlığına bakın).\n",

	MsgHistoryTableHeader: "ZAMAN\tMOD\tRİSK\tÇIKIŞ\tKOMUT",
	MsgHistoryEmpty:       "Henüz kayıtlı komut yok.",

	MsgInitPowerShellManualFallback: "%s\n\nDüzenlenecek bir profil dosyası otomatik olarak bulunamadı (%s).\nYukarıdaki bloğu PowerShell profilinize elle ekleyin.\n",
	MsgInitAlreadyInstalled:         "cli-comrade kabuk entegrasyonu zaten %s içinde kurulu\n",
	MsgInitPreview:                  "Şu blok %s dosyasına eklenecek:\n\n%s\n\n",
	MsgInitConfirmPrompt:            "cli-comrade kabuk entegrasyonu %s dosyasına eklensin mi? [e/H] ",
	MsgInitAborted:                  "Vazgeçildi; hiçbir değişiklik yapılmadı.",
	MsgInitInstalled:                "cli-comrade kabuk entegrasyonu %s içine kuruldu\n",
	MsgInitRemoveNoProfile:          "Kaldırılacak bir şey yok: bir profil dosyası bulunamadı (%s).\n",
	MsgInitNotInstalled:             "cli-comrade kabuk entegrasyonu %s içinde kurulu değil; yapılacak bir şey yok.\n",
	MsgInitRemoved:                  "cli-comrade kabuk entegrasyonu %s içinden kaldırıldı\n",
	MsgInitFishCompletionsInstalled: "fish için kabuk tamamlama kuruldu: %s\n",
	MsgInitFishCompletionsRemoved:   "fish için kabuk tamamlama kaldırıldı: %s\n",

	MsgInitPSVariantAlreadyInstalled: "%s: cli-comrade kabuk entegrasyonu zaten %s içinde kurulu\n",
	MsgInitPSVariantInstalled:        "%s: cli-comrade kabuk entegrasyonu %s içine kuruldu\n",
	MsgInitPSVariantNotInstalled:     "%s: cli-comrade kabuk entegrasyonu %s içinde kurulu değil; yapılacak bir şey yok.\n",
	MsgInitPSVariantRemoved:          "%s: cli-comrade kabuk entegrasyonu %s içinden kaldırıldı\n",
	MsgInitPSVariantUnresolved:       "%s: profil yolu çözülemedi (%s)\n",
	MsgInitConfirmPromptMulti:        "Yukarıdaki profil(ler)e cli-comrade kabuk entegrasyonu eklensin mi? [e/H] ",

	MsgHelpShortRoot:          "comrade, terminalde çapraz platform çalışan bir yapay zeka CLI yoldaşıdır",
	MsgHelpShortDo:            "Serbest metinli bir istek için plan üretir ve aktif moda göre çalıştırır",
	MsgHelpShortFix:           "Son başarısız komutu (veya verilen bir komutu) teşhis eder ve düzeltir",
	MsgHelpShortExplain:       "Bir komutun ne yaptığını, bayrak bayrak, onu çalıştırmadan açıklar",
	MsgHelpShortChat:          "Bağlamı koruyan, etkileşimli bir sohbet oturumu başlatır",
	MsgHelpShortConfig:        "cli-comrade yapılandırmasını görüntüler ve düzenler",
	MsgHelpShortConfigGet:     "Bir yapılandırma anahtarının geçerli değerini yazdırır",
	MsgHelpShortConfigSet:     "Bir yapılandırma anahtarının değerini doğrular ve kaydeder",
	MsgHelpShortConfigList:    "Her yapılandırma anahtarını, geçerli değerini ve kaynağını listeler",
	MsgHelpShortConfigEdit:    "Yapılandırma dosyasını $EDITOR ile açar",
	MsgHelpShortConfigPath:    "Yapılandırma dosyasının yolunu yazdırır",
	MsgHelpShortConfigTestLLM: "Yapılandırılmış LLM sağlayıcı zinciri üzerinden küçük bir test isteği gönderir",
	MsgHelpShortConfigModels:  "Aktif sağlayıcı için kullanılabilir modelleri listeler ve birini seçmenizi sağlar",
	MsgHelpShortAuth:          "Kayıtlı LLM sağlayıcı API anahtarlarını yönetir (anahtar zinciri, dosya yedeğiyle)",
	MsgHelpShortAuthLogin:     "Bir sağlayıcı için API anahtarı kaydeder, ardından küçük bir test isteği gönderir",
	MsgHelpShortAuthLogout:    "Kayıtlı bir API anahtarını kaldırır",
	MsgHelpShortAuthStatus:    "Hangi sağlayıcıların kayıtlı veya ortam değişkeninde API anahtarı olduğunu gösterir",
	MsgHelpShortInit:          "Kabuk (shell) entegrasyon kancalarını kurar",
	MsgHelpShortHistory:       "Denetim kaydından son çalıştırılan komutları gösterir",
	MsgHelpShortHook:          "Kabuk entegrasyonu tarafından çağrılan dahili kancalar (doğrudan kullanım için değildir)",
	MsgHelpShortHookRecord:    "Son çalıştırılan kabuk komutunu kaydeder (kabuk kancaları tarafından çağrılır)",

	MsgFlagDryRun: "üretilen planı çalıştırmadan yazdırır",
	MsgFlagAuto:   "bu çalıştırma için auto modda çalışır (COMRADE_MODE/config ayarını geçersiz kılar)",
	MsgFlagAsk:    "bu çalıştırma için ask modda çalışır (COMRADE_MODE/config ayarını geçersiz kılar)",
	MsgFlagInfo:   "planı yazdırır ve açıklar, hiçbir şey çalıştırmaz",
	MsgFlagYolo:   "TEHLİKELİ: config'de safety.confirm_destructive/confirm_elevated de kapalıysa, auto modda destructive/elevated onayını atlar",
	MsgFlagRerun:  "teşhis etmeden önce son kaydedilen komutu yeniden çalıştırarak güncel çıktı yakalar",
	MsgFlagJSON:   "her kaydı tablo yerine satır başına bir JSON nesnesi olarak yazdırır",
	MsgFlagLimit:  "gösterilecek en yeni kayıtların azami sayısı",
	MsgFlagPrint:  "Yalnızca kabuk (shell) parçacığını yazdırır; hiçbir dosyada değişiklik yapmaz",
	MsgFlagRemove: "cli-comrade bloğunu kabuk rc/profil dosyasından kaldırır",
	MsgFlagYes:    "Evet varsayar ve onay istemini atlar",

	MsgAuthOllamaNoKeyError:          "auth login: ollama API anahtarı gerektirmez — doğrudan yerel bir sunucuyla konuşur; bunun yerine llm.ollama.base_url ayarını yapın",
	MsgAuthUnknownProviderError:      "auth login: bilinmeyen sağlayıcı %q (beklenen: %s)",
	MsgAuthNoKeyEnteredError:         "auth login: hiçbir anahtar girilmedi",
	MsgAuthLoginRequiresTTY:          "auth login, etkileşimli bir terminal gerektirir (stdin bir TTY değil) — doğrudan bir terminalde çalıştırın, boru hattına yönlendirmeyin.",
	MsgLLMNoKeyError:                 "%s için henüz bir API anahtarı yapılandırılmamış — kurmak için \"comrade auth login %s\" çalıştırın (ya da doğrudan ortam değişkenini ayarlayın; ayrıntılar için \"comrade auth login --help\")",
	MsgLLMBaseURLRejected:            "API anahtarınız %s (şu an %q) adresine gönderilmeyecek — güvenli bir uç nokta değil; düzeltmek için: comrade config set %s <geçerli-url>",
	MsgFixRerunNoLastCommandError:    "--rerun: kayıtlı son komut bulunamadı; önce kabuk entegrasyonu kurulu bir komut çalıştırın ya da `comrade fix -- <komut>` kullanın",
	MsgFlagsModeExclusiveError:       "--auto, --ask veya --info bayraklarından yalnızca biri verilebilir",
	MsgInitPrintRemoveExclusiveError: "init: --print ve --remove birlikte kullanılamaz",
	MsgInitShellUndetectedError:      "init: kabuğunuz tespit edilemedi; örneğin \"comrade init bash\" komutunu açıkça çalıştırın",
	MsgInitShellUnsupportedError:     "init: tespit edilen kabuk %q desteklenmiyor; \"comrade init bash|zsh|fish|powershell\" komutunu açıkça çalıştırın",
	MsgInitPowerShellNoneFoundError:  "init: bu makinede hiçbir PowerShell kurulumu bulunamadı (PATH üzerinde ne \"powershell\" ne de \"pwsh\" bulundu)",
	MsgModelsNoModelsError:           "config models: %q sağlayıcısı hiçbir model döndürmedi",
	MsgModelsUnknownProviderError:    "bilinmeyen sağlayıcı %q",
	MsgModelsChoiceNotANumber:        "%q bir sayı değil (beklenen: 1-%d)",
	MsgModelsChoiceOutOfRange:        "%d aralık dışında (beklenen: 1-%d)",

	MsgAuthLoginUsageError:    "kullanım: comrade auth login <sağlayıcı> (beklenen: %s)",
	MsgAuthLogoutUsageError:   "kullanım: comrade auth logout <sağlayıcı> (beklenen: %s)",
	MsgDoUsageError:           `kullanım: comrade do <istek...> (örnek: comrade do "docker kur")`,
	MsgInitUsageError:         "kullanım: comrade init [bash|zsh|fish|powershell]",
	MsgConfigGetUsageError:    "kullanım: comrade config get <anahtar>",
	MsgUsageNoArgsError:       "%s hiçbir argüman almaz",
	MsgUnknownSubcommandError: "%q: %s için bilinmeyen alt komut (beklenen: %s)",

	MsgUpgradeDevBuildError:          "upgrade: bu bir geliştirme (dev) derlemesi (derleme zamanında bir sürüm gömülmemiş); `comrade upgrade` kullanmak için yayımlanmış bir derleme kurun",
	MsgUpgradeUpToDate:               "zaten en güncel sürümdesiniz (%s)\n",
	MsgUpgradeNewerAvailable:         "daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s) — %s\n",
	MsgUpgradeDownloading:            "comrade %s indiriliyor...\n",
	MsgUpgradeInstalled:              "%s sürümüne güncellendi. Çalışan bir comrade oturumu varsa bunu yansıtması için yeniden başlatın.\n",
	MsgUpgradeNoReleaseFound:         "henüz yayımlanmış bir comrade sürümü yok — daha sonra tekrar kontrol edin",
	MsgUpgradeFetchFailed:            "daha yeni bir sürüm kontrol edilirken GitHub'a ulaşılamadı — daha sonra tekrar deneyin",
	MsgUpgradeSymlinkResolveWarning:  "cli-comrade: uyarı: %s için sembolik bağlar çözülemedi (%v); doğrudan bu yol değiştiriliyor\n",
	MsgUpgradeSignatureNotConfigured: "cli-comrade: sürüm imza doğrulaması yapılandırılmamış; yalnızca sağlama toplamı (checksum) ile doğrulamaya devam ediliyor\n",
	MsgUpgradeSignatureMissing:       "bu sürüm bir imza dosyası içermiyor; kurulum reddediliyor",
	MsgUpgradeSignatureInvalid:       "bu sürümün imzası doğrulanamadı; kurulum reddediliyor",

	MsgHelpShortUpgrade: "comrade'in daha yeni bir yayımlanmış sürümünü denetler veya kurar",
	MsgFlagCheck:        "yalnızca daha yeni bir sürüm olup olmadığını bildirir; indirmez veya kurmaz",

	MsgHelpGroupCore:  "Temel:",
	MsgHelpGroupSetup: "Kurulum:",
	MsgHelpGroupInfo:  "Bilgi:",
	MsgHelpExamplesRoot: "  comrade docker kur                # serbest metin istek -> do modu\n" +
		"  comrade fix                       # son başarısız komutu teşhis et\n" +
		"  comrade explain \"git rebase -i HEAD~5\"\n" +
		"  comrade chat                      # etkileşimli oturum başlat",

	MsgHelpLabelUsage:                "Kullanım:",
	MsgHelpLabelAliases:              "Takma adlar:",
	MsgHelpLabelExamples:             "Örnekler:",
	MsgHelpLabelAvailableCommands:    "Kullanılabilir Komutlar:",
	MsgHelpLabelAdditionalCommands:   "Ek Komutlar:",
	MsgHelpLabelFlags:                "Bayraklar:",
	MsgHelpLabelGlobalFlags:          "Genel Bayraklar:",
	MsgHelpLabelAdditionalHelpTopics: "Ek yardım konuları:",
	MsgHelpMoreInfo:                  `Bir komut hakkında daha fazla bilgi için "{{.CommandPath}} [command] --help" kullanın.`,

	MsgUpdateAvailableNotice: "\ncomrade: daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s). Güncellemek için `comrade upgrade` çalıştırın.\n",

	MsgConfirmLegend:     "[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü: ",
	MsgConfirmEditHeader: "Komutu düzenle (onaylamak için enter, iptal için esc):\n",
	MsgSpinnerThinking:   "düşünüyorum…",

	// --- token usage ---
	MsgFlagUsage:             "bu çalıştırma için token kullanımı ve tahmini maliyeti göster",
	MsgUsageSummary:          "token: %s giriş / %s çıkış, %d istekte (%s/%s)",
	MsgUsageCostEstimate:     " · tah. %s",
	MsgUsageCostLocal:        " · yerel",
	MsgChatUsageSessionTotal: "oturum toplamı — %s",

	MsgDoctorVersionTitle:   "sürüm",
	MsgDoctorPathTitle:      "PATH",
	MsgDoctorShellHookTitle: "kabuk entegrasyonu",
	MsgDoctorKeyTitle:       "API anahtarı",
	MsgDoctorReachTitle:     "sağlayıcıya erişilebilirlik",
	MsgDoctorBaseURLTitle:   "base_url tutarlılığı",
	MsgDoctorConfigTitle:    "yapılandırma ve anahtarlık",

	MsgDoctorVersionDevSkip:    "geliştirme sürümü; sürüm kontrolü atlandı",
	MsgDoctorVersionFetchError: "daha yeni bir sürüm olup olmadığı kontrol edilemedi",
	MsgDoctorVersionBehind:     "daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s)",
	MsgDoctorVersionUpToDate:   "güncel (%s)",

	MsgDoctorPathNotFound: "%q, PATH üzerinde bulunamadı",
	MsgDoctorPathStale:    "PATH, şu anda çalışan comrade ikili dosyasından farklı bir kopyaya işaret ediyor (%s)",
	MsgDoctorPathOK:       "PATH üzerinde bulundu (%s)",

	MsgDoctorShellHookUndetected:  "mevcut kabuk tespit edilemedi",
	MsgDoctorShellHookUnsupported: "%q kabuğu comrade'ın entegre olduğu kabuklardan biri değil",
	MsgDoctorShellHookUnresolved:  "%s için bir profil/rc dosyası çözümlenemedi",
	MsgDoctorShellHookMissing:     "%s için kabuk entegrasyonu kurulu değil (ya da güncel değil)",
	MsgDoctorShellHookOK:          "%s için kabuk entegrasyonu kurulu",

	MsgDoctorKeySkipOllama: "ollama için API anahtarı gerekmez",
	MsgDoctorKeyFound:      "%s için API anahtarı bulundu (%s)",
	MsgDoctorKeyMissing:    "%s için API anahtarı yapılandırılmamış",

	MsgDoctorReachSkip:           "%s: bilinmeyen sağlayıcı; erişilebilirlik kontrolü atlanıyor",
	MsgDoctorReachFail:           "%s adresine erişilemedi",
	MsgDoctorReachOllamaNoModels: "ollama'ya erişilebiliyor ama hiç model indirilmemiş",
	MsgDoctorReachOK:             "%s adresine erişilebiliyor",
	MsgDoctorReachLiveOK:         "%s adresine erişilebiliyor; canlı ping başarılı (%s)",
	MsgDoctorReachLiveRejected:   "%s, yapılandırılan API anahtarını reddetti",
	MsgDoctorReachLiveFailed:     "%s adresine canlı ping başarısız oldu (anahtar yine de geçerli olabilir)",

	MsgDoctorBaseURLSkip:            "aktif sağlayıcı openai_compat değil; atlanıyor",
	MsgDoctorBaseURLOK:              "base_url sorunsuz görünüyor",
	MsgDoctorBaseURLSuspectedVendor: "llm.openai_compat.base_url hâlâ OpenAI'nin varsayılanı, ama yapılandırılan anahtar bir %s anahtarına benziyor",

	MsgDoctorConfigLoadError:    "yapılandırma yüklenemedi",
	MsgDoctorConfigFileFallback: "kullanılabilir bir işletim sistemi anahtarlığı yok; kimlik bilgileri bunun yerine 0600 izinli bir dosyada saklanıyor",
	MsgDoctorConfigOK:           "yapılandırma yüklendi ve bir işletim sistemi anahtarlığı kullanılabilir",

	MsgDoctorFixLabel:      "    çözüm: %s\n",
	MsgDoctorDetailLabel:   "    ayrıntı: %s\n",
	MsgDoctorFailedSummary: "comrade doctor: %d kontrol başarısız oldu",

	MsgFlagLive: "aktif sağlayıcıya gerçek, minimal bir kimlik doğrulamalı istek gönder (bir token harcar; asla varsayılan olarak açık değildir)",

	MsgFlagUndoRun:  "en yeni uygun çalıştırma yerine, belirli bir kayıtlı çalıştırmayı kimliğine göre geri al",
	MsgFlagUndoList: "bir çalıştırmayı geri almak yerine son kayıtlı çalıştırmaları listele",

	MsgUndoRunNotFoundError:       "--run %q ile eşleşen kayıtlı bir çalıştırma yok.",
	MsgUndoNoTargetError:          "Denetim kaydında henüz geri alınabilir bir çalıştırma bulunamadı (ya da tüm kayıtlı çalıştırmalar zaten geri alındı).",
	MsgUndoNothingReversibleError: "Bu çalıştırmadaki her adım ya başarısız oldu ya da hiçbir etki yaratmadı; geri alınacak bir şey yok.",

	MsgUndoListHeader: "ÇALIŞTIRMA ID\tZAMAN\tADIM\tİSTEK",
	MsgUndoListEmpty:  "Henüz kayıtlı bir çalıştırma yok.",

	MsgUndoStepSkippedNote:    "adım %d (%s): atlandı — sıfırdan farklı bir durumla sonuçlandı ve hiçbir etki yaratmadı",
	MsgUndoStepDowngradedNote: "adım (%s): otomatik geri alma komutu göreli bir yol kullanıyor, ancak %s dizininde kaydedildi ve bu geri alma %s dizininde çalışıyor — tahmin etmek yerine modele soruluyor",
	MsgUndoLLMFallbackNote:    "bir veya daha fazla adım yerleşik bir kuralla geri alınamadı; bunun yerine tüm geri alma planı modele soruluyor",

	MsgUndoHeuristicRationale: "Geri alınan: %s",
	MsgUndoPlanSummary:        "%d adımı, %s çalıştırmasından en yeniden en eskiye doğru geri alır.",

	// --- config profiles ---
	MsgFlagProfile:            "bu çalıştırma için bu adlandırılmış config profilini kullan (COMRADE_PROFILE ve general.profile'ı geçersiz kılar)",
	MsgFlagProfileFromCurrent: "yeni profili mevcut dosya seviyesindeki [llm] bölümünün değerleriyle doldur",

	MsgConfigProfileListHeader: "PROFİL\tAKTİF\tANAHTAR",
	MsgConfigProfileUsageError: "kullanım: %s %s",

	MsgConfigProfileNotFound:      "%q profili tanımlı değil; tanımlı profilleri görmek için \"comrade config profile list\" çalıştırın",
	MsgConfigProfileAlreadyExists: "%q profili zaten var",
	MsgConfigProfileInvalidName:   "%q geçersiz profil adı: küçük harf veya rakamla başlamalı ve yalnızca küçük harf, rakam, - veya _ içermeli (en fazla 32 karakter)",
	MsgConfigProfileKeyNotAllowed: "%q config anahtarı bir profil içinde ayarlanamaz (aktif profili seçen anahtarın kendisi budur)",

	MsgConfigProfileSafetyOverrideWarning: "⚠ %q profili şu safety ayarlarının üzerine yazıyor: %s — bu profil aktifken bunlar otomatik olarak uygulanır",

	MsgConfigProfileActivated: "%q profili etkinleştirildi\n",
	MsgConfigProfileAdded:     "%q profili oluşturuldu\n",
	MsgConfigProfileRemoved:   "%q profili kaldırıldı\n",

	MsgConfigProfileShowActive:   "profil %q (aktif)\n",
	MsgConfigProfileShowInactive: "profil %q\n",

	// --- plan review ---
	MsgFlagReview:   "çalıştırmadan önce tam planı gözden geçirme/düzenleme ekranını göster (adımları yeniden sırala, atla, düzenle veya sil)",
	MsgFlagNoReview: "general.plan_review \"ask\" olsa bile bu çalıştırma için plan gözden geçirme/düzenleme ekranını asla gösterme",

	MsgPlanReviewHeader:        "Aşağıdaki planı gözden geçirin, ardından onaylayın:\n",
	MsgPlanReviewLegend:        "[y]ukarı [a]şağı [d]üzenle [s]il [boşluk] atla [t]ümünü onayla [esc] iptal: ",
	MsgPlanReviewSkippedMarker: "[atlandı]",
	MsgPlanReviewBlockedMarker: "ENGELLENDİ(%s)",
}
