//go:build darwin && cgo

package imagepdf

import (
	"context"
	"os"
	"testing"

	"dgs-toolbox/internal/doc/scanmeta"
)

func TestHEICConversion(t *testing.T) {
	input, err := os.ReadFile("../../photoencode/testdata/rgb.heic")
	if err != nil {
		t.Fatal(err)
	}
	output, err := Convert(context.Background(), "scan.heic", input)
	if err != nil {
		t.Fatal(err)
	}
	info, err := scanmeta.Read(output)
	if err != nil || len(info.Images) != 1 || info.Images[0].Width != 32 || info.Images[0].Height != 16 {
		t.Fatalf("HEIC PDF: %+v %v", info, err)
	}
}
