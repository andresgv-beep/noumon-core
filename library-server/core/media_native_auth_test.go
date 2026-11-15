package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNativePDFAndCoverUseMediaTokenWithoutCookies(t *testing.T) {
	s := testAuthServer(t, "")
	if _, err := s.createUser("native-pdf", "clave-segura!", 30, true); err != nil {
		t.Fatal(err)
	}
	session, err := s.newSession("native-pdf")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	seedCollection(t, root, "Cabinet/manual", "manual.pdf", "%PDF-1.4 test", sidecar{Template: "pdf", Source: "cabinet", Title: "Manual", Cover: "cover.png"})
	if err := os.WriteFile(filepath.Join(root, "Cabinet/manual/cover.png"), []byte("image bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &mediaDeps{root: root}
	s.media = m
	issue := httptest.NewRequest(http.MethodPost, "/api/auth/media-token", nil)
	issue.Header.Set("Authorization", "Bearer "+session)
	w := httptest.NewRecorder()
	s.handleMediaToken(w, issue)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Token == "" {
		t.Fatalf("token no emitido: %v", err)
	}
	for _, file := range []string{"manual.pdf", "cover.png"} {
		path := "/media/Cabinet/manual/" + file
		for _, tc := range []struct {
			query  string
			status int
		}{
			{"", 403}, {"?st=" + session, 403}, {"?st=" + result.Token, 206},
		} {
			r := httptest.NewRequest(http.MethodGet, path+tc.query, nil)
			r.Header.Set("Range", "bytes=0-3")
			w = httptest.NewRecorder()
			s.gateMediaFile(m)(w, r)
			if w.Code != tc.status {
				t.Fatalf("%s: estado=%d esperado=%d", file, w.Code, tc.status)
			}
			if w.Code == 206 && len(w.Body.Bytes()) != 4 {
				t.Fatal("rango incompleto")
			}
		}
	}
	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logout.Header.Set("Authorization", "Bearer "+session)
	s.handleLogout(httptest.NewRecorder(), logout)
	w = httptest.NewRecorder()
	s.gateMediaFile(m)(w, httptest.NewRequest(http.MethodGet, "/media/Cabinet/manual/manual.pdf?st="+result.Token, nil))
	if w.Code != 403 {
		t.Fatalf("el token multimedia sigue vivo tras cerrar sesion: %d", w.Code)
	}
}
