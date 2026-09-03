package bisect

import (
	"sort"

	"github.com/calvinpuram/latencybisect/pkg/sample"
	"github.com/calvinpuram/latencybisect/pkg/stats"
)

type Finding struct {
	PathKey  string
	Depth    int
	Self     stats.Result
	Total    stats.Result
	Explains []string
}

type Report struct {
	Findings []Finding
	Skipped  map[string]string
}

func Run(before, after *sample.Sample, cfg stats.Config) Report {
	rep := Report{Skipped: make(map[string]string)}

	selfRes := make(map[string]stats.Result)
	totalRes := make(map[string]stats.Result)

	for key, a := range after.ByPath {
		b, ok := before.ByPath[key]
		if !ok {
			rep.Skipped[key] = "not present in before sample"
			continue
		}
		selfRes[key] = stats.Compare(b.SelfTimes, a.SelfTimes, cfg)
		totalRes[key] = stats.Compare(b.Durations, a.Durations, cfg)
	}

	for key, res := range selfRes {
		if !res.Significant {
			continue
		}
		rep.Findings = append(rep.Findings, Finding{
			PathKey:  key,
			Depth:    after.ByPath[key].Depth,
			Self:     res,
			Total:    totalRes[key],
			Explains: explains(key, selfRes, totalRes),
		})
	}

	sort.Slice(rep.Findings, func(i, j int) bool {
		if rep.Findings[i].Self.Delta != rep.Findings[j].Self.Delta {
			return rep.Findings[i].Self.Delta > rep.Findings[j].Self.Delta
		}
		return rep.Findings[i].PathKey < rep.Findings[j].PathKey
	})

	return rep
}

func explains(key string, selfRes, totalRes map[string]stats.Result) []string {
	var out []string
	for ancestor := sample.ParentPath(key); ancestor != ""; ancestor = sample.ParentPath(ancestor) {
		tr, ok := totalRes[ancestor]
		if !ok || !tr.Significant {
			continue
		}
		if sr, ok := selfRes[ancestor]; ok && sr.Significant {
			continue
		}
		out = append(out, ancestor)
	}
	return out
}
