//go:build darwin && cgo

package ocr

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreGraphics -framework PDFKit -framework Vision
#include <stdlib.h>
char *dgs_ocr_page(const char *path, int page);
*/
import "C"

import "unsafe"

const available = true

// recognize hands one page of the PDF to ocr_darwin.m, which answers one
// JSON object.
func recognize(path string, page int) ([]byte, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	answer := C.dgs_ocr_page(cpath, C.int(page))
	defer C.free(unsafe.Pointer(answer))
	return []byte(C.GoString(answer)), nil
}
