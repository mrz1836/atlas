package backlog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"pending is valid", StatusPending, true},
		{"promoted is valid", StatusPromoted, true},
		{"dismissed is valid", StatusDismissed, true},
		{"completed is valid", StatusCompleted, true},
		{"empty is invalid", Status(""), false},
		{"unknown is invalid", Status("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

func TestCategory_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		category Category
		want     bool
	}{
		{"bug is valid", CategoryBug, true},
		{"security is valid", CategorySecurity, true},
		{"performance is valid", CategoryPerformance, true},
		{"maintainability is valid", CategoryMaintainability, true},
		{"testing is valid", CategoryTesting, true},
		{"documentation is valid", CategoryDocumentation, true},
		{"empty is invalid", Category(""), false},
		{"unknown is invalid", Category("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.category.IsValid())
		})
	}
}

func TestSeverity_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		severity Severity
		want     bool
	}{
		{"low is valid", SeverityLow, true},
		{"medium is valid", SeverityMedium, true},
		{"high is valid", SeverityHigh, true},
		{"critical is valid", SeverityCritical, true},
		{"empty is invalid", Severity(""), false},
		{"unknown is invalid", Severity("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.severity.IsValid())
		})
	}
}

func TestValidStatuses(t *testing.T) {
	t.Parallel()
	statuses := ValidStatuses()
	assert.Len(t, statuses, 4)
	assert.Contains(t, statuses, StatusPending)
	assert.Contains(t, statuses, StatusPromoted)
	assert.Contains(t, statuses, StatusDismissed)
	assert.Contains(t, statuses, StatusCompleted)
}

func TestValidCategories(t *testing.T) {
	t.Parallel()
	categories := ValidCategories()
	assert.Len(t, categories, 6)
	assert.Contains(t, categories, CategoryBug)
	assert.Contains(t, categories, CategorySecurity)
	assert.Contains(t, categories, CategoryPerformance)
	assert.Contains(t, categories, CategoryMaintainability)
	assert.Contains(t, categories, CategoryTesting)
	assert.Contains(t, categories, CategoryDocumentation)
}

func TestValidSeverities(t *testing.T) {
	t.Parallel()
	severities := ValidSeverities()
	assert.Len(t, severities, 4)
	assert.Contains(t, severities, SeverityLow)
	assert.Contains(t, severities, SeverityMedium)
	assert.Contains(t, severities, SeverityHigh)
	assert.Contains(t, severities, SeverityCritical)
}

func TestDiscovery_ValidateID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// New format tests
		{"valid new format", "item-ABC234", false},
		{"valid new format all chars", "item-HJKMNP", false},
		{"valid new format with numbers", "item-23456X", false},
		// Legacy format tests (backward compatibility)
		{"valid legacy id", "disc-abc123", false},
		{"valid legacy id with numbers", "disc-1a2b3c", false},
		// Invalid tests
		{"empty id", "", true},
		{"missing prefix", "abc123", true},
		{"wrong prefix underscore", "disc_abc123", true},
		{"wrong prefix item underscore", "item_ABC234", true},
		{"new format too short", "item-ABC", true},
		{"new format too long", "item-ABC2345", true},
		{"legacy too short", "disc-abc", true},
		{"legacy too long", "disc-abc1234", true},
		{"new format lowercase", "item-abc234", true},
		{"legacy uppercase", "disc-ABC123", true},
		{"special characters new", "item-ABC!23", true},
		{"special characters legacy", "disc-abc!23", true},
		// Ambiguous character rejection tests (new format)
		{"ambiguous char 0 in new", "item-ABC0DE", true},
		{"ambiguous char O in new", "item-ABCODE", true},
		{"ambiguous char 1 in new", "item-ABC1DE", true},
		{"ambiguous char I in new", "item-ABCIDE", true},
		{"ambiguous char L in new", "item-ABCLDE", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{ID: tt.id}
			err := d.ValidateID()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateGUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		guid    string
		wantErr bool
	}{
		{"empty guid is valid (optional)", "", false},
		{"valid uuid v4", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid uuid v4 lowercase", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", false},
		{"invalid format missing hyphens", "550e8400e29b41d4a716446655440000", true},
		{"invalid version not 4", "550e8400-e29b-31d4-a716-446655440000", true},
		{"invalid variant", "550e8400-e29b-41d4-2716-446655440000", true},
		{"too short", "550e8400-e29b-41d4-a716", true},
		{"too long", "550e8400-e29b-41d4-a716-4466554400001", true},
		{"uppercase is invalid", "550E8400-E29B-41D4-A716-446655440000", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{GUID: tt.guid}
			err := d.ValidateGUID()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_IsLegacy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		id       string
		isLegacy bool
	}{
		{"legacy format", "disc-abc123", true},
		{"new format", "item-ABC234", false},
		{"empty", "", false},
		{"invalid", "unknown-123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{ID: tt.id}
			assert.Equal(t, tt.isLegacy, d.IsLegacy())
		})
	}
}

func TestDiscovery_ValidateTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{"valid title", "Missing error handling", false},
		{"short title", "Bug", false},
		{"max length title", string(make([]byte, MaxTitleLength)), false},
		{"empty title", "", true},
		{"whitespace only", "   ", true},
		{"too long title", string(make([]byte, MaxTitleLength+1)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{Title: tt.title}
			err := d.ValidateTitle()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		category Category
		wantErr  bool
	}{
		{"valid bug", CategoryBug, false},
		{"valid security", CategorySecurity, false},
		{"empty category", Category(""), true},
		{"invalid category", Category("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{Content: Content{Category: tt.category}}
			err := d.ValidateCategory()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateSeverity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		severity Severity
		wantErr  bool
	}{
		{"valid low", SeverityLow, false},
		{"valid high", SeverityHigh, false},
		{"empty severity", Severity(""), true},
		{"invalid severity", Severity("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{Content: Content{Severity: tt.severity}}
			err := d.ValidateSeverity()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		tags    []string
		wantErr bool
	}{
		{"no tags", nil, false},
		{"empty tags", []string{}, false},
		{"valid single tag", []string{"bug"}, false},
		{"valid multiple tags", []string{"bug", "config", "error-handling"}, false},
		{"valid tag with underscore", []string{"error_handling"}, false},
		{"valid tag with numbers", []string{"go123"}, false},
		{"max tags", make([]string, MaxTags), false}, // MaxTags empty strings will fail
		{"empty tag", []string{""}, true},
		{"too many tags", make([]string, MaxTags+1), true},
		{"tag too long", []string{string(make([]byte, MaxTagLength+1))}, true},
		{"tag starts with hyphen", []string{"-tag"}, true},
		{"tag with uppercase", []string{"Tag"}, true},
		{"tag with spaces", []string{"my tag"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fill empty tags with valid values for "max tags" test
			if tt.name == "max tags" {
				for i := range tt.tags {
					tt.tags[i] = "tag" + string(rune('a'+i))
				}
			}
			d := &Discovery{Content: Content{Tags: tt.tags}}
			err := d.ValidateTags()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateLocation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		location *Location
		wantErr  bool
	}{
		{"nil location", nil, false},
		{"file only", &Location{File: "main.go"}, false},
		{"file and line", &Location{File: "main.go", Line: 10}, false},
		{"line without file", &Location{Line: 10}, true},
		{"negative line", &Location{File: "main.go", Line: -1}, true},
		{"zero line is ok", &Location{File: "main.go", Line: 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{Location: tt.location}
			err := d.ValidateLocation()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_ValidateStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  Status
		wantErr bool
	}{
		{"pending", StatusPending, false},
		{"promoted", StatusPromoted, false},
		{"dismissed", StatusDismissed, false},
		{"completed", StatusCompleted, false},
		{"empty", Status(""), true},
		{"invalid", Status("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &Discovery{Status: tt.status}
			err := d.ValidateStatus()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscovery_Validate(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	validDiscovery := func() *Discovery {
		return &Discovery{
			SchemaVersion: SchemaVersion,
			ID:            "item-ABC234",
			GUID:          "550e8400-e29b-41d4-a716-446655440000",
			Title:         "Test discovery",
			Status:        StatusPending,
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
			Context: Context{
				DiscoveredAt: now,
				DiscoveredBy: "human:tester",
			},
		}
	}

	t.Run("valid pending discovery", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		assert.NoError(t, d.Validate())
	})

	t.Run("valid promoted discovery with task ID", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusPromoted
		d.Lifecycle.PromotedToTask = "task-001"
		assert.NoError(t, d.Validate())
	})

	t.Run("promoted without task ID", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusPromoted
		assert.Error(t, d.Validate())
	})

	t.Run("valid dismissed discovery with reason", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusDismissed
		d.Lifecycle.DismissedReason = "duplicate"
		assert.NoError(t, d.Validate())
	})

	t.Run("dismissed without reason", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusDismissed
		assert.Error(t, d.Validate())
	})

	t.Run("valid completed discovery with task ID and timestamp", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusCompleted
		d.Lifecycle.PromotedToTask = "task-001"
		d.Lifecycle.CompletedAt = now
		assert.NoError(t, d.Validate())
	})

	t.Run("completed without task ID", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusCompleted
		d.Lifecycle.CompletedAt = now
		assert.Error(t, d.Validate())
	})

	t.Run("completed without completed timestamp", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Status = StatusCompleted
		d.Lifecycle.PromotedToTask = "task-001"
		// CompletedAt is zero time
		assert.Error(t, d.Validate())
	})

	t.Run("missing discovered_by", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Context.DiscoveredBy = ""
		assert.Error(t, d.Validate())
	})

	t.Run("missing discovered_at", func(t *testing.T) {
		t.Parallel()
		d := validDiscovery()
		d.Context.DiscoveredAt = time.Time{}
		assert.Error(t, d.Validate())
	})
}

func TestFilter_Match(t *testing.T) {
	t.Parallel()
	pending := StatusPending
	promoted := StatusPromoted
	completed := StatusCompleted
	bug := CategoryBug
	security := CategorySecurity
	high := SeverityHigh
	low := SeverityLow

	discovery := &Discovery{
		Status: StatusPending,
		Content: Content{
			Category: CategoryBug,
			Severity: SeverityHigh,
		},
	}

	tests := []struct {
		name   string
		filter Filter
		want   bool
	}{
		{"empty filter matches all", Filter{}, true},
		{"matching status", Filter{Status: &pending}, true},
		{"non-matching status", Filter{Status: &promoted}, false},
		{"non-matching completed status", Filter{Status: &completed}, false},
		{"matching category", Filter{Category: &bug}, true},
		{"non-matching category", Filter{Category: &security}, false},
		{"matching severity", Filter{Severity: &high}, true},
		{"non-matching severity", Filter{Severity: &low}, false},
		{"all matching", Filter{Status: &pending, Category: &bug, Severity: &high}, true},
		{"one non-matching", Filter{Status: &pending, Category: &security}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.filter.Match(discovery)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilter_Match_CompletedDiscovery(t *testing.T) {
	t.Parallel()
	pending := StatusPending
	completed := StatusCompleted
	bug := CategoryBug

	discovery := &Discovery{
		Status: StatusCompleted,
		Content: Content{
			Category: CategoryBug,
			Severity: SeverityHigh,
		},
		Lifecycle: Lifecycle{
			PromotedToTask: "task-123",
			CompletedAt:    time.Now().UTC(),
		},
	}

	t.Run("completed status matches completed filter", func(t *testing.T) {
		t.Parallel()
		filter := Filter{Status: &completed}
		assert.True(t, filter.Match(discovery))
	})

	t.Run("completed status does not match pending filter", func(t *testing.T) {
		t.Parallel()
		filter := Filter{Status: &pending}
		assert.False(t, filter.Match(discovery))
	})

	t.Run("completed status with category filter", func(t *testing.T) {
		t.Parallel()
		filter := Filter{Status: &completed, Category: &bug}
		assert.True(t, filter.Match(discovery))
	})
}

func TestGenerateID(t *testing.T) {
	t.Parallel()
	t.Run("generates valid GUID and ID", func(t *testing.T) {
		t.Parallel()
		guid, id, err := GenerateID()
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, guid)
		assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, id)
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		t.Parallel()
		ids := make(map[string]bool)
		guids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			guid, id, err := GenerateID()
			require.NoError(t, err)
			assert.False(t, ids[id], "duplicate ID: %s", id)
			assert.False(t, guids[guid], "duplicate GUID: %s", guid)
			ids[id] = true
			guids[guid] = true
		}
	})

	t.Run("DeriveShortID is deterministic", func(t *testing.T) {
		t.Parallel()
		guid := "550e8400-e29b-41d4-a716-446655440000"
		id1, err := DeriveShortID(guid)
		require.NoError(t, err)
		id2, err := DeriveShortID(guid)
		require.NoError(t, err)
		assert.Equal(t, id1, id2)
		assert.Regexp(t, `^item-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$`, id1)
	})

	t.Run("DeriveShortID fails on invalid GUID", func(t *testing.T) {
		t.Parallel()
		_, err := DeriveShortID("not-a-guid")
		assert.Error(t, err)
	})

	t.Run("GenerateGUID creates valid UUID v4", func(t *testing.T) {
		t.Parallel()
		guid, err := GenerateGUID()
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, guid)
	})

	t.Run("new ID excludes ambiguous characters", func(t *testing.T) {
		t.Parallel()
		ambiguous := "01OIL"
		for i := 0; i < 100; i++ {
			_, id, err := GenerateID()
			require.NoError(t, err)
			suffix := id[5:] // Skip "item-" prefix
			for _, c := range ambiguous {
				assert.NotContains(t, suffix, string(c), "ID contains ambiguous character: %s", id)
			}
		}
	})

	t.Run("ID uses uppercase unambiguous charset", func(t *testing.T) {
		t.Parallel()
		const idChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
		for i := 0; i < 100; i++ {
			_, id, err := GenerateID()
			require.NoError(t, err)
			suffix := id[5:] // Skip "item-" prefix
			for _, c := range suffix {
				assert.Contains(t, idChars, string(c), "ID contains invalid character: %c in %s", c, id)
			}
		}
	})
}

func TestGenerateLegacyID(t *testing.T) {
	t.Parallel()
	t.Run("generates valid legacy ID", func(t *testing.T) {
		t.Parallel()
		id, err := GenerateLegacyID()
		require.NoError(t, err)
		assert.Regexp(t, `^disc-[a-z0-9]{6}$`, id)
	})

	t.Run("uniform character distribution", func(t *testing.T) {
		t.Parallel()
		// Generate many IDs and count character frequency
		const legacyChars = "abcdefghijklmnopqrstuvwxyz0123456789"
		const numIDs = 10000
		const charsPerID = 6

		charCounts := make(map[byte]int)
		for _, c := range []byte(legacyChars) {
			charCounts[c] = 0
		}

		for i := 0; i < numIDs; i++ {
			id, err := GenerateLegacyID()
			require.NoError(t, err)
			// Count characters in suffix (skip "disc-" prefix)
			suffix := id[5:]
			for j := 0; j < len(suffix); j++ {
				charCounts[suffix[j]]++
			}
		}

		// Each character should appear roughly equal times
		// Expected: (numIDs * charsPerID) / 36 = 1666.67
		// Allow 20% deviation for statistical variance
		totalChars := numIDs * charsPerID
		expectedPerChar := float64(totalChars) / float64(len(legacyChars))
		tolerance := expectedPerChar * 0.20

		for c, count := range charCounts {
			deviation := float64(count) - expectedPerChar
			if deviation < 0 {
				deviation = -deviation
			}
			assert.LessOrEqual(t, deviation, tolerance,
				"character %c has count %d, expected ~%.0f (deviation %.0f > tolerance %.0f)",
				c, count, expectedPerChar, deviation, tolerance)
		}
	})
}

func TestIsLegacyID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		id       string
		isLegacy bool
	}{
		{"legacy format", "disc-abc123", true},
		{"new format", "item-ABC234", false},
		{"empty", "", false},
		{"invalid", "unknown-123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.isLegacy, IsLegacyID(tt.id))
		})
	}
}

func TestIsNewID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		id    string
		isNew bool
	}{
		{"new format", "item-ABC234", true},
		{"legacy format", "disc-abc123", false},
		{"empty", "", false},
		{"invalid", "unknown-123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.isNew, IsNewID(tt.id))
		})
	}
}
