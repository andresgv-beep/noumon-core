package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func importTestServer(t *testing.T) *Server {
	root := t.TempDir()
	return &Server{mediaRoot: root, media: &mediaDeps{root: root}}
}

func importTestUpload(s *Server, surface, path, data string, declared int) *httptest.ResponseRecorder {
	query := url.Values{"surface": {surface}, "path": {path}, "size": {strconv.Itoa(declared)}}
	r := httptest.NewRequest("POST", "/api/admin/import/upload?"+query.Encode(), strings.NewReader(data))
	w := httptest.NewRecorder()
	s.handleLocalImportUpload(w, r)
	return w
}

func importTestFinish(s *Server, surface string, files []string) *httptest.ResponseRecorder {
	data, _ := json.Marshal(map[string]any{"surface": surface, "files": files})
	r := httptest.NewRequest("POST", "/api/admin/import/finish", strings.NewReader(string(data)))
	w := httptest.NewRecorder()
	s.handleLocalImportFinish(w, r)
	return w
}

func TestLocalImportPreservesPublishedCardsAndAssociatedFiles(t *testing.T) {
	for _, surface := range []string{"cabinet", "moments", "depot"} {
		t.Run(surface, func(t *testing.T) {
			s := importTestServer(t)
			file, template := "book.pdf", "pdf"
			if surface == "moments" {
				file, template = "video.mp4", "video"
			}
			if surface == "depot" {
				file, template = "setup.exe", "program"
			}
			sc := sidecar{Template: template, Title: "Original title", Media: file, Source: surface, Cover: "cover.jpg", Description: "Keep this", Version: "1.2"}
			if surface == "depot" {
				sc.Files = []sidecarFile{{Media: file, Name: "Original.exe", OS: "win", Arch: "x64", SHA256: strings.Repeat("a", 64)}}
			}
			data, _ := json.Marshal(sc)
			content := map[string]string{file: "media bytes", "cover.jpg": "image bytes", "original.json": string(data)}
			var paths []string
			for name, body := range content {
				path := "Collection/" + name
				w := importTestUpload(s, surface, path, body, len(body))
				if w.Code != 201 {
					t.Fatalf("upload %s: %d %s", path, w.Code, w.Body)
				}
				paths = append(paths, path)
			}
			if w := importTestFinish(s, surface, paths); w.Code != 200 {
				t.Fatalf("finish: %s", w.Body)
			}
			items, err := s.media.scan("")
			if err != nil || len(items) != 1 || items[0].Title != sc.Title || items[0].Description != sc.Description || items[0].CoverURL == "" || items[0].Source != surface {
				t.Fatalf("items=%+v err=%v", items, err)
			}
			app, _ := appDirFor(surface)
			stored, _ := os.ReadFile(filepath.Join(s.mediaRoot, app, "Collection", "original.json"))
			if string(stored) != string(data) {
				t.Fatal("metadata was rewritten")
			}
		})
	}
}

func TestLocalImportDoesNotOverwriteAndRemovesIncompleteFiles(t *testing.T) {
	s := importTestServer(t)
	if w := importTestUpload(s, "cabinet", "Books/a.pdf", "original", 8); w.Code != 201 {
		t.Fatal(w.Body)
	}
	if w := importTestUpload(s, "cabinet", "Books/a.pdf", "replacement", 11); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := importTestUpload(s, "cabinet", "Books/b.pdf", "short", 99); w.Code != 400 {
		t.Fatal(w.Code)
	}
	stored, _ := os.ReadFile(filepath.Join(s.mediaRoot, "Cabinet/Books/a.pdf"))
	if string(stored) != "original" {
		t.Fatal("original changed")
	}
	entries, _ := os.ReadDir(filepath.Join(s.mediaRoot, "Cabinet/Books"))
	if len(entries) != 1 {
		t.Fatalf("incomplete files leaked: %v", entries)
	}
	if w := importTestFinish(s, "cabinet", []string{"Books/a.pdf"}); w.Code != 200 {
		t.Fatal(w.Body)
	}
	items, _ := s.media.scan("")
	if len(items) != 1 || items[0].Template != "pdf" {
		t.Fatalf("missing basic card: %+v", items)
	}
}

func TestLocalImportRejectsTraversalAndSymlinkEscape(t *testing.T) {
	s := importTestServer(t)
	for _, name := range []string{"../escape.pdf", "Books/../../escape.pdf", "/Books/escape.pdf", "Books\\escape.pdf", "Books/C:escape.pdf", "Books/.hidden/a.pdf"} {
		if w := importTestUpload(s, "cabinet", name, "file", 4); w.Code != 400 {
			t.Fatalf("accepted %s: %d", name, w.Code)
		}
	}
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(s.mediaRoot, "Cabinet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(s.mediaRoot, "Cabinet/escape")); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	w := importTestUpload(s, "cabinet", "escape/a.pdf", "file", 4)
	if w.Code < 400 {
		t.Fatal("followed symlink outside pool")
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 0 {
		t.Fatal("wrote outside pool")
	}
}

func TestLocalImportRejectsInvalidMetadata(t *testing.T) {
	s := importTestServer(t)
	for _, data := range []string{`{`, `{"source":"moments","media":"video.mp4"}`} {
		if w := importTestUpload(s, "cabinet", "Books/card.json", data, len(data)); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
}

func TestLocalImportInterruptedFolderDoesNotCreateDuplicateCards(t *testing.T) {
	s := importTestServer(t)
	if w := importTestUpload(s, "moments", "Channel/video.mkv", "video", 5); w.Code != 201 {
		t.Fatal(w.Body)
	}
	r := httptest.NewRequest("POST", "/api/admin/import/finish", strings.NewReader(`{"surface":"moments","files":["Channel/video.mkv"],"createCards":false}`))
	w := httptest.NewRecorder()
	s.handleLocalImportFinish(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if _, err := os.Stat(filepath.Join(s.mediaRoot, "Moments/Channel/video.json")); !os.IsNotExist(err) {
		t.Fatal("created a stored card before original metadata arrived")
	}
	metadata := `{"template":"video","title":"Original title","source":"moments","media":"video.mkv"}`
	if w := importTestUpload(s, "moments", "Channel/original.json", metadata, len(metadata)); w.Code != 201 {
		t.Fatal(w.Body)
	}
	if w := importTestFinish(s, "moments", []string{"Channel/video.mkv", "Channel/original.json"}); w.Code != 200 {
		t.Fatal(w.Body)
	}
	items, _ := s.media.scan("")
	if len(items) != 1 || items[0].Template != "video" || items[0].Title != "Original title" {
		t.Fatal("missing MKV import", items)
	}
}

func TestLocalImportRoutesRequireAdmin(t *testing.T) {
	s := testAuthServer(t, "")
	mux := http.NewServeMux()
	s.registerLocalImportRoutes(mux)
	handler := s.requireAdmin(mux)
	cookie := sessionFor(t, s, "reader", 30, false)
	for _, path := range []string{"/api/admin/import/upload", "/api/admin/import/finish"} {
		for _, authenticated := range []bool{false, true} {
			r := httptest.NewRequest("POST", path, nil)
			want := 401
			if authenticated {
				r.AddCookie(cookie)
				want = 403
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%s got %d want %d", path, w.Code, want)
			}
		}
	}
}

type interruptedImportReader struct{ sent bool }

func (r *interruptedImportReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.ErrUnexpectedEOF
	}
	r.sent = true
	return copy(p, "part"), nil
}
func TestLocalImportDisconnectCleansTemporaryFile(t *testing.T) {
	s := importTestServer(t)
	r := httptest.NewRequest("POST", "/api/admin/import/upload?surface=moments&path=Channel/video.mp4&size=100", &interruptedImportReader{})
	w := httptest.NewRecorder()
	s.handleLocalImportUpload(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	entries, _ := os.ReadDir(filepath.Join(s.mediaRoot, "Moments/Channel"))
	if len(entries) != 0 {
		t.Fatal("partial file left behind")
	}
}
