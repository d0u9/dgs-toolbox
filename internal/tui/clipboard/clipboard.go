// Package clipboard puts text on the clipboard through the terminal, with an
// OSC 52 sequence, so it works over SSH and needs no helper program.
package clipboard

import (
	"os"

	"github.com/aymanbagabas/go-osc52/v2"
)

// MaxSize bounds what is copied: terminals cap the OSC 52 sequence, and
// several drop anything much larger without a word.
const MaxSize = 64 << 10

// Copy asks the terminal to put text on the clipboard. It is written to
// stderr, which Bubble Tea leaves alone while it draws on stdout.
func Copy(text string) error {
	_, err := osc52.New(text).WriteTo(os.Stderr)
	return err
}
