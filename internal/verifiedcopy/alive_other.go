//go:build !unix

package verifiedcopy

// processAlive cannot tell here, so a lock is never taken over.
func processAlive(int) bool { return true }
