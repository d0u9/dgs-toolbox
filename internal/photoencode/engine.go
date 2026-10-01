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
	"strings"
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
func (o Options) Validate() error {
	if _, err := BackgroundColor(o.Background); err != nil {
		return err
	}
	if o.Quality < 1 || o.Quality > 100 {
		return errors.New("quality must be 1–100")
	}
	if o.Size < 1 || o.Size > 65535 {
		return errors.New("size must be 1–65535")
	}
	if o.PPI < 1 || o.PPI > 65535 {
		return errors.New("ppi must be 1–65535")
	}
	if o.MaxPixels < 1 || o.MaxPixels > DefaultMaxPixels {
		return fmt.Errorf("max_pixels must be 1–%d", DefaultMaxPixels)
	}
	return nil
}

type Job struct {
	Source, Destination string
	Problem             string
}
type Outcome struct {
	Job                     Job
	BytesBefore, BytesAfter int64
	Width, Height           int
	Error                   string
	Published               bool
}

// Plan performs no writes. Unsupported TIFFs carry a copyable shell command.
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
		j := Job{Source: path, Destination: out}
		if ext == ".tif" || ext == ".tiff" {
			j.Problem = "TIFF is not supported. Run manually: " + TIFFCommand(path, out, o)
		}
		if _, err := os.Lstat(out); err == nil {
			j.Problem = "output already exists"
		} else if !errors.Is(err, os.ErrNotExist) {
			j.Problem = err.Error()
		}
		jobs = append(jobs, j)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, errors.New("source must be a regular file or directory")
		}
		add(src, filepath.Base(src))
		if len(jobs) == 0 {
			return nil, errors.New("supported inputs: JPEG, PNG, HEIC; TIFF gets a manual sips command")
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
			return e
		}
		if d.IsDir() {
			if path != src && (strings.HasPrefix(d.Name(), ".") || !o.Recursive || path == resolvedDst) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
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
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func TIFFCommand(source, destination string, o Options) string {
	return fmt.Sprintf("sips %s -Z %d -s dpiWidth %d -s dpiHeight %d -s format jpeg -s formatOptions %d --out %s", shellQuote(source), o.Size, o.PPI, o.PPI, o.Quality, shellQuote(destination))
}

// Encode never changes Source. Publication is atomic and refuses replacement.
// Guarantees end at filesystem/user-space APIs, not physical media durability.
func Encode(ctx context.Context, j Job, o Options) (result Outcome) {
	result.Job = j
	fail := func(err error) Outcome { result.Error = err.Error(); return result }
	if err := o.Validate(); err != nil {
		return fail(err)
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
	var src image.Image
	var m metadata
	switch strings.ToLower(filepath.Ext(j.Source)) {
	case ".heic", ".heif":
		src, m, err = decodeHEIC(data, o)
	default:
		var cfg image.Config
		var format string
		cfg, format, err = image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fail(err)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > int64(o.MaxPixels) {
			return fail(errors.New("image exceeds configured pixel limit"))
		}
		if cfg.ColorModel == color.CMYKModel {
			return fail(errors.New("CMYK requires colour conversion and is not supported"))
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
			err = checkICC(m.icc)
		}
		if err == nil {
			src, _, err = image.Decode(bytes.NewReader(data))
		}
	}
	if err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	background, _ := BackgroundColor(o.Background)
	dst := Transform(src, m.orientation, o.Size, background)
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
	if err = os.MkdirAll(filepath.Dir(j.Destination), 0755); err != nil {
		return fail(err)
	}
	f, err := os.CreateTemp(filepath.Dir(j.Destination), ".dgs-encode-*.dgs-part")
	if err != nil {
		return fail(err)
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, err = f.Write(output); err != nil {
		f.Close()
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return fail(err)
	}
	if err = f.Close(); err != nil {
		return fail(err)
	}
	readback, err := os.ReadFile(temporary)
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
	// Link atomically creates the final name only if absent. On filesystems without
	// hard links this fails clearly; never fall back to an overwriting rename.
	if err = os.Link(temporary, j.Destination); err != nil {
		return fail(fmt.Errorf("atomic publication: %w", err))
	}
	result.Published = true
	result.BytesAfter = int64(len(readback))
	return result
}
