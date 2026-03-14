// Package contracts provides shared interfaces and utilities to avoid circular dependencies.
// This package can be imported by any internal package and should have minimal dependencies.
//
// Import rules:
//   - CAN import: internal/constants, standard library
//   - MUST NOT import: any other internal packages
package contracts

import "github.com/mrz1836/atlas/internal/constants"

// attentionStatuses defines task statuses that require user attention.
// This is the single source of truth for attention status determination.
//
//nolint:gochecknoglobals // Read-only lookup table for attention status checks
var attentionStatuses = map[constants.TaskStatus]bool{
	constants.TaskStatusValidationFailed: true,
	constants.TaskStatusAwaitingApproval: true,
	constants.TaskStatusGHFailed:         true,
	constants.TaskStatusCIFailed:         true,
	constants.TaskStatusCITimeout:        true,
}

// IsAttentionStatus returns true if the status requires user attention.
// These statuses should be highlighted, trigger notifications, and
// be sorted to the top of status lists.
func IsAttentionStatus(status constants.TaskStatus) bool {
	return attentionStatuses[status]
}
