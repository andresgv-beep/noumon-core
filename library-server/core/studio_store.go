package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const studioDocumentColumns = `
	id, owner_user_id, template_key, status, title, summary, language,
	author_label, tags_json, classification_json, metadata_json, content_json,
	cover_asset_id, revision, published_revision, publication_kind,
	publication_target, created, updated, published`

type StudioDocumentSummary struct {
	ID                string               `json:"id"`
	TemplateKey       string               `json:"templateKey"`
	Status            string               `json:"status"`
	Title             string               `json:"title"`
	Summary           string               `json:"summary,omitempty"`
	Language          string               `json:"language,omitempty"`
	AuthorLabel       string               `json:"authorLabel,omitempty"`
	Tags              []string             `json:"tags"`
	Classification    StudioClassification `json:"classification"`
	Revision          int                  `json:"revision"`
	PublishedRevision *int                 `json:"publishedRevision,omitempty"`
	PublicationKind   string               `json:"publicationKind,omitempty"`
	Created           int64                `json:"created"`
	Updated           int64                `json:"updated"`
	Published         *int64               `json:"published,omitempty"`
}

type studioScanner interface {
	Scan(dest ...any) error
}

func scanStudioDocument(row studioScanner) (StudioDocument, error) {
	var doc StudioDocument
	var owner sql.NullInt64
	var cover, pubKind, pubTarget sql.NullString
	var pubRevision sql.NullInt64
	var published sql.NullInt64
	var tagsJSON, classificationJSON, metadataJSON, contentJSON string
	err := row.Scan(
		&doc.ID, &owner, &doc.TemplateKey, &doc.Status, &doc.Title, &doc.Summary,
		&doc.Language, &doc.AuthorLabel, &tagsJSON, &classificationJSON,
		&metadataJSON, &contentJSON, &cover, &doc.Revision, &pubRevision,
		&pubKind, &pubTarget, &doc.Created, &doc.Updated, &published,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return doc, errStudioNotFound
		}
		return doc, err
	}
	if owner.Valid {
		v := owner.Int64
		doc.OwnerUserID = &v
	}
	if cover.Valid {
		doc.CoverAssetID = cover.String
	}
	if pubRevision.Valid {
		v := int(pubRevision.Int64)
		doc.PublishedRevision = &v
	}
	if pubKind.Valid {
		doc.PublicationKind = pubKind.String
	}
	if pubTarget.Valid {
		doc.PublicationTarget = pubTarget.String
	}
	if published.Valid {
		v := published.Int64
		doc.Published = &v
	}
	if err := json.Unmarshal([]byte(tagsJSON), &doc.Tags); err != nil {
		return doc, fmt.Errorf("studio tags: %w", err)
	}
	if doc.Tags == nil {
		doc.Tags = []string{}
	}
	if err := json.Unmarshal([]byte(classificationJSON), &doc.Classification); err != nil {
		return doc, fmt.Errorf("studio classification: %w", err)
	}
	doc.Metadata = json.RawMessage(metadataJSON)
	doc.Content = json.RawMessage(contentJSON)
	return doc, nil
}

func studioSummary(doc StudioDocument) StudioDocumentSummary {
	return StudioDocumentSummary{
		ID: doc.ID, TemplateKey: doc.TemplateKey, Status: doc.Status,
		Title: doc.Title, Summary: doc.Summary, Language: doc.Language,
		AuthorLabel: doc.AuthorLabel, Tags: append([]string(nil), doc.Tags...),
		Classification: doc.Classification, Revision: doc.Revision,
		PublishedRevision: doc.PublishedRevision, PublicationKind: doc.PublicationKind,
		Created: doc.Created, Updated: doc.Updated, Published: doc.Published,
	}
}

func (s *Store) createStudioDocument(owner *User, valid studioValidatedInput) (StudioDocument, error) {
	id, err := newStudioID()
	if err != nil {
		return StudioDocument{}, err
	}
	now := time.Now().Unix()
	tagsJSON, _ := json.Marshal(valid.Input.Tags)
	classificationJSON, _ := json.Marshal(valid.Classification)
	doc := StudioDocument{
		ID: id, OwnerUserID: &owner.ID, TemplateKey: valid.Input.TemplateKey,
		Status: "draft", Title: valid.Input.Title, Summary: valid.Input.Summary,
		Language: valid.Input.Language, AuthorLabel: valid.Input.AuthorLabel,
		Tags: valid.Input.Tags, Classification: valid.Classification,
		Metadata: valid.Input.Metadata, Content: valid.Input.Content,
		Revision: 1, Created: now, Updated: now,
	}
	snapshot, err := json.Marshal(doc)
	if err != nil {
		return StudioDocument{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StudioDocument{}, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO studio_documents (
			id, owner_user_id, template_key, status, title, summary, language,
			author_label, tags_json, classification_json, metadata_json,
			content_json, plain_text, revision, created, updated
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		doc.ID, owner.ID, doc.TemplateKey, doc.Status, doc.Title, doc.Summary,
		doc.Language, doc.AuthorLabel, string(tagsJSON), string(classificationJSON),
		string(doc.Metadata), string(doc.Content), valid.PlainText, doc.Revision,
		doc.Created, doc.Updated)
	if err != nil {
		return StudioDocument{}, err
	}
	if _, err = tx.Exec(`
		INSERT INTO studio_revisions
			(document_id, revision, editor_user_id, editor_label, snapshot_json, created)
		VALUES (?,?,?,?,?,?)`,
		doc.ID, doc.Revision, owner.ID, owner.Username, string(snapshot), now); err != nil {
		return StudioDocument{}, err
	}
	if err := replaceStudioDerived(tx, doc.ID, valid); err != nil {
		return StudioDocument{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, err
	}
	return doc, nil
}

func replaceStudioDerived(tx *sql.Tx, documentID string, valid studioValidatedInput) error {
	if _, err := tx.Exec(`DELETE FROM studio_links WHERE source_document_id=?`, documentID); err != nil {
		return err
	}
	for _, target := range valid.Links {
		if _, err := tx.Exec(`INSERT INTO studio_links (source_document_id, target_item_id) VALUES (?,?)`,
			documentID, target); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM studio_facets WHERE document_id=?`, documentID); err != nil {
		return err
	}
	for facet, values := range valid.Facets {
		for _, value := range values {
			if _, err := tx.Exec(`INSERT INTO studio_facets (document_id, facet, value) VALUES (?,?,?)`,
				documentID, facet, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureStudioAssets(tx *sql.Tx, documentID string, assetIDs []string, publish bool) error {
	for _, assetID := range assetIDs {
		var state string
		err := tx.QueryRow(`
			SELECT state FROM studio_assets
			WHERE id=? AND document_id=?`, assetID, documentID).Scan(&state)
		if err == sql.ErrNoRows || state == "deleted" {
			return errStudioAssetInvalid
		}
		if err != nil {
			return err
		}
		if publish && state != "published" {
			result, err := tx.Exec(`
				UPDATE studio_assets SET state='published'
				WHERE id=? AND document_id=? AND state='staged'`, assetID, documentID)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				return errStudioAssetInvalid
			}
		}
	}
	return nil
}

func (s *Store) getStudioDocument(id string, viewer *User) (StudioDocument, error) {
	doc, err := scanStudioDocument(s.db.QueryRow(
		`SELECT `+studioDocumentColumns+` FROM studio_documents WHERE id=?`, id))
	if err != nil {
		return StudioDocument{}, err
	}
	if !viewer.IsAdmin && (doc.OwnerUserID == nil || *doc.OwnerUserID != viewer.ID) {
		return StudioDocument{}, errStudioForbidden
	}
	return doc, nil
}

func (s *Store) listStudioDocuments(viewer *User, status string, limit, offset int) ([]StudioDocumentSummary, error) {
	if limit < 1 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "draft"
	}
	if status != "draft" && status != "published" && status != "archived" && status != "all" {
		return nil, fmt.Errorf("invalid status")
	}
	query := `SELECT ` + studioDocumentColumns + ` FROM studio_documents WHERE `
	args := []any{}
	if viewer.IsAdmin {
		query += `1=1`
	} else {
		query += `owner_user_id=?`
		args = append(args, viewer.ID)
	}
	if status != "all" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY updated DESC, id LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudioDocumentSummary{}
	for rows.Next() {
		doc, err := scanStudioDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, studioSummary(doc))
	}
	return out, rows.Err()
}

func (s *Store) updateStudioDocument(id string, editor *User, valid studioValidatedInput) (StudioDocument, int, error) {
	current, err := s.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, 0, err
	}
	if valid.Input.BaseRevision < 1 || valid.Input.BaseRevision != current.Revision {
		return StudioDocument{}, current.Revision, errStudioConflict
	}
	now := time.Now().Unix()
	nextRevision := current.Revision + 1
	tagsJSON, _ := json.Marshal(valid.Input.Tags)
	classificationJSON, _ := json.Marshal(valid.Classification)
	current.TemplateKey = valid.Input.TemplateKey
	current.Title = valid.Input.Title
	current.Summary = valid.Input.Summary
	current.Language = valid.Input.Language
	current.AuthorLabel = valid.Input.AuthorLabel
	current.Tags = valid.Input.Tags
	current.Classification = valid.Classification
	current.Metadata = valid.Input.Metadata
	current.Content = valid.Input.Content
	current.Revision = nextRevision
	current.Updated = now
	snapshot, err := json.Marshal(current)
	if err != nil {
		return StudioDocument{}, 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StudioDocument{}, 0, err
	}
	defer tx.Rollback()
	if err := ensureStudioAssets(tx, id, valid.Assets, false); err != nil {
		return StudioDocument{}, 0, err
	}
	result, err := tx.Exec(`
		UPDATE studio_documents SET
			template_key=?, title=?, summary=?, language=?, author_label=?,
			tags_json=?, classification_json=?, metadata_json=?, content_json=?,
			plain_text=?, revision=?, updated=?
		WHERE id=? AND revision=?`,
		current.TemplateKey, current.Title, current.Summary, current.Language,
		current.AuthorLabel, string(tagsJSON), string(classificationJSON),
		string(current.Metadata), string(current.Content), valid.PlainText,
		nextRevision, now, id, valid.Input.BaseRevision)
	if err != nil {
		return StudioDocument{}, 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return StudioDocument{}, 0, err
	}
	if changed != 1 {
		var revision int
		_ = tx.QueryRow(`SELECT revision FROM studio_documents WHERE id=?`, id).Scan(&revision)
		return StudioDocument{}, revision, errStudioConflict
	}
	if _, err = tx.Exec(`
		INSERT INTO studio_revisions
			(document_id, revision, editor_user_id, editor_label, snapshot_json, created)
		VALUES (?,?,?,?,?,?)`,
		id, nextRevision, editor.ID, editor.Username, string(snapshot), now); err != nil {
		return StudioDocument{}, 0, err
	}
	if err := replaceStudioDerived(tx, id, valid); err != nil {
		return StudioDocument{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, 0, err
	}
	return current, nextRevision, nil
}

func (s *Store) archiveStudioDocument(id string, editor *User) (StudioDocument, error) {
	current, err := s.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	if current.Status == "archived" {
		return current, nil
	}
	now := time.Now().Unix()
	current.Status = "archived"
	current.Revision++
	current.Updated = now
	snapshot, err := json.Marshal(current)
	if err != nil {
		return StudioDocument{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StudioDocument{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		UPDATE studio_documents SET status='archived', revision=?, updated=?
		WHERE id=? AND revision=?`, current.Revision, now, id, current.Revision-1)
	if err != nil {
		return StudioDocument{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return StudioDocument{}, errStudioConflict
	}
	if _, err = tx.Exec(`
		INSERT INTO studio_revisions
			(document_id, revision, editor_user_id, editor_label, snapshot_json, created)
		VALUES (?,?,?,?,?,?)`,
		id, current.Revision, editor.ID, editor.Username, string(snapshot), now); err != nil {
		return StudioDocument{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, err
	}
	return current, nil
}

type StudioRevision struct {
	Revision    int    `json:"revision"`
	EditorLabel string `json:"editorLabel,omitempty"`
	Created     int64  `json:"created"`
}

func (s *Store) listStudioRevisions(id string, viewer *User) ([]StudioRevision, error) {
	if _, err := s.getStudioDocument(id, viewer); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT revision, editor_label, created
		FROM studio_revisions WHERE document_id=? ORDER BY revision DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudioRevision{}
	for rows.Next() {
		var revision StudioRevision
		if err := rows.Scan(&revision.Revision, &revision.EditorLabel, &revision.Created); err != nil {
			return nil, err
		}
		out = append(out, revision)
	}
	return out, rows.Err()
}
