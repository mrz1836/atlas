package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Example JSON documents for documentation purposes.
// These demonstrate the expected JSON format with snake_case field names.
const (
	// exampleTaskJSON shows the expected JSON serialization format for Task.
	exampleTaskJSON = `{
    "id": "task-550e8400-e29b-41d4-a716-446655440000",
    "workspace_id": "auth-workspace",
    "template_id": "bugfix",
    "description": "Fix null pointer in parseConfig",
    "status": "running",
    "current_step": 1,
    "steps": [
        {
            "name": "analyze",
            "type": "ai",
            "status": "completed",
            "started_at": "2025-12-27T10:00:00Z",
            "completed_at": "2025-12-27T10:05:00Z",
            "attempts": 1
        },
        {
            "name": "implement",
            "type": "ai",
            "status": "running",
            "started_at": "2025-12-27T10:05:00Z",
            "attempts": 1
        }
    ],
    "created_at": "2025-12-27T10:00:00Z",
    "updated_at": "2025-12-27T10:05:00Z",
    "config": {
        "model": "claude-sonnet-4-20250514",
        "max_turns": 10
    },
    "schema_version": "1.0"
}`

	// exampleWorkspaceJSON shows the expected JSON serialization format for Workspace.
	exampleWorkspaceJSON = `{
    "name": "auth-feature",
    "path": "/home/user/.atlas/workspaces/auth-feature/",
    "worktree_path": "../repo-auth-feature/",
    "branch": "feat/user-auth",
    "status": "active",
    "tasks": [
        {
            "id": "task-550e8400-e29b-41d4-a716-446655440000",
            "status": "completed",
            "started_at": "2025-12-27T10:00:00Z",
            "completed_at": "2025-12-27T10:30:00Z"
        }
    ],
    "created_at": "2025-12-27T09:00:00Z",
    "updated_at": "2025-12-27T10:30:00Z",
    "schema_version": 1
}`
)

// TestTask_JSONSerialization verifies Task marshals to JSON with snake_case keys.
func TestTask_JSONSerialization(t *testing.T) {
	now := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)

	task := Task{
		ID:          "task-550e8400-e29b-41d4-a716-446655440000",
		WorkspaceID: "auth-workspace",
		TemplateID:  "bugfix",
		Description: "Fix null pointer in parseConfig",
		Status:      TaskStatusRunning,
		CurrentStep: 1,
		Steps: []Step{
			{
				Name:        "analyze",
				Type:        StepTypeAI,
				Status:      "completed",
				StartedAt:   &now,
				CompletedAt: &later,
				Attempts:    1,
			},
		},
		CreatedAt: now,
		UpdatedAt: later,
		Config: TaskConfig{
			Model:    "claude-sonnet-4-20250514",
			MaxTurns: 10,
		},
		SchemaVersion: "1.0",
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"workspace_id"`)
	assert.Contains(t, jsonStr, `"template_id"`)
	assert.Contains(t, jsonStr, `"current_step"`)
	assert.Contains(t, jsonStr, `"created_at"`)
	assert.Contains(t, jsonStr, `"updated_at"`)
	assert.Contains(t, jsonStr, `"schema_version"`)
	assert.Contains(t, jsonStr, `"max_turns"`)
	assert.Contains(t, jsonStr, `"started_at"`)
	assert.Contains(t, jsonStr, `"completed_at"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"workspaceId"`)
	assert.NotContains(t, jsonStr, `"templateId"`)
	assert.NotContains(t, jsonStr, `"currentStep"`)
	assert.NotContains(t, jsonStr, `"createdAt"`)
	assert.NotContains(t, jsonStr, `"updatedAt"`)
	assert.NotContains(t, jsonStr, `"schemaVersion"`)
	assert.NotContains(t, jsonStr, `"maxTurns"`)
	assert.NotContains(t, jsonStr, `"startedAt"`)
	assert.NotContains(t, jsonStr, `"completedAt"`)

	// Round-trip test
	var decoded Task
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, task.ID, decoded.ID)
	assert.Equal(t, task.WorkspaceID, decoded.WorkspaceID)
	assert.Equal(t, task.TemplateID, decoded.TemplateID)
	assert.Equal(t, task.Description, decoded.Description)
	assert.Equal(t, task.Status, decoded.Status)
	assert.Equal(t, task.CurrentStep, decoded.CurrentStep)
	assert.Equal(t, task.SchemaVersion, decoded.SchemaVersion)
	require.Len(t, decoded.Steps, 1)
	assert.Equal(t, task.Steps[0].Name, decoded.Steps[0].Name)
	assert.Equal(t, task.Steps[0].Type, decoded.Steps[0].Type)
}

// TestWorkspace_JSONSerialization verifies Workspace marshals to JSON with snake_case keys.
func TestWorkspace_JSONSerialization(t *testing.T) {
	now := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
	later := now.Add(30 * time.Minute)

	ws := Workspace{
		Name:         "auth-feature",
		Path:         "/home/user/.atlas/workspaces/auth-feature/",
		WorktreePath: "../repo-auth-feature/",
		Branch:       "feat/user-auth",
		Status:       WorkspaceStatusActive,
		Tasks: []TaskRef{
			{
				ID:          "task-550e8400-e29b-41d4-a716-446655440000",
				Status:      TaskStatusCompleted,
				StartedAt:   &now,
				CompletedAt: &later,
			},
		},
		CreatedAt:     now,
		UpdatedAt:     later,
		SchemaVersion: 1,
	}

	data, err := json.Marshal(ws)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"worktree_path"`)
	assert.Contains(t, jsonStr, `"created_at"`)
	assert.Contains(t, jsonStr, `"updated_at"`)
	assert.Contains(t, jsonStr, `"schema_version"`)
	assert.Contains(t, jsonStr, `"started_at"`)
	assert.Contains(t, jsonStr, `"completed_at"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"worktreePath"`)
	assert.NotContains(t, jsonStr, `"createdAt"`)
	assert.NotContains(t, jsonStr, `"updatedAt"`)
	assert.NotContains(t, jsonStr, `"schemaVersion"`)
	assert.NotContains(t, jsonStr, `"startedAt"`)
	assert.NotContains(t, jsonStr, `"completedAt"`)

	// Round-trip test
	var decoded Workspace
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, ws.Name, decoded.Name)
	assert.Equal(t, ws.Path, decoded.Path)
	assert.Equal(t, ws.WorktreePath, decoded.WorktreePath)
	assert.Equal(t, ws.Branch, decoded.Branch)
	assert.Equal(t, ws.Status, decoded.Status)
	assert.Equal(t, ws.SchemaVersion, decoded.SchemaVersion)
	require.Len(t, decoded.Tasks, 1)
	assert.Equal(t, ws.Tasks[0].ID, decoded.Tasks[0].ID)
}

// TestTemplate_JSONSerialization verifies Template marshals to JSON with snake_case keys.
func TestTemplate_JSONSerialization(t *testing.T) {
	tmpl := Template{
		Name:         "bugfix",
		Description:  "Fix a reported bug",
		BranchPrefix: "fix/",
		DefaultModel: "claude-sonnet-4-20250514",
		Steps: []StepDefinition{
			{
				Name:        "analyze",
				Type:        StepTypeAI,
				Description: "Analyze the bug",
				Required:    true,
				Timeout:     10 * time.Minute,
				RetryCount:  2,
			},
			{
				Name:     "validate",
				Type:     StepTypeValidation,
				Required: true,
			},
		},
		ValidationCommands: []string{"magex lint", "magex test"},
	}

	data, err := json.Marshal(tmpl)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"branch_prefix"`)
	assert.Contains(t, jsonStr, `"default_model"`)
	assert.Contains(t, jsonStr, `"validation_commands"`)
	assert.Contains(t, jsonStr, `"retry_count"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"branchPrefix"`)
	assert.NotContains(t, jsonStr, `"defaultModel"`)
	assert.NotContains(t, jsonStr, `"validationCommands"`)
	assert.NotContains(t, jsonStr, `"retryCount"`)

	// Round-trip test
	var decoded Template
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, tmpl.Name, decoded.Name)
	assert.Equal(t, tmpl.Description, decoded.Description)
	assert.Equal(t, tmpl.BranchPrefix, decoded.BranchPrefix)
	assert.Equal(t, tmpl.DefaultModel, decoded.DefaultModel)
	require.Len(t, decoded.Steps, 2)
	assert.Equal(t, tmpl.Steps[0].Name, decoded.Steps[0].Name)
	assert.Equal(t, tmpl.Steps[0].Type, decoded.Steps[0].Type)
	require.Len(t, decoded.ValidationCommands, 2)
}

// TestAIRequest_JSONSerialization verifies AIRequest marshals to JSON with snake_case keys.
func TestAIRequest_JSONSerialization(t *testing.T) {
	req := AIRequest{
		Prompt:         "Fix the null pointer in parseConfig",
		Context:        "This is a Go project",
		Model:          "claude-sonnet-4-20250514",
		MaxTurns:       10,
		Timeout:        30 * time.Minute,
		PermissionMode: "plan",
		SystemPrompt:   "You are a helpful coding assistant",
		WorkingDir:     "/path/to/repo",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"max_turns"`)
	assert.Contains(t, jsonStr, `"permission_mode"`)
	assert.Contains(t, jsonStr, `"system_prompt"`)
	assert.Contains(t, jsonStr, `"working_dir"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"maxTurns"`)
	assert.NotContains(t, jsonStr, `"permissionMode"`)
	assert.NotContains(t, jsonStr, `"systemPrompt"`)
	assert.NotContains(t, jsonStr, `"workingDir"`)

	// Round-trip test
	var decoded AIRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.Prompt, decoded.Prompt)
	assert.Equal(t, req.Context, decoded.Context)
	assert.Equal(t, req.Model, decoded.Model)
	assert.Equal(t, req.MaxTurns, decoded.MaxTurns)
	assert.Equal(t, req.PermissionMode, decoded.PermissionMode)
	assert.Equal(t, req.WorkingDir, decoded.WorkingDir)
}

// TestAIResult_JSONSerialization verifies AIResult marshals to JSON with snake_case keys.
func TestAIResult_JSONSerialization(t *testing.T) {
	result := AIResult{
		Success:      true,
		Output:       "I've fixed the null pointer issue",
		SessionID:    "sess-abc123",
		DurationMs:   45000,
		NumTurns:     5,
		TotalCostUSD: 0.15,
		FilesChanged: []string{"internal/config/parser.go"},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"session_id"`)
	assert.Contains(t, jsonStr, `"duration_ms"`)
	assert.Contains(t, jsonStr, `"num_turns"`)
	assert.Contains(t, jsonStr, `"total_cost_usd"`)
	assert.Contains(t, jsonStr, `"files_changed"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"sessionId"`)
	assert.NotContains(t, jsonStr, `"durationMs"`)
	assert.NotContains(t, jsonStr, `"numTurns"`)
	assert.NotContains(t, jsonStr, `"totalCostUsd"`)
	assert.NotContains(t, jsonStr, `"filesChanged"`)

	// Round-trip test
	var decoded AIResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.Success, decoded.Success)
	assert.Equal(t, result.Output, decoded.Output)
	assert.Equal(t, result.SessionID, decoded.SessionID)
	assert.Equal(t, result.DurationMs, decoded.DurationMs)
	assert.Equal(t, result.NumTurns, decoded.NumTurns)
	assert.InDelta(t, result.TotalCostUSD, decoded.TotalCostUSD, 0.0001)
	require.Len(t, decoded.FilesChanged, 1)
}

// TestStepResult_JSONSerialization verifies StepResult marshals to JSON with snake_case keys.
func TestStepResult_JSONSerialization(t *testing.T) {
	now := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
	later := now.Add(45 * time.Second)

	result := StepResult{
		StepIndex:    1,
		StepName:     "implement",
		Status:       "success",
		StartedAt:    now,
		CompletedAt:  later,
		DurationMs:   45000,
		Output:       "Created 3 files",
		FilesChanged: []string{"cmd/main.go", "internal/service.go"},
		ArtifactPath: "/tmp/logs/step-1.log",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"step_index"`)
	assert.Contains(t, jsonStr, `"step_name"`)
	assert.Contains(t, jsonStr, `"duration_ms"`)
	assert.Contains(t, jsonStr, `"files_changed"`)
	assert.Contains(t, jsonStr, `"artifact_path"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"stepIndex"`)
	assert.NotContains(t, jsonStr, `"stepName"`)
	assert.NotContains(t, jsonStr, `"durationMs"`)
	assert.NotContains(t, jsonStr, `"filesChanged"`)
	assert.NotContains(t, jsonStr, `"artifactPath"`)

	// Round-trip test
	var decoded StepResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.StepIndex, decoded.StepIndex)
	assert.Equal(t, result.StepName, decoded.StepName)
	assert.Equal(t, result.Status, decoded.Status)
	assert.Equal(t, result.DurationMs, decoded.DurationMs)
	assert.Equal(t, result.Output, decoded.Output)
	require.Len(t, decoded.FilesChanged, 2)
}

// TestStepType_String verifies StepType String() method.
func TestStepType_String(t *testing.T) {
	tests := []struct {
		stepType StepType
		want     string
	}{
		{StepTypeAI, "ai"},
		{StepTypeValidation, "validation"},
		{StepTypeGit, "git"},
		{StepTypeHuman, "human"},
		{StepTypeSDD, "sdd"},
		{StepTypeCI, "ci"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.stepType.String())
		})
	}
}

// TestStatusReexports verifies that status constants are properly re-exported.
func TestStatusReexports(t *testing.T) {
	// Verify TaskStatus re-exports
	assert.Equal(t, "pending", string(TaskStatusPending))
	assert.Equal(t, "running", string(TaskStatusRunning))
	assert.Equal(t, "validating", string(TaskStatusValidating))
	assert.Equal(t, "validation_failed", string(TaskStatusValidationFailed))
	assert.Equal(t, "awaiting_approval", string(TaskStatusAwaitingApproval))
	assert.Equal(t, "completed", string(TaskStatusCompleted))
	assert.Equal(t, "rejected", string(TaskStatusRejected))
	assert.Equal(t, "abandoned", string(TaskStatusAbandoned))
	assert.Equal(t, "gh_failed", string(TaskStatusGHFailed))
	assert.Equal(t, "ci_failed", string(TaskStatusCIFailed))
	assert.Equal(t, "ci_timeout", string(TaskStatusCITimeout))

	// Verify WorkspaceStatus re-exports
	assert.Equal(t, "active", string(WorkspaceStatusActive))
	assert.Equal(t, "paused", string(WorkspaceStatusPaused))
	assert.Equal(t, "closed", string(WorkspaceStatusClosed))
}

// TestTask_OmitemptyFields verifies optional fields are omitted when empty.
func TestTask_OmitemptyFields(t *testing.T) {
	task := Task{
		ID:            "task-550e8400-e29b-41d4-a716-446655440000",
		WorkspaceID:   "ws-1",
		TemplateID:    "bugfix",
		Description:   "Test task",
		Status:        TaskStatusPending,
		CurrentStep:   0,
		Steps:         []Step{},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Config:        TaskConfig{},
		SchemaVersion: "1.0",
		// CompletedAt and Metadata are intentionally nil/empty
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when empty
	assert.NotContains(t, jsonStr, `"completed_at"`)
	assert.NotContains(t, jsonStr, `"metadata"`)
}

// TestWorkspace_OmitemptyFields verifies optional fields are omitted when empty.
func TestWorkspace_OmitemptyFields(t *testing.T) {
	ws := Workspace{
		Name:          "test-ws",
		Path:          "/tmp/ws",
		WorktreePath:  "/tmp/worktree",
		Branch:        "main",
		Status:        WorkspaceStatusActive,
		Tasks:         []TaskRef{},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		SchemaVersion: 1,
		// Metadata is intentionally nil
	}

	data, err := json.Marshal(ws)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty field is not present when empty
	assert.NotContains(t, jsonStr, `"metadata"`)
}

// TestTaskRef_OmitemptyFields verifies optional fields are omitted when empty.
func TestTaskRef_OmitemptyFields(t *testing.T) {
	ref := TaskRef{
		ID:     "task-1",
		Status: TaskStatusPending,
		// StartedAt and CompletedAt are intentionally nil
	}

	data, err := json.Marshal(ref)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when nil
	assert.NotContains(t, jsonStr, `"started_at"`)
	assert.NotContains(t, jsonStr, `"completed_at"`)
}

// TestStep_OmitemptyFields verifies optional fields are omitted when empty.
func TestStep_OmitemptyFields(t *testing.T) {
	step := Step{
		Name:     "test",
		Type:     StepTypeAI,
		Status:   "pending",
		Attempts: 0,
		// StartedAt, CompletedAt, and Error are intentionally nil/empty
	}

	data, err := json.Marshal(step)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when empty
	assert.NotContains(t, jsonStr, `"started_at"`)
	assert.NotContains(t, jsonStr, `"completed_at"`)
	assert.NotContains(t, jsonStr, `"error"`)
}

// TestAIResult_OmitemptyFields verifies optional fields are omitted when empty.
func TestAIResult_OmitemptyFields(t *testing.T) {
	result := AIResult{
		Success:    true,
		Output:     "Done",
		SessionID:  "sess-1",
		DurationMs: 1000,
		NumTurns:   1,
		// Error and FilesChanged are intentionally empty
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when empty
	assert.NotContains(t, jsonStr, `"error"`)
	assert.NotContains(t, jsonStr, `"files_changed"`)
}

// TestDeserializeExampleTaskJSON verifies we can parse the documented example JSON.
func TestDeserializeExampleTaskJSON(t *testing.T) {
	var task Task
	err := json.Unmarshal([]byte(exampleTaskJSON), &task)
	require.NoError(t, err)

	assert.Equal(t, "task-550e8400-e29b-41d4-a716-446655440000", task.ID)
	assert.Equal(t, "auth-workspace", task.WorkspaceID)
	assert.Equal(t, "bugfix", task.TemplateID)
	assert.Equal(t, "Fix null pointer in parseConfig", task.Description)
	assert.Equal(t, TaskStatusRunning, task.Status)
	assert.Equal(t, 1, task.CurrentStep)
	assert.Equal(t, "1.0", task.SchemaVersion)
	require.Len(t, task.Steps, 2)
	assert.Equal(t, "analyze", task.Steps[0].Name)
	assert.Equal(t, StepTypeAI, task.Steps[0].Type)
	assert.Equal(t, "completed", task.Steps[0].Status)
	assert.Equal(t, "implement", task.Steps[1].Name)
	assert.Equal(t, "running", task.Steps[1].Status)
}

// TestDeserializeExampleWorkspaceJSON verifies we can parse the documented example JSON.
func TestDeserializeExampleWorkspaceJSON(t *testing.T) {
	var ws Workspace
	err := json.Unmarshal([]byte(exampleWorkspaceJSON), &ws)
	require.NoError(t, err)

	assert.Equal(t, "auth-feature", ws.Name)
	assert.Equal(t, "/home/user/.atlas/workspaces/auth-feature/", ws.Path)
	assert.Equal(t, "../repo-auth-feature/", ws.WorktreePath)
	assert.Equal(t, "feat/user-auth", ws.Branch)
	assert.Equal(t, WorkspaceStatusActive, ws.Status)
	assert.Equal(t, 1, ws.SchemaVersion)
	require.Len(t, ws.Tasks, 1)
	assert.Equal(t, "task-550e8400-e29b-41d4-a716-446655440000", ws.Tasks[0].ID)
	assert.Equal(t, TaskStatusCompleted, ws.Tasks[0].Status)
}

// TestTaskConfig_JSONSerialization verifies TaskConfig marshals to JSON with snake_case keys.
func TestTaskConfig_JSONSerialization(t *testing.T) {
	cfg := TaskConfig{
		Model:              "claude-sonnet-4-20250514",
		MaxTurns:           15,
		Timeout:            30 * time.Minute,
		PermissionMode:     "plan",
		ValidationCommands: []string{"magex lint", "magex test"},
		Variables: map[string]string{
			"branch_name": "feat/test",
		},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify snake_case keys are present
	assert.Contains(t, jsonStr, `"max_turns"`)
	assert.Contains(t, jsonStr, `"permission_mode"`)
	assert.Contains(t, jsonStr, `"validation_commands"`)

	// Verify camelCase keys are NOT present
	assert.NotContains(t, jsonStr, `"maxTurns"`)
	assert.NotContains(t, jsonStr, `"permissionMode"`)
	assert.NotContains(t, jsonStr, `"validationCommands"`)

	// Round-trip test
	var decoded TaskConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, cfg.Model, decoded.Model)
	assert.Equal(t, cfg.MaxTurns, decoded.MaxTurns)
	assert.Equal(t, cfg.PermissionMode, decoded.PermissionMode)
	require.Len(t, decoded.ValidationCommands, 2)
	assert.Equal(t, "magex lint", decoded.ValidationCommands[0])
	require.Len(t, decoded.Variables, 1)
	assert.Equal(t, "feat/test", decoded.Variables["branch_name"])
}

// TestTaskConfig_OmitemptyFields verifies optional TaskConfig fields are omitted when empty.
func TestTaskConfig_OmitemptyFields(t *testing.T) {
	cfg := TaskConfig{
		// All fields are empty/zero
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when empty
	assert.NotContains(t, jsonStr, `"model"`)
	assert.NotContains(t, jsonStr, `"max_turns"`)
	assert.NotContains(t, jsonStr, `"timeout"`)
	assert.NotContains(t, jsonStr, `"permission_mode"`)
	assert.NotContains(t, jsonStr, `"validation_commands"`)
	assert.NotContains(t, jsonStr, `"variables"`)
}

// TestTemplateVariable_JSONSerialization verifies TemplateVariable marshals to JSON with snake_case keys.
func TestTemplateVariable_JSONSerialization(t *testing.T) {
	v := TemplateVariable{
		Description: "The target branch name",
		Default:     "main",
		Required:    true,
	}

	data, err := json.Marshal(v)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify expected keys are present
	assert.Contains(t, jsonStr, `"description"`)
	assert.Contains(t, jsonStr, `"default"`)
	assert.Contains(t, jsonStr, `"required"`)

	// Round-trip test
	var decoded TemplateVariable
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, v.Description, decoded.Description)
	assert.Equal(t, v.Default, decoded.Default)
	assert.Equal(t, v.Required, decoded.Required)
}

// TestTemplateVariable_OmitemptyFields verifies optional TemplateVariable fields are omitted when empty.
func TestTemplateVariable_OmitemptyFields(t *testing.T) {
	v := TemplateVariable{
		Required: false, // Only non-omitempty field
	}

	data, err := json.Marshal(v)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify omitempty fields are not present when empty
	assert.NotContains(t, jsonStr, `"description"`)
	assert.NotContains(t, jsonStr, `"default"`)
	// required is not omitempty, so it should be present
	assert.Contains(t, jsonStr, `"required"`)
}
