package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRiskClassKnownValues(t *testing.T) {
	cases := []struct {
		in   string
		want RiskClass
	}{
		{"read", RiskRead},
		{"write", RiskWrite},
		{"network", RiskNetwork},
		{"elevated", RiskElevated},
		{"destructive", RiskDestructive},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseRiskClass(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseRiskClassUnknownReturnsError(t *testing.T) {
	cases := []string{"", "READ", "Read", "unknown", "critical", "dangerous", "elevate"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := ParseRiskClass(in)
			assert.Error(t, err)
		})
	}
}

func TestRiskClassStringRoundTripsThroughParseRiskClass(t *testing.T) {
	for _, r := range []RiskClass{RiskRead, RiskWrite, RiskNetwork, RiskElevated, RiskDestructive} {
		t.Run(r.String(), func(t *testing.T) {
			parsed, err := ParseRiskClass(r.String())
			require.NoError(t, err)
			assert.Equal(t, r, parsed)
		})
	}
}

func TestRiskClassSeverityOrderingIsAscending(t *testing.T) {
	assert.Less(t, int(RiskRead), int(RiskWrite))
	assert.Less(t, int(RiskWrite), int(RiskNetwork))
	assert.Less(t, int(RiskNetwork), int(RiskElevated))
	assert.Less(t, int(RiskElevated), int(RiskDestructive))
}

func TestRiskClassStringOutOfRangeDoesNotPanic(t *testing.T) {
	assert.Equal(t, "unknown(99)", RiskClass(99).String())
}
