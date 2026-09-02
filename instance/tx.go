package instance

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/support/log"
)

// Transactional subflow support (FLOGO-19484).
//
// LAYERING RULE, NON-NEGOTIABLE: this package must NOT import database/sql. flow/instance is
// linked into every Flogo binary, including apps that never touch a database. The transaction
// itself is owned by the subflow activity, which lives in the separately-versioned
// flow/activity/subflow module; the engine reaches it only through the TxFinalizer interface and
// the TxContextDecorator func below.
//
// If a test in this package needs a fake SQL driver, the layering has been violated.

// ---------------------------------------------------------------------------
// Indirection across the layering boundary
// ---------------------------------------------------------------------------

// TxFinalizer commits or rolls back the transaction a transactional subflow runs on.
//
// Commit MUST:
//  1. acquire the connector-facing operation lock and BLOCK for it - no cancellation, no
//     timeout. A commit must never race a live statement;
//  2. call confirm() WHILE HOLDING that lock and, if it returns false, ROLL BACK instead and
//     return an error for which IsTxDowngraded reports true;
//  3. release the pooled connection whatever happens.
//
// Rollback MUST cancel any in-flight statement first, is best-effort about the operation lock,
// and must release the pooled connection whatever happens.
//
// Both must be safe to call exactly once; the caller guarantees exactly-once through
// txScope.done.
type TxFinalizer interface {
	Commit(confirm func() bool) error
	Rollback() error
}

// TxContextDecorator layers the ambient transaction onto the context handed to the embedded
// subflow instance. Supplied by the subflow activity, which is the only component that knows
// about database/sql.
type TxContextDecorator func(context.Context) context.Context

// txContextPropagator copies a transaction registry from one context onto another while
// preserving the destination's cancellation. It defaults to the identity so that a build which
// never links the subflow activity allocates nothing and behaves exactly as before.
var txContextPropagator = func(_, dst context.Context) context.Context { return dst }

// SetTxContextPropagator registers the real propagator. Called from the subflow activity's
// init(); passing nil restores the identity default.
func SetTxContextPropagator(fn func(src, dst context.Context) context.Context) {
	if fn == nil {
		fn = func(_, dst context.Context) context.Context { return dst }
	}
	txContextPropagator = fn
}

func propagateTxCtx(src, dst context.Context) context.Context {
	return txContextPropagator(src, dst)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// Error codes. CodeTxRolledBack is the ONLY finalisation error that is an *activity.Error.
const (
	CodeTxRolledBack   = "SUBFLOW-TX-001" // rollback - RETRIABLE *activity.Error (D14)
	CodeTxCommitFailed = "SUBFLOW-TX-003" // commit   - PLAIN error, never retried (D14)
	CodeTxSweep        = "SUBFLOW-TX-006"
	CodeTxNotResumable = "SUBFLOW-TX-007"
)

// ErrTxDowngraded is returned by Commit when confirm() vetoed the commit under the operation
// lock. The transaction was ROLLED BACK.
var ErrTxDowngraded = errors.New("SUBFLOW-TX-001: commit downgraded to rollback: a task failed while the transaction was being committed")

// IsTxDowngraded reports whether a Commit was downgraded to a rollback.
func IsTxDowngraded(err error) bool { return errors.Is(err, ErrTxDowngraded) }

var errTxSweep = errors.New(CodeTxSweep + ": flow stopped with the transaction still open")

// newTxRollbackError builds the error the engine hands to the host TaskInst when a transactional
// subflow ROLLS BACK.
//
// D14: it is a RETRIABLE *activity.Error on purpose. model/simple/taskbehavior.go retries any
// *activity.Error when the host task has retryOnError configured; that replay re-enters
// SubFlowActivity.Eval, which begins a BRAND NEW transaction on a BRAND NEW embedded instance.
// D2 guarantees nothing from the failed attempt was committed, so the replay is safe - it is
// exactly what retryOnError is for. Retriable is also set so the Eval-side check, which does
// consult it, agrees, and so a future tightening of the PostEval side does not silently disable
// retry here.
//
// Being an *activity.Error also means appendErrorData builds a full error object (code,
// category, data) for the user's error branch, instead of the bare string a plain error yields.
func newTxRollbackError(connID string, cause, rbErr error, downgraded bool) *activity.Error {
	msg := fmt.Sprintf("%s: transactional subflow rolled back on connection '%s'", CodeTxRolledBack, connID)
	if cause != nil {
		msg += ": " + cause.Error()
	}
	if rbErr != nil {
		msg += fmt.Sprintf(" (rollback itself failed: %v)", rbErr)
	}

	data := map[string]interface{}{
		"connectionId": connID,
		"downgraded":   downgraded,
	}
	if cause != nil {
		data["cause"] = cause.Error()
	}
	if rbErr != nil {
		data["rollbackError"] = rbErr.Error()
	}

	return activity.NewRetriableActivityError(msg, CodeTxRolledBack, activity.ActivityError, data)
}

// TxCommitError is returned when COMMIT itself failed. It is deliberately a PLAIN error and
// deliberately NOT an *activity.Error.
//
// D14: a failed COMMIT is IN DOUBT - the server may have applied the transaction and lost the
// acknowledgement. Replaying the subflow could double-apply every write in it, plus any
// non-transactional side effect it performed. taskbehavior.go's PostEval path retries any
// *activity.Error WITHOUT consulting Retriable, so the only reliable way to stay un-retried is
// to not be one.
//
// Do not "improve" this into an activity.NewActivityError; TestCommitFailureErrorIsNotAnActivityError
// exists to stop that.
type TxCommitError struct {
	ConnID string
	Err    error
}

func (e *TxCommitError) Error() string {
	return fmt.Sprintf("%s: transactional subflow commit failed on connection '%s': %v",
		CodeTxCommitFailed, e.ConnID, e.Err)
}

func (e *TxCommitError) Unwrap() error { return e.Err }

// txResumeErr is the D10 tripwire: a flow was resumed into the middle of a transactional
// subflow despite the record-side marker and the restore-side rejection.
func txResumeErr(instanceID, subflowName string) error {
	return fmt.Errorf("%s: flow instance [%s] was resumed inside transactional subflow '%s'; the transaction did not survive, so its work cannot be completed",
		CodeTxNotResumable, instanceID, subflowName)
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

// txScope hangs off exactly ONE Instance: the embedded instance of a transactional subflow, the
// OWNER. Unexported and NOT serialised - instance_ser.go marshals through explicit
// representation structs that list their fields.
type txScope struct {
	fin    TxFinalizer
	connID string
	logger log.Logger

	mu       sync.Mutex
	failed   bool  // the D2 latch; set by markTxFailed, read by decideTx and by the confirm callback
	firstErr error // first non-nil error observed
	done     bool  // claim-once flag: the finalizer runs exactly once
}

// ---------------------------------------------------------------------------
// TWO SCOPE LOOKUPS. THE DIFFERENCE IS THE WHOLE OF BLOCKER Y1. DO NOT MERGE THEM.
// ---------------------------------------------------------------------------

// txScopeOwner returns the nearest ancestor instance - inst itself first - that OWNS a
// transaction scope, or nil. It walks UP the host chain, so an error raised inside a nested
// NON-transactional subflow, which inherits the ambient handle through the go context, also
// latches the outer transaction. Hop-bounded so a corrupt host chain cannot spin.
//
// USE FOR LATCHING ONLY (markTxFailed). Never for finalisation.
func (inst *Instance) txScopeOwner() *txScope {
	cur := inst
	for hops := 0; cur != nil && hops < 64; hops++ {
		if cur.txScope != nil {
			return cur.txScope
		}
		host, ok := cur.host.(*TaskInst)
		if !ok || host == nil {
			return nil
		}
		cur = host.flowInst
	}
	return nil
}

// txScopeOwned returns the scope this instance OWNS, or nil. NO walk.
//
// USE FOR FINALISATION ONLY (decideTx). Y1: every finishTx call site sits inside a
// `containerInst != inst.Instance` branch that fires for ANY embedded instance. With a walking
// lookup, a plain subflow B nested inside a transactional subflow A would claim and COMMIT A's
// transaction the moment B finished, while A was still mid-flight; A's own terminal transition
// would then find scope.done already true, drop its verdict, and report success with
// host.returnError == nil.
func (inst *Instance) txScopeOwned() *txScope {
	if inst == nil {
		return nil
	}
	return inst.txScope
}

// markTxFailed is the D2 latch. It is called from every point at which the engine observes an
// activity error that SURVIVED retry (D12).
//
// It walks UP on purpose: an error inside a nested non-transactional subflow must roll the OUTER
// transaction back.
//
// `failed` is set UNCONDITIONALLY. Every handleTaskCancelled call site passes err == nil, and an
// execTimeout-driven cancellation must still roll back (D8). Only a non-nil err is recorded, and
// only the FIRST one.
//
// Takes only scope.mu, so it is safe to call with or without the instance state lock held.
func markTxFailed(containerInst *Instance, err error) {
	if containerInst == nil {
		return
	}
	scope := containerInst.txScopeOwner() // WALK UP - see Y1
	if scope == nil {
		return
	}

	scope.mu.Lock()
	scope.failed = true
	if scope.firstErr == nil && err != nil {
		scope.firstErr = err
	}
	scope.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Verdict
// ---------------------------------------------------------------------------

type txVerdict struct {
	scope  *txScope
	commit bool
	cause  error

	// master is retained so apply() can drop txScopeActive only once the transaction is truly
	// finished. See the note in decideTx.
	master *IndependentInstance
}

// decideTx claims the scope OWNED BY containerInst exactly once and returns the verdict, or nil
// when containerInst owns no scope or another goroutine already claimed it. It performs NO I/O
// and takes no instance lock, so it is safe to call with the state lock held.
//
// Y1: the lookup is txScopeOwned - no walk. finishTx therefore NO-OPS for a nested, non-owning
// embedded instance. A nested failure still rolls back correctly, because markTxFailed has
// already latched scope.failed on the owning ancestor and that owner's own terminal transition
// consumes it.
//
// completedOK is true only at the single success point, handleTaskDone.
func decideTx(containerInst *Instance, completedOK bool, cause error) *txVerdict {
	if containerInst == nil {
		return nil
	}
	scope := containerInst.txScopeOwned() // STRICT OWNER - see Y1
	if scope == nil {
		return nil
	}

	scope.mu.Lock()
	if scope.done {
		scope.mu.Unlock()
		return nil
	}
	scope.done = true
	failed, first := scope.failed, scope.firstErr
	scope.mu.Unlock()

	if cause == nil {
		cause = first
	}
	// NOTE: txScopeActive is NOT decremented here. The counter must stay above zero until the
	// Commit or Rollback has actually completed, because it gates transaction-registry
	// propagation to concurrently-running tasks. Decrementing at claim time meant a task still
	// in flight lost the registry mid-transaction and silently fell back to the pool - writing
	// outside the transaction - instead of getting the loud ErrTxFinished it is supposed to get.
	// The decrement lives in txVerdict.apply(), after the round trip.

	// D2: COMMIT only when the subflow reached its normal completion point AND nothing inside it,
	// or inside anything nested in it, ever errored. Everything else - including an error a user
	// error branch consumed so the subflow returned normally - rolls back.
	return &txVerdict{scope: scope, commit: completedOK && !failed && cause == nil, cause: cause,
		master: containerInst.master}
}

// apply performs the COMMIT or the ROLLBACK. It MUST run with no instance state lock held.
// Returns nil ONLY when the transaction committed successfully.
func (v *txVerdict) apply() error {
	s := v.scope

	// Drop the active count only once the round trip is over, never at claim time - a task still
	// in flight must keep seeing the registry so it gets ErrTxFinished rather than silently
	// falling back to the pool.
	defer func() {
		if v.master != nil {
			v.master.txScopeActive.Add(-1)
		}
	}()

	if v.commit {
		// Recheck the latch under the operation lock: decideTx ran with the state lock held and
		// apply runs with it released, so a sibling branch can have failed in between.
		confirm := func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return !s.failed
		}

		err := s.fin.Commit(confirm)
		switch {
		case err == nil:
			s.logger.Debugf("FLOGO-19484: committed the transactional subflow on connection '%s'", s.connID)
			return nil
		case IsTxDowngraded(err):
			s.mu.Lock()
			late := s.firstErr
			s.mu.Unlock()
			s.logger.Errorf("FLOGO-19484: COMMIT on connection '%s' was downgraded to ROLLBACK: a task failed while the transaction was being committed: %v", s.connID, late)
			return newTxRollbackError(s.connID, late, nil, true)
		default:
			s.logger.Errorf("FLOGO-19484: COMMIT failed for the transactional subflow on connection '%s': %v", s.connID, err)
			// Do NOT attempt a Rollback afterwards - its error would mask this one and the driver
			// has already resolved the transaction one way or the other.
			return &TxCommitError{ConnID: s.connID, Err: err}
		}
	}

	rbErr := s.fin.Rollback()
	if rbErr != nil {
		s.logger.Errorf("FLOGO-19484: ROLLBACK failed for the transactional subflow on connection '%s': %v", s.connID, rbErr)
	}
	return newTxRollbackError(s.connID, v.cause, rbErr, false)
}

// ---------------------------------------------------------------------------
// Finalisation
// ---------------------------------------------------------------------------

// withStateUnlocked runs fn with the instance state lock RELEASED when the CALLER says it holds
// it, re-acquiring it afterwards, including on panic. unlockState/lockState are no-ops when
// stateLock is nil (sequential mode), so passing lockHeld=true there is harmless.
//
// lockHeld is threaded down from the driver rather than sniffed from a flag. A per-master
// atomic.Bool cannot express mutex OWNERSHIP - Instance.Return and newEmbeddedInstance both take
// and release the state lock from activity goroutines, so "set" does not mean "set by me".
// Getting that wrong means Unlocking a mutex owned by another goroutine, which sync.Mutex does
// not detect: it silently loses mutual exclusion and only later panics with "unlock of unlocked
// mutex".
func (inst *Instance) withStateUnlocked(lockHeld bool, fn func()) {
	if lockHeld {
		inst.unlockState()
		defer inst.lockState()
	}
	fn()
}

// finishTx decides under whatever lock the caller holds, then runs the Commit/Rollback round trip
// with the state lock released.
//
// lockHeld MUST be the truth; it is threaded from the drivers.
func finishTx(containerInst *Instance, completedOK bool, cause error, lockHeld bool) error {
	v := decideTx(containerInst, completedOK, cause)
	if v == nil {
		return nil
	}

	var err error
	containerInst.withStateUnlocked(lockHeld, func() { err = v.apply() })
	return err
}

// ---------------------------------------------------------------------------
// D10 - resume marking
// ---------------------------------------------------------------------------

// TxInFlightAttr marks an instance whose recorded state was captured while a transactional
// subflow was in flight. Instance.Delete is an empty function, so the marker is CLEARED by
// setting it to false, never by deleting it.
const TxInFlightAttr = "_txInFlight"

func setTxInFlight(inst *Instance, v bool) {
	if inst != nil {
		_ = inst.SetValue(TxInFlightAttr, v)
	}
}

// stampTxInFlight marks the master instance when any embedded subflow currently owns a
// transaction, so a later resume can be refused (D10).
func (inst *IndependentInstance) stampTxInFlight() {
	active := inst.txScopeActive.Load() > 0
	if !active && !isTxInFlight(inst.Instance) {
		return // nothing to set and nothing to clear
	}
	setTxInFlight(inst.Instance, active)
	if active && !inst.txRecordWarned {
		inst.txRecordWarned = true
		inst.logger.Warnf("FLOGO-19484: flow instance [%s] state is being recorded while a transactional subflow is in flight; this checkpoint is NOT resumable", inst.ID())
	}
}

// isTxInFlight reads through Instance.GetValue so it takes rlockAttrs. A raw inst.attrs read
// races the SetValue a sibling branch's activity can be executing: the commit-point tripwire runs
// under stateLock, which is a DIFFERENT lock from attrsLock.
//
// GetValue falls through to the flow definition when the key is absent, which is harmless for a
// reserved "_"-prefixed name no flow definition declares.
func isTxInFlight(inst *Instance) bool {
	if inst == nil {
		return false
	}
	v, ok := inst.GetValue(TxInFlightAttr)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// returnErrorLocked reads Instance.returnError under the attrs lock. Instance.Return writes it
// under lockAttrs from an activity goroutine, so the commit point - which holds only stateLock -
// must not read the field directly. GetError is deliberately not reused: it does not lock.
func (inst *Instance) returnErrorLocked() error {
	if inst == nil {
		return nil
	}
	inst.rlockAttrs()
	defer inst.runlockAttrs()
	return inst.returnError
}

// RejectIfTxInFlight refuses to resume a flow whose recorded state was captured inside a
// transactional subflow. The transaction did not survive the restart, so resuming would run the
// remainder of the subflow with no transaction at all and then report success (D10).
func RejectIfTxInFlight(inst *IndependentInstance) error {
	if inst == nil {
		return nil
	}
	if isTxInFlight(inst.Instance) {
		return txResumeErr(inst.ID(), inst.Name())
	}
	for _, sub := range inst.subflows {
		if isTxInFlight(sub) {
			return txResumeErr(inst.ID(), sub.Name())
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Leak sweep
// ---------------------------------------------------------------------------

// RollbackOpenTransactions rolls back any transaction still open when the flow stops running.
//
// It is registered as the FIRST defer in each driver's goroutine so it runs LAST, surviving a
// panic in the tail and the detached branch, which has no defer of its own. Reaching it means a
// terminal transition was missed - the step loop hit its max-step cap, or the driver returned
// early - so it logs at ERROR rather than staying silent.
//
// O(1) when the feature is unused: txScopeActive is zero and nothing is scanned.
func RollbackOpenTransactions(inst *IndependentInstance) {
	if inst == nil || inst.txScopeActive.Load() == 0 {
		return
	}

	var victims []*Instance
	inst.lockState()
	for _, sub := range inst.subflows {
		if sub != nil && sub.txScope != nil {
			victims = append(victims, sub)
		}
	}
	inst.unlockState()

	for _, ci := range victims {
		v := decideTx(ci, false, errTxSweep)
		if v == nil {
			continue
		}
		inst.logger.Errorf("FLOGO-19484: transaction on connection '%s' in subflow '%s' of flow instance [%s] was still open when the flow stopped; rolling back. A terminal transition was missed.",
			v.scope.connID, ci.Name(), inst.ID())
		_ = v.apply() // no lock held here
	}
}
