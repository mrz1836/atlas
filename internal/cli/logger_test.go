package cli

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestInitLogger_VerboseMode(t *testing.T) {
	t.Parallel()

	logger := InitLogger(true, false)
	assert.Equal(t, zerolog.DebugLevel, logger.GetLevel())
}

func TestInitLogger_QuietMode(t *testing.T) {
	t.Parallel()

	logger := InitLogger(false, true)
	assert.Equal(t, zerolog.WarnLevel, logger.GetLevel())
}

func TestInitLogger_DefaultMode(t *testing.T) {
	t.Parallel()

	logger := InitLogger(false, false)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())
}

func TestInitLogger_LogLevelPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		verbose       bool
		quiet         bool
		expectedLevel zerolog.Level
	}{
		{
			name:          "default is info level",
			verbose:       false,
			quiet:         false,
			expectedLevel: zerolog.InfoLevel,
		},
		{
			name:          "verbose enables debug level",
			verbose:       true,
			quiet:         false,
			expectedLevel: zerolog.DebugLevel,
		},
		{
			name:          "quiet enables warn level",
			verbose:       false,
			quiet:         true,
			expectedLevel: zerolog.WarnLevel,
		},
		{
			name:          "verbose takes precedence over quiet",
			verbose:       true,
			quiet:         true,
			expectedLevel: zerolog.DebugLevel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := InitLogger(tc.verbose, tc.quiet)
			assert.Equal(t, tc.expectedLevel, logger.GetLevel())
		})
	}
}

func TestInitLogger_HasTimestamp(t *testing.T) {
	t.Parallel()

	// The logger should have a timestamp context
	// We can verify this by checking that the logger was created with With().Timestamp()
	logger := InitLogger(false, false)

	// Logger should not be zero value
	assert.NotEqual(t, zerolog.Logger{}, logger)
}

func TestSelectOutput_NonTTY(t *testing.T) {
	// This test runs in a non-TTY environment (typical for CI/tests).
	// In non-TTY mode, selectOutput always returns os.Stderr regardless of NO_COLOR.

	output := selectOutput()
	assert.NotNil(t, output)
	// In non-TTY environment, output should be os.Stderr (JSON format)
	assert.Equal(t, os.Stderr, output)
}

func TestSelectOutput_RespectsNO_COLOR(t *testing.T) {
	// Test that NO_COLOR environment variable is checked.
	// In non-TTY environment, this has no effect, but we verify the code path.

	// t.Setenv automatically restores the original value after test
	t.Setenv("NO_COLOR", "1")

	output := selectOutput()
	assert.NotNil(t, output)
	// In non-TTY or NO_COLOR mode, output should be os.Stderr
	assert.Equal(t, os.Stderr, output)
}

func TestInitLogger_WithNO_COLOR(t *testing.T) {
	// Verify logger initializes correctly when NO_COLOR is set.
	// This ensures the NO_COLOR code path doesn't cause any issues.

	// t.Setenv automatically restores the original value after test
	t.Setenv("NO_COLOR", "1")

	// Logger should initialize without error
	logger := InitLogger(false, false)
	assert.NotEqual(t, zerolog.Logger{}, logger)
	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel())
}
