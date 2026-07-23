package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type studioZeroReader struct{}

func (studioZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func validStudioDocumentBody(title string, baseRevision int) string {
	body := map[string]any{
		"templateKey": "document",
		"title":       title,
		"summary":     "Una prueba local",
		"language":    "es",
		"tags":        []string{"Historia", "historia", "Local"},
		"content": map[string]any{
			"schemaVersion": 1,
			"classification": map[string]any{
				"workType": "manual",
				"topics":   []string{"Historia", "local"},
				"audience": []string{"clase"},
			},
			"presentation": map[string]any{
				"contentWidth": "reading",
				"fontPreset":   "editorial",
			},
			"blocks": []any{
				map[string]any{"id": "titulo", "type": "heading", "level": 1, "text": title},
				map[string]any{"id": "intro", "type": "paragraph", "text": "Contenido de prueba"},
				map[string]any{
					"id": "referencia", "type": "itemRef",
					"itemId": "zim:wikipedia_es:Q0hB", "titleSnapshot": "Historia",
				},
			},
		},
	}
	if baseRevision > 0 {
		body["baseRevision"] = baseRevision
	}
	encoded, _ := json.Marshal(body)
	return string(encoded)
}

func studioTestMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	s.registerStudioRoutes(mux)
	return mux
}

func studioRequest(h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func grantStudio(t *testing.T, s *Server, username string, publish bool) int64 {
	t.Helper()
	var userID int64
	if err := s.store.db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&userID); err != nil {
		t.Fatalf("lookup user: %v", err)
	}
	if _, err := s.store.db.Exec(`
		INSERT INTO user_capabilities (user_id, can_author, can_publish, quota_bytes, updated)
		VALUES (?,?,?,?,1)`, userID, 1, boolInt(publish), int64(2<<30)); err != nil {
		t.Fatalf("grant studio: %v", err)
	}
	return userID
}

func decodeStudioDocumentResponse(t *testing.T, rec *httptest.ResponseRecorder) StudioDocument {
	t.Helper()
	var payload struct {
		Document StudioDocument `json:"document"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode document response: %v (%s)", err, rec.Body.String())
	}
	return payload.Document
}

func studioUploadRequest(t *testing.T, h http.Handler, path, filename string, payload []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func studioTestPNG(t *testing.T) []byte {
	t.Helper()
	var payload bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 120, G: 80, B: 220, A: 255})
	if err := png.Encode(&payload, img); err != nil {
		t.Fatal(err)
	}
	return payload.Bytes()
}

func decodeStudioAssetResponse(t *testing.T, rec *httptest.ResponseRecorder) StudioAsset {
	t.Helper()
	var payload struct {
		Asset StudioAsset `json:"asset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode asset response: %v (%s)", err, rec.Body.String())
	}
	return payload.Asset
}

func studioDocumentBodyWithImage(t *testing.T, title string, baseRevision int, assetID string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(validStudioDocumentBody(title, baseRevision)), &body); err != nil {
		t.Fatal(err)
	}
	content := body["content"].(map[string]any)
	blocks := content["blocks"].([]any)
	content["blocks"] = append(blocks, map[string]any{
		"id": "imagen", "type": "image", "assetId": assetID,
		"caption": "Pie de imagen", "alt": "Descripción",
	})
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestStudioSchemaIsAdditive(t *testing.T) {
	s := testAuthServer(t, "")
	for _, table := range []string{
		"user_capabilities", "studio_documents", "studio_revisions", "studio_assets",
		"studio_publish_targets", "studio_links", "studio_facets", "content_origins",
		"users", "sessions", "collection_access",
	} {
		var found string
		if err := s.store.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&found); err != nil {
			t.Fatalf("falta tabla %s tras migración aditiva: %v", table, err)
		}
	}
}

func TestStudioValidationNormalizesPortableFacetsAndLinks(t *testing.T) {
	var input StudioDocumentInput
	if err := json.Unmarshal([]byte(validStudioDocumentBody("Mi manual", 0)), &input); err != nil {
		t.Fatal(err)
	}
	valid, err := validateStudioInput(input)
	if err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if got := strings.Join(valid.Input.Tags, ","); got != "Historia,Local" {
		t.Fatalf("tags normalizados = %q", got)
	}
	if valid.Classification.WorkType != "manual" ||
		strings.Join(valid.Facets["topic"], ",") != "historia,local" {
		t.Fatalf("facetas inesperadas: %#v", valid.Facets)
	}
	if len(valid.Links) != 1 || valid.Links[0] != "zim:wikipedia_es:Q0hB" {
		t.Fatalf("enlaces inesperados: %#v", valid.Links)
	}

	input.Content = json.RawMessage(`{
		"schemaVersion":1,
		"blocks":[
			{"id":"repetido","type":"paragraph","text":"uno"},
			{"id":"repetido","type":"paragraph","text":"dos"}
		]
	}`)
	if _, err := validateStudioInput(input); err == nil {
		t.Fatal("un bloque duplicado no fue rechazado")
	}
}

func TestStudioCapabilitiesAndPrivateDraftLifecycle(t *testing.T) {
	s := testAuthServer(t, "")
	h := studioTestMux(s)
	authorCookie := sessionFor(t, s, "autora", 30, false)
	otherCookie := sessionFor(t, s, "lector", 30, false)

	if rec := studioRequest(h, http.MethodGet, "/api/studio/documents", "", authorCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("sin capacidad: quiero 403, tengo %d (%s)", rec.Code, rec.Body.String())
	}
	authorID := grantStudio(t, s, "autora", true)
	grantStudio(t, s, "lector", false)

	createdRec := studioRequest(h, http.MethodPost, "/api/studio/documents",
		validStudioDocumentBody("Primera versión", 0), authorCookie)
	if createdRec.Code != http.StatusCreated {
		t.Fatalf("crear: quiero 201, tengo %d (%s)", createdRec.Code, createdRec.Body.String())
	}
	created := decodeStudioDocumentResponse(t, createdRec)
	if created.ID == "" || created.Revision != 1 || created.OwnerUserID == nil || *created.OwnerUserID != authorID {
		t.Fatalf("documento creado inesperado: %#v", created)
	}

	privateRec := studioRequest(h, http.MethodGet, "/api/studio/documents/"+created.ID, "", otherCookie)
	if privateRec.Code != http.StatusNotFound {
		t.Fatalf("otro autor pudo descubrir el borrador: %d (%s)", privateRec.Code, privateRec.Body.String())
	}

	updatedRec := studioRequest(h, http.MethodPut, "/api/studio/documents/"+created.ID,
		validStudioDocumentBody("Segunda versión", 1), authorCookie)
	if updatedRec.Code != http.StatusOK {
		t.Fatalf("actualizar: quiero 200, tengo %d (%s)", updatedRec.Code, updatedRec.Body.String())
	}
	if updated := decodeStudioDocumentResponse(t, updatedRec); updated.Revision != 2 || updated.Title != "Segunda versión" {
		t.Fatalf("actualización inesperada: %#v", updated)
	}

	conflictRec := studioRequest(h, http.MethodPut, "/api/studio/documents/"+created.ID,
		validStudioDocumentBody("Edición antigua", 1), authorCookie)
	if conflictRec.Code != http.StatusConflict ||
		!strings.Contains(conflictRec.Body.String(), `"currentRevision":2`) {
		t.Fatalf("conflicto optimista incorrecto: %d (%s)", conflictRec.Code, conflictRec.Body.String())
	}

	revisionsRec := studioRequest(h, http.MethodGet,
		"/api/studio/documents/"+created.ID+"/revisions", "", authorCookie)
	if revisionsRec.Code != http.StatusOK {
		t.Fatalf("revisiones: %d (%s)", revisionsRec.Code, revisionsRec.Body.String())
	}
	var revisions struct {
		Revisions []StudioRevision `json:"revisions"`
	}
	if err := json.Unmarshal(revisionsRec.Body.Bytes(), &revisions); err != nil ||
		len(revisions.Revisions) != 2 || revisions.Revisions[0].Revision != 2 {
		t.Fatalf("historial inesperado: %#v, err=%v", revisions, err)
	}

	archivedRec := studioRequest(h, http.MethodDelete, "/api/studio/documents/"+created.ID, "", authorCookie)
	if archivedRec.Code != http.StatusOK || decodeStudioDocumentResponse(t, archivedRec).Status != "archived" {
		t.Fatalf("archivar: %d (%s)", archivedRec.Code, archivedRec.Body.String())
	}
}

func TestStudioAdminCapabilityContract(t *testing.T) {
	s := testAuthServer(t, "")
	userCookie := sessionFor(t, s, "creadora", 30, false)
	_ = userCookie
	var userID int64
	if err := s.store.db.QueryRow(`SELECT id FROM users WHERE username='creadora'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	rec := studioRequest(http.HandlerFunc(s.handleAdminStudioCapabilities), http.MethodPut,
		"/api/admin/studio/capabilities/"+strconv.FormatInt(userID, 10),
		`{"canAuthor":false,"canPublish":true,"quotaBytes":1048576}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("guardar capacidades: %d (%s)", rec.Code, rec.Body.String())
	}
	var author, publish int
	if err := s.store.db.QueryRow(
		`SELECT can_author, can_publish FROM user_capabilities WHERE user_id=?`, userID,
	).Scan(&author, &publish); err != nil || author != 1 || publish != 1 {
		t.Fatalf("publicar debe implicar autor: author=%d publish=%d err=%v", author, publish, err)
	}
}

func TestStudioPublicationUsesImmutableRevisionSnapshot(t *testing.T) {
	s := testAuthServer(t, "")
	cookie := sessionFor(t, s, "publicadora", 30, false)
	_ = cookie
	grantStudio(t, s, "publicadora", true)
	var owner User
	if err := s.store.db.QueryRow(`
		SELECT id, username, age, is_admin FROM users WHERE username='publicadora'`).
		Scan(&owner.ID, &owner.Username, &owner.Age, &owner.IsAdmin); err != nil {
		t.Fatal(err)
	}
	var input StudioDocumentInput
	if err := json.Unmarshal([]byte(validStudioDocumentBody("Versión pública", 0)), &input); err != nil {
		t.Fatal(err)
	}
	valid, err := validateStudioInput(input)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.store.createStudioDocument(&owner, valid)
	if err != nil {
		t.Fatal(err)
	}
	published, err := s.store.publishStudioDocument(created.ID, &owner)
	if err != nil || published.PublishedRevision == nil || *published.PublishedRevision != 1 {
		t.Fatalf("publicar revisión 1: doc=%#v err=%v", published, err)
	}

	var access string
	if err := s.store.db.QueryRow(`
		SELECT access FROM collection_access WHERE collection_id=?`,
		studioDocumentsCollectionID).Scan(&access); err != nil || access != "login" {
		t.Fatalf("colección Documents no nació cerrada a anónimos: access=%q err=%v", access, err)
	}

	if err := json.Unmarshal([]byte(validStudioDocumentBody("Borrador secreto nuevo", 1)), &input); err != nil {
		t.Fatal(err)
	}
	input.Summary = "Resumen privado todavía no publicado"
	valid, err = validateStudioInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.store.updateStudioDocument(created.ID, &owner, valid); err != nil {
		t.Fatal(err)
	}
	public, err := s.store.publishedStudioDocument(created.ID)
	if err != nil || public.Title != "Versión pública" || public.Revision != 1 {
		t.Fatalf("el borrador posterior contaminó la publicación: %#v err=%v", public, err)
	}
	hits, err := s.store.searchPublishedStudioDocuments("secreto nuevo")
	if err != nil || len(hits) != 0 {
		t.Fatalf("la búsqueda filtró texto privado: hits=%#v err=%v", hits, err)
	}
	hits, err = s.store.searchPublishedStudioDocuments("Versión pública")
	if err != nil || len(hits) != 1 ||
		hits[0].Title != "Versión pública" ||
		hits[0].Snippet != "Una prueba local" {
		t.Fatalf("título o snippet no proceden del snapshot público: hits=%#v err=%v", hits, err)
	}

	republished, err := s.store.publishStudioDocument(created.ID, &owner)
	if err != nil || republished.PublishedRevision == nil || *republished.PublishedRevision != 2 {
		t.Fatalf("republicar revisión 2: doc=%#v err=%v", republished, err)
	}
	public, err = s.store.publishedStudioDocument(created.ID)
	if err != nil || public.Title != "Borrador secreto nuevo" || public.Revision != 2 {
		t.Fatalf("la republicación no cambió el snapshot: %#v err=%v", public, err)
	}
	if _, err := s.store.unpublishStudioDocument(created.ID, &owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.publishedStudioDocument(created.ID); !errors.Is(err, errStudioNotFound) {
		t.Fatalf("despublicar dejó el documento visible: %v", err)
	}
}

func TestStudioAssetsStayPrivateUntilTheirSnapshotIsPublished(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	authorCookie := sessionFor(t, s, "autora-assets", 30, false)
	grantStudio(t, s, "autora-assets", true)
	readerCookie := sessionFor(t, s, "lectora-assets", 20, false)
	h := studioTestMux(s)

	createdRec := studioRequest(h, http.MethodPost, "/api/studio/documents",
		validStudioDocumentBody("Página con imagen", 0), authorCookie)
	if createdRec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", createdRec.Code, createdRec.Body.String())
	}
	doc := decodeStudioDocumentResponse(t, createdRec)
	upload := studioUploadRequest(t, h,
		"/api/studio/documents/"+doc.ID+"/assets", "mapa.png", studioTestPNG(t), authorCookie)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", upload.Code, upload.Body.String())
	}
	asset := decodeStudioAssetResponse(t, upload)

	privateRead := studioRequest(h, http.MethodGet,
		"/api/studio/documents/"+doc.ID+"/assets/"+asset.ID, "", readerCookie)
	if privateRead.Code != http.StatusNotFound {
		t.Fatalf("staged asset leaked: %d", privateRead.Code)
	}
	update := studioRequest(h, http.MethodPut, "/api/studio/documents/"+doc.ID,
		studioDocumentBodyWithImage(t, "Página con imagen", doc.Revision, asset.ID), authorCookie)
	if update.Code != http.StatusOK {
		t.Fatalf("update: %d %s", update.Code, update.Body.String())
	}
	doc = decodeStudioDocumentResponse(t, update)
	publish := studioRequest(h, http.MethodPost,
		"/api/studio/documents/"+doc.ID+"/publish", "", authorCookie)
	if publish.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", publish.Code, publish.Body.String())
	}
	var publishedState string
	if err := s.store.db.QueryRow(`SELECT state FROM studio_assets WHERE id=?`, asset.ID).Scan(&publishedState); err != nil {
		t.Fatal(err)
	}
	if publishedState != "published" {
		t.Fatalf("asset promotion was not atomic with publication: %q", publishedState)
	}

	publicRead := studioRequest(h, http.MethodGet,
		"/api/studio/documents/"+doc.ID+"/assets/"+asset.ID, "", readerCookie)
	if publicRead.Code != http.StatusOK {
		t.Fatalf("published asset unreadable: %d %s", publicRead.Code, publicRead.Body.String())
	}
	if publicRead.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(publicRead.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("missing hardened asset headers: %#v", publicRead.Header())
	}
	if !bytes.Equal(publicRead.Body.Bytes(), studioTestPNG(t)) {
		t.Fatal("served asset differs from uploaded bytes")
	}

	secondUpload := studioUploadRequest(t, h,
		"/api/studio/documents/"+doc.ID+"/assets", "privada.png", studioTestPNG(t), authorCookie)
	if secondUpload.Code != http.StatusCreated {
		t.Fatalf("second upload: %d %s", secondUpload.Code, secondUpload.Body.String())
	}
	second := decodeStudioAssetResponse(t, secondUpload)
	draftUpdate := studioRequest(h, http.MethodPut, "/api/studio/documents/"+doc.ID,
		studioDocumentBodyWithImage(t, "Borrador nuevo", doc.Revision, second.ID), authorCookie)
	if draftUpdate.Code != http.StatusOK {
		t.Fatalf("draft update: %d %s", draftUpdate.Code, draftUpdate.Body.String())
	}
	leakAttempt := studioRequest(h, http.MethodGet,
		"/api/studio/documents/"+doc.ID+"/assets/"+second.ID, "", readerCookie)
	if leakAttempt.Code != http.StatusNotFound {
		t.Fatalf("draft-only asset leaked through published document: %d", leakAttempt.Code)
	}
	oldSnapshotRead := studioRequest(h, http.MethodGet,
		"/api/studio/documents/"+doc.ID+"/assets/"+asset.ID, "", readerCookie)
	if oldSnapshotRead.Code != http.StatusOK {
		t.Fatalf("published snapshot asset disappeared after draft edit: %d", oldSnapshotRead.Code)
	}
	inUseDelete := studioRequest(h, http.MethodDelete,
		"/api/studio/documents/"+doc.ID+"/assets/"+asset.ID, "", authorCookie)
	if inUseDelete.Code != http.StatusConflict {
		t.Fatalf("published asset deletion was not blocked: %d", inUseDelete.Code)
	}
	unusedUpload := studioUploadRequest(t, h,
		"/api/studio/documents/"+doc.ID+"/assets", "sin-usar.png", studioTestPNG(t), authorCookie)
	if unusedUpload.Code != http.StatusCreated {
		t.Fatalf("unused upload: %d %s", unusedUpload.Code, unusedUpload.Body.String())
	}
	unused := decodeStudioAssetResponse(t, unusedUpload)
	unusedDelete := studioRequest(h, http.MethodDelete,
		"/api/studio/documents/"+doc.ID+"/assets/"+unused.ID, "", authorCookie)
	if unusedDelete.Code != http.StatusNoContent {
		t.Fatalf("unused asset logical delete: %d %s", unusedDelete.Code, unusedDelete.Body.String())
	}
	var deletedState string
	if err := s.store.db.QueryRow(`SELECT state FROM studio_assets WHERE id=?`, unused.ID).Scan(&deletedState); err != nil {
		t.Fatal(err)
	}
	if deletedState != "deleted" {
		t.Fatalf("asset was not logically deleted: %q", deletedState)
	}
}

func TestStudioAssetUploadTokenIsDocumentBoundAndSingleUse(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	cookie := sessionFor(t, s, "autora-token", 30, false)
	grantStudio(t, s, "autora-token", false)
	h := studioTestMux(s)

	first := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Primero", 0), cookie))
	second := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Segundo", 0), cookie))
	issue := func(documentID string) string {
		t.Helper()
		rec := studioRequest(h, http.MethodPost,
			"/api/studio/documents/"+documentID+"/upload-token", "", cookie)
		if rec.Code != http.StatusCreated {
			t.Fatalf("issue token: %d %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Token
	}

	wrongToken := issue(first.ID)
	wrong := studioUploadRequest(t, h,
		"/api/studio/documents/"+second.ID+"/assets?ut="+wrongToken,
		"foto.png", studioTestPNG(t), nil)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("cross-document token accepted: %d", wrong.Code)
	}
	consumed := studioUploadRequest(t, h,
		"/api/studio/documents/"+first.ID+"/assets?ut="+wrongToken,
		"foto.png", studioTestPNG(t), nil)
	if consumed.Code != http.StatusUnauthorized {
		t.Fatalf("failed token was reusable: %d", consumed.Code)
	}

	token := issue(first.ID)
	ok := studioUploadRequest(t, h,
		"/api/studio/documents/"+first.ID+"/assets?ut="+token,
		"foto.png", studioTestPNG(t), nil)
	if ok.Code != http.StatusCreated {
		t.Fatalf("valid direct upload: %d %s", ok.Code, ok.Body.String())
	}
	reused := studioUploadRequest(t, h,
		"/api/studio/documents/"+first.ID+"/assets?ut="+token,
		"otra.png", studioTestPNG(t), nil)
	if reused.Code != http.StatusUnauthorized {
		t.Fatalf("upload token reused: %d", reused.Code)
	}
}

func TestStudioDirectUploadTokenPassesMiddlewareWithoutAdminToken(t *testing.T) {
	s := testAuthServer(t, strings.Repeat("m", 32))
	s.studioRoot = t.TempDir()
	cookie := sessionFor(t, s, "autora-directa", 30, false)
	grantStudio(t, s, "autora-directa", false)
	raw := studioTestMux(s)
	doc := decodeStudioDocumentResponse(t, studioRequest(raw, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Directa", 0), cookie))
	tokenRec := studioRequest(raw, http.MethodPost,
		"/api/studio/documents/"+doc.ID+"/upload-token", "", cookie)
	var tokenPayload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokenPayload); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "foto.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(studioTestPNG(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/studio/documents/"+doc.ID+"/assets?ut="+tokenPayload.Token, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", "https://webview.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	s.middleware(raw).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("direct token blocked by middleware: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://webview.example" {
		t.Fatalf("direct upload response lacks scoped CORS: %#v", rec.Header())
	}
}

func TestStudioAssetRejectsActiveOrMismatchedContent(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	cookie := sessionFor(t, s, "autora-mime", 30, false)
	grantStudio(t, s, "autora-mime", false)
	h := studioTestMux(s)
	doc := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("MIME", 0), cookie))

	for _, candidate := range []struct {
		name string
		data []byte
	}{
		{name: "ataque.png", data: []byte(`<html><script>alert(1)</script></html>`)},
		{name: "disfraz.jpg", data: studioTestPNG(t)},
		{name: "vector.svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
	} {
		rec := studioUploadRequest(t, h,
			"/api/studio/documents/"+doc.ID+"/assets", candidate.name, candidate.data, cookie)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("%s accepted with status %d: %s", candidate.name, rec.Code, rec.Body.String())
		}
	}
	var count int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM studio_assets WHERE document_id=?`, doc.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected assets left %d database rows", count)
	}
	entries, err := os.ReadDir(filepath.Join(s.studioRoot, doc.ID, "assets"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".part") {
			t.Fatalf("rejected upload left staging file %q", entry.Name())
		}
	}
}

func TestStudioAssetPathRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	documentID := "document-safe"
	if err := os.Mkdir(filepath.Join(root, documentID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, documentID, "assets")); err != nil {
		t.Skipf("symlinks unavailable on this system: %v", err)
	}
	if _, err := secureStudioAssetDir(root, documentID, false); err == nil {
		t.Fatal("symlinked asset directory was accepted")
	}
}

func TestStudioAssetStreamingLimitRemovesPartialFile(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	source := io.MultiReader(
		bytes.NewReader(studioTestPNG(t)),
		io.LimitReader(studioZeroReader{}, studioMaxImageBytes),
	)
	_, _, _, err := s.persistStudioAsset(
		"oversize-document", &User{ID: 1, IsAdmin: true}, "grande.png", source)
	if !errors.Is(err, errStudioAssetTooLarge) {
		t.Fatalf("oversized stream returned %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(s.studioRoot, "oversize-document", "assets"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".part") {
			t.Fatalf("oversized stream left partial file %q", entry.Name())
		}
	}
}

func TestStudioAssetQuotaRejectsWithoutRowOrFile(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	cookie := sessionFor(t, s, "autora-cuota", 30, false)
	userID := grantStudio(t, s, "autora-cuota", false)
	if _, err := s.store.db.Exec(`UPDATE user_capabilities SET quota_bytes=10 WHERE user_id=?`, userID); err != nil {
		t.Fatal(err)
	}
	h := studioTestMux(s)
	doc := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Cuota", 0), cookie))
	if _, err := s.store.db.Exec(`
		INSERT INTO studio_assets
			(id, document_id, owner_user_id, filename, mime_type, size_bytes, sha256, state, created)
		VALUES ('quota-used', ?, ?, 'usado.png', 'image/png', 10, 'hash', 'staged', 1)`,
		doc.ID, userID); err != nil {
		t.Fatal(err)
	}
	upload := studioUploadRequest(t, h,
		"/api/studio/documents/"+doc.ID+"/assets", "foto.png", studioTestPNG(t), cookie)
	if upload.Code != http.StatusInsufficientStorage {
		t.Fatalf("quota upload: %d %s", upload.Code, upload.Body.String())
	}
	var count int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM studio_assets WHERE document_id=?`, doc.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("quota rejection changed the existing %d asset rows", count)
	}
	if _, err := os.Stat(filepath.Join(s.studioRoot, doc.ID)); !os.IsNotExist(err) {
		t.Fatalf("exhausted quota still created a staging directory: %v", err)
	}
}

func TestStudioAssetQuotaIsAtomicAcrossConcurrentUploads(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	cookie := sessionFor(t, s, "autora-concurrente", 30, false)
	userID := grantStudio(t, s, "autora-concurrente", false)
	imageBytes := studioTestPNG(t)
	if _, err := s.store.db.Exec(`
		UPDATE user_capabilities SET quota_bytes=? WHERE user_id=?`,
		int64(len(imageBytes)+10), userID); err != nil {
		t.Fatal(err)
	}
	h := studioTestMux(s)
	doc := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Concurrente", 0), cookie))

	start := make(chan struct{})
	results := make(chan int, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			rec := studioUploadRequest(t, h,
				"/api/studio/documents/"+doc.ID+"/assets", "foto.png", imageBytes, cookie)
			results <- rec.Code
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var created, rejected int
	for status := range results {
		if status == http.StatusCreated {
			created++
		}
		if status == http.StatusInsufficientStorage {
			rejected++
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("concurrent quota results: created=%d rejected=%d", created, rejected)
	}
	var count int
	if err := s.store.db.QueryRow(`
		SELECT COUNT(*) FROM studio_assets
		WHERE document_id=? AND state='staged'`, doc.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent quota committed %d assets", count)
	}
}

func TestStudioDocumentCannotReferenceForeignAsset(t *testing.T) {
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	firstCookie := sessionFor(t, s, "autora-uno", 30, false)
	secondCookie := sessionFor(t, s, "autora-dos", 30, false)
	grantStudio(t, s, "autora-uno", false)
	grantStudio(t, s, "autora-dos", false)
	h := studioTestMux(s)
	first := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Uno", 0), firstCookie))
	second := decodeStudioDocumentResponse(t, studioRequest(h, http.MethodPost,
		"/api/studio/documents", validStudioDocumentBody("Dos", 0), secondCookie))
	upload := studioUploadRequest(t, h,
		"/api/studio/documents/"+first.ID+"/assets", "foto.png", studioTestPNG(t), firstCookie)
	asset := decodeStudioAssetResponse(t, upload)

	update := studioRequest(h, http.MethodPut, "/api/studio/documents/"+second.ID,
		studioDocumentBodyWithImage(t, "Dos", second.Revision, asset.ID), secondCookie)
	if update.Code != http.StatusUnprocessableEntity {
		t.Fatalf("foreign asset reference accepted: %d %s", update.Code, update.Body.String())
	}
}
