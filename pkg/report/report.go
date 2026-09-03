package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/calvinpuram/latencybisect/pkg/bisect"
)

func ms(nanos float64) string {
	return fmt.Sprintf("%.1fms", nanos/1e6)
}

func Text(w io.Writer, rep bisect.Report, beforeTraces, afterTraces int) error {
	var b strings.Builder

	fmt.Fprintf(&b, "compared %d before traces against %d after traces\n\n", beforeTraces, afterTraces)

	if len(rep.Findings) == 0 {
		b.WriteString("no significant self-time regressions\n")
		if len(rep.Skipped) > 0 {
			fmt.Fprintf(&b, "%d paths skipped\n", len(rep.Skipped))
		}
		_, err := io.WriteString(w, b.String())
		return err
	}

	for i, f := range rep.Findings {
		fmt.Fprintf(&b, "%d. %s\n", i+1, f.PathKey)
		fmt.Fprintf(&b, "   self time  %s -> %s  (+%s, t=%.1f)\n",
			ms(f.Self.MeanBefore), ms(f.Self.MeanAfter), ms(f.Self.Delta), f.Self.TStat)
		fmt.Fprintf(&b, "   spread     +/-%s after, n=%d/%d\n",
			ms(f.Self.StdDevAfter), f.Self.CountBefore, f.Self.CountAfter)

		if f.Total.Significant {
			fmt.Fprintf(&b, "   total time %s -> %s (+%s)\n",
				ms(f.Total.MeanBefore), ms(f.Total.MeanAfter), ms(f.Total.Delta))
		}

		if len(f.Explains) > 0 {
			b.WriteString("   slow because of this, not independently:\n")
			for _, a := range f.Explains {
				b.WriteString("     " + a + "\n")
			}
		}
		b.WriteString("\n")
	}

	if len(rep.Skipped) > 0 {
		fmt.Fprintf(&b, "%d paths skipped (not in both samples)\n", len(rep.Skipped))
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func JSON(w io.Writer, rep bisect.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
