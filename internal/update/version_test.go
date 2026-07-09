package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsDevBuild(t *testing.T) {
	assert.True(t, IsDevBuild("dev"))
	assert.True(t, IsDevBuild(""))
	assert.False(t, IsDevBuild("v0.1.0"))
	assert.False(t, IsDevBuild("0.1.0"))
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"patch bump", "v0.1.0", "v0.1.1", true},
		{"minor bump", "v0.1.0", "v0.2.0", true},
		{"major bump", "v0.1.0", "v1.0.0", true},
		{"same version", "v0.1.0", "v0.1.0", false},
		{"latest older", "v0.2.0", "v0.1.0", false},
		{"no v prefix either side", "0.1.0", "0.2.0", true},
		{"missing trailing component treated as zero", "v1.2", "v1.2.0", false},
		{"snapshot suffix ignored for ordering", "v0.1.0", "v0.1.0-SNAPSHOT-abc123", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsNewer(tc.current, tc.latest)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsNewerRejectsNonNumericComponent(t *testing.T) {
	_, err := IsNewer("v0.1.0", "vX.Y.Z")
	require.Error(t, err)
	assert.ErrorContains(t, err, "non-numeric version component")
}
