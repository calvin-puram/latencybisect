# latencybisect

Finds which span in a distributed trace is responsible for a latency regression.

Aggregate latency graphs tell you P99 went from 120ms to 340ms after Tuesday's deploy. They don't tell you which of the twenty spans in the request is at fault. That part is still a human opening traces in Jaeger and eyeballing flamegraphs, which doesn't scale and is slowest exactly when you need it fastest.

The hard part isn't spotting slow spans. It's that a slow leaf makes every one of its ancestors look slow too. If `db.query` gains 180ms, then `inventory.check` and `checkout` each gain 180ms as well, and a naive diff reports all three.

## The approach

Compare **self time** (a span's duration minus its children's), not total duration.

- Total duration regressed → this span *or anything beneath it* got slower.
- Self time regressed → *this span's own work* got slower.

Only the second is causal. Ancestors whose duration grew but whose self time held flat are collateral, and get reported as such instead of as suspects.

Concretely, on the bundled fixtures:

```
$ latencybisect -before testdata/before.json -after testdata/after.json
compared 300 before traces against 300 after traces

1. checkout>inventory.check>db.query
   self time  8.0ms -> 185.6ms  (+177.7ms, t=79.1)
   spread     +/-38.8ms after, n=300/300
   total time 8.0ms -> 185.6ms (+177.7ms)
   slow because of this, not independently:
     checkout>inventory.check
     checkout
```

`checkout` and `inventory.check` both gained the same 177.7ms of wall time. Neither is the cause.

## How it works

1. **Match spans across samples.** Span and trace IDs are unique per request, so they can't identify "the same operation" in two different traces. The key is the root-to-node path of span names: `checkout>inventory.check>db.query`. Using the full path rather than the bare name matters because the same operation often appears in several places in a call graph, and `cache.get` under `pricing` is not `cache.get` under `inventory` — averaging them together smears a real regression into noise.

2. **Build per-position distributions.** Each path accumulates one self-time observation per trace. Raw samples are kept rather than running means, because plenty of real regressions are distributional: a span going from a steady 5ms to a bimodal 5ms/200ms barely moves the mean but is very much a problem.

3. **Test for significance.** Welch's t-test per path. Welch rather than Student's because a regressed span almost always gets noisier as well as slower (8ms±2 becoming 190ms±40), and assuming equal variance there is simply wrong.

4. **Attribute.** Every path with a significant self-time regression is a finding. Ancestors with a significant *total* regression but no self-time regression are attached to it as collateral. Two genuinely independent regressions produce two findings — a parent's own work slowing down explains nothing about its child's, since self time is disjoint.

## Guards

Statistical significance and operational importance are different things, so a finding needs all three:

| Flag | Default | Why |
|---|---|---|
| `-min-samples` | 20 | Refuse to judge a path seen in a handful of traces. |
| `-min-delta` | 1.0ms | A 0.2ms shift across 5000 samples is rock solid and completely irrelevant. |
| `-t` | 3.0 | Welch t statistic required to flag. |

Spans present in only one sample (new code paths) are skipped and counted, never treated as infinite regressions. When a path is not flagged, the result carries the reason it was not — `too few samples`, `delta below threshold`, `below t threshold` — so declining to report is distinguishable from failing to work.

## Usage

```sh
go build -o latencybisect ./cmd/latencybisect

# generate fixtures with a known injected regression
go run ./cmd/gendata

./latencybisect -before testdata/before.json -after testdata/after.json
./latencybisect -before before.json -after after.json -json
./latencybisect -before before.json -after after.json -fail   # exit 2 on any finding, for CI
```

Input is a JSON array of traces:

```json
[{"traceId":"t0","spans":[
  {"spanId":"s1","parentSpanId":"","name":"checkout","startNano":0,"endNano":21000000},
  {"spanId":"s2","parentSpanId":"s1","name":"db.query","startNano":1000000,"endNano":9000000}
]}]
```

This is a deliberate simplification of the OpenTelemetry export format — same fields that matter (span id, parent, name, start/end unix nanos), without the `resourceSpans`/`scopeSpans` nesting. Real backends get an adapter rather than a schema rewrite.

## Testing

Correctness is checked against synthetic traces with a known culprit. `internal/synth` builds traces bottom-up from per-span self-time distributions and lays children out sequentially, so the self time recovered from timestamps equals the self time injected — the ground truth is real, not circular.

```sh
go test ./...
```

The cases that matter: a regressed leaf is reported while its ancestors are not; a regressed parent is reported while its unchanged leaf is not; two independent regressions both surface, ranked; a speedup is not a regression; a new span is skipped rather than flagged.

## Known limitations

- **Concurrent children.** Self time is `duration - sum(children)`, which assumes children don't overlap. With parallel calls it understates self time, and currently clamps at zero rather than going negative. The fix is to subtract the union of child intervals instead of their sum.
- **No backend adapter yet.** Reads JSON files; a Jaeger/Tempo query layer is the next step.
- **Mean-based significance.** Welch's t-test on means catches most regressions but is weakest on exactly the bimodal case argued for above. Comparing tail quantiles would catch more.

## Next

- Jaeger and Tempo query adapters, with deploy-anchored window selection
- Interval-union self time for concurrent children
- Quantile comparison alongside the mean test
- Correlate the identified span with recent deploys and config changes to that service

No external dependencies; standard library only.
