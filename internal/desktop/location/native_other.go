//go:build !darwin || !cgo

package location

func native(bool) ([]byte, error) { return nil, ErrUnavailable }
