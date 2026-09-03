package subflow

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/sqltx"
	"github.com/project-flogo/core/support/test"
	"github.com/project-flogo/flow/instance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// txFin is driven directly here: it is instance.TxFinalizer, so the engine's contract with it is
// fully expressible without a running flow.
var _ instance.TxFinalizer = (*txFin)(nil)

// ---------------------------------------------------------------------------
// fixture
// ---------------------------------------------------------------------------

type txFixture struct {
	db    *sql.DB
	drv   *fakeDriver
	tx    *sql.Tx
	h     *sqltx.Handle
	txCtx context.Context
	fin   *txFin

	// cancelCalls counts calls to the CancelFunc handed to txFin. When realCancel is false the
	// func does NOT cancel txCtx, which is how the rollback test proves it was CancelInFlight -
	// and not the context teardown - that unwedged the in-flight statement.
	cancelCalls int32
}

func newTxFixture(t *testing.T, realCancel bool) *txFixture {
	t.Helper()

	db, drv := newFakeDB(t, 0)

	// Rooted at Background exactly like evalTransactional does: database/sql auto-rolls-back the
	// transaction when the BeginTx context is cancelled, and the engine cancels the subflow's
	// context before the commit point.
	txCtx, txCancel := context.WithCancel(context.Background())
	t.Cleanup(txCancel)

	tx, err := db.BeginTx(txCtx, nil)
	require.NoError(t, err)

	fx := &txFixture{db: db, drv: drv, tx: tx, txCtx: txCtx}
	fx.h = sqltx.NewHandle("conn-under-test", db, tx, txCtx)

	cancel := func() {
		atomic.AddInt32(&fx.cancelCalls, 1)
		if realCancel {
			txCancel()
		}
	}
	fx.fin = &txFin{h: fx.h, cancel: cancel, logger: log.RootLogger()}

	return fx
}

func (fx *txFixture) cancelCount() int32 { return atomic.LoadInt32(&fx.cancelCalls) }

func assertDoneSoon(t *testing.T, ctx context.Context, what string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("%s was never cancelled", what)
	}
}

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

func TestTxFinCommitHappensExactlyOnce(t *testing.T) {
	fx := newTxFixture(t, true)

	require.NoError(t, fx.fin.Commit(nil))
	assert.EqualValues(t, 1, fx.drv.commitCount())
	assert.EqualValues(t, 0, fx.drv.rollbackCount())
	assert.True(t, fx.h.IsDone(), "MarkDone must run before the commit")

	// sync.Once: a second Commit replays the stored result and touches nothing.
	require.NoError(t, fx.fin.Commit(nil))
	// ...and so does a Rollback afterwards. Without the Once this would be
	// "sql: transaction has already been committed or rolled back".
	require.NoError(t, fx.fin.Rollback())

	assert.EqualValues(t, 1, fx.drv.commitCount(), "the driver must see exactly one COMMIT")
	assert.EqualValues(t, 0, fx.drv.rollbackCount(), "no ROLLBACK may follow a COMMIT")
	assert.EqualValues(t, 1, fx.cancelCount(), "the tx context must be released exactly once")
}

func TestTxFinCommitWithConfirmTrueCommits(t *testing.T) {
	fx := newTxFixture(t, true)

	confirmed := false
	require.NoError(t, fx.fin.Commit(func() bool { confirmed = true; return true }))

	assert.True(t, confirmed, "confirm must be consulted under the operation lock")
	assert.EqualValues(t, 1, fx.drv.commitCount())
	assert.EqualValues(t, 0, fx.drv.rollbackCount())
}

// TestTxFinCommitDowngradesWhenConfirmVetoes covers the D2 latch race: the engine computed
// "commit" while holding the instance state lock, released it, and a sibling branch failed before
// the finalizer got the operation lock. The recheck under that lock must turn the COMMIT into a
// ROLLBACK.
func TestTxFinCommitDowngradesWhenConfirmVetoes(t *testing.T) {
	fx := newTxFixture(t, true)

	err := fx.fin.Commit(func() bool { return false })

	require.Error(t, err)
	assert.True(t, instance.IsTxDowngraded(err), "got %v", err)
	assert.EqualValues(t, 0, fx.drv.commitCount(), "the driver must NOT have seen a COMMIT")
	assert.EqualValues(t, 1, fx.drv.rollbackCount(), "the driver must have seen a ROLLBACK")
	assert.True(t, fx.h.IsDone())

	// The stored error is replayed, and nothing else runs.
	assert.True(t, instance.IsTxDowngraded(fx.fin.Rollback()))
	assert.EqualValues(t, 1, fx.drv.rollbackCount())
	assert.EqualValues(t, 1, fx.cancelCount())
}

// TestTxFinCommitBlocksWhileTheOperationLockIsHeld is the regression test for "a COMMIT must never
// race a live statement".
//
// With concurrent branches (D3), an actreturn in one branch can drive the subflow to completion
// while a sibling branch's INSERT is still executing on the transaction's pinned connection.
// finishCommit therefore takes the operation lock with NO cancel and NO deadline. If someone ever
// swaps that for TryLockFor, this test fails.
func TestTxFinCommitBlocksWhileTheOperationLockIsHeld(t *testing.T) {
	fx := newTxFixture(t, true)

	// Stand in for a sibling branch that is mid-statement.
	fx.h.Lock()

	done := make(chan error, 1)
	go func() { done <- fx.fin.Commit(nil) }()

	select {
	case err := <-done:
		t.Fatalf("Commit returned while the operation lock was held (err=%v)", err)
	case <-time.After(300 * time.Millisecond):
		// expected: still blocked
	}
	assert.EqualValues(t, 0, fx.drv.commitCount(), "no COMMIT may reach the driver while a statement is live")
	assert.False(t, fx.h.IsDone(), "MarkDone must not run before the lock is acquired")

	// The sibling statement finishes.
	fx.h.Unlock()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Commit did not complete after the operation lock was released")
	}

	assert.EqualValues(t, 1, fx.drv.commitCount())
	assert.EqualValues(t, 0, fx.drv.rollbackCount())
}

// ---------------------------------------------------------------------------
// Rollback
// ---------------------------------------------------------------------------

func TestTxFinRollbackRollsBackExactlyOnce(t *testing.T) {
	fx := newTxFixture(t, true)

	require.NoError(t, fx.fin.Rollback())
	assert.EqualValues(t, 1, fx.drv.rollbackCount())
	assert.EqualValues(t, 0, fx.drv.commitCount())
	assert.True(t, fx.h.IsDone())

	require.NoError(t, fx.fin.Rollback())
	require.NoError(t, fx.fin.Commit(nil), "a Commit after a Rollback must be a no-op, not an error")

	assert.EqualValues(t, 1, fx.drv.rollbackCount())
	assert.EqualValues(t, 0, fx.drv.commitCount(), "the driver must never see a COMMIT after a ROLLBACK")
	assert.EqualValues(t, 1, fx.cancelCount())
}

// TestTxFinRollbackTreatsErrTxDoneAsSuccess: the transaction may already have been resolved by
// database/sql's own context watchdog, or by a driver-side abort. That is not a failure - the
// unit of work is discarded either way, which is exactly what the finalizer wanted.
func TestTxFinRollbackTreatsErrTxDoneAsSuccess(t *testing.T) {
	fx := newTxFixture(t, true)

	// Someone else got there first.
	require.NoError(t, fx.tx.Rollback())
	require.ErrorIs(t, fx.tx.Rollback(), sql.ErrTxDone, "fixture sanity: the tx is now done")

	require.NoError(t, fx.fin.Rollback(), "sql.ErrTxDone must be swallowed")
	assert.EqualValues(t, 1, fx.drv.rollbackCount(), "database/sql short-circuits, so the driver sees only the first ROLLBACK")
	assert.EqualValues(t, 1, fx.cancelCount())
}

// TestTxFinRollbackCancelsInFlightStatement: the rollback path is the ONLY one that cancels a live
// statement, so an abandoned execTimeout goroutine cannot wedge the finalizer.
//
// The fixture's cancel func deliberately does NOT cancel txCtx here, so the only thing that can
// close the statement context is Handle.CancelInFlight.
func TestTxFinRollbackCancelsInFlightStatement(t *testing.T) {
	fx := newTxFixture(t, false)

	opCtx, release := fx.h.OpContext(0)
	defer release()
	require.NoError(t, opCtx.Err(), "fixture sanity: the statement context starts live")

	require.NoError(t, fx.fin.Rollback())

	assertDoneSoon(t, opCtx, "the in-flight statement context")
	assert.ErrorIs(t, opCtx.Err(), context.Canceled)
	assert.NoError(t, fx.txCtx.Err(), "fixture sanity: this variant does not cancel the tx context")
}

// TestTxFinCommitDoesNotCancelInFlightStatements is the flip side: cancelling a sibling statement
// and committing anyway would commit a partial unit of work and issue COMMIT on a connection whose
// previous command was asynchronously cancelled.
func TestTxFinCommitDoesNotCancelInFlightStatements(t *testing.T) {
	fx := newTxFixture(t, false)

	opCtx, release := fx.h.OpContext(0)
	defer release()

	require.NoError(t, fx.fin.Commit(nil))

	assert.NoError(t, opCtx.Err(), "COMMIT must not cancel statement contexts; only ROLLBACK does")
	assert.EqualValues(t, 1, fx.drv.commitCount())
}

// ---------------------------------------------------------------------------
// Releasing the pooled connection
// ---------------------------------------------------------------------------

// TestTxFinReleasesTheTxContextOnBothPaths guards against a future edit that returns early from
// finishCommit or finishRollback. Cancelling the transaction's own context is what tears down
// database/sql's BeginTx watchdog goroutine and releases the pinned pooled connection; skipping it
// leaks a connection per transactional subflow invocation until the pool is exhausted.
func TestTxFinReleasesTheTxContextOnBothPaths(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		fx := newTxFixture(t, true)
		require.NoError(t, fx.fin.Commit(nil))
		assertDoneSoon(t, fx.txCtx, "the transaction context after COMMIT")
		assert.EqualValues(t, 1, fx.cancelCount())
	})

	t.Run("downgraded commit", func(t *testing.T) {
		fx := newTxFixture(t, true)
		require.Error(t, fx.fin.Commit(func() bool { return false }))
		assertDoneSoon(t, fx.txCtx, "the transaction context after a downgraded COMMIT")
		assert.EqualValues(t, 1, fx.cancelCount())
	})

	t.Run("rollback", func(t *testing.T) {
		fx := newTxFixture(t, true)
		require.NoError(t, fx.fin.Rollback())
		assertDoneSoon(t, fx.txCtx, "the transaction context after ROLLBACK")
		assert.EqualValues(t, 1, fx.cancelCount())
	})
}

// TestTxFinCommitReleasesThePooledConnection asserts the observable consequence of the above: the
// pool is usable again afterwards, at maxOpenConnection=1 - the setting that turns a leak into a
// hard hang.
func TestTxFinCommitReleasesThePooledConnection(t *testing.T) {
	db, drv := newFakeDB(t, 1)

	txCtx, txCancel := context.WithCancel(context.Background())
	defer txCancel()

	tx, err := db.BeginTx(txCtx, nil)
	require.NoError(t, err)

	h := sqltx.NewHandle("conn-single", db, tx, txCtx)
	fin := &txFin{h: h, cancel: txCancel, logger: log.RootLogger()}

	require.NoError(t, fin.Commit(nil))
	assert.EqualValues(t, 1, drv.commitCount())

	// The single pooled connection must be back. Without a deadline this would hang forever on a
	// leak, so bound it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx2, err := db.BeginTx(ctx, nil)
	require.NoError(t, err, "the pooled connection was not released by the finalizer")
	require.NoError(t, tx2.Rollback())
}

// ---------------------------------------------------------------------------
// Rollback while the operation lock is held
// ---------------------------------------------------------------------------

// TestTxFinRollbackProceedsWithoutTheOperationLock: unlike COMMIT, ROLLBACK is best effort about
// the lock. It must NOT block for the whole flow lifetime behind an abandoned goroutine - it waits
// at most rollbackLockBudget and then rolls back anyway.
func TestTxFinRollbackProceedsWithoutTheOperationLock(t *testing.T) {
	fx := newTxFixture(t, true)

	fx.h.Lock()
	defer fx.h.Unlock()

	start := time.Now()
	require.NoError(t, fx.fin.Rollback())
	elapsed := time.Since(start)

	assert.EqualValues(t, 1, fx.drv.rollbackCount(), "ROLLBACK must proceed without the operation lock")
	assert.GreaterOrEqual(t, elapsed, rollbackLockBudget, "it must have waited out the budget first")
	assert.Less(t, elapsed, rollbackLockBudget+5*time.Second, "but not much longer than the budget")
	assertDoneSoon(t, fx.txCtx, "the transaction context after a lock-less ROLLBACK")
}

// TestTxFinConcurrentCommitAndRollback: the engine guarantees exactly-once through txScope.done,
// but the finalizer must not depend on discipline it cannot enforce. Under -race this also proves
// there is no data race on f.err.
func TestTxFinConcurrentCommitAndRollback(t *testing.T) {
	fx := newTxFixture(t, true)

	const n = 8
	results := make(chan error, 2*n)
	startGate := make(chan struct{})

	for i := 0; i < n; i++ {
		go func() { <-startGate; results <- fx.fin.Commit(nil) }()
		go func() { <-startGate; results <- fx.fin.Rollback() }()
	}
	close(startGate)

	for i := 0; i < 2*n; i++ {
		select {
		case err := <-results:
			// Whichever won, every caller sees the same stored result.
			if err != nil {
				require.True(t, instance.IsTxDowngraded(err), "unexpected error %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("a finalizer call never returned")
		}
	}

	total := fx.drv.commitCount() + fx.drv.rollbackCount()
	assert.EqualValues(t, 1, total, "exactly one of COMMIT/ROLLBACK may reach the driver")
	assert.EqualValues(t, 1, fx.cancelCount())
}

// TestEvalRejectsDetachedTransactional covers the defence-in-depth guard in Eval.
//
// New() already rejects this combination at app load, so a correctly-constructed activity can
// never reach it. The guard exists for anything that builds a SubFlowActivity directly, where
// reaching evalTransactional would BeginTx a transaction that nothing can commit or roll back.
func TestEvalRejectsDetachedTransactional(t *testing.T) {
	a := &SubFlowActivity{
		flowURI:            "res://flow:sub",
		transactional:      true,
		detachedInvocation: true, // impossible via New(), constructed directly on purpose
	}

	done, err := a.Eval(test.NewActivityContext(activityMd))
	if err == nil {
		t.Fatal("expected an error for detached+transactional, got nil")
	}
	if !strings.Contains(err.Error(), "SUBFLOW-TX-010") {
		t.Fatalf("expected SUBFLOW-TX-010, got: %v", err)
	}
	if done {
		t.Fatal("expected done=false when the guard fires")
	}
}
