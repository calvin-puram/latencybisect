package synth

import (
	"fmt"
	"math/rand"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

type SpanSpec struct {
	Name       string
	SelfMean   float64
	SelfStdDev float64
	Children   []SpanSpec
}

type generator struct {
	rnd    *rand.Rand
	nextID int
}

func Generate(spec SpanSpec, n int, seed int64) []trace.Trace {
	g := &generator{rnd: rand.New(rand.NewSource(seed))}
	traces := make([]trace.Trace, 0, n)
	for i := 0; i < n; i++ {
		g.nextID = 0
		traceID := fmt.Sprintf("t%d", i)
		var spans []trace.Span
		g.build(spec, traceID, "", 0, &spans)
		traces = append(traces, trace.Trace{TraceID: traceID, Spans: spans})
	}
	return traces
}

func (g *generator) build(spec SpanSpec, traceID, parentID string, start int64, out *[]trace.Span) int64 {
	g.nextID++
	id := fmt.Sprintf("s%d", g.nextID)

	self := int64(g.rnd.NormFloat64()*spec.SelfStdDev + spec.SelfMean)
	if self < 0 {
		self = 0
	}

	idx := len(*out)
	*out = append(*out, trace.Span{
		TraceID:      traceID,
		SpanID:       id,
		ParentSpanID: parentID,
		Name:         spec.Name,
		StartNano:    start,
	})

	cursor := start + self/2
	for _, child := range spec.Children {
		cursor = g.build(child, traceID, id, cursor, out)
	}

	end := cursor + (self - self/2)
	(*out)[idx].EndNano = end
	return end
}
