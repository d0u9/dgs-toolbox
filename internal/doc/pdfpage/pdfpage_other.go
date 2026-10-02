//go:build !darwin || !cgo

package pdfpage

const available = false

func render(string, int, int, float64) ([]byte, error) { return nil, ErrUnavailable }

func count(string) (int, error) { return 0, ErrUnavailable }
