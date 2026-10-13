//go:build darwin && cgo

package filethumb

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework CoreGraphics -framework QuickLookThumbnailing
#include <stdlib.h>
int dgs_file_thumbnail(const char *path, unsigned char **out, long *length);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Render(path string) ([]byte, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var out *C.uchar
	var length C.long
	if C.dgs_file_thumbnail(p, &out, &length) != 0 {
		return nil, fmt.Errorf("no QuickLook thumbnail for this file")
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoBytes(unsafe.Pointer(out), C.int(length)), nil
}
