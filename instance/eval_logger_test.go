package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/project-flogo/core/support/log"
	"github.com/stretchr/testify/assert"
)

type fakeSpan struct{ traceID, spanID string }

func (s *fakeSpan) TraceObject() interface{}            { return s }
func (s *fakeSpan) SetTags(map[string]interface{}) bool { return true }
func (s *fakeSpan) SetTag(string, interface{}) bool     { return true }
func (s *fakeSpan) LogKV(map[string]interface{}) bool   { return true }
func (s *fakeSpan) TraceID() string                     { return s.traceID }
func (s *fakeSpan) SpanID() string                      { return s.spanID }

const evalLoggerChildEnv = "FLOW_EVAL_LOGGER_TEST_CHILD"

var evalLoggerChildRuns atomic.Int64

// runEvalLoggerChild re-runs the test in a fresh process with FLOGO_LOG_CTX=true, which is read once at start-up.
func runEvalLoggerChild(t *testing.T) string {
	t.Helper()
	timeout := 10 * time.Minute
	if deadline, ok := t.Deadline(); ok {
		timeout = time.Until(deadline) * 9 / 10
	}
	if timeout < time.Second {
		timeout = time.Second
	}
	args := []string{"-test.run=^" + t.Name() + "$", "-test.count=1", "-test.v", "-test.timeout=" + timeout.String()}
	if testing.CoverMode() != "" {
		dir := os.Getenv("TEST_CHILD_COVERDIR")
		if dir == "" {
			t.Fatal("cover mode is on: set TEST_CHILD_COVERDIR to keep the child process's cover profile")
		}
		name := fmt.Sprintf("child-evallogger-%d-%d.out", os.Getpid(), evalLoggerChildRuns.Add(1))
		args = append(args, "-test.coverprofile="+filepath.Join(dir, name))
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), evalLoggerChildEnv+"=1", "FLOGO_LOG_CTX=true", "FLOGO_LOG_FORMAT=JSON",
		"FLOGO_LOG_LEVEL=INFO", "FLOGO_LOG_CONSOLE_STREAM=stdout")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed: %v\n%s", err, out)
	}
	return string(out)
}

// Each evaluation derives from the pristine base, captured once when unset: a repeat carries one trace.id, its own.
func TestEvalLoggerDerivesFromPristineBase(t *testing.T) {
	if os.Getenv(evalLoggerChildEnv) != "1" {
		out := runEvalLoggerChild(t)
		lines := 0
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "{") || !strings.Contains(line, "second evaluation") {
				continue
			}
			lines++
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("unparsable line %q: %v", line, err)
			}
			if n := strings.Count(line, `"trace.id"`); n != 1 || entry["trace.id"] != "2" || entry["span.id"] != "b" {
				t.Errorf("%d trace.id fields, trace.id %v and span.id %v, want one with 2 and b: %s", n, entry["trace.id"],
					entry["span.id"], line)
			}
			if n := strings.Count(line, "[traceID:"); n != 1 || !strings.Contains(line, "[traceID: 2] [spanID: b]") {
				t.Errorf("%d trace prefixes, want one for trace 2 and span b: %s", n, line)
			}
		}
		if lines != 1 {
			t.Fatalf("expected the second evaluation's line once, found %d:\n%s", lines, out)
		}
		return
	}
	inst, err := NewIndependentInstance("eval-logger", "", getDef(), nil, log.RootLogger(), context.Background())
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	ti := NewTaskInst(inst.Instance, inst.flowDef.GetTask("LogStart"))
	pristine := ti.logger
	ti.baseLogger = nil

	ti.logger = ti.evalLogger(&fakeSpan{traceID: "1", spanID: "a"})
	ti.logger = ti.evalLogger(&fakeSpan{traceID: "2", spanID: "b"})
	ti.logger.Info("second evaluation")

	assert.Equal(t, pristine, ti.baseLogger, "the base must be the pristine logger, not a derived one")
	assert.Equal(t, "2", ti.logger.GetTracingContext()[log.KeyTraceID])
	assert.Nil(t, pristine.GetTracingContext(), "the pristine logger must not be modified")
	assert.Nil(t, ti.evalLogger(nil).GetTracingContext(), "no span, no tracing context")
}
