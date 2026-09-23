// Package digest computes the two digests a Box identifies a scan by.
//
// Two, because one is not enough for scanned paper. The whole-file digest
// catches a rescan and a restored backup. It misses the case that is actually
// common here: the same scan rewrapped by other software — merged, re-saved,
// passed through a mail client, stripped of metadata. The container differs
// byte for byte while the scan inside it does not, so the second digest covers
// the embedded image streams alone and ignores everything around them.
//
// Both are SHA-256 and both are written as "sha256:" followed by lowercase hex,
// which is what goes into a sidecar.
package digest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"

	"dgs-toolbox/internal/box/scanmeta"
)

// Prefix names the algorithm in a stored digest. It is written down rather than
// assumed so a Box whose digests were computed by this build stays readable if
// another algorithm is ever added beside it.
const Prefix = "sha256:"

// Whole is the digest of every byte of the file.
func Whole(data []byte) string {
	sum := sha256.Sum256(data)
	return Prefix + hex.EncodeToString(sum[:])
}

// WholeFrom is Whole over a stream, for a file not held in memory.
func WholeFrom(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", err
	}
	return Prefix + hex.EncodeToString(hasher.Sum(nil)), nil
}

// Images is the digest of a scan's embedded image streams alone, in page order,
// and whether there was anything to digest.
//
// A scan with no embedded image — a vector page, a composite, a file whose
// streams could not be read — has no image digest rather than the digest of
// nothing. Were it the digest of nothing, every such scan in the Box would
// collide with every other one and the duplicate list would be useless in
// exactly the place a person most needs to trust it.
//
// Each stream's length is folded in before its bytes. Without that, two streams
// of "ab" + "c" and one of "abc" hash the same, and a scan split differently by
// two producers would be called a duplicate of itself in one direction and not
// in the other.
func Images(images []scanmeta.Image) (string, bool) {
	if len(images) == 0 {
		return "", false
	}
	hasher := sha256.New()
	found := false
	for _, image := range images {
		if len(image.Bytes) == 0 {
			continue
		}
		found = true
		writeLength(hasher, len(image.Bytes))
		hasher.Write(image.Bytes)
	}
	if !found {
		return "", false
	}
	return Prefix + hex.EncodeToString(hasher.Sum(nil)), true
}

func writeLength(hasher hash.Hash, length int) {
	var header [8]byte
	binary.BigEndian.PutUint64(header[:], uint64(length))
	hasher.Write(header[:])
}

// Short is the first length hex digits of a digest, which is what names a
// sidecar and what is appended to a published file's own name.
func Short(digest string, length int) string {
	body := digest
	if len(body) > len(Prefix) && body[:len(Prefix)] == Prefix {
		body = body[len(Prefix):]
	}
	if length <= 0 || length > len(body) {
		length = len(body)
	}
	return body[:length]
}
