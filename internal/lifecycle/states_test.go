package lifecycle_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/lifecycle"
)

func TestDaemonStateLabel(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{lifecycle.StateDaemonQueued, lifecycle.LabelQueued},
		{lifecycle.StateDaemonRunning, lifecycle.LabelRunning},
		{lifecycle.StateDaemonAwaitingApproval, lifecycle.LabelAwaitingApproval},
		{lifecycle.StateDaemonCompleted, lifecycle.LabelCompleted},
		{lifecycle.StateDaemonFailed, lifecycle.LabelFailed},
		{lifecycle.StateDaemonCanceled, lifecycle.LabelCanceled},
		{lifecycle.StateDaemonAbandoned, lifecycle.LabelAbandoned},
		{lifecycle.StateDaemonPaused, lifecycle.LabelPaused},
		{lifecycle.StateDaemonDegraded, lifecycle.LabelDegraded},
		{"", lifecycle.LabelUnknown},
		{"some_custom_state", "some_custom_state"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			assert.Equal(t, tc.want, lifecycle.DaemonStateLabel(tc.status))
		})
	}
}

func TestFileTaskLabel(t *testing.T) {
	cases := []struct {
		status constants.TaskStatus
		want   string
	}{
		// pending maps to "queued" — unified vocabulary
		{constants.TaskStatusPending, lifecycle.LabelQueued},
		{constants.TaskStatusRunning, lifecycle.LabelRunning},
		{constants.TaskStatusValidating, lifecycle.LabelRunning},
		{constants.TaskStatusAwaitingApproval, lifecycle.LabelAwaitingApproval},
		{constants.TaskStatusCompleted, lifecycle.LabelCompleted},
		{constants.TaskStatusValidationFailed, lifecycle.LabelFailed},
		{constants.TaskStatusGHFailed, lifecycle.LabelFailed},
		{constants.TaskStatusCIFailed, lifecycle.LabelFailed},
		{constants.TaskStatusCITimeout, lifecycle.LabelFailed},
		{constants.TaskStatusRejected, lifecycle.LabelFailed},
		{constants.TaskStatusInterrupted, lifecycle.LabelInterrupted},
		{constants.TaskStatusAbandoned, lifecycle.LabelAbandoned},
		{"", lifecycle.LabelUnknown},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.want, lifecycle.FileTaskLabel(tc.status))
		})
	}
}

// TestVocabularyConsistency verifies that the canonical labels used for daemon states
// and filesystem states are consistent for equivalent states.
func TestVocabularyConsistency(t *testing.T) {
	// "queued" daemon state and "pending" file state must render the same label
	assert.Equal(t,
		lifecycle.DaemonStateLabel(lifecycle.StateDaemonQueued),
		lifecycle.FileTaskLabel(constants.TaskStatusPending),
		"queued/pending must render the same canonical label",
	)

	// "running" must be consistent across daemon and file
	assert.Equal(t,
		lifecycle.DaemonStateLabel(lifecycle.StateDaemonRunning),
		lifecycle.FileTaskLabel(constants.TaskStatusRunning),
		"running must render the same canonical label",
	)

	// "awaiting_approval" must be consistent
	assert.Equal(t,
		lifecycle.DaemonStateLabel(lifecycle.StateDaemonAwaitingApproval),
		lifecycle.FileTaskLabel(constants.TaskStatusAwaitingApproval),
		"awaiting_approval must render the same canonical label",
	)

	// "completed" must be consistent
	assert.Equal(t,
		lifecycle.DaemonStateLabel(lifecycle.StateDaemonCompleted),
		lifecycle.FileTaskLabel(constants.TaskStatusCompleted),
		"completed must render the same canonical label",
	)

	// "abandoned" must be consistent
	assert.Equal(t,
		lifecycle.DaemonStateLabel(lifecycle.StateDaemonAbandoned),
		lifecycle.FileTaskLabel(constants.TaskStatusAbandoned),
		"abandoned must render the same canonical label",
	)
}

func TestDaemonStateIcon(t *testing.T) {
	// Icons should be non-empty for all known states
	states := []string{
		lifecycle.StateDaemonQueued,
		lifecycle.StateDaemonRunning,
		lifecycle.StateDaemonAwaitingApproval,
		lifecycle.StateDaemonCompleted,
		lifecycle.StateDaemonFailed,
		lifecycle.StateDaemonCanceled,
		lifecycle.StateDaemonAbandoned,
		lifecycle.StateDaemonPaused,
		lifecycle.StateDaemonDegraded,
	}
	for _, s := range states {
		t.Run(s, func(t *testing.T) {
			icon := lifecycle.DaemonStateIcon(s)
			assert.NotEmpty(t, icon)
		})
	}
}

func TestIsTerminalDaemonState(t *testing.T) {
	terminal := []string{
		lifecycle.StateDaemonCompleted,
		lifecycle.StateDaemonFailed,
		lifecycle.StateDaemonCanceled,
		lifecycle.StateDaemonAbandoned,
	}
	for _, s := range terminal {
		t.Run(s, func(t *testing.T) {
			assert.True(t, lifecycle.IsTerminalDaemonState(s))
		})
	}

	nonTerminal := []string{
		lifecycle.StateDaemonQueued,
		lifecycle.StateDaemonRunning,
		lifecycle.StateDaemonAwaitingApproval,
		lifecycle.StateDaemonPaused,
	}
	for _, s := range nonTerminal {
		t.Run("non_terminal_"+s, func(t *testing.T) {
			assert.False(t, lifecycle.IsTerminalDaemonState(s))
		})
	}
}

func TestIsActiveDaemonState(t *testing.T) {
	active := []string{
		lifecycle.StateDaemonQueued,
		lifecycle.StateDaemonRunning,
		lifecycle.StateDaemonAwaitingApproval,
		lifecycle.StateDaemonPaused,
	}
	for _, s := range active {
		t.Run(s, func(t *testing.T) {
			assert.True(t, lifecycle.IsActiveDaemonState(s))
		})
	}

	inactive := []string{
		lifecycle.StateDaemonCompleted,
		lifecycle.StateDaemonFailed,
		lifecycle.StateDaemonCanceled,
		lifecycle.StateDaemonAbandoned,
	}
	for _, s := range inactive {
		t.Run("inactive_"+s, func(t *testing.T) {
			assert.False(t, lifecycle.IsActiveDaemonState(s))
		})
	}
}
