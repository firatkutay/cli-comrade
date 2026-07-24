package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeExitCoder is a minimal exitCoder double, standing in for
// internal/cli/doctor.go's doctorFailedError without importing
// internal/cli (main.go itself deliberately never imports doctorFailedError
// directly — see exitCoder's own doc comment on why this is a small,
// unexported structural interface instead).
type fakeExitCoder struct {
	code int
}

func (e *fakeExitCoder) Error() string { return "fake exit coder error" }
func (e *fakeExitCoder) ExitCode() int { return e.code }

// TestExitCodeForHonorsExitCoderInterface uses a deliberately
// NON-default code (42, not 1) so this test can only pass if exitCodeFor
// actually took the exitCoder branch — not merely fallen through to its
// own hardcoded default, which every real exitCoder in this codebase
// today (doctorFailedError) happens to also return.
func TestExitCodeForHonorsExitCoderInterface(t *testing.T) {
	err := &fakeExitCoder{code: 42}
	assert.Equal(t, 42, exitCodeFor(err))
}

// TestExitCodeForSeesThroughWrappedErrors proves errors.As's own
// unwrap-chain walk is what exitCodeFor relies on — an exitCoder wrapped
// by fmt.Errorf("...: %w", err) (exactly how most errors reach
// Execute()'s return value in this codebase) still resolves to its own
// ExitCode(), not the default 1.
func TestExitCodeForSeesThroughWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("comrade: %w", &fakeExitCoder{code: 42})
	assert.Equal(t, 42, exitCodeFor(wrapped))
}

func TestExitCodeForDefaultsToOneForAPlainError(t *testing.T) {
	assert.Equal(t, 1, exitCodeFor(errors.New("some ordinary command error")))
}
