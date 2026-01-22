package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGarbageDetector_DefaultConfig(t *testing.T) {
	detector := NewGarbageDetector(nil)
	require.NotNil(t, detector)
	require.NotNil(t, detector.config)
	assert.NotEmpty(t, detector.config.DebugPatterns)
	assert.NotEmpty(t, detector.config.SecretPatterns)
	assert.NotEmpty(t, detector.config.BuildPatterns)
	assert.NotEmpty(t, detector.config.TempFilePatterns)
}

func TestNewGarbageDetector_CustomConfig(t *testing.T) {
	config := &GarbageConfig{
		DebugPatterns: []string{"*.debug"},
	}
	detector := NewGarbageDetector(config)
	require.NotNil(t, detector)
	assert.Equal(t, []string{"*.debug"}, detector.config.DebugPatterns)
}

func TestGarbageDetector_DetectGarbage_DebugFiles(t *testing.T) {
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		files    []string
		expected int
		category GarbageCategory
	}{
		{
			name:     "debug binary",
			files:    []string{"__debug_bin"},
			expected: 1,
			category: GarbageDebug,
		},
		{
			name:     "debug binary with pid",
			files:    []string{"__debug_bin_atlas"},
			expected: 1,
			category: GarbageDebug,
		},
		{
			name:     "normal go file not detected",
			files:    []string{"main.go"},
			expected: 0,
		},
		{
			name:     "test file not detected as debug",
			files:    []string{"parser_test.go"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage(tt.files)
			assert.Len(t, garbage, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.category, garbage[0].Category)
			}
		})
	}
}

func TestGarbageDetector_DetectGarbage_Secrets(t *testing.T) {
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		files    []string
		expected int
		category GarbageCategory
	}{
		{
			name:     "env file",
			files:    []string{".env"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "env local",
			files:    []string{".env.local"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "env production",
			files:    []string{".env.production"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "credentials file",
			files:    []string{"credentials.json"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "private key",
			files:    []string{"server.key"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "pem file",
			files:    []string{"cert.pem"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "p12 file",
			files:    []string{"keystore.p12"},
			expected: 1,
			category: GarbageSecrets,
		},
		{
			name:     "public key is allowed",
			files:    []string{"server.pub"},
			expected: 0,
		},
		{
			name:     "go file not detected as secret",
			files:    []string{"config.go"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage(tt.files)
			assert.Len(t, garbage, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.category, garbage[0].Category)
			}
		})
	}
}

func TestGarbageDetector_DetectGarbage_BuildArtifacts(t *testing.T) {
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		files    []string
		expected int
		category GarbageCategory
	}{
		{
			name:     "coverage output",
			files:    []string{"coverage.out"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "coverage html",
			files:    []string{"coverage.html"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "coverage reject html",
			files:    []string{"coverage_reject.html"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "coverage test html",
			files:    []string{"coverage_test.html"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "vendor directory file",
			files:    []string{"vendor/github.com/pkg/mod.go"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "node_modules file",
			files:    []string{"node_modules/lodash/index.js"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "DS_Store",
			files:    []string{".DS_Store"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "nested DS_Store",
			files:    []string{"docs/.DS_Store"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "exe file",
			files:    []string{"app.exe"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "dll file",
			files:    []string{"lib.dll"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "so file",
			files:    []string{"lib.so"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "dylib file",
			files:    []string{"lib.dylib"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "dist directory file",
			files:    []string{"dist/bundle.js"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "build directory file",
			files:    []string{"build/output.o"},
			expected: 1,
			category: GarbageBuildArtifact,
		},
		{
			name:     "go source not detected",
			files:    []string{"main.go"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage(tt.files)
			assert.Len(t, garbage, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.category, garbage[0].Category)
			}
		})
	}
}

func TestGarbageDetector_DetectGarbage_TempFiles(t *testing.T) {
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		files    []string
		expected int
		category GarbageCategory
	}{
		{
			name:     "tmp file",
			files:    []string{"data.tmp"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "bak file",
			files:    []string{"config.bak"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "swp file",
			files:    []string{".config.swp"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "tilde backup",
			files:    []string{"file~"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "orig file",
			files:    []string{"config.orig"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "go file with tmp extension",
			files:    []string{"something.go.tmp"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "json file with tmp extension",
			files:    []string{"config.json.tmp"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "go file with bak extension",
			files:    []string{"main.go.bak"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "css file with orig extension",
			files:    []string{"style.css.orig"},
			expected: 1,
			category: GarbageTempFile,
		},
		{
			name:     "go file not detected",
			files:    []string{"temp.go"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage(tt.files)
			assert.Len(t, garbage, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.category, garbage[0].Category)
			}
		})
	}
}

func TestGarbageDetector_DetectGarbage_MixedFiles(t *testing.T) {
	detector := NewGarbageDetector(nil)

	files := []string{
		"main.go",            // clean
		"internal/config.go", // clean
		".env",               // secret
		"coverage.out",       // build artifact
		"data.tmp",           // temp file
		"__debug_bin",        // debug
		"README.md",          // clean
	}

	garbage := detector.DetectGarbage(files)
	assert.Len(t, garbage, 4)

	summary := GarbageSummary(garbage)
	assert.Equal(t, 1, summary[GarbageDebug])
	assert.Equal(t, 1, summary[GarbageSecrets])
	assert.Equal(t, 1, summary[GarbageBuildArtifact])
	assert.Equal(t, 1, summary[GarbageTempFile])
}

func TestGarbageDetector_DetectGarbage_EmptyInput(t *testing.T) {
	detector := NewGarbageDetector(nil)

	garbage := detector.DetectGarbage([]string{})
	assert.Empty(t, garbage)

	garbage = detector.DetectGarbage(nil)
	assert.Empty(t, garbage)
}

func TestHasGarbage(t *testing.T) {
	assert.False(t, HasGarbage(nil))
	assert.False(t, HasGarbage([]GarbageFile{}))
	assert.True(t, HasGarbage([]GarbageFile{{Path: "test", Category: GarbageDebug}}))
}

func TestFilterByCategory(t *testing.T) {
	garbage := []GarbageFile{
		{Path: ".env", Category: GarbageSecrets},
		{Path: "__debug_bin", Category: GarbageDebug},
		{Path: "coverage.out", Category: GarbageBuildArtifact},
		{Path: "credentials.json", Category: GarbageSecrets},
	}

	secrets := FilterByCategory(garbage, GarbageSecrets)
	assert.Len(t, secrets, 2)

	debug := FilterByCategory(garbage, GarbageDebug)
	assert.Len(t, debug, 1)

	temp := FilterByCategory(garbage, GarbageTempFile)
	assert.Empty(t, temp)
}

func TestGarbageSummary(t *testing.T) {
	garbage := []GarbageFile{
		{Path: ".env", Category: GarbageSecrets},
		{Path: "__debug_bin", Category: GarbageDebug},
		{Path: "coverage.out", Category: GarbageBuildArtifact},
		{Path: "credentials.json", Category: GarbageSecrets},
		{Path: "data.tmp", Category: GarbageTempFile},
	}

	summary := GarbageSummary(garbage)
	assert.Equal(t, 2, summary[GarbageSecrets])
	assert.Equal(t, 1, summary[GarbageDebug])
	assert.Equal(t, 1, summary[GarbageBuildArtifact])
	assert.Equal(t, 1, summary[GarbageTempFile])
}

func TestDefaultGarbageConfig(t *testing.T) {
	config := DefaultGarbageConfig()

	// Verify debug patterns
	assert.Contains(t, config.DebugPatterns, "__debug_bin*")

	// Verify secret patterns
	assert.Contains(t, config.SecretPatterns, ".env")
	assert.Contains(t, config.SecretPatterns, "*.key")
	assert.Contains(t, config.SecretPatterns, "*.pem")

	// Verify build patterns
	assert.Contains(t, config.BuildPatterns, "coverage.out")
	assert.Contains(t, config.BuildPatterns, "coverage*.html")
	assert.Contains(t, config.BuildPatterns, "vendor/")
	assert.Contains(t, config.BuildPatterns, ".DS_Store")

	// Verify temp patterns
	assert.Contains(t, config.TempFilePatterns, "*.tmp")
	assert.Contains(t, config.TempFilePatterns, "*.bak")
}

func TestGarbageFile_Fields(t *testing.T) {
	gf := GarbageFile{
		Path:     ".env.local",
		Category: GarbageSecrets,
		Reason:   "matches pattern: .env.*",
	}

	assert.Equal(t, ".env.local", gf.Path)
	assert.Equal(t, GarbageSecrets, gf.Category)
	assert.Contains(t, gf.Reason, "matches pattern")
}

func TestGarbageCategory_Values(t *testing.T) {
	// Verify category string values for consistency
	assert.Equal(t, GarbageDebug, GarbageCategory("debug"))
	assert.Equal(t, GarbageSecrets, GarbageCategory("secrets"))
	assert.Equal(t, GarbageBuildArtifact, GarbageCategory("build_artifact"))
	assert.Equal(t, GarbageTempFile, GarbageCategory("temp_file"))
}

func TestGarbageDetector_DetectGarbage_GoSpecificPatterns(t *testing.T) {
	// Ensure Go-specific files are NOT flagged as garbage
	detector := NewGarbageDetector(nil)

	goFiles := []string{
		"main.go",
		"main_test.go",
		"parser_test.go",
		"internal/config/config.go",
		"internal/config/config_test.go",
		"go.mod",
		"go.sum",
		"Makefile",
		"README.md",
		"LICENSE",
		".gitignore",
		".golangci.yml",
	}

	garbage := detector.DetectGarbage(goFiles)
	assert.Empty(t, garbage, "Go project files should not be detected as garbage")
}

func TestGarbageDetector_DetectGarbage_PathsWithSlashes(t *testing.T) {
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		files    []string
		expected int
	}{
		{
			name:     "nested vendor path",
			files:    []string{"vendor/github.com/stretchr/testify/assert/assertions.go"},
			expected: 1,
		},
		{
			name:     "nested node_modules",
			files:    []string{"frontend/node_modules/react/index.js"},
			expected: 1,
		},
		{
			name:     "nested dist",
			files:    []string{"frontend/dist/bundle.js"},
			expected: 1,
		},
		{
			name:     "nested build",
			files:    []string{"backend/build/main.o"},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage(tt.files)
			assert.Len(t, garbage, tt.expected)
		})
	}
}

func TestGarbageDetector_MultiExtensionFiles(t *testing.T) {
	// This test explicitly verifies that files with multiple extensions
	// are correctly detected as garbage when the final extension matches.
	detector := NewGarbageDetector(nil)

	tests := []struct {
		name     string
		file     string
		category GarbageCategory
	}{
		{"something.go.tmp", "something.go.tmp", GarbageTempFile},
		{"config.yaml.bak", "config.yaml.bak", GarbageTempFile},
		{"script.js.swp", "script.js.swp", GarbageTempFile},
		{"main.rs.orig", "main.rs.orig", GarbageTempFile},
		{"data.json~", "data.json~", GarbageTempFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garbage := detector.DetectGarbage([]string{tt.file})
			require.Len(t, garbage, 1)
			assert.Equal(t, tt.category, garbage[0].Category)
		})
	}
}
