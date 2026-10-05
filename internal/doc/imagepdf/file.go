package imagepdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// Version names the converter. It is part of every cache key, so a change
// to what ConvertAll writes is never answered by an older entry.
const Version = "dgs-imagepdf-v2"

// Cached prepares images, as one PDF, for preview and OCR in a discardable
// local cache. A single PDF is answered as it is. The key holds the
// converter version and every input's extension and content, never a path.
func Cached(ctx context.Context, sources []string, dir string) (string, error) {
	if len(sources) == 1 && !Supported(sources[0]) {
		return sources[0], nil
	}
	select {
	case cachedWorker <- struct{}{}:
		defer func() { <-cachedWorker }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	inputs := make([]Input, len(sources))
	h := sha256.New()
	h.Write([]byte(Version))
	for i, source := range sources {
		if !Supported(source) {
			return "", fmt.Errorf("%s: only images become pages", filepath.Base(source))
		}
		data, err := Read(source)
		if err != nil {
			return "", err
		}
		inputs[i] = Input{Name: source, Data: data}
		fmt.Fprintf(h, "\x00%s\x00%d\x00", strings.ToLower(filepath.Ext(source)), len(data))
		h.Write(data)
	}
	key := h.Sum(nil)
	var err error
	if dir == "" {
		dir, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(dir, "dgs", "doc")
	}
	dir = filepath.Join(dir, "imagepdf")
	path := filepath.Join(dir, hex.EncodeToString(key)+".pdf")
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
	output, err := ConvertAll(ctx, inputs)
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
