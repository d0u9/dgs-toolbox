package photo

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/photo/postprocess"
)

// dateRange selects shots by capture day, inclusive at both ends:
// YYYYMMDD-YYYYMMDD, -YYYYMMDD (up to) or YYYYMMDD- (from). An empty end is
// open. Days are the camera's own clock, as organizing names its folders.
type dateRange struct{ from, to string }

const dayLayout = "20060102"

func parseDateRange(text string) (dateRange, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return dateRange{}, nil
	}
	from, to, ok := strings.Cut(text, "-")
	if !ok || (from == "" && to == "") || strings.Contains(to, "-") {
		return dateRange{}, fmt.Errorf("range %q must be YYYYMMDD-YYYYMMDD, -YYYYMMDD or YYYYMMDD-", text)
	}
	for _, day := range []string{from, to} {
		if day == "" {
			continue
		}
		if parsed, err := time.Parse(dayLayout, day); err != nil || parsed.Format(dayLayout) != day {
			return dateRange{}, fmt.Errorf("range: %q is not a date as YYYYMMDD", day)
		}
	}
	if from != "" && to != "" && from > to {
		return dateRange{}, fmt.Errorf("range: %s is after %s", from, to)
	}
	return dateRange{from, to}, nil
}

func (r dateRange) set() bool { return r.from != "" || r.to != "" }

func (r dateRange) contains(day string) bool {
	return (r.from == "" || day >= r.from) && (r.to == "" || day <= r.to)
}

// captureDays reads one capture day per shot under root, keyed by pairKey:
// a RAW and its JPG are one shot, dated by the JPG when there is one. A shot
// whose date cannot be read maps to "".
func captureDays(root string, files []scannedFile) map[string]string {
	chosen := map[string]string{}
	for _, file := range files {
		if !postprocess.IsPhoto(file.path) {
			continue
		}
		key := pairKey(file.path)
		if prior, ok := chosen[key]; !ok || (!isJPEGName(prior) && isJPEGName(file.path)) {
			chosen[key] = file.path
		}
	}
	days := make(map[string]string, len(chosen))
	for key, path := range chosen {
		taken, err := postprocess.CaptureDate(filepath.Join(root, filepath.FromSlash(path)))
		if err == nil {
			days[key] = taken.Format(dayLayout)
		} else {
			days[key] = ""
		}
	}
	return days
}

func isJPEGName(path string) bool {
	extension := strings.ToUpper(filepath.Ext(path))
	return extension == ".JPG" || extension == ".JPEG"
}

// withinRange keeps the files whose shot was taken in r. Files with no
// readable capture day — videos, or photos without EXIF — are left out and
// returned, since their day is unknown.
func withinRange(files []scannedFile, days map[string]string, r dateRange) (in []scannedFile, undated []string) {
	for _, file := range files {
		day := days[pairKey(file.path)]
		switch {
		case day == "":
			undated = append(undated, file.path)
		case r.contains(day):
			in = append(in, file)
		}
	}
	return in, undated
}
