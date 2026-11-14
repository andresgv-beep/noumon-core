package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMomentsDirectoryPickerListsFoldersWithoutRegisteringThem(t *testing.T) {
	s := testAuthServer(t, strings.Repeat("a", 32))
	m := &mediaDeps{root: t.TempDir()}
	root := t.TempDir()
	for _, name := range []string{"zeta", "Árbol", "Alpha", ".oculta"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "privado.txt"), []byte("no debe exponerse"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(root, "Alpha"), filepath.Join(root, "enlace"))
	handler := s.requireAdmin(s.handleMomentsDirectories(m))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/moments/directories?path="+url.QueryEscape(root), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 401 || strings.Contains(recorder.Body.String(), root) {
		t.Fatalf("sin admin: %d %s", recorder.Code, recorder.Body.String())
	}
	request.Header.Set("X-Noumon-Token", s.token)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatal(recorder.Body.String())
	}
	var result struct {
		Path        string           `json:"path"`
		Parent      string           `json:"parent"`
		Selectable  bool             `json:"selectable"`
		Directories []directoryEntry `json:"directories"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(root)
	if result.Path != canonical || result.Parent != filepath.Dir(canonical) || !result.Selectable {
		t.Fatalf("navegacion: %+v", result)
	}
	if len(result.Directories) != 3 || result.Directories[0].Name != "Alpha" || result.Directories[1].Name != "zeta" {
		t.Fatalf("carpetas: %+v", result.Directories)
	}
	for _, d := range result.Directories {
		if d.Path != filepath.Join(canonical, d.Name) {
			t.Fatalf("ruta incorrecta: %+v", d)
		}
	}
	var count int
	if err := s.store.db.QueryRow(`SELECT count(*) FROM moments_folders`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("el selector ha cambiado las fuentes: %d %v", count, err)
	}
}

func TestMomentsDirectoryPickerLocationsAndErrors(t *testing.T) {
	s := testAuthServer(t, strings.Repeat("a", 32))
	m := &mediaDeps{root: t.TempDir()}
	if err := os.Mkdir(filepath.Join(m.root, "Moments"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := s.requireAdmin(s.handleMomentsDirectories(m))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "", 200},
		{http.MethodGet, m.root, 200},
		{http.MethodGet, "relative/path", 400},
		{http.MethodGet, filepath.Join(m.root, "no-existe"), 400},
		{http.MethodPost, m.root, 405},
	} {
		request := httptest.NewRequest(tc.method, "/api/admin/moments/directories?path="+url.QueryEscape(tc.path), nil)
		request.Header.Set("X-Noumon-Token", s.token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != tc.status {
			t.Fatalf("%s %q: %d %s", tc.method, tc.path, recorder.Code, recorder.Body.String())
		}
		if tc.path == "" {
			var result struct {
				Locations  []directoryLocation `json:"locations"`
				Selectable bool                `json:"selectable"`
			}
			json.Unmarshal(recorder.Body.Bytes(), &result)
			if len(result.Locations) == 0 || result.Selectable {
				t.Fatalf("ubicaciones: %+v", result)
			}
			found := false
			for _, l := range result.Locations {
				if l.Path == filepath.Join(m.root, "Moments") {
					found = true
				}
			}
			if !found {
				t.Fatal("no hay acceso a la carpeta de Moments")
			}
		}
		if tc.path == m.root && tc.method == http.MethodGet {
			var result struct {
				Selectable bool   `json:"selectable"`
				Hint       string `json:"selectionHint"`
			}
			json.Unmarshal(recorder.Body.Bytes(), &result)
			if result.Selectable || result.Hint != "picker.overlap" {
				t.Fatalf("pool seleccionable: %+v", result)
			}
		}
	}
}
