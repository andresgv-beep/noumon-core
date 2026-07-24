package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type StudioCapabilities struct {
	Available        bool  `json:"available"`
	CanAuthor        bool  `json:"canAuthor"`
	CanPublish       bool  `json:"canPublish"`
	QuotaBytes       int64 `json:"quotaBytes"`
	SchemaVersion    int   `json:"schemaVersion"`
	MaxDocumentBytes int   `json:"maxDocumentBytes"`
}

type studioTemplateDescriptor struct {
	Key            string `json:"key"`
	Surface        string `json:"surface"`
	LabelKey       string `json:"labelKey"`
	DescriptionKey string `json:"descriptionKey"`
}

func (s *Server) studioCapabilities(user *User) StudioCapabilities {
	caps := StudioCapabilities{
		Available: true, SchemaVersion: studioSchemaVersion,
		MaxDocumentBytes: studioMaxRequest, QuotaBytes: 2 << 30,
	}
	if user == nil {
		return caps
	}
	if user.IsAdmin {
		caps.CanAuthor = true
		caps.CanPublish = true
		return caps
	}
	var author, publish int
	var quota int64
	err := s.store.db.QueryRow(`
		SELECT can_author, can_publish, quota_bytes
		FROM user_capabilities WHERE user_id=?`, user.ID).
		Scan(&author, &publish, &quota)
	if err == nil {
		caps.CanAuthor = author == 1
		caps.CanPublish = publish == 1
		if quota > 0 {
			caps.QuotaBytes = quota
		}
	}
	return caps
}

func (s *Server) requireStudioAuthor(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user := s.currentUser(r)
	if user == nil {
		writeStudioError(w, http.StatusUnauthorized, "studio.auth_required", nil)
		return nil, false
	}
	if !s.studioCapabilities(user).CanAuthor {
		writeStudioError(w, http.StatusForbidden, "studio.author_required", nil)
		return nil, false
	}
	return user, true
}

func (s *Server) registerStudioRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/studio/capabilities", s.handleStudioCapabilities)
	mux.HandleFunc("/api/studio/templates", s.handleStudioTemplates)
	mux.HandleFunc("/api/studio/publish-targets", s.handleStudioPublishTargets)
	mux.HandleFunc("/api/studio/documents", s.handleStudioDocuments)
	mux.HandleFunc("/api/studio/documents/", s.handleStudioDocumentSub)
	s.registerPublishedDocumentRoutes(mux)
}

func (s *Server) registerStudioAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/studio/capabilities/", s.handleAdminStudioCapabilities)
}

func (s *Server) handleStudioCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.studioCapabilities(s.currentUser(r)))
}

func (s *Server) handleStudioTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	if _, ok := s.requireStudioAuthor(w, r); !ok {
		return
	}
	templates := []studioTemplateDescriptor{
		{Key: "document", Surface: "documents", LabelKey: "studio.template.document", DescriptionKey: "studio.template.documentDesc"},
		{Key: "technical", Surface: "documents", LabelKey: "studio.template.technical", DescriptionKey: "studio.template.technicalDesc"},
		{Key: "story", Surface: "documents", LabelKey: "studio.template.story", DescriptionKey: "studio.template.storyDesc"},
		{Key: "cabinet.pdf", Surface: "cabinet", LabelKey: "studio.template.cabinetPdf", DescriptionKey: "studio.template.cabinetPdfDesc"},
		{Key: "cabinet.reader", Surface: "cabinet", LabelKey: "studio.template.cabinetReader", DescriptionKey: "studio.template.cabinetReaderDesc"},
		{Key: "cabinet.gallery", Surface: "cabinet", LabelKey: "studio.template.cabinetGallery", DescriptionKey: "studio.template.cabinetGalleryDesc"},
		{Key: "cabinet.audio", Surface: "cabinet", LabelKey: "studio.template.cabinetAudio", DescriptionKey: "studio.template.cabinetAudioDesc"},
		{Key: "cabinet.video", Surface: "cabinet", LabelKey: "studio.template.cabinetVideo", DescriptionKey: "studio.template.cabinetVideoDesc"},
		{Key: "moments.video", Surface: "moments", LabelKey: "studio.template.momentsVideo", DescriptionKey: "studio.template.momentsVideoDesc"},
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func (s *Server) handleStudioPublishTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	user, ok := s.requireStudioAuthor(w, r)
	if !ok {
		return
	}
	caps := s.studioCapabilities(user)
	if !caps.CanPublish {
		writeJSON(w, http.StatusOK, map[string]any{"targets": []any{}})
		return
	}
	rows, err := s.store.db.Query(`
		SELECT collection_id FROM studio_publish_targets
		WHERE user_id=? ORDER BY collection_id`, user.ID)
	if err != nil {
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	defer rows.Close()
	targets := []string{}
	for rows.Next() {
		var target string
		if rows.Scan(&target) == nil {
			targets = append(targets, target)
		}
	}
	if err := rows.Err(); err != nil {
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (s *Server) handleStudioDocuments(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireStudioAuthor(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := queryInt(r, "limit", 30)
		offset := queryInt(r, "offset", 0)
		documents, err := s.store.listStudioDocuments(user, r.URL.Query().Get("status"), limit, offset)
		if err != nil {
			writeStudioError(w, http.StatusBadRequest, "studio.list_invalid", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"documents": documents, "limit": minInt(maxInt(limit, 1), 100), "offset": maxInt(offset, 0),
		})
	case http.MethodPost:
		var input StudioDocumentInput
		if err := decodeStudioJSON(w, r, &input); err != nil {
			writeStudioError(w, http.StatusBadRequest, "studio.body_invalid", nil)
			return
		}
		valid, err := validateStudioInput(input)
		if err != nil {
			writeStudioError(w, http.StatusUnprocessableEntity, "studio.document_invalid",
				map[string]any{"reason": err.Error()})
			return
		}
		if len(valid.Assets) != 0 {
			writeStudioError(w, http.StatusUnprocessableEntity, "studio.asset_invalid", nil)
			return
		}
		document, err := s.store.createStudioDocument(user, valid)
		if err != nil {
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"document": document})
	default:
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
	}
}

func (s *Server) handleStudioDocumentSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/studio/documents/"), "/")
	if rest == "" {
		writeStudioError(w, http.StatusNotFound, "studio.document_not_found", nil)
		return
	}
	id, sub, _ := strings.Cut(rest, "/")
	if !studioIDRE.MatchString(id) {
		writeStudioError(w, http.StatusBadRequest, "studio.document_id_invalid", nil)
		return
	}
	if sub == "assets" && r.Method == http.MethodPost {
		if token := strings.TrimSpace(r.URL.Query().Get("ut")); token != "" {
			user, ok := s.consumeStudioUploadGrant(token, id)
			if !ok {
				writeStudioError(w, http.StatusUnauthorized, "studio.upload_token_invalid", nil)
				return
			}
			s.handleStudioAssetUpload(w, r, id, user)
			return
		}
	}
	if strings.HasPrefix(sub, "assets/") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		s.handleStudioAsset(w, r, id, strings.TrimPrefix(sub, "assets/"), s.currentUser(r))
		return
	}
	user, ok := s.requireStudioAuthor(w, r)
	if !ok {
		return
	}
	if sub == "revisions" {
		if r.Method != http.MethodGet {
			writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
			return
		}
		revisions, err := s.store.listStudioRevisions(id, user)
		if err != nil {
			writeStudioStoreError(w, err, 0)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
		return
	}
	if strings.HasPrefix(sub, "restore/") {
		if r.Method != http.MethodPost {
			writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
			return
		}
		revisionText := strings.TrimPrefix(sub, "restore/")
		targetRevision, err := strconv.Atoi(revisionText)
		if err != nil || targetRevision < 1 || strings.Contains(revisionText, "/") {
			writeStudioError(w, http.StatusBadRequest, "studio.revision_invalid", nil)
			return
		}
		var body struct {
			BaseRevision int `json:"baseRevision"`
		}
		if err := decodeStudioJSON(w, r, &body); err != nil {
			writeStudioError(w, http.StatusBadRequest, "studio.body_invalid", nil)
			return
		}
		document, currentRevision, err := s.store.restoreStudioRevision(
			id, targetRevision, body.BaseRevision, user)
		if err != nil {
			writeStudioStoreError(w, err, currentRevision)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"document": document})
		return
	}
	if sub == "upload-token" {
		s.handleStudioUploadToken(w, r, id, user)
		return
	}
	if sub == "assets" {
		s.handleStudioAssetUpload(w, r, id, user)
		return
	}
	if strings.HasPrefix(sub, "assets/") {
		s.handleStudioAsset(w, r, id, strings.TrimPrefix(sub, "assets/"), user)
		return
	}
	if sub == "publish" || sub == "unpublish" {
		if r.Method != http.MethodPost {
			writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
			return
		}
		if !s.studioCapabilities(user).CanPublish {
			writeStudioError(w, http.StatusForbidden, "studio.publish_required", nil)
			return
		}
		var document StudioDocument
		var err error
		if sub == "publish" {
			document, err = s.publishStudioContent(id, user)
		} else {
			document, err = s.unpublishStudioContent(id, user)
		}
		if err != nil {
			writeStudioStoreError(w, err, 0)
			return
		}
		s.invalidateAccessCache()
		writeJSON(w, http.StatusOK, map[string]any{"document": document})
		return
	}
	if sub != "" {
		writeStudioError(w, http.StatusNotFound, "studio.route_not_found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := s.store.getStudioDocument(id, user)
		if err != nil {
			writeStudioStoreError(w, err, 0)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"document": document})
	case http.MethodPut:
		var input StudioDocumentInput
		if err := decodeStudioJSON(w, r, &input); err != nil {
			writeStudioError(w, http.StatusBadRequest, "studio.body_invalid", nil)
			return
		}
		valid, err := validateStudioInput(input)
		if err != nil {
			writeStudioError(w, http.StatusUnprocessableEntity, "studio.document_invalid",
				map[string]any{"reason": err.Error()})
			return
		}
		document, currentRevision, err := s.store.updateStudioDocument(id, user, valid)
		if err != nil {
			writeStudioStoreError(w, err, currentRevision)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"document": document})
	case http.MethodDelete:
		document, err := s.archiveStudioContent(id, user)
		if err != nil {
			writeStudioStoreError(w, err, 0)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"document": document})
	default:
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
	}
}

func (s *Server) handleAdminStudioCapabilities(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(
		r.URL.Path, "/api/admin/studio/capabilities/"), "/"), 10, 64)
	if err != nil || userID < 1 {
		writeStudioError(w, http.StatusBadRequest, "studio.user_id_invalid", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var author, publish int
		var quota, updated int64
		err := s.store.db.QueryRow(`
			SELECT can_author, can_publish, quota_bytes, updated
			FROM user_capabilities WHERE user_id=?`, userID).
			Scan(&author, &publish, &quota, &updated)
		if err != nil {
			// A missing row means defaults, not an error.
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusOK, map[string]any{
					"userId": userID, "canAuthor": false, "canPublish": false,
					"quotaBytes": int64(2 << 30), "updated": int64(0),
				})
				return
			}
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"userId": userID, "canAuthor": author == 1, "canPublish": publish == 1,
			"quotaBytes": quota, "updated": updated,
		})
	case http.MethodPut:
		var body struct {
			CanAuthor  bool  `json:"canAuthor"`
			CanPublish bool  `json:"canPublish"`
			QuotaBytes int64 `json:"quotaBytes"`
		}
		if err := decodeStudioJSON(w, r, &body); err != nil {
			writeStudioError(w, http.StatusBadRequest, "studio.body_invalid", nil)
			return
		}
		if body.CanPublish {
			body.CanAuthor = true
		}
		if body.QuotaBytes <= 0 {
			body.QuotaBytes = 2 << 30
		}
		var exists int
		if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, userID).Scan(&exists); err != nil || exists != 1 {
			writeStudioError(w, http.StatusNotFound, "studio.user_not_found", nil)
			return
		}
		now := time.Now().Unix()
		_, err := s.store.db.Exec(`
			INSERT INTO user_capabilities (user_id, can_author, can_publish, quota_bytes, updated)
			VALUES (?,?,?,?,?)
			ON CONFLICT(user_id) DO UPDATE SET
				can_author=excluded.can_author,
				can_publish=excluded.can_publish,
				quota_bytes=excluded.quota_bytes,
				updated=excluded.updated`,
			userID, boolInt(body.CanAuthor), boolInt(body.CanPublish), body.QuotaBytes, now)
		if err != nil {
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"userId": userID, "canAuthor": body.CanAuthor, "canPublish": body.CanPublish,
			"quotaBytes": body.QuotaBytes, "updated": now,
		})
	default:
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
	}
}

func decodeStudioJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, studioMaxRequest)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func writeStudioStoreError(w http.ResponseWriter, err error, currentRevision int) {
	switch {
	case errors.Is(err, errStudioNotFound):
		writeStudioError(w, http.StatusNotFound, "studio.document_not_found", nil)
	case errors.Is(err, errStudioForbidden):
		// Do not reveal whether another user's private draft exists.
		writeStudioError(w, http.StatusNotFound, "studio.document_not_found", nil)
	case errors.Is(err, errStudioConflict):
		var details map[string]any
		if currentRevision > 0 {
			details = map[string]any{"currentRevision": currentRevision}
		}
		writeStudioError(w, http.StatusConflict, "studio.revision_conflict", details)
	case errors.Is(err, errStudioRevisionNotFound):
		writeStudioError(w, http.StatusNotFound, "studio.revision_not_found", nil)
	case errors.Is(err, errStudioAssetInvalid):
		writeStudioError(w, http.StatusUnprocessableEntity, "studio.asset_invalid", nil)
	case errors.Is(err, errStudioMediaIncomplete):
		writeStudioError(w, http.StatusUnprocessableEntity, "studio.media_incomplete", nil)
	default:
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
	}
}

func writeStudioError(w http.ResponseWriter, status int, code string, details map[string]any) {
	body := map[string]any{"errorCode": code}
	if details != nil {
		body["details"] = details
	}
	writeJSON(w, status, body)
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
