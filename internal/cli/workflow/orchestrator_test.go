package workflow

import (
	"testing"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// checkExpectedKeys verifies that all expected keys are present in task metadata
func checkExpectedKeys(t *testing.T, task *domain.Task, wantKeys []string) {
	t.Helper()
	for _, key := range wantKeys {
		if _, ok := task.Metadata[key]; !ok {
			t.Errorf("expected key %q to be present in metadata", key)
		}
	}
}

// checkUnexpectedKeys verifies that no unexpected keys are present in task metadata
func checkUnexpectedKeys(t *testing.T, task *domain.Task, wantKeys []string) {
	t.Helper()
	allKeys := []string{
		constants.MetaKeyVerifyOverride,
		constants.MetaKeyNoVerifyOverride,
		constants.MetaKeyAgentOverride,
		constants.MetaKeyModelOverride,
	}
	for _, key := range allKeys {
		if !contains(wantKeys, key) {
			if _, ok := task.Metadata[key]; ok {
				t.Errorf("unexpected key %q in metadata", key)
			}
		}
	}
}

// contains checks if a string slice contains a specific value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// verifyMetadataValues checks that metadata values match expected values
func verifyMetadataValues(t *testing.T, task *domain.Task, verify, noVerify bool, agent, model string) {
	t.Helper()
	if verify {
		if v, _ := task.Metadata[constants.MetaKeyVerifyOverride].(bool); !v {
			t.Errorf("expected cli_verify to be true")
		}
	}
	if noVerify {
		if v, _ := task.Metadata[constants.MetaKeyNoVerifyOverride].(bool); !v {
			t.Errorf("expected cli_no_verify to be true")
		}
	}
	if agent != "" {
		if v, _ := task.Metadata[constants.MetaKeyAgentOverride].(string); v != agent {
			t.Errorf("expected cli_agent to be %q, got %q", agent, v)
		}
	}
	if model != "" {
		if v, _ := task.Metadata[constants.MetaKeyModelOverride].(string); v != model {
			t.Errorf("expected cli_model to be %q, got %q", model, v)
		}
	}
}

func TestStoreCLIOverrides(t *testing.T) {
	tests := []struct {
		name     string
		verify   bool
		noVerify bool
		agent    string
		model    string
		wantKeys []string // keys that should be present
	}{
		{
			name:     "no flags set",
			verify:   false,
			noVerify: false,
			agent:    "",
			model:    "",
			wantKeys: []string{},
		},
		{
			name:     "verify flag set",
			verify:   true,
			noVerify: false,
			agent:    "",
			model:    "",
			wantKeys: []string{constants.MetaKeyVerifyOverride},
		},
		{
			name:     "no-verify flag set",
			verify:   false,
			noVerify: true,
			agent:    "",
			model:    "",
			wantKeys: []string{constants.MetaKeyNoVerifyOverride},
		},
		{
			name:     "agent flag set",
			verify:   false,
			noVerify: false,
			agent:    "gemini",
			model:    "",
			wantKeys: []string{constants.MetaKeyAgentOverride},
		},
		{
			name:     "model flag set",
			verify:   false,
			noVerify: false,
			agent:    "",
			model:    "opus",
			wantKeys: []string{constants.MetaKeyModelOverride},
		},
		{
			name:     "all flags set",
			verify:   true,
			noVerify: false,
			agent:    "claude",
			model:    "sonnet",
			wantKeys: []string{
				constants.MetaKeyVerifyOverride,
				constants.MetaKeyAgentOverride,
				constants.MetaKeyModelOverride,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID: "test-task",
			}

			StoreCLIOverrides(task, tt.verify, tt.noVerify, tt.agent, tt.model)

			checkExpectedKeys(t, task, tt.wantKeys)
			checkUnexpectedKeys(t, task, tt.wantKeys)
			verifyMetadataValues(t, task, tt.verify, tt.noVerify, tt.agent, tt.model)
		})
	}
}

func TestStoreCLIOverrides_NilMetadata(t *testing.T) {
	task := &domain.Task{
		ID:       "test-task",
		Metadata: nil,
	}

	StoreCLIOverrides(task, true, false, "claude", "sonnet")

	if task.Metadata == nil {
		t.Error("expected metadata to be initialized")
	}
	if v, _ := task.Metadata[constants.MetaKeyVerifyOverride].(bool); !v {
		t.Error("expected cli_verify to be set")
	}
}

func TestApplyCLIOverridesFromTask(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]any
		wantVerify bool
		wantAgent  domain.Agent
		wantModel  string
	}{
		{
			name:       "nil metadata",
			metadata:   nil,
			wantVerify: false, // template default
			wantAgent:  "",
			wantModel:  "",
		},
		{
			name:       "empty metadata",
			metadata:   map[string]any{},
			wantVerify: false,
			wantAgent:  "",
			wantModel:  "",
		},
		{
			name: "verify override",
			metadata: map[string]any{
				constants.MetaKeyVerifyOverride: true,
			},
			wantVerify: true,
			wantAgent:  "",
			wantModel:  "",
		},
		{
			name: "no-verify override",
			metadata: map[string]any{
				constants.MetaKeyNoVerifyOverride: true,
			},
			wantVerify: false,
			wantAgent:  "",
			wantModel:  "",
		},
		{
			name: "agent override",
			metadata: map[string]any{
				constants.MetaKeyAgentOverride: "gemini",
			},
			wantVerify: false,
			wantAgent:  domain.Agent("gemini"),
			wantModel:  "",
		},
		{
			name: "model override",
			metadata: map[string]any{
				constants.MetaKeyModelOverride: "opus",
			},
			wantVerify: false,
			wantAgent:  "",
			wantModel:  "opus",
		},
		{
			name: "all overrides",
			metadata: map[string]any{
				constants.MetaKeyVerifyOverride: true,
				constants.MetaKeyAgentOverride:  "claude",
				constants.MetaKeyModelOverride:  "sonnet",
			},
			wantVerify: true,
			wantAgent:  domain.Agent("claude"),
			wantModel:  "sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:       "test-task",
				Metadata: tt.metadata,
			}

			// Create a template with a verify step to test verify override
			tmpl := &domain.Template{
				Name:         "test-template",
				Verify:       false,
				DefaultAgent: "",
				DefaultModel: "",
				Steps: []domain.StepDefinition{
					{
						Name:     "verify",
						Type:     domain.StepTypeVerify,
						Required: false,
					},
				},
			}

			ApplyCLIOverridesFromTask(task, tmpl)

			if tmpl.Verify != tt.wantVerify {
				t.Errorf("Verify = %v, want %v", tmpl.Verify, tt.wantVerify)
			}
			if tmpl.DefaultAgent != tt.wantAgent {
				t.Errorf("DefaultAgent = %v, want %v", tmpl.DefaultAgent, tt.wantAgent)
			}
			if tmpl.DefaultModel != tt.wantModel {
				t.Errorf("DefaultModel = %v, want %v", tmpl.DefaultModel, tt.wantModel)
			}
		})
	}
}

func TestApplyCLIOverridesFromTask_VerifyStepRequired(t *testing.T) {
	// Test that verify step's Required field is updated
	task := &domain.Task{
		ID: "test-task",
		Metadata: map[string]any{
			constants.MetaKeyVerifyOverride: true,
		},
	}

	tmpl := &domain.Template{
		Name:   "test-template",
		Verify: false,
		Steps: []domain.StepDefinition{
			{
				Name:     "implement",
				Type:     domain.StepTypeAI,
				Required: true,
			},
			{
				Name:     "verify",
				Type:     domain.StepTypeVerify,
				Required: false, // Start as optional
			},
		},
	}

	ApplyCLIOverridesFromTask(task, tmpl)

	// Verify step should now be required
	verifyStep := tmpl.Steps[1]
	if !verifyStep.Required {
		t.Error("expected verify step to be required after applying override")
	}
}

func TestCLIOverrides_RoundTrip(t *testing.T) {
	// Test that storing and then applying overrides produces the expected result
	tests := []struct {
		name     string
		verify   bool
		noVerify bool
		agent    string
		model    string
	}{
		{
			name:   "verify enabled",
			verify: true,
		},
		{
			name:     "verify disabled",
			noVerify: true,
		},
		{
			name:  "agent override",
			agent: "gemini",
		},
		{
			name:  "model override",
			model: "opus",
		},
		{
			name:   "all overrides",
			verify: true,
			agent:  "claude",
			model:  "sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a task and store overrides
			task := &domain.Task{ID: "test-task"}
			StoreCLIOverrides(task, tt.verify, tt.noVerify, tt.agent, tt.model)

			// Create a template with defaults that differ from overrides
			tmpl := &domain.Template{
				Name:         "test-template",
				Verify:       !tt.verify && !tt.noVerify, // Opposite of expected
				DefaultAgent: "default-agent",
				DefaultModel: "default-model",
				Steps: []domain.StepDefinition{
					{
						Name:     "verify",
						Type:     domain.StepTypeVerify,
						Required: false,
					},
				},
			}

			// Apply overrides from task
			ApplyCLIOverridesFromTask(task, tmpl)

			// Verify the template was updated correctly
			if tt.verify && !tmpl.Verify {
				t.Error("expected Verify to be true after round-trip")
			}
			if tt.noVerify && tmpl.Verify {
				t.Error("expected Verify to be false after round-trip")
			}
			if tt.agent != "" && string(tmpl.DefaultAgent) != tt.agent {
				t.Errorf("expected DefaultAgent to be %q, got %q", tt.agent, tmpl.DefaultAgent)
			}
			if tt.model != "" && tmpl.DefaultModel != tt.model {
				t.Errorf("expected DefaultModel to be %q, got %q", tt.model, tmpl.DefaultModel)
			}
		})
	}
}

func TestApplyCLIOverridesFromTask_WrongTypes(t *testing.T) {
	// Test that wrong types in metadata don't cause panics
	task := &domain.Task{
		ID: "test-task",
		Metadata: map[string]any{
			constants.MetaKeyVerifyOverride:   "not-a-bool",     // wrong type
			constants.MetaKeyNoVerifyOverride: 123,              // wrong type
			constants.MetaKeyAgentOverride:    true,             // wrong type
			constants.MetaKeyModelOverride:    []string{"test"}, // wrong type
		},
	}

	tmpl := &domain.Template{
		Name:         "test-template",
		Verify:       true, // should remain unchanged
		DefaultAgent: "original",
		DefaultModel: "original",
		Steps: []domain.StepDefinition{
			{
				Name:     "verify",
				Type:     domain.StepTypeVerify,
				Required: true,
			},
		},
	}

	// Should not panic
	ApplyCLIOverridesFromTask(task, tmpl)

	// Values should remain unchanged due to type assertion failures
	if !tmpl.Verify {
		t.Error("Verify should remain true when metadata has wrong type")
	}
	if tmpl.DefaultAgent != "original" {
		t.Errorf("DefaultAgent should remain 'original', got %q", tmpl.DefaultAgent)
	}
	if tmpl.DefaultModel != "original" {
		t.Errorf("DefaultModel should remain 'original', got %q", tmpl.DefaultModel)
	}
}

func TestStoreCLIOverrides_PreservesExistingMetadata(t *testing.T) {
	// Test that storing overrides doesn't overwrite existing metadata
	task := &domain.Task{
		ID: "test-task",
		Metadata: map[string]any{
			"branch":       "feature/test",
			"worktree_dir": "/path/to/worktree",
			"custom_key":   "custom_value",
		},
	}

	StoreCLIOverrides(task, true, false, "claude", "sonnet")

	// Original metadata should still be present
	if task.Metadata["branch"] != "feature/test" {
		t.Error("branch metadata was overwritten")
	}
	if task.Metadata["worktree_dir"] != "/path/to/worktree" {
		t.Error("worktree_dir metadata was overwritten")
	}
	if task.Metadata["custom_key"] != "custom_value" {
		t.Error("custom_key metadata was overwritten")
	}

	// New overrides should be added
	if v, _ := task.Metadata[constants.MetaKeyVerifyOverride].(bool); !v {
		t.Error("cli_verify should be set")
	}
	if v, _ := task.Metadata[constants.MetaKeyAgentOverride].(string); v != "claude" {
		t.Error("cli_agent should be set to 'claude'")
	}
	if v, _ := task.Metadata[constants.MetaKeyModelOverride].(string); v != "sonnet" {
		t.Error("cli_model should be set to 'sonnet'")
	}
}

func TestApplyCLIOverridesFromTask_NoVerifyDisablesVerify(t *testing.T) {
	// Test that --no-verify actually disables verification even when template enables it
	task := &domain.Task{
		ID: "test-task",
		Metadata: map[string]any{
			constants.MetaKeyNoVerifyOverride: true,
		},
	}

	tmpl := &domain.Template{
		Name:   "test-template",
		Verify: true, // Template enables verify by default
		Steps: []domain.StepDefinition{
			{
				Name:     "verify",
				Type:     domain.StepTypeVerify,
				Required: true, // Should become false
			},
		},
	}

	ApplyCLIOverridesFromTask(task, tmpl)

	if tmpl.Verify {
		t.Error("expected Verify to be false after applying --no-verify override")
	}
	if tmpl.Steps[0].Required {
		t.Error("expected verify step to be not required after applying --no-verify override")
	}
}
