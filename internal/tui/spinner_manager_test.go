package tui_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/tui"
)

// TestSpinnerManager_SingleActiveAcrossReplacements locks in the stop-then-create
// contract the flicker fix relies on: after stopping the current spinner and
// starting a replacement, the manager's active spinner is the replacement (never
// nil). This is what keeps the logger's spinnerAwareWriter coordinating with the
// live spinner across step/retry transitions.
func TestSpinnerManager_SingleActiveAcrossReplacements(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	mgr := tui.GlobalSpinnerManager()

	a := tui.NewTerminalSpinner(&buf)
	a.Start(ctx, "A")
	assert.Same(t, a, mgr.GetActive(), "first spinner is active after Start")

	// Stop A before starting B (the stop-then-create ordering the fix relies on
	// so Stop()'s ClearActive() never clears the replacement's active pointer).
	a.Stop()
	assert.Nil(t, mgr.GetActive(), "no active spinner after Stop")

	b := tui.NewTerminalSpinner(&buf)
	b.Start(ctx, "B")
	assert.Same(t, b, mgr.GetActive(), "replacement spinner becomes the active one")

	b.Stop()
	assert.Nil(t, mgr.GetActive(), "no active spinner after the replacement stops")
}
