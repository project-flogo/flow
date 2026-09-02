package subflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/sqltx"
	"github.com/project-flogo/flow/instance"
)

// Transactional subflow (FLOGO-19484).
//
// This file owns the only database/sql in the whole engine-side change. flow/instance reaches the
// transaction through instance.TxFinalizer and instance.TxContextDecorator, so it stays free of
// any database dependency.

// evalTransactional begins the transaction and hands the subflow to the engine.
//
// It returns done=false, exactly like the existing non-detached paths: the subflow runs as a
// continuation on the parent's work queue, and this Eval is re-entered when it completes. The
// COMMIT or ROLLBACK happens in the engine at the subflow's terminal transition, not here.
func (a *SubFlowActivity) evalTransactional(ctx activity.Context, input map[string]interface{}) (bool, error) {
	goCtx := ctx.GoContext()
	if goCtx == nil {
		goCtx = context.Background()
	}

	// Nested guard: reject ANY ambient handle, not only one on the same connection.
	//
	// Two transactions across two databases is a two-phase-commit problem this feature does not
	// solve. Nesting on the SAME connection is worse than unsupported: a second BeginTx takes a
	// DIFFERENT pooled connection and then blocks on the row locks the outer transaction holds,
	// which is a self-deadlock with no timeout.
	//
	// This one may be an *activity.Error - it is returned from Eval, where the retry check does
	// consult Retriable(), and a nested-transaction misconfiguration is not retriable.
	if sqltx.HasAny(goCtx) {
		return false, activity.NewActivityError(
			fmt.Sprintf("nested transactional subflows are not supported; '%s' is already running inside a transaction on connection(s) %v",
				a.flowURI, sqltx.ConnIDs(goCtx)),
			"SUBFLOW-TX-002", activity.ActivityError, nil)
	}

	db, ok := a.connMgr.GetConnection().(*sql.DB)
	if !ok || db == nil {
		return false, activity.NewActivityError(
			fmt.Sprintf("connection '%s' does not expose a *sql.DB", a.connID),
			"SUBFLOW-TX-014", activity.ActivityError, nil)
	}

	if st := db.Stats(); st.MaxOpenConnections == 1 {
		ctx.Logger().Warnf("FLOGO-19484: connection '%s' is configured with maxOpenConnection=1; a transactional subflow pins that single connection for its whole duration, so every other flow using this connection will block until it commits or rolls back", a.connID)
	}

	// The transaction gets its OWN root context, deliberately NOT derived from the flow's.
	//
	// database/sql starts a watchdog goroutine in BeginTx that AUTO-ROLLS-BACK the transaction
	// as soon as that context is cancelled. The engine cancels the embedded instance's context
	// when the subflow completes - BEFORE the commit point runs - so inheriting it would roll
	// back every transactional subflow just before it was due to commit. We own this lifetime;
	// only the finalizer cancels it, after Commit or Rollback.
	txCtx, txCancel := context.WithCancel(context.Background())

	tx, err := db.BeginTx(txCtx, nil)
	if err != nil {
		txCancel()
		return false, activity.NewActivityError(
			fmt.Sprintf("unable to begin a transaction on connection '%s': %v", a.connID, err),
			"SUBFLOW-TX-015", activity.ActivityError, nil)
	}

	h := sqltx.NewHandle(a.connID, db, tx, txCtx)
	fin := &txFin{h: h, cancel: txCancel, logger: ctx.Logger()}

	// Between BeginTx and the engine publishing txScope, this transaction is reachable ONLY
	// through the local `fin`. Anything that panics in that window - GetDefinition,
	// newEmbeddedInstance, addSubFlowToCoverage - unwinds past us; the engine's own recover
	// catches it, but `fin` is gone and the leak sweep cannot help because txScope was never
	// published. Since txCtx is rooted at Background and only f.cancel() cancels it, the
	// database/sql watchdog never fires either, so the *sql.Tx and its pooled connection would be
	// pinned for the life of the process.
	//
	// Roll back and re-panic so the engine's error handling is unchanged.
	committedToEngine := false
	defer func() {
		if r := recover(); r != nil {
			if !committedToEngine {
				_ = fin.Rollback()
			}
			panic(r)
		}
	}()

	decorate := func(parent context.Context) context.Context {
		return sqltx.WithHandle(parent, a.connID, h)
	}

	// The feature's canary. The connector logs the id it derived; if the two ever differ, the
	// activities silently run outside the transaction with no error anywhere.
	ctx.Logger().Debugf("FLOGO-19484: enlisting transactional subflow '%s' on connection id '%s'", a.flowURI, a.connID)

	if err = instance.StartTransactionalSubFlow(ctx, a.flowURI, input, a.timeout, a.connID, decorate, fin); err != nil {
		// Nothing was scheduled, so nothing else will ever finalise this transaction.
		_ = fin.Rollback()
		return false, err
	}

	// From here the engine owns the scope and will finalise it; the recover above must no longer
	// roll back on our behalf.
	committedToEngine = true

	return false, nil // EvalWait
}

// txFin implements instance.TxFinalizer.
//
// Commit and Rollback have deliberately DIFFERENT locking policies; see finishCommit.
type txFin struct {
	h      *sqltx.Handle
	cancel context.CancelFunc
	logger log.Logger
	once   sync.Once
	err    error
}

// rollbackLockBudget bounds the best-effort wait for the operation lock on the ROLLBACK path
// only. The COMMIT path has no budget.
const rollbackLockBudget = 2 * time.Second

// commitLockWatchdog is how long to wait before warning that a commit is still blocked on the
// operation lock. It does not give up afterwards; it only makes the stall visible.
const commitLockWatchdog = 30 * time.Second

func (f *txFin) Commit(confirm func() bool) error {
	f.once.Do(func() { f.err = f.finishCommit(confirm) })
	return f.err
}

func (f *txFin) Rollback() error {
	f.once.Do(func() { f.err = f.finishRollback() })
	return f.err
}

// finishCommit BLOCKS on the operation lock. No CancelInFlight, no timeout.
//
// A COMMIT must never race a live statement. With concurrent branches, an actreturn in one branch
// drives the subflow to completion while a sibling branch's INSERT is still executing on the
// transaction's pinned connection. Cancelling that statement and committing anyway would commit a
// partial unit of work AND issue COMMIT on a connection whose previous command was asynchronously
// cancelled, which is "commands out of sync" territory on mssql and mysql.
//
// Blocking is the correct trade: the sibling statement is inside the very transaction we are
// about to commit, so waiting for it is waiting for our own work. If it never returns the flow
// was wedged either way, and the watchdog makes that visible instead of silent.
func (f *txFin) finishCommit(confirm func() bool) error {
	stop := make(chan struct{})
	go func() {
		select {
		case <-time.After(commitLockWatchdog):
			f.logger.Warnf("FLOGO-19484: COMMIT of the transactional subflow on connection '%s' has been waiting %v for the operation lock; a statement inside the transaction has not returned",
				f.h.ConnID(), commitLockWatchdog)
		case <-stop:
		}
	}()

	f.h.Lock() // BLOCKING: no cancel, no deadline.
	close(stop)

	defer func() {
		f.h.Unlock()
		f.cancel() // release the pooled connection and the transaction context, unconditionally
	}()

	// Recheck the latch under the operation lock. The engine computed the verdict while holding
	// the instance state lock and released it before calling us, so a sibling branch can have
	// failed in between. Rechecking here is exact, because this is the same lock that serialises
	// statement execution.
	if confirm != nil && !confirm() {
		f.h.MarkDone()
		if rbErr := f.h.Tx().Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			f.logger.Errorf("FLOGO-19484: downgraded ROLLBACK failed on connection '%s': %v", f.h.ConnID(), rbErr)
			return fmt.Errorf("%w (rollback itself failed: %v)", instance.ErrTxDowngraded, rbErr)
		}
		return instance.ErrTxDowngraded
	}

	f.h.MarkDone() // no further statements; the commit is about to close every one of them

	if err := f.h.Tx().Commit(); err != nil {
		// ErrTxDone here means the driver had ALREADY resolved the transaction. txCtx is only
		// cancelled after this point, so that resolution can only have been a driver-side abort -
		// nothing was committed. Reporting it as a commit failure would be wrong twice over: the
		// engine wraps a commit failure in the deliberately non-retriable *TxCommitError, so the
		// user would get "the commit is in doubt" for a transaction that provably did not commit,
		// and would lose the retry that a rollback is entitled to. Route it to the rollback shape
		// instead, matching finishRollback, which already treats ErrTxDone as resolved.
		if errors.Is(err, sql.ErrTxDone) {
			return instance.ErrTxDowngraded
		}
		return err
	}
	return nil
}

// finishRollback is the ONLY path that cancels an in-flight statement. The work is being discarded
// anyway, so unwedging an abandoned execTimeout goroutine is strictly better than waiting for it.
func (f *txFin) finishRollback() error {
	f.h.CancelInFlight()
	f.h.MarkDone()

	got := f.h.TryLockFor(rollbackLockBudget)
	if !got {
		f.logger.Warnf("FLOGO-19484: proceeding with ROLLBACK on connection '%s' without the operation lock after %v",
			f.h.ConnID(), rollbackLockBudget)
	}

	defer func() {
		if got {
			f.h.Unlock()
		}
		f.cancel()
	}()

	err := f.h.Tx().Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		// Already resolved by the driver, e.g. the context watchdog got there first. Not an error.
		return nil
	}
	return err
}
