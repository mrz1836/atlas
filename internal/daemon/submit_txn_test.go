package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubmitTxn_CommitSkipsRollback(t *testing.T) {
	t.Parallel()

	ran := false
	var tx submitTxn
	tx.onRollback(func() { ran = true })
	tx.commit()

	tx.rollbackIfNeeded()

	assert.False(t, ran, "committed transaction must not run rollbacks")
}

func TestSubmitTxn_NoCommitRunsRollbacksInReverse(t *testing.T) {
	t.Parallel()

	var order []int
	var tx submitTxn
	tx.onRollback(func() { order = append(order, 1) })
	tx.onRollback(func() { order = append(order, 2) })
	tx.onRollback(func() { order = append(order, 3) })

	// No commit — deferred-style invocation runs every compensation, LIFO.
	tx.rollbackIfNeeded()

	assert.Equal(t, []int{3, 2, 1}, order,
		"uncommitted transaction must run all rollbacks in reverse registration order")
}

func TestSubmitTxn_RunsEveryRollbackIncludingTheFirst(t *testing.T) {
	t.Parallel()

	// This is the exact invariant that prevents the historical worktree-lock leak:
	// even when a later step is never reached, the FIRST registered compensation
	// (the lock release) still runs on any non-committed return.
	firstRan := false
	var tx submitTxn
	tx.onRollback(func() { firstRan = true }) // e.g. release worktree lock
	tx.onRollback(func() {})                  // e.g. delete task hash (never registered if hash write fails)

	tx.rollbackIfNeeded()

	assert.True(t, firstRan, "the first-registered compensation must run on rollback")
}

func TestSubmitTxn_NilRollbackIsNoop(t *testing.T) {
	t.Parallel()

	var tx submitTxn
	tx.onRollback(nil)
	ran := false
	tx.onRollback(func() { ran = true })

	assert.NotPanics(t, func() { tx.rollbackIfNeeded() },
		"a nil compensation must be skipped without panicking")
	assert.True(t, ran, "non-nil compensations must still run when a nil one is present")
}

func TestSubmitTxn_RollbackIsIdempotent(t *testing.T) {
	t.Parallel()

	count := 0
	var tx submitTxn
	tx.onRollback(func() { count++ })

	tx.rollbackIfNeeded()
	tx.rollbackIfNeeded() // an accidental second call must not double-run

	assert.Equal(t, 1, count, "compensations must run at most once across repeated rollbackIfNeeded calls")
}

func TestSubmitTxn_EmptyIsSafe(t *testing.T) {
	t.Parallel()

	var tx submitTxn
	assert.NotPanics(t, func() { tx.rollbackIfNeeded() }, "empty transaction rollback must be a no-op")

	var committed submitTxn
	committed.commit()
	assert.NotPanics(t, func() { committed.rollbackIfNeeded() }, "committed empty transaction must be a no-op")
}
