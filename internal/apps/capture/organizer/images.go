package organizer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"dgs-toolbox/internal/imaging"
	"dgs-toolbox/internal/verifiedcopy"
)

// DefaultImageFolder is where a daily note's pictures go, relative to the
// note's own folder: beside it, in a folder named after it. It is the layout
// the Custom Attachment Location plugin writes as "./assets/${noteFileName}",
// chosen as the default because a note's pictures then move with it when a
// rename plugin moves the note.
const DefaultImageFolder = "assets/{{.Note}}"

// imageKind is the attachment kind this Action picks up, in any case: the
// shortcuts write "Image". Every other kind is left where it is: an audio note
// has no place in a line of pictures.
const imageKind = "image"

// ImageFolderData is what the picture folder's template may name. The day is
// there too, so a vault that files pictures by month rather than by note can
// say so.
type ImageFolderData struct {
	// Note is the daily note's filename without its extension.
	Note  string
	Date  string
	Year  string
	Month string
	Day   string
}

// dailyImage is one of the Capture's pictures on its way into the vault.
type dailyImage struct {
	source string
	// target is vault-relative, and name its last element, which is what the
	// entry links: a filename is enough for Obsidian to find a picture, and it
	// keeps reading correctly after the note's folder is renamed.
	target string
	name   string
}

// appendToDailyNote writes the Capture's pictures beside the day's note and
// then the entry that shows them. The pictures go first: a note never links a
// picture that is not yet there under its final name. A Capture with no
// pictures is only the entry.
func appendToDailyNote(ctx Context, plan ActionPlan) (skipped string, err error) {
	note, err := dailyNotePath(ctx)
	if err != nil {
		return "", err
	}
	notePath, ok := ctx.Settings.vaultPath(note)
	if !ok {
		return "", ErrNoVault
	}
	// Checked before any picture is written: a Capture already in the note has
	// had its pictures written too, and writing them again would only be work.
	if existing, err := os.ReadFile(notePath); err == nil && containsMark(string(existing), captureMark(ctx.Capture)) {
		return fmt.Sprintf("this capture is already in %s, as %s", note, captureMark(ctx.Capture)), nil
	}
	images, err := dailyImages(ctx, note)
	if err != nil {
		return "", err
	}
	// Every picture is read before any is written, so a Capture holding one
	// that is not a JPEG is refused whole rather than half written.
	encoded := make([][]byte, len(images))
	for index, picture := range images {
		data, err := os.ReadFile(picture.source)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", filepath.Base(picture.source), err)
		}
		encoded[index], err = imaging.TranscodeJPEG(data, imaging.JPEGOptions{
			MaxSide: ctx.Settings.ImageMaxSide,
			Quality: ctx.Settings.ImageQuality,
		})
		if err != nil {
			return "", fmt.Errorf("%s: %w", filepath.Base(picture.source), err)
		}
	}
	names := make([]string, len(images))
	for index, picture := range images {
		destination, _ := ctx.Settings.vaultPath(picture.target)
		if err := publishImage(encoded[index], destination); err != nil {
			return "", fmt.Errorf("write %s: %w", picture.target, err)
		}
		names[index] = picture.name
	}
	return appendEntry(ctx, names)
}

// publishImage writes a re-encoded picture to its place in the vault, verified
// the way every file this toolbox publishes is. The encoded bytes are written
// to a temporary file outside the vault first, so what is copied in is a file
// read back from a disk, not a buffer that merely was one.
//
// A picture already at its place is this Capture's own from a run that wrote
// the pictures and not the entry — the name is the Capture's — so it is kept
// rather than written again or refused.
func publishImage(data []byte, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return nil
	}
	staged, err := os.CreateTemp("", "dgs-capture-*.jpg")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	_, err = verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: staged.Name(), Destination: destination})
	if errors.Is(err, verifiedcopy.ErrDestinationExists) {
		return nil
	}
	return err
}

// dailyImages is where each of the Capture's pictures will go. The names come
// from the Capture's own time and the picture's place among its pictures, so a
// second run names them the same and never makes a second copy.
func dailyImages(ctx Context, note string) ([]dailyImage, error) {
	day, err := captureDay(ctx)
	if err != nil {
		return nil, err
	}
	stamp := day.Format("20060102150405.000")
	stamp = strings.Replace(stamp, ".", "", 1)
	var images []dailyImage
	var folder string
	for _, attachment := range ctx.Capture.Index.Attachments {
		if !strings.EqualFold(attachment.Kind, imageKind) || attachment.Name == "" {
			continue
		}
		name := filepath.Clean(attachment.Name)
		if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
			return nil, fmt.Errorf("attachment %q is not a file in the Capture", attachment.Name)
		}
		// The folder is worked out at the first picture, so a Capture with
		// none never depends on it.
		if folder == "" {
			if folder, err = imageFolder(ctx, note, day); err != nil {
				return nil, err
			}
		}
		file := fmt.Sprintf("file-%s-%d.jpg", stamp, len(images)+1)
		images = append(images, dailyImage{
			source: filepath.Join(ctx.Capture.Path, name),
			target: path.Join(folder, file),
			name:   file,
		})
	}
	return images, nil
}

// imageFolder is the vault-relative folder a note's pictures go into, from the
// configured template, resolved against the note's own folder.
func imageFolder(ctx Context, note string, day time.Time) (string, error) {
	text := strings.TrimSpace(ctx.Settings.ImageFolder)
	if text == "" {
		text = DefaultImageFolder
	}
	parsed, err := template.New("image folder").Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("image folder: %w", err)
	}
	var out strings.Builder
	err = parsed.Execute(&out, ImageFolderData{
		Note:  strings.TrimSuffix(path.Base(note), path.Ext(note)),
		Date:  day.Format("2006-01-02"),
		Year:  day.Format("2006"),
		Month: day.Format("01"),
		Day:   day.Format("02"),
	})
	if err != nil {
		return "", fmt.Errorf("image folder: %w", err)
	}
	folder := path.Clean(path.Join(path.Dir(note), strings.TrimSpace(out.String())))
	// The folder is relative to the note and must stay inside the vault: a
	// template climbing out of it would write pictures nobody would find.
	if folder == ".." || strings.HasPrefix(folder, "../") || path.IsAbs(folder) {
		return "", fmt.Errorf("image folder %q leaves the vault", folder)
	}
	return folder, nil
}
