package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// newConfigModelsCmd implements `comrade config models`
// (UYGULAMA_PLANI.md FAZ 8 item 4): fetch the model list for the
// currently active provider (llm.provider), print it as a numbered menu,
// read a selection from stdin, and persist the choice to llm.model via
// loader.SetAndSave — the same persistence path `comrade config set`
// uses.
func newConfigModelsCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List models available for the active provider and select one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			models, docsURL, err := fetchModelsForProvider(cmd.Context(), cmd.ErrOrStderr(), *cfg)
			if err != nil {
				return fmt.Errorf("config models: %w", err)
			}
			if len(models) == 0 {
				return fmt.Errorf("config models: provider %q returned no models", cfg.LLM.Provider)
			}

			for i, m := range models {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d) %s\n", i+1, m); err != nil {
					return err
				}
			}
			if docsURL != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "(static snapshot — see %s for the current list)\n", docsURL); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), "Select a model number: "); err != nil {
				return err
			}

			choice, err := readModelChoice(cmd.InOrStdin(), len(models))
			if err != nil {
				return fmt.Errorf("config models: %w", err)
			}
			selected := models[choice-1]

			if err := loader.SetAndSave("llm.model", selected); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "llm.model = %s\n", selected)
			return err
		},
	}
}

// fetchModelsForProvider returns the model list (and, for the two
// providers with only a static snapshot, a docs URL to print alongside
// it — see llm.AnthropicModelsDocsURL/llm.GoogleModelsDocsURL) for
// cfg.LLM.Provider. openai_compat resolves its API key through the exact
// same secretsKeyResolver chain (keychain/file, then env) every other
// FAZ 8 command uses, so listing models never requires a *second*,
// differently-sourced credential.
func fetchModelsForProvider(ctx context.Context, stderr io.Writer, cfg config.Config) (models []string, docsURL string, err error) {
	switch cfg.LLM.Provider {
	case "anthropic":
		return llm.KnownAnthropicModels(), llm.AnthropicModelsDocsURL, nil

	case "google":
		return llm.KnownGoogleModels(), llm.GoogleModelsDocsURL, nil

	case "openai_compat":
		store, err := newSecretsStore(stderr)
		if err != nil {
			return nil, "", err
		}
		key, err := secretsKeyResolver(store)("openai_compat")
		if err != nil {
			return nil, "", err
		}
		names, err := llm.ListOpenAICompatModels(ctx, cfg.LLM.OpenAICompat.BaseURL, key, nil)
		return names, "", err

	case "ollama":
		names, err := llm.ListOllamaModels(ctx, cfg.LLM.Ollama.BaseURL, nil)
		return names, "", err

	default:
		return nil, "", fmt.Errorf("unknown provider %q", cfg.LLM.Provider)
	}
}

// errInvalidSelection is wrapped by readModelChoice's own error messages
// so callers/tests can errors.Is against one stable sentinel regardless
// of which way the input was invalid (not a number vs. out of range).
var errInvalidSelection = errors.New("invalid selection")

// readModelChoice reads a single line from in, parses it as a 1-based
// index into a count-item list, and validates it is in range. It reads
// exactly one line and errors on anything invalid rather than
// re-prompting in a loop — simpler to reason about and to test
// (UYGULAMA_PLANI.md FAZ 8 item 4 leaves this choice open; a plain,
// single-shot numbered prompt is this project's pick over a bubbletea
// list, per the item's own "easier to test" note).
func readModelChoice(in io.Reader, count int) (int, error) {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("read selection: %w", err)
	}
	line = strings.TrimSpace(line)

	n, convErr := strconv.Atoi(line)
	if convErr != nil {
		return 0, fmt.Errorf("%w: %q is not a number (expected 1-%d)", errInvalidSelection, line, count)
	}
	if n < 1 || n > count {
		return 0, fmt.Errorf("%w: %d is out of range (expected 1-%d)", errInvalidSelection, n, count)
	}
	return n, nil
}
