package steps

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// MockArtifactSaver provides a thread-safe mock for testing artifact saving.
// Use in all step executor tests for consistency.
//
// Features:
//   - Thread-safe for concurrent tests
//   - Tracks all saved artifacts with content
//   - Simulates version numbering for versioned artifacts
//   - Configurable error injection for testing error paths
//   - Call history tracking for assertions
//
// Example usage:
//
//	saver := NewMockArtifactSaver()
//	executor := NewGitExecutor(workDir, WithGitArtifactSaver(saver))
//	// ... run test ...
//	saver.AssertSaved(t, "commit_step/commit-result.json")
type MockArtifactSaver struct {
	mu sync.Mutex

	// SavedArtifacts stores all saved artifacts by filename.
	// Key is the full artifact path (e.g., "commit_step/commit-result.json").
	SavedArtifacts map[string][]byte

	// VersionCounters tracks version numbers per base name.
	// Used by SaveVersionedArtifact to simulate version numbering.
	VersionCounters map[string]int

	// SaveError when set, causes all save operations to return this error.
	// Useful for testing error handling paths.
	SaveError error

	// Calls records all save method invocations for verification.
	Calls []MockArtifactCall
}

// MockArtifactCall records a single call to a save method.
type MockArtifactCall struct {
	Method      string // "SaveArtifact" or "SaveVersionedArtifact"
	WorkspaceID string
	TaskID      string
	Filename    string
	DataSize    int
}

// NewMockArtifactSaver creates a new mock artifact saver ready for use.
func NewMockArtifactSaver() *MockArtifactSaver {
	return &MockArtifactSaver{
		SavedArtifacts:  make(map[string][]byte),
		VersionCounters: make(map[string]int),
		Calls:           make([]MockArtifactCall, 0),
	}
}

// SaveArtifact implements ArtifactSaver.SaveArtifact.
func (m *MockArtifactSaver) SaveArtifact(_ context.Context,
	workspaceID, taskID, filename string, data []byte,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, MockArtifactCall{
		Method:      "SaveArtifact",
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		Filename:    filename,
		DataSize:    len(data),
	})

	if m.SaveError != nil {
		return m.SaveError
	}

	// Make a copy to avoid issues with reused buffers
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.SavedArtifacts[filename] = dataCopy

	return nil
}

// SaveVersionedArtifact implements ArtifactSaver.SaveVersionedArtifact.
func (m *MockArtifactSaver) SaveVersionedArtifact(_ context.Context,
	workspaceID, taskID, baseName string, data []byte,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, MockArtifactCall{
		Method:      "SaveVersionedArtifact",
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		Filename:    baseName,
		DataSize:    len(data),
	})

	if m.SaveError != nil {
		return "", m.SaveError
	}

	// Increment version counter and generate versioned filename
	m.VersionCounters[baseName]++
	version := m.VersionCounters[baseName]

	// Insert version number before extension
	// e.g., "validation.json" -> "validation.1.json"
	// e.g., "sdd/spec.md" -> "sdd/spec.1.md"
	var filename string
	lastDot := strings.LastIndex(baseName, ".")
	if lastDot > 0 {
		filename = fmt.Sprintf("%s.%d%s", baseName[:lastDot], version, baseName[lastDot:])
	} else {
		filename = fmt.Sprintf("%s.%d", baseName, version)
	}

	// Make a copy to avoid issues with reused buffers
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.SavedArtifacts[filename] = dataCopy

	return filename, nil
}

// GetArtifact retrieves a saved artifact by filename.
// Returns the artifact data and true if found, nil and false otherwise.
func (m *MockArtifactSaver) GetArtifact(filename string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.SavedArtifacts[filename]
	return data, ok
}

// GetArtifactString retrieves a saved artifact as a string.
// Convenience method for text artifacts.
func (m *MockArtifactSaver) GetArtifactString(filename string) (string, bool) {
	data, ok := m.GetArtifact(filename)
	if !ok {
		return "", false
	}
	return string(data), true
}

// AssertSaved verifies an artifact was saved with the given filename.
// Fails the test if the artifact was not saved.
func (m *MockArtifactSaver) AssertSaved(t *testing.T, filename string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.SavedArtifacts[filename]; !ok {
		t.Errorf("expected artifact %q to be saved, but it wasn't", filename)
		t.Logf("saved artifacts: %v", m.artifactNames())
	}
}

// AssertNotSaved verifies an artifact was NOT saved with the given filename.
// Fails the test if the artifact was saved.
func (m *MockArtifactSaver) AssertNotSaved(t *testing.T, filename string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.SavedArtifacts[filename]; ok {
		t.Errorf("expected artifact %q to NOT be saved, but it was", filename)
	}
}

// AssertSavedContains verifies an artifact was saved and contains the given substring.
func (m *MockArtifactSaver) AssertSavedContains(t *testing.T, filename, contains string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.SavedArtifacts[filename]
	if !ok {
		t.Errorf("expected artifact %q to be saved, but it wasn't", filename)
		t.Logf("saved artifacts: %v", m.artifactNames())
		return
	}

	if !strings.Contains(string(data), contains) {
		t.Errorf("artifact %q does not contain %q", filename, contains)
		t.Logf("actual content: %s", string(data))
	}
}

// AssertCallCount verifies the total number of save calls.
func (m *MockArtifactSaver) AssertCallCount(t *testing.T, expected int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Calls) != expected {
		t.Errorf("expected %d save calls, got %d", expected, len(m.Calls))
		for i, call := range m.Calls {
			t.Logf("  call %d: %s(%s)", i, call.Method, call.Filename)
		}
	}
}

// AssertVersionCount verifies the version count for a base name.
func (m *MockArtifactSaver) AssertVersionCount(t *testing.T, baseName string, expected int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	count := m.VersionCounters[baseName]
	if count != expected {
		t.Errorf("expected version count %d for %q, got %d", expected, baseName, count)
	}
}

// Reset clears all saved artifacts, calls, and counters.
// Useful for reusing a mock across multiple test cases.
func (m *MockArtifactSaver) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SavedArtifacts = make(map[string][]byte)
	m.VersionCounters = make(map[string]int)
	m.Calls = make([]MockArtifactCall, 0)
	m.SaveError = nil
}

// SetError configures the mock to return an error on all save operations.
// Pass nil to clear the error.
func (m *MockArtifactSaver) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SaveError = err
}

// ArtifactCount returns the number of saved artifacts.
func (m *MockArtifactSaver) ArtifactCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.SavedArtifacts)
}

// artifactNames returns a slice of all saved artifact names.
// Must be called with lock held.
func (m *MockArtifactSaver) artifactNames() []string {
	names := make([]string, 0, len(m.SavedArtifacts))
	for name := range m.SavedArtifacts {
		names = append(names, name)
	}
	return names
}

// Compile-time interface check.
var _ ArtifactSaver = (*MockArtifactSaver)(nil)
