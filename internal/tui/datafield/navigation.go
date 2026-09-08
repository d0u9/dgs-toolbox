// Package datafield provides spatial focus navigation between workspace regions.
package datafield

type Field struct {
	ID  string
	Row int
	Col int
}

type Navigator struct {
	fields  []Field
	current int
	bounds  map[string]Bounds
}

type Bounds struct{ X, Y, Width, Height int }

func New(fields ...Field) Navigator {
	return Navigator{fields: append([]Field(nil), fields...), bounds: make(map[string]Bounds)}
}

func (n Navigator) Current() string {
	if n.current < 0 || n.current >= len(n.fields) {
		return ""
	}
	return n.fields[n.current].ID
}

func (n *Navigator) SetBounds(id string, bounds Bounds) {
	if n.bounds == nil {
		n.bounds = make(map[string]Bounds)
	}
	n.bounds[id] = bounds
}

// HitAt returns the field containing a workspace-relative cell without
// changing focus. It is useful for hover-based interactions such as scrolling.
func (n Navigator) HitAt(x, y int) string {
	for _, field := range n.fields {
		bounds, ok := n.bounds[field.ID]
		if ok && x >= bounds.X && x < bounds.X+bounds.Width && y >= bounds.Y && y < bounds.Y+bounds.Height {
			return field.ID
		}
	}
	return ""
}

// FocusAt focuses the field containing the workspace-relative cell.
func (n *Navigator) FocusAt(x, y int) bool {
	id := n.HitAt(x, y)
	if id == "" {
		return false
	}
	n.Set(id)
	return true
}

func (n *Navigator) Set(id string) {
	for index, field := range n.fields {
		if field.ID == id {
			n.current = index
			return
		}
	}
}

// Move handles Alt+h/j/k/l and their Alt+arrow equivalents. It chooses the
// nearest field in the requested spatial direction.
func (n *Navigator) Move(key string) bool {
	dr, dc := 0, 0
	switch key {
	case "alt+h", "alt+left":
		dc = -1
	case "alt+l", "alt+right":
		dc = 1
	case "alt+k", "alt+up":
		dr = -1
	case "alt+j", "alt+down":
		dr = 1
	default:
		return false
	}
	if len(n.fields) == 0 {
		return true
	}
	current := n.fields[n.current]
	best, bestScore := -1, int(^uint(0)>>1)
	for index, candidate := range n.fields {
		rDiff, cDiff := candidate.Row-current.Row, candidate.Col-current.Col
		if (dr < 0 && rDiff >= 0) || (dr > 0 && rDiff <= 0) || (dc < 0 && cDiff >= 0) || (dc > 0 && cDiff <= 0) {
			continue
		}
		score := abs(rDiff)*10 + abs(cDiff)*10
		if dr != 0 {
			score += abs(cDiff)
		} else {
			score += abs(rDiff)
		}
		if score < bestScore {
			best, bestScore = index, score
		}
	}
	if best >= 0 {
		n.current = best
	}
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
