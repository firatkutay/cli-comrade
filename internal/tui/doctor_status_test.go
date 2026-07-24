package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/doctor"
)

func TestDoctorSeverityLabelColorDisabledUsesWordFallback(t *testing.T) {
	assert.Equal(t, "[OK]", DoctorSeverityLabel(doctor.SeverityOK, false))
	assert.Equal(t, "[WARN]", DoctorSeverityLabel(doctor.SeverityWarn, false))
	assert.Equal(t, "[FAIL]", DoctorSeverityLabel(doctor.SeverityFail, false))
	assert.Equal(t, "[SKIP]", DoctorSeverityLabel(doctor.SeveritySkip, false))
}

func TestDoctorSeverityLabelColorEnabledRendersGlyph(t *testing.T) {
	assert.Contains(t, DoctorSeverityLabel(doctor.SeverityOK, true), "✓")
	assert.Contains(t, DoctorSeverityLabel(doctor.SeverityWarn, true), "⚠")
	assert.Contains(t, DoctorSeverityLabel(doctor.SeverityFail, true), "✗")
	assert.Contains(t, DoctorSeverityLabel(doctor.SeveritySkip, true), "-")
}

func TestPrintDoctorLineColorDisabledIsPlainAndByteClean(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, PrintDoctorLine(&buf, doctor.SeverityWarn, "shell integration is not installed for bash", false))

	assert.Equal(t, "[WARN] shell integration is not installed for bash\n", buf.String())
}

func TestPrintDoctorLineColorEnabledIncludesText(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, PrintDoctorLine(&buf, doctor.SeverityOK, "up to date (v1.0.0)", true))

	assert.Contains(t, buf.String(), "up to date (v1.0.0)")
	assert.Contains(t, buf.String(), "✓")
}
