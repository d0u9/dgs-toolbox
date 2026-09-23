package digest_test

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/scanmeta"
)

func TestWholeNamesItsAlgorithm(t *testing.T) {
	got := digest.Whole([]byte("paper"))
	if !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("got %q, which does not say what computed it", got)
	}
	if len(got) != len("sha256:")+64 {
		t.Errorf("got %q, want 64 hex digits", got)
	}
	if got != digest.Whole([]byte("paper")) {
		t.Error("the same bytes digested differently twice")
	}
	if got == digest.Whole([]byte("paperr")) {
		t.Error("different bytes digested the same")
	}
}

func TestWholeFromMatchesWhole(t *testing.T) {
	data := []byte("a scanned receipt")
	streamed, err := digest.WholeFrom(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if streamed != digest.Whole(data) {
		t.Error("digesting a stream and digesting the bytes disagreed")
	}
}

// A scan with no embedded image has no image digest. Were it the digest of
// nothing, every vector page in the Box would collide with every other one and
// the duplicate list would be wrong exactly where it matters.
func TestNoImagesMeansNoDigest(t *testing.T) {
	if _, found := digest.Images(nil); found {
		t.Error("a scan with no images was given an image digest")
	}
	if _, found := digest.Images([]scanmeta.Image{{Page: 1}}); found {
		t.Error("an image with no bytes was given an image digest")
	}
}

// Without a length fold, "ab"+"c" and "abc" hash the same, and a scan split
// differently by two producers would be a duplicate of itself in one direction
// and not the other.
func TestStreamBoundariesMatter(t *testing.T) {
	split, ok := digest.Images([]scanmeta.Image{
		{Page: 1, Bytes: []byte("ab")},
		{Page: 2, Bytes: []byte("c")},
	})
	if !ok {
		t.Fatal("no digest")
	}
	joined, ok := digest.Images([]scanmeta.Image{{Page: 1, Bytes: []byte("abc")}})
	if !ok {
		t.Fatal("no digest")
	}
	if split == joined {
		t.Error("two streams and one stream of the same bytes digested the same")
	}
}

// The digest covers the stored stream, never the decoded picture. A digest
// lives in a sidecar forever, so it may not move when a library changes how it
// re-encodes an image.
func TestDigestIgnoresTheRenderedForm(t *testing.T) {
	first, _ := digest.Images([]scanmeta.Image{{Page: 1, Bytes: []byte("stream"), Rendered: []byte("one rendering")}})
	second, _ := digest.Images([]scanmeta.Image{{Page: 1, Bytes: []byte("stream"), Rendered: []byte("a quite different rendering")}})
	if first != second {
		t.Error("the image digest moved when only the rendered form changed")
	}
}

func TestShortTrimsThePrefix(t *testing.T) {
	if got := digest.Short("sha256:a1b2c3d4e5", 8); got != "a1b2c3d4" {
		t.Errorf("got %q", got)
	}
	if got := digest.Short("a1b2c3d4e5", 8); got != "a1b2c3d4" {
		t.Errorf("a digest with no prefix: got %q", got)
	}
	if got := digest.Short("sha256:a1b2", 40); got != "a1b2" {
		t.Errorf("a length past the end: got %q", got)
	}
}
