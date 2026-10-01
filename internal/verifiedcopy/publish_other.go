//go:build !darwin && !linux

package verifiedcopy

func renameExclusive(from, to string) error { return errNoExclusiveRename }
