package daemon

// submitTxn accumulates compensating (rollback) actions during a multi-step
// transactional operation such as task submission.
//
// Usage:
//
//	var tx submitTxn
//	defer tx.rollbackIfNeeded()
//	// ... perform step; on success:
//	tx.onRollback(func() { /* undo the step */ })
//	// ... after all steps succeed:
//	tx.commit()
//
// Because rollbackIfNeeded is deferred, ANY early return (an explicit error
// return, a panic, or a forgotten failure branch) runs every registered
// compensation in reverse order — unless commit has been called. This makes the
// transaction misuse-proof: it is impossible to leave a partial commit behind by
// forgetting to roll back on a specific error path.
//
// submitTxn is not safe for concurrent use; each transaction is confined to a
// single goroutine's call stack.
type submitTxn struct {
	rollbacks []func()
	committed bool
}

// onRollback registers a compensating action to run if the transaction is not
// committed. Actions run in reverse registration order (last-in, first-out), so
// each step's compensation should be registered immediately after that step
// succeeds. A nil action is accepted and treated as a no-op.
func (tx *submitTxn) onRollback(f func()) {
	tx.rollbacks = append(tx.rollbacks, f)
}

// commit marks the transaction as successful, turning the deferred
// rollbackIfNeeded into a no-op.
func (tx *submitTxn) commit() {
	tx.committed = true
}

// rollbackIfNeeded runs all registered compensations in reverse order unless the
// transaction was committed. It is intended to be deferred immediately after the
// transaction is created and is safe to call exactly once.
func (tx *submitTxn) rollbackIfNeeded() {
	if tx.committed {
		return
	}
	for i := len(tx.rollbacks) - 1; i >= 0; i-- {
		if rb := tx.rollbacks[i]; rb != nil {
			rb()
		}
	}
	// Clear so a second (accidental) call cannot double-run compensations.
	tx.rollbacks = nil
}
