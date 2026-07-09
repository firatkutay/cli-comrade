package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// errDryRunRequired is returned verbatim (main.go prints it to stderr and
// exits 1) when `comrade do` is invoked without --dry-run. FAZ 6 replaces
// this with the real three-mode execution loop; until then, this is the
// command's entire non-dry-run behavior, per UYGULAMA_PLANI.md FAZ 5 item
// 4.
var errDryRunRequired = errors.New("execution arrives in a later phase; use --dry-run")

// newDoCmd builds the hidden "comrade do <request...>" command: FAZ 5's
// end-to-end proof that a free-text request turns into a real,
// safety-annotated Plan. It is hidden (not yet the product's real `do`
// entry point — FAZ 6 wires the root-command fallback and the actual
// three-mode execution loop) and requires --dry-run: this phase performs
// no execution whatsoever.
func newDoCmd(newLoader loaderFactory) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:    "do <request...>",
		Short:  "Generate a risk-labeled plan for a free-text request",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				return errDryRunRequired
			}
			return runDoDryRun(cmd, newLoader, strings.Join(args, " "))
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the generated plan without executing it (required in this phase)")
	return cmd
}

// runDoDryRun implements `comrade do --dry-run`: load the effective
// config, collect the real system context, generate a plan through the
// real llm.Client + internal/engine.Planner (which itself always runs
// every step through internal/safety.Engine — see Planner.GeneratePlan),
// and render it.
func runDoDryRun(cmd *cobra.Command, newLoader loaderFactory, request string) error {
	loader, err := newLoader()
	if err != nil {
		return err
	}
	cfg, created, err := loader.Load()
	if err != nil {
		return err
	}
	if created {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), firstRunNoticeFormat, loader.Path()); err != nil {
			return err
		}
	}

	client, err := llm.New(*cfg)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	collector := contextpkg.NewCollector()
	sysCtx := collector.Collect(cmd.Context(), contextpkg.Options{
		SendHistory:  cfg.Context.SendHistory,
		HistoryDepth: cfg.Context.HistoryDepth,
		SendEnvNames: cfg.Context.SendEnvNames,
	})

	planner := engine.NewPlanner(client, *cfg)
	plan, err := planner.GeneratePlan(cmd.Context(), request, sysCtx)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	return renderPlan(cmd.OutOrStdout(), plan)
}

// renderPlan prints plan.Summary followed by a tabwriter-aligned
// STEP/COMMAND/RISK/REVERSIBLE/RATIONALE table, per UYGULAMA_PLANI.md FAZ
// 5 item 4. The RISK column always renders internal/safety's independent
// EffectiveRisk, never the LLM's raw step.Risk label — that is the whole
// point of this table: to surface the second check, not to redisplay
// what the model claimed. A Blocked step renders "BLOCKED(<reason>)"; a
// step the safety engine escalated to Confirm renders
// "CONFIRM(<effective risk>)" so a risk bump is visible even when it
// isn't severe enough to Block; a plain Allow renders just the risk name.
func renderPlan(w io.Writer, plan engine.Plan) error {
	if _, err := fmt.Fprintln(w, plan.Summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "STEP\tCOMMAND\tRISK\tREVERSIBLE\tRATIONALE"); err != nil {
		return err
	}
	for i, step := range plan.Steps {
		risk := step.Decision.EffectiveRisk.String()
		switch step.Decision.Action {
		case safety.Block:
			risk = fmt.Sprintf("BLOCKED(%s)", step.Decision.Reason)
		case safety.Confirm:
			risk = fmt.Sprintf("CONFIRM(%s)", step.Decision.EffectiveRisk.String())
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%t\t%s\n", i+1, step.Command, risk, step.Reversible, step.Rationale); err != nil {
			return err
		}
	}
	return tw.Flush()
}
