package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calvinpuram/latencybisect/internal/synth"
	"github.com/calvinpuram/latencybisect/pkg/trace"
)

func spec(dbMean, dbStdDev float64) synth.SpanSpec {
	return synth.SpanSpec{
		Name: "checkout", SelfMean: 2e6, SelfStdDev: 5e5,
		Children: []synth.SpanSpec{
			{
				Name: "auth.verify", SelfMean: 4e6, SelfStdDev: 8e5,
			},
			{
				Name: "inventory.check", SelfMean: 1e6, SelfStdDev: 3e5,
				Children: []synth.SpanSpec{
					{Name: "db.query", SelfMean: dbMean, SelfStdDev: dbStdDev},
				},
			},
			{
				Name: "pricing.calculate", SelfMean: 6e6, SelfStdDev: 1e6,
				Children: []synth.SpanSpec{
					{Name: "cache.get", SelfMean: 1e6, SelfStdDev: 2e5},
				},
			},
		},
	}
}

func main() {
	outDir := flag.String("out", "testdata", "directory to write before.json and after.json")
	n := flag.Int("n", 300, "traces per sample")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gendata: "+err.Error())
		os.Exit(1)
	}

	files := []struct {
		name   string
		traces []trace.Trace
	}{
		{"before.json", synth.Generate(spec(8e6, 2e6), *n, 1)},
		{"after.json", synth.Generate(spec(190e6, 40e6), *n, 2)},
	}

	for _, f := range files {
		path := filepath.Join(*outDir, f.name)
		if err := write(path, f.traces); err != nil {
			fmt.Fprintln(os.Stderr, "gendata: "+err.Error())
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d traces)\n", path, len(f.traces))
	}
}

func write(path string, traces []trace.Trace) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(traces)
}
