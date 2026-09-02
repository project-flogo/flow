package instance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/state"
	"github.com/stretchr/testify/assert"
)

// FLOGO-19484 - engine-side tests for the transactional subflow.
//
// LAYERING: this file deliberately contains NO database/sql. The engine only ever sees a
// TxFinalizer, so the double here is a stub that records an ordered event log. If a test in this
// package ever needs a fake SQL driver, tx.go's layering rule has been broken.

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeFin is the stand-in for the subflow activity's txFin. It records an ORDERED event log so a
// test can assert not just "one commit happened" but "the commit happened after B finished".
type fakeFin struct {
	mu     sync.Mutex
	events []string

	commitErr   error
	rollbackErr error

	// confirmSeen records what the confirm callback returned on the Commit path.
	confirmSeen *bool

	// beforeCommit runs at the top of Commit, before confirm() is consulted. It is how a test
	// injects a sibling failure into the commit window, or probes for a held state lock.
	beforeCommit func()
}

func (f *fakeFin) record(e string) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

func (f *fakeFin) Events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fakeFin) Commit(confirm func() bool) error {
	if f.beforeCommit != nil {
		f.beforeCommit()
	}
	ok := true
	if confirm != nil {
		ok = confirm()
	}
	f.mu.Lock()
	f.confirmSeen = &ok
	f.mu.Unlock()

	if !ok {
		// Mirrors the real finalizer: a vetoed commit becomes a rollback and reports downgrade.
		f.record("rollback:downgraded")
		return ErrTxDowngraded
	}
	f.record("commit")
	return f.commitErr
}

func (f *fakeFin) Rollback() error {
	f.record("rollback")
	return f.rollbackErr
}

// recordingLogger captures Errorf/Warnf output so the sweep's "a terminal transition was missed"
// diagnostic can be asserted. Everything else is delegated to the embedded root logger.
type recordingLogger struct {
	log.Logger
	mu   sync.Mutex
	errs []string
	wrns []string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{Logger: log.RootLogger()}
}

func (l *recordingLogger) Errorf(template string, args ...interface{}) {
	l.mu.Lock()
	l.errs = append(l.errs, fmt.Sprintf(template, args...))
	l.mu.Unlock()
}

func (l *recordingLogger) Warnf(template string, args ...interface{}) {
	l.mu.Lock()
	l.wrns = append(l.wrns, fmt.Sprintf(template, args...))
	l.mu.Unlock()
}

func (l *recordingLogger) Errors() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.errs...)
}

func (l *recordingLogger) Warns() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.wrns...)
}

func containsSubstr(t *testing.T, lines []string, want string) bool {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tree builder
// ---------------------------------------------------------------------------

// txTree is the Y1 topology:
//
//	master ──taskA──▶ A (TRANSACTIONAL, owns the txScope)
//	                    └──taskB──▶ B (PLAIN, owns nothing)
//	                                  └── innerB (an ordinary task)
type txTree struct {
	master *IndependentInstance
	taskA  *TaskInst // host task of A; lives in master
	a      *Instance // transactional subflow - the scope OWNER
	taskB  *TaskInst // host task of B; lives in A
	b      *Instance // plain nested subflow - owns nothing
	innerB *TaskInst // an ordinary task inside B
	fin    *fakeFin
	scope  *txScope
	logger *recordingLogger
}

func newTxTree(t *testing.T, concurrent bool, fin *fakeFin) *txTree {
	t.Helper()

	master := newConcurrencyTestInstance(t, concurrent)
	// handleGlobalError/handleCancelError call master.RecordState unconditionally for an embedded
	// instance, and RecordState dereferences instRecorder. Recording is OFF, so nothing is stamped.
	master.instRecorder = NewStateInstanceRecorder(nil, state.RecordingModeOff, false)
	rl := newRecordingLogger()
	master.logger = rl
	master.SetStatus(model.FlowStatusActive)

	taskA, _ := master.FindOrCreateTaskInst(master.flowDef.GetTask("LogStart"))
	a := master.newEmbeddedInstance(taskA, "", getDef(), context.Background(), nil)
	a.SetStatus(model.FlowStatusActive)

	scope := &txScope{fin: fin, connID: "conn-y1", logger: rl}
	master.lockState()
	a.txScope = scope
	master.unlockState()
	master.txScopeActive.Add(1)

	taskB, _ := a.FindOrCreateTaskInst(a.flowDef.GetTask("LogStart"))
	b := master.newEmbeddedInstance(taskB, "", getDef(), context.Background(), nil)
	b.SetStatus(model.FlowStatusActive)

	innerB, _ := b.FindOrCreateTaskInst(b.flowDef.GetTask("LogResult"))

	return &txTree{master: master, taskA: taskA, a: a, taskB: taskB, b: b, innerB: innerB, fin: fin, scope: scope, logger: rl}
}

// completeSubflow drives the terminal-success transition of `sub` through its host task.
func (tr *txTree) completeSubflow(sub *Instance, viaTask *TaskInst) {
	sub.forceCompletion = true
	tr.master.handleTaskDone(&stubBehavior{evalResult: model.EvalDone}, viaTask, false)
}

func forEachMode(t *testing.T, fn func(t *testing.T, concurrent bool)) {
	t.Helper()
	for _, concurrent := range []bool{false, true} {
		name := "sequential"
		if concurrent {
			name = "concurrent"
		}
		t.Run(name, func(t *testing.T) { fn(t, concurrent) })
	}
}

// ---------------------------------------------------------------------------
// Y1 - the blocker: a nested PLAIN subflow must never finalise the outer transaction
// ---------------------------------------------------------------------------

// T-Y1-1: B completes normally => ZERO commits and ZERO rollbacks at that instant; A then
// completes => exactly ONE commit. The ORDERING is the assertion, not just the totals: with a
// walking finalisation lookup B would commit A's transaction while A was still mid-flight.
func TestY1NestedPlainSubflowDoesNotCommitOuterTransaction(t *testing.T) {
	forEachMode(t, func(t *testing.T, concurrent bool) {
		fin := &fakeFin{}
		tr := newTxTree(t, concurrent, fin)

		// --- B's terminal transition ---
		tr.completeSubflow(tr.b, tr.innerB)

		assert.Empty(t, fin.Events(),
			"Y1: a nested PLAIN subflow completing must not commit or roll back the outer transaction")
		assert.False(t, tr.scope.done, "Y1: B must not have claimed A's scope")
		assert.Equal(t, int32(1), tr.master.txScopeActive.Load(), "the scope is still open after B finishes")

		// --- A's terminal transition ---
		tr.completeSubflow(tr.a, tr.taskB)

		assert.Equal(t, []string{"commit"}, fin.Events(),
			"exactly one commit, and it happens at A's terminal transition")
		assert.True(t, tr.scope.done)
		assert.Equal(t, int32(0), tr.master.txScopeActive.Load())
		assert.Nil(t, tr.taskA.returnError, "a clean commit leaves the host task's error nil")
	})
}

// T-Y1-2: B FAILS => markTxFailed walks up and latches on A, but the ROLLBACK happens at A's
// terminal transition, not B's. This also covers D2: the subflow returned normally (its host was
// rescheduled with the error consumed) and it still rolls back.
func TestY1NestedFailureLatchesOwnerAndRollsBackAtOwnerTerminal(t *testing.T) {
	forEachMode(t, func(t *testing.T, concurrent bool) {
		fin := &fakeFin{}
		tr := newTxTree(t, concurrent, fin)

		boom := errors.New("inner activity blew up")

		// --- B's task fails; nothing in B owns a scope ---
		tr.master.handleTaskError(&stubBehavior{}, tr.innerB, boom, false)

		tr.scope.mu.Lock()
		failed, first := tr.scope.failed, tr.scope.firstErr
		tr.scope.mu.Unlock()
		assert.True(t, failed, "Y1: markTxFailed must WALK UP from B and latch A's scope")
		assert.Equal(t, boom, first, "the first error is recorded")

		assert.Empty(t, fin.Events(),
			"Y1: the rollback must NOT happen at B's terminal transition")
		assert.False(t, tr.scope.done)

		// --- A's terminal transition consumes the latch ---
		tr.completeSubflow(tr.a, tr.taskB)

		assert.Equal(t, []string{"rollback"}, fin.Events(),
			"the rollback happens exactly once, at A's terminal transition")
		assert.Equal(t, int32(0), tr.master.txScopeActive.Load())

		// D14: what the host task sees is a RETRIABLE *activity.Error.
		require(t, tr.taskA.returnError != nil, "A's host task must be handed the rollback error")
		ae, ok := tr.taskA.returnError.(*activity.Error)
		assert.True(t, ok, "the rollback error must be an *activity.Error")
		assert.Equal(t, CodeTxRolledBack, ae.Code())
		assert.True(t, ae.Retriable())
		assert.Contains(t, ae.Error(), boom.Error(), "the original cause is reported")
	})
}

// T-Y1-4: negative control. decideTx(B, ...) must return nil and A's scope must still be unclaimed
// immediately after B's terminal transition.
func TestY1DecideTxOnNestedPlainSubflowReturnsNil(t *testing.T) {
	forEachMode(t, func(t *testing.T, concurrent bool) {
		fin := &fakeFin{}
		tr := newTxTree(t, concurrent, fin)

		tr.completeSubflow(tr.b, tr.innerB)

		assert.Nil(t, decideTx(tr.b, true, nil), "B owns no scope, so it can never produce a verdict")

		tr.scope.mu.Lock()
		done := tr.scope.done
		tr.scope.mu.Unlock()
		assert.False(t, done, "A's scope must still be unclaimed after B's terminal transition")
		assert.Empty(t, fin.Events())

		// And the strict/walking distinction itself:
		assert.Nil(t, tr.b.txScopeOwned(), "txScopeOwned must NOT walk")
		assert.Same(t, tr.scope, tr.b.txScopeOwner(), "txScopeOwner MUST walk")
	})
}

// ---------------------------------------------------------------------------
// decideTx / markTxFailed
// ---------------------------------------------------------------------------

func newScopedInst(fin TxFinalizer) (*Instance, *txScope) {
	s := &txScope{fin: fin, connID: "conn-unit", logger: log.RootLogger()}
	return &Instance{txScope: s}, s
}

// TestDecideTxClaimsScopeExactlyOnce runs two concurrent claims on one scope; exactly one may
// produce a verdict. Meaningful under -race.
func TestDecideTxClaimsScopeExactlyOnce(t *testing.T) {
	for i := 0; i < 200; i++ {
		ci, _ := newScopedInst(&fakeFin{})

		var wg sync.WaitGroup
		results := make([]*txVerdict, 2)
		start := make(chan struct{})
		for g := 0; g < 2; g++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				<-start
				results[n] = decideTx(ci, true, nil)
			}(g)
		}
		close(start)
		wg.Wait()

		nonNil := 0
		for _, v := range results {
			if v != nil {
				nonNil++
			}
		}
		assert.Equal(t, 1, nonNil, "the scope must be claimed exactly once")
	}
}

// TestDecideTxCommitTruthTable pins D2: commit ONLY when completedOK && !failed && cause == nil.
func TestDecideTxCommitTruthTable(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name        string
		completedOK bool
		failed      bool
		cause       error
		wantCommit  bool
	}{
		{"completed, clean", true, false, nil, true},
		{"completed, latched failure", true, true, nil, false},
		{"completed, explicit cause", true, false, boom, false},
		{"completed, latched + cause", true, true, boom, false},
		{"not completed, clean", false, false, nil, false},
		{"not completed, latched failure", false, true, nil, false},
		{"not completed, explicit cause", false, false, boom, false},
		{"not completed, latched + cause", false, true, boom, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ci, s := newScopedInst(&fakeFin{})
			if tc.failed {
				markTxFailed(ci, nil)
			}
			v := decideTx(ci, tc.completedOK, tc.cause)
			assert.NotNil(t, v)
			assert.Equal(t, tc.wantCommit, v.commit)
			assert.Same(t, s, v.scope)
		})
	}
}

// TestDecideTxFallsBackToFirstLatchedError verifies the verdict reports the latched cause when the
// caller supplies none, and that an explicit cause wins.
func TestDecideTxFallsBackToFirstLatchedError(t *testing.T) {
	latched := errors.New("latched")
	explicit := errors.New("explicit")

	ci, _ := newScopedInst(&fakeFin{})
	markTxFailed(ci, latched)
	v := decideTx(ci, true, nil)
	assert.NotNil(t, v)
	assert.Equal(t, latched, v.cause)

	ci2, _ := newScopedInst(&fakeFin{})
	markTxFailed(ci2, latched)
	v2 := decideTx(ci2, true, explicit)
	assert.NotNil(t, v2)
	assert.Equal(t, explicit, v2.cause)
}

// TestDecideTxNoScopeOrNilInstance covers the two no-op guards.
func TestDecideTxNoScopeOrNilInstance(t *testing.T) {
	assert.Nil(t, decideTx(nil, true, nil))
	assert.Nil(t, decideTx(&Instance{}, true, nil))
	assert.Nil(t, finishTx(nil, true, nil, false))
	assert.Nil(t, finishTx(&Instance{}, true, nil, false))
}

// TestDecideTxDecrementsScopeActive verifies the O(1) feature gate is released on the claim.
// TestScopeActiveStaysUpUntilTheTransactionIsFinished pins a contract that is easy to get wrong
// and silent when you do.
//
// txScopeActive must NOT drop when decideTx claims the scope - only after apply() has finished
// the Commit or Rollback. execTaskConcurrent used to gate transaction-registry propagation on
// this counter, so decrementing at claim time meant a task still in flight lost the registry
// mid-transaction and silently fell back to the connection pool, writing OUTSIDE the transaction,
// instead of getting the loud ErrTxFinished it is meant to get.
func TestScopeActiveStaysUpUntilTheTransactionIsFinished(t *testing.T) {
	master := newConcurrencyTestInstance(t, true)
	fin := &fakeFin{}
	ci := &Instance{master: master, txScope: &txScope{fin: fin, logger: log.RootLogger()}}
	master.txScopeActive.Add(1)

	v := decideTx(ci, true, nil)
	assert.NotNil(t, v)
	assert.Equal(t, int32(1), master.txScopeActive.Load(),
		"claiming the scope must NOT drop the active count; the transaction is not finished yet")

	assert.NoError(t, v.apply())
	assert.Equal(t, int32(0), master.txScopeActive.Load(),
		"the active count drops once the commit/rollback round trip is done")

	// A second, losing claim must neither finalise nor decrement again.
	assert.Nil(t, decideTx(ci, true, nil))
	assert.Equal(t, int32(0), master.txScopeActive.Load())
	assert.Equal(t, []string{"commit"}, fin.Events(), "the finalizer runs exactly once")
}

// TestMarkTxFailedSetsFailedUnconditionally: `failed` is set even for err == nil, which is exactly
// what every handleTaskCancelled call site passes - the execTimeout rollback depends on it. Only
// the FIRST non-nil error is recorded.
func TestMarkTxFailedSetsFailedUnconditionally(t *testing.T) {
	ci, s := newScopedInst(&fakeFin{})

	// err == nil - the handleTaskCancelled / execTimeout shape.
	markTxFailed(ci, nil)
	s.mu.Lock()
	failed, first := s.failed, s.firstErr
	s.mu.Unlock()
	assert.True(t, failed, "a nil-error latch must still force a rollback (D8/execTimeout)")
	assert.Nil(t, first)

	// A verdict built from that latch must NOT commit.
	probe, probeScope := newScopedInst(&fakeFin{})
	markTxFailed(probe, nil)
	assert.False(t, decideTx(probe, true, nil).commit)
	assert.True(t, probeScope.failed)

	// First error wins, later ones are dropped; failed stays set.
	e1 := errors.New("first")
	e2 := errors.New("second")
	markTxFailed(ci, e1)
	markTxFailed(ci, e2)
	s.mu.Lock()
	failed, first = s.failed, s.firstErr
	s.mu.Unlock()
	assert.True(t, failed)
	assert.Equal(t, e1, first, "only the FIRST error is recorded")
}

// TestMarkTxFailedNoOpGuards: nil instance and an instance with no scope anywhere up the chain.
func TestMarkTxFailedNoOpGuards(t *testing.T) {
	assert.NotPanics(t, func() { markTxFailed(nil, errors.New("x")) })
	assert.NotPanics(t, func() { markTxFailed(&Instance{}, errors.New("x")) })
}

// TestMarkTxFailedIsConcurrencySafe hammers the latch from several goroutines (-race).
func TestMarkTxFailedIsConcurrencySafe(t *testing.T) {
	ci, s := newScopedInst(&fakeFin{})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				markTxFailed(ci, nil)
			} else {
				markTxFailed(ci, fmt.Errorf("e%d", n))
			}
		}(i)
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	assert.True(t, s.failed)
}

// TestTxScopeOwnerWalksUpAndOwnedDoesNot pins the Y1 distinction directly.
func TestTxScopeOwnerWalksUpAndOwnedDoesNot(t *testing.T) {
	top, s := newScopedInst(&fakeFin{})
	mid := &Instance{host: &TaskInst{flowInst: top}}
	leaf := &Instance{host: &TaskInst{flowInst: mid}}

	assert.Same(t, s, leaf.txScopeOwner(), "txScopeOwner walks up the host chain")
	assert.Same(t, s, mid.txScopeOwner())
	assert.Same(t, s, top.txScopeOwner(), "inst itself counts first")

	assert.Nil(t, leaf.txScopeOwned(), "txScopeOwned must NOT walk")
	assert.Nil(t, mid.txScopeOwned())
	assert.Same(t, s, top.txScopeOwned())

	// nil receiver / non-TaskInst host both terminate the walk.
	var nilInst *Instance
	assert.Nil(t, nilInst.txScopeOwned())
	assert.Nil(t, (&Instance{host: "not-a-task-inst"}).txScopeOwner())
	assert.Nil(t, (&Instance{}).txScopeOwner())
}

// TestTxScopeOwnerTerminatesOnCyclicHostChain: a corrupt host chain must not spin forever.
func TestTxScopeOwnerTerminatesOnCyclicHostChain(t *testing.T) {
	i1 := &Instance{}
	i2 := &Instance{}
	i1.host = &TaskInst{flowInst: i2}
	i2.host = &TaskInst{flowInst: i1}

	done := make(chan *txScope, 1)
	go func() { done <- i1.txScopeOwner() }()

	select {
	case got := <-done:
		assert.Nil(t, got, "a cycle with no scope must terminate and report nil")
	case <-time.After(10 * time.Second):
		t.Fatal("txScopeOwner did not terminate on a cyclic host chain")
	}
}

// TestTxScopeOwnerIsHopBounded documents that the walk gives up past its hop limit rather than
// scanning an unbounded chain.
func TestTxScopeOwnerIsHopBounded(t *testing.T) {
	top, s := newScopedInst(&fakeFin{})

	// 8 hops: found.
	cur := top
	for i := 0; i < 8; i++ {
		cur = &Instance{host: &TaskInst{flowInst: cur}}
	}
	assert.Same(t, s, cur.txScopeOwner())

	// 200 hops: past the bound, so nil.
	deep := top
	for i := 0; i < 200; i++ {
		deep = &Instance{host: &TaskInst{flowInst: deep}}
	}
	assert.Nil(t, deep.txScopeOwner(), "the walk is hop-bounded")
}

// ---------------------------------------------------------------------------
// D14 - error types
// ---------------------------------------------------------------------------

// TestRollbackErrorIsRetriableActivityError pins D14's rollback half: the subflow activity must be
// retriable with a FRESH transaction, so the error has to be an *activity.Error with Retriable().
func TestRollbackErrorIsRetriableActivityError(t *testing.T) {
	cause := errors.New("inner failure")
	rbErr := errors.New("rollback also failed")

	err := newTxRollbackError("conn-42", cause, rbErr, false)

	var ae *activity.Error
	assert.True(t, errors.As(error(err), &ae), "a rollback must be an *activity.Error")
	assert.Equal(t, CodeTxRolledBack, ae.Code())
	assert.Equal(t, "SUBFLOW-TX-001", ae.Code())
	assert.True(t, ae.Retriable(), "D14: a rollback is safe to retry - nothing was committed")

	data, ok := ae.Data().(map[string]interface{})
	assert.True(t, ok, "Data() must be a map so the user's error branch can read it")
	assert.Equal(t, "conn-42", data["connectionId"])
	assert.Equal(t, false, data["downgraded"])
	assert.Equal(t, cause.Error(), data["cause"])
	assert.Equal(t, rbErr.Error(), data["rollbackError"])

	assert.Contains(t, ae.Error(), "SUBFLOW-TX-001")
	assert.Contains(t, ae.Error(), "conn-42")
	assert.Contains(t, ae.Error(), cause.Error())
	assert.Contains(t, ae.Error(), rbErr.Error())
}

// TestCommitFailureErrorIsNotAnActivityError is a DELIBERATE GUARD - see D14.
//
// If this test fails, a commit failure has become retriable and every in-doubt commit will be
// replayed: model/simple/taskbehavior.go's PostEval path retries any *activity.Error WITHOUT
// consulting Retriable(), and a failed COMMIT is IN DOUBT - the server may have applied the
// transaction and lost the acknowledgement. Replaying would double-apply every write in the
// subflow plus any non-transactional side effect it performed. Do not "improve" TxCommitError
// into an activity.NewActivityError.
func TestCommitFailureErrorIsNotAnActivityError(t *testing.T) {
	inner := errors.New("connection reset during commit")
	var err error = &TxCommitError{ConnID: "conn-7", Err: inner}

	_, isActivityErr := err.(*activity.Error)
	assert.False(t, isActivityErr,
		"D14: a commit failure MUST NOT be an *activity.Error, or taskbehavior.go will retry an in-doubt commit")

	var ae *activity.Error
	assert.False(t, errors.As(err, &ae),
		"D14: errors.As must not reach an *activity.Error through a commit failure either")

	var ce *TxCommitError
	assert.True(t, errors.As(err, &ce), "errors.As into *TxCommitError must succeed")
	assert.Equal(t, "conn-7", ce.ConnID)

	assert.True(t, errors.Is(err, inner), "the driver error stays unwrappable")
	assert.Equal(t, inner, errors.Unwrap(err))

	assert.Contains(t, err.Error(), CodeTxCommitFailed)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-003")
	assert.Contains(t, err.Error(), "conn-7")
	assert.Contains(t, err.Error(), inner.Error())
}

// TestIsTxDowngraded covers the downgrade sentinel and its wrapping.
func TestIsTxDowngraded(t *testing.T) {
	assert.True(t, IsTxDowngraded(ErrTxDowngraded))
	assert.True(t, IsTxDowngraded(fmt.Errorf("wrapped: %w", ErrTxDowngraded)))
	assert.False(t, IsTxDowngraded(errors.New("something else")))
	assert.False(t, IsTxDowngraded(nil))
}

// TestTxResumeErrShape covers the D10 tripwire message.
func TestTxResumeErrShape(t *testing.T) {
	err := txResumeErr("inst-1", "TxSubFlow")
	assert.Contains(t, err.Error(), CodeTxNotResumable)
	assert.Contains(t, err.Error(), "inst-1")
	assert.Contains(t, err.Error(), "TxSubFlow")
}

// ---------------------------------------------------------------------------
// apply()
// ---------------------------------------------------------------------------

// TestApplyCommitPath: confirm() true => Commit called, Rollback NOT called, nil returned.
func TestApplyCommitPath(t *testing.T) {
	fin := &fakeFin{}
	ci, _ := newScopedInst(fin)

	v := decideTx(ci, true, nil)
	assert.NotNil(t, v)
	assert.True(t, v.commit)

	assert.Nil(t, v.apply(), "a clean commit returns nil - and ONLY a clean commit returns nil")
	assert.Equal(t, []string{"commit"}, fin.Events())

	fin.mu.Lock()
	seen := fin.confirmSeen
	fin.mu.Unlock()
	assert.NotNil(t, seen)
	assert.True(t, *seen, "confirm() must report OK when nothing failed")
}

// TestApplyCommitDowngradedByConfirm: a sibling branch fails inside the commit window, so confirm()
// vetoes; the finalizer rolls back and returns ErrTxDowngraded, and apply() converts that into a
// RETRIABLE rollback error carrying downgraded == true.
func TestApplyCommitDowngradedByConfirm(t *testing.T) {
	fin := &fakeFin{}
	ci, s := newScopedInst(fin)

	v := decideTx(ci, true, nil)
	assert.NotNil(t, v)
	assert.True(t, v.commit, "the verdict is decided BEFORE the late failure arrives")

	late := errors.New("sibling branch failed during the commit")
	// The late failure lands after decideTx and before/while Commit runs - exactly the race the
	// confirm() recheck exists for.
	fin.beforeCommit = func() { markTxFailed(ci, late) }

	err := v.apply()

	assert.Equal(t, []string{"rollback:downgraded"}, fin.Events(), "the commit was downgraded to a rollback")

	var ae *activity.Error
	assert.True(t, errors.As(err, &ae), "a downgrade surfaces as a retriable rollback error")
	assert.Equal(t, CodeTxRolledBack, ae.Code())
	assert.True(t, ae.Retriable())

	data, ok := ae.Data().(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, true, data["downgraded"], "the error must say it was a downgraded commit")
	assert.Equal(t, late.Error(), data["cause"], "the late failure is reported as the cause")

	s.mu.Lock()
	assert.True(t, s.failed)
	s.mu.Unlock()
}

// TestApplyCommitFailureDoesNotRollBack: a failed COMMIT returns *TxCommitError and MUST NOT then
// call Rollback - the driver has already resolved the transaction and a rollback error would mask
// the real one.
func TestApplyCommitFailureDoesNotRollBack(t *testing.T) {
	driverErr := errors.New("write conflict at commit")
	fin := &fakeFin{commitErr: driverErr}
	ci, _ := newScopedInst(fin)

	v := decideTx(ci, true, nil)
	assert.NotNil(t, v)

	err := v.apply()

	assert.Equal(t, []string{"commit"}, fin.Events(), "Rollback must NOT be called after a failed commit")

	var ce *TxCommitError
	assert.True(t, errors.As(err, &ce))
	assert.Equal(t, "conn-unit", ce.ConnID)
	assert.Equal(t, driverErr, ce.Err)

	var ae *activity.Error
	assert.False(t, errors.As(err, &ae), "D14 again: never retriable")
}

// TestApplyRollbackPath: the plain rollback verdict, including a rollback that itself fails.
func TestApplyRollbackPath(t *testing.T) {
	cause := errors.New("task failed")

	fin := &fakeFin{}
	ci, _ := newScopedInst(fin)
	err := decideTx(ci, false, cause).apply()
	assert.Equal(t, []string{"rollback"}, fin.Events())

	var ae *activity.Error
	assert.True(t, errors.As(err, &ae))
	assert.Equal(t, CodeTxRolledBack, ae.Code())
	assert.True(t, ae.Retriable())
	data := ae.Data().(map[string]interface{})
	assert.Equal(t, false, data["downgraded"])
	assert.Equal(t, cause.Error(), data["cause"])
	assert.NotContains(t, data, "rollbackError")

	// A rollback that itself fails is still reported as a rollback, with the driver error attached.
	rbErr := errors.New("rollback: connection is dead")
	fin2 := &fakeFin{rollbackErr: rbErr}
	ci2, _ := newScopedInst(fin2)
	err2 := decideTx(ci2, false, cause).apply()
	assert.Equal(t, []string{"rollback"}, fin2.Events())
	var ae2 *activity.Error
	assert.True(t, errors.As(err2, &ae2))
	assert.Equal(t, rbErr.Error(), ae2.Data().(map[string]interface{})["rollbackError"])
}

// TestFinishTxReleasesStateLockDuringApply: apply() performs I/O and MUST NOT run with the
// instance state lock held, or a commit that blocks on the operation lock deadlocks the flow.
func TestFinishTxReleasesStateLockDuringApply(t *testing.T) {
	master := newConcurrencyTestInstance(t, true)
	assert.NotNil(t, master.stateLock, "this test is meaningless without a real state lock")

	fin := &fakeFin{}
	ci := &Instance{master: master, txScope: &txScope{fin: fin, connID: "conn-lock", logger: log.RootLogger()}}
	master.txScopeActive.Add(1)

	lockWasFree := make(chan bool, 1)
	fin.beforeCommit = func() {
		got := make(chan struct{})
		go func() {
			master.lockState()
			master.unlockState()
			close(got)
		}()
		select {
		case <-got:
			lockWasFree <- true
		case <-time.After(3 * time.Second):
			lockWasFree <- false
		}
	}

	master.lockState()
	err := finishTx(ci, true, nil, true) // lockHeld = true, the truth
	master.unlockState()

	assert.Nil(t, err)
	assert.True(t, <-lockWasFree, "the state lock must be RELEASED while the finalizer runs")

	// And the lock is re-acquired on the way out, so the caller's Unlock above was legal.
	assert.NotPanics(t, func() { master.lockState(); master.unlockState() })
}

// TestWithStateUnlockedReacquiresOnPanic ensures the lock is restored even if fn panics.
func TestWithStateUnlockedReacquiresOnPanic(t *testing.T) {
	master := newConcurrencyTestInstance(t, true)

	master.lockState()
	assert.Panics(t, func() {
		master.Instance.withStateUnlocked(true, func() { panic("boom") })
	})
	// The deferred lockState re-acquired it; releasing must be legal and must not block.
	master.unlockState()

	done := make(chan struct{})
	go func() { master.lockState(); master.unlockState(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("withStateUnlocked did not re-acquire/leave the state lock in a sane state")
	}
}

// ---------------------------------------------------------------------------
// RollbackOpenTransactions - the leak sweep
// ---------------------------------------------------------------------------

// TestRollbackOpenTransactionsIsFreeWhenUnused: the sweep is on the tail of EVERY flow, so with no
// transactional subflow it must neither scan nor allocate.
func TestRollbackOpenTransactionsIsFreeWhenUnused(t *testing.T) {
	assert.NotPanics(t, func() { RollbackOpenTransactions(nil) })

	master := newConcurrencyTestInstance(t, true)
	taskA, _ := master.FindOrCreateTaskInst(master.flowDef.GetTask("LogStart"))
	// A subflow exists, but no transaction does.
	_ = master.newEmbeddedInstance(taskA, "", getDef(), context.Background(), nil)
	assert.Equal(t, int32(0), master.txScopeActive.Load())

	allocs := testing.AllocsPerRun(200, func() { RollbackOpenTransactions(master) })
	assert.Zero(t, allocs, "the sweep must be allocation-free when txScopeActive == 0")
}

// TestRollbackOpenTransactionsRollsBackAndLogs: reaching the sweep means a terminal transition was
// missed, so it rolls back AND reports it at ERROR.
func TestRollbackOpenTransactionsRollsBackAndLogs(t *testing.T) {
	fin := &fakeFin{}
	tr := newTxTree(t, true, fin)

	RollbackOpenTransactions(tr.master)

	assert.Equal(t, []string{"rollback"}, fin.Events(), "an open transaction must be swept")
	assert.True(t, tr.scope.done)
	assert.Equal(t, int32(0), tr.master.txScopeActive.Load())

	errs := tr.logger.Errors()
	assert.True(t, containsSubstr(t, errs, "still open when the flow stopped"),
		"the sweep must log the missed terminal transition, got: %v", errs)
	assert.True(t, containsSubstr(t, errs, "conn-y1"), "the log must name the connection, got: %v", errs)

	// Idempotent: a second sweep finds nothing (txScopeActive is back to zero).
	RollbackOpenTransactions(tr.master)
	assert.Equal(t, []string{"rollback"}, fin.Events())
}

// TestRollbackOpenTransactionsRacesDecideTxExactlyOnce: the sweep and a real terminal transition
// can run concurrently; the scope must still be claimed exactly once. Meaningful under -race.
func TestRollbackOpenTransactionsRacesDecideTxExactlyOnce(t *testing.T) {
	for i := 0; i < 100; i++ {
		fin := &fakeFin{}
		tr := newTxTree(t, true, fin)

		var wg sync.WaitGroup
		start := make(chan struct{})

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			RollbackOpenTransactions(tr.master)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if v := decideTx(tr.a, true, nil); v != nil {
				_ = v.apply()
			}
		}()

		close(start)
		wg.Wait()

		assert.Len(t, fin.Events(), 1, "the finalizer must run exactly once, whoever wins")
		assert.Equal(t, int32(0), tr.master.txScopeActive.Load())
	}
}

// ---------------------------------------------------------------------------
// D10 - resume marking
// ---------------------------------------------------------------------------

// TestTxInFlightRoundTrip covers set/clear/read through the attrs lock.
func TestTxInFlightRoundTrip(t *testing.T) {
	forEachMode(t, func(t *testing.T, concurrent bool) {
		master := newConcurrencyTestInstance(t, concurrent)

		assert.False(t, isTxInFlight(nil))
		assert.False(t, isTxInFlight(master.Instance), "absent by default")

		setTxInFlight(master.Instance, true)
		assert.True(t, isTxInFlight(master.Instance))

		// Instance.Delete is a no-op, so the marker is CLEARED by setting false, never by deleting.
		setTxInFlight(master.Instance, false)
		assert.False(t, isTxInFlight(master.Instance))

		assert.NotPanics(t, func() { setTxInFlight(nil, true) })
	})
}

// TestRejectIfTxInFlight covers the master, the subflow scan, and the clean case.
func TestRejectIfTxInFlight(t *testing.T) {
	assert.Nil(t, RejectIfTxInFlight(nil))

	// Clean.
	fin := &fakeFin{}
	tr := newTxTree(t, true, fin)
	assert.Nil(t, RejectIfTxInFlight(tr.master), "nothing marked => resumable")

	// Master marked.
	setTxInFlight(tr.master.Instance, true)
	err := RejectIfTxInFlight(tr.master)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), CodeTxNotResumable)
	assert.Contains(t, err.Error(), tr.master.ID())
	setTxInFlight(tr.master.Instance, false)
	assert.Nil(t, RejectIfTxInFlight(tr.master))

	// A subflow marked.
	setTxInFlight(tr.a, true)
	err = RejectIfTxInFlight(tr.master)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), CodeTxNotResumable)
	assert.Contains(t, err.Error(), tr.a.Name())
	setTxInFlight(tr.a, false)
	assert.Nil(t, RejectIfTxInFlight(tr.master))
}

// TestStampTxInFlight covers the record-side marker and its once-only warning.
func TestStampTxInFlight(t *testing.T) {
	fin := &fakeFin{}
	tr := newTxTree(t, true, fin) // txScopeActive == 1

	tr.master.stampTxInFlight()
	assert.True(t, isTxInFlight(tr.master.Instance))
	assert.True(t, containsSubstr(t, tr.logger.Warns(), "NOT resumable"),
		"recording state mid-transaction must warn, got: %v", tr.logger.Warns())

	warnsAfterFirst := len(tr.logger.Warns())
	tr.master.stampTxInFlight()
	assert.Len(t, tr.logger.Warns(), warnsAfterFirst, "the warning is emitted once per instance")

	// Transaction finished: the next stamp CLEARS the marker. Note it takes decideTx AND apply -
	// the active count deliberately stays up until the round trip completes, so that a task still
	// in flight keeps seeing the transaction registry.
	v := decideTx(tr.a, true, nil)
	assert.NotNil(t, v)
	tr.master.stampTxInFlight()
	assert.True(t, isTxInFlight(tr.master.Instance),
		"still in flight between claim and commit: a checkpoint taken here is genuinely not resumable")

	assert.NoError(t, v.apply())
	tr.master.stampTxInFlight()
	assert.False(t, isTxInFlight(tr.master.Instance))

	// And with nothing set and nothing to clear it is a pure no-op.
	before := len(tr.logger.Warns())
	tr.master.stampTxInFlight()
	assert.False(t, isTxInFlight(tr.master.Instance))
	assert.Len(t, tr.logger.Warns(), before)
}

// TestReturnErrorLocked covers the attrs-locked read of Instance.returnError.
func TestReturnErrorLocked(t *testing.T) {
	var nilInst *Instance
	assert.Nil(t, nilInst.returnErrorLocked())

	master := newConcurrencyTestInstance(t, true)
	assert.Nil(t, master.Instance.returnErrorLocked())

	boom := errors.New("returned failure")
	master.Instance.Return(nil, boom)
	assert.Equal(t, boom, master.Instance.returnErrorLocked())
}

// ---------------------------------------------------------------------------
// Context propagator
// ---------------------------------------------------------------------------

type txCtxKey string

// TestTxContextPropagatorDefaultIsIdentity: flow/instance is linked into EVERY Flogo binary, so
// with the subflow activity absent the propagator must be the identity and must allocate nothing.
func TestTxContextPropagatorDefaultIsIdentity(t *testing.T) {
	t.Cleanup(func() { SetTxContextPropagator(nil) })

	src := context.WithValue(context.Background(), txCtxKey("registry"), "present")
	dst, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Default (restored explicitly so the test does not depend on run order).
	SetTxContextPropagator(nil)
	got := propagateTxCtx(src, dst)
	assert.True(t, got == dst, "the default propagator must return dst IDENTICALLY")
	assert.Nil(t, got.Value(txCtxKey("registry")), "the default must not copy anything across")

	allocs := testing.AllocsPerRun(500, func() { _ = propagateTxCtx(src, dst) })
	assert.Zero(t, allocs, "the unused feature must allocate nothing on this path")

	// A real propagator is honoured...
	SetTxContextPropagator(func(s, d context.Context) context.Context {
		return context.WithValue(d, txCtxKey("registry"), s.Value(txCtxKey("registry")))
	})
	got = propagateTxCtx(src, dst)
	assert.False(t, got == dst)
	assert.Equal(t, "present", got.Value(txCtxKey("registry")))
	assert.Nil(t, got.Err(), "the destination's cancellation is preserved, not replaced")

	// ...and nil restores the identity default.
	SetTxContextPropagator(nil)
	got = propagateTxCtx(src, dst)
	assert.True(t, got == dst, "SetTxContextPropagator(nil) must restore the identity")
	assert.Nil(t, got.Value(txCtxKey("registry")))
}

// ---------------------------------------------------------------------------
// tiny helper
// ---------------------------------------------------------------------------

func require(t *testing.T, cond bool, msg string) {
	t.Helper()
	if !cond {
		t.Fatal(msg)
	}
}
