package instance

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
	flowSupport "github.com/project-flogo/flow/support"
	"github.com/stretchr/testify/assert"
)

// outputDefJSON is a minimal flow definition with output metadata, used to exercise the
// lazy returnData build in GetReturnData.
const outputDefJSON = `{
  "name": "OutFlow",
  "metadata": { "output": [ {"name":"result","type":"string","value":"built"} ] },
  "tasks": [ {"id":"LogStart","activity":{"ref":"log","input":{"message":"hi"}}} ],
  "links": []
}`

func newInstanceFromJSON(t *testing.T, defJSON string, concurrent bool) *IndependentInstance {
	t.Helper()
	if concurrent {
		t.Setenv(flowSupport.ConcurrentExecution, "true")
	} else {
		t.Setenv(flowSupport.ConcurrentExecution, "false")
	}
	defRep := &definition.DefinitionRep{}
	err := json.Unmarshal([]byte(defJSON), defRep)
	assert.Nil(t, err)
	def, err := definition.NewDefinition(defRep)
	assert.Nil(t, err)
	inst, err := NewIndependentInstance("test-json-"+strconv.Itoa(int(rngCounter.Add(1))), "", def, nil, log.RootLogger(), context.Background())
	assert.Nil(t, err)
	return inst
}

// newConcurrencyTestInstance builds an IndependentInstance with the concurrency flag set
// to the desired value, so the per-instance locks are (or are not) created accordingly.
func newConcurrencyTestInstance(t *testing.T, concurrent bool) *IndependentInstance {
	t.Helper()
	if concurrent {
		t.Setenv(flowSupport.ConcurrentExecution, "true")
	} else {
		t.Setenv(flowSupport.ConcurrentExecution, "false")
	}
	inst, err := NewIndependentInstance("test-"+strconv.Itoa(int(rngCounter.Add(1))), "", getDef(), nil, log.RootLogger(), context.Background())
	assert.Nil(t, err)
	assert.NotNil(t, inst)
	return inst
}

// rngCounter gives each test instance a unique id without using Math/rand or time.
var rngCounter atomicCounter

type atomicCounter struct {
	mu sync.Mutex
	n  int64
}

func (c *atomicCounter) Add(d int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n += d
	return c.n
}

type mockResultHandler struct {
	data map[string]interface{}
	err  error
}

func (m *mockResultHandler) HandleResult(d map[string]interface{}, e error) {
	m.data = d
	m.err = e
}

func (m *mockResultHandler) Done() {}

// TestIsConcurrentTaskExcutionEnabled verifies the flag reader delegates to the support
// env helper (single source of truth shared with the change tracker).
func TestIsConcurrentTaskExcutionEnabled(t *testing.T) {
	t.Setenv(flowSupport.ConcurrentExecution, "true")
	assert.True(t, IsConcurrentTaskExcutionEnabled())
	t.Setenv(flowSupport.ConcurrentExecution, "false")
	assert.False(t, IsConcurrentTaskExcutionEnabled())
}

// TestConcurrencyLocksCreatedByFlag verifies the per-instance locks exist only when the
// concurrent-execution flag is enabled (so the sequential path stays lock-free).
func TestConcurrencyLocksCreatedByFlag(t *testing.T) {
	off := newConcurrencyTestInstance(t, false)
	assert.Nil(t, off.stateLock, "stateLock must be nil when concurrency is off")
	assert.Nil(t, off.attrsLock, "attrsLock must be nil when concurrency is off")

	on := newConcurrencyTestInstance(t, true)
	assert.NotNil(t, on.stateLock, "stateLock must be set when concurrency is on")
	assert.NotNil(t, on.attrsLock, "attrsLock must be set when concurrency is on")
}

// TestLockHelpersNoOpWhenNil ensures the lock helpers are safe no-ops in sequential mode.
func TestLockHelpersNoOpWhenNil(t *testing.T) {
	inst := newConcurrencyTestInstance(t, false)
	assert.NotPanics(t, func() {
		inst.lockState()
		inst.unlockState()
		inst.lockAttrs()
		inst.unlockAttrs()
		inst.rlockAttrs()
		inst.runlockAttrs()
	})
}

// TestLockHelpersWhenEnabled ensures the lock helpers acquire and release cleanly (no
// deadlock on repeated lock/unlock) when concurrency is enabled.
func TestLockHelpersWhenEnabled(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	assert.NotPanics(t, func() {
		inst.lockState()
		inst.unlockState()
		inst.lockState()
		inst.unlockState()

		inst.lockAttrs()
		inst.unlockAttrs()
		inst.rlockAttrs()
		inst.runlockAttrs()
	})
}

// TestLatchConcurrentError verifies first-error-wins latching, sibling cancellation, and
// that takeLatchedError clears the latch.
func TestLatchConcurrentError(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)

	cancelled := false
	inst.concurCancel = func() { cancelled = true }

	err1 := errors.New("first")
	err2 := errors.New("second")
	ci := inst.Instance

	inst.latchConcurrentError(ci, err1)
	assert.True(t, inst.errLatchedLoad(), "error should be latched")
	assert.True(t, cancelled, "siblings should be cancelled on first error")

	// Second error is ignored; first wins.
	inst.latchConcurrentError(ci, err2)

	gotCi, gotErr := inst.takeLatchedError()
	assert.Equal(t, ci, gotCi)
	assert.Equal(t, err1, gotErr)

	// Latch is cleared after take.
	assert.False(t, inst.errLatchedLoad())
	gotCi2, gotErr2 := inst.takeLatchedError()
	assert.Nil(t, gotCi2)
	assert.Nil(t, gotErr2)
}

// TestMarkTerminatedIfDone verifies the terminal flag flips only at a terminal status.
func TestMarkTerminatedIfDone(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)

	inst.status = model.FlowStatusActive
	inst.markTerminatedIfDone()
	assert.False(t, inst.concurTerminated.Load())

	inst.status = model.FlowStatusCompleted
	inst.markTerminatedIfDone()
	assert.True(t, inst.concurTerminated.Load())
}

// TestHandleGlobalErrorDeferred verifies that while the pool is running, HandleGlobalError
// latches the error instead of executing (drain-then-fail), leaving status untouched.
func TestHandleGlobalErrorDeferred(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	inst.SetStatus(model.FlowStatusActive)
	inst.concurCancel = func() {}
	inst.deferErrors.Store(true)

	inst.HandleGlobalError(inst.Instance, errors.New("boom"))

	assert.True(t, inst.errLatchedLoad(), "deferred error should be latched")
	assert.Equal(t, model.FlowStatusActive, inst.Status(), "status must not change while deferring")
}

// TestSetGetValueConcurrentSafe exercises the attrs lock path under concurrent writers.
func TestSetGetValueConcurrentSafe(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	inst.SetStatus(model.FlowStatusActive)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "k" + strconv.Itoa(n)
			_ = inst.SetValue(key, n)
			_, _ = inst.GetValue(key)
		}(i)
	}
	wg.Wait()

	v, ok := inst.GetValue("k3")
	assert.True(t, ok)
	assert.Equal(t, 3, v)
}

// TestReturnGuarded verifies Return populates forceCompletion/returnData/returnError under
// the locks and that GetReturnData reads them back.
func TestReturnGuarded(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	rd := map[string]interface{}{"a": 1}

	inst.Return(rd, nil)
	assert.True(t, inst.forceCompletion)

	got, err := inst.GetReturnData()
	assert.Nil(t, err)
	assert.Equal(t, rd, got)
}

// TestReplyGuarded verifies Reply stores returnData (under the attrs lock) and forwards to
// the result handler.
func TestReplyGuarded(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	h := &mockResultHandler{}
	inst.SetResultHandler(h)

	rd := map[string]interface{}{"x": 2}
	inst.Reply(rd, nil)

	assert.Equal(t, rd, h.data)
	got, _ := inst.GetReturnData()
	assert.Equal(t, rd, got)
}

// TestScheduleEvalAtomicCounter verifies the work-item counter increments atomically and a
// work item is enqueued.
func TestScheduleEvalAtomicCounter(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	task := inst.flowDef.GetTask("LogStart")
	assert.NotNil(t, task)

	ti, _ := inst.FindOrCreateTaskInst(task)
	before := inst.wiCounter.Load()
	inst.scheduleEval(ti)

	assert.Equal(t, before+1, inst.wiCounter.Load())
	assert.Equal(t, 1, inst.workItemQueue.Size())
}

// stubBehavior is a minimal model.TaskBehavior used to drive evalTaskBehavior /
// handleEvalResult directly (white-box) without importing the simple model.
type stubBehavior struct {
	evalResult  model.EvalResult
	evalErr     error
	panicOnEval bool
}

func (b *stubBehavior) Enter(model.TaskContext) model.EnterResult { return model.EREval }
func (b *stubBehavior) Eval(model.TaskContext) (model.EvalResult, error) {
	if b.panicOnEval {
		panic("boom in eval")
	}
	return b.evalResult, b.evalErr
}
func (b *stubBehavior) PostEval(model.TaskContext) (model.EvalResult, error) {
	return b.evalResult, b.evalErr
}
func (b *stubBehavior) Done(model.TaskContext) (bool, []*model.TaskEntry, error) {
	return true, nil, nil
}
func (b *stubBehavior) Skip(model.TaskContext) (bool, []*model.TaskEntry, bool) {
	return true, nil, false
}
func (b *stubBehavior) Error(model.TaskContext, error) (bool, []*model.TaskEntry) {
	return false, nil
}

// TestEvalTaskBehaviorPaths covers the shared evaluation helper: the default (Eval) path,
// the resumed (PostEval) path, and the already-skipped short-circuit.
func TestEvalTaskBehaviorPaths(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	task := inst.flowDef.GetTask("LogStart")
	ti, _ := inst.FindOrCreateTaskInst(task)
	b := &stubBehavior{evalResult: model.EvalDone}

	ti.SetStatus(model.TaskStatusReady)
	er, err, skipped := inst.evalTaskBehavior(b, ti)
	assert.False(t, skipped)
	assert.Nil(t, err)
	assert.Equal(t, model.EvalDone, er)

	ti.SetStatus(model.TaskStatusWaiting)
	er, _, skipped = inst.evalTaskBehavior(b, ti)
	assert.False(t, skipped)
	assert.Equal(t, model.EvalDone, er)

	ti.SetStatus(model.TaskStatusSkipped)
	_, _, skipped = inst.evalTaskBehavior(b, ti)
	assert.True(t, skipped)
}

// TestHandleEvalResultPaths covers the shared result-handling helper across all outcomes.
func TestHandleEvalResultPaths(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	inst.SetStatus(model.FlowStatusActive)
	b := &stubBehavior{}

	ti, _ := inst.FindOrCreateTaskInst(inst.flowDef.GetTask("LogStart"))

	inst.handleEvalResult(b, ti, model.EvalWait)
	assert.Equal(t, model.TaskStatusWaiting, ti.Status())

	inst.handleEvalResult(b, ti, model.EvalFail)
	assert.Equal(t, model.TaskStatusFailed, ti.Status())

	before := inst.workItemQueue.Size()
	inst.handleEvalResult(b, ti, model.EvalRepeat)
	assert.Equal(t, before+1, inst.workItemQueue.Size())

	// EvalDone / EvalSkip both route through handleTaskDone.
	assert.NotPanics(t, func() {
		tiDone, _ := inst.FindOrCreateTaskInst(inst.flowDef.GetTask("LogResult"))
		inst.handleEvalResult(b, tiDone, model.EvalDone)
		tiSkip, _ := inst.FindOrCreateTaskInst(inst.flowDef.GetTask("LogStart"))
		inst.handleEvalResult(b, tiSkip, model.EvalSkip)
	})
}

// TestExecTaskConcurrentRecover covers the defensive panic-recover path in the concurrent
// worker: a panic during evaluation is recovered, latched as a global error (deferred), and
// the worker unwinds cleanly without leaving the state lock held.
func TestExecTaskConcurrentRecover(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	inst.SetStatus(model.FlowStatusActive)
	inst.concurCtx = context.Background()
	inst.concurCancel = func() {}
	inst.deferErrors.Store(true)

	ti, _ := inst.FindOrCreateTaskInst(inst.flowDef.GetTask("LogStart"))
	ti.SetStatus(model.TaskStatusReady)

	assert.NotPanics(t, func() {
		inst.execTaskConcurrent(&stubBehavior{panicOnEval: true}, ti, nil, time.Time{})
	})
	assert.True(t, inst.errLatchedLoad(), "panic must be recovered and latched as a failure")

	// The state lock must have been released (re-acquire would deadlock otherwise).
	done := make(chan struct{})
	go func() { inst.lockState(); inst.unlockState(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("state lock was left held by the recover path")
	}
}

// TestNewEmbeddedInstance covers subflow creation, including the locked subflows-map write
// and the atomic subflow counter.
func TestNewEmbeddedInstance(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	ti, _ := inst.FindOrCreateTaskInst(inst.flowDef.GetTask("LogStart"))

	emb := inst.newEmbeddedInstance(ti, "", inst.flowDef, context.Background(), nil)
	assert.NotNil(t, emb)
	assert.Equal(t, int64(1), inst.subflowCtr.Load())
	assert.NotNil(t, inst.subflows)
	assert.Equal(t, emb, inst.subflows[1])
}

// TestStepID covers the StepID accessor over the atomic counter.
func TestStepID(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	assert.Equal(t, 0, inst.StepID())
	inst.stepID.Add(5)
	assert.Equal(t, 5, inst.StepID())
}

// TestGetReturnDataLazyBuild covers the lazy build of returnData from flow output metadata
// and instance attributes.
func TestGetReturnDataLazyBuild(t *testing.T) {
	inst := newInstanceFromJSON(t, outputDefJSON, true)
	// Set the attr to exercise the "attr exists" branch; the declared output default then
	// overrides it, exercising the value-override branch too.
	_ = inst.SetValue("result", "ok")

	rd, err := inst.GetReturnData()
	assert.Nil(t, err)
	assert.NotNil(t, rd)
	assert.Equal(t, "built", rd["result"])
}

// TestSetValueTracksChanges covers the change-tracking branch of SetValue (active when
// state recording is enabled).
func TestSetValueTracksChanges(t *testing.T) {
	t.Setenv("FLOGO_FLOW_SM_ENDPOINT", "http://localhost:9999")
	inst := newConcurrencyTestInstance(t, true)
	assert.True(t, inst.trackingChanges)

	inst.SetStatus(model.FlowStatusActive)
	err := inst.SetValue("tracked", "yes")
	assert.Nil(t, err)

	v, ok := inst.GetValue("tracked")
	assert.True(t, ok)
	assert.Equal(t, "yes", v)
}

// TestGoContextEvalCtxOverride verifies the per-task context override used for branch
// cancellation, falling back to the flow context when unset.
func TestGoContextEvalCtxOverride(t *testing.T) {
	inst := newConcurrencyTestInstance(t, true)
	task := inst.flowDef.GetTask("LogStart")
	ti, _ := inst.FindOrCreateTaskInst(task)

	// Default: falls back to the flow goContext.
	assert.Equal(t, inst.goContext, ti.GoContext())
	assert.Equal(t, inst.goContext, ti.GetGoContext())

	type ctxKey string
	override := context.WithValue(context.Background(), ctxKey("k"), "v")
	ti.evalCtx = override
	assert.Equal(t, override, ti.GoContext())
	assert.Equal(t, override, ti.GetGoContext())
}
