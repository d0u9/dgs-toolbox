package config

import (
	"testing"

	"dgs-toolbox/internal/doc/dates"
)

func TestDocDateOrder(t *testing.T) {
	config, err := LoadPath(writeConfig(t, `{}`))
	if err != nil || config.DocDateOrder() != dates.DMY {
		t.Fatalf("default: %v %v", err, config.DocDateOrder())
	}
	config, err = LoadPath(writeConfig(t, `{"doc": {"date_order": "mdy"}}`))
	if err != nil || config.DocDateOrder() != dates.MDY {
		t.Fatalf("mdy: %v %v", err, config.DocDateOrder())
	}
	if _, err := LoadPath(writeConfig(t, `{"doc": {"date_order": "YMD"}}`)); err == nil {
		t.Fatal("YMD accepted")
	}
}
