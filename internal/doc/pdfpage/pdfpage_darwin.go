//go:build darwin && cgo

package pdfpage

/*
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation -framework CoreGraphics -framework ImageIO -framework PDFKit
#include <stdlib.h>
int dgs_pdf_count(const char *path);
int dgs_pdf_render(const char *path, int page, int longSide, double quality, unsigned char **out, long *length);
*/
import "C"

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const available = true

// Core Graphics writes its complaints about a damaged PDF straight to the
// process's stderr ("CoreGraphics PDF has logged an error..."), which
// scrawls over a TUI drawn on the same terminal. It still draws such a PDF,
// so its words are dropped: stderr points at /dev/null while Core Graphics
// runs. The descriptor is the whole process's, so calls take turns.
var quietly sync.Mutex

func quiet(f func()) {
	quietly.Lock()
	defer quietly.Unlock()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		f()
		return
	}
	defer null.Close()
	saved, err := syscall.Dup(2)
	if err != nil {
		f()
		return
	}
	defer syscall.Close(saved)
	if syscall.Dup2(int(null.Fd()), 2) != nil {
		f()
		return
	}
	defer syscall.Dup2(saved, 2)
	f()
}

func render(path string, n, longSide int, quality float64) ([]byte, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var out *C.uchar
	var length C.long
	var status C.int
	quiet(func() { status = C.dgs_pdf_render(cpath, C.int(n), C.int(longSide), C.double(quality), &out, &length) })
	switch status {
	case 0:
		defer C.free(unsafe.Pointer(out))
		return C.GoBytes(unsafe.Pointer(out), C.int(length)), nil
	case 1:
		return nil, fmt.Errorf("%s is not a PDF Core Graphics can open", path)
	case 2:
		return nil, fmt.Errorf("%s has no page %d", path, n)
	default:
		return nil, fmt.Errorf("page %d of %s could not be drawn", n, path)
	}
}

func count(path string) (int, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var n int
	quiet(func() { n = int(C.dgs_pdf_count(cpath)) })
	if n < 0 {
		return 0, fmt.Errorf("%s is not a PDF Core Graphics can open", path)
	}
	return n, nil
}
