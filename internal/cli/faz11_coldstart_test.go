package cli

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// coldStartCeiling is a deliberately generous regression ceiling for
// TestFAZ11ColdStartStaysWellUnderOneSecond — NOT the actual FAZ 11 target
// (see docs/phases/FAZ-11.md's measured numbers: ~4-5ms on a native
// filesystem after this run's fix). A hard assertion at the real <100ms
// target would flake on a loaded/shared CI runner or a DrvFs-backed
// checkout; this ceiling exists only to catch a GROSS regression of
// exactly the class this run found and fixed — github.com/atotto/
// clipboard v0.1.4's Unix build ran up to five sequential exec.LookPath
// PATH scans unconditionally at package init (see this repo's go.mod
// replace directive and third_party/atotto-clipboard), which cost this
// sandbox's own WSL2 shell (a ~124-entry PATH, most 9p/DrvFs-mounted)
// around 600ms on EVERY invocation, including --version. 500ms leaves
// that regression caught while giving normal CI variance (process
// spawn, antivirus-scanned first launch on a fresh Windows runner, a
// noisy shared VM) comfortable headroom.
const coldStartCeiling = 500 * time.Millisecond

// TestFAZ11ColdStartStaysWellUnderOneSecond builds the real comrade
// binary once and times `--version`, `--help`, and `config path` —
// UYGULAMA_PLANI.md FAZ 11 item 3's three named no-LLM-call commands —
// against it, failing if any of them regresses past coldStartCeiling.
// See docs/phases/FAZ-11.md for the actual measured numbers (both before
// and after this run's clipboard-init fix) against a native filesystem,
// which is where the real <100ms target is evaluated — this test's
// ceiling is a coarse CI-safe backstop, not a restatement of that target.
func TestFAZ11ColdStartStaysWellUnderOneSecond(t *testing.T) {
	bin := buildComradeBinary(t)

	cases := []struct {
		name string
		args []string
	}{
		{"version", []string{"--version"}},
		{"help", []string{"--help"}},
		{"config-path", []string{"config", "path"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// One warm-up run so a one-time page-cache-cold read of the
			// binary itself never gets blamed on the code path under
			// test; every measured run after this is what actually
			// gates the assertion.
			warmup := exec.Command(bin, tc.args...)
			_ = warmup.Run()

			start := time.Now()
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(start)

			require.NoError(t, err, "output: %s", out)
			t.Logf("%s: %s", tc.name, elapsed)
			require.Less(t, elapsed, coldStartCeiling, "%s took %s — see coldStartCeiling's doc comment for why this is a coarse regression backstop, not the real <100ms target", tc.name, elapsed)
		})
	}
}
