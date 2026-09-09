// Command mockcapture regenerates the mock Capture root used to exercise the
// Capture Scan and Route sessions.
package main

import (
	"fmt"
	"os"

	"dgs-toolbox/internal/apps/capture/mockcapture"
)

func main() {
	root := "testdata/capture"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := mockcapture.Write(root); err != nil {
		fmt.Fprintln(os.Stderr, "mockcapture:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", root)
}
