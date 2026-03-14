package logging

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper functions construct fake secret strings at runtime to avoid
// gitleaks false positives. These use obvious test/example patterns.
func fakeAnthropicKey() string  { return "sk-" + "ant-api03-test-key-do-not-use" }
func fakeAnthropicKey2() string { return "sk-" + "ant-api03-example-value-only" }
func fakeGitHubPAT() string     { return "ghp_" + "xxxxxxxxxxTESTONLYxxxxxxxxxx" }
func fakeGitHubOAuth() string   { return "gho_" + "xxxxxxxxxxTESTONLYxxxxxxxxxx" }
func fakeGitHubUser() string    { return "ghu_" + "xxxxxxxxxxTESTONLYxxxxxxxxxx" }
func fakeGitHubApp() string     { return "ghs_" + "xxxxxxxxxxTESTONLYxxxxxxxxxx" }
func fakeGitHubRefresh() string { return "ghr_" + "xxxxxxxxxxTESTONLYxxxxxxxxxx" }
func fakeOpenAIKey() string     { return "sk-" + "TESTONLYxxxxxxxxxxxxxxxxxxxx1234" }
func fakeGenericAPIKey() string { return "TESTONLY" + "apikey12345678" }
func fakeBearerToken() string   { return "TESTONLYbearer" + "token1234567890" }
func fakePassword() string      { return "testonly" + "password123" }
func fakeSecret() string        { return "testonly" + "secretvalue456" }
func fakeCredential() string    { return "testonly" + "credential789" }

func TestContainsSensitiveData_AnthropicAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "anthropic api key",
			input:    "using key " + fakeAnthropicKey(),
			expected: true,
		},
		{
			name:     "anthropic key in config",
			input:    "ANTHROPIC_API_KEY=" + fakeAnthropicKey2(),
			expected: true,
		},
		{
			name:     "no api key",
			input:    "just a normal message",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, ContainsSensitiveData(tc.input))
		})
	}
}

func TestContainsSensitiveData_GitHubTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "github personal access token",
			input:    "token: " + fakeGitHubPAT(),
			expected: true,
		},
		{
			name:     "github oauth token",
			input:    fakeGitHubOAuth(),
			expected: true,
		},
		{
			name:     "github user token",
			input:    fakeGitHubUser(),
			expected: true,
		},
		{
			name:     "github app token",
			input:    fakeGitHubApp(),
			expected: true,
		},
		{
			name:     "github refresh token",
			input:    fakeGitHubRefresh(),
			expected: true,
		},
		{
			name:     "github url without token",
			input:    "https://github.com/user/repo",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, ContainsSensitiveData(tc.input))
		})
	}
}

func TestContainsSensitiveData_OpenAIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "openai api key",
			input:    "key: " + fakeOpenAIKey(),
			expected: true,
		},
		{
			name:     "short sk prefix not matched",
			input:    "sk-short", // too short
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, ContainsSensitiveData(tc.input))
		})
	}
}

func TestContainsSensitiveData_GenericPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "api_key assignment",
			input:    `api_key = "` + fakeGenericAPIKey() + `"`,
			expected: true,
		},
		{
			name:     "apikey colon",
			input:    `apikey: ` + fakeGenericAPIKey(),
			expected: true,
		},
		{
			name:     "bearer token",
			input:    `Authorization: Bearer ` + fakeBearerToken(),
			expected: true,
		},
		{
			name:     "password assignment",
			input:    `password = "` + fakePassword() + `"`,
			expected: true,
		},
		{
			name:     "secret in config",
			input:    `secret: ` + fakeSecret(),
			expected: true,
		},
		{
			name:     "credential value",
			input:    `credential = "` + fakeCredential() + `"`,
			expected: true,
		},
		{ //nolint:gosec // G101: test data for filter verification, not a real credential
			name:     "ssh private key header",
			input:    `-----BEGIN RSA PRIVATE KEY-----`,
			expected: true,
		},
		{
			name:     "normal message",
			input:    `loading configuration from file`,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, ContainsSensitiveData(tc.input))
		})
	}
}

func TestFilterSensitiveValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "anthropic key redacted",
			input:    "using key " + fakeAnthropicKey(),
			expected: "using key [REDACTED]",
		},
		{
			name:     "github token redacted",
			input:    "token: " + fakeGitHubPAT(),
			expected: "token: [REDACTED]",
		},
		{
			name:     "multiple sensitive values",
			input:    "key1: " + fakeAnthropicKey() + ", key2: " + fakeGitHubPAT(),
			expected: "key1: [REDACTED], key2: [REDACTED]",
		},
		{
			name:     "no sensitive data unchanged",
			input:    "normal log message without secrets",
			expected: "normal log message without secrets",
		},
		{
			name:     "password assignment redacted",
			input:    `config: password = "` + fakePassword() + `"`,
			expected: `config: [REDACTED]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := FilterSensitiveValue(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsSensitiveFieldName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fieldName   string
		isSensitive bool
	}{
		// Exact matches
		{"api_key", "api_key", true},
		{"API_KEY uppercase", "API_KEY", true},
		{"apikey", "apikey", true},
		{"password", "password", true},
		{"secret", "secret", true},
		{"access_token", "access_token", true},
		{"refresh_token", "refresh_token", true},
		{"private_key", "private_key", true},
		{"authorization", "authorization", true},
		{"anthropic_api_key", "anthropic_api_key", true},
		{"github_token", "github_token", true},

		// Prefix patterns (sensitive_*)
		{"user_api_key field", "user_api_key", true},
		{"password_hash", "password_hash", true},
		{"secret-value with dash", "secret-value", true},

		// Suffix patterns (*_sensitive)
		{"my_secret_value", "my_secret_value", true},
		{"db_password", "db_password", true},
		{"user-password with dash", "user-password", true},

		// Infix patterns (*_sensitive_*)
		{"my_password_field", "my_password_field", true},
		{"app-secret-key", "app-secret-key", true},

		// Mixed separator patterns
		{"my_password-field", "my_password-field", true},
		{"my-password_field", "my-password_field", true},

		// Non-sensitive fields
		{"normal field", "workspace_name", false},
		{"task_id", "task_id", false},
		{"status", "status", false},
		{"duration_ms", "duration_ms", false},
		{"secretariat - partial word match should not trigger", "secretariat", false},
		{"passwords - plural not exact", "passwords", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.isSensitive, IsSensitiveFieldName(tc.fieldName))
		})
	}
}

func TestMatchesSensitivePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		sensitive string
		expected  bool
	}{
		// Exact match
		{"exact match", "password", "password", true},
		{"no exact match", "passwords", "password", false},

		// Prefix: sensitive_*
		{"prefix underscore", "password_hash", "password", true},
		{"prefix dash", "password-hash", "password", true},

		// Suffix: *_sensitive
		{"suffix underscore", "db_password", "password", true},
		{"suffix dash", "db-password", "password", true},

		// Neither prefix nor suffix (partial word)
		{"not prefix or suffix - partial word", "mypassword_hash", "password", false},
		{"not suffix - different word", "password_hash", "hash", true}, // hash is suffix of password_hash

		// Infix: *_sensitive_*
		{"infix underscore", "my_password_field", "password", true},
		{"infix dash", "my-password-field", "password", true},

		// Mixed separators
		{"mixed underscore-dash", "my_password-field", "password", true},
		{"mixed dash-underscore", "my-password_field", "password", true},

		// Edge cases
		{"empty name", "", "password", false},
		{"empty sensitive", "password", "", false},
		{"partial match no boundary", "mypassword", "password", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, matchesSensitivePattern(tc.fieldName, tc.sensitive))
		})
	}
}

func TestRedactIfSensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		value     string
		expected  string
	}{
		{
			name:      "sensitive field name redacted",
			fieldName: "api_key",
			value:     "my-test-api-key-value",
			expected:  RedactedValue,
		},
		{
			name:      "sensitive field password redacted",
			fieldName: "password",
			value:     "testpassword",
			expected:  RedactedValue,
		},
		{
			name:      "normal field unchanged",
			fieldName: "workspace_name",
			value:     "my-workspace",
			expected:  "my-workspace",
		},
		{
			name:      "normal field with sensitive value pattern",
			fieldName: "config_output",
			value:     "key: " + fakeAnthropicKey(),
			expected:  "key: [REDACTED]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := RedactIfSensitive(tc.fieldName, tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSafeValue(t *testing.T) {
	t.Parallel()

	// SafeValue is an alias for RedactIfSensitive
	result := SafeValue("api_key", "secret-value")
	assert.Equal(t, RedactedValue, result)

	result = SafeValue("workspace", "my-workspace")
	assert.Equal(t, "my-workspace", result)
}

func TestSensitiveDataHook_Run(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	hook := NewSensitiveDataHook()
	logger := zerolog.New(&buf).Hook(hook)

	// Log message with sensitive data - hook adds flag to indicate detection.
	// The hook cannot modify the message (zerolog limitation).
	// Actual redaction is done by FilteringWriter wrapping the file output.
	logger.Info().Msg("using key " + fakeAnthropicKey())

	output := buf.String()
	assert.Contains(t, output, "contains_filtered_data")
	// The raw output still contains the key because the hook can only flag, not redact.
	// FilteringWriter handles actual redaction at the io.Writer level.
}

func TestSensitiveDataHook_NoSensitiveData(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	hook := NewSensitiveDataHook()
	logger := zerolog.New(&buf).Hook(hook)

	// Log message without sensitive data - no flag added
	logger.Info().Msg("normal operation completed")

	output := buf.String()
	assert.NotContains(t, output, "contains_filtered_data")
}

func TestNewSensitiveDataHook(t *testing.T) {
	t.Parallel()

	hook := NewSensitiveDataHook()
	assert.NotNil(t, hook)
}

func TestContainsSensitiveData_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "whitespace only",
			input:    "   \t\n  ",
			expected: false,
		},
		{
			name:     "sk prefix alone",
			input:    "sk-",
			expected: false,
		},
		{
			name:     "gh prefix alone",
			input:    "ghp_",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, ContainsSensitiveData(tc.input))
		})
	}
}

func TestNewFilteringWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	fw := NewFilteringWriter(&buf)
	assert.NotNil(t, fw)
}

func TestFilteringWriter_RedactsSensitiveData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		shouldContain  []string
		shouldNotMatch []string // patterns that should NOT appear
	}{
		{
			name:           "anthropic api key redacted",
			input:          `{"level":"info","event":"using key ` + fakeAnthropicKey() + `"}`,
			shouldContain:  []string{`"level":"info"`, `[REDACTED]`},
			shouldNotMatch: []string{"sk-" + "ant-api"},
		},
		{
			name:           "github token redacted",
			input:          `{"level":"info","token":"` + fakeGitHubPAT() + `"}`,
			shouldContain:  []string{`"level":"info"`, `[REDACTED]`},
			shouldNotMatch: []string{"ghp_" + "xxxx"},
		},
		{
			name:           "password field redacted",
			input:          `{"level":"info","config":"password: ` + fakePassword() + `"}`,
			shouldContain:  []string{`"level":"info"`, `[REDACTED]`},
			shouldNotMatch: []string{fakePassword()},
		},
		{
			name:          "normal message unchanged",
			input:         `{"level":"info","event":"task completed successfully"}`,
			shouldContain: []string{`"level":"info"`, `task completed successfully`},
		},
		{
			name:           "multiple sensitive values redacted",
			input:          `{"key1":"` + fakeAnthropicKey() + `","key2":"` + fakeGitHubPAT() + `"}`,
			shouldContain:  []string{`[REDACTED]`},
			shouldNotMatch: []string{"sk-" + "ant-api", "ghp_" + "xxxx"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			fw := NewFilteringWriter(&buf)

			n, err := fw.Write([]byte(tc.input))
			require.NoError(t, err)
			assert.Equal(t, len(tc.input), n, "should return original length")

			output := buf.String()

			for _, s := range tc.shouldContain {
				assert.Contains(t, output, s)
			}
			for _, s := range tc.shouldNotMatch {
				assert.NotContains(t, output, s, "sensitive data should be redacted")
			}
		})
	}
}

func TestFilteringWriter_WithZerolog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	fw := NewFilteringWriter(&buf)

	// Create logger that writes through filtering writer
	logger := zerolog.New(fw)

	// Log a message containing sensitive data
	logger.Info().Msg("connecting with key " + fakeAnthropicKey())

	output := buf.String()

	// Verify sensitive data is redacted
	assert.NotContains(t, output, "sk-"+"ant-api03", "API key should be redacted")
	assert.Contains(t, output, "[REDACTED]", "should contain redaction marker")
	assert.Contains(t, output, "connecting with key", "non-sensitive part preserved")
}

func TestFilteringWriter_PreservesWriteLength(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	fw := NewFilteringWriter(&buf)

	input := "test message with " + fakeAnthropicKey() + " in it"
	n, err := fw.Write([]byte(input))

	require.NoError(t, err)
	// Should return original length even though output is different
	assert.Equal(t, len(input), n)
}

func TestContainsWordBoundary(t *testing.T) {
	t.Parallel()

	seps := []string{"_", "-"}

	tests := []struct {
		name     string
		input    string
		word     string
		expected bool
	}{
		// Prefix patterns
		{"prefix underscore", "password_hash", "password", true},
		{"prefix dash", "password-hash", "password", true},

		// Suffix patterns
		{"suffix underscore", "db_password", "password", true},
		{"suffix dash", "db-password", "password", true},

		// Infix patterns
		{"infix underscore", "my_password_field", "password", true},
		{"infix dash", "my-password-field", "password", true},

		// No boundary
		{"no boundary - partial", "mypassword", "password", false},
		{"no boundary - exact", "password", "password", false}, // exact match is not a boundary
		{"no boundary - suffix only", "password_", "password", true},

		// Edge cases
		{"empty name", "", "password", false},
		{"empty word", "password", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, containsWordBoundary(tc.input, tc.word, seps))
		})
	}
}

// BenchmarkIsSensitiveFieldName benchmarks the O(1) optimized lookup.
func BenchmarkIsSensitiveFieldName(b *testing.B) {
	testCases := []string{
		"api_key",          // exact match (fast path)
		"password",         // exact match (fast path)
		"user_api_key",     // word boundary (slow path)
		"workspace_name",   // non-sensitive (full scan)
		"task_id",          // non-sensitive (full scan)
		"my_password_hash", // word boundary (slow path)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			IsSensitiveFieldName(tc)
		}
	}
}
