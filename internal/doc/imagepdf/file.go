package imagepdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Read bounds a document image's encoded input before decoding it.
func Read(source string) ([]byte, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (256<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 256<<20 {
		return nil, fmt.Errorf("input exceeds 256 MiB limit")
	}
	return data, nil
}

// cachedWorker bounds conversion allocations across concurrent page requests.
var cachedWorker = make(chan struct{}, 1)

// Cached prepares an image for preview and OCR in a discardable local cache.
// The key includes the converter version and source content, never its path.
func Cached(ctx context.Context, source, dir string) (string, error) {
	if !Supported(source) {
		return source, nil
	}
	select {
	case cachedWorker <- struct{}{}:
		defer func() { <-cachedWorker }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	data, err := Read(source)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256(append([]byte("dgs-imagepdf-v1\x00"+filepath.Ext(source)), data...))
	if dir == "" {
		dir, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(dir, "dgs", "doc")
	}
	dir = filepath.Join(dir, "imagepdf")
	path := filepath.Join(dir, hex.EncodeToString(key[:])+".pdf")
	// A checksum accompanies the disposable PDF. Missing or corrupt entries
	// are rebuilt; only content verified against that checksum is reused.
	if kept, err := os.ReadFile(path); err == nil {
		if checksum, err := os.ReadFile(path + ".sha256"); err == nil {
			sum := sha256.Sum256(kept)
			if string(checksum) == hex.EncodeToString(sum[:]) {
				return path, nil
			}
		}
	}
	output, err := Convert(ctx, source, data)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".dgs-imagepdf-*.dgs-part")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(output)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	sum := sha256.Sum256(output)
	if err := os.WriteFile(path+".sha256", []byte(hex.EncodeToString(sum[:])), 0600); err != nil {
		return "", err
	}
	return path, nil
}
