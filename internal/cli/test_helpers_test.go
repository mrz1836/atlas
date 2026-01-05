package cli

// This file contains test utilities and mocks for testing CLI functions.
// These helpers are only available in test files (*_test.go).

// mockFormRunner is a test helper that implements the formRunner interface
// (defined in abandon.go as a package-level interface).
// Use this to mock Charm Huh forms in tests.
type mockFormRunner struct {
	// runErr is the error to return from Run()
	runErr error

	// onRun is an optional callback executed when Run() is called
	// Use this to simulate user input by modifying form values
	onRun func()
}

// Run executes the mock form, optionally calling the onRun callback.
func (m *mockFormRunner) Run() error {
	if m.onRun != nil {
		m.onRun()
	}
	return m.runErr
}

// mockTerminalCheckFunc returns a function that can replace terminalCheck in tests.
// The returned cleanup function should be deferred to restore the original.
//
// Example:
//
//	cleanup := mockTerminalCheckFunc(true)
//	defer cleanup()
func mockTerminalCheckFunc(isTerminal bool) func() {
	original := terminalCheck
	terminalCheck = func() bool { return isTerminal }
	return func() { terminalCheck = original }
}
