package verifiedcopy

import "os"

// CountSourceOpens makes every source open increment *opens until the
// returned function restores the real opener.
func CountSourceOpens(opens *int) func() {
	openSource = func(path string) (*os.File, error) {
		*opens++
		return os.Open(path)
	}
	return func() { openSource = os.Open }
}

// ReplaceReadBackOpener lets filesystem tests delay an independent readback.
func ReplaceReadBackOpener(opener func(string) (*os.File, error)) func() {
	previous := openReadBack
	openReadBack = opener
	return func() { openReadBack = previous }
}

// FailWritesTo closes the temporary file of destination as soon as it is
// created, so every write to it fails while other targets carry on.
func FailWritesTo(destination string) func() {
	createdTemporary = func(target string, file *os.File) {
		if target == destination {
			_ = file.Close()
		}
	}
	return func() { createdTemporary = func(string, *os.File) {} }
}

// ReplacePublishDirectorySync lets tests hold a publication before its sync.
func ReplacePublishDirectorySync(sync func(string) error) func() {
	previous := syncPublishedDirectory
	syncPublishedDirectory = sync
	return func() { syncPublishedDirectory = previous }
}
