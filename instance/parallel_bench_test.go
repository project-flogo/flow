package instance_test

// Benchmarks for FLOGO-18450 concurrent execution, comparing the flag OFF (sequential) vs
// ON (concurrent worker pool). They isolate (a) the raw lock/unlock overhead added to the
// shared scope accessors, (b) the concurrent-machinery overhead on a flow with no
// parallelism, and (c) the speedup on parallel branches doing trivial vs CPU-bound work,
// including fan-out width scaling. Run from engine-debug, e.g.:
//
//   go test -run=^$ -bench=. -benchmem -benchtime=500ms -count=5 \
//     github.com/project-flogo/flow/instance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
)

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func modeName(concurrent bool) string {
	if concurrent {
		return "conc"
	}
	return "seq"
}

func parseDef(tb testing.TB, defJSON string) *definition.Definition {
	tb.Helper()
	defRep := &definition.DefinitionRep{}
	if err := json.Unmarshal([]byte(defJSON), defRep); err != nil {
		tb.Fatal(err)
	}
	def, err := definition.NewDefinition(defRep)
	if err != nil {
		tb.Fatal(err)
	}
	return def
}

func runFlow(tb testing.TB, def *definition.Definition, concurrent bool) {
	inst, err := instance.NewIndependentInstance("bench", "", def, nil, log.RootLogger(), context.Background())
	if err != nil {
		tb.Fatal(err)
	}
	inst.Start(nil)
	if concurrent {
		inst.RunConcurrent(0, 1<<30, nil)
	} else {
		for inst.Status() < model.FlowStatusCompleted && inst.DoStep() {
		}
	}
}

// forkDefBench: start fans out to `width` branches (each a single task of the given mode and
// work amount) that all join at a counter task.
func forkDefBench(width int, mode string, work int) string {
	var tasks, links strings.Builder
	tasks.WriteString(fmt.Sprintf(`{"id":"start","activity":{"ref":"%s","input":{"mode":"noop"}}}`, flowActivityRef))
	tasks.WriteString(fmt.Sprintf(`,{"id":"join","activity":{"ref":"%s","input":{"mode":"count"}}}`, flowActivityRef))
	lid := 1
	for i := 0; i < width; i++ {
		bid := fmt.Sprintf("b%d", i)
		tasks.WriteString(fmt.Sprintf(`,{"id":"%s","activity":{"ref":"%s","input":{"mode":"%s","ms":%d}}}`, bid, flowActivityRef, mode, work))
		links.WriteString(fmt.Sprintf(`{"id":%d,"from":"start","to":"%s","type":"label"},`, lid, bid))
		lid++
		links.WriteString(fmt.Sprintf(`{"id":%d,"from":"%s","to":"join","type":"label"}`, lid, bid))
		lid++
		if i < width-1 {
			links.WriteString(",")
		}
	}
	return fmt.Sprintf(`{"name":"forkbench","tasks":[%s],"links":[%s]}`, tasks.String(), links.String())
}

// seqChainDefBench: a linear chain start -> t0 -> t1 -> ... of `length` tasks (no fan-out).
func seqChainDefBench(length int, mode string, work int) string {
	var tasks, links strings.Builder
	tasks.WriteString(fmt.Sprintf(`{"id":"start","activity":{"ref":"%s","input":{"mode":"noop"}}}`, flowActivityRef))
	prev := "start"
	for i := 0; i < length; i++ {
		id := fmt.Sprintf("t%d", i)
		tasks.WriteString(fmt.Sprintf(`,{"id":"%s","activity":{"ref":"%s","input":{"mode":"%s","ms":%d}}}`, id, flowActivityRef, mode, work))
		links.WriteString(fmt.Sprintf(`{"id":%d,"from":"%s","to":"%s","type":"label"}`, i+1, prev, id))
		if i < length-1 {
			links.WriteString(",")
		}
		prev = id
	}
	return fmt.Sprintf(`{"name":"seqbench","tasks":[%s],"links":[%s]}`, tasks.String(), links.String())
}

func benchFlow(b *testing.B, defJSON string, concurrent bool) {
	b.Setenv(concurrentFlagEnv, boolStr(concurrent))
	def := parseDef(b, defJSON) // parse once; construct+run a fresh instance per iteration
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runFlow(b, def, concurrent)
	}
}

// BenchmarkLockSetGetValue isolates the raw cost the feature adds to the shared scope
// accessors: OFF => nil locks (no-op), ON => RWMutex lock/unlock per Set + Get.
func BenchmarkLockSetGetValue(b *testing.B) {
	for _, on := range []bool{false, true} {
		name := "FlagOff"
		if on {
			name = "FlagOn"
		}
		b.Run(name, func(b *testing.B) {
			b.Setenv(concurrentFlagEnv, boolStr(on))
			def := parseDef(b, forkDefBench(1, "noop", 0))
			inst, err := instance.NewIndependentInstance("bench", "", def, nil, log.RootLogger(), context.Background())
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = inst.SetValue("k", i)
				_, _ = inst.GetValue("k")
			}
		})
	}
}

// BenchmarkFlow compares whole-flow execution OFF vs ON across scenarios. Compare each
// "/seq" against its "/conc" sibling.
func BenchmarkFlow(b *testing.B) {
	scenarios := []struct {
		name string
		def  string
	}{
		// Overhead regime (no real per-branch work): concurrency can only cost here.
		{"SeqChain8_noop", seqChainDefBench(8, "noop", 0)}, // no parallelism at all
		{"Par8_noop", forkDefBench(8, "noop", 0)},          // trivial branches
		// CPU-work sweep at width 8: find the crossover where concurrency starts to pay off.
		{"Par8_cpu20", forkDefBench(8, "cpu", 20)},     // ~20us/branch
		{"Par8_cpu50", forkDefBench(8, "cpu", 50)},     // ~50us/branch  (pin the crossover)
		{"Par8_cpu100", forkDefBench(8, "cpu", 100)},   // ~100us/branch (pin the crossover)
		{"Par8_cpu200", forkDefBench(8, "cpu", 200)},   // ~200us/branch
		{"Par8_cpu1000", forkDefBench(8, "cpu", 1000)}, // ~1ms/branch
		{"Par8_cpu5000", forkDefBench(8, "cpu", 5000)}, // ~5ms/branch
		// I/O-like wait (the original motivation: delay/REST branches): the big win.
		{"Par8_sleep1ms", forkDefBench(8, "sleep", 1)},
		// Width scaling at a fixed ~1ms/branch CPU cost.
		{"Par4_cpu1000", forkDefBench(4, "cpu", 1000)},
		{"Par16_cpu1000", forkDefBench(16, "cpu", 1000)},
	}
	for _, s := range scenarios {
		for _, on := range []bool{false, true} {
			b.Run(s.name+"/"+modeName(on), func(b *testing.B) {
				benchFlow(b, s.def, on)
			})
		}
	}
}
