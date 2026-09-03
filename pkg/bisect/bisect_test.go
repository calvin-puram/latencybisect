package bisect

import (
	"testing"

	"github.com/calvinpuram/latencybisect/internal/synth"
	"github.com/calvinpuram/latencybisect/pkg/sample"
	"github.com/calvinpuram/latencybisect/pkg/stats"
)

func baseSpec(dbMean, dbStdDev float64) synth.SpanSpec {
	return synth.SpanSpec{
		Name: "checkout", SelfMean: 2e6, SelfStdDev: 5e5,
		Children: []synth.SpanSpec{
			{
				Name: "inventory.check", SelfMean: 1e6, SelfStdDev: 3e5,
				Children: []synth.SpanSpec{
					{Name: "db.query", SelfMean: dbMean, SelfStdDev: dbStdDev},
				},
			},
			{Name: "pricing.calculate", SelfMean: 6e6, SelfStdDev: 1e6},
		},
	}
}

func collect(t *testing.T, spec synth.SpanSpec, n int, seed int64) *sample.Sample {
	t.Helper()
	s, err := sample.Collect(synth.Generate(spec, n, seed))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return s
}

func run(t *testing.T, beforeSpec, afterSpec synth.SpanSpec) Report {
	t.Helper()
	before := collect(t, beforeSpec, 300, 1)
	after := collect(t, afterSpec, 300, 2)
	return Run(before, after, stats.DefaultConfig())
}

func TestFindsRegressedLeafNotItsAncestors(t *testing.T) {
	rep := run(t, baseSpec(8e6, 2e6), baseSpec(190e6, 40e6))

	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(rep.Findings), rep.Findings)
	}

	f := rep.Findings[0]
	want := "checkout>inventory.check>db.query"
	if f.PathKey != want {
		t.Fatalf("culprit = %q, want %q", f.PathKey, want)
	}
	if f.Self.Delta < 170e6 || f.Self.Delta > 195e6 {
		t.Errorf("self delta = %v, want roughly 182ms", f.Self.Delta)
	}

	explained := map[string]bool{}
	for _, a := range f.Explains {
		explained[a] = true
	}
	for _, a := range []string{"checkout", "checkout>inventory.check"} {
		if !explained[a] {
			t.Errorf("ancestor %q missing from Explains %v", a, f.Explains)
		}
	}
}

func TestNoRegressionYieldsNoFindings(t *testing.T) {
	rep := run(t, baseSpec(8e6, 2e6), baseSpec(8e6, 2e6))
	if len(rep.Findings) != 0 {
		t.Fatalf("got findings on identical specs: %+v", rep.Findings)
	}
}

func TestSpeedupIsNotAFinding(t *testing.T) {
	rep := run(t, baseSpec(190e6, 40e6), baseSpec(8e6, 2e6))
	if len(rep.Findings) != 0 {
		t.Fatalf("speedup reported as regression: %+v", rep.Findings)
	}
}

func TestTwoIndependentRegressionsBothReported(t *testing.T) {
	before := baseSpec(8e6, 2e6)
	after := baseSpec(190e6, 40e6)
	after.Children[1].SelfMean = 90e6
	after.Children[1].SelfStdDev = 10e6

	rep := run(t, before, after)

	if len(rep.Findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(rep.Findings), rep.Findings)
	}
	if rep.Findings[0].PathKey != "checkout>inventory.check>db.query" {
		t.Errorf("first finding = %q, want db.query ranked first by delta", rep.Findings[0].PathKey)
	}
	if rep.Findings[1].PathKey != "checkout>pricing.calculate" {
		t.Errorf("second finding = %q, want pricing.calculate", rep.Findings[1].PathKey)
	}
}

func TestParentOwnWorkRegressionAttributedToParent(t *testing.T) {
	before := baseSpec(8e6, 2e6)
	after := baseSpec(8e6, 2e6)
	after.Children[0].SelfMean = 120e6
	after.Children[0].SelfStdDev = 15e6

	rep := run(t, before, after)

	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(rep.Findings), rep.Findings)
	}
	if got := rep.Findings[0].PathKey; got != "checkout>inventory.check" {
		t.Fatalf("culprit = %q, want checkout>inventory.check", got)
	}
	for _, a := range rep.Findings[0].Explains {
		if a == "checkout>inventory.check>db.query" {
			t.Error("leaf listed as explained by its own parent")
		}
	}
}

func TestPathMissingFromBeforeIsSkipped(t *testing.T) {
	before := baseSpec(8e6, 2e6)
	after := baseSpec(8e6, 2e6)
	after.Children[0].Children = append(after.Children[0].Children, synth.SpanSpec{
		Name: "cache.warm", SelfMean: 50e6, SelfStdDev: 5e6,
	})

	rep := run(t, before, after)

	key := "checkout>inventory.check>cache.warm"
	if _, ok := rep.Skipped[key]; !ok {
		t.Errorf("new span %q not in Skipped: %v", key, rep.Skipped)
	}
	for _, f := range rep.Findings {
		if f.PathKey == key {
			t.Error("new span reported as a regression")
		}
	}
}
