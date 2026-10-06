package tree

import (
	"fmt"
	"time"
)

// A revision is one paper: a card issued, a renewal of it. Each snapshot
// names its revision by the ref of that revision's first snapshot, its
// Paper; every other snapshot of a revision is a correction of it. Only the
// owner begins a revision. See docs/apps/doc/index.md, "Revisions and their
// snapshots".

// beginsRevision are the history actions that began a revision before
// snapshots named their Paper: an import, and Import's adding to an Item.
var beginsRevision = map[string]bool{
	"import": true, "create_without_pdf": true,
	"import_revision": true, "create_revision_without_pdf": true,
}

// papers is each snapshot's Paper: its own when it has one, else read from
// the history — a snapshot an action that began a revision made begins one,
// any other belongs to the revision of the snapshot HEAD named when it was
// made — else its own ref. A record has one revision for good.
func (i Item) papers() map[string]string {
	m := map[string]string{}
	if len(i.Revisions) == 0 {
		return m
	}
	if i.Kind == KindRecord {
		first := i.Revisions[0].Ref()
		for _, r := range i.Revisions {
			m[r.Ref()] = first
		}
		return m
	}
	for _, r := range i.Revisions {
		if r.Paper != "" {
			m[r.Ref()] = r.Paper
		}
	}
	head := ""
	for _, e := range i.History {
		switch {
		case e.Action == "make_head":
			head = e.Digest
		case createsRevision(e.Action) && e.Digest != "":
			if _, ok := m[e.Digest]; !ok {
				if p := m[head]; !beginsRevision[e.Action] && p != "" {
					m[e.Digest] = p
				} else {
					m[e.Digest] = e.Digest
				}
			}
			head = e.Digest
		}
	}
	for _, r := range i.Revisions {
		if _, ok := m[r.Ref()]; !ok {
			m[r.Ref()] = r.Ref()
		}
	}
	return m
}

// paper is the revision a snapshot saved with m corrects: none when the
// owner confirmed a new one, else HEAD's.
func (m RevisionMetadata) paper(item Item) string {
	if m.NewRevision && item.Kind != KindRecord {
		return ""
	}
	return item.PaperOf(item.Current())
}

// correcting is the revision a snapshot made from ref corrects: none when
// the owner confirmed a new one.
func correcting(item Item, ref string, newRevision bool) string {
	if newRevision && item.Kind != KindRecord {
		return ""
	}
	return item.PaperOf(ref)
}

// settlePapers writes each snapshot's Paper into it.
func (i *Item) settlePapers() {
	m := i.papers()
	for n, r := range i.Revisions {
		i.Revisions[n].Paper = m[r.Ref()]
	}
}

// PaperOf is the Paper of the snapshot ref.
func (i Item) PaperOf(ref string) string { return i.papers()[ref] }

// Papers are the Item's revisions, by Paper, in the order they began.
func (i Item) Papers() []string {
	m := i.papers()
	var out []string
	seen := map[string]bool{}
	for _, r := range i.Revisions {
		if p := m[r.Ref()]; !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// RevisionNumber is the number of the revision the snapshot ref belongs
// to, counting from 1 in the order revisions began; 0 for none.
func (i Item) RevisionNumber(ref string) int {
	p := i.PaperOf(ref)
	for n, q := range i.Papers() {
		if q == p {
			return n + 1
		}
	}
	return 0
}

// Latest is the current snapshot of each revision: HEAD for its own, the
// newest for every other.
func (i Item) Latest() map[string]bool {
	m := i.papers()
	newest := map[string]string{}
	for _, r := range i.Revisions {
		newest[m[r.Ref()]] = r.Ref()
	}
	if head := i.Current(); head != "" {
		if p, ok := m[head]; ok {
			newest[p] = head
		}
	}
	out := map[string]bool{}
	for _, ref := range newest {
		out[ref] = true
	}
	return out
}

// MakeRevision begins a revision with the correction ref: it and the
// snapshots of its revision after it in the list move to the new one.
func (st *Store) MakeRevision(id string, ref string, now time.Time) (Item, error) {
	return st.changePaper(id, ref, now, "make_revision", func(item *Item, from string) error {
		if item.Kind == KindRecord {
			return fmt.Errorf("a record has one revision")
		}
		if from == ref {
			return fmt.Errorf("snapshot %s already begins a revision", ref)
		}
		after := false
		for n, r := range item.Revisions {
			if r.Ref() == ref {
				after = true
			}
			if after && r.Paper == from {
				item.Revisions[n].Paper = ref
			}
		}
		return nil
	})
}

// MakeCorrection makes the revision ref begins part of the one before it.
func (st *Store) MakeCorrection(id string, ref string, now time.Time) (Item, error) {
	return st.changePaper(id, ref, now, "make_correction", func(item *Item, from string) error {
		if from != ref {
			return fmt.Errorf("snapshot %s is already a correction", ref)
		}
		papers := item.Papers()
		before := ""
		for n, p := range papers {
			if p == ref && n > 0 {
				before = papers[n-1]
			}
		}
		if before == "" {
			return fmt.Errorf("the first revision has none before it")
		}
		for n, r := range item.Revisions {
			if r.Paper == ref {
				item.Revisions[n].Paper = before
			}
		}
		return nil
	})
}

func (st *Store) changePaper(id string, ref string, now time.Time, action string, change func(*Item, string) error) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	item, _, err := st.FindItem(id)
	if err != nil {
		return Item{}, err
	}
	if _, ok := item.Revision(ref); !ok {
		return Item{}, fmt.Errorf("unknown revision %s", ref)
	}
	item.settlePapers()
	if err := change(&item, item.PaperOf(ref)); err != nil {
		return Item{}, err
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: action, Digest: ref})
	return item, st.WriteItem(item)
}
