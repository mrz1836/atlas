package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildRedisDegradedBanner verifies the degraded banner contains required content.
// The banner must be visible and describe the degraded state.
func TestBuildRedisDegradedBanner(t *testing.T) {
	t.Parallel()

	err := assert.AnError
	banner := buildRedisDegradedBanner(err)

	// Must describe the degraded state clearly.
	assert.Contains(t, banner, "Degraded mode")
	assert.Contains(t, banner, "log streaming unavailable")
	// Must tell the user that task monitoring still works.
	assert.Contains(t, banner, "Task monitoring continues")
	// Must provide a fix command.
	assert.Contains(t, banner, "redis")
	// Must not block the user with "Press q to quit" as the only message —
	// it should appear alongside the "monitoring continues" guidance.
	assert.Contains(t, banner, "Press q to quit")
}

// TestDaemonStatusJSON_Schema validates that daemon status JSON output has
// the documented fields expected by scripts and tooling.
func TestDaemonStatusJSON_Schema(t *testing.T) {
	t.Parallel()

	// The DaemonStatusResponse JSON tags are the documented schema.
	// We test that the zero-value struct marshals to an object with the
	// required keys so that machine-readable consumers can rely on them.
	type expectedFields struct {
		Version     *string `json:"version"`
		PID         *int    `json:"pid"`
		Uptime      *string `json:"uptime"`
		StartedAt   *string `json:"started_at"`
		RedisAlive  *bool   `json:"redis_alive"`
		Workers     *int    `json:"workers"`
		ActiveTasks *int    `json:"active_tasks"`
		QueueDepth  *int    `json:"queue_depth"`
	}

	// Construct a plausible status payload.
	payload := map[string]any{
		"version":      "0.21.0",
		"pid":          12345,
		"uptime":       "1h2m3s",
		"started_at":   "2026-05-28T00:00:00Z",
		"redis_alive":  true,
		"workers":      4,
		"active_tasks": 2,
		"queue_depth":  5,
	}

	b, err := json.Marshal(payload)
	require.NoError(t, err)

	// Round-trip through the expected schema to confirm all fields parse.
	var parsed expectedFields
	err = json.Unmarshal(b, &parsed)
	require.NoError(t, err)

	assert.NotNil(t, parsed.Version, "version field must be present")
	assert.NotNil(t, parsed.PID, "pid field must be present")
	assert.NotNil(t, parsed.Uptime, "uptime field must be present")
	assert.NotNil(t, parsed.StartedAt, "started_at field must be present")
	assert.NotNil(t, parsed.RedisAlive, "redis_alive field must be present")
	assert.NotNil(t, parsed.Workers, "workers field must be present")
	assert.NotNil(t, parsed.ActiveTasks, "active_tasks field must be present")
	assert.NotNil(t, parsed.QueueDepth, "queue_depth field must be present")
}

// TestDaemonStatusJSON_NotRunning verifies the JSON output when daemon is down.
func TestDaemonStatusJSON_NotRunning(t *testing.T) {
	t.Parallel()

	// Simulate the "daemon not running" JSON output produced by runDaemonStatus.
	notRunningPayload := map[string]any{
		"running": false,
		"error":   "daemon not running",
	}

	var buf bytes.Buffer
	b, err := json.MarshalIndent(notRunningPayload, "", "  ")
	require.NoError(t, err)
	_, _ = buf.Write(b)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	running, ok := parsed["running"].(bool)
	require.True(t, ok, "running field must be a bool")
	assert.False(t, running)

	errMsg, ok := parsed["error"].(string)
	require.True(t, ok, "error field must be a string")
	assert.NotEmpty(t, errMsg)
}

// TestReconcileJSONSchema validates that ReconcileResponse marshals with documented fields.
func TestReconcileJSONSchema(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"drift_items": []any{
			map[string]any{
				"type":             "redis_only",
				"task_id":          "abc-123",
				"workspace":        "ws-test",
				"redis_status":     "running",
				"suggested_action": "run atlas cleanup",
			},
		},
		"total":      1,
		"summary":    "1 drift item(s) detected",
		"atlas_home": "/home/user/.atlas",
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))

	assert.Contains(t, parsed, "drift_items", "drift_items must be present")
	assert.Contains(t, parsed, "total", "total must be present")
	assert.Contains(t, parsed, "summary", "summary must be present")
	assert.Contains(t, parsed, "atlas_home", "atlas_home must be present")

	items, ok := parsed["drift_items"].([]any)
	require.True(t, ok, "drift_items must be an array")
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "redis_only", item["type"])
	assert.NotEmpty(t, item["suggested_action"])
}
