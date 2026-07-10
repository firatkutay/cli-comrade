package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// newHistoryCmd builds "comrade history": a read-only view over the
// audit.jsonl log internal/engine.Runner appends to (via
// internal/cli/do.go's buildAuditSink). It never writes to the audit
// file itself — no retention cleanup runs here (that is `comrade do`'s
// concern, lazily, once per invocation that actually executes something;
// see runDo/buildAuditSink) — so simply viewing history can never mutate
// it.
func newHistoryCmd(newLoader loaderFactory) *cobra.Command {
	var (
		asJSON bool
		limit  int
	)

	cmd := &cobra.Command{
		Use:               "history",
		Short:             "Show recently executed commands from the audit log",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			path, err := audit.DefaultPath()
			if err != nil {
				return err
			}
			logger, err := audit.NewLogger(path)
			if err != nil {
				return err
			}
			entries, err := logger.ReadAll()
			if err != nil {
				return err
			}
			entries = lastN(entries, limit)

			if asJSON {
				return printHistoryJSON(cmd.OutOrStdout(), entries)
			}
			return printHistoryTable(cmd.OutOrStdout(), entries, tr)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, enUsageDefault(i18n.MsgFlagJSON))
	cmd.Flags().IntVar(&limit, "limit", 20, enUsageDefault(i18n.MsgFlagLimit))
	return cmd
}

// lastN returns the last (most recent) n entries from entries, which
// internal/audit.Logger.ReadAll always returns oldest-first. n <= 0 or
// n >= len(entries) returns entries unchanged (no limiting).
func lastN(entries []audit.Entry, n int) []audit.Entry {
	if n <= 0 || n >= len(entries) {
		return entries
	}
	return entries[len(entries)-n:]
}

// printHistoryJSON prints entries as one compact JSON object per line
// (JSONL) — docs/history/UYGULAMA_PLANI.md FAZ 6 item 4's "--json flag'i ham çıktı
// verir". Raw data, not prose, so it is never translated.
func printHistoryJSON(w io.Writer, entries []audit.Entry) error {
	enc := json.NewEncoder(w)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

// printHistoryTable prints entries as a tabwriter-aligned
// TIME/MODE/RISK/EXIT/COMMAND table, timestamps rendered in the local
// zone as RFC3339 — or, when entries is empty, MsgHistoryEmpty instead of
// a bare header row with nothing under it.
func printHistoryTable(w io.Writer, entries []audit.Entry, tr i18n.Translator) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, tr.T(i18n.MsgHistoryEmpty))
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgHistoryTableHeader)); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			e.Timestamp.Local().Format(time.RFC3339), e.Mode, e.Risk, e.ExitCode, e.Command); err != nil {
			return err
		}
	}
	return tw.Flush()
}
