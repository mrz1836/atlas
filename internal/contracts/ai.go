// Package contracts defines shared interfaces to avoid circular imports.
package contracts

import (
	"context"

	"github.com/mrz1836/atlas/internal/domain"
)

// AIRunner defines the interface for AI execution.
// Implemented by ai.Runner and used by validation and backlog packages.
type AIRunner interface {
	Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}
