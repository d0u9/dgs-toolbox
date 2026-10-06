package doc

import (
	"bytes"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/config"
)

func TestInitAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs")
	var out bytes.Buffer
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := testStore(t, root).Require(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("id_card.yaml")) {
		t.Errorf("output = %s", out.String())
	}
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err == nil {
		t.Fatal("second init succeeded")
	}
}
