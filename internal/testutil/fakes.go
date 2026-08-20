// Package testutil provides testing utilities for ATLAS.
//
// This file contains configurable test doubles (fakes) shared across test
// files, replacing per-package hand-rolled mocks. Each fake favors a
// func-field style so tests can override behavior precisely while relying on
// sensible zero-value defaults for the common case.
package testutil

import (
	"context"

	"github.com/mrz1836/atlas/internal/domain"
)

// FakeAIRunner is a configurable test double implementing ai.Runner.
//
// Behavior precedence in Run:
//  1. If RunFunc is set, it is called and its result returned.
//  2. Otherwise, if Err is set, (nil, Err) is returned.
//  3. Otherwise, (Result, nil) is returned.
//
// Every request passed to Run is appended to Requests so tests can assert on
// what the code under test sent.
type FakeAIRunner struct {
	// Result is returned from Run when RunFunc and Err are both nil.
	Result *domain.AIResult

	// Err is returned from Run (with a nil result) when RunFunc is nil.
	Err error

	// RunFunc, when set, fully overrides Run's behavior.
	RunFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)

	// Requests records every request passed to Run, in order.
	Requests []*domain.AIRequest
}

// Run implements the ai.Runner interface.
func (f *FakeAIRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	f.Requests = append(f.Requests, req)
	if f.RunFunc != nil {
		return f.RunFunc(ctx, req)
	}
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Result, nil
}

// LastRequest returns the most recent request passed to Run, or nil if Run has
// not been called.
func (f *FakeAIRunner) LastRequest() *domain.AIRequest {
	if len(f.Requests) == 0 {
		return nil
	}
	return f.Requests[len(f.Requests)-1]
}
