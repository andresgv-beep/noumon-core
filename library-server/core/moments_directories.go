package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type directoryLocation struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	LabelKey string `json:"labelKey,omitempty"`
}

type directoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func momentsDirectoryLocations(m *mediaDeps) []directoryLocation {
	locations := []directoryLocation{}
	seen := map[string]bool{}
	add := func(path, name, label string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return
		}
		seen[path] = true
		locations = append(locations, directoryLocation{Path: path, Name: name, LabelKey: label})
	}
	for _, volume := range listStorageVolumes() {
		add(volume.Path, volume.Path, "picker.drive")
	}
	add(filepath.Join(m.root, "Moments"), "Moments", "picker.library")
	for _, pattern := range []string{"/media/*/*", "/run/media/*/*", "/mnt/*"} {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			add(path, filepath.Base(path), "")
		}
	}
	for _, f := range m.momentsFolders() {
		add(f.Path, f.Name, "")
	}
	return locations
}

// Ruta administrativa de solo lectura: enumera carpetas del servidor y nunca
// lee archivos. Seleccionar una ubicacion no la registra como fuente.
func (s *Server) handleMomentsDirectories(m *mediaDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeJSON(w, 405, map[string]string{"errorCode": "folders.invalid"})
			return
		}
		locations := momentsDirectoryLocations(m)
		raw := strings.TrimSpace(r.URL.Query().Get("path"))
		if raw == "" {
			writeJSON(w, 200, map[string]any{"path": "", "parent": "", "directories": []directoryEntry{}, "locations": locations, "selectable": false})
			return
		}
		if len(raw) > 8192 || !filepath.IsAbs(raw) {
			writeJSON(w, 400, map[string]string{"errorCode": "folders.absolute"})
			return
		}
		path, err := filepath.EvalSymlinks(filepath.Clean(raw))
		if err != nil {
			writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
			return
		}
		directories := []directoryEntry{}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			directories = append(directories, directoryEntry{Name: entry.Name(), Path: filepath.Join(path, entry.Name())})
		}
		sort.Slice(directories, func(i, j int) bool {
			return strings.ToLower(directories[i].Name) < strings.ToLower(directories[j].Name)
		})
		parent := filepath.Dir(path)
		if parent == path {
			parent = ""
		}
		hint := ""
		if parent == "" {
			hint = "picker.pickSubfolder"
		}
		if pathContains(m.root, path) || pathContains(path, m.root) {
			hint = "picker.overlap"
		}
		for _, f := range m.momentsFolders() {
			if pathContains(f.Path, path) || pathContains(path, f.Path) {
				hint = "picker.overlap"
				break
			}
		}
		writeJSON(w, 200, map[string]any{"path": path, "parent": parent, "directories": directories, "locations": locations, "selectable": hint == "", "selectionHint": hint})
	}
}
