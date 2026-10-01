//go:build darwin && cgo

package location

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreLocation -framework Contacts
#include <stdlib.h>
char *dgs_location_get(int placemark,int timeout);
*/
import "C"
import (
	"runtime"
	"unsafe"
)

func native(placemark bool) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lookup := C.int(0)
	if placemark {
		lookup = 1
	}
	answer := C.dgs_location_get(lookup, C.int(TimeoutSeconds))
	defer C.free(unsafe.Pointer(answer))
	return []byte(C.GoString(answer)), nil
}
