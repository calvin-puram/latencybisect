package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/calvinpuram/latencybisect/pkg/bisect"
	"github.com/calvinpuram/latencybisect/pkg/report"
	"github.com/calvinpuram/latencybisect/pkg/sample"
	"github.com/calvinpuram/latencybisect/pkg/stats"
	"github.com/calvinpuram/latencybisect/pkg/trace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "latencybisect: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	beforePath := flag.String("before", "", "trace json from the known-good window")
	afterPath := flag.String("after", "", "trace json from the suspect window")
	minSamples := flag.Int("min-samples", 20, "minimum observations per span before judging it")
	minDeltaMS := flag.Float64("min-delta", 1.0, "ignore regressions smaller than this many ms")
	tThreshold := flag.Float64("t", 3.0, "welch t statistic required to flag a regression")
	asJSON := flag.Bool("json", false, "emit json instead of text")
	failOnFinding := flag.Bool("fail", false, "exit 2 if any regression is found")
	flag.Parse()

	if *beforePath == "" || *afterPath == "" {
		return fmt.Errorf("both -before and -after are required")
	}

	beforeTraces, err := load(*beforePath)
	if err != nil {
		return err
	}
	afterTraces, err := load(*afterPath)
	if err != nil {
		return err
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
