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

// Check is a condition asked of one Item: its shape as Tree draws it, with
// whether each part held and what each comparison's key holds. A group's
// parts are in Parts, not Items.
type Check struct {
	Node
	// Met is whether the part held, its Not applied.
	Met bool `json:"met"`
	// Held is what a comparison's key holds for the Item.
	Held  []string `json:"held,omitempty"`
	Parts []Check  `json:"parts,omitempty"`
}

// Explain asks e of env as Eval does, and records every part, each asked
// even where Eval would stop early, so a reader sees why it held or not.
func Explain(e Expr, env Env) Check {
	switch x := e.(type) {
	case not:
		c := Explain(x.e, env)
		c.Not, c.Met = !c.Not, !c.Met
		return c
	case both:
		op := map[bool]string{true: "&&", false: "||"}[x.and]
		out := Check{Node: Node{Op: op}, Met: x.Eval(env)}
		for _, side := range []Expr{x.a, x.b} {
			c := Explain(side, env)
			if c.Op == op && !c.Not {
				out.Parts = append(out.Parts, c.Parts...)
			} else {
				out.Parts = append(out.Parts, c)
			}
		}
		return out
	case cmp:
		return Check{Node: Tree(x), Met: x.Eval(env), Held: env.Values(x.key)}
	}
	return Check{}
}
