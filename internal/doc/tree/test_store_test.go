package tree

// testStoreAt preserves unchecked package-internal fixtures, including the
// tests that assert operations fail after a marker is missing.
func testStoreAt(root string) *Store { return at(root) }
