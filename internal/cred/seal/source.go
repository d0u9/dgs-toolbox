package seal

import (
	"fmt"
	"os"
	"path/filepath"
)

// SourceInfo rejects links and special files, including directory links hidden
// by a trailing separator or /. It preserves intermediate symlink/.. semantics.
func SourceInfo(source string, allowDirectory bool) (os.FileInfo, error) {
	check := source
	volume := len(filepath.VolumeName(check))
	for {
		for len(check) > volume+1 && os.IsPathSeparator(check[len(check)-1]) {
			check = check[:len(check)-1]
		}
		if len(check) >= 2 && check[len(check)-1] == '.' && os.IsPathSeparator(check[len(check)-2]) {
			check = check[:len(check)-1]
			continue
		}
		break
	}
	info, err := os.Lstat(check)
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", source, err)
	}
	if !info.Mode().IsRegular() && !(allowDirectory && info.IsDir()) {
		kinds := "a regular file"
		if allowDirectory {
			kinds += " or directory"
		}
		return nil, fmt.Errorf("source %q must be %s; symbolic links and special files are refused", source, kinds)
	}
	return info, nil
}
