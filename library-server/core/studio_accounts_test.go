package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func studioAccountDocument(t *testing.T, s *Server, owner *User, title string, publish bool) StudioDocument {
	t.Helper()
	var input StudioDocumentInput
	if err := json.Unmarshal([]byte(validStudioDocumentBody(title, 0)), &input); err != nil {
		t.Fatal(err)
	}
	valid, err := validateStudioInput(input)
	if err != nil {
		t.Fatal(err)
	}
	document, err := s.store.createStudioDocument(owner, valid)
	if err != nil {
		t.Fatal(err)
	}
	if publish {
		document, err = s.store.publishStudioDocument(document.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
	}
	return document
}

func deleteStudioAuthor(t *testing.T, s *Server, cookie *http.Cookie, userID int64, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/admin/users/" + strconv.FormatInt(userID, 10)
	if query != "" {
		path += "?" + query
	}
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	s.handleAdminUserOp(response, request)
	return response
}

func TestDeleteStudioAuthorRequiresExplicitStrategy(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-custodia", 40, true)
	author, err := s.createUser("autora-baja", "password!9", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	document := studioAccountDocument(t, s, author, "Obra privada", false)

	response := deleteStudioAuthor(t, s, adminCookie, author.ID, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("delete without strategy: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		ErrorCode string               `json:"errorCode"`
		Details   StudioDeletionImpact `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ErrorCode != "users.studio_strategy_required" || payload.Details.Documents != 1 {
		t.Fatalf("unexpected impact: %+v", payload)
	}
	var users int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, author.ID).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatal("author was deleted despite missing strategy")
	}
	stored, err := s.store.getStudioDocument(document.ID, author)
	if err != nil || stored.OwnerUserID == nil || *stored.OwnerUserID != author.ID {
		t.Fatalf("document ownership changed on rejected deletion: %+v %v", stored, err)
	}
}

func TestDeleteStudioAuthorTransfersDocumentsAssetsAndAudit(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-transfer", 40, true)
	var adminID int64
	if err := s.store.db.QueryRow(`SELECT id FROM users WHERE username='admin-transfer'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	author, _ := s.createUser("autora-transfer", "password!9", 30, false)
	target, _ := s.createUser("autora-destino", "password!9", 30, false)
	grantStudio(t, s, target.Username, false)
	document := studioAccountDocument(t, s, author, "Obra publicada", true)
	if _, err := s.store.db.Exec(`
		INSERT INTO studio_assets
			(id, document_id, owner_user_id, filename, mime_type, size_bytes, sha256, state, created)
		VALUES ('asset-transfer', ?, ?, 'obra.png', 'image/png', 10, 'hash', 'staged', 1)`,
		document.ID, author.ID); err != nil {
		t.Fatal(err)
	}

	response := deleteStudioAuthor(t, s, adminCookie, author.ID,
		"studioStrategy=transfer&transferTo="+strconv.FormatInt(target.ID, 10))
	if response.Code != http.StatusOK {
		t.Fatalf("transfer delete: %d %s", response.Code, response.Body.String())
	}
	var users int
	_ = s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, author.ID).Scan(&users)
	if users != 0 {
		t.Fatal("source author survived successful transfer")
	}
	transferred, err := s.store.getStudioDocument(document.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.OwnerUserID == nil || *transferred.OwnerUserID != target.ID ||
		transferred.Status != "published" || transferred.PublishedRevision == nil {
		t.Fatalf("document transfer changed publication incorrectly: %+v", transferred)
	}
	var assetOwner int64
	if err := s.store.db.QueryRow(`SELECT owner_user_id FROM studio_assets WHERE id='asset-transfer'`).Scan(&assetOwner); err != nil {
		t.Fatal(err)
	}
	if assetOwner != target.ID {
		t.Fatalf("asset owner = %d, want %d", assetOwner, target.ID)
	}
	var editorID int64
	if err := s.store.db.QueryRow(`
		SELECT editor_user_id FROM studio_revisions
		WHERE document_id=? AND revision=?`, document.ID, transferred.Revision).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	if editorID != adminID {
		t.Fatalf("ownership audit editor = %d, want admin %d", editorID, adminID)
	}
}

func TestDeleteStudioAuthorCustodyPreservesPublication(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-preserva", 40, true)
	author, _ := s.createUser("autora-custodia", "password!9", 30, false)
	document := studioAccountDocument(t, s, author, "Obra custodiada", true)

	response := deleteStudioAuthor(t, s, adminCookie, author.ID, "studioStrategy=custody")
	if response.Code != http.StatusOK {
		t.Fatalf("custody delete: %d %s", response.Code, response.Body.String())
	}
	var owner any
	var status string
	var publishedRevision *int
	if err := s.store.db.QueryRow(`
		SELECT owner_user_id, status, published_revision
		FROM studio_documents WHERE id=?`, document.ID).
		Scan(&owner, &status, &publishedRevision); err != nil {
		t.Fatal(err)
	}
	if owner != nil || status != "published" || publishedRevision == nil {
		t.Fatalf("custody state owner=%v status=%s published=%v", owner, status, publishedRevision)
	}
	if _, err := s.store.publishedStudioDocument(document.ID); err != nil {
		t.Fatalf("custody hid published document: %v", err)
	}
}

func TestDeleteStudioAuthorWithdrawsAndArchives(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-retira", 40, true)
	author, _ := s.createUser("autora-retirada", "password!9", 30, false)
	document := studioAccountDocument(t, s, author, "Obra retirada", true)

	response := deleteStudioAuthor(t, s, adminCookie, author.ID, "studioStrategy=withdraw")
	if response.Code != http.StatusOK {
		t.Fatalf("withdraw delete: %d %s", response.Code, response.Body.String())
	}
	var owner any
	var status string
	var publishedRevision any
	if err := s.store.db.QueryRow(`
		SELECT owner_user_id, status, published_revision
		FROM studio_documents WHERE id=?`, document.ID).
		Scan(&owner, &status, &publishedRevision); err != nil {
		t.Fatal(err)
	}
	if owner != nil || status != "archived" || publishedRevision != nil {
		t.Fatalf("withdraw state owner=%v status=%s published=%v", owner, status, publishedRevision)
	}
	if _, err := s.store.publishedStudioDocument(document.ID); !errors.Is(err, errStudioNotFound) {
		t.Fatalf("withdrawn document remains public: %v", err)
	}
}

func TestDeleteStudioAuthorRejectsIneligibleTransferAtomically(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-invalida", 40, true)
	author, _ := s.createUser("autora-origen", "password!9", 30, false)
	target, _ := s.createUser("lector-no-autor", "password!9", 30, false)
	document := studioAccountDocument(t, s, author, "No transferir", false)

	response := deleteStudioAuthor(t, s, adminCookie, author.ID,
		"studioStrategy=transfer&transferTo="+strconv.FormatInt(target.ID, 10))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid target: %d %s", response.Code, response.Body.String())
	}
	var users int
	_ = s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, author.ID).Scan(&users)
	if users != 1 {
		t.Fatal("source author deleted after rejected transfer")
	}
	stored, err := s.store.getStudioDocument(document.ID, author)
	if err != nil || stored.OwnerUserID == nil || *stored.OwnerUserID != author.ID {
		t.Fatalf("rejected transfer mutated document: %+v %v", stored, err)
	}
}

func TestDeleteStudioAuthorRejectsTransferOverQuotaAtomically(t *testing.T) {
	s := testAuthServer(t, "")
	adminCookie := sessionFor(t, s, "admin-cuota", 40, true)
	author, _ := s.createUser("autora-con-assets", "password!9", 30, false)
	target, _ := s.createUser("autora-sin-espacio", "password!9", 30, false)
	grantStudio(t, s, target.Username, false)
	document := studioAccountDocument(t, s, author, "No cabe", false)
	if _, err := s.store.db.Exec(`
		UPDATE user_capabilities SET quota_bytes=5 WHERE user_id=?`, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.db.Exec(`
		INSERT INTO studio_assets
			(id, document_id, owner_user_id, filename, mime_type, size_bytes, sha256, state, created)
		VALUES ('asset-too-large', ?, ?, 'obra.png', 'image/png', 10, 'hash-quota', 'staged', 1)`,
		document.ID, author.ID); err != nil {
		t.Fatal(err)
	}

	response := deleteStudioAuthor(t, s, adminCookie, author.ID,
		"studioStrategy=transfer&transferTo="+strconv.FormatInt(target.ID, 10))
	if response.Code != http.StatusConflict {
		t.Fatalf("over-quota transfer: %d %s", response.Code, response.Body.String())
	}
	var users int
	_ = s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, author.ID).Scan(&users)
	if users != 1 {
		t.Fatal("source author deleted after rejected quota check")
	}
	stored, err := s.store.getStudioDocument(document.ID, author)
	if err != nil || stored.OwnerUserID == nil || *stored.OwnerUserID != author.ID {
		t.Fatalf("quota rejection mutated document: %+v %v", stored, err)
	}
	var assetOwner int64
	if err := s.store.db.QueryRow(`
		SELECT owner_user_id FROM studio_assets WHERE id='asset-too-large'`).Scan(&assetOwner); err != nil {
		t.Fatal(err)
	}
	if assetOwner != author.ID {
		t.Fatalf("quota rejection transferred asset to %d", assetOwner)
	}
}
