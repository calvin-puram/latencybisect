package trace

import (
	"fmt"
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
	self := n.Span.Duration()
	for _, c := range n.Children {
		self -= c.Span.Duration()
	}
	if self < 0 {
		return 0
	}
	return self
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
