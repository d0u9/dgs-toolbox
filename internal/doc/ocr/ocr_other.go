//go:build !darwin || !cgo

package ocr

const available = false

func recognize(string, int) ([]byte, error) { return nil, ErrUnavailable }
