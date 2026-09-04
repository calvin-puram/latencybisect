package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/calvinpuram/latencybisect/pkg/adapter"
	"github.com/calvinpuram/latencybisect/pkg/bisect"
	"github.com/calvinpuram/latencybisect/pkg/report"
	"github.com/calvinpuram/latencybisect/pkg/sample"
	"github.com/calvinpuram/latencybisect/pkg/stats"
	"github.com/calvinpuram/latencybisect/pkg/trace"
)

type window struct {
	start, end time.Time
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "latencybisect: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	beforePath := flag.String("before", "", "trace json from the known-good window")
	afterPath := flag.String("after", "", "trace json from the suspect window")

	jaegerURL := flag.String("jaeger", "", "jaeger query base url, e.g. http://localhost:16686")
	service := flag.String("service", "", "service to pull traces for")
	deploy := flag.String("deploy", "", "deploy time (RFC3339); windows are taken either side of it")
	windowDur := flag.Duration("window", time.Hour, "window length either side of -deploy")
	beforeStart := flag.String("before-start", "", "RFC3339 start of the before window")
	beforeEnd := flag.String("before-end", "", "RFC3339 end of the before window")
	afterStart := flag.String("after-start", "", "RFC3339 start of the after window")
	afterEnd := flag.String("after-end", "", "RFC3339 end of the after window")
	limit := flag.Int("limit", 500, "max traces to pull per window")

	minSamples := flag.Int("min-samples", 20, "minimum observations per span before judging it")
	minDeltaMS := flag.Float64("min-delta", 1.0, "ignore regressions smaller than this many ms")
	tThreshold := flag.Float64("t", 3.0, "welch t statistic required to flag a regression")
	asJSON := flag.Bool("json", false, "emit json instead of text")
	failOnFinding := flag.Bool("fail", false, "exit 2 if any regression is found")
	flag.Parse()

	var beforeTraces, afterTraces []trace.Trace
	var err error

	switch {
	case *jaegerURL != "":
		var bw, aw window
		bw, aw, err = windows(*deploy, *windowDur, *beforeStart, *beforeEnd, *afterStart, *afterEnd)
		if err != nil {
			return err
		}
		src := adapter.NewJaeger(*jaegerURL)
		ctx := context.Background()
		if beforeTraces, err = src.Fetch(ctx, *service, bw.start, bw.end, *limit); err != nil {
			return fmt.Errorf("before window: %w", err)
		}
		if afterTraces, err = src.Fetch(ctx, *service, aw.start, aw.end, *limit); err != nil {
			return fmt.Errorf("after window: %w", err)
		}
	case *beforePath != "" && *afterPath != "":
		if beforeTraces, err = load(*beforePath); err != nil {
			return err
		}
		if afterTraces, err = load(*afterPath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("need either -before and -after files, or -jaeger with -service")
	}

	if len(beforeTraces) == 0 {
		return fmt.Errorf("before window has no traces")
	}
	if len(afterTraces) == 0 {
		return fmt.Errorf("after window has no traces")
	}

	beforeSample, err := sample.Collect(beforeTraces)
	if err != nil {
		return fmt.Errorf("before sample: %w", err)
	}
	afterSample, err := sample.Collect(afterTraces)
	if err != nil {
		return fmt.Errorf("after sample: %w", err)
	}

	cfg := stats.Config{
		MinSamples: *minSamples,
		MinDelta:   *minDeltaMS * 1e6,
		TThreshold: *tThreshold,
	}
	rep := bisect.Run(beforeSample, afterSample, cfg)

	if *asJSON {
		if err := report.JSON(os.Stdout, rep); err != nil {
			return err
		}
	} else if err := report.Text(os.Stdout, rep, beforeSample.Traces, afterSample.Traces); err != nil {
		return err
	}

	if *failOnFinding && len(rep.Findings) > 0 {
		os.Exit(2)
	}
	return nil
}

func windows(deploy string, dur time.Duration, bs, be, as, ae string) (window, window, error) {
	if deploy != "" {
		d, err := time.Parse(time.RFC3339, deploy)
		if err != nil {
			return window{}, window{}, fmt.Errorf("-deploy: %w", err)
		}
		if dur <= 0 {
			return window{}, window{}, fmt.Errorf("-window must be positive")
		}
		return window{d.Add(-dur), d}, window{d, d.Add(dur)}, nil
	}

	if bs == "" || be == "" || as == "" || ae == "" {
		return window{}, window{}, fmt.Errorf("need -deploy, or all of -before-start -before-end -after-start -after-end")
	}

	fields := []struct {
		name  string
		value string
	}{
		{"-before-start", bs},
		{"-before-end", be},
		{"-after-start", as},
		{"-after-end", ae},
	}
	parsed := make([]time.Time, len(fields))
	for i, f := range fields {
		t, err := time.Parse(time.RFC3339, f.value)
		if err != nil {
			return window{}, window{}, fmt.Errorf("%s: %w", f.name, err)
		}
		parsed[i] = t
	}

	bw := window{parsed[0], parsed[1]}
	aw := window{parsed[2], parsed[3]}
	if !bw.end.After(bw.start) {
		return window{}, window{}, fmt.Errorf("before window ends before it starts")
	}
	if !aw.end.After(aw.start) {
		return window{}, window{}, fmt.Errorf("after window ends before it starts")
	}
	return bw, aw, nil
}

func load(path string) ([]trace.Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	traces, err := trace.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(traces) == 0 {
		return nil, fmt.Errorf("%s: no traces", path)
	}
	return traces, nil
}
