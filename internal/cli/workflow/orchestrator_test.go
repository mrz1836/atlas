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

func TestGenerateWorkspaceName(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "simple description",
			description: "Add user authentication",
			want:        "add-user-authentication",
		},
		{
			name:        "description with special characters",
			description: "Fix bug #123: API error!",
			want:        "fix-bug-123-api-error",
		},
		{
			name:        "description with multiple spaces",
			description: "Update   multiple   spaces",
			want:        "update-multiple-spaces",
		},
		{
			name:        "empty description",
			description: "",
			want:        "", // Will be replaced with timestamp
		},
		{
			name:        "long description",
			description: "This is a very long description that should be truncated to fit within the maximum workspace name length limit",
			want:        "this-is-a-very-long-description-that-should-be-tru",
		},
		{
			name:        "description with trailing hyphen after truncation",
			description: "this is a description that ends with special chars-----------",
			want:        "this-is-a-description-that-ends-with-special-chars",
		},
		{
			name:        "only special characters",
			description: "!@#$%^&*()",
			want:        "", // Will be replaced with timestamp
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateWorkspaceName(tt.description)
			if tt.want == "" {
				// Check that a timestamp-based name was generated
				if got == "" || len(got) < 5 {
					t.Errorf("GenerateWorkspaceName() = %v, expected non-empty timestamp-based name", got)
				}
			} else {
				if got != tt.want {
					t.Errorf("GenerateWorkspaceName() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSanitizeWorkspaceName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic sanitization",
			input: "Hello World",
			want:  "hello-world",
		},
		{
			name:  "special characters removed",
			input: "test@#$name",
			want:  "testname",
		},
		{
			name:  "multiple hyphens collapsed",
			input: "test---name",
			want:  "test-name",
		},
		{
			name:  "leading and trailing hyphens trimmed",
			input: "-test-name-",
			want:  "test-name",
		},
		{
			name:  "truncated to max length",
			input: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
			want:  "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeWorkspaceName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeWorkspaceName() = %v, want %v", got, tt.want)
			}
			if len(got) > maxWorkspaceNameLen {
				t.Errorf("sanitizeWorkspaceName() length = %v, exceeds max %v", len(got), maxWorkspaceNameLen)
			}
		})
	}
}

func TestApplyAgentModelOverrides(t *testing.T) {
	tests := []struct {
		name      string
		agent     string
		model     string
		wantAgent domain.Agent
		wantModel string
	}{
		{
			name:      "both agent and model set",
			agent:     "gemini",
			model:     "opus",
			wantAgent: domain.Agent("gemini"),
			wantModel: "opus",
		},
		{
			name:      "only agent set",
			agent:     "claude",
			model:     "",
			wantAgent: domain.Agent("claude"),
			wantModel: "",
		},
		{
			name:      "only model set",
			agent:     "",
			model:     "sonnet",
			wantAgent: "",
			wantModel: "sonnet",
		},
		{
			name:      "neither set",
			agent:     "",
			model:     "",
			wantAgent: "",
			wantModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &domain.Template{
				Name:         "test-template",
				DefaultAgent: "",
				DefaultModel: "",
			}

			ApplyAgentModelOverrides(tmpl, tt.agent, tt.model)

			if tmpl.DefaultAgent != tt.wantAgent {
				t.Errorf("DefaultAgent = %v, want %v", tmpl.DefaultAgent, tt.wantAgent)
			}
			if tmpl.DefaultModel != tt.wantModel {
				t.Errorf("DefaultModel = %v, want %v", tmpl.DefaultModel, tt.wantModel)
			}
		})
	}
}

func TestApplyVerifyOverrides(t *testing.T) {
	tests := []struct {
		name       string
		verify     bool
		noVerify   bool
		tmplVerify bool
		wantVerify bool
	}{
		{
			name:       "verify flag overrides template default",
			verify:     true,
			noVerify:   false,
			tmplVerify: false,
			wantVerify: true,
		},
		{
			name:       "no-verify flag overrides template default",
			verify:     false,
			noVerify:   true,
			tmplVerify: true,
			wantVerify: false,
		},
		{
			name:       "no flags uses template default true",
			verify:     false,
			noVerify:   false,
			tmplVerify: true,
			wantVerify: true,
		},
		{
			name:       "no flags uses template default false",
			verify:     false,
			noVerify:   false,
			tmplVerify: false,
			wantVerify: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &domain.Template{
				Name:   "test-template",
				Verify: tt.tmplVerify,
				Steps: []domain.StepDefinition{
					{
						Name:     "verify",
						Type:     domain.StepTypeVerify,
						Required: tt.tmplVerify,
					},
				},
			}

			ApplyVerifyOverrides(tmpl, tt.verify, tt.noVerify)

			if tmpl.Verify != tt.wantVerify {
				t.Errorf("Verify = %v, want %v", tmpl.Verify, tt.wantVerify)
			}
			if tmpl.Steps[0].Required != tt.wantVerify {
				t.Errorf("verify step Required = %v, want %v", tmpl.Steps[0].Required, tt.wantVerify)
			}
		})
	}
}

func TestApplyVerifyOverrides_VerifyModelPropagation(t *testing.T) {
	t.Run("propagates VerifyModel when no different agent", func(t *testing.T) {
		tmpl := &domain.Template{
			Name:        "test-template",
			Verify:      true,
			VerifyModel: "opus-3",
			Steps: []domain.StepDefinition{
				{
					Name:     "verify",
					Type:     domain.StepTypeVerify,
					Required: true,
					Config:   nil,
				},
			},
		}

		ApplyVerifyOverrides(tmpl, true, false)

		if tmpl.Steps[0].Config == nil {
			t.Fatal("expected Config to be initialized")
		}
		if model, ok := tmpl.Steps[0].Config["model"].(string); !ok || model != "opus-3" {
			t.Errorf("expected model to be 'opus-3', got %v", model)
		}
	})

	t.Run("does not propagate VerifyModel when step has different agent", func(t *testing.T) {
		tmpl := &domain.Template{
			Name:         "test-template",
			Verify:       true,
			VerifyModel:  "opus-3",
			DefaultAgent: domain.Agent("claude"),
			Steps: []domain.StepDefinition{
				{
					Name:     "verify",
					Type:     domain.StepTypeVerify,
					Required: true,
					Config: map[string]any{
						"agent": "gemini", // Different agent
					},
				},
			},
		}

		ApplyVerifyOverrides(tmpl, true, false)

		// Model should not be propagated because step has different agent
		if model, ok := tmpl.Steps[0].Config["model"].(string); ok && model == "opus-3" {
			t.Errorf("expected model not to be propagated when step has different agent")
		}
	})

	t.Run("does not override existing step model", func(t *testing.T) {
		tmpl := &domain.Template{
			Name:        "test-template",
			Verify:      true,
			VerifyModel: "opus-3",
			Steps: []domain.StepDefinition{
				{
					Name:     "verify",
					Type:     domain.StepTypeVerify,
					Required: true,
					Config: map[string]any{
						"model": "sonnet-4", // Existing model
					},
				},
			},
		}

		ApplyVerifyOverrides(tmpl, true, false)

		// Existing model should be preserved
		if model, ok := tmpl.Steps[0].Config["model"].(string); !ok || model != "sonnet-4" {
			t.Errorf("expected model to remain 'sonnet-4', got %v", model)
		}
	})
}

func TestStepHasDifferentAgent(t *testing.T) {
	tests := []struct {
		name         string
		stepConfig   map[string]any
		defaultAgent domain.Agent
		want         bool
	}{
		{
			name:         "nil config",
			stepConfig:   nil,
			defaultAgent: domain.Agent("claude"),
			want:         false,
		},
		{
			name:         "empty config",
			stepConfig:   map[string]any{},
			defaultAgent: domain.Agent("claude"),
			want:         false,
		},
		{
			name: "same agent",
			stepConfig: map[string]any{
				"agent": "claude",
			},
			defaultAgent: domain.Agent("claude"),
			want:         false,
		},
		{
			name: "different agent",
			stepConfig: map[string]any{
				"agent": "gemini",
			},
			defaultAgent: domain.Agent("claude"),
			want:         true,
		},
		{
			name: "empty agent string",
			stepConfig: map[string]any{
				"agent": "",
			},
			defaultAgent: domain.Agent("claude"),
			want:         false,
		},
		{
			name: "agent wrong type",
			stepConfig: map[string]any{
				"agent": 123,
			},
			defaultAgent: domain.Agent("claude"),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &domain.StepDefinition{
				Config: tt.stepConfig,
			}
			got := stepHasDifferentAgent(step, tt.defaultAgent)
			if got != tt.want {
				t.Errorf("stepHasDifferentAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldPropagateVerifyModel(t *testing.T) {
	tests := []struct {
		name                  string
		verifyModel           string
		stepHasDifferentAgent bool
		want                  bool
	}{
		{
			name:                  "should propagate when model set and no different agent",
			verifyModel:           "opus-3",
			stepHasDifferentAgent: false,
			want:                  true,
		},
		{
			name:                  "should not propagate when model empty",
			verifyModel:           "",
			stepHasDifferentAgent: false,
			want:                  false,
		},
		{
			name:                  "should not propagate when step has different agent",
			verifyModel:           "opus-3",
			stepHasDifferentAgent: true,
			want:                  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldPropagateVerifyModel(tt.verifyModel, tt.stepHasDifferentAgent)
			if got != tt.want {
				t.Errorf("shouldPropagateVerifyModel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPropagateVerifyModel(t *testing.T) {
	t.Run("initializes config if nil", func(t *testing.T) {
		step := &domain.StepDefinition{
			Config: nil,
		}
		propagateVerifyModel(step, "opus-3")
		if step.Config == nil {
			t.Error("expected Config to be initialized")
		}
		if model, ok := step.Config["model"].(string); !ok || model != "opus-3" {
			t.Errorf("expected model to be 'opus-3', got %v", model)
		}
	})

	t.Run("sets model when config exists but model not set", func(t *testing.T) {
		step := &domain.StepDefinition{
			Config: map[string]any{"other": "value"},
		}
		propagateVerifyModel(step, "opus-3")
		if model, ok := step.Config["model"].(string); !ok || model != "opus-3" {
			t.Errorf("expected model to be 'opus-3', got %v", model)
		}
	})

	t.Run("does not override existing model", func(t *testing.T) {
		step := &domain.StepDefinition{
			Config: map[string]any{"model": "existing-model"},
		}
		propagateVerifyModel(step, "opus-3")
		if model, ok := step.Config["model"].(string); !ok || model != "existing-model" {
			t.Errorf("expected model to remain 'existing-model', got %v", model)
		}
	})
}
