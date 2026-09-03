package stats

import (
	"math"
	"math/rand"
	"testing"
)

func normal(r *rand.Rand, n int, mean, stddev float64) []int64 {
	xs := make([]int64, n)
	for i := range xs {
		xs[i] = int64(r.NormFloat64()*stddev + mean)
	}
	return xs
}

func testConfig() Config {
	return Config{MinSamples: 20, MinDelta: 1e6, TThreshold: 3.0}
}

func TestMeanVarianceStdDev(t *testing.T) {
	xs := []int64{2, 4, 4, 4, 5, 5, 7, 9}
	if got := Mean(xs); got != 5 {
		t.Errorf("Mean = %v, want 5", got)
	}
	if got := Variance(xs); math.Abs(got-4.571428) > 1e-5 {
		t.Errorf("Variance = %v, want 4.571428", got)
	}
	if got := StdDev(xs); math.Abs(got-2.13809) > 1e-4 {
		t.Errorf("StdDev = %v, want 2.13809", got)
	}
}

func TestMeanVarianceEdgeCases(t *testing.T) {
	if got := Mean(nil); got != 0 {
		t.Errorf("Mean(nil) = %v, want 0", got)
	}
	if got := Variance([]int64{5}); got != 0 {
		t.Errorf("Variance(single) = %v, want 0", got)
	}
}

func TestCompareDetectsRealRegression(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	before := normal(r, 200, 8e6, 2e6)
	after := normal(r, 200, 190e6, 40e6)

	res := Compare(before, after, testConfig())
	if !res.Significant {
		t.Fatalf("not significant: %+v", res)
	}
	if res.Delta < 170e6 || res.Delta > 190e6 {
		t.Errorf("Delta = %v, want roughly 182ms", res.Delta)
	}
}

func TestCompareIgnoresNoise(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	before := normal(r, 200, 8e6, 2e6)
	after := normal(r, 200, 8e6, 2e6)

	res := Compare(before, after, testConfig())
	if res.Significant {
		t.Errorf("noise flagged as significant: %+v", res)
	}
}

func TestCompareIgnoresTinyButConsistentShift(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	before := normal(r, 5000, 8e6, 1e5)
	after := normal(r, 5000, 8.2e6, 1e5)

	res := Compare(before, after, testConfig())
	if res.Significant {
		t.Errorf("0.2ms shift flagged despite MinDelta: %+v", res)
	}
	if res.Reason != "delta below threshold" {
		t.Errorf("Reason = %q, want delta below threshold", res.Reason)
	}
}

func TestCompareRefusesThinData(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	before := normal(r, 3, 8e6, 2e6)
	after := normal(r, 3, 190e6, 40e6)

	res := Compare(before, after, testConfig())
	if res.Significant {
		t.Errorf("judged on 3 samples: %+v", res)
	}
	if res.Reason != "too few samples" {
		t.Errorf("Reason = %q, want too few samples", res.Reason)
	}
}

func TestCompareIgnoresImprovement(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	before := normal(r, 200, 190e6, 40e6)
	after := normal(r, 200, 8e6, 2e6)

	res := Compare(before, after, testConfig())
	if res.Significant {
		t.Errorf("speedup flagged as regression: %+v", res)
	}
	if res.Delta >= 0 {
		t.Errorf("Delta = %v, want negative", res.Delta)
	}
}

func TestCompareHandlesUnequalVariance(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	before := normal(r, 200, 50e6, 1e6)
	after := normal(r, 200, 54e6, 60e6)

	res := Compare(before, after, testConfig())
	if res.Significant {
		t.Errorf("4ms shift swamped by 60ms spread flagged: %+v", res)
	}
}

func TestCompareZeroVariance(t *testing.T) {
	before := make([]int64, 50)
	after := make([]int64, 50)
	for i := range before {
		before[i] = 8e6
		after[i] = 190e6
	}

	res := Compare(before, after, testConfig())
	if !res.Significant {
		t.Fatalf("constant shift not flagged: %+v", res)
	}
	if !math.IsInf(res.TStat, 1) {
		t.Errorf("TStat = %v, want +Inf", res.TStat)
	}
}
