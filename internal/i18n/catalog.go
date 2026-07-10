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
	// the 401/403 case, which does NOT use this message) — the key is
	// still stored, since this class of failure says nothing about
	// whether the key itself is actually good (QA MAJOR-2). Two args:
	// provider name, the ping error.
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

	MsgConfigUnknownKey:     "unknown config key %q; valid keys are: %s",
	MsgConfigInvalidEnum:    "invalid value %q for %s; must be one of: %s",
	MsgConfigInvalidBool:    "invalid value %q for %s: must be a boolean (true/false)",
	MsgConfigInvalidInt:     "invalid value %q for %s: must be an integer",
	MsgConfigNotPositive:    "invalid value %q for %s: must be greater than 0",
	MsgConfigNotNonNegative: "invalid value %q for %s: must be 0 or greater",

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
	MsgAuthStoredKeyPingFailed:    "Stored key for %s. Could not verify it right now (%v) — this looks like a network or connectivity issue, not necessarily a bad key. The key was saved.\n",
	MsgAuthKeyRejected:            "The provider rejected this key for %s (%v) — it was not saved. Double-check the key and try \"comrade auth login %s\" again.\n",
	MsgAuthStoredKeyPingSucceeded: "Stored key for %s. Test request succeeded (model=%s, latency=%s).\n",
	MsgAuthNoStoredKey:            "No stored key for %s.\n",
	MsgAuthRemovedStoredKey:       "Removed stored key for %s.\n",
	MsgAuthStatusHeader:           "PROVIDER\tSTATUS",
	MsgAuthStatusOllamaRow:        "ollama\t(no key required)",
	MsgAuthStatusSet:              "set (%s)",
	MsgAuthStatusSetEnv:           "set (env: %s)",
	MsgAuthStatusNotSet:           "not set",

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

	MsgUpgradeDevBuildError:  "upgrade: this is a dev build (no version was embedded at build time); install a released build to use `comrade upgrade`",
	MsgUpgradeUpToDate:       "you're already on the latest version (%s)\n",
	MsgUpgradeNewerAvailable: "a newer version is available: %s (you have %s) — %s\n",
	MsgUpgradeDownloading:    "downloading comrade %s...\n",
	MsgUpgradeInstalled:      "updated to %s. Restart any running comrade session to pick it up.\n",
	MsgUpgradeNoReleaseFound: "no published release of comrade is available yet — check back later",
	MsgUpgradeFetchFailed:    "could not reach GitHub to check for a newer version — try again later",

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

	MsgConfigUnknownKey:     "bilinmeyen config anahtarı %q; geçerli anahtarlar: %s",
	MsgConfigInvalidEnum:    "%q değeri %s için geçersiz; şunlardan biri olmalı: %s",
	MsgConfigInvalidBool:    "%q değeri %s için geçersiz: bir boolean olmalı (true/false)",
	MsgConfigInvalidInt:     "%q değeri %s için geçersiz: bir tam sayı olmalı",
	MsgConfigNotPositive:    "%q değeri %s için geçersiz: 0'dan büyük olmalı",
	MsgConfigNotNonNegative: "%q değeri %s için geçersiz: 0 veya daha büyük olmalı",

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
	MsgAuthStoredKeyPingFailed:    "%s için anahtar kaydedildi. Şu anda doğrulanamadı (%v) — bu, ağ veya bağlantı sorunundan kaynaklanıyor olabilir, anahtarın hatalı olduğu anlamına gelmez. Anahtar kaydedildi.\n",
	MsgAuthKeyRejected:            "%s için bu anahtar sağlayıcı tarafından reddedildi (%v) — kaydedilmedi. Anahtarı kontrol edip \"comrade auth login %s\" komutunu tekrar deneyin.\n",
	MsgAuthStoredKeyPingSucceeded: "%s için anahtar kaydedildi. Test isteği başarılı oldu (model=%s, gecikme=%s).\n",
	MsgAuthNoStoredKey:            "%s için kayıtlı anahtar yok.\n",
	MsgAuthRemovedStoredKey:       "%s için kayıtlı anahtar kaldırıldı.\n",
	MsgAuthStatusHeader:           "SAĞLAYICI\tDURUM",
	MsgAuthStatusOllamaRow:        "ollama\t(anahtar gerekmez)",
	MsgAuthStatusSet:              "kayıtlı (%s)",
	MsgAuthStatusSetEnv:           "kayıtlı (ortam değişkeni: %s)",
	MsgAuthStatusNotSet:           "kayıtlı değil",

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

	MsgUpgradeDevBuildError:  "upgrade: bu bir geliştirme (dev) derlemesi (derleme zamanında bir sürüm gömülmemiş); `comrade upgrade` kullanmak için yayımlanmış bir derleme kurun",
	MsgUpgradeUpToDate:       "zaten en güncel sürümdesiniz (%s)\n",
	MsgUpgradeNewerAvailable: "daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s) — %s\n",
	MsgUpgradeDownloading:    "comrade %s indiriliyor...\n",
	MsgUpgradeInstalled:      "%s sürümüne güncellendi. Çalışan bir comrade oturumu varsa bunu yansıtması için yeniden başlatın.\n",
	MsgUpgradeNoReleaseFound: "henüz yayımlanmış bir comrade sürümü yok — daha sonra tekrar kontrol edin",
	MsgUpgradeFetchFailed:    "daha yeni bir sürüm kontrol edilirken GitHub'a ulaşılamadı — daha sonra tekrar deneyin",

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
}
