package instance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/state"
	"github.com/stretchr/testify/assert"
)

// armedLogger runs a hook once, from the next Debugf or Error call, to fail the code under test at that point.
type armedLogger struct {
	log.Logger
	onDebugf, onError func()
}

func (l *armedLogger) Debugf(template string, args ...interface{}) {
	if f := l.onDebugf; f != nil {
		l.onDebugf = nil
		f()
	}
	l.Logger.Debugf(template, args...)
}

func (l *armedLogger) Error(args ...interface{}) {
	if f := l.onError; f != nil {
		l.onError = nil
		f()
	}
	l.Logger.Error(args...)
}

type scriptedBehavior struct {
	eval func() (model.EvalResult, error)
}

func (b *scriptedBehavior) Enter(model.TaskContext) model.EnterResult { return model.EREval }
func (b *scriptedBehavior) Eval(model.TaskContext) (model.EvalResult, error) {
	return b.eval()
}
func (b *scriptedBehavior) PostEval(model.TaskContext) (model.EvalResult, error) {
	return model.EvalDone, nil
}
func (b *scriptedBehavior) Done(model.TaskContext) (bool, []*model.TaskEntry, error) {
	return false, nil, nil
}
func (b *scriptedBehavior) Skip(model.TaskContext) (bool, []*model.TaskEntry, bool) {
	return false, nil, false
}
func (b *scriptedBehavior) Error(model.TaskContext, error) (bool, []*model.TaskEntry) {
	return false, nil
}

func newConcurrentInstance(t *testing.T, logger log.Logger) (*IndependentInstance, *TaskInst) {
	t.Helper()
	t.Setenv("FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY", "true")
	inst, err := NewIndependentInstance("concurrent-recover", "", getDef(), nil, logger, context.Background())
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.NotNil(t, inst.stateLock)
	inst.concurCtx, inst.concurCancel = context.WithCancel(context.Background())
	t.Cleanup(inst.concurCancel)
	return inst, NewTaskInst(inst.Instance, inst.flowDef.GetTask("LogStart"))
}

func TestExecTaskConcurrentRecoverAndEvalCtx(t *testing.T) {
	t.Run("panic during evaluation clears evalCtx", func(t *testing.T) {
		inst, ti := newConcurrentInstance(t, log.RootLogger())
		behavior := &scriptedBehavior{eval: func() (model.EvalResult, error) { panic("evaluation failed") }}
		inst.execTaskConcurrent(behavior, ti, nil, time.Now())
		assert.Nil(t, ti.evalCtx, "a task must not keep the group context after a failed evaluation")
	})

	// Once a repeat is rescheduled another worker may own the task; a later panic must not wipe its context.
	t.Run("panic after a reschedule keeps the next worker's evalCtx", func(t *testing.T) {
		l := &armedLogger{Logger: log.RootLogger()}
		inst, ti := newConcurrentInstance(t, l)
		next, cancel := context.WithCancel(context.Background())
		defer cancel()
		l.onDebugf = func() {
			ti.evalCtx = next
			panic("tail failed")
		}
		behavior := &scriptedBehavior{eval: func() (model.EvalResult, error) { return model.EvalRepeat, nil }}
		inst.execTaskConcurrent(behavior, ti, nil, time.Now())
		assert.Equal(t, next, ti.evalCtx, "the recover wiped the context of the worker that took the task over")
	})

	t.Run("normal evaluation clears evalCtx", func(t *testing.T) {
		inst, ti := newConcurrentInstance(t, log.RootLogger())
		var during context.Context
		behavior := &scriptedBehavior{eval: func() (model.EvalResult, error) {
			during = ti.evalCtx
			return model.EvalWait, nil
		}}
		inst.execTaskConcurrent(behavior, ti, nil, time.Now())
		assert.NotNil(t, during, "the evaluation must run with the group context")
		assert.Nil(t, ti.evalCtx, "a task must not keep the group context after its evaluation")
	})
}

// A panic while a latched branch error is handled must not leave the state lock held.
func TestRunConcurrentUnlocksWhenErrorHandlingPanics(t *testing.T) {
	l := &armedLogger{Logger: log.RootLogger()}
	inst, _ := newConcurrentInstance(t, l)
	l.onError = func() { panic("error handling failed") }
	inst.latchConcurrentError(inst.Instance, errors.New("branch failed"))

	func() {
		defer func() { _ = recover() }()
		inst.RunConcurrent(0, 10, nil)
	}()

	locked := inst.stateLock.TryLock()
	if locked {
		inst.stateLock.Unlock()
	}
	assert.True(t, locked, "the state lock is still held after the panic")
}

type noopRecorder struct{}

func (noopRecorder) RecordStart(*state.FlowState) error   { return nil }
func (noopRecorder) RecordSnapshot(*state.Snapshot) error { return nil }
func (noopRecorder) RecordStep(*state.Step) error         { return nil }
func (noopRecorder) RecordDone(*state.FlowState) error    { return nil }

// A latched branch error fails the flow once all branches drained, with state recording on.
func TestRunConcurrentHandlesLatchedErrorWithRecorder(t *testing.T) {
	inst, _ := newConcurrentInstance(t, log.RootLogger())
	inst.latchConcurrentError(inst.Instance, errors.New("branch failed"))
	inst.RunConcurrent(0, 10, noopRecorder{})
	assert.Equal(t, model.FlowStatusFailed, inst.Status())
	locked := inst.stateLock.TryLock()
	if locked {
		inst.stateLock.Unlock()
	}
	assert.True(t, locked)
}
