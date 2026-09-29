package expr

// Node is a parsed condition's shape, for drawing it: a group of
// conditions joined by one Op, or one comparison. Chains of one Op are
// flattened into one group, and Not marks a negated group or comparison.
type Node struct {
	// Op is "&&" or "||" for a group, empty for a comparison.
	Op    string `json:"op,omitempty"`
	Items []Node `json:"items,omitempty"`
	// Key, Cmp and Values are a comparison's: Cmp is ==, ~, in or has.
	Key    string   `json:"key,omitempty"`
	Cmp    string   `json:"cmp,omitempty"`
	Values []string `json:"values,omitempty"`
	Not    bool     `json:"not,omitempty"`
}

// Tree is e's shape.
func Tree(e Expr) Node {
	switch x := e.(type) {
	case not:
		n := Tree(x.e)
		n.Not = !n.Not
		return n
	case both:
		op := map[bool]string{true: "&&", false: "||"}[x.and]
		var items []Node
		for _, side := range []Expr{x.a, x.b} {
			n := Tree(side)
			if n.Op == op && !n.Not {
				items = append(items, n.Items...)
			} else {
				items = append(items, n)
			}
		}
		return Node{Op: op, Items: items}
	case cmp:
		name := map[op]string{opIs: "==", opIn: "in", opContains: "~", opFilled: "has"}[x.op]
		return Node{Key: x.key, Cmp: name, Values: x.want}
	}
	return Node{}
}
