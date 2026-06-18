package instance_test

// Black-box integration tests for concurrent flow execution (FLOGO-18450). This external
// test package can import the simple model (which imports the instance package) without an
// import cycle, so it exercises the real RunConcurrent path end-to-end via the public API.
//
// All tasks use a single modern (non-legacy) activity distinguished by a "mode" input. It
// must be modern so the activity receives the TaskInst as its context (legacy activities
// get a LegacyCtx whose GoContext() is nil and therefore cannot be cancelled).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/stretchr/testify/assert"
)

const concurrentFlagEnv = "FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY"

var (
	joinCount      atomic.Int64
	sleepsStarted  atomic.Int64
	sleepsReturned atomic.Int64

	flowActivityRef string
)

func init() {
	model.RegisterDefault(simple.New())

	act := &flowActivity{md: &activity.Metadata{IOMetadata: &metadata.IOMetadata{Input: map[string]data.TypedValue{
		"mode": data.NewTypedValue(data.TypeString, ""),
		"ms":   data.NewTypedValue(data.TypeInt, 0),
	}}}}
	_ = activity.Register(act) // modern registration -> non-legacy -> cancellable context
	flowActivityRef = activity.GetRef(act)
}

// flowActivity is a single modern activity whose behavior is selected by the "mode" input:
// noop, sleep (context-aware), count (join marker), or fail.
type flowActivity struct{ md *activity.Metadata }

func (a *flowActivity) Metadata() *activity.Metadata { return a.md }

func (a *flowActivity) Eval(ctx activity.Context) (bool, error) {
	mode, _ := coerce.ToString(ctx.GetInput("mode"))
	switch mode {
	case "sleep":
		sleepsStarted.Add(1)
		defer sleepsReturned.Add(1)

		ms, _ := coerce.ToInt(ctx.GetInput("ms"))
		d := time.Duration(ms) * time.Millisecond

		gctx := ctx.GoContext()
		if gctx == nil {
			time.Sleep(d)
			return true, nil
		}
		select {
		case <-time.After(d):
			return true, nil
		case <-gctx.Done():
			return false, gctx.Err()
		}
	case "count":
		joinCount.Add(1)
		return true, nil
	case "fail":
		return false, errors.New("branch failed intentionally")
	case "wait":
		return false, nil // not done -> EvalWait (task parked, never resumed in this test)
	case "cpu":
		// Busy CPU work sized by the "ms" input (reused as work units), with a sink to
		// prevent the compiler from optimizing the loop away. Used by benchmarks.
		work, _ := coerce.ToInt(ctx.GetInput("ms"))
		x := 0
		for i := 0; i < work*1000; i++ {
			x += (i * 2654435761) & 0x7fffffff
		}
		cpuSink.Add(int64(x & 1))
		return true, nil
	default: // noop
		return true, nil
	}
}

// cpuSink prevents dead-code elimination of the benchmark CPU work.
var cpuSink atomic.Int64

func waitDef() string {
	return fmt.Sprintf(`{
  "name": "waitflow",
  "tasks": [
    {"id":"start","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
    {"id":"w","activity":{"ref":"%[1]s","input":{"mode":"wait"}}}
  ],
  "links": [
    {"id":1,"from":"start","to":"w","type":"label"}
  ]
}`, flowActivityRef)
}

// forkJoinDef: start fans out to three sleep branches that all join at a counter task.
func forkJoinDef(branchMs int) string {
	return fmt.Sprintf(`{
  "name": "forkjoin",
  "tasks": [
    {"id":"start","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
    {"id":"s1","activity":{"ref":"%[1]s","input":{"mode":"sleep","ms":%[2]d}}},
    {"id":"s2","activity":{"ref":"%[1]s","input":{"mode":"sleep","ms":%[2]d}}},
    {"id":"s3","activity":{"ref":"%[1]s","input":{"mode":"sleep","ms":%[2]d}}},
    {"id":"join","activity":{"ref":"%[1]s","input":{"mode":"count"}}}
  ],
  "links": [
    {"id":1,"from":"start","to":"s1","type":"label"},
    {"id":2,"from":"start","to":"s2","type":"label"},
    {"id":3,"from":"start","to":"s3","type":"label"},
    {"id":4,"from":"s1","to":"join","type":"label"},
    {"id":5,"from":"s2","to":"join","type":"label"},
    {"id":6,"from":"s3","to":"join","type":"label"}
  ]
}`, flowActivityRef, branchMs)
}

// forkFailDef: like forkJoinDef but the middle branch fails immediately; the two long sleep
// branches must be cancelled and drained rather than run to completion.
func forkFailDef(branchMs int) string {
	return fmt.Sprintf(`{
  "name": "forkfail",
  "tasks": [
    {"id":"start","activity":{"ref":"%[1]s","input":{"mode":"noop"}}},
    {"id":"s1","activity":{"ref":"%[1]s","input":{"mode":"sleep","ms":%[2]d}}},
    {"id":"f","activity":{"ref":"%[1]s","input":{"mode":"fail"}}},
    {"id":"s3","activity":{"ref":"%[1]s","input":{"mode":"sleep","ms":%[2]d}}},
    {"id":"join","activity":{"ref":"%[1]s","input":{"mode":"count"}}}
  ],
  "links": [
    {"id":1,"from":"start","to":"s1","type":"label"},
    {"id":2,"from":"start","to":"f","type":"label"},
    {"id":3,"from":"start","to":"s3","type":"label"},
    {"id":4,"from":"s1","to":"join","type":"label"},
    {"id":5,"from":"f","to":"join","type":"label"},
    {"id":6,"from":"s3","to":"join","type":"label"}
  ]
}`, flowActivityRef, branchMs)
}

func buildInstance(t *testing.T, defJSON string, concurrent bool) *instance.IndependentInstance {
	t.Helper()
	if concurrent {
		t.Setenv(concurrentFlagEnv, "true")
	} else {
		t.Setenv(concurrentFlagEnv, "false")
	}

	defRep := &definition.DefinitionRep{}
	err := json.Unmarshal([]byte(defJSON), defRep)
	assert.NoError(t, err)

	def, err := definition.NewDefinition(defRep)
	assert.NoError(t, err)
	assert.NotNil(t, def)

	inst, err := instance.NewIndependentInstance("it-"+def.Name(), "", def, nil, log.RootLogger(), context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, inst)
	return inst
}

func resetCounters() {
	joinCount.Store(0)
	sleepsStarted.Store(0)
	sleepsReturned.Store(0)
}

// TestRunConcurrentForkJoinParallel asserts the three branches overlap (wall-clock close to
// one branch, not the sum) and the join task fires exactly once.
func TestRunConcurrentForkJoinParallel(t *testing.T) {
	resetCounters()
	inst := buildInstance(t, forkJoinDef(200), true)
	assert.True(t, inst.Start(nil))

	start := time.Now()
	inst.RunConcurrent(0, 1000000, nil)
	elapsed := time.Since(start)

	assert.Equal(t, model.FlowStatusCompleted, inst.Status())
	assert.Equal(t, int64(1), joinCount.Load(), "join must run exactly once")
	assert.Equal(t, int64(3), sleepsStarted.Load(), "all three branches must run")
	assert.Equal(t, int64(3), sleepsReturned.Load())
	assert.Less(t, elapsed, 500*time.Millisecond,
		"three 200ms branches should overlap (<500ms); got %v", elapsed)
}

// TestRunSequentialForkJoin asserts the sequential (flag-off) path is unchanged: branches
// run one after another (~600ms) and the join still fires exactly once.
func TestRunSequentialForkJoin(t *testing.T) {
	resetCounters()
	inst := buildInstance(t, forkJoinDef(200), false)
	assert.True(t, inst.Start(nil))

	start := time.Now()
	for inst.Status() < model.FlowStatusCompleted {
		if !inst.DoStep() {
			break
		}
	}
	elapsed := time.Since(start)

	assert.Equal(t, model.FlowStatusCompleted, inst.Status())
	assert.Equal(t, int64(1), joinCount.Load(), "join must run exactly once")
	assert.Equal(t, int64(3), sleepsStarted.Load())
	assert.Greater(t, elapsed, 500*time.Millisecond,
		"three sequential 200ms branches should take ~600ms; got %v", elapsed)
}

// TestRunConcurrentEvalWaitQuiesces covers the EvalWait result path: a task that returns
// "not done" is parked as Waiting, and with nothing left to schedule the pool quiesces and
// RunConcurrent returns with the flow still active (neither completed nor failed).
func TestRunConcurrentEvalWaitQuiesces(t *testing.T) {
	resetCounters()
	inst := buildInstance(t, waitDef(), true)
	assert.True(t, inst.Start(nil))

	inst.RunConcurrent(0, 1000000, nil)

	assert.Equal(t, model.FlowStatusActive, inst.Status(),
		"a parked (waiting) task should leave the flow active after the pool quiesces")
}

// TestRunConcurrentDrainThenFail asserts that a failing branch fails the whole flow, the
// join never runs, all in-flight branches drain, and the context-aware sibling sleeps are
// cancelled early (drain-then-fail) rather than running to completion.
func TestRunConcurrentDrainThenFail(t *testing.T) {
	resetCounters()
	inst := buildInstance(t, forkFailDef(2000), true)
	assert.True(t, inst.Start(nil))

	start := time.Now()
	inst.RunConcurrent(0, 1000000, nil)
	elapsed := time.Since(start)

	assert.Equal(t, model.FlowStatusFailed, inst.Status(), "flow must fail when a branch fails")
	assert.Error(t, inst.GetError(), "the failure must be surfaced")
	assert.Equal(t, int64(0), joinCount.Load(), "join must not run when a branch fails")
	assert.Equal(t, sleepsStarted.Load(), sleepsReturned.Load(),
		"every in-flight branch must drain (started == returned)")
	assert.Less(t, elapsed, 1500*time.Millisecond,
		"sibling 2s sleeps must be cancelled, not run to completion; got %v", elapsed)
}
