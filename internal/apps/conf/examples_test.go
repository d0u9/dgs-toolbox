package conf

import (
	"bytes"
	"strings"
	"testing"

	rcli "github.com/d0u9/rhumb/cli"
	"github.com/d0u9/rhumb/engine"
	"github.com/d0u9/rhumb/target"
	"github.com/d0u9/rhumb/validate"
)

func TestGeneratorExampleLoadsValidatesAndRenders(t *testing.T) {
	root, secrets := examplesRoot(t)
	loaded, err := engine.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if broken := rcli.BrokenFiles(loaded.Inv); len(broken) > 0 {
		t.Fatalf("broken inventory: %v", broken)
	}
	if len(loaded.ServiceProblems) > 0 {
		t.Fatal(loaded.ServiceProblems)
	}
	if issues := validate.Validate(loaded.Inv, loaded.Manifests, loaded.Exports, loaded.Derived, nil); len(issues) > 0 {
		t.Fatal(issues)
	}
	targets := target.List(loaded.Inv, loaded.Derived)
	if len(targets) == 0 {
		t.Fatal("example has no render targets")
	}
	renderer := engine.Renderer{Data: loaded, RootPath: root, SecretsDir: secrets}
	for _, tg := range targets {
		if tg.Broken != "" {
			t.Fatal(tg.Broken)
		}
		files, err := renderer.RenderTarget(tg.Instance)
		if err != nil || len(files) == 0 {
			t.Fatalf("render %s: %v", tg.Instance, err)
		}
	}
	var report bytes.Buffer
	if err := exportAction(strings.NewReader(""), &report, []string{"*"}, map[string]string{"to": t.TempDir(), "yes": "true"}, configFor(root, secrets)); err != nil {
		t.Fatalf("export: %v\n%s", err, &report)
	}
}
