package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/calvinpuram/latencybisect/pkg/bisect"
	"github.com/calvinpuram/latencybisect/pkg/stats"
)

func finding(path string, before, after float64, explains ...string) bisect.Finding {
	res := stats.Result{
		CountBefore: 300,
		CountAfter:  300,
		MeanBefore:  before,
		MeanAfter:   after,
		StdDevAfter: 38.8e6,
		Delta:       after - before,
		TStat:       79.1,
		Significant: true,
		Reason:      "significant",
	}
	return bisect.Finding{PathKey: path, Depth: 2, Self: res, Total: res, Explains: explains}
}

func renderText(t *testing.T, rep bisect.Report) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Text(&buf, rep, 300, 300); err != nil {
		t.Fatalf("Text: %v", err)
	}
	return buf.String()
}

func TestTextReportsCulpritAndCollateral(t *testing.T) {
	rep := bisect.Report{
		Findings: []bisect.Finding{
			finding("checkout>inventory.check>db.query", 8e6, 185.6e6, "checkout>inventory.check", "checkout"),
		},
		Skipped: map[string]string{},
	}

	out := renderText(t, rep)

	for _, want := range []string{
		"compared 300 before traces against 300 after traces",
		"1. checkout>inventory.check>db.query",
		"8.0ms -> 185.6ms",
		"+177.6ms",
		"t=79.1",
		"n=300/300",
		"slow because of this, not independently:",
		"checkout>inventory.check",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTextNoFindings(t *testing.T) {
	out := renderText(t, bisect.Report{Skipped: map[string]string{}})

	if !strings.Contains(out, "no significant self-time regressions") {
		t.Errorf("output missing clean verdict:\n%s", out)
	}
	if strings.Contains(out, "slow because of this") {
		t.Errorf("collateral section rendered with no findings:\n%s", out)
	}
}

func TestTextReportsSkippedCount(t *testing.T) {
	rep := bisect.Report{
		Findings: []bisect.Finding{finding("a>b", 8e6, 100e6)},
		Skipped:  map[string]string{"a>c": "not present in before sample", "a>d": "not present in before sample"},
	}

	out := renderText(t, rep)
	if !strings.Contains(out, "2 paths skipped") {
		t.Errorf("skipped count missing:\n%s", out)
	}
}

func TestTextSkippedShownWithNoFindings(t *testing.T) {
	rep := bisect.Report{Skipped: map[string]string{"a>c": "not present in before sample"}}

	out := renderText(t, rep)
	if !strings.Contains(out, "1 paths skipped") {
		t.Errorf("skipped count missing on clean run:\n%s", out)
	}
}

func TestTextOmitsCollateralSectionWhenEmpty(t *testing.T) {
	rep := bisect.Report{
		Findings: []bisect.Finding{finding("a>b", 8e6, 100e6)},
		Skipped:  map[string]string{},
	}

	if out := renderText(t, rep); strings.Contains(out, "slow because of this") {
		t.Errorf("collateral header rendered with no ancestors:\n%s", out)
	}
}

func TestTextNumbersFindings(t *testing.T) {
	rep := bisect.Report{
		Findings: []bisect.Finding{
			finding("a>slow", 8e6, 200e6),
			finding("a>slower", 8e6, 100e6),
		},
		Skipped: map[string]string{},
	}

	out := renderText(t, rep)
	if !strings.Contains(out, "1. a>slow") || !strings.Contains(out, "2. a>slower") {
		t.Errorf("findings not numbered in order:\n%s", out)
	}
}

func TestJSONRoundTrips(t *testing.T) {
	rep := bisect.Report{
		Findings: []bisect.Finding{finding("checkout>db.query", 8e6, 185.6e6, "checkout")},
		Skipped:  map[string]string{"checkout>cache.get": "not present in before sample"},
	}

	var buf bytes.Buffer
	if err := JSON(&buf, rep); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got bisect.Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(got.Findings))
	}
	if got.Findings[0].PathKey != "checkout>db.query" {
		t.Errorf("PathKey = %q, want checkout>db.query", got.Findings[0].PathKey)
	}
	if got.Findings[0].Self.Delta != rep.Findings[0].Self.Delta {
		t.Errorf("Delta = %v, want %v", got.Findings[0].Self.Delta, rep.Findings[0].Self.Delta)
	}
	if got.Skipped["checkout>cache.get"] == "" {
		t.Error("skipped map lost in round trip")
	}
}

func TestMillisecondFormatting(t *testing.T) {
	cases := []struct {
		nanos float64
		want  string
	}{
		{0, "0.0ms"},
		{1e6, "1.0ms"},
		{185_600_000, "185.6ms"},
		{-4.2e6, "-4.2ms"},
	}
	for _, tc := range cases {
		if got := ms(tc.nanos); got != tc.want {
			t.Errorf("ms(%v) = %q, want %q", tc.nanos, got, tc.want)
		}
	}
}
