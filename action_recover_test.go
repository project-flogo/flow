package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/core/support/log"
	coretest "github.com/project-flogo/core/support/test"
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/core/trigger"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/state"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/support/event"
	"github.com/stretchr/testify/require"
)

func noopFlow(name string) string {
	return fmt.Sprintf(`{"name":"%[2]s","tasks":[{"id":"noop","name":"noop","activity":{"ref":"%[1]s","input":{"mode":"noop"}}}]}`,
		loggerTestRef, name)
}

// initWithFlows initialises the flow action factory with flow resources; it must run before any other Initialize.
func initWithFlows(t *testing.T, defs map[string]string) {
	t.Helper()
	ictx := coretest.NewActionInitCtx()
	require.NoError(t, (&ActionFactory{}).Initialize(ictx))
	for id, def := range defs {
		require.NoError(t, ictx.AddResource(flowsupport.ResTypeFlow, &resource.Config{ID: id, Data: []byte(def)}))
	}
}

func resourceFlowAction(t *testing.T, id string) *FlowAction {
	t.Helper()
	a, err := (&ActionFactory{}).New(&action.Config{Settings: map[string]interface{}{"flowURI": "res://" + id}})
	require.NoError(t, err)
	return a.(*FlowAction)
}

// The flow span is finished once, even when the result handler panics after the body finished it.
func TestRecoveredPanicFinishesSpanOnce(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_LEVEL=DEBUG")
		require.Contains(t, out, "StackTrace:", tail(out, 3000))
		return
	}
	registerTestTracer(t)
	fa := newTestFlowAction(t, noopFlow("finish_once"))
	h := newTestResultHandler()
	h.panicOnResult = true
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Equal(t, int64(1), testTracer.rootFinishes.Load(), "the flow span must be finished exactly once")
	require.Equal(t, []string{"result", "done"}, h.eventLog(), "the handler gets no second result")
}

// In concurrent mode a panic after the branches drained still gives the caller one error before Done.
func TestRecoveredPanicInConcurrentModeRepliesOnce(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_LEVEL=INFO", "FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY=true")
		return
	}
	registerTestTracer(t)
	testTracer.panicOnFinish.Store(true)
	fa := newTestFlowAction(t, parallelIterFlow(3))
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Equal(t, []string{"result", "done"}, h.eventLog())
	require.Error(t, h.err)
	require.Contains(t, h.err.Error(), "unhandled error executing flow instance")
	require.Equal(t, int64(1), testTracer.rootFinishes.Load())
}

// In sequential mode a panic while the flow span is finished still gives the caller one error before Done.
func TestFlowGoroutinePanicIsRecovered(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_LEVEL=INFO", "FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY=false")
		return
	}
	registerTestTracer(t)
	testTracer.panicOnFinish.Store(true)
	fa := newTestFlowAction(t, noopFlow("sequential_panic"))
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Equal(t, []string{"result", "done"}, h.eventLog())
	require.Error(t, h.err)
	require.Contains(t, h.err.Error(), "unhandled error executing flow instance")
	require.Equal(t, int64(1), testTracer.rootFinishes.Load())
}

type panickingRecorder struct {
	steps, done  atomic.Int64
	panicAtStep  int64
	lastDone     atomic.Value
	resultFirst  chan struct{}
	beforeResult atomic.Bool
}

func (r *panickingRecorder) RecordStart(*state.FlowState) error   { return nil }
func (r *panickingRecorder) RecordSnapshot(*state.Snapshot) error { return nil }
func (r *panickingRecorder) Start() error                         { return nil }
func (r *panickingRecorder) Stop() error                          { return nil }
func (r *panickingRecorder) Name() string                         { return "panicking-recorder" }

// RecordDone waits for the caller's result, as a slow state store would hold the caller up if it came first.
func (r *panickingRecorder) RecordDone(s *state.FlowState) error {
	if r.resultFirst != nil {
		select {
		case <-r.resultFirst:
		case <-time.After(5 * time.Second):
			r.beforeResult.Store(true)
		}
	}
	r.done.Add(1)
	r.lastDone.Store(s)
	return nil
}

func (r *panickingRecorder) RecordStep(*state.Step) error {
	if r.steps.Add(1) == r.panicAtStep {
		panic("recorder failure")
	}
	return nil
}

// A recovered panic marks a running instance failed and records it as done, so it is not left as running.
func TestRecoveredPanicMarksInstanceFailedAndRecordsDone(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_LEVEL=INFO")
		require.Equal(t, 1, strings.Count(out, "unhandled error executing flow instance"), "only the run that panics is recovered:\n%s", tail(out, 3000))
		return
	}
	registerTestTracer(t)
	fa := newTestFlowAction(t, fmt.Sprintf(`{"name":"records_done","tasks":[
  {"id":"a","name":"a","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
  {"id":"b","name":"b","activity":{"ref":"%[1]s","input":{"mode":"noop"}}}],
 "links":[{"id":1,"from":"a","to":"b","type":"label"}]}`, loggerTestRef))
	instance.EnableChangeTracking(true, state.RecordingModeStep)
	defer func() { stateRecorder, stateRecordingMode = nil, state.RecordingModeOff }()

	for _, c := range []struct {
		panicAtStep int64
		status      string
	}{{0, string(event.COMPLETED)}, {2, string(event.FAILED)}} {
		h := newTestResultHandler()
		rec := &panickingRecorder{panicAtStep: c.panicAtStep, resultFirst: h.resultArrived}
		stateRecorder, stateRecordingMode = rec, state.RecordingModeStep
		finishes := testTracer.rootFinishes.Load()
		require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
		h.wait(t)
		require.Equal(t, []string{"result", "done"}, h.eventLog())
		require.False(t, rec.beforeResult.Load(), "RecordDone ran before the caller got its result")
		require.Equal(t, c.panicAtStep != 0, h.err != nil, "only the run whose recorder panics fails")
		require.Equal(t, int64(1), rec.done.Load(), "RecordDone must be called once")
		require.Equal(t, finishes+1, testTracer.rootFinishes.Load(), "the flow span must be finished once")
		done, _ := rec.lastDone.Load().(*state.FlowState)
		require.NotNil(t, done)
		require.Equal(t, c.status, done.FlowStats)
	}
}

// A panic after the flow completed must not record it as failed.
func TestRecoveredPanicAfterCompletionKeepsCompletedState(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_LEVEL=INFO")
		return
	}
	fa := newTestFlowAction(t, noopFlow("stays_completed"))
	instance.EnableChangeTracking(true, state.RecordingModeStep)
	h := newTestResultHandler()
	h.panicOnResult = true
	rec := &panickingRecorder{resultFirst: h.resultArrived}
	stateRecorder, stateRecordingMode = rec, state.RecordingModeStep
	defer func() { stateRecorder, stateRecordingMode = nil, state.RecordingModeOff }()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Equal(t, int64(1), rec.done.Load())
	done, _ := rec.lastDone.Load().(*state.FlowState)
	require.NotNil(t, done)
	require.Equal(t, string(event.COMPLETED), done.FlowStats)
}

type panicOnceDoneRecorder struct {
	panickingRecorder
	calls atomic.Int64
}

func (r *panicOnceDoneRecorder) RecordDone(*state.FlowState) error {
	if r.calls.Add(1) == 1 {
		panic("record done failure")
	}
	return nil
}

// A state store that panics in RecordDone is not called again by the recover.
func TestRecoveredPanicInRecordDoneIsNotRetried(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_LEVEL=INFO")
		return
	}
	fa := newTestFlowAction(t, noopFlow("done_once"))
	instance.EnableChangeTracking(true, state.RecordingModeStep)
	rec := &panicOnceDoneRecorder{}
	stateRecorder, stateRecordingMode = rec, state.RecordingModeStep
	defer func() { stateRecorder, stateRecordingMode = nil, state.RecordingModeOff }()
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.NoError(t, h.err)
	require.Equal(t, int64(1), rec.calls.Load())
}

type cleanupFailingRecorder struct {
	panickingRecorder
	doneCalls atomic.Int64
}

func (r *cleanupFailingRecorder) RecordDone(*state.FlowState) error {
	r.doneCalls.Add(1)
	panic("record done failure")
}

// The recover still makes each of its tracer, handler and state store calls once when every one of them panics.
func TestRecoveredPanicSurvivesFailingCleanup(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_LEVEL=INFO")
		require.Contains(t, out, "unhandled error executing flow instance", tail(out, 3000))
		return
	}
	registerTestTracer(t)
	testTracer.panicOnFinish.Store(true)
	fa := newTestFlowAction(t, noopFlow("failing_cleanup"))
	instance.EnableChangeTracking(true, state.RecordingModeStep)
	rec := &cleanupFailingRecorder{}
	rec.panicAtStep = 2
	stateRecorder, stateRecordingMode = rec, state.RecordingModeStep
	defer func() { stateRecorder, stateRecordingMode = nil, state.RecordingModeOff }()
	h := newTestResultHandler()
	h.panicOnResult = true
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Equal(t, []string{"result", "done"}, h.eventLog())
	require.Equal(t, int64(1), testTracer.rootFinishes.Load())
	require.Equal(t, int64(1), rec.doneCalls.Load())
}

// The max-step failure finishes the span once and fails the request.
func TestMaxStepFailureFinishesSpanOnce(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_LEVEL=INFO", "FLOGO_MAX_STEP_COUNT=1")
		return
	}
	registerTestTracer(t)
	fa := newTestFlowAction(t, fmt.Sprintf(`{"name":"max_step","tasks":[
  {"id":"a","name":"a","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
  {"id":"b","name":"b","activity":{"ref":"%[1]s","input":{"mode":"noop"}}}],
 "links":[{"id":1,"from":"a","to":"b","type":"label"}]}`, loggerTestRef))
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.Error(t, h.err)
	require.Contains(t, h.err.Error(), "max step count")
	require.Equal(t, int64(1), testTracer.rootFinishes.Load())
}

func twoTaskFlow(name string) string {
	return fmt.Sprintf(`{"name":"%[2]s","tasks":[
  {"id":"a","name":"a","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
  {"id":"b","name":"b","activity":{"ref":"%[1]s","input":{"mode":"noop"}}}],
 "links":[{"id":1,"from":"a","to":"b","type":"label"}]}`, loggerTestRef, name)
}

// A handler that panics on the failed, max-step or returned-id reply gets no second result, and the span ends once.
func TestRecoveredHandlerPanicRepliesOnce(t *testing.T) {
	for _, c := range []struct {
		name, flow string
		env        []string
		ro         *instance.RunOptions
	}{
		{name: "failed", flow: failFlow("reply_failed", "reply_failed")},
		{name: "max_step", flow: twoTaskFlow("reply_max_step"), env: []string{"FLOGO_MAX_STEP_COUNT=1"}},
		{name: "return_id", flow: noopFlow("reply_return_id"), ro: &instance.RunOptions{Op: instance.OpStart, ReturnID: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if os.Getenv(childEnv) != "1" {
				runChild(t, append([]string{"FLOGO_LOG_LEVEL=INFO"}, c.env...)...)
				return
			}
			registerTestTracer(t)
			fa := newTestFlowAction(t, c.flow)
			h := newTestResultHandler()
			h.panicOnResult = true
			inputs := map[string]interface{}{}
			if c.ro != nil {
				inputs["_run_options"] = c.ro
			}
			require.NoError(t, fa.Run(context.Background(), inputs, h))
			h.wait(t)
			require.Equal(t, []string{"result", "done"}, h.eventLog(), "the handler gets no second result")
			require.Equal(t, int64(1), testTracer.rootFinishes.Load(), "the flow span must be finished once")
		})
	}
}

// doneSignalRecorder closes recorded at the first RecordDone, which the recover makes after its (skipped) reply.
type doneSignalRecorder struct {
	panickingRecorder
	recorded chan struct{}
}

func (r *doneSignalRecorder) RecordDone(*state.FlowState) error {
	if r.done.Add(1) == 1 {
		close(r.recorded)
	}
	return nil
}

// A detached run has no caller: a panic is logged and nothing is sent to the handler after Done.
func TestDetachedFlowPanicIsRecovered(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_LEVEL=INFO")
		require.Contains(t, out, "unhandled error executing flow instance", tail(out, 3000))
		return
	}
	registerTestTracer(t)
	testTracer.panicOnFinish.Store(true)
	fa := newTestFlowAction(t, noopFlow("detached"))
	instance.EnableChangeTracking(true, state.RecordingModeStep)
	rec := &doneSignalRecorder{recorded: make(chan struct{})}
	stateRecorder, stateRecordingMode = rec, state.RecordingModeStep
	defer func() { stateRecorder, stateRecordingMode = nil, state.RecordingModeOff }()
	h := newTestResultHandler()
	ro := &instance.RunOptions{Op: instance.OpStart, DetachExecution: true}
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{"_run_options": ro}, h))
	h.wait(t)
	select {
	case <-rec.recorded:
	case <-time.After(20 * time.Second):
		t.Fatal("the recover did not record the detached flow as done")
	}
	require.Equal(t, int64(1), testTracer.rootFinishes.Load())
	require.Equal(t, []string{"done"}, h.eventLog(), "a detached run never sends a result")
}

// startWithSpan starts an instance outside FlowAction.Run, as a state store would have; tracing needs a span.
func startWithSpan(t *testing.T, inst *instance.IndependentInstance) {
	t.Helper()
	tc, err := testTracer.StartTrace(trace.Config{}, nil)
	require.NoError(t, err)
	inst.SetTracingContext(tc)
	inst.Start(nil)
}

func failFlow(name, role string) string {
	return fmt.Sprintf(`{"name":"%[2]s","tasks":[{"id":"fail","name":"fail","activity":{"ref":"%[1]s","input":{"mode":"fail","role":"%[3]s"}}}]}`,
		loggerTestRef, name, role)
}

// checkRoleLines asserts that every JSON line about the role carries exactly the trace id that role reported.
func checkRoleLines(t *testing.T, out, role string) {
	t.Helper()
	var trace string
	for _, m := range markRe.FindAllStringSubmatch(out, -1) {
		if m[1] == role {
			trace = m[3]
		}
	}
	require.NotEmpty(t, trace, "the task must report its trace id:\n%s", tail(out, 3000))
	checked := 0
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "{") || !strings.Contains(line, "role="+role) {
			continue
		}
		var entry map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
		checked++
		checkOneTrace(t, line, entry, trace)
	}
	require.GreaterOrEqual(t, checked, 2, "expected the task's and the instance's error lines:\n%s", tail(out, 3000))
}

// A restarted instance logs through its own traced logger, including the instance-level lines.
func TestRestartedInstanceGetsLoggerAndTrace(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO")
		checkRoleLines(t, out, "restart")
		return
	}
	registerTestTracer(t)
	initWithFlows(t, map[string]string{"flow:restart_flow": failFlow("restart_flow", "restart")})
	def, _, err := flowsupport.GetDefinition("res://flow:restart_flow")
	require.NoError(t, err)
	inst, err := instance.NewIndependentInstance("first-run", "res://flow:restart_flow", def, nil, log.RootLogger(), context.Background())
	require.NoError(t, err)
	startWithSpan(t, inst)

	fa := resourceFlowAction(t, "flow:restart_flow")
	h := newTestResultHandler()
	ro := &instance.RunOptions{Op: instance.OpRestart, ReturnID: true, InitialState: inst}
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{"_run_options": ro}, h))
	h.wait(t)
	require.Error(t, h.err)
}

// A resumed instance logs through a traced logger built like a started one's, not through the one it had.
func TestResumedInstanceGetsLoggerAndTrace(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO", "FLOGO_ENV=e2e")
		checkRoleLines(t, out, "resume")
		return
	}
	registerTestTracer(t)
	initWithFlows(t, map[string]string{"flow:resume_flow": failFlow("resume_flow", "resume")})
	def, _, err := flowsupport.GetDefinition("res://flow:resume_flow")
	require.NoError(t, err)
	inst, err := instance.NewIndependentInstance("resumed", "res://flow:resume_flow", def, nil, log.RootLogger(), context.Background())
	require.NoError(t, err)
	startWithSpan(t, inst)

	fa := resourceFlowAction(t, "flow:resume_flow")
	h := newTestResultHandler()
	ro := &instance.RunOptions{Op: instance.OpResume, InitialState: inst}
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{"_run_options": ro}, h))
	h.wait(t)
	require.Error(t, h.err)
}

// Without tracing, a resumed instance logs through the flow logger with its own context fields.
func TestUntracedResumeLogsWithInstanceFields(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO", "FLOGO_ENV=e2e")
		want := map[string]string{"flow.name": "resume_untraced", "flow.id": "resumed-untraced", "event.id": "evt-resume-1",
			"deployment.environment": "e2e"}
		checked := 0
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "{") || !strings.Contains(line, "role=resume_untraced") {
				continue
			}
			var entry map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
			if entry["logger"] != "flogo.flow" {
				continue
			}
			checked++
			for k, v := range want {
				if entry[k] != v {
					t.Errorf("%s = %v, want %s: %s", k, entry[k], v, line)
				}
			}
			for _, k := range []string{"app.name", "app.version"} {
				if _, ok := entry[k]; !ok {
					t.Errorf("no %s field: %s", k, line)
				}
			}
		}
		require.GreaterOrEqual(t, checked, 2, "expected the instance's error lines from the flow logger:\n%s", tail(out, 3000))
		return
	}
	initWithFlows(t, map[string]string{"flow:resume_untraced": failFlow("resume_untraced", "resume_untraced")})
	def, _, err := flowsupport.GetDefinition("res://flow:resume_untraced")
	require.NoError(t, err)
	inst, err := instance.NewIndependentInstance("resumed-untraced", "res://flow:resume_untraced", def, nil, log.RootLogger(), context.Background())
	require.NoError(t, err)
	inst.Start(nil)
	fa := resourceFlowAction(t, "flow:resume_untraced")
	h := newTestResultHandler()
	ro := &instance.RunOptions{Op: instance.OpResume, InitialState: inst}
	ctx := trigger.NewContextWithEventId(context.Background(), "evt-resume-1")
	require.NoError(t, fa.Run(ctx, map[string]interface{}{"_run_options": ro}, h))
	h.wait(t)
	require.Error(t, h.err)
}

// Iterations of a task inside a subflow carry the parent's trace id once each, with their own spans.
func TestSubflowLinesCarryTheParentTrace(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO")
		var parentTrace string
		for _, m := range markRe.FindAllStringSubmatch(out, -1) {
			parentTrace = m[3]
		}
		require.NotEmpty(t, parentTrace, tail(out, 3000))
		spans := map[string]bool{}
		lines := 0
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "{") || !strings.Contains(line, "subflow iteration") {
				continue
			}
			var entry map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
			lines++
			checkOneTrace(t, line, entry, parentTrace)
			span, _ := entry["span.id"].(string)
			spans[span] = true
		}
		require.Equal(t, 3, lines, tail(out, 3000))
		require.Len(t, spans, 3, "each iteration has its own span")
		return
	}
	registerTestTracer(t)
	parent := fmt.Sprintf(`{"name":"parent_flow","tasks":[{"id":"sub","name":"sub","activity":{"ref":"%[1]s","input":{"mode":"startsub","role":"child_flow"}}}]}`, loggerTestRef)
	child := fmt.Sprintf(`{"name":"child_flow","tasks":[{"id":"subloop","name":"subloop","type":"iterator","settings":{"iterate":3},"activity":{"ref":"%[1]s","input":{"mode":"subiter"}}}]}`, loggerTestRef)
	initWithFlows(t, map[string]string{"flow:parent_flow": parent, "flow:child_flow": child})
	fa := resourceFlowAction(t, "flow:parent_flow")
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.NoError(t, h.err)
	require.Equal(t, int64(3), evalCount.Load())
}
