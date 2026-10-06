package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func depotTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s := testAuthServer(t, "")
	s.studioRoot = t.TempDir()
	s.mediaRoot = t.TempDir()
	s.media = &mediaDeps{root: s.mediaRoot}
	return s, studioTestMux(s)
}

func depotBody(t *testing.T, template string, baseRevision int, metadata map[string]any) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(validStudioDocumentBody("VLC media player", baseRevision)), &body); err != nil {
		t.Fatal(err)
	}
	body["templateKey"] = template
	body["summary"] = "Reproduce casi cualquier vídeo"
	body["authorLabel"] = "VideoLAN"
	body["metadata"] = metadata
	encoded, _ := json.Marshal(body)
	return string(encoded)
}

// Depot reparte ejecutables: solo el admin crea, convierte o publica ahí,
// aunque el usuario tenga permiso de publicar en el resto de apartados.
func TestDepotIsAdminOnly(t *testing.T) {
	s, h := depotTestServer(t)
	cookie := sessionFor(t, s, "autora", 30, false)
	grantStudio(t, s, "autora", true)

	rec := studioRequest(h, http.MethodPost, "/api/studio/documents",
		depotBody(t, "depot.program", 0, map[string]any{"collection": "General"}), cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("una autora sin admin creó un Depot: %d %s", rec.Code, rec.Body.String())
	}

	rec = studioRequest(h, http.MethodPost, "/api/studio/documents",
		depotBody(t, "cabinet.pdf", 0, map[string]any{"collection": "General"}), cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cabinet: %d %s", rec.Code, rec.Body.String())
	}
	document := decodeStudioDocumentResponse(t, rec)
	rec = studioRequest(h, http.MethodPut, "/api/studio/documents/"+document.ID,
		depotBody(t, "depot.program", document.Revision, map[string]any{"collection": "General"}), cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un borrador de Cabinet se convirtió en Depot: %d %s", rec.Code, rec.Body.String())
	}

	if caps := s.studioCapabilities(&User{ID: 9}); caps.CanDepot {
		t.Fatal("canDepot para quien no es admin")
	}
}

func TestDepotUploadRecognisesPackagesBySignature(t *testing.T) {
	s, h := depotTestServer(t)
	cookie := sessionFor(t, s, "admin", 40, true)
	rec := studioRequest(h, http.MethodPost, "/api/studio/documents",
		depotBody(t, "depot.program", 0, map[string]any{"collection": "General"}), cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create depot: %d %s", rec.Code, rec.Body.String())
	}
	document := decodeStudioDocumentResponse(t, rec)
	upload := func(name string, payload []byte) int {
		return studioUploadRequest(t, h,
			"/api/studio/documents/"+document.ID+"/assets?purpose=package", name, payload, cookie).Code
	}
	cases := []struct {
		name    string
		payload []byte
		want    int
	}{
		{"setup.exe", []byte("MZ\x90\x00 cabecera PE"), http.StatusCreated},
		{"setup.msi", []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1, 0}, http.StatusCreated},
		{"vlc_amd64.deb", []byte("!<arch>\ndebian-binary"), http.StatusCreated},
		{"portable.zip", []byte("PK\x03\x04 resto"), http.StatusCreated},
		{"skins.rar", []byte("Rar!\x1a\x07\x00"), http.StatusCreated},
		{"falso.exe", []byte("PK\x03\x04 un zip con otro nombre"), http.StatusUnsupportedMediaType},
		{"script.sh", []byte("#!/bin/sh\n"), http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		if got := upload(tc.name, tc.payload); got != tc.want {
			t.Fatalf("%s: %d, quiero %d", tc.name, got, tc.want)
		}
	}
}

func TestDepotPublishesBlockedProgramWithFiles(t *testing.T) {
	s, h := depotTestServer(t)
	cookie := sessionFor(t, s, "admin", 40, true)
	rec := studioRequest(h, http.MethodPost, "/api/studio/documents",
		depotBody(t, "depot.program", 0, map[string]any{"collection": "General"}), cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create depot: %d %s", rec.Code, rec.Body.String())
	}
	document := decodeStudioDocumentResponse(t, rec)
	exe := decodeStudioAssetResponse(t, studioUploadRequest(t, h,
		"/api/studio/documents/"+document.ID+"/assets?purpose=package",
		"vlc-3.0.21-win64.exe", []byte("MZ\x90\x00 cabecera PE"), cookie))
	zip := decodeStudioAssetResponse(t, studioUploadRequest(t, h,
		"/api/studio/documents/"+document.ID+"/assets?purpose=package",
		"vlc-portable.zip", []byte("PK\x03\x04 resto"), cookie))
	shot := decodeStudioAssetResponse(t, studioUploadRequest(t, h,
		"/api/studio/documents/"+document.ID+"/assets?purpose=screenshot",
		"captura.png", studioTestPNG(t), cookie))
	icon := decodeStudioAssetResponse(t, studioUploadRequest(t, h,
		"/api/studio/documents/"+document.ID+"/assets?purpose=cover",
		"icono.png", studioTestPNG(t), cookie))

	save := func(zipOS string) StudioDocument {
		t.Helper()
		rec := studioRequest(h, http.MethodPut, "/api/studio/documents/"+document.ID,
			depotBody(t, "depot.program", document.Revision, map[string]any{
				"collection": "General", "version": "3.0.21", "shelf": "multimedia",
				"coverAssetId": icon.ID,
				"files": []map[string]any{
					{"assetId": exe.ID, "os": "win", "arch": "x64", "label": "Instalador"},
					{"assetId": zip.ID, "os": zipOS, "arch": "x64", "label": "Portable"},
				},
				"screenshots": []map[string]any{{"assetId": shot.ID, "caption": "Reproduciendo"}},
			}), cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("update depot: %d %s", rec.Code, rec.Body.String())
		}
		return decodeStudioDocumentResponse(t, rec)
	}

	// Un zip no dice para qué sistema es: sin elegirlo no se publica.
	document = save("")
	if rec := studioRequest(h, http.MethodPost, "/api/studio/documents/"+document.ID+"/publish", "", cookie); rec.Code == http.StatusOK {
		t.Fatal("publicó un archivo sin sistema")
	}
	document = save("win")
	rec = studioRequest(h, http.MethodPost, "/api/studio/documents/"+document.ID+"/publish", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish depot: %d %s", rec.Code, rec.Body.String())
	}
	if published := decodeStudioDocumentResponse(t, rec); published.PublicationKind != "depot" {
		t.Fatalf("publicación inesperada: %#v", published)
	}

	if _, err := os.Stat(filepath.Join(s.mediaRoot, "Depot", "General", "studio-"+document.ID+".json")); err != nil {
		t.Fatalf("no hay ficha en Depot/General: %v", err)
	}
	if cfg := s.accessMap()[surfaceAccessDepot]; cfg.Access != "blocked" {
		t.Fatalf("Depot no nació bloqueado: %+v", cfg)
	}

	// El admin lo ve, con sus archivos y su SHA-256; un anónimo no.
	req := httptest.NewRequest(http.MethodGet, "/api/items/surface?provider=depot", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.handleSurfaceItems(s.media)(w, req)
	var payload struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || len(payload.Items) != 1 {
		t.Fatalf("items de Depot: %d %s", w.Code, w.Body.String())
	}
	program := payload.Items[0].Program
	if program == nil || len(program.Files) != 2 || len(program.Screenshots) != 1 ||
		program.Files[0].Name != "vlc-3.0.21-win64.exe" || program.Files[0].Format != "exe" ||
		program.Files[0].SHA256 != exe.SHA256 || program.Version != "3.0.21" ||
		len(program.Published) != len("2006-01-02") {
		t.Fatalf("ficha de programa incompleta: %+v", program)
	}
	w = httptest.NewRecorder()
	s.handleSurfaceItems(s.media)(w, httptest.NewRequest(http.MethodGet, "/api/items/surface?provider=depot", nil))
	if strings.Contains(w.Body.String(), "VLC") {
		t.Fatal("un anónimo ve un Depot bloqueado")
	}

	// El ejecutable nunca se abre en el navegador: se descarga con su nombre.
	exePath := strings.TrimPrefix(program.Files[0].URL, "/media/")
	req = httptest.NewRequest(http.MethodGet, "/media/"+exePath+"?name=vlc-3.0.21-win64.exe", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.gateMediaFile(s.media)(w, req)
	disposition := w.Header().Get("Content-Disposition")
	if w.Code != http.StatusOK || !strings.HasPrefix(disposition, "attachment;") ||
		!strings.Contains(disposition, `filename="vlc-3.0.21-win64.exe"`) {
		t.Fatalf("descarga del exe: %d %q", w.Code, disposition)
	}
	w = httptest.NewRecorder()
	s.gateMediaFile(s.media)(w, httptest.NewRequest(http.MethodGet, "/media/"+exePath, nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("un anónimo descargó de un Depot bloqueado: %d", w.Code)
	}
}

func TestAttachmentDispositionKeepsServedExtension(t *testing.T) {
	served := "Depot/General/studio-x-r2-file-001-abc.exe"
	if got := attachmentDisposition(served, "../../setup.exe"); strings.Contains(got, "..") {
		t.Fatalf("aceptó una ruta como nombre: %s", got)
	}
	if got := attachmentDisposition(served, "setup.bat"); strings.Contains(got, "setup.bat") {
		t.Fatalf("aceptó otra extensión: %s", got)
	}
	if got := attachmentDisposition(served, "Instalador ñ.exe"); !strings.Contains(got, `filename*=UTF-8''Instalador%20%C3%B1.exe`) {
		t.Fatalf("nombre UTF-8 mal codificado: %s", got)
	}
}
