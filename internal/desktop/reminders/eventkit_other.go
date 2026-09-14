//go:build !darwin || !cgo

package reminders

const available = false

func eventKit(string, []byte) ([]byte, error) { return nil, ErrUnavailable }
