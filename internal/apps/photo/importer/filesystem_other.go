//go:build !darwin

package importer

// mountSource is not reported on this platform; the device comparison remains.
func mountSource(string) string { return "" }
