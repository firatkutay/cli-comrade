package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAdminUnixRootIsAdminAndKnown(t *testing.T) {
	isAdmin, known := IsAdmin("linux", func() int { return 0 })
	assert.True(t, isAdmin)
	assert.True(t, known)
}

func TestIsAdminUnixNonRootIsNotAdminButKnown(t *testing.T) {
	isAdmin, known := IsAdmin("linux", func() int { return 1000 })
	assert.False(t, isAdmin)
	assert.True(t, known)
}

func TestIsAdminDarwinFollowsSameEuidLogic(t *testing.T) {
	isAdmin, known := IsAdmin("darwin", func() int { return 0 })
	assert.True(t, isAdmin)
	assert.True(t, known)
}

func TestIsAdminWindowsAlwaysUnknown(t *testing.T) {
	isAdmin, known := IsAdmin("windows", func() int { return 0 })
	assert.False(t, isAdmin, "windows must never claim admin=true without a real check")
	assert.False(t, known, "windows honestly reports it did not check, rather than guessing")
}

func TestIsAdminNilGeteuidIsUnknown(t *testing.T) {
	isAdmin, known := IsAdmin("linux", nil)
	assert.False(t, isAdmin)
	assert.False(t, known)
}
