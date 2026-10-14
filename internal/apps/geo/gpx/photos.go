// Waypoint photos. Only a GPX dgs wrote, on disk, holds them: each photo is a
// WebP in the folder beside it, encoded by the page, <name>.photos/, and a <link> of the waypoint
// in the GPX itself. A photo is published into the folder when it is added;
// the GPX links it only once the edit is saved, and a photo no saved waypoint
// links to is moved into the folder's .trash/. Nothing in it is deleted.

package gpx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
	"dgs-toolbox/internal/geo/stops"
	"dgs-toolbox/internal/verifiedcopy"

	_ "golang.org/x/image/webp" // registers WebP with image.DecodeConfig
)

// maxPhotoUpload bounds one uploaded image, so a request cannot fill memory.
const maxPhotoUpload = 64 << 20

// photoName is how a published photo is named: the first 12 hex digits of
// its SHA-256 and .webp.
var photoName = regexp.MustCompile(`^[0-9a-f]{12}\.webp$`)

// photoDir is the folder beside a GPX that holds its photos, as a GPX link
// names it: trip.gpx has trip.photos.
func photoDir(path string) string {
	if isDraft(path) {
		id, _, _ := strings.Cut(strings.TrimPrefix(path, draftPrefix), "/")
		return "draft-" + id + ".photos"
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".photos"
}

// splitLinks parts a waypoint's links into its photos — those into the GPX's
// own photo folder — and the rest, which are kept as they are.
func splitLinks(links []gpxfile.Link, dir string) (others []gpxfile.Link, photos []sidecar.Photo) {
	for _, link := range links {
		name, ok := strings.CutPrefix(link.Href, dir+"/")
		if ok && photoName.MatchString(name) {
			photos = append(photos, sidecar.Photo{Name: name, Caption: link.Text})
		} else {
			others = append(others, link)
		}
	}
	return others, photos
}

// photoLinks are the links a waypoint is written with: the ones it keeps, and
// one per photo in order.
func photoLinks(others []gpxfile.Link, photos []sidecar.Photo, dir string) []gpxfile.Link {
	links := append([]gpxfile.Link(nil), others...)
	for _, photo := range photos {
		links = append(links, gpxfile.Link{Href: dir + "/" + photo.Name, Text: photo.Caption, Type: "image/webp"})
	}
	return links
}

// waypointPhotos is what the page shows of a waypoint: what is held for it
// when anything is, else what the GPX says.
func waypointPhotos(a analysis, key string, wpt gpxfile.Waypoint) sidecar.Photos {
	if held, ok := a.cleaning.Photos[key]; ok {
		return held
	}
	_, photos := splitLinks(wpt.Links, photoDir(a.path))
	return sidecar.Photos{Description: wpt.Description, Photos: photos}
}

// transferWaypoint copies the edited waypoint and its photos before any source is removed.
func transferWaypoint(a analysis, key string, wpt gpxfile.Waypoint, target string) (gpxfile.Waypoint, error) {
	shown := waypointPhotos(a, key, wpt)
	others, _ := splitLinks(wpt.Links, photoDir(a.path))
	if name, ok := a.cleaning.Names[key]; ok {
		wpt.Name = name
	}
	wpt.Description = shown.Description
	photos := make([]sidecar.Photo, 0, len(shown.Photos))
	for _, photo := range shown.Photos {
		data, err := readPhoto(a.path, photo.Name)
		if err != nil {
			return wpt, err
		}
		var name string
		if isDraft(target) {
			name, err = drafts.keepPhoto(target, data)
		} else {
			name, err = publishPhoto(filepath.Join(filepath.Dir(target), photoDir(target)), data)
		}
		if err != nil {
			return wpt, err
		}
		photos = append(photos, sidecar.Photo{Name: name, Caption: photo.Caption})
	}
	wpt.Links = photoLinks(others, photos, photoDir(target))
	return wpt, nil
}

func readPhoto(path, name string) ([]byte, error) {
	if !photoName.MatchString(name) {
		return nil, errors.New("invalid photo name")
	}
	if !isDraft(path) {
		return os.ReadFile(filepath.Join(filepath.Dir(path), photoDir(path), name))
	}
	drafts.mu.Lock()
	defer drafts.mu.Unlock()
	data, ok := drafts.photos[path][name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, os.ErrNotExist)
	}
	return data, nil
}

func (s *draftStore) keepPhoto(path string, data []byte) (string, error) {
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != "webp" {
		return "", errors.New("a photo is sent as WebP")
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:])[:12] + ".webp"
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.drafts[path]; !ok {
		return "", errNoDraft
	}
	if s.photos == nil {
		s.photos = map[string]map[string][]byte{}
	}
	if s.photos[path] == nil {
		s.photos[path] = map[string][]byte{}
	}
	if before, ok := s.photos[path][name]; ok && !bytes.Equal(before, data) {
		return "", errors.New("photo name is already occupied by different bytes")
	}
	s.photos[path][name] = data
	return name, nil
}

// ownGPX refuses any path but a GPX dgs wrote, on disk: a draft has no folder
// to hold photos, and a recording is never written.
func ownGPX(path string) error {
	if isDraft(path) {
		return errors.New("save the new GPX before adding photos to its waypoints")
	}
	if err := checkGPX(path); err != nil {
		return err
	}
	file, err := gpxfile.Open(path)
	if err != nil {
		return err
	}
	if !file.IsOurs() {
		return fmt.Errorf("%s: %w", path, gpxfile.ErrNotOurs)
	}
	return nil
}

// addPhoto publishes one uploaded WebP, as the page encoded it, into the GPX's
// photo folder, answering its name. The same picture added twice is one file.
func (a api) addPhoto(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if err := ownGPX(path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhotoUpload))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != "webp" {
		writeError(w, http.StatusUnprocessableEntity, errors.New("a photo is sent as WebP"))
		return
	}
	name, err := publishPhoto(filepath.Join(filepath.Dir(path), photoDir(path)), data)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]string{"name": name})
}

// publishPhoto writes data into dir under its digest. The final name appears
// only once a same-folder .dgs-part file reads back to the same SHA-256 and
// decodes as a WebP.
func publishPhoto(dir string, data []byte) (string, error) {
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != "webp" {
		return "", errors.New("a photo is sent as WebP")
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:])[:12] + ".webp"
	final := filepath.Join(dir, name)
	if before, err := os.ReadFile(final); err == nil {
		if !bytes.Equal(before, data) {
			return "", fmt.Errorf("%s: photo name is already occupied by different bytes", final)
		}
		return name, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	part := final + ".dgs-part"
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer os.Remove(part)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	back, err := os.ReadFile(part)
	if err != nil {
		return "", err
	}
	if sha256.Sum256(back) != sum {
		return "", fmt.Errorf("%s: the photo did not read back as written", part)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(back)); err != nil || format != "webp" {
		return "", fmt.Errorf("%s: the photo read back does not decode", part)
	}
	if err := verifiedcopy.Publish(part, final); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		existing, err := os.ReadFile(final)
		if err != nil {
			return "", err
		}
		if !bytes.Equal(existing, data) {
			return "", fmt.Errorf("%s: photo name is already occupied by different bytes", final)
		}
	}
	return name, verifiedcopy.SyncDirectory(dir)
}

// setPhotos holds a waypoint's description and photos until the edit is
// saved.
func (a api) setPhotos(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Key  string `json:"key"`
		sidecar.Photos
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if err := ownGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if kind, _, err := partKey(body.Key); err != nil || kind != 'w' {
		writeError(w, http.StatusBadRequest, errors.New(`a waypoint is keyed "w0"`))
		return
	}
	dir := filepath.Join(filepath.Dir(body.Path), photoDir(body.Path))
	for _, photo := range body.Photos.Photos {
		if !photoName.MatchString(photo.Name) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%q is not a photo of this GPX", photo.Name))
			return
		}
		if _, err := os.Stat(filepath.Join(dir, photo.Name)); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
	}
	result, err := analyse(body.Path, stops.Defaults())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if result.sidecarErr != nil {
		writeError(w, http.StatusUnprocessableEntity, result.sidecarErr)
		return
	}
	if _, err := locate(result, body.Key); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	file := result.cleaning
	if file.Photos == nil {
		file.Photos = map[string]sidecar.Photos{}
	}
	body.Photos.Description = strings.TrimSpace(body.Photos.Description)
	file.Photos[body.Key] = body.Photos
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// servePhoto answers one photo a waypoint of the GPX shows, unsaved edits
// included. Any other name is refused, so no other file is read.
func (a api) servePhoto(w http.ResponseWriter, r *http.Request) {
	path, name := r.URL.Query().Get("path"), r.URL.Query().Get("name")
	if !photoName.MatchString(name) {
		writeError(w, http.StatusBadRequest, errors.New("invalid photo name"))
		return
	}
	result, err := analyse(path, stops.Defaults())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	for i, wpt := range result.file.Waypoints {
		for _, photo := range waypointPhotos(result, result.keys.Waypoints[i], wpt).Photos {
			if photo.Name == name {
				w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
				if isDraft(path) {
					data, err := readPhoto(path, name)
					if err != nil {
						writeError(w, statusFor(err), err)
						return
					}
					w.Header().Set("Content-Type", "image/webp")
					_, _ = w.Write(data)
					return
				}
				http.ServeFile(w, r, filepath.Join(filepath.Dir(path), photoDir(path), name))
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, errors.New("no waypoint of this GPX shows that photo"))
}

// trashUnlinked moves every photo no waypoint of the saved GPX links to into
// the photo folder's .trash/.
func trashUnlinked(path string) error {
	file, err := gpxfile.Open(path)
	if err != nil {
		return err
	}
	if !file.IsOurs() {
		return nil
	}
	dir := filepath.Join(filepath.Dir(path), photoDir(path))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	linked := map[string]bool{}
	for _, wpt := range file.Waypoints {
		_, photos := splitLinks(wpt.Links, photoDir(path))
		for _, photo := range photos {
			linked[photo.Name] = true
		}
	}
	// Photos added but not saved yet are linked by what is held.
	if held, ok := pending.get(path); ok {
		for _, photos := range held.Photos {
			for _, photo := range photos.Photos {
				linked[photo.Name] = true
			}
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || !photoName.MatchString(entry.Name()) || linked[entry.Name()] {
			continue
		}
		trash := filepath.Join(dir, ".trash")
		if err := os.MkdirAll(trash, 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(dir, entry.Name()), filepath.Join(trash, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// renamePhotos follows a renamed GPX: its photo folder takes the new name and
// the GPX's photo links point into it.
func renamePhotos(from, to string) error {
	oldDir, newDir := filepath.Join(filepath.Dir(from), photoDir(from)), filepath.Join(filepath.Dir(to), photoDir(to))
	if _, err := os.Stat(oldDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}
	file, err := gpxfile.Open(to)
	if err != nil {
		return err
	}
	// A recording's photos are named in its sidecar, by file name only: the
	// folder moving is all. Only a GPX dgs wrote links them itself.
	if !file.IsOurs() {
		return nil
	}
	return gpxfile.Update(to, func(staged string) error {
		for i, wpt := range file.Waypoints {
			others, photos := splitLinks(wpt.Links, photoDir(from))
			if len(photos) == 0 {
				continue
			}
			if err := gpxfile.SetWaypointLinks(staged, i, wpt.Description, photoLinks(others, photos, photoDir(to))); err != nil {
				return err
			}
		}
		return nil
	})
}
