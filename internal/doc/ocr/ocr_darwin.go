//go:build darwin && cgo

package ocr

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreGraphics -framework PDFKit -framework Vision
#include <stdlib.h>
char *dgs_ocr_pdf(const char *path, int maxPages);
*/
import "C"

import "unsafe"

const available = true

// recognize hands the path to ocr_darwin.m, which answers one JSON object.
func recognize(path string, maxPages int) ([]byte, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	answer := C.dgs_ocr_pdf(cpath, C.int(maxPages))
	defer C.free(unsafe.Pointer(answer))
	return []byte(C.GoString(answer)), nil
}
