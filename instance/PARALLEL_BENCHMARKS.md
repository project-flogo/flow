# FLOGO-18450 — Concurrent branch execution: benchmark & when-to-enable guide

Flag: `FLOGO_FLOW_CONCURRENT_TASK_EXECUTION` (default **off**). Off = sequential `DoStep`
loop (unchanged). On = `RunConcurrent` worker pool of `min(GOMAXPROCS, 32)` workers.

**Bottom line:** keep it **off by default** (as shipped). Enable it per workload only when a
flow has **2+ parallel branches that each do non-trivial work** — any blocking I/O
(delay/REST/DB/messaging) or **≳ ~100 µs of CPU per branch**. There it's a **2×–7× wall-clock
win**. Below that it's a pure tax (~90–100 µs fixed overhead/run, +12 allocs), making trivial
or linear flows **2–3× slower**. Default-off means no existing flow regresses unless opted in.

## Method
- Machine: AMD EPYC 7763, `windows/amd64`, `GOMAXPROCS=16` (shared cloud VM — high variance).
- `go test -run=^$ -bench=. -benchmem -benchtime=1s -count=6`, `FLOGO_LOG_LEVEL=ERROR`.
- Each iteration builds a fresh instance + runs to completion in **both** modes (symmetric;
  JSON parse excluded via `b.ResetTimer`). CPU work is a real integer loop with an atomic
  sink (not optimized away). Numbers are **medians**; `[min–max]` is the speedup envelope.
- Reproduce: `go test -run=^$ -bench='BenchmarkFlow|BenchmarkLockSetGetValue' -benchmem -benchtime=1s -count=6 github.com/project-flogo/flow/instance` (from `engine-debug`).

## Results (median of 6; speedup = seq ÷ conc, >1 = concurrent faster)

| Scenario | per-branch work | seq | conc | speedup | envelope |
|---|---|---|---|---|---|
| SeqChain8_noop (linear, no fan-out) | — | 49.7 µs | 139.5 µs | **0.36×** (slower) | 0.27–0.54× |
| Par8_noop (8 trivial branches) | ~0 | 68.1 µs | 167.4 µs | **0.41×** (slower) | 0.31–0.63× |
| Par8_cpu20 | ~20 µs | 230 µs | 279 µs | 0.82× (slower) | 0.57–1.37× |
| Par8_cpu50 | ~50 µs | 512 µs | 369 µs | 1.39× (break-even) | 1.00–2.07× |
| Par8_cpu100 | ~100 µs | 968 µs | 547 µs | **1.77×** | 1.23–2.70× |
| Par8_cpu200 | ~200 µs | 1.59 ms | 0.77 ms | **2.07×** | 1.21–2.22× |
| Par8_cpu1000 | ~1 ms | 8.95 ms | 2.25 ms | **3.98×** | 2.70–5.32× |
| Par8_cpu5000 | ~5 ms | 36.7 ms | 10.8 ms | **3.39×** | 2.71–4.18× |
| **Par8_sleep1ms (I/O-like)** | 1 ms wait | 11.9 ms | 1.77 ms | **6.72×** | 6.44–7.31× |
| Par4_cpu1000 | ~1 ms ×4 | 3.87 ms | 1.71 ms | 2.27× | 1.39–3.94× |
| Par16_cpu1000 | ~1 ms ×16 | 14.7 ms | 3.91 ms | 3.77× | 3.20–4.25× |

**Lock micro-overhead** (`SetValue`+`GetValue`, uncontended): off 87 ns → on 128 ns =
**~40 ns added per pair, 0 added allocs**. Negligible vs the per-run fixed cost.

## What the numbers mean
- **Lock hit is not the issue.** ~40 ns/access is dwarfed by a **fixed ~90–100 µs
  orchestration cost per `RunConcurrent` run** (group `context.WithCancel`, `WaitGroup`,
  `Cond`, spinning up 16 workers) — independent of branch count. Same +12 allocs / ~+0.5 KB
  per run at every width (the one rock-stable signal).
- **Crossover ≈ 50 µs of CPU per branch** (break-even at cpu50, worst-case 1.00×; robustly
  faster by cpu100). Because the synthetic CPU loop is a best case (no allocs/cache misses),
  treat **~100 µs/branch** as the practical "comfortably worth it" threshold for real
  activities.
- **CPU speedup is core-bounded** (~4× at width 8 on 16 vcpus — EPYC SMT, not 8× physical) and
  **sub-linear in width** (4-wide 2.3×, 8-wide 4.0×, 16-wide 3.8×) due to the serialized
  fan-in/join tail + oversubscription at width 16.
- **I/O speedup is the big, robust win** (6.7× at 8×1 ms sleep, tightest variance) and scales
  with **branch count, not cores** — blocked branches consume no CPU. This is the original
  FLOGO-18450 motivation (delay/REST branches).

## Decision table

| Workload shape | Recommendation | Expected effect |
|---|---|---|
| Linear flow, no fan-out | **Keep OFF** | ~2.9× slower; only cost, no benefit |
| Fan-out of trivial/instant branches | **Keep OFF** | ~2.0× slower; fixed overhead dwarfs the work |
| Fan-out, light CPU (≲ ~50 µs/branch) | **Keep OFF** (indeterminate, leans negative) | Near break-even; not worth an uncertain few-% |
| Fan-out, moderate–heavy CPU (≳ ~100 µs/branch) | **ENABLE** | 1.8×–4× faster, grows with work; core-bounded |
| Fan-out with blocking I/O (delay/REST/DB) | **ENABLE** | ~7× faster, most robust; scales past GOMAXPROCS |
| Wide fan-out (≥ GOMAXPROCS) of CPU work | **ENABLE**, expect sub-linear | Clear win but ~26% of ideal at 16-wide; leave spare cores |
| Very high flow throughput, GC-sensitive | Enable only if timing criteria above also hold | +12 allocs/~0.5 KB per run (~+3% allocs) — not a gating factor |

## Caveats
- Ratios are ~1 significant figure on a noisy shared VM (per-cell CV 3–22%); reason in ranges.
- The synthetic CPU loop is an optimistic best case → real crossover likely **higher**,
  width-efficiency **lower**. The lock cost is **uncontended**; true fan-in contention shows up
  as the serialized-tail term in width scaling.
- `-race` was **not** run here (Windows has no CGO/C compiler); use `run_race_wsl.sh`.

## Optional follow-ups
- `benchstat` (Mann-Whitney) over the samples for confidence intervals.
- `GOMAXPROCS` sweep to split fixed pool spin-up from per-goroutine cost.
- Re-run with realistic allocating/logging activities (REST/transform) to validate against
  these synthetic bounds.
- Cheap alloc trims if ever needed (not a gating reason): reuse `cancelCtx`/`Cond`/`WaitGroup`
  across `RunConcurrent` retry-loop turns; hoist the worker/stop closures to methods.
