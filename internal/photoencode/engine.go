package photoencode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dgs-toolbox/internal/verifiedcopy"
)

// Options are per-run values. Destination is a directory, never an input file.
type Options struct {
	Background                    string
	Quality, Size, PPI, MaxPixels int
	Recursive, PreserveGPS        bool
}

func DefaultOptions() Options {
	return Options{Quality: DefaultQuality, Size: DefaultSize, PPI: DefaultPPI, MaxPixels: DefaultMaxPixels, Background: DefaultBackground}
}

// OptionError names the option Validate refused, so a form can mark the field.
type OptionError struct {
	Option  string // background, quality, size, ppi or max_pixels
	Message string
}

func (e *OptionError) Error() string { return e.Message }

func (o Options) Validate() error {
	if _, err := BackgroundColor(o.Background); err != nil {
		return &OptionError{"background", err.Error()}
	}
	if o.Quality < 1 || o.Quality > 100 {
		return &OptionError{"quality", "quality must be 1–100"}
	}
	if o.Size < 1 || o.Size > 65535 {
		return &OptionError{"size", "size must be 1–65535"}
	}
	if o.PPI < 1 || o.PPI > 65535 {
		return &OptionError{"ppi", "ppi must be 1–65535"}
	}
	if o.MaxPixels < 1 || o.MaxPixels > DefaultMaxPixels {
		return &OptionError{"max_pixels", fmt.Sprintf("max_pixels must be 1–%d", DefaultMaxPixels)}
	}
	return nil
}

type Job struct {
	Source, Destination string
	// Root is the selected destination directory. Directories below it are
	// created without following symbolic links.
	Root    string
	Problem string
	// Exists marks an output that is already there: the job is skipped, not
	// failed, so running the same plan again reports nothing new.
	Exists bool
}
type Outcome struct {
	Job                     Job
	BytesBefore, BytesAfter int64
	Width, Height           int
	Error                   string
	Published               bool
	Skipped                 bool
	// Cancelled is a job stopped by the caller, not a failure of the image.
	Cancelled bool
}

// outputPattern matches names Plan gives outputs, so a later run reading the
// destination tree never encodes an earlier run's outputs again.
var outputPattern = regexp.MustCompile(`(?i)-q\d+-s\d+\.jpg$`)

// nativeKind names inputs decoded by macOS ImageIO rather than Go, or "".
func nativeKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".heic", ".heif":
		return "HEIC"
	case ".tif", ".tiff":
		return "TIFF"
	}
	return ""
}

// Plan performs no writes. Inputs this build cannot decode carry the reason.
func Plan(source, destination string, o Options) ([]Job, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if source == "" || destination == "" {
		return nil, errors.New("source and destination are required")
	}
	src, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	dst, err := filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("select the actual source, not a symbolic link")
	}
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return nil, err
	}
	if di, err := os.Stat(dst); err == nil && !di.IsDir() {
		return nil, errors.New("destination must be a directory")
	}
	var jobs []Job
	add := func(path, relative string) {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".heic" && ext != ".heif" && ext != ".tif" && ext != ".tiff" {
			return
		}
		name := strings.TrimSuffix(relative, filepath.Ext(relative))
		out := filepath.Join(dst, fmt.Sprintf("%s-q%d-s%d.jpg", name, o.Quality, o.Size))
		j := Job{Source: path, Destination: out, Root: dst}
		if kind := nativeKind(path); kind != "" {
			j.Problem = nativeProblem(kind)
		}
		if _, err := os.Lstat(out); err == nil {
			j.Exists = true
			j.Problem = joinProblems(j.Problem, "output already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			j.Problem = joinProblems(j.Problem, err.Error())
		}
		jobs = append(jobs, j)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, errors.New("source must be a regular file or directory")
		}
		add(src, filepath.Base(src))
		if len(jobs) == 0 {
			return nil, errors.New("supported inputs: JPEG, PNG, HEIC/HEIF, TIFF")
		}
		return jobs, nil
	}
	// Resolve the destination's existing ancestors to avoid scanning a nested output.
	resolvedDst := dst
	ancestor := dst
	var tail []string
	for {
		real, e := filepath.EvalSymlinks(ancestor)
		if e == nil {
			resolvedDst = filepath.Join(append([]string{real}, tail...)...)
			break
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		tail = append([]string{filepath.Base(ancestor)}, tail...)
		ancestor = parent
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			if path == src {
				return e
			}
			// One unreadable entry is reported on its own; the scan goes on.
			jobs = append(jobs, Job{Source: path, Problem: "cannot read: " + e.Error()})
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != src && (strings.HasPrefix(d.Name(), ".") || !o.Recursive || path == resolvedDst) {
				return filepath.SkipDir
			}
			return nil
		}
		// Hidden files include AppleDouble "._" companions on non-Apple volumes.
		// An earlier run's output inside the destination is not an input, even
		// when the destination is the source folder itself.
		if d.Type().IsRegular() && !strings.HasPrefix(d.Name(), ".") && !(outputPattern.MatchString(d.Name()) && insideDir(resolvedDst, path)) {
			rel, e := filepath.Rel(src, path)
			if e != nil {
				return e
			}
			add(path, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, errors.New("no supported images found")
	}
	// Two inputs with the same stem must never compete for one output.
	seen := map[string]int{}
	for i, j := range jobs {
		if j.Destination == "" {
			continue
		}
		key := strings.ToLower(j.Destination)
		if previous, ok := seen[key]; ok {
			jobs[i].Problem = "multiple inputs share this output name"
			jobs[previous].Problem = jobs[i].Problem
		} else {
			seen[key] = i
		}
	}
	return jobs, nil
}
func insideDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func joinProblems(first, second string) string {
	if first == "" {
		return second
	}
	return first + "; " + second
}

// Encode never changes Source. Publication is atomic and refuses replacement.
// Guarantees end at filesystem/user-space APIs, not physical media durability.
func Encode(ctx context.Context, j Job, o Options) (result Outcome) {
	result.Job = j
	fail := func(err error) Outcome {
		result.Error = err.Error()
		result.Cancelled = ctx.Err() != nil
		return result
	}
	if err := o.Validate(); err != nil {
		return fail(err)
	}
	if j.Exists {
		result.Skipped, result.Error = true, "output already exists"
		return result
	}
	if j.Problem != "" {
		return fail(errors.New(j.Problem))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	info, err := os.Lstat(j.Source)
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(errors.New("source is no longer a regular file"))
	}
	if info.Size() > 256<<20 {
		return fail(errors.New("input exceeds 256 MiB limit"))
	}
	input, err := os.Open(j.Source)
	if err != nil {
		return fail(err)
	}
	data, err := io.ReadAll(io.LimitReader(input, (256<<20)+1))
	closeErr := input.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	if len(data) > 256<<20 {
		return fail(errors.New("input exceeds 256 MiB limit"))
	}
	result.BytesBefore = int64(len(data))
	src, m, err := decodeImage(ctx, j.Source, data, o)
	if err != nil {
		return fail(err)
	}
	background, _ := BackgroundColor(o.Background)
	// Every export is sRGB, as the web and most viewers assume. Transparency is
	// flattened after conversion, in the space the background colour names.
	converted, profile, err := toSRGB(Fit(src, m.orientation, o.Size), m.icc)
	if err != nil {
		return fail(err)
	}
	dst := Composite(converted, background)
	m.icc = profile
	result.Width, result.Height = dst.Bounds().Dx(), dst.Bounds().Dy()
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, dst, &jpeg.Options{Quality: o.Quality}); err != nil {
		return fail(err)
	}
	output, err := addMetadata(encoded.Bytes(), m, o.PreserveGPS, o.PPI, result.Width, result.Height)
	if err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if err = verifiedcopy.EnsureDirectory(j.Root, filepath.Dir(j.Destination)); err != nil {
		return fail(err)
	}
	f, err := os.CreateTemp(filepath.Dir(j.Destination), ".dgs-encode-*.dgs-part")
	if err != nil {
		return fail(err)
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	written, err := writeTemporary(f, output)
	if err != nil {
		return fail(err)
	}
	readback, err := readTemporary(temporary, written)
	if err != nil {
		return fail(err)
	}
	if sha256.Sum256(readback) != sha256.Sum256(output) {
		return fail(errors.New("destination SHA-256 readback mismatch"))
	}
	verified, err := jpeg.Decode(bytes.NewReader(readback))
	if err != nil {
		return fail(fmt.Errorf("destination decode: %w", err))
	}
	if verified.Bounds() != dst.Bounds() {
		return fail(errors.New("destination dimensions mismatch"))
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	// The name about to be published must still be the file that was verified.
	if current, err := os.Lstat(temporary); err != nil || !os.SameFile(current, written) {
		return fail(errors.New("temporary file changed before publication"))
	}
	if err = verifiedcopy.Publish(temporary, j.Destination); err != nil {
		return fail(fmt.Errorf("publication: %w", err))
	}
	result.Published = true
	if err = verifiedcopy.SyncDirectory(filepath.Dir(j.Destination)); err != nil {
		// The verified file already wears its final name; only the directory
		// entry's flush is in doubt, as with Import.
		result.Error = "sync destination directory: " + err.Error()
	}
	result.BytesAfter = int64(len(readback))
	return result
}

func isGray(model color.Model) bool { return model == color.GrayModel || model == color.Gray16Model }

// writeTemporary writes, sets the published permission and syncs through one
// handle, returning that file's identity for the readback to match.
func writeTemporary(f *os.File, output []byte) (os.FileInfo, error) {
	_, err := f.Write(output)
	if err == nil {
		err = f.Chmod(verifiedcopy.DefaultPhotoMode)
	}
	if err == nil {
		err = f.Sync()
	}
	var written os.FileInfo
	if err == nil {
		written, err = f.Stat()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return written, err
}

// readTemporary reads back through a fresh handle that must be the same file.
func readTemporary(path string, written os.FileInfo) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, written) {
		return nil, errors.New("readback opened a different file")
	}
	return io.ReadAll(f)
}

// DecodeImage returns oriented, opaque sRGB pixels for a document conversion.
// name selects the native HEIC/HEIF or TIFF decoder. Options bound allocation
// and specify the background and maximum side; no source file is changed.
func DecodeImage(ctx context.Context, name string, data []byte, o Options) (*image.NRGBA, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if len(data) > 256<<20 {
		return nil, errors.New("input exceeds 256 MiB limit")
	}
	src, m, err := decodeImage(ctx, name, data, o)
	if err != nil {
		return nil, err
	}
	background, _ := BackgroundColor(o.Background)
	// Every export is sRGB, as the web and most viewers assume.
	dst, _, err := toSRGB(Transform(src, m.orientation, o.Size, background), m.icc)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func decodeImage(ctx context.Context, name string, data []byte, o Options) (image.Image, metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, metadata{}, err
	}
	var err error
	var src image.Image
	var m metadata
	if kind := nativeKind(name); kind != "" {
		src, m, err = decodeNative(kind, data, o)
	} else {
		var cfg image.Config
		var format string
		cfg, format, err = image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, m, err
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > int64(o.MaxPixels) {
			return nil, m, errors.New("image exceeds configured pixel limit")
		}
		if cfg.ColorModel == color.CMYKModel {
			return nil, m, errors.New("CMYK requires colour conversion and is not supported")
		}
		switch format {
		case "jpeg":
			m, err = jpegMetadata(data)
		case "png":
			m, err = pngMetadata(data)
		default:
			err = errors.New("only JPEG and PNG are supported by the Go decoder")
		}
		if err == nil {
			err = checkICC(m.icc, isGray(cfg.ColorModel))
		}
		if err == nil {
			src, _, err = image.Decode(bytes.NewReader(data))
		}
	}
	if err != nil {
		return nil, m, err
	}
	if err = ctx.Err(); err != nil {
		return nil, m, err
	}
	return src, m, nil
}
