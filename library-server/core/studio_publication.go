package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const studioDocumentsCollectionID = "col:studio:documents"

func (s *Store) publishStudioDocument(id string, editor *User) (StudioDocument, error) {
	current, err := s.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	surface, ok := studioSurfaceForTemplate(current.TemplateKey)
	if !ok || surface != "documents" {
		return StudioDocument{}, errors.New("studio publication surface unsupported")
	}
	var input StudioDocumentInput
	input.TemplateKey = current.TemplateKey
	input.Title = current.Title
	input.Summary = current.Summary
	input.Language = current.Language
	input.AuthorLabel = current.AuthorLabel
	input.Tags = current.Tags
	input.Metadata = current.Metadata
	input.Content = current.Content
	valid, err := validateStudioInput(input)
	if err != nil {
		return StudioDocument{}, err
	}

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return StudioDocument{}, err
	}
	defer tx.Rollback()
	if err := ensureStudioAssets(tx, id, valid.Assets, true); err != nil {
		return StudioDocument{}, err
	}
	result, err := tx.Exec(`
		UPDATE studio_documents SET
			status='published', published_revision=revision,
			publication_kind='documents', publication_target=?,
			published_plain_text=?, published=?, updated=?
		WHERE id=? AND revision=?`,
		studioDocumentsCollectionID, valid.PlainText, now, now, id, current.Revision)
	if err != nil {
		return StudioDocument{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StudioDocument{}, errStudioConflict
	}
	if err := replaceStudioPublishedLinks(tx, id, valid.Links); err != nil {
		return StudioDocument{}, err
	}
	if _, err = tx.Exec(`
		INSERT INTO content_origins
			(document_id, origin_content_id, origin_creator_key, origin_version, imported)
		VALUES (?,?,?,?,?)
		ON CONFLICT(document_id) DO UPDATE SET
			origin_version=excluded.origin_version`,
		id, id, "local:"+editor.Username, studioRevisionString(current.Revision), now); err != nil {
		return StudioDocument{}, err
	}
	// Una colección nueva nace visible solo para cuentas locales. El administrador
	// puede hacerla abierta, limitarla por edad o bloquearla desde el mismo gate
	// que Cabinet/Moments; nunca queda expuesta por accidente a anónimos.
	if _, err = tx.Exec(`
		INSERT OR IGNORE INTO collection_access
			(collection_id, access, min_age, allow_download, updated)
		VALUES (?, 'login', 0, 0, ?)`, studioDocumentsCollectionID, now); err != nil {
		return StudioDocument{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, err
	}
	current.Status = "published"
	current.PublishedRevision = &current.Revision
	current.PublicationKind = "documents"
	current.PublicationTarget = studioDocumentsCollectionID
	current.Published = &now
	current.Updated = now
	return current, nil
}

func studioRevisionString(revision int) string {
	return "revision-" + strconv.Itoa(revision)
}

func (s *Store) unpublishStudioDocument(id string, editor *User) (StudioDocument, error) {
	current, err := s.getStudioDocument(id, editor)
	if err != nil {
		return StudioDocument{}, err
	}
	if current.PublishedRevision == nil {
		return current, nil
	}
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return StudioDocument{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		UPDATE studio_documents SET
			status='draft', published_revision=NULL, publication_kind=NULL,
			publication_target=NULL, published_plain_text='', published=NULL, updated=?
		WHERE id=? AND revision=?`, now, id, current.Revision)
	if err != nil {
		return StudioDocument{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StudioDocument{}, errStudioConflict
	}
	if err := replaceStudioPublishedLinks(tx, id, nil); err != nil {
		return StudioDocument{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudioDocument{}, err
	}
	current.Status = "draft"
	current.PublishedRevision = nil
	current.PublicationKind = ""
	current.PublicationTarget = ""
	current.Published = nil
	current.Updated = now
	return current, nil
}

func (s *Store) publishedStudioDocument(id string) (StudioDocument, error) {
	var revision int
	var published sql.NullInt64
	err := s.db.QueryRow(`
		SELECT published_revision, published
		FROM studio_documents
		WHERE id=? AND published_revision IS NOT NULL AND status!='archived'`, id).
		Scan(&revision, &published)
	if err == sql.ErrNoRows {
		return StudioDocument{}, errStudioNotFound
	}
	if err != nil {
		return StudioDocument{}, err
	}
	var snapshot string
	if err := s.db.QueryRow(`
		SELECT snapshot_json FROM studio_revisions
		WHERE document_id=? AND revision=?`, id, revision).Scan(&snapshot); err != nil {
		return StudioDocument{}, err
	}
	var doc StudioDocument
	if err := json.Unmarshal([]byte(snapshot), &doc); err != nil {
		return StudioDocument{}, err
	}
	doc.Status = "published"
	doc.PublishedRevision = &revision
	doc.PublicationKind = "documents"
	doc.PublicationTarget = studioDocumentsCollectionID
	if published.Valid {
		v := published.Int64
		doc.Published = &v
	}
	// El propietario es estado administrativo, no parte del documento público.
	// Las revisiones inmutables pueden conservar el ID de una cuenta eliminada
	// como procedencia interna, pero nunca debe salir por esta API.
	doc.OwnerUserID = nil
	return doc, nil
}

func (s *Store) listPublishedStudioDocuments() ([]StudioDocumentSummary, error) {
	rows, err := s.db.Query(`
		SELECT id FROM studio_documents
		WHERE published_revision IS NOT NULL AND status!='archived'
		ORDER BY published DESC, title`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
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
	out := make([]StudioDocumentSummary, 0, len(ids))
	for _, id := range ids {
		doc, err := s.publishedStudioDocument(id)
		if err != nil {
			return nil, err
		}
		out = append(out, studioSummary(doc))
	}
	return out, nil
}

type StudioDocumentRelations struct {
	OutgoingItemIDs []string                `json:"outgoingItemIds"`
	Backlinks       []StudioDocumentSummary `json:"backlinks"`
}

func (s *Store) publishedStudioRelations(id string) (StudioDocumentRelations, error) {
	relations := StudioDocumentRelations{
		OutgoingItemIDs: []string{},
		Backlinks:       []StudioDocumentSummary{},
	}
	rows, err := s.db.Query(`
		SELECT target_item_id FROM studio_published_links
		WHERE source_document_id=? ORDER BY target_item_id`, id)
	if err != nil {
		return relations, err
	}
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			rows.Close()
			return relations, err
		}
		relations.OutgoingItemIDs = append(relations.OutgoingItemIDs, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return relations, err
	}
	if err := rows.Close(); err != nil {
		return relations, err
	}

	rows, err = s.db.Query(`
		SELECT p.source_document_id
		FROM studio_published_links p
		JOIN studio_documents d ON d.id=p.source_document_id
		WHERE p.target_item_id=?
		  AND d.published_revision IS NOT NULL AND d.status!='archived'
		ORDER BY d.published DESC, d.id`, "studio:"+id)
	if err != nil {
		return relations, err
	}
	var sourceIDs []string
	for rows.Next() {
		var sourceID string
		if err := rows.Scan(&sourceID); err != nil {
			rows.Close()
			return relations, err
		}
		sourceIDs = append(sourceIDs, sourceID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return relations, err
	}
	if err := rows.Close(); err != nil {
		return relations, err
	}
	for _, sourceID := range sourceIDs {
		document, err := s.publishedStudioDocument(sourceID)
		if err != nil {
			return relations, err
		}
		relations.Backlinks = append(relations.Backlinks, studioSummary(document))
	}
	return relations, nil
}

func (s *Server) registerPublishedDocumentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/documents", s.handlePublishedDocuments)
	mux.HandleFunc("/api/documents/", s.handlePublishedDocument)
}

func (s *Server) handlePublishedDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	if !s.canSeeCollectionID(s.currentUser(r), studioDocumentsCollectionID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sin acceso a esta coleccion"})
		return
	}
	documents, err := s.store.listPublishedStudioDocuments()
	if err != nil {
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": documents})
}

func (s *Server) handlePublishedDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	if !s.canSeeCollectionID(s.currentUser(r), studioDocumentsCollectionID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sin acceso a esta coleccion"})
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/documents/"), "/")
	id, action, hasAction := strings.Cut(rest, "/")
	if !studioIDRE.MatchString(id) {
		writeStudioError(w, http.StatusNotFound, "studio.document_not_found", nil)
		return
	}
	doc, err := s.store.publishedStudioDocument(id)
	if err != nil {
		writeStudioStoreError(w, err, 0)
		return
	}
	if hasAction {
		if action != "relations" {
			writeStudioError(w, http.StatusNotFound, "studio.route_not_found", nil)
			return
		}
		relations, err := s.store.publishedStudioRelations(id)
		if err != nil {
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
			return
		}
		writeJSON(w, http.StatusOK, relations)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document": doc, "snapshot": studioPortableSnapshot(doc),
	})
}

func studioDocumentToItem(doc StudioDocument) Item {
	return Item{
		ID:           "studio:" + doc.ID,
		CollectionID: studioDocumentsCollectionID,
		Source: ItemSourceInfo{
			Provider: "studio", ProviderItemID: doc.ID,
		},
		Kind:        "document",
		Title:       doc.Title,
		Description: doc.Summary,
		Language:    doc.Language,
		Authors:     compactStudioAuthor(doc.AuthorLabel),
		Tags:        append([]string(nil), doc.Tags...),
		Preview:     Preview{Kind: "text", Text: doc.Summary, Icon: "note"},
		Open: &OpenTarget{
			Mode: "document", ItemID: "studio:" + doc.ID,
			Title: doc.Title, Provider: "studio",
		},
		Capabilities: ItemCapabilities{
			Open: true, Search: true, Preview: true,
			Favorite: true, Note: true, Tag: true,
		},
	}
}

func compactStudioAuthor(author string) []string {
	if strings.TrimSpace(author) == "" {
		return nil
	}
	return []string{strings.TrimSpace(author)}
}

func (s *Store) searchPublishedStudioDocuments(query string) ([]FederatedSearchResult, error) {
	rows, err := s.db.Query(`
		SELECT d.id, r.snapshot_json, d.published_plain_text
		FROM studio_documents d
		JOIN studio_revisions r
		  ON r.document_id=d.id AND r.revision=d.published_revision
		WHERE d.published_revision IS NOT NULL AND d.status!='archived'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nq := normalizeText(query)
	tokens := queryTokens(query)
	out := []FederatedSearchResult{}
	for rows.Next() {
		var id, snapshot, plain string
		if err := rows.Scan(&id, &snapshot, &plain); err != nil {
			return nil, err
		}
		var published StudioDocument
		if err := json.Unmarshal([]byte(snapshot), &published); err != nil {
			return nil, err
		}
		haystack := strings.Join([]string{
			published.Title, published.Summary, published.AuthorLabel,
			strings.Join(published.Tags, " "), plain,
		}, " ")
		normalized := normalizeText(haystack)
		if !strings.Contains(normalized, nq) && !coversAllTokens(tokens, haystack) {
			continue
		}
		score := scoreHit(query, published.Title, published.AuthorLabel, published.Summary)
		if strings.Contains(normalizeText(published.Title), nq) {
			score += 300
		}
		out = append(out, FederatedSearchResult{
			ItemID: "studio:" + id, CollectionID: studioDocumentsCollectionID,
			Title: published.Title, Subtitle: published.AuthorLabel,
			Snippet: published.Summary, Kind: "document",
			Score:   score + 140,
			Preview: Preview{Kind: "text", Text: published.Summary, Icon: "note"},
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, rows.Err()
}
