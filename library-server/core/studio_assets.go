package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	studioMaxImageBytes     = int64(12 << 20)
	studioMaxTextAssetBytes = int64(5 << 20)
	studioMaxMediaBytes     = int64(2 << 30)
	studioMaxDocumentAssets = int64(2 << 30)
	studioUploadTokenTTL    = 5 * time.Minute
)

type studioUploadGrant struct {
	UserID     int64
	DocumentID string
	Expires    time.Time
}

type StudioAsset struct {
	ID         string `json:"id"`
	DocumentID string `json:"documentId"`
	Filename   string `json:"filename"`
	MIMEType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
	State      string `json:"state"`
	Created    int64  `json:"created"`
}

func (s *Server) handleStudioUploadToken(w http.ResponseWriter, r *http.Request, documentID string, user *User) {
	if r.Method != http.MethodPost {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	document, err := s.store.getStudioDocument(documentID, user)
	if err != nil {
		writeStudioStoreError(w, err, 0)
		return
	}
	if document.Status == "archived" {
		writeStudioError(w, http.StatusConflict, "studio.asset_invalid", nil)
		return
	}
	token, err := newStudioUploadToken()
	if err != nil {
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	expires := time.Now().Add(studioUploadTokenTTL)
	s.studioUploadMu.Lock()
	if s.studioUploads == nil {
		s.studioUploads = make(map[string]studioUploadGrant)
	}
	for key, grant := range s.studioUploads {
		if time.Now().After(grant.Expires) {
			delete(s.studioUploads, key)
		}
	}
	s.studioUploads[token] = studioUploadGrant{
		UserID: user.ID, DocumentID: documentID, Expires: expires,
	}
	s.studioUploadMu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token, "expires": expires.Unix(),
	})
}

func newStudioUploadToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Server) consumeStudioUploadGrant(token, documentID string) (*User, bool) {
	if token == "" || len(token) > 128 {
		return nil, false
	}
	s.studioUploadMu.Lock()
	var matchedKey string
	var grant studioUploadGrant
	for key, candidate := range s.studioUploads {
		if subtle.ConstantTimeCompare([]byte(key), []byte(token)) == 1 {
			matchedKey = key
			grant = candidate
			break
		}
	}
	if matchedKey != "" {
		delete(s.studioUploads, matchedKey)
	}
	s.studioUploadMu.Unlock()
	if matchedKey == "" || time.Now().After(grant.Expires) || grant.DocumentID != documentID {
		return nil, false
	}
	var user User
	var admin int
	if err := s.store.db.QueryRow(`
		SELECT id, username, age, is_admin FROM users WHERE id=?`, grant.UserID).
		Scan(&user.ID, &user.Username, &user.Age, &admin); err != nil {
		return nil, false
	}
	user.IsAdmin = admin == 1
	if !s.studioCapabilities(&user).CanAuthor {
		return nil, false
	}
	document, err := s.store.getStudioDocument(documentID, &user)
	if err != nil || document.Status == "archived" {
		return nil, false
	}
	return &user, true
}

func (s *Server) handleStudioAssetUpload(w http.ResponseWriter, r *http.Request, documentID string, user *User) {
	if r.Method != http.MethodPost {
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
		return
	}
	document, err := s.store.getStudioDocument(documentID, user)
	if err != nil {
		writeStudioStoreError(w, err, 0)
		return
	}
	if document.Status == "archived" {
		writeStudioError(w, http.StatusConflict, "studio.asset_invalid", nil)
		return
	}
	purpose := strings.TrimSpace(r.URL.Query().Get("purpose"))
	if purpose == "" {
		purpose = "image"
	}
	if !studioAssetPurposeAllowed(document.TemplateKey, purpose) {
		writeStudioError(w, http.StatusUnsupportedMediaType, "studio.asset_type_invalid", nil)
		return
	}
	if s.studioRoot == "" {
		writeStudioError(w, http.StatusServiceUnavailable, "studio.assets_unavailable", nil)
		return
	}
	s.cleanupStudioStaging(documentID)
	// Rechazo barato antes de abrir el multipart: evita escribir hasta 12 MB
	// transitorios cuando el documento o el autor ya no tienen ni un byte libre.
	// La comprobación exacta se repite después con el tamaño real y, finalmente,
	// dentro de la transacción del INSERT para cerrar carreras concurrentes.
	if err := s.checkStudioAssetQuota(documentID, user, 1); err != nil {
		if errors.Is(err, errStudioAssetQuota) {
			writeStudioError(w, http.StatusInsufficientStorage, "studio.asset_quota", nil)
		} else {
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		}
		return
	}
	if ok, err := studioHasDiskHeadroom(s.studioRoot); err != nil || !ok {
		writeStudioError(w, http.StatusInsufficientStorage, "studio.storage_low", nil)
		return
	}
	maxBytes := studioMaxBytesForPurpose(purpose)
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		writeStudioError(w, http.StatusBadRequest, "studio.asset_invalid", nil)
		return
	}
	part, filename, err := studioMultipartFile(reader)
	if err != nil {
		writeStudioError(w, http.StatusBadRequest, "studio.asset_invalid", nil)
		return
	}
	defer part.Close()

	asset, stagingPath, finalPath, err := s.persistStudioAssetForPurpose(
		documentID, user, filename, purpose, maxBytes, part)
	if err != nil {
		switch {
		case errors.Is(err, errStudioAssetTooLarge):
			writeStudioError(w, http.StatusRequestEntityTooLarge, "studio.asset_too_large", nil)
		case errors.Is(err, errStudioAssetType):
			writeStudioError(w, http.StatusUnsupportedMediaType, "studio.asset_type_invalid", nil)
		case errors.Is(err, errStudioAssetQuota):
			writeStudioError(w, http.StatusInsufficientStorage, "studio.asset_quota", nil)
		default:
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		}
		return
	}
	if !studioAssetRoleMIMEAllowed(document.TemplateKey, purpose, asset.MIMEType) {
		_ = os.Remove(stagingPath)
		writeStudioError(w, http.StatusUnsupportedMediaType, "studio.asset_type_invalid", nil)
		return
	}
	if err := s.insertStudioAsset(asset, user.ID, s.studioCapabilities(user).QuotaBytes); err != nil {
		_ = os.Remove(stagingPath)
		if errors.Is(err, errStudioAssetQuota) {
			writeStudioError(w, http.StatusInsufficientStorage, "studio.asset_quota", nil)
		} else {
			writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		}
		return
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		s.discardStudioAsset(asset.ID, stagingPath, finalPath)
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	result, err := s.store.db.Exec(`
		UPDATE studio_assets SET state='staged'
		WHERE id=? AND document_id=? AND state='staging'`, asset.ID, documentID)
	if err != nil {
		s.discardStudioAsset(asset.ID, stagingPath, finalPath)
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		s.discardStudioAsset(asset.ID, stagingPath, finalPath)
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	asset.State = "staged"
	writeJSON(w, http.StatusCreated, map[string]any{"asset": asset})
}

var (
	errStudioAssetTooLarge = errors.New("studio asset too large")
	errStudioAssetType     = errors.New("studio asset type")
	errStudioAssetQuota    = errors.New("studio asset quota")
)

func studioMultipartFile(reader *multipart.Reader) (*multipart.Part, string, error) {
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil, "", errors.New("file part missing")
		}
		if err != nil {
			return nil, "", err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			name := filepath.Base(strings.TrimSpace(part.FileName()))
			if name == "." || name == "" || len(name) > 255 {
				part.Close()
				return nil, "", errors.New("invalid filename")
			}
			for _, char := range name {
				if unicode.IsControl(char) {
					part.Close()
					return nil, "", errors.New("invalid filename")
				}
			}
			return part, name, nil
		}
		part.Close()
	}
}

func studioAllowedImageExtension(extension string) bool {
	switch extension {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func studioAssetPurposeAllowed(template, purpose string) bool {
	switch purpose {
	case "image":
		return template == "document" || template == "technical" || template == "story"
	case "cover":
		return strings.HasPrefix(template, "cabinet.") || template == "moments.video"
	case "avatar":
		return template == "moments.video"
	case "primary":
		return strings.HasPrefix(template, "cabinet.") || template == "moments.video"
	case "track", "waveform":
		return template == "cabinet.audio"
	case "subtitle":
		return template == "moments.video"
	default:
		return false
	}
}

func studioMaxBytesForPurpose(purpose string) int64 {
	switch purpose {
	case "image", "cover", "avatar", "waveform":
		return studioMaxImageBytes
	case "subtitle":
		return studioMaxTextAssetBytes
	default:
		return studioMaxMediaBytes
	}
}

func studioAssetRoleMIMEAllowed(template, purpose, mimeType string) bool {
	switch purpose {
	case "image", "cover", "avatar", "waveform":
		return strings.HasPrefix(mimeType, "image/")
	case "subtitle":
		return mimeType == "text/vtt"
	case "track":
		return template == "cabinet.audio" && strings.HasPrefix(mimeType, "audio/")
	case "primary":
		switch template {
		case "moments.video", "cabinet.video":
			return strings.HasPrefix(mimeType, "video/")
		case "cabinet.audio":
			return strings.HasPrefix(mimeType, "audio/")
		case "cabinet.pdf":
			return mimeType == "application/pdf"
		case "cabinet.reader":
			return mimeType == "application/epub+zip" ||
				mimeType == "text/plain" || mimeType == "text/markdown"
		case "cabinet.gallery":
			return strings.HasPrefix(mimeType, "image/")
		}
	}
	return false
}

func studioImageFormat(header []byte) (mimeType, extension string, err error) {
	mimeType = http.DetectContentType(header)
	switch mimeType {
	case "image/jpeg":
		return mimeType, ".jpg", nil
	case "image/png":
		return mimeType, ".png", nil
	case "image/gif":
		return mimeType, ".gif", nil
	case "image/webp":
		return mimeType, ".webp", nil
	default:
		return "", "", errStudioAssetType
	}
}

func (s *Server) persistStudioAsset(documentID string, user *User, originalName string, src io.Reader) (StudioAsset, string, string, error) {
	return s.persistStudioAssetForPurpose(
		documentID, user, originalName, "image", studioMaxImageBytes, src)
}

func (s *Server) persistStudioAssetForPurpose(
	documentID string,
	user *User,
	originalName string,
	purpose string,
	maxBytes int64,
	src io.Reader,
) (StudioAsset, string, string, error) {
	var asset StudioAsset
	dir, err := secureStudioAssetDir(s.studioRoot, documentID, true)
	if err != nil {
		return asset, "", "", err
	}
	id, err := newStudioID()
	if err != nil {
		return asset, "", "", err
	}
	tmpPath := filepath.Join(dir, "."+id+".part")
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return asset, "", "", err
	}
	cleanup := true
	defer func() {
		out.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	header := make([]byte, 512)
	n, readErr := io.ReadFull(src, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return asset, "", "", readErr
	}
	mimeType, extension, err := studioAssetFormat(
		purpose, strings.ToLower(filepath.Ext(originalName)), header[:n])
	if err != nil {
		return asset, "", "", err
	}
	total, err := io.Copy(io.MultiWriter(out, hasher),
		io.LimitReader(io.MultiReader(bytes.NewReader(header[:n]), src), maxBytes+1))
	if err != nil {
		return asset, "", "", err
	}
	if total > maxBytes {
		return asset, "", "", errStudioAssetTooLarge
	}
	if err := out.Sync(); err != nil {
		return asset, "", "", err
	}
	if err := out.Close(); err != nil {
		return asset, "", "", err
	}
	if studioAssetPurposeIsRaster(purpose) {
		if err := validateStoredStudioRaster(tmpPath, mimeType); err != nil {
			return asset, "", "", err
		}
	}
	if err := s.checkStudioAssetQuota(documentID, user, total); err != nil {
		return asset, "", "", err
	}
	cleanup = false
	now := time.Now().Unix()
	asset = StudioAsset{
		ID: id, DocumentID: documentID, Filename: originalName,
		MIMEType: mimeType, SizeBytes: total,
		SHA256: hex.EncodeToString(hasher.Sum(nil)), State: "staging", Created: now,
	}
	return asset, tmpPath, filepath.Join(dir, id+extension), nil
}

func studioAssetPurposeIsRaster(purpose string) bool {
	switch purpose {
	case "image", "cover", "avatar", "waveform":
		return true
	default:
		return false
	}
}

func studioAssetFormat(purpose, extension string, header []byte) (string, string, error) {
	if studioAssetPurposeIsRaster(purpose) {
		mimeType, normalizedExtension, err := studioImageFormat(header)
		if err != nil || !studioImageExtensionMatches(extension, mimeType) {
			return "", "", errStudioAssetType
		}
		return mimeType, normalizedExtension, nil
	}
	if purpose == "subtitle" {
		text := strings.TrimPrefix(string(header), "\ufeff")
		if extension != ".vtt" || !strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
			return "", "", errStudioAssetType
		}
		return "text/vtt", ".vtt", nil
	}
	mimeType, normalizedExtension, kind := studioMediaFormat(extension, header)
	if mimeType == "" || !studioPurposeAcceptsMediaKind(purpose, kind) {
		return "", "", errStudioAssetType
	}
	return mimeType, normalizedExtension, nil
}

func studioPurposeAcceptsMediaKind(purpose, kind string) bool {
	switch purpose {
	case "track":
		return kind == "audio"
	case "primary":
		return kind == "video" || kind == "audio" || kind == "pdf" ||
			kind == "reader" || kind == "image"
	default:
		return false
	}
}

func studioMediaFormat(extension string, header []byte) (mimeType, normalizedExtension, kind string) {
	hasPrefix := func(prefix string) bool {
		return len(header) >= len(prefix) && string(header[:len(prefix)]) == prefix
	}
	switch extension {
	case ".mp4", ".m4v":
		if len(header) >= 12 && string(header[4:8]) == "ftyp" {
			return "video/mp4", extension, "video"
		}
	case ".mov":
		if len(header) >= 12 && string(header[4:8]) == "ftyp" {
			return "video/quicktime", ".mov", "video"
		}
	case ".webm":
		if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
			return "video/webm", ".webm", "video"
		}
	case ".mp3":
		if hasPrefix("ID3") || (len(header) >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0) {
			return "audio/mpeg", ".mp3", "audio"
		}
	case ".ogg", ".oga":
		if hasPrefix("OggS") {
			return "audio/ogg", extension, "audio"
		}
	case ".flac":
		if hasPrefix("fLaC") {
			return "audio/flac", ".flac", "audio"
		}
	case ".wav":
		if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WAVE" {
			return "audio/wav", ".wav", "audio"
		}
	case ".m4a":
		if len(header) >= 12 && string(header[4:8]) == "ftyp" {
			return "audio/mp4", ".m4a", "audio"
		}
	case ".pdf":
		if hasPrefix("%PDF-") {
			return "application/pdf", ".pdf", "pdf"
		}
	case ".epub":
		if len(header) >= 4 && bytes.Equal(header[:4], []byte{'P', 'K', 3, 4}) {
			return "application/epub+zip", ".epub", "reader"
		}
	case ".txt":
		if strings.HasPrefix(http.DetectContentType(header), "text/plain") {
			return "text/plain", ".txt", "reader"
		}
	case ".md", ".markdown":
		if strings.HasPrefix(http.DetectContentType(header), "text/plain") {
			return "text/markdown", ".md", "reader"
		}
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		mimeType, normalized, err := studioImageFormat(header)
		if err == nil && studioImageExtensionMatches(extension, mimeType) {
			return mimeType, normalized, "image"
		}
	}
	return "", "", ""
}

func validateStoredStudioRaster(path, mimeType string) error {
	if mimeType == "image/webp" {
		// La stdlib no incluye decodificador WebP. En el MVP aceptamos el riesgo
		// acotado de un WebP corrupto (imagen rota, nunca contenido ejecutable):
		// DetectContentType + extensión coherente + nosniff + CSP siguen siendo
		// obligatorios. Si se incorpora un decoder local, validar aquí también.
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width < 1 || config.Height < 1 {
		return errStudioAssetType
	}
	return nil
}

func studioImageExtensionMatches(extension, mimeType string) bool {
	switch mimeType {
	case "image/jpeg":
		return extension == ".jpg" || extension == ".jpeg"
	case "image/png":
		return extension == ".png"
	case "image/gif":
		return extension == ".gif"
	case "image/webp":
		return extension == ".webp"
	default:
		return false
	}
}

func (s *Server) checkStudioAssetQuota(documentID string, user *User, incoming int64) error {
	var documentBytes, userBytes int64
	if err := s.store.db.QueryRow(`
		SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
		WHERE document_id=? AND state!='deleted'`, documentID).Scan(&documentBytes); err != nil {
		return err
	}
	if documentBytes+incoming > studioMaxDocumentAssets {
		return errStudioAssetQuota
	}
	if err := s.store.db.QueryRow(`
		SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
		WHERE owner_user_id=? AND state!='deleted'`, user.ID).Scan(&userBytes); err != nil {
		return err
	}
	if userBytes+incoming > s.studioCapabilities(user).QuotaBytes {
		return errStudioAssetQuota
	}
	return nil
}

func (s *Server) insertStudioAsset(asset StudioAsset, ownerUserID, quotaBytes int64) error {
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var documentBytes, userBytes int64
	if err := tx.QueryRow(`
		SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
		WHERE document_id=? AND state!='deleted'`, asset.DocumentID).Scan(&documentBytes); err != nil {
		return err
	}
	if documentBytes+asset.SizeBytes > studioMaxDocumentAssets {
		return errStudioAssetQuota
	}
	if err := tx.QueryRow(`
		SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
		WHERE owner_user_id=? AND state!='deleted'`, ownerUserID).Scan(&userBytes); err != nil {
		return err
	}
	if userBytes+asset.SizeBytes > quotaBytes {
		return errStudioAssetQuota
	}
	if _, err := tx.Exec(`
		INSERT INTO studio_assets
			(id, document_id, owner_user_id, filename, mime_type, size_bytes, sha256, state, created)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		asset.ID, asset.DocumentID, ownerUserID, asset.Filename, asset.MIMEType,
		asset.SizeBytes, asset.SHA256, asset.State, asset.Created); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) discardStudioAsset(assetID string, paths ...string) {
	for _, path := range paths {
		if path != "" {
			_ = os.Remove(path)
		}
	}
	_, _ = s.store.db.Exec(`DELETE FROM studio_assets WHERE id=? AND state='staging'`, assetID)
}

func (s *Server) cleanupStudioStaging(documentID string) {
	rows, err := s.store.db.Query(`
		SELECT id, mime_type, state FROM studio_assets
		WHERE document_id=? AND (
			(state='staging' AND created<?) OR state='deleted'
		)`,
		documentID, time.Now().Add(-10*time.Minute).Unix())
	if err != nil {
		return
	}
	type staleAsset struct {
		id, mimeType, state string
	}
	var stale []staleAsset
	for rows.Next() {
		var asset staleAsset
		if rows.Scan(&asset.id, &asset.mimeType, &asset.state) == nil {
			stale = append(stale, asset)
		}
	}
	rows.Close()
	dir, err := secureStudioAssetDir(s.studioRoot, documentID, false)
	if err != nil {
		return
	}
	for _, asset := range stale {
		_ = os.Remove(filepath.Join(dir, "."+asset.id+".part"))
		if extension := studioExtensionForMIME(asset.mimeType); extension != "" {
			_ = os.Remove(filepath.Join(dir, asset.id+extension))
		}
		_, _ = s.store.db.Exec(`
			DELETE FROM studio_assets
			WHERE id=? AND document_id=? AND state=?`, asset.id, documentID, asset.state)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".part") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "."), ".part")
		decoded, decodeErr := hex.DecodeString(id)
		info, infoErr := entry.Info()
		if decodeErr != nil || len(decoded) != 16 || infoErr != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func secureStudioAssetDir(root, documentID string, create bool) (string, error) {
	if !studioIDRE.MatchString(documentID) {
		return "", errStudioAssetInvalid
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(absRoot, 0o700); err != nil {
			return "", err
		}
	}
	if err := rejectStudioSymlink(absRoot); err != nil {
		return "", err
	}
	dir := filepath.Join(absRoot, documentID, "assets")
	if create {
		if err := os.Mkdir(filepath.Join(absRoot, documentID), 0o700); err != nil && !os.IsExist(err) {
			return "", err
		}
		if err := rejectStudioSymlink(filepath.Join(absRoot, documentID)); err != nil {
			return "", err
		}
		if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
			return "", err
		}
	}
	if err := rejectStudioSymlink(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func rejectStudioSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("studio path is not a real directory")
	}
	return nil
}

func (s *Server) handleStudioAsset(w http.ResponseWriter, r *http.Request, documentID, assetID string, user *User) {
	if !studioIDRE.MatchString(assetID) || strings.Contains(assetID, "/") {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.serveStudioAsset(w, r, documentID, assetID, user)
	case http.MethodDelete:
		if user == nil {
			writeStudioError(w, http.StatusUnauthorized, "studio.auth_required", nil)
			return
		}
		s.deleteStudioAsset(w, documentID, assetID, user)
	default:
		writeStudioError(w, http.StatusMethodNotAllowed, "studio.method_not_allowed", nil)
	}
}

func (s *Server) studioAsset(assetID, documentID string) (StudioAsset, error) {
	var asset StudioAsset
	err := s.store.db.QueryRow(`
		SELECT id, document_id, filename, mime_type, size_bytes, sha256, state, created
		FROM studio_assets WHERE id=? AND document_id=?`, assetID, documentID).
		Scan(&asset.ID, &asset.DocumentID, &asset.Filename, &asset.MIMEType,
			&asset.SizeBytes, &asset.SHA256, &asset.State, &asset.Created)
	if err != nil {
		return StudioAsset{}, errStudioNotFound
	}
	return asset, nil
}

func studioAssetReferenced(doc StudioDocument, assetID string) bool {
	valid, err := validateStudioInput(StudioDocumentInput{
		TemplateKey: doc.TemplateKey, Title: doc.Title, Summary: doc.Summary,
		Language: doc.Language, AuthorLabel: doc.AuthorLabel, Tags: doc.Tags,
		Metadata: doc.Metadata, Content: doc.Content,
	})
	if err != nil {
		return false
	}
	for _, referenced := range valid.Assets {
		if referenced == assetID {
			return true
		}
	}
	return false
}

func (s *Server) serveStudioAsset(w http.ResponseWriter, r *http.Request, documentID, assetID string, user *User) {
	asset, err := s.studioAsset(assetID, documentID)
	if err != nil || (asset.State != "staged" && asset.State != "published") {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	private := false
	if user != nil {
		if _, docErr := s.store.getStudioDocument(documentID, user); docErr == nil {
			private = true
		}
	}
	if !private {
		if asset.State != "published" ||
			!s.canSeeCollectionID(user, studioDocumentsCollectionID) {
			writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
			return
		}
		published, pubErr := s.store.publishedStudioSnapshot(documentID)
		if pubErr != nil || published.PublicationKind != "documents" ||
			!studioAssetReferenced(published, assetID) {
			writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
			return
		}
	}
	dir, err := secureStudioAssetDir(s.studioRoot, documentID, false)
	if err != nil {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	extension := studioExtensionForMIME(asset.MIMEType)
	if extension == "" {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	path := filepath.Join(dir, asset.ID+extension)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", asset.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, asset.Filename, info.ModTime(), file)
}

func studioExtensionForMIME(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "audio/mpeg":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/wav":
		return ".wav"
	case "audio/mp4":
		return ".m4a"
	case "application/pdf":
		return ".pdf"
	case "application/epub+zip":
		return ".epub"
	case "text/plain":
		return ".txt"
	case "text/markdown":
		return ".md"
	case "text/vtt":
		return ".vtt"
	default:
		return ""
	}
}

func (s *Server) deleteStudioAsset(w http.ResponseWriter, documentID, assetID string, user *User) {
	doc, err := s.store.getStudioDocument(documentID, user)
	if err != nil {
		writeStudioStoreError(w, err, 0)
		return
	}
	if studioAssetReferenced(doc, assetID) {
		writeStudioError(w, http.StatusConflict, "studio.asset_in_use", nil)
		return
	}
	if published, err := s.store.publishedStudioSnapshot(documentID); err == nil &&
		studioAssetReferenced(published, assetID) {
		writeStudioError(w, http.StatusConflict, "studio.asset_in_use", nil)
		return
	}
	result, err := s.store.db.Exec(`
		UPDATE studio_assets SET state='deleted'
		WHERE id=? AND document_id=? AND state!='deleted'`, assetID, documentID)
	if err != nil {
		writeStudioError(w, http.StatusInternalServerError, "studio.internal", nil)
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		writeStudioError(w, http.StatusNotFound, "studio.asset_not_found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
