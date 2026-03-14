package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmitBellTo(t *testing.T) {
	var buf bytes.Buffer

	EmitBellTo(&buf)

	assert.Equal(t, "\a", buf.String(), "Should write BEL character")
}

func TestShouldNotify(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		bellEnabled bool
		events      []string
		want        bool
	}{
		{
			name:        "bell disabled returns false",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: false,
			events:      []string{NotifyEventAwaitingApproval},
			want:        false,
		},
		{
			name:        "event in list returns true",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval},
			want:        true,
		},
		{
			name:        "event not in list returns false",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: true,
			events:      []string{NotifyEventValidationFailed},
			want:        false,
		},
		{
			name:        "empty events returns false",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: true,
			events:      []string{},
			want:        false,
		},
		{
			name:        "multiple events with match returns true",
			event:       NotifyEventCIFailed,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval, NotifyEventCIFailed, NotifyEventGitHubFailed},
			want:        true,
		},
		{
			name:        "all events enabled",
			event:       NotifyEventGitHubFailed,
			bellEnabled: true,
			events:      AllNotificationEvents(),
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &NotificationConfig{
				BellEnabled: tt.bellEnabled,
				Events:      tt.events,
			}
			got := ShouldNotify(tt.event, cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldNotify_NilConfig(t *testing.T) {
	got := ShouldNotify(NotifyEventAwaitingApproval, nil)
	assert.False(t, got, "Should return false for nil config")
}

func TestNotifyIfEnabledTo(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		bellEnabled bool
		events      []string
		wantBell    bool
	}{
		{
			name:        "should emit bell when enabled and event matches",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval},
			wantBell:    true,
		},
		{
			name:        "should not emit bell when disabled",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: false,
			events:      []string{NotifyEventAwaitingApproval},
			wantBell:    false,
		},
		{
			name:        "should not emit bell when event not in list",
			event:       NotifyEventCIFailed,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval},
			wantBell:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &NotificationConfig{
				BellEnabled: tt.bellEnabled,
				Events:      tt.events,
			}

			NotifyIfEnabledTo(&buf, tt.event, cfg)

			if tt.wantBell {
				assert.Equal(t, "\a", buf.String(), "Should write BEL character")
			} else {
				assert.Empty(t, buf.String(), "Should not write anything")
			}
		})
	}
}

func TestNotifyIfEnabledTo_NilConfig(t *testing.T) {
	var buf bytes.Buffer

	NotifyIfEnabledTo(&buf, NotifyEventAwaitingApproval, nil)

	assert.Empty(t, buf.String(), "Should not write anything for nil config")
}

func TestEmitBell(t *testing.T) {
	// EmitBell is a thin wrapper around EmitBellTo(os.Stdout).
	// We can't easily capture stdout in tests, but we can verify it's callable.
	// The core logic is tested in TestEmitBellTo.
	assert.NotPanics(t, func() {
		EmitBell()
	}, "EmitBell should not panic")
}

func TestNotifyIfEnabled(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		bellEnabled bool
		events      []string
	}{
		{
			name:        "should call EmitBell when enabled and event matches",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval},
		},
		{
			name:        "should not call EmitBell when disabled",
			event:       NotifyEventAwaitingApproval,
			bellEnabled: false,
			events:      []string{NotifyEventAwaitingApproval},
		},
		{
			name:        "should not call EmitBell when event not in list",
			event:       NotifyEventCIFailed,
			bellEnabled: true,
			events:      []string{NotifyEventAwaitingApproval},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &NotificationConfig{
				BellEnabled: tt.bellEnabled,
				Events:      tt.events,
			}

			// NotifyIfEnabled calls ShouldNotify and EmitBell internally.
			// We verify it's callable without panic.
			// The logic is tested through ShouldNotify and EmitBellTo tests.
			assert.NotPanics(t, func() {
				NotifyIfEnabled(tt.event, cfg)
			}, "NotifyIfEnabled should not panic")
		})
	}
}

func TestNotifyIfEnabled_NilConfig(t *testing.T) {
	assert.NotPanics(t, func() {
		NotifyIfEnabled(NotifyEventAwaitingApproval, nil)
	}, "NotifyIfEnabled should not panic with nil config")
}
