package stats

import "math"

type Result struct {
	CountBefore int
	CountAfter  int
	MeanBefore  float64
	MeanAfter   float64
	StdDevAfter float64
	Delta       float64
	TStat       float64
	Significant bool
	Reason      string
}

type Config struct {
	MinSamples int
	MinDelta   float64
	TThreshold float64
}

func DefaultConfig() Config {
	return Config{MinSamples: 20, MinDelta: 1e6, TThreshold: 3.0}
}

func Mean(xs []int64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	return sum / float64(len(xs))
}

func Variance(xs []int64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := Mean(xs)
	var ss float64
	for _, x := range xs {
		d := float64(x) - m
		ss += d * d
	}
	return ss / float64(len(xs)-1)
}

func StdDev(xs []int64) float64 {
	return math.Sqrt(Variance(xs))
}

func Compare(before, after []int64, cfg Config) Result {
	r := Result{
		CountBefore: len(before),
		CountAfter:  len(after),
		MeanBefore:  Mean(before),
		MeanAfter:   Mean(after),
		StdDevAfter: StdDev(after),
	}
	r.Delta = r.MeanAfter - r.MeanBefore

	if len(before) < cfg.MinSamples || len(after) < cfg.MinSamples {
		r.Reason = "too few samples"
		return r
	}
	if r.Delta < cfg.MinDelta {
		r.Reason = "delta below threshold"
		return r
	}

	vb, va := Variance(before), Variance(after)
	se := math.Sqrt(vb/float64(len(before)) + va/float64(len(after)))
	if se == 0 {
		if r.Delta > 0 {
			r.TStat = math.Inf(1)
			r.Significant = true
			r.Reason = "zero variance, non-zero delta"
		} else {
			r.Reason = "zero variance, no delta"
		}
		return r
	}

	r.TStat = r.Delta / se
	if r.TStat >= cfg.TThreshold {
		r.Significant = true
		r.Reason = "significant"
	} else {
		r.Reason = "below t threshold"
	}
	return r
}
