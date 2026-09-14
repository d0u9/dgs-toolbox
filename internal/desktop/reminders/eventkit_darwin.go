//go:build darwin && cgo

package reminders

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework EventKit -framework CoreLocation
#include <stdlib.h>
char *dgs_reminders_call(const char *op, const char *input, int length);
*/
import "C"

import "unsafe"

const available = true

// eventKit hands op and its JSON input to eventkit_darwin.m.
func eventKit(op string, input []byte) ([]byte, error) {
	cop := C.CString(op)
	defer C.free(unsafe.Pointer(cop))
	cinput := C.CBytes(input)
	defer C.free(cinput)
	answer := C.dgs_reminders_call(cop, (*C.char)(cinput), C.int(len(input)))
	defer C.free(unsafe.Pointer(answer))
	return []byte(C.GoString(answer)), nil
}
