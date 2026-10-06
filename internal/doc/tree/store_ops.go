package tree

import (
	"context"
	"time"
)

func (s *Store) AddRevisionWithMetadata(ctx context.Context, id string, source string, template Template, given map[string]string, metadata RevisionMetadata, now time.Time) (Item, error) {
	return AddRevisionWithMetadata(ctx, s.root, id, source, template, given, metadata, now)
}
func (s *Store) AddWithoutPDF(id string, template Template, given map[string]string, metadata RevisionMetadata, now time.Time) (Item, error) {
	return AddWithoutPDF(s.root, id, template, given, metadata, now)
}
func (s *Store) AttachFileAs(ctx context.Context, id string, ref string, sources []string, keepOriginal bool, newRevision bool, now time.Time) (Item, error) {
	return AttachFileAs(ctx, s.root, id, ref, sources, keepOriginal, newRevision, now)
}
func (s *Store) ChangeType(id string, target Template, fields map[string]string, revisions map[string]map[string]string, now time.Time) (Item, error) {
	return ChangeType(s.root, id, target, fields, revisions, now)
}
func (s *Store) GetItem(id string) (Item, error)       { return GetItem(s.root, id) }
func (s *Store) LoadAllTemplates() ([]Template, error) { return LoadAllTemplates(s.root) }
func (s *Store) LoadTemplates() ([]Template, error)    { return LoadTemplates(s.root) }
func (s *Store) OriginalPath(item Item, pdf string, o Original) string {
	return OriginalPath(s.root, item, pdf, o)
}
func (s *Store) RecordExports(published []Exported, now time.Time, progress func(done, total int)) error {
	return RecordExports(s.root, published, now, progress)
}
func (s *Store) Require() error { return Require(s.root) }
func (s *Store) SaveTemplate(previous string, data []byte, now time.Time) (Template, error) {
	return SaveTemplate(s.root, previous, data, now)
}
func (s *Store) SetFieldsAs(id string, digest string, template Template, given map[string]string, newRevision bool, now time.Time) (Item, error) {
	return SetFieldsAs(s.root, id, digest, template, given, newRevision, now)
}
func (s *Store) SetHead(id string, digest string, now time.Time) (Item, error) {
	return SetHead(s.root, id, digest, now)
}
func (s *Store) Supersede(oldID string, newID string, now time.Time) (Item, error) {
	return Supersede(s.root, oldID, newID, now)
}
func (s *Store) Trash(id string, now time.Time) (string, error) { return Trash(s.root, id, now) }
func (s *Store) TrashRevision(id string, digest string, now time.Time) (Item, string, error) {
	return TrashRevision(s.root, id, digest, now)
}
func (s *Store) TrashTemplate(typ string, now time.Time) (string, error) {
	return TrashTemplate(s.root, typ, now)
}
func (s *Store) UndoSupersession(id string, now time.Time) (Item, error) {
	return UndoSupersession(s.root, id, now)
}
func (s *Store) ValidateSupersession(oldID string, typ string, fields map[string]string) error {
	return ValidateSupersession(s.root, oldID, typ, fields)
}

func (s *Store) Import(ctx context.Context, request ImportRequest) (Item, error) {
	request.Root = s.root
	return Import(ctx, request)
}
func (s *Store) CreateWithoutPDF(request ImportRequest) (Item, error) {
	request.Root = s.root
	return CreateWithoutPDF(request)
}
func (s *Store) MakeRevision(id, ref string, now time.Time) (Item, error) {
	return MakeRevision(s.root, id, ref, now)
}
func (s *Store) MakeCorrection(id, ref string, now time.Time) (Item, error) {
	return MakeCorrection(s.root, id, ref, now)
}
