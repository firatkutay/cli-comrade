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
	// it — see docs/phases/FAZ-09.md).
	MsgExplainRiskHeading MessageID = "explain_risk_heading"

	// -- chat (comrade chat) ------------------------------------------

	// MsgChatWelcome is the one-line banner chat prints when the session
	// starts.
	MsgChatWelcome MessageID = "chat_welcome"

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
	// docs/phases/FAZ-09.md). One arg: the file path.
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
	// primary output (UYGULAMA_PLANI.md FAZ 5 item 4).
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

	// -- comrade auth ----------------------------------------------------

	// MsgAuthEnterKeyPrompt is `comrade auth login`'s no-echo key prompt.
	// One arg: the provider name.
	MsgAuthEnterKeyPrompt MessageID = "auth_enter_key_prompt"

	// MsgAuthStoredKeyPingFailed reports a stored key whose live test
	// request failed (the key is still stored — see auth.go's own doc
	// comment on why). Two args: provider name, the ping error.
	MsgAuthStoredKeyPingFailed MessageID = "auth_stored_key_ping_failed"

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
	// deliberately NOT migrated here — see docs/phases/FAZ-09.md's exact
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

	// -- per-flag --help descriptions (comrade upgrade) -------------------

	// MsgFlagCheck is --check's --help description.
	MsgFlagCheck MessageID = "flag_check"

	// -- passive version-update notification (FAZ 10 item 4) -------------

	// MsgUpdateAvailableNotice is the single line printed at the end of a
	// command, at most once per week, when a background check found a
	// newer release than the running build. Two args: the latest
	// version, the current version.
	MsgUpdateAvailableNotice MessageID = "update_available_notice"
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

	MsgTestLLMResult:    "provider=%s model=%s latency=%s\n",
	MsgConfigListHeader: "KEY\tVALUE\tSOURCE",

	MsgModelsDocsNote:     "(static snapshot — see %s for the current list)\n",
	MsgModelsSelectPrompt: "Select a model number: ",
	MsgModelsSetConfirm:   "llm.model = %s\n",

	MsgChatWelcome: "cli-comrade chat — type a message, or /help for commands.",
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

	MsgAuthEnterKeyPrompt:         "Enter API key for %s: ",
	MsgAuthStoredKeyPingFailed:    "Stored key for %s. Test request failed (%v) — the key may still be correct; this can also mean the network or provider is unreachable right now.\n",
	MsgAuthStoredKeyPingSucceeded: "Stored key for %s. Test request succeeded (model=%s, latency=%s).\n",
	MsgAuthNoStoredKey:            "No stored key for %s.\n",
	MsgAuthRemovedStoredKey:       "Removed stored key for %s.\n",
	MsgAuthStatusHeader:           "PROVIDER\tSTATUS",
	MsgAuthStatusOllamaRow:        "ollama\t(no key required)",
	MsgAuthStatusSet:              "set (%s)",
	MsgAuthStatusSetEnv:           "set (env: %s)",
	MsgAuthStatusNotSet:           "not set",

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
	MsgFixRerunNoLastCommandError:    "--rerun: no recorded last command found; run a command with shell integration installed first, or use `comrade fix -- <command>`",
	MsgFlagsModeExclusiveError:       "only one of --auto, --ask, or --info may be given",
	MsgInitPrintRemoveExclusiveError: "init: --print and --remove are mutually exclusive",
	MsgInitShellUndetectedError:      "init: could not detect your shell; run e.g. \"comrade init bash\" explicitly",
	MsgInitShellUnsupportedError:     "init: detected shell %q is not supported; run \"comrade init bash|zsh|fish|powershell\" explicitly",
	MsgModelsNoModelsError:           "config models: provider %q returned no models",
	MsgModelsUnknownProviderError:    "unknown provider %q",
	MsgModelsChoiceNotANumber:        "%q is not a number (expected 1-%d)",
	MsgModelsChoiceOutOfRange:        "%d is out of range (expected 1-%d)",

	MsgUpgradeDevBuildError:  "upgrade: this is a dev build (no version was embedded at build time); install a released build to use `comrade upgrade`",
	MsgUpgradeUpToDate:       "you're already on the latest version (%s)\n",
	MsgUpgradeNewerAvailable: "a newer version is available: %s (you have %s) — %s\n",
	MsgUpgradeDownloading:    "downloading comrade %s...\n",
	MsgUpgradeInstalled:      "updated to %s. Restart any running comrade session to pick it up.\n",

	MsgHelpShortUpgrade: "Check for or install a newer released version of comrade",
	MsgFlagCheck:        "only report whether a newer version is available; do not download or install it",

	MsgUpdateAvailableNotice: "\ncomrade: a new version is available: %s (you have %s). Run `comrade upgrade` to update.\n",
}

// catalogTR is the Turkish catalog. Every message here is a natural,
// idiomatic Turkish translation — not a literal machine translation — of
// its catalogEN counterpart; see docs/phases/FAZ-09.md's translation
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

	MsgTestLLMResult:    "sağlayıcı=%s model=%s gecikme=%s\n",
	MsgConfigListHeader: "ANAHTAR\tDEĞER\tKAYNAK",

	MsgModelsDocsNote:     "(sabit bir liste — güncel liste için: %s)\n",
	MsgModelsSelectPrompt: "Bir model numarası seçin: ",
	MsgModelsSetConfirm:   "llm.model = %s\n",

	MsgChatWelcome: "cli-comrade sohbet — bir mesaj yazın, ya da komutlar için /help yazın.",
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

	MsgAuthEnterKeyPrompt:         "%s için API anahtarını girin: ",
	MsgAuthStoredKeyPingFailed:    "%s için anahtar kaydedildi. Test isteği başarısız oldu (%v) — anahtar yine de doğru olabilir; bu, ağın veya sağlayıcının şu an erişilemez olduğu anlamına da gelebilir.\n",
	MsgAuthStoredKeyPingSucceeded: "%s için anahtar kaydedildi. Test isteği başarılı oldu (model=%s, gecikme=%s).\n",
	MsgAuthNoStoredKey:            "%s için kayıtlı anahtar yok.\n",
	MsgAuthRemovedStoredKey:       "%s için kayıtlı anahtar kaldırıldı.\n",
	MsgAuthStatusHeader:           "SAĞLAYICI\tDURUM",
	MsgAuthStatusOllamaRow:        "ollama\t(anahtar gerekmez)",
	MsgAuthStatusSet:              "kayıtlı (%s)",
	MsgAuthStatusSetEnv:           "kayıtlı (ortam değişkeni: %s)",
	MsgAuthStatusNotSet:           "kayıtlı değil",

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
	MsgFixRerunNoLastCommandError:    "--rerun: kayıtlı son komut bulunamadı; önce kabuk entegrasyonu kurulu bir komut çalıştırın ya da `comrade fix -- <komut>` kullanın",
	MsgFlagsModeExclusiveError:       "--auto, --ask veya --info bayraklarından yalnızca biri verilebilir",
	MsgInitPrintRemoveExclusiveError: "init: --print ve --remove birlikte kullanılamaz",
	MsgInitShellUndetectedError:      "init: kabuğunuz tespit edilemedi; örneğin \"comrade init bash\" komutunu açıkça çalıştırın",
	MsgInitShellUnsupportedError:     "init: tespit edilen kabuk %q desteklenmiyor; \"comrade init bash|zsh|fish|powershell\" komutunu açıkça çalıştırın",
	MsgModelsNoModelsError:           "config models: %q sağlayıcısı hiçbir model döndürmedi",
	MsgModelsUnknownProviderError:    "bilinmeyen sağlayıcı %q",
	MsgModelsChoiceNotANumber:        "%q bir sayı değil (beklenen: 1-%d)",
	MsgModelsChoiceOutOfRange:        "%d aralık dışında (beklenen: 1-%d)",

	MsgUpgradeDevBuildError:  "upgrade: bu bir geliştirme (dev) derlemesi (derleme zamanında bir sürüm gömülmemiş); `comrade upgrade` kullanmak için yayımlanmış bir derleme kurun",
	MsgUpgradeUpToDate:       "zaten en güncel sürümdesiniz (%s)\n",
	MsgUpgradeNewerAvailable: "daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s) — %s\n",
	MsgUpgradeDownloading:    "comrade %s indiriliyor...\n",
	MsgUpgradeInstalled:      "%s sürümüne güncellendi. Çalışan bir comrade oturumu varsa bunu yansıtması için yeniden başlatın.\n",

	MsgHelpShortUpgrade: "comrade'in daha yeni bir yayımlanmış sürümünü denetler veya kurar",
	MsgFlagCheck:        "yalnızca daha yeni bir sürüm olup olmadığını bildirir; indirmez veya kurmaz",

	MsgUpdateAvailableNotice: "\ncomrade: daha yeni bir sürüm mevcut: %s (mevcut sürümünüz: %s). Güncellemek için `comrade upgrade` çalıştırın.\n",
}
