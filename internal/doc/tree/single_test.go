package tree

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSingleTemplateTakesNoRevision(t *testing.T) {
	template, err := ParseTemplate([]byte("type: ticket\nkind: document\nsingle: true\nfields:\n  - key: owner\n    required: true\n"))
	if err != nil || !template.Single {
		t.Fatalf("parse: %+v %v", template, err)
	}
	st := &Store{}
	if _, err := st.AddRevisionWithMetadata(context.Background(), "x", "a.pdf", template, nil, RevisionMetadata{}, time.Now()); !errors.Is(err, ErrSingle) {
		t.Fatalf("revision: %v", err)
	}
	if _, err := st.AddWithoutPDF("x", template, nil, RevisionMetadata{}, time.Now()); !errors.Is(err, ErrSingle) {
		t.Fatalf("without PDF: %v", err)
	}
}
