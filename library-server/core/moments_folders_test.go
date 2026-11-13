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

func writeLooseVideo(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLooseMomentsPreservesPublishedMetadata(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Moments/Canal", "curated.mp4", "video", sidecar{Template: "video", Source: "moments", Title: "Titulo editorial", Author: "Creador", Description: "Descripcion", ChannelAvatar: "logo.jpg"})
	writeLooseVideo(t, root, "Moments/Canal/mi_video-ñ.MP4")
	writeLooseVideo(t, root, "Moments/Canal/mi_video-ñ.webm")
	writeLooseVideo(t, root, "Cabinet/no-importar.mp4")
	m := &mediaDeps{root: root}
	items, err := m.scan("")
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	ids := map[string]bool{}
	for _, it := range items {
		id := mediaToItem(it).ID
		if ids[id] {
			t.Fatal("ID duplicado")
		}
		ids[id] = true
		if it.Media == "curated.mp4" {
			if it.AutoIndexed || it.Title != "Titulo editorial" || it.Author != "Creador" || it.Description != "Descripcion" || it.ChannelAvatarURL == "" {
				t.Fatalf("se perdieron metadatos: %+v", it)
			}
		} else if !it.AutoIndexed || it.Title != "mi video ñ" || it.Source != "moments" {
			t.Fatalf("video automatico: %+v", it)
		}
	}
	filtered, err := m.itemsFor("Moments/Canal")
	if err != nil || len(filtered) != 3 {
		t.Fatalf("filtro: %d %v", len(filtered), err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "Moments/Canal"))
	if len(entries) != 5 {
		t.Fatalf("el escaneo escribio archivos: %d", len(entries))
	}
}

func TestMomentsExternalFolderLifecycleAndRange(t *testing.T) {
	s := testAuthServer(t, strings.Repeat("a", 32))
	m := &mediaDeps{root: t.TempDir()}
	s.media = m
	external := t.TempDir()
	video := writeLooseVideo(t, external, "Subcarpeta/Árbol #1.mp4")
	writeLooseVideo(t, external, "secreto.txt")
	body, _ := json.Marshal(map[string]string{"path": external, "name": "Disco de videos"})
	handler := s.requireAdmin(s.handleMomentsFolders(m))
	request := httptest.NewRequest(http.MethodPost, "/api/admin/moments/folders", strings.NewReader(string(body)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 401 {
		t.Fatalf("alta sin admin: %d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/admin/moments/folders", strings.NewReader(string(body)))
	request.Header.Set("X-Noumon-Token", s.token)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 201 {
		t.Fatalf("alta: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	json.Unmarshal(recorder.Body.Bytes(), &response)
	loaded := &mediaDeps{root: m.root}
	if err := s.loadMomentsFolders(loaded); err != nil || len(loaded.momentsFolders()) != 1 {
		t.Fatalf("persistencia: %v", err)
	}
	items, err := m.itemsFor("")
	if err != nil || len(items) != 1 {
		t.Fatalf("catalogo externo: %+v %v", items, err)
	}
	it := items[0]
	if m.collectionMetadata(it.Collection).Title != "Disco de videos" {
		t.Fatal("nombre de coleccion perdido")
	}
	// Los bytes pasan por la misma cerradura de Moments que los publicados.
	stream := s.gateMediaFile(m)
	r := httptest.NewRequest(http.MethodGet, it.MediaURL, nil)
	r.Header.Set("Range", "bytes=2-5")
	recorder = httptest.NewRecorder()
	stream(recorder, r)
	if recorder.Code != 403 {
		t.Fatalf("stream anonimo: %d", recorder.Code)
	}
	_, err = s.store.db.Exec(`INSERT INTO collection_access(collection_id,access,min_age,allow_download,updated) VALUES(?,?,0,0,0)`, surfaceAccessMoments, "open")
	if err != nil {
		t.Fatal(err)
	}
	s.invalidateAccessCache()
	recorder = httptest.NewRecorder()
	stream(recorder, r)
	if recorder.Code != 206 || recorder.Body.String() != "2345" {
		t.Fatalf("Range: %d %q", recorder.Code, recorder.Body.String())
	}
	for _, rel := range []string{momentsFolderPrefix + response.ID + "/secreto.txt", momentsFolderPrefix + response.ID + "/../../escape.mp4"} {
		if _, err := m.safeResolve(rel); err == nil {
			t.Fatalf("ruta externa permitida: %s", rel)
		}
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/moments/folders?id="+url.QueryEscape(response.ID), nil)
	request.Header.Set("X-Noumon-Token", s.token)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatal(recorder.Body.String())
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatal("se borro el video original")
	}
	if items, err = m.itemsFor(""); err != nil || len(items) != 0 {
		t.Fatalf("catalogo tras desconectar: %+v %v", items, err)
	}
	decoded, _ := url.PathUnescape(strings.TrimPrefix(it.MediaURL, "/media/"))
	if _, err := m.safeResolve(decoded); err == nil {
		t.Fatal("la carpeta desconectada aun sirve videos")
	}
}

func TestMomentsScannerRefreshRemovalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	m := &mediaDeps{root: root, folders: []momentsFolder{{ID: "test", Path: external, Name: "Videos"}}}
	video := writeLooseVideo(t, external, "primero.mp4")
	outside := writeLooseVideo(t, t.TempDir(), "privado.mp4")
	if err := os.Symlink(outside, filepath.Join(external, "enlace.mp4")); err == nil {
		if _, err := m.safeResolve(momentsFolderPrefix + "test/enlace.mp4"); err == nil {
			t.Fatal("se siguio enlace")
		}
	}
	items, err := m.itemsFor("")
	if err != nil || len(items) != 1 {
		t.Fatalf("escaneo inicial: %d %v", len(items), err)
	}
	if err := os.Remove(video); err != nil {
		t.Fatal(err)
	}
	writeLooseVideo(t, external, "segundo.webm")
	m.invalidate()
	items, err = m.itemsFor("")
	if err != nil || len(items) != 1 || items[0].Title != "segundo" {
		t.Fatalf("nuevo escaneo: %+v %v", items, err)
	}
	if err := os.Rename(external, external+"-offline"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(external+"-offline", external) })
	m.invalidate()
	items, err = m.itemsFor("")
	if err != nil || len(items) != 0 {
		t.Fatalf("disco desconectado: %+v %v", items, err)
	}
}
