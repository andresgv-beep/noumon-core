package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type studioMediaMaterialization struct {
	sidecarPath    string
	preparedPath   string
	oldSidecar     []byte
	hadSidecar     bool
	restoreSidecar bool
	newFiles       []string
}

func (m *studioMediaMaterialization) rollback() {
	for _, path := range m.newFiles {
		_ = os.Remove(path)
	}
	if m.preparedPath != "" {
		_ = os.Remove(m.preparedPath)
	}
	if m.restoreSidecar && m.hadSidecar {
		_ = os.WriteFile(m.sidecarPath, m.oldSidecar, 0o644)
	} else if m.restoreSidecar && m.sidecarPath != "" {
		_ = os.Remove(m.sidecarPath)
	}
}

func (m *studioMediaMaterialization) finalize() error {
	if m.preparedPath == "" || m.sidecarPath == "" {
		return errors.New("studio publication package is incomplete")
	}
	if err := replaceStudioFile(m.preparedPath, m.sidecarPath); err != nil {
		return err
	}
	m.preparedPath = ""
	return nil
}

func (s *Server) publishStudioContent(id string, editor *User) (StudioDocument, error) {
	s.studioPublishMu.Lock()
	defer s.studioPublishMu.Unlock()
	current, err := s.store.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	surface, ok := studioSurfaceForTemplate(current.TemplateKey)
	if !ok {
		return StudioDocument{}, errors.New("studio publication surface unsupported")
	}
	if surface == "documents" {
		return s.store.publishStudioDocument(id, editor)
	}
	return s.publishStudioMediaDocument(current, surface, editor)
}

func (s *Server) publishStudioMediaDocument(
	current StudioDocument,
	surface string,
	editor *User,
) (StudioDocument, error) {
	if s.mediaRoot == "" || s.media == nil {
		return StudioDocument{}, errors.New("studio media publication unavailable")
	}
	valid, err := validateStudioInput(StudioDocumentInput{
		TemplateKey: current.TemplateKey, Title: current.Title,
		Summary: current.Summary, Language: current.Language,
		AuthorLabel: current.AuthorLabel, Tags: current.Tags,
		Metadata: current.Metadata, Content: current.Content,
	})
	if err != nil {
		return StudioDocument{}, err
	}
	metadata, _, err := validateStudioMediaMetadata(current.TemplateKey, current.Metadata)
	if err != nil {
		return StudioDocument{}, err
	}
	if err := studioMediaReadyForPublication(current.TemplateKey, metadata); err != nil {
		return StudioDocument{}, err
	}
	assets, err := s.studioMediaAssets(current.ID, metadata)
	if err != nil {
		return StudioDocument{}, err
	}
	if err := validateStudioMediaAssetRoles(current.TemplateKey, metadata, assets); err != nil {
		return StudioDocument{}, err
	}

	materialized, targetCollection, err := s.materializeStudioMedia(
		current, surface, metadata, assets)
	if err != nil {
		return StudioDocument{}, err
	}
	committed := false
	defer func() {
		if !committed {
			materialized.rollback()
		}
	}()

	now := time.Now().Unix()
	tx, err := s.store.db.Begin()
	if err != nil {
		return StudioDocument{}, err
	}
	defer tx.Rollback()
	if err := ensureStudioAssets(tx, current.ID, valid.Assets, true); err != nil {
		return StudioDocument{}, err
	}
	result, err := tx.Exec(`
		UPDATE studio_documents SET
			status='published', published_revision=revision,
			publication_kind=?, publication_target=?,
			published_plain_text=?, published=?, updated=?
		WHERE id=? AND revision=?`,
		surface, targetCollection, valid.PlainText, now, now,
		current.ID, current.Revision)
	if err != nil {
		return StudioDocument{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StudioDocument{}, errStudioConflict
	}
	if err := replaceStudioPublishedLinks(tx, current.ID, nil); err != nil {
		return StudioDocument{}, err
	}
	if _, err := tx.Exec(`DELETE FROM studio_published_fts WHERE document_id=?`, current.ID); err != nil {
		return StudioDocument{}, err
	}
	if _, err := tx.Exec(`
		INSERT INTO content_origins
			(document_id, origin_content_id, origin_creator_key, origin_version, imported)
		VALUES (?,?,?,?,?)
		ON CONFLICT(document_id) DO UPDATE SET origin_version=excluded.origin_version`,
		current.ID, current.ID, "local:"+editor.Username,
		studioRevisionString(current.Revision), now); err != nil {
		return StudioDocument{}, err
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO collection_access
			(collection_id, access, min_age, allow_download, updated)
		VALUES (?, 'login', 0, 0, ?)`, targetCollection, now); err != nil {
		return StudioDocument{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, err
	}
	if err := materialized.finalize(); err != nil {
		_ = s.restoreStudioMediaPublicationState(current)
		return StudioDocument{}, err
	}
	committed = true
	s.cleanupPreviousStudioMedia(current, materialized)
	s.media.invalidate()
	s.invalidateAccessCache()

	current.Status = "published"
	current.PublishedRevision = &current.Revision
	current.PublicationKind = surface
	current.PublicationTarget = targetCollection
	current.Published = &now
	current.Updated = now
	return current, nil
}

func (s *Server) studioMediaAssets(
	documentID string,
	metadata StudioMediaMetadata,
) (map[string]StudioAsset, error) {
	ids := []string{
		metadata.PrimaryAssetID, metadata.CoverAssetID, metadata.ChannelAvatarAssetID,
	}
	for _, track := range metadata.Tracks {
		ids = append(ids, track.AssetID, track.WaveformAssetID)
	}
	for _, subtitle := range metadata.Subtitles {
		ids = append(ids, subtitle.AssetID)
	}
	assets := map[string]StudioAsset{}
	for _, id := range ids {
		if id == "" || assets[id].ID != "" {
			continue
		}
		asset, err := s.studioAsset(id, documentID)
		if err != nil || (asset.State != "staged" && asset.State != "published") {
			return nil, errStudioAssetInvalid
		}
		assets[id] = asset
	}
	return assets, nil
}

func validateStudioMediaAssetRoles(
	template string,
	metadata StudioMediaMetadata,
	assets map[string]StudioAsset,
) error {
	is := func(id string, allowed func(string) bool) bool {
		return id == "" || (assets[id].ID != "" && allowed(assets[id].MIMEType))
	}
	image := func(mimeType string) bool { return strings.HasPrefix(mimeType, "image/") }
	audio := func(mimeType string) bool { return strings.HasPrefix(mimeType, "audio/") }
	video := func(mimeType string) bool { return strings.HasPrefix(mimeType, "video/") }
	if !is(metadata.CoverAssetID, image) || !is(metadata.ChannelAvatarAssetID, image) {
		return errStudioAssetInvalid
	}
	for _, track := range metadata.Tracks {
		if !is(track.AssetID, audio) || !is(track.WaveformAssetID, image) {
			return errStudioAssetInvalid
		}
	}
	for _, subtitle := range metadata.Subtitles {
		if !is(subtitle.AssetID, func(mimeType string) bool { return mimeType == "text/vtt" }) {
			return errStudioAssetInvalid
		}
	}
	if metadata.PrimaryAssetID == "" {
		return nil
	}
	switch template {
	case "moments.video", "cabinet.video":
		if !is(metadata.PrimaryAssetID, video) {
			return errStudioAssetInvalid
		}
	case "cabinet.audio":
		if !is(metadata.PrimaryAssetID, audio) {
			return errStudioAssetInvalid
		}
	case "cabinet.pdf":
		if !is(metadata.PrimaryAssetID, func(mimeType string) bool { return mimeType == "application/pdf" }) {
			return errStudioAssetInvalid
		}
	case "cabinet.reader":
		if !is(metadata.PrimaryAssetID, func(mimeType string) bool {
			return mimeType == "application/epub+zip" ||
				mimeType == "text/plain" || mimeType == "text/markdown"
		}) {
			return errStudioAssetInvalid
		}
	case "cabinet.gallery":
		if !is(metadata.PrimaryAssetID, image) {
			return errStudioAssetInvalid
		}
	default:
		return errStudioAssetInvalid
	}
	return nil
}

func (s *Server) materializeStudioMedia(
	document StudioDocument,
	surface string,
	metadata StudioMediaMetadata,
	assets map[string]StudioAsset,
) (studioMediaMaterialization, string, error) {
	var materialized studioMediaMaterialization
	appDir, ok := appDirFor(surface)
	if !ok {
		return materialized, "", errors.New("invalid studio surface")
	}
	dir, err := secureStudioPublicationDir(s.mediaRoot, appDir, metadata.Collection)
	if err != nil {
		return materialized, "", err
	}
	base := "studio-" + document.ID
	materialized.sidecarPath = filepath.Join(dir, base+".json")
	if old, readErr := os.ReadFile(materialized.sidecarPath); readErr == nil {
		materialized.oldSidecar = old
		materialized.hadSidecar = true
	} else if !os.IsNotExist(readErr) {
		return materialized, "", readErr
	}

	copyRole := func(assetID, role string) (string, error) {
		if assetID == "" {
			return "", nil
		}
		asset := assets[assetID]
		extension := studioExtensionForMIME(asset.MIMEType)
		name := fmt.Sprintf("%s-r%d-%s-%s%s",
			base, document.Revision, role, asset.ID, extension)
		path := filepath.Join(dir, name)
		created, err := s.copyStudioAssetToPublication(document.ID, asset, path)
		if err != nil {
			return "", err
		}
		if created {
			materialized.newFiles = append(materialized.newFiles, path)
		}
		return name, nil
	}

	primary, err := copyRole(metadata.PrimaryAssetID, "media")
	if err != nil {
		materialized.rollback()
		return materialized, "", err
	}
	cover, err := copyRole(metadata.CoverAssetID, "cover")
	if err != nil {
		materialized.rollback()
		return materialized, "", err
	}
	avatar, err := copyRole(metadata.ChannelAvatarAssetID, "avatar")
	if err != nil {
		materialized.rollback()
		return materialized, "", err
	}
	tracks := make([]sidecarTrack, 0, len(metadata.Tracks))
	for index, track := range metadata.Tracks {
		media, copyErr := copyRole(track.AssetID, fmt.Sprintf("track-%03d", index+1))
		if copyErr != nil {
			materialized.rollback()
			return materialized, "", copyErr
		}
		waveform, copyErr := copyRole(track.WaveformAssetID, fmt.Sprintf("wave-%03d", index+1))
		if copyErr != nil {
			materialized.rollback()
			return materialized, "", copyErr
		}
		tracks = append(tracks, sidecarTrack{
			Title: track.Title, Media: media, Waveform: waveform,
		})
		if primary == "" {
			primary = media
		}
	}
	subtitles := make([]sidecarSub, 0, len(metadata.Subtitles))
	for index, subtitle := range metadata.Subtitles {
		file, copyErr := copyRole(subtitle.AssetID, fmt.Sprintf("subtitle-%03d", index+1))
		if copyErr != nil {
			materialized.rollback()
			return materialized, "", copyErr
		}
		subtitles = append(subtitles, sidecarSub{Lang: subtitle.Lang, File: file})
	}
	chapters := make([]sidecarChapter, 0, len(metadata.Chapters))
	for _, chapter := range metadata.Chapters {
		chapters = append(chapters, sidecarChapter{
			Start: chapter.Start, Title: chapter.Title,
		})
	}
	template, collectionType := studioMediaTemplate(document.TemplateKey)
	if template == "" {
		materialized.rollback()
		return materialized, "", errors.New("unsupported studio media template")
	}
	sc := sidecar{
		Template: template, Title: document.Title, Media: primary,
		Author: document.AuthorLabel, Date: metadata.Date,
		Description: document.Summary, Tags: append([]string(nil), document.Tags...),
		Keywords: keywordsFromSubjects(document.Tags), Source: surface,
		SourceID: document.ID, Language: document.Language,
		Contributor: metadata.Contributor, License: metadata.License,
		Cover: cover, Tracks: tracks, Duration: metadata.Duration,
		Subtitles: subtitles, Chapters: chapters,
		ChannelAvatar: avatar,
	}
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		materialized.rollback()
		return materialized, "", err
	}
	data = append(data, '\n')
	materialized.preparedPath = filepath.Join(
		dir, fmt.Sprintf(".%s-r%d.publish.part", base, document.Revision))
	if err := os.WriteFile(materialized.preparedPath, data, 0o644); err != nil {
		materialized.rollback()
		return materialized, "", err
	}
	collectionPath := filepath.Join(dir, "collection.json")
	_, _ = writeJSONFileIfAbsent(collectionPath, collectionMeta{
		Type: collectionType, Template: template, Title: metadata.Collection,
		Source: surface,
	})
	collection := filepath.ToSlash(filepath.Join(appDir, metadata.Collection))
	return materialized, collectionIDForMedia(collection), nil
}

func studioMediaTemplate(templateKey string) (template, collectionType string) {
	switch templateKey {
	case "moments.video", "cabinet.video":
		return "video", "video"
	case "cabinet.audio":
		return "audio", "audio"
	case "cabinet.pdf":
		return "pdf", "pdf"
	case "cabinet.reader":
		return "reader", "documents"
	case "cabinet.gallery":
		return "gallery", "images"
	default:
		return "", ""
	}
}

func secureStudioPublicationDir(root, appDir, collection string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	collection = sanitizeSegment(collection)
	if collection == "" {
		return "", errors.New("invalid publication collection")
	}
	appPath := filepath.Join(root, appDir)
	dir := filepath.Join(appPath, collection)
	for _, path := range []string{root, appPath, dir} {
		if path != root {
			if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
				return "", err
			}
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("publication path is not a real directory")
		}
	}
	return dir, nil
}

func (s *Server) copyStudioAssetToPublication(
	documentID string,
	asset StudioAsset,
	destination string,
) (bool, error) {
	if existing, err := os.Lstat(destination); err == nil {
		if existing.Mode().IsRegular() && existing.Size() == asset.SizeBytes {
			return false, nil
		}
		return false, errors.New("publication asset collision")
	} else if !os.IsNotExist(err) {
		return false, err
	}
	dir, err := secureStudioAssetDir(s.studioRoot, documentID, false)
	if err != nil {
		return false, err
	}
	source := filepath.Join(dir, asset.ID+studioExtensionForMIME(asset.MIMEType))
	info, err := os.Lstat(source)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errStudioAssetInvalid
	}
	input, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer input.Close()
	temporary := destination + ".part"
	_ = os.Remove(temporary)
	// Studio y el catálogo suelen vivir en el mismo pool. Un enlace físico
	// conserva el snapshot sin duplicar gigabytes; si el volumen no lo admite,
	// se degrada de forma transparente a una copia en streaming.
	if err := os.Link(source, temporary); err == nil {
		if err := os.Rename(temporary, destination); err != nil {
			_ = os.Remove(temporary)
			return false, err
		}
		return true, nil
	}
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(temporary)
		if copyErr != nil {
			return false, copyErr
		}
		return false, closeErr
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return false, err
	}
	return true, nil
}

func (s *Server) unpublishStudioContent(id string, editor *User) (StudioDocument, error) {
	s.studioPublishMu.Lock()
	defer s.studioPublishMu.Unlock()
	current, err := s.store.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	if current.PublicationKind == "" || current.PublicationKind == "documents" {
		return s.store.unpublishStudioDocument(id, editor)
	}
	backup, err := s.withdrawStudioMediaSidecar(current)
	if err != nil {
		return StudioDocument{}, err
	}
	updated, err := s.store.unpublishStudioDocument(id, editor)
	if err != nil {
		backup.rollback()
		return StudioDocument{}, err
	}
	s.removeStudioMediaFiles(current)
	if s.media != nil {
		s.media.invalidate()
	}
	return updated, nil
}

func (s *Server) archiveStudioContent(id string, editor *User) (StudioDocument, error) {
	s.studioPublishMu.Lock()
	defer s.studioPublishMu.Unlock()
	current, err := s.store.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	if current.Status == "archived" || current.PublicationKind == "" ||
		current.PublicationKind == "documents" {
		return s.store.archiveStudioDocument(id, editor)
	}
	backup, err := s.withdrawStudioMediaSidecar(current)
	if err != nil {
		return StudioDocument{}, err
	}
	updated, err := s.store.archiveStudioDocument(id, editor)
	if err != nil {
		backup.rollback()
		return StudioDocument{}, err
	}
	s.removeStudioMediaFiles(current)
	if s.media != nil {
		s.media.invalidate()
	}
	return updated, nil
}

func (s *Server) withdrawStudioMediaSidecar(document StudioDocument) (studioMediaMaterialization, error) {
	var backup studioMediaMaterialization
	collection, ok := studioMediaCollectionPath(document.PublicationTarget)
	if !ok {
		return backup, errors.New("invalid studio publication target")
	}
	path := filepath.Join(s.mediaRoot, filepath.FromSlash(collection), "studio-"+document.ID+".json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return backup, nil
	}
	if err != nil {
		return backup, err
	}
	backup.sidecarPath = path
	backup.oldSidecar = raw
	backup.hadSidecar = true
	backup.restoreSidecar = true
	if err := os.Remove(path); err != nil {
		return studioMediaMaterialization{}, err
	}
	return backup, nil
}

func (s *Server) restoreStudioMediaPublicationState(previous StudioDocument) error {
	var publishedRevision any
	if previous.PublishedRevision != nil {
		publishedRevision = *previous.PublishedRevision
	}
	var published any
	if previous.Published != nil {
		published = *previous.Published
	}
	_, err := s.store.db.Exec(`
		UPDATE studio_documents SET
			status=?, published_revision=?, publication_kind=?,
			publication_target=?, published=?, updated=?
		WHERE id=? AND revision=?`,
		previous.Status, publishedRevision, nullableStudioString(previous.PublicationKind),
		nullableStudioString(previous.PublicationTarget), published, previous.Updated,
		previous.ID, previous.Revision)
	return err
}

func nullableStudioString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func studioMediaCollectionPath(target string) (string, bool) {
	const prefix = "col:media:"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	decoded, ok := decodeOpaque(strings.TrimPrefix(target, prefix))
	if !ok {
		return "", false
	}
	decoded = filepath.ToSlash(strings.Trim(decoded, "/"))
	parts := strings.Split(decoded, "/")
	if len(parts) != 2 || (parts[0] != "Moments" && parts[0] != "Cabinet") ||
		parts[1] == "" || sanitizeSegment(parts[1]) != parts[1] {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

func (s *Server) removeStudioMediaFiles(document StudioDocument) {
	collection, ok := studioMediaCollectionPath(document.PublicationTarget)
	if !ok {
		return
	}
	dir := filepath.Join(s.mediaRoot, filepath.FromSlash(collection))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := "studio-" + document.ID + "-"
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func (s *Server) cleanupPreviousStudioMedia(
	previous StudioDocument,
	current studioMediaMaterialization,
) {
	if previous.PublicationTarget != "" {
		collection, ok := studioMediaCollectionPath(previous.PublicationTarget)
		if ok {
			dir := filepath.Join(s.mediaRoot, filepath.FromSlash(collection))
			if filepath.Clean(filepath.Join(dir, "studio-"+previous.ID+".json")) !=
				filepath.Clean(current.sidecarPath) {
				_ = os.Remove(filepath.Join(dir, "studio-"+previous.ID+".json"))
			}
			entries, _ := os.ReadDir(dir)
			prefix := "studio-" + previous.ID + "-"
			currentRevision := fmt.Sprintf("studio-%s-r%d-", previous.ID, previous.Revision)
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) &&
					!strings.HasPrefix(entry.Name(), currentRevision) {
					_ = os.Remove(filepath.Join(dir, entry.Name()))
				}
			}
		}
	}
}

func (s *Server) studioPublishedMediaOwnedBy(userID int64) ([]StudioDocument, error) {
	rows, err := s.store.db.Query(`
		SELECT id FROM studio_documents
		WHERE owner_user_id=? AND published_revision IS NOT NULL
		  AND status!='archived' AND publication_kind IN ('moments','cabinet')
		ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	documents := make([]StudioDocument, 0, len(ids))
	for _, id := range ids {
		document, err := s.store.publishedStudioSnapshot(id)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}
