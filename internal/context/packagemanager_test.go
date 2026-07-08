package context

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func stubLookPath(present map[string]bool) func(string) (string, error) {
	return func(name string) (string, error) {
		if present[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestDetectPackageManagersReturnsFoundOnesInStableOrder(t *testing.T) {
	lookPath := stubLookPath(map[string]bool{
		"brew":   true,
		"apt":    true,
		"choco":  true,
		"pacman": false,
	})

	got := DetectPackageManagers(lookPath)

	// Stable order = packageManagerNames' order, not insertion order.
	assert.Equal(t, []string{"apt", "brew", "choco"}, got)
}

func TestDetectPackageManagersNoneFound(t *testing.T) {
	got := DetectPackageManagers(stubLookPath(map[string]bool{}))
	assert.Empty(t, got)
}

func TestDetectPackageManagersNilLookPath(t *testing.T) {
	got := DetectPackageManagers(nil)
	assert.Nil(t, got)
}
