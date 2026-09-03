package sample

import (
	"strings"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

const pathSep = ">"

type PathStats struct {
	PathKey   string
	Depth     int
	SelfTimes []int64
	Durations []int64
}

func (p *PathStats) Count() int {
	return len(p.SelfTimes)
}

type Sample struct {
	ByPath map[string]*PathStats
	Traces int
}

func Collect(traces []trace.Trace) (*Sample, error) {
	s := &Sample{ByPath: make(map[string]*PathStats)}

	for _, t := range traces {
		root, err := trace.BuildTree(t)
		if err != nil {
			return nil, err
		}
		root.Walk(func(n *trace.Node) {
			key := n.PathKey()
			ps, ok := s.ByPath[key]
			if !ok {
				ps = &PathStats{PathKey: key, Depth: n.Depth()}
				s.ByPath[key] = ps
			}
			ps.SelfTimes = append(ps.SelfTimes, n.SelfTime())
			ps.Durations = append(ps.Durations, n.Span.Duration())
		})
		s.Traces++
	}

	return s, nil
}

func ParentPath(key string) string {
	i := strings.LastIndex(key, pathSep)
	if i < 0 {
		return ""
	}
	return key[:i]
}
