//go:build !windows

package archive

import "syscall"

const crossDeviceError = syscall.EXDEV
