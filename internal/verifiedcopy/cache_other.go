//go:build !darwin && !linux

package verifiedcopy

import "os"

// bypassCache and dropCache are not available on this platform; readback is
// then served however the operating system chooses.
func bypassCache(*os.File) error { return nil }
func dropCache(*os.File) error   { return nil }
