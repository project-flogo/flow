package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support/log"
	coretest "github.com/project-flogo/core/support/test"
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/core/trigger"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/stretchr/testify/require"
)

// Tests that need a tracer or logging env vars run in a child process, as both are process-wide. With -cover,
// set TEST_CHILD_COVERDIR to a directory for the children's cover profiles: without it these tests fail.
const childEnv = "FLOW_LOGGER_TEST_CHILD"

var loggerTestRef string

func init() {
	act := &loggerTestActivity{md: &activity.Metadata{IOMetadata: &metadata.IOMetadata{Input: map[string]data.TypedValue{
		"mode": data.NewTypedValue(data.TypeString, ""),
		"role": data.NewTypedValue(data.TypeString, ""),
	}}}}
	_ = activity.Register(act)
	loggerTestRef = activity.GetRef(act)
}

type mockTracer struct {
	traces, spans, rootFinishes atomic.Int64
	panicOnFinish               atomic.Bool
}

type mockSpan struct {
	traceID, spanID string
	root            bool
}

func (s *mockSpan) TraceObject() interface{}                                              { return s }
func (s *mockSpan) SetTags(map[string]interface{}) bool                                   { return true }
func (s *mockSpan) SetTag(string, interface{}) bool                                       { return true }
func (s *mockSpan) LogKV(map[string]interface{}) bool                                     { return true }
func (s *mockSpan) TraceID() string                                                       { return s.traceID }
func (s *mockSpan) SpanID() string                                                        { return s.spanID }
func (m *mockTracer) Start() error                                                        { return nil }
func (m *mockTracer) Stop() error                                                         { return nil }
func (m *mockTracer) Name() string                                                        { return "mock" }
func (m *mockTracer) Inject(trace.TracingContext, trace.CarrierFormat, interface{}) error { return nil }
func (m *mockTracer) Extract(trace.CarrierFormat, interface{}) (trace.TracingContext, error) {
	return nil, nil
}

func (m *mockTracer) StartTrace(_ trace.Config, parent trace.TracingContext) (trace.TracingContext, error) {
	s := &mockSpan{spanID: fmt.Sprintf("%016x", m.spans.Add(1))}
	if parent != nil {
		s.traceID = parent.TraceID()
	} else {
		s.traceID, s.root = fmt.Sprintf("%032x", m.traces.Add(1)), true
	}
	return s, nil
}

func (m *mockTracer) FinishTrace(tc trace.TracingContext, _ error) error {
	if s, ok := tc.(*mockSpan); ok && s.root {
		m.rootFinishes.Add(1)
		if m.panicOnFinish.Load() {
			panic("tracer failure")
		}
	}
	return nil
}

var testTracer = &mockTracer{}

var (
	firstStarted, secondStarted, firstLogged chan struct{}
	evalCount                                atomic.Int64
	ctxViolations                            atomic.Int64
	iterCheckpoints                          = map[int64]bool{1: true, 2: true, 3: true, 1000: true, 5000: true}
	iterTotal                                int64
)

type loggerTestActivity struct{ md *activity.Metadata }

func (a *loggerTestActivity) Metadata() *activity.Metadata { return a.md }

func waitFor(ch chan struct{}, what string) {
	select {
	case <-ch:
	case <-time.After(20 * time.Second):
		panic("timed out waiting for " + what)
	}
}

func (a *loggerTestActivity) Eval(ctx activity.Context) (bool, error) {
	mode, _ := coerce.ToString(ctx.GetInput("mode"))
	role, _ := coerce.ToString(ctx.GetInput("role"))
	id, traceID := ctx.ActivityHost().ID(), ""
	if tc := ctx.GetTracingContext(); tc != nil {
		traceID = tc.TraceID()
	}
	switch mode {
	case "sync":
		fmt.Printf("MARK role=%s instance=%s trace=%s\n", role, id, traceID)
		if role == "first" {
			close(firstStarted)
			waitFor(secondStarted, "the second flow")
			ctx.Logger().Infof("activity line role=%s instance=%s", role, id)
			close(firstLogged)
		} else {
			close(secondStarted)
			waitFor(firstLogged, "the first flow's log line")
			ctx.Logger().Infof("activity line role=%s instance=%s", role, id)
		}
	case "iter":
		n := evalCount.Add(1)
		if iterCheckpoints[n] || n == iterTotal {
			ctx.Logger().Infof("iteration %d instance=%s", n, id)
		}
		if n == 1000 || n == iterTotal {
			runtime.GC()
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Printf("HEAP iteration=%d alloc=%d\n", n, ms.HeapAlloc)
		}
	case "ctxcheck":
		evalCount.Add(1)
		if gc := ctx.GoContext(); gc == nil || gc.Done() == nil {
			ctxViolations.Add(1)
		}
	case "fail":
		fmt.Printf("MARK role=%s instance=%s trace=%s\n", role, id, traceID)
		return false, errors.New("task failed on purpose role=" + role)
	case "startsub":
		fmt.Printf("MARK role=%s instance=%s trace=%s\n", role, id, traceID)
		return false, instance.StartSubFlow(ctx, "res://flow:"+role, map[string]interface{}{})
	case "subiter":
		ctx.Logger().Infof("subflow iteration %d", evalCount.Add(1))
	case "mark":
		fmt.Printf("MARK role=%s instance=%s trace=%s\n", role, id, traceID)
	}
	return true, nil
}

type testResultHandler struct {
	done          chan struct{}
	resultArrived chan struct{}
	firstResult   sync.Once
	mu            sync.Mutex
	results       map[string]interface{}
	err           error
	events        []string
	panicOnResult bool
}

func newTestResultHandler() *testResultHandler {
	return &testResultHandler{done: make(chan struct{}), resultArrived: make(chan struct{})}
}

func (h *testResultHandler) HandleResult(results map[string]interface{}, err error) {
	h.mu.Lock()
	h.events = append(h.events, "result")
	h.mu.Unlock()
	h.firstResult.Do(func() { close(h.resultArrived) })
	if h.panicOnResult {
		panic("result handler failure")
	}
	h.mu.Lock()
	h.results, h.err = results, err
	h.mu.Unlock()
}

func (h *testResultHandler) Done() {
	h.mu.Lock()
	h.events = append(h.events, "done")
	h.mu.Unlock()
	close(h.done)
}

func (h *testResultHandler) eventLog() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.events...)
}

func (h *testResultHandler) wait(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(60 * time.Second):
		t.Fatal("flow did not finish")
	}
}

func newTestFlowAction(t *testing.T, defJSON string) *FlowAction {
	t.Helper()
	require.NoError(t, (&ActionFactory{}).Initialize(coretest.NewActionInitCtx()))
	rep := &definition.DefinitionRep{}
	require.NoError(t, json.Unmarshal([]byte(defJSON), rep))
	def, err := definition.NewDefinition(rep)
	require.NoError(t, err)
	return &FlowAction{flowURI: "res://flow:" + def.Name(), resFlow: def}
}

func syncFlow(role string) string {
	return fmt.Sprintf(`{"name":"sync_%[2]s","tasks":[{"id":"act_%[2]s","name":"act_%[2]s","activity":{"ref":"%[1]s","input":{"mode":"sync","role":"%[2]s"}}}]}`,
		loggerTestRef, role)
}

func iterFlow(n int) string {
	return fmt.Sprintf(`{"name":"iter","tasks":[{"id":"loop","name":"loop","type":"iterator","settings":{"iterate":%[2]d},"activity":{"ref":"%[1]s","input":{"mode":"iter"}}}]}`,
		loggerTestRef, n)
}

func parallelIterFlow(n int) string {
	return fmt.Sprintf(`{"name":"pariter","tasks":[
  {"id":"start","name":"start","activity":{"ref":"%[1]s","input":{"mode":"ctxcheck"}}},
  {"id":"a","name":"a","type":"iterator","settings":{"iterate":%[2]d},"activity":{"ref":"%[1]s","input":{"mode":"ctxcheck"}}},
  {"id":"b","name":"b","type":"iterator","settings":{"iterate":%[2]d},"activity":{"ref":"%[1]s","input":{"mode":"ctxcheck"}}},
  {"id":"c","name":"c","type":"iterator","settings":{"iterate":%[2]d},"activity":{"ref":"%[1]s","input":{"mode":"ctxcheck"}}}],
 "links":[{"id":1,"from":"start","to":"a","type":"label"},{"id":2,"from":"start","to":"b","type":"label"},{"id":3,"from":"start","to":"c","type":"label"}]}`,
		loggerTestRef, n)
}

var childRuns atomic.Int64

// childCoverArgs gives a child its own cover profile in TEST_CHILD_COVERDIR; go test deletes the parent's directory.
func childCoverArgs(t *testing.T) []string {
	if testing.CoverMode() == "" {
		return nil
	}
	dir := os.Getenv("TEST_CHILD_COVERDIR")
	if dir == "" {
		t.Fatal("cover mode is on: set TEST_CHILD_COVERDIR to keep the child processes' cover profiles")
	}
	name := fmt.Sprintf("child-%s-%d-%d.out", strings.ReplaceAll(t.Name(), "/", "_"), os.Getpid(), childRuns.Add(1))
	return []string{"-test.coverprofile=" + filepath.Join(dir, name)}
}

// childTimeout keeps a hung child within the parent's deadline, with a tenth of the time left to report it.
func childTimeout(t *testing.T) string {
	d := 10 * time.Minute
	if deadline, ok := t.Deadline(); ok {
		d = time.Until(deadline) * 9 / 10
	}
	if d < time.Second {
		d = time.Second
	}
	return "-test.timeout=" + d.String()
}

// runChild re-runs the calling test in a fresh process with the given environment and returns its output.
func runChild(t *testing.T, env ...string) string {
	t.Helper()
	args := append([]string{"-test.run=^" + t.Name() + "$", "-test.count=1", "-test.v", childTimeout(t)}, childCoverArgs(t)...)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), childEnv+"=1", "FLOGO_LOG_CONSOLE_STREAM=stdout"), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed: %v\n%s", err, tail(string(out), 6000))
	}
	return string(out)
}

func tail(s string, n int) string {
	if len(s) > n {
		return "...\n" + s[len(s)-n:]
	}
	return s
}

func registerTestTracer(t *testing.T) {
	t.Helper()
	if !trace.Enabled() {
		require.NoError(t, trace.RegisterTracer(testTracer))
	}
}

func runSyncFlows(t *testing.T) {
	registerTestTracer(t)
	firstStarted, secondStarted, firstLogged = make(chan struct{}), make(chan struct{}), make(chan struct{})
	first, second := newTestFlowAction(t, syncFlow("first")), newTestFlowAction(t, syncFlow("second"))
	h1, h2 := newTestResultHandler(), newTestResultHandler()

	start := make(chan struct{})
	runErrs := make(chan error, 2)
	for _, run := range []struct {
		fa *FlowAction
		h  *testResultHandler
	}{{first, h1}, {second, h2}} {
		go func(fa *FlowAction, h *testResultHandler) {
			<-start
			runErrs <- fa.Run(context.Background(), map[string]interface{}{}, h)
		}(run.fa, run.h)
	}
	close(start)
	require.NoError(t, <-runErrs)
	require.NoError(t, <-runErrs)
	h1.wait(t)
	h2.wait(t)
	require.NoError(t, h1.err)
	require.NoError(t, h2.err)
}

var (
	markRe  = regexp.MustCompile(`MARK role=(\w+) instance=(\S+) trace=(\w*)`)
	traceRe = regexp.MustCompile(`\[traceID: (\w*)\]`)
)

// checkOneTrace fails unless the JSON log line carries exactly one trace.id field and one trace prefix, both want.
func checkOneTrace(t *testing.T, line string, entry map[string]interface{}, want string) {
	t.Helper()
	if n := strings.Count(line, `"trace.id"`); n != 1 || entry["trace.id"] != want {
		t.Errorf("%d trace.id fields, value %v, want one with %s: %.1000s", n, entry["trace.id"], want, line)
	}
	if got := traceRe.FindAllStringSubmatch(line, -1); len(got) != 1 || got[0][1] != want {
		t.Errorf("%d trace prefixes, want one with %s: %.1000s", len(got), want, line)
	}
}

type flowMark struct{ role, instance, trace string }

func parseMarks(t *testing.T, out string) []flowMark {
	t.Helper()
	var marks []flowMark
	for _, m := range markRe.FindAllStringSubmatch(out, -1) {
		marks = append(marks, flowMark{m[1], m[2], m[3]})
	}
	require.Len(t, marks, 2, "both flows must report their instance and trace id")
	require.NotEqual(t, marks[0].trace, marks[1].trace)
	require.NotEmpty(t, marks[0].trace)
	return marks
}

// Every log line about a flow instance must carry that flow's own trace id.
func TestConcurrentFlowsLogTheirOwnTraceIDs(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		runSyncFlows(t)
		return
	}
	out := runChild(t, "FLOGO_LOG_CTX=false", "FLOGO_LOG_FORMAT=CONSOLE", "FLOGO_LOG_LEVEL=INFO")

	checked := 0
	for _, m := range parseMarks(t, out) {
		for _, line := range strings.Split(out, "\n") {
			if !strings.Contains(line, "instance="+m.instance) && !strings.Contains(line, "Flow Instance ["+m.instance+"]") {
				continue
			}
			if strings.HasPrefix(line, "MARK") {
				continue
			}
			checked++
			got := traceRe.FindStringSubmatch(line)
			if got == nil || got[1] != m.trace {
				t.Errorf("flow %s (trace %s) logged with another trace context: %s", m.role, m.trace, line)
			}
			if strings.Contains(line, "trace.id") {
				t.Errorf("trace fields on a line logged with FLOGO_LOG_CTX=false: %s", line)
			}
		}
	}
	require.GreaterOrEqual(t, checked, 6, "expected the activity, executing and completed lines of both flows:\n%s", out)
}

// With FLOGO_LOG_CTX=true the trace fields and the prefix must both belong to the flow that logs.
func TestConcurrentFlowsLogTheirOwnTraceIDsWithContextFields(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		runSyncFlows(t)
		return
	}
	out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO")

	checked := 0
	for _, m := range parseMarks(t, out) {
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "{") || !strings.Contains(line, m.instance) {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Errorf("unparsable line %q: %v", line, err)
				continue
			}
			checked++
			if entry["flow.id"] != m.instance {
				t.Errorf("flow %s: flow.id = %v in %s", m.role, entry["flow.id"], line)
			}
			checkOneTrace(t, line, entry, m.trace)
		}
	}
	require.GreaterOrEqual(t, checked, 6, "expected the activity, executing and completed lines of both flows:\n%s", out)
}

// After a traced run the loggers shared by all flows must not hold any flow's tracing context.
func TestTracedRunLeavesSharedLoggersUntouched(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		runChild(t, "FLOGO_LOG_CTX=false", "FLOGO_LOG_LEVEL=INFO")
		return
	}
	runSyncFlows(t)
	if tc := logger.GetTracingContext(); tc != nil {
		t.Errorf("shared flow logger holds a tracing context after the run: %v", tc)
	}
	if tc := activity.GetLogger(loggerTestRef).GetTracingContext(); tc != nil {
		t.Errorf("shared activity logger holds a tracing context after the run: %v", tc)
	}
	if tc := log.RootLogger().GetTracingContext(); tc != nil {
		t.Errorf("root logger holds a tracing context after the run: %v", tc)
	}
}

var flowDebugExempt = []string{"Creating Flow Instance: ", "Creating Flow Instance [", "Running FlowAction for URI: '",
	"Restarting Flow Instance: ", "Resuming Flow Instance: ", "Applying Exec Options to instance: "}

// At DEBUG every flow-logger line except the six pre-trace ones must carry the flow's own trace id.
func TestFlowLoggerLinesCarryTraceAtDebug(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		out := runChild(t, "FLOGO_LOG_LEVEL=DEBUG", "FLOGO_LOG_FORMAT=CONSOLE", "FLOGO_LOG_CTX=false")
		marks := markRe.FindAllStringSubmatch(out, -1)
		require.Len(t, marks, 1, tail(out, 3000))
		own := marks[0][3]
		require.NotEmpty(t, own)
		checked, eventLine := 0, false
		for _, line := range strings.Split(out, "\n") {
			i := strings.Index(line, "[flogo.flow] -")
			if i < 0 {
				continue
			}
			msg := strings.TrimLeft(line[i+len("[flogo.flow] -"):], " \t")
			exempt := false
			for _, p := range flowDebugExempt {
				exempt = exempt || strings.HasPrefix(msg, p)
			}
			if exempt {
				continue
			}
			checked++
			eventLine = eventLine || strings.Contains(msg, "evt-debug-1")
			if !strings.HasPrefix(msg, "[traceID: "+own+"]") {
				t.Errorf("flow logger line without the flow's trace id %s: %s", own, msg)
			}
		}
		require.True(t, eventLine, "expected the event id line among the checked lines:\n%s", tail(out, 3000))
		t.Logf("flow logger lines checked: %d", checked)
		return
	}
	registerTestTracer(t)
	fa := newTestFlowAction(t, fmt.Sprintf(`{"name":"debug_lines","tasks":[{"id":"mark","name":"mark","activity":{"ref":"%[1]s","input":{"mode":"mark","role":"debug"}}}]}`, loggerTestRef))
	h := newTestResultHandler()
	ctx := trigger.NewContextWithEventId(context.Background(), "evt-debug-1")
	require.NoError(t, fa.Run(ctx, map[string]interface{}{}, h))
	h.wait(t)
	require.NoError(t, h.err)
}

func iterationsFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv("FLOW_TEST_ITERATIONS")); err == nil && n > 0 {
		return n
	}
	return 10000
}

// A repeating task must log its current span only, and must not retain memory per iteration.
func TestRepeatingTaskDoesNotNestLoggers(t *testing.T) {
	n := iterationsFromEnv()
	if os.Getenv(childEnv) == "1" {
		registerTestTracer(t)
		iterTotal = int64(n)
		evalCount.Store(0)
		fa := newTestFlowAction(t, iterFlow(n))
		h := newTestResultHandler()
		start := time.Now()
		require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
		h.wait(t)
		require.NoError(t, h.err)
		require.Equal(t, int64(n), evalCount.Load())
		fmt.Printf("ELAPSED %s\n", time.Since(start))
		return
	}
	out := runChild(t, "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON", "FLOGO_LOG_LEVEL=INFO", fmt.Sprintf("FLOW_TEST_ITERATIONS=%d", n))

	lines, flowTrace := 0, ""
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "{") || !strings.Contains(line, "iteration ") {
			continue
		}
		var entry map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "%.300s", line)
		if lines++; lines == 1 {
			flowTrace, _ = entry["trace.id"].(string)
		}
		checkOneTrace(t, line, entry, flowTrace)
	}
	require.GreaterOrEqual(t, lines, 5, "expected the checkpoint lines:\n%s", tail(out, 3000))
	if m := regexp.MustCompile(`ELAPSED (\S+)`).FindStringSubmatch(out); m != nil {
		t.Logf("%d iterations in %s", n, m[1])
	}

	heap := regexp.MustCompile(`HEAP iteration=(\d+) alloc=(\d+)`).FindAllStringSubmatch(out, -1)
	require.Len(t, heap, 2, tail(out, 3000))
	first, _ := strconv.ParseFloat(heap[0][2], 64)
	last, _ := strconv.ParseFloat(heap[1][2], 64)
	perIteration := (last - first) / float64(n-1000)
	t.Logf("retained heap: %.0f -> %.0f bytes, %.1f bytes per iteration", first, last, perIteration)
	if perIteration > 16 {
		t.Errorf("retained heap grows by %.1f bytes per iteration, want flat (<= 16)", perIteration)
	}
}

// A repeated task must always be evaluated with the concurrent group context while branches run in parallel.
func TestConcurrentRepeatedTaskKeepsItsGroupContext(t *testing.T) {
	t.Setenv("FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY", "true")
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	evalCount.Store(0)
	ctxViolations.Store(0)

	const n = 2000
	fa := newTestFlowAction(t, parallelIterFlow(n))
	h := newTestResultHandler()
	require.NoError(t, fa.Run(context.Background(), map[string]interface{}{}, h))
	h.wait(t)
	require.NoError(t, h.err)
	require.Equal(t, int64(1+3*n), evalCount.Load())
	require.Zero(t, ctxViolations.Load(), "activities evaluated without the concurrent group context")
}
