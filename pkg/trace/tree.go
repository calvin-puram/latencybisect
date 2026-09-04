package trace

import (
	"fmt"
	"sort"
	"strings"
)

const pathSep = ">"

type Node struct {
	Span     Span
	Parent   *Node
	Children []*Node
}

func BuildTree(t Trace) (*Node, error) {
	if len(t.Spans) == 0 {
		return nil, fmt.Errorf("trace %s: no spans", t.TraceID)
	}

	nodes := make(map[string]*Node, len(t.Spans))
	for _, s := range t.Spans {
		if _, dup := nodes[s.SpanID]; dup {
			return nil, fmt.Errorf("trace %s: duplicate span id %s", t.TraceID, s.SpanID)
		}
		nodes[s.SpanID] = &Node{Span: s}
	}

	var root *Node
	for _, s := range t.Spans {
		n := nodes[s.SpanID]
		if s.ParentSpanID == "" {
			if root != nil {
				return nil, fmt.Errorf("trace %s: multiple roots (%s, %s)", t.TraceID, root.Span.SpanID, s.SpanID)
			}
			root = n
			continue
		}
		parent, ok := nodes[s.ParentSpanID]
		if !ok {
			return nil, fmt.Errorf("trace %s: span %s has missing parent %s", t.TraceID, s.SpanID, s.ParentSpanID)
		}
		n.Parent = parent
		parent.Children = append(parent.Children, n)
	}

	if root == nil {
		return nil, fmt.Errorf("trace %s: no root span", t.TraceID)
	}
	return root, nil
}

func (n *Node) PathKey() string {
	var parts []string
	for cur := n; cur != nil; cur = cur.Parent {
		parts = append(parts, cur.Span.Name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, pathSep)
}

func (n *Node) SelfTime() int64 {
	self := n.Span.Duration() - n.childCoverage()
	if self < 0 {
		return 0
	}
	return self
}

func (n *Node) childCoverage() int64 {
	if len(n.Children) == 0 {
		return 0
	}

	type interval struct{ start, end int64 }
	spans := make([]interval, 0, len(n.Children))
	for _, c := range n.Children {
		start, end := c.Span.StartNano, c.Span.EndNano
		if start < n.Span.StartNano {
			start = n.Span.StartNano
		}
		if end > n.Span.EndNano {
			end = n.Span.EndNano
		}
		if end > start {
			spans = append(spans, interval{start, end})
		}
	}
	if len(spans) == 0 {
		return 0
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var covered int64
	cur := spans[0]
	for _, s := range spans[1:] {
		if s.start > cur.end {
			covered += cur.end - cur.start
			cur = s
			continue
		}
		if s.end > cur.end {
			cur.end = s.end
		}
	}
	covered += cur.end - cur.start
	return covered
}

func (n *Node) Depth() int {
	d := 0
	for cur := n.Parent; cur != nil; cur = cur.Parent {
		d++
	}
	return d
}

func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		c.Walk(fn)
	}
}
