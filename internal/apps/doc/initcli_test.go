package doc

import (
	"bytes"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/tree"
)

func TestInitAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs")
	var out bytes.Buffer
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := tree.Require(root); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("id_card.yaml")) {
		t.Errorf("output = %s", out.String())
	}
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err == nil {
		t.Fatal("second init succeeded")
	}
}
