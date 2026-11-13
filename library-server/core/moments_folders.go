package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const momentsFolderPrefix = "Moments/@folders/"
const looseMomentsSuffix = ".noumon-auto-video"

type momentsFolder struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

func (m *mediaDeps) momentsFolders() []momentsFolder {
	m.foldersMu.RLock()
	defer m.foldersMu.RUnlock()
	return append([]momentsFolder{}, m.folders...)
}

func (s *Server) loadMomentsFolders(m *mediaDeps) error {
	rows, err := s.store.db.Query(`SELECT id, path, name FROM moments_folders ORDER BY name, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	folders := []momentsFolder{}
	for rows.Next() {
		var f momentsFolder
		if err := rows.Scan(&f.ID, &f.Path, &f.Name); err != nil {
			return err
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	m.foldersMu.Lock()
	m.folders = folders
	m.foldersMu.Unlock()
	m.invalidate()
	return nil
}

func (m *mediaDeps) momentsFolderForRel(rel string) (momentsFolder, string, bool) {
	if !strings.HasPrefix(rel, momentsFolderPrefix) {
		return momentsFolder{}, "", false
	}
	id, tail, ok := strings.Cut(strings.TrimPrefix(rel, momentsFolderPrefix), "/")
	if !ok || id == "" {
		return momentsFolder{}, "", false
	}
	for _, f := range m.momentsFolders() {
		if f.ID == id {
			return f, tail, true
		}
	}
	return momentsFolder{}, "", false
}

func isMomentsVideo(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".webm", ".mov", ".mkv", ".avi", ".ogv", ".mpg", ".mpeg", ".ts", ".mts", ".m2ts":
		return true
	}
	return false
}

// Los discos adicionales son de solo lectura y solo exponen videos. No se
// siguen enlaces ni se sirven otros archivos de esas carpetas por /media.
func (m *mediaDeps) resolveMomentsVideo(rel string) (string, error) {
	folder, tail, ok := m.momentsFolderForRel(rel)
	if !ok || !isMomentsVideo(tail) {
		return "", fmt.Errorf("video no encontrado")
	}
	// Reutilizar el anclaje conservador del pool para cada raiz autorizada.
	base := &mediaDeps{root: folder.Path}
	resolved, err := base.safeResolve(tail)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(folder.Path)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("carpeta no disponible")
	}
	canonical, err := filepath.EvalSymlinks(folder.Path)
	if err != nil || canonical != folder.Path {
		return "", fmt.Errorf("la ubicacion de la carpeta ha cambiado")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("video no disponible")
	}
	return resolved, nil
}

// Escaneo virtual: no modifica ni copia el video, ni crea fichas junto a el.
// Los sidecars existentes del pool conservan prioridad sobre esta proyeccion.
func (m *mediaDeps) scanLooseMoments(represented map[string]bool) []mediaItem {
	type scanRoot struct{ path, prefix string }
	roots := []scanRoot{{filepath.Join(m.root, "Moments"), "Moments"}}
	for _, f := range m.momentsFolders() {
		roots = append(roots, scanRoot{f.Path, momentsFolderPrefix + f.ID})
	}
	var out []mediaItem
	for _, root := range roots {
		_ = filepath.WalkDir(root.path, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if path != root.path && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !isMomentsVideo(path) || represented[filepath.Clean(path)] {
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return nil
			}
			rel, err := filepath.Rel(root.path, path)
			if err != nil {
				return nil
			}
			virtual := root.prefix + "/" + filepath.ToSlash(rel)
			// Comprobar tambien los componentes padre antes de catalogar.
			if _, err := m.safeResolve(virtual); err != nil {
				return nil
			}
			out = append(out, mediaItem{
				ID: virtual + looseMomentsSuffix, AutoIndexed: true,
				Collection: filepath.ToSlash(filepath.Dir(virtual)),
				Template:   "video", Source: "moments", Title: titleFromFilename(path),
				Media: d.Name(), MediaURL: "/media/" + mediaURLPath(virtual), Date: info.ModTime().Format("2006-01-02"),
			})
			return nil
		})
	}
	return out
}

func pathContains(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *Server) handleMomentsFolders(m *mediaDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			m.foldersConfigMu.Lock()
			defer m.foldersConfigMu.Unlock()
		}
		switch r.Method {
		case http.MethodGet:
			catalog, err := m.catalogSnapshot()
			if err != nil {
				writeJSON(w, 500, map[string]string{"errorCode": "folders.scanFail"})
				return
			}
			type status struct {
				momentsFolder
				Available bool `json:"available"`
				Count     int  `json:"count"`
			}
			folders := []status{}
			for _, f := range m.momentsFolders() {
				st := status{momentsFolder: f}
				if dir, err := os.Open(f.Path); err == nil {
					info, statErr := dir.Stat()
					st.Available = statErr == nil && info.IsDir()
					dir.Close()
				}
				for _, it := range catalog.items {
					if strings.HasPrefix(it.ID, momentsFolderPrefix+f.ID+"/") {
						st.Count++
					}
				}
				folders = append(folders, st)
			}
			writeJSON(w, 200, map[string]any{"defaultPath": filepath.Join(m.root, "Moments"), "folders": folders, "intervalSeconds": int(mediaCatalogTTL.Seconds())})
		case http.MethodPost:
			var input struct {
				Path string `json:"path"`
				Name string `json:"name"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input) != nil {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.invalid"})
				return
			}
			root := strings.TrimSpace(input.Path)
			if !filepath.IsAbs(root) {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.absolute"})
				return
			}
			root, err := filepath.EvalSymlinks(root)
			if err != nil {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
				return
			}
			root = filepath.Clean(root)
			if filepath.Dir(root) == root {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.diskRoot"})
				return
			}
			dir, err := os.Open(root)
			if err != nil {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
				return
			}
			info, err := dir.Stat()
			if err != nil || !info.IsDir() {
				dir.Close()
				writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
				return
			}
			_, err = dir.ReadDir(1)
			dir.Close()
			if err != nil && err != io.EOF {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.unreadable"})
				return
			}
			if pathContains(m.root, root) || pathContains(root, m.root) {
				writeJSON(w, 409, map[string]string{"errorCode": "folders.overlap"})
				return
			}
			for _, f := range m.momentsFolders() {
				if pathContains(f.Path, root) || pathContains(root, f.Path) {
					writeJSON(w, 409, map[string]string{"errorCode": "folders.overlap"})
					return
				}
			}
			name := strings.TrimSpace(input.Name)
			if name == "" {
				name = filepath.Base(root)
			}
			if len(name) > 200 {
				writeJSON(w, 400, map[string]string{"errorCode": "folders.invalid"})
				return
			}
			var random [16]byte
			if _, err := rand.Read(random[:]); err != nil {
				writeJSON(w, 500, map[string]string{"errorCode": "folders.saveFail"})
				return
			}
			id := hex.EncodeToString(random[:])
			if _, err := s.store.db.Exec(`INSERT INTO moments_folders(id,path,name) VALUES(?,?,?)`, id, root, name); err != nil {
				writeJSON(w, 409, map[string]string{"errorCode": "folders.saveFail"})
				return
			}
			if err := s.loadMomentsFolders(m); err != nil {
				writeJSON(w, 500, map[string]string{"errorCode": "folders.saveFail"})
				return
			}
			writeJSON(w, 201, map[string]string{"id": id})
		case http.MethodDelete:
			if _, err := s.store.db.Exec(`DELETE FROM moments_folders WHERE id=?`, r.URL.Query().Get("id")); err != nil {
				writeJSON(w, 500, map[string]string{"errorCode": "folders.saveFail"})
				return
			}
			if err := s.loadMomentsFolders(m); err != nil {
				writeJSON(w, 500, map[string]string{"errorCode": "folders.saveFail"})
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		default:
			w.Header().Set("Allow", "GET, POST, DELETE")
			writeJSON(w, 405, map[string]string{"errorCode": "folders.invalid"})
		}
	}
}

func (s *Server) handleMomentsRescan(m *mediaDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeJSON(w, 405, map[string]string{"errorCode": "folders.invalid"})
			return
		}
		m.invalidate()
		if _, err := m.catalogSnapshot(); err != nil {
			writeJSON(w, 500, map[string]string{"errorCode": "folders.scanFail"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
