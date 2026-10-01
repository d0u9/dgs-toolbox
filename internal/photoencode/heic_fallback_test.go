//go:build !darwin || !cgo

package photoencode

import (
	"strings"
	"testing"
)

func TestHEICFallbackExplainsBuildRequirement(t *testing.T) {
	_, _, e := decodeHEIC([]byte("heic"), DefaultOptions())
	if e == nil || !strings.Contains(e.Error(), "macOS") || !strings.Contains(e.Error(), "cgo") {
		t.Fatal(e)
	}
}
