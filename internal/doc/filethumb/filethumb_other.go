//go:build !darwin || !cgo

package filethumb

import "errors"

func Render(path string) ([]byte, error) {
	return nil, errors.New("file thumbnails are unavailable on this platform")
}
