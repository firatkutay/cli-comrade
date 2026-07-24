package doctor

import (
	"context"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// BaseURLCheck applies only to the openai_compat provider (Skip
// otherwise): it sniffs the LOCALLY-resolved key's prefix — never a
// network call, never printing any fragment of the key itself — and
// warns when llm.openai_compat.base_url is still the shipped OpenAI
// default AND the key's prefix looks like a different vendor's key
// format (a user who logged in with, say, a Groq key without ever
// customizing base_url — see internal/cli/auth.go's
// promptOpenAICompatBaseURLIfDefault, which this check is the read-only,
// after-the-fact diagnostic counterpart of).
func BaseURLCheck(ctx context.Context, deps Deps) Result {
	if deps.ConfigErr != nil || deps.Cfg.LLM.Provider != "openai_compat" {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorBaseURLSkip}
	}
	if deps.Cfg.LLM.OpenAICompat.BaseURL != config.Default().LLM.OpenAICompat.BaseURL {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorBaseURLOK}
	}

	key := ""
	if deps.Store != nil {
		if k, source, err := deps.Store.Get(ctx, "openai_compat"); err == nil && source != secrets.SourceNone {
			key = k
		}
	}
	if key == "" {
		if k, err := llm.ResolveEnvKey("openai_compat"); err == nil {
			key = k
		}
	}

	vendor, ok := sniffVendorFromKeyPrefix(key)
	if !ok {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorBaseURLOK}
	}
	return Result{
		Severity:    SeverityWarn,
		Summary:     i18n.MsgDoctorBaseURLSuspectedVendor,
		SummaryArgs: []any{vendor},
		Fix:         "comrade config set llm.openai_compat.base_url <url>",
	}
}

// vendorKeyPrefixes maps a known non-OpenAI vendor's API key prefix to
// its display name, for BaseURLCheck's key-prefix sniff. Order matters
// only in that longer/more-specific prefixes must be checked before any
// shorter prefix that could also match — none of these four currently
// overlap, so a simple ordered slice (rather than a map, whose iteration
// order is random) keeps the match deterministic regardless.
var vendorKeyPrefixes = []struct {
	prefix string
	vendor string
}{
	{"sk-ant-", "Anthropic"},
	{"gsk_", "Groq"},
	{"sk-or-", "OpenRouter"},
	{"AIza", "Google"},
}

// sniffVendorFromKeyPrefix reports the vendor name a KNOWN non-OpenAI key
// prefix belongs to, if key's prefix matches one — never true for an
// empty key (nothing to sniff) or one with no recognized prefix
// (including a genuine OpenAI "sk-" key, which is exactly the
// no-warning case).
func sniffVendorFromKeyPrefix(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	for _, vp := range vendorKeyPrefixes {
		if strings.HasPrefix(key, vp.prefix) {
			return vp.vendor, true
		}
	}
	return "", false
}
