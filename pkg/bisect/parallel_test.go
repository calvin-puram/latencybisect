package bisect

import (
	"testing"

	"github.com/calvinpuram/latencybisect/internal/synth"
	"github.com/calvinpuram/latencybisect/pkg/trace"
)

func fanoutSpec(shipMean, gatewaySelfMean float64) synth.SpanSpec {
	return synth.SpanSpec{
		Name: "checkout", SelfMean: 2e6, SelfStdDev: 5e5,
		Children: []synth.SpanSpec{
			{
				Name: "gateway.fanout", SelfMean: gatewaySelfMean, SelfStdDev: 5e5, Parallel: true,
				Children: []synth.SpanSpec{
					{Name: "inventory.lookup", SelfMean: 30e6, SelfStdDev: 3e6},
					{Name: "shipping.quote", SelfMean: shipMean, SelfStdDev: 3e6},
					{Name: "tax.calculate", SelfMean: 25e6, SelfStdDev: 3e6},
				},
			},
		},
	}
}

func TestParallelChildrenOverlapInGeneratedTraces(t *testing.T) {
	traces := synth.Generate(fanoutSpec(28e6, 3e6), 1, 11)
	root, err := trace.BuildTree(traces[0])
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	fanout := root.Children[0]
	if len(fanout.Children) != 3 {
		t.Fatalf("fanout has %d children, want 3", len(fanout.Children))
	}

	a, b := fanout.Children[0], fanout.Children[1]
	if a.Span.StartNano != b.Span.StartNano {
		t.Errorf("parallel children start at %d and %d, want the same instant", a.Span.StartNano, b.Span.StartNano)
	}

	var sum int64
	for _, c := range fanout.Children {
		sum += c.Span.Duration()
	}
	if sum <= fanout.Span.Duration() {
		t.Fatalf("children sum %d does not exceed parent %d, so this is not testing overlap", sum, fanout.Span.Duration())
	}

	if got := fanout.SelfTime(); got <= 0 {
		t.Errorf("fanout self time = %d, want positive under interval union", got)
	}
}

func TestRegressionUnderParallelFanoutIsFound(t *testing.T) {
	rep := run(t, fanoutSpec(28e6, 3e6), fanoutSpec(210e6, 3e6))

	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(rep.Findings), rep.Findings)
	}
	want := "checkout>gateway.fanout>shipping.quote"
	if rep.Findings[0].PathKey != want {
		t.Fatalf("culprit = %q, want %q", rep.Findings[0].PathKey, want)
	}

	explained := map[string]bool{}
	for _, a := range rep.Findings[0].Explains {
		explained[a] = true
	}
	if !explained["checkout>gateway.fanout"] {
		t.Errorf("fanout parent not listed as collateral: %v", rep.Findings[0].Explains)
	}
}

func TestFanoutOwnWorkRegressionFoundDespiteParallelChildren(t *testing.T) {
	rep := run(t, fanoutSpec(28e6, 3e6), fanoutSpec(28e6, 120e6))

	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(rep.Findings), rep.Findings)
	}
	if got := rep.Findings[0].PathKey; got != "checkout>gateway.fanout" {
		t.Fatalf("culprit = %q, want checkout>gateway.fanout", got)
	}
	if d := rep.Findings[0].Self.Delta; d < 100e6 {
		t.Errorf("self delta = %v, want roughly 117ms", d)
	}
}

func TestSlowestParallelBranchNotBlamedWhenAnotherRegresses(t *testing.T) {
	rep := run(t, fanoutSpec(28e6, 3e6), fanoutSpec(210e6, 3e6))

	for _, f := range rep.Findings {
		if f.PathKey == "checkout>gateway.fanout>inventory.lookup" || f.PathKey == "checkout>gateway.fanout>tax.calculate" {
			t.Errorf("unchanged parallel sibling %q reported as a regression", f.PathKey)
		}
	}
}
