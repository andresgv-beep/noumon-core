package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Administrative copy of published files, not an alternative Studio editor.
// File bodies are streamed to disk; metadata is transferred after the media.
func (s *Server) registerLocalImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/import/upload", s.handleLocalImportUpload)
	mux.HandleFunc("/api/admin/import/finish", s.handleLocalImportFinish)
}

func localImportPath(surface, relative string) (string, error) {
	app, ok := appDirFor(surface)
	if !ok || len(relative) > 2048 || strings.ContainsAny(relative, "\\:\x00\r\n") || path.Clean(relative) != relative {
		return "", errors.New("invalid path")
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 2 || len(parts) > 32 {
		return "", errors.New("collection required")
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || len(part) > 240 || strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") {
			return "", errors.New("invalid component")
		}
	}
	return filepath.FromSlash(app + "/" + relative), nil
}

func localImportError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"errorCode": "import.local." + code})
}

// Link is an atomic no-replace commit. Filesystems without hardlinks use an
// exclusive destination and streaming copy instead; existing files never change.
func commitLocalImport(root *os.Root, temporary, destination string) error {
	if err := root.Link(temporary, destination); err == nil {
		return nil
	} else if errors.Is(err, fs.ErrExist) {
		return err
	}
	out, err := root.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	in, err := root.Open(temporary)
	if err != nil {
		out.Close()
		root.Remove(destination)
		return err
	}
	_, err = io.Copy(out, in)
	in.Close()
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		root.Remove(destination)
	}
	return err
}

func (s *Server) handleLocalImportUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		localImportError(w, 405, "method")
		return
	}
	surface := r.URL.Query().Get("surface")
	destination, err := localImportPath(surface, r.URL.Query().Get("path"))
	size, sizeErr := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
	if err != nil || sizeErr != nil || size < 0 || size > 1<<40 {
		localImportError(w, 400, "invalidPath")
		return
	}
	if strings.EqualFold(filepath.Ext(destination), ".json") && size > 2<<20 {
		localImportError(w, 400, "invalidMetadata")
		return
	}
	if s.media == nil || s.mediaRoot == "" {
		localImportError(w, 503, "unavailable")
		return
	}
	root, err := os.OpenRoot(s.mediaRoot)
	if err != nil {
		localImportError(w, 500, "writeFailed")
		return
	}
	defer root.Close()
	if _, err = root.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		localImportError(w, 409, "exists")
		return
	}
	if ok, err := studioHasDiskHeadroom(s.mediaRoot); err != nil || !ok {
		localImportError(w, 507, "diskFull")
		return
	}
	if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		localImportError(w, 400, "invalidPath")
		return
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		localImportError(w, 500, "writeFailed")
		return
	}
	temporary := filepath.Join(filepath.Dir(destination), ".import-"+hex.EncodeToString(nonce)+".part")
	out, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		localImportError(w, 500, "writeFailed")
		return
	}
	defer root.Remove(temporary)
	defer out.Close()
	r.Body = http.MaxBytesReader(w, r.Body, size+1)
	buffer := make([]byte, 64<<10)
	hash := sha256.New()
	var written, checked int64
	for {
		if r.Context().Err() != nil {
			return
		}
		n, readErr := r.Body.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > size {
				localImportError(w, 400, "incomplete")
				return
			}
			if _, err = out.Write(buffer[:n]); err != nil {
				localImportError(w, 507, "writeFailed")
				return
			}
			hash.Write(buffer[:n])
			if written-checked >= 8<<20 {
				checked = written
				if ok, err := studioHasDiskHeadroom(s.mediaRoot); err != nil || !ok {
					localImportError(w, 507, "diskFull")
					return
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			localImportError(w, 400, "incomplete")
			return
		}
	}
	if written != size {
		localImportError(w, 400, "incomplete")
		return
	}
	if err := out.Sync(); err != nil {
		localImportError(w, 507, "writeFailed")
		return
	}
	if err := out.Close(); err != nil {
		localImportError(w, 500, "writeFailed")
		return
	}
	if strings.EqualFold(filepath.Ext(destination), ".json") {
		data, err := root.ReadFile(temporary)
		if err != nil || !json.Valid(data) {
			localImportError(w, 400, "invalidMetadata")
			return
		}
		var sc sidecar
		_ = json.Unmarshal(data, &sc)
		if sc.Media != "" && sc.Source != surface {
			localImportError(w, 400, "wrongSurface")
			return
		}
	}
	if err := commitLocalImport(root, temporary, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			localImportError(w, 409, "exists")
		} else {
			localImportError(w, 500, "writeFailed")
		}
		return
	}
	s.media.invalidate()
	writeJSON(w, 201, map[string]any{"bytes": written, "sha256": hex.EncodeToString(hash.Sum(nil))})
}

func (s *Server) handleLocalImportFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		localImportError(w, 405, "method")
		return
	}
	var input struct {
		Surface     string   `json:"surface"`
		Files       []string `json:"files"`
		CreateCards *bool    `json:"createCards"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&input); err != nil || len(input.Files) > 10000 {
		localImportError(w, 400, "invalidPath")
		return
	}
	if s.media == nil || s.mediaRoot == "" {
		localImportError(w, 503, "unavailable")
		return
	}
	root, err := os.OpenRoot(s.mediaRoot)
	if err != nil {
		localImportError(w, 500, "writeFailed")
		return
	}
	defer root.Close()
	var files []string
	directories := map[string]bool{}
	for _, relative := range input.Files {
		file, err := localImportPath(input.Surface, relative)
		if err != nil {
			localImportError(w, 400, "invalidPath")
			return
		}
		info, err := root.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			localImportError(w, 400, "invalidPath")
			return
		}
		files = append(files, file)
		directories[filepath.Dir(file)] = true
	}
	if input.CreateCards != nil && !*input.CreateCards {
		s.media.invalidate()
		writeJSON(w, 200, map[string]any{"created": 0})
		return
	}
	// Existing sidecars win. Covers, screenshots and tracks are not separate items.
	represented := map[string]bool{}
	for dir := range directories {
		folder, err := root.Open(dir)
		if err != nil {
			localImportError(w, 500, "writeFailed")
			return
		}
		entries, err := folder.ReadDir(-1)
		folder.Close()
		if err != nil {
			localImportError(w, 500, "writeFailed")
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
				continue
			}
			raw, err := root.ReadFile(filepath.Join(dir, entry.Name()))
			var sc sidecar
			if err != nil || json.Unmarshal(raw, &sc) != nil {
				continue
			}
			refs := []string{sc.Media, sc.Cover, sc.Text, sc.ChannelAvatar}
			for _, file := range sc.Files {
				refs = append(refs, file.Media)
			}
			for _, shot := range sc.Screenshots {
				refs = append(refs, shot.File)
			}
			for _, track := range sc.Tracks {
				refs = append(refs, track.Media, track.Waveform)
			}
			for _, sub := range sc.Subtitles {
				refs = append(refs, sub.File)
			}
			for _, ref := range refs {
				if ref != "" {
					represented[filepath.Clean(filepath.Join(dir, ref))] = true
				}
			}
		}
	}
	created := 0
	for _, file := range files {
		if represented[file] {
			continue
		}
		template, _ := templateForExt(file)
		if input.Surface == "moments" && isMomentsVideo(file) {
			template = "video"
		}
		if input.Surface == "moments" && template != "video" {
			continue
		}
		var packages []sidecarFile
		if input.Surface == "depot" {
			if !isPackageFile(file) {
				continue
			}
			template = "program"
			info, err := root.Stat(file)
			if err != nil {
				localImportError(w, 500, "writeFailed")
				return
			}
			reader, err := root.Open(file)
			if err != nil {
				localImportError(w, 500, "writeFailed")
				return
			}
			hash := sha256.New()
			_, err = io.Copy(hash, reader)
			reader.Close()
			if err != nil {
				localImportError(w, 500, "writeFailed")
				return
			}
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
			osName := ""
			if format == "exe" || format == "msi" {
				osName = "win"
			} else if format == "deb" {
				osName = "linux"
			}
			packages = []sidecarFile{{Media: filepath.Base(file), Name: filepath.Base(file), Format: format, OS: osName, Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}}
		}
		if template == "" {
			continue
		}
		sc := sidecar{Template: template, Title: titleFromFilename(file), Media: filepath.Base(file), Source: input.Surface, Files: packages}
		destination := sidecarPathFor(file)
		data, _ := json.MarshalIndent(sc, "", "  ")
		writer, err := root.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			localImportError(w, 500, "writeFailed")
			return
		}
		_, writeErr := fmt.Fprintln(writer, string(data))
		closeErr := writer.Close()
		if writeErr != nil || closeErr != nil {
			root.Remove(destination)
			localImportError(w, 500, "writeFailed")
			return
		}
		created++
	}
	s.media.invalidate()
	writeJSON(w, 200, map[string]any{"created": created})
}
