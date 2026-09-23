package box

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// CacheKey is the directory name one Box's discardable cache lives under: a
// short digest of its root path, cleaned first so that /Volumes/nas/Box/ and
// /Volumes/nas/Box are one Box rather than two.
//
// The root is part of the cache's path rather than of its contents. That is
// what lets two Boxes, and two machines, each keep a cache without ever
// agreeing on anything — and a cache nobody has to agree on is a cache that
// can be deleted at any time.
func CacheKey(root string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(digest[:8])
}
