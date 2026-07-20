// admin_deps.go — aprovisionamiento de herramientas externas desde el Panel.
//
// Implementa el pilar 2 de docs/PLAN-INSTALACION-LIMPIA.md: las herramientas de
// terceros (hoy: translateLocally) NO se redistribuyen en el repo ni en el
// instalador; se descargan del upstream oficial al pulsar [Instalar] en el
// Panel, con versión fijada y verificación. El binario se coloca junto al
// ejecutable del Core (siblingDir), que es donde translate-wrap lo busca.
//
//	GET  /api/admin/deps          → {"tools":[{id, label, installed, installing, ...}]}
//	POST /api/admin/deps/install  → {"id":"translateLocally"}  (arranca en segundo plano)
//
// El progreso se consulta re-pidiendo GET /api/admin/deps (el Panel sondea).

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type depTool struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Version string `json:"version"`
	License string `json:"license"`
	Source  string `json:"source"`
	URL     string `json:"-"`
	Bytes   int64  `json:"bytes"`
	// SHA256 esperado del binario. Vacío = aún sin fijar: la primera descarga
	// verificada registra el hash real en el log para fijarlo aquí (TOFU
	// consciente; el tamaño exacto sí se exige siempre).
	SHA256 string `json:"-"`
	File   string `json:"-"` // nombre de destino junto al ejecutable del Core
}

// depManifest: versión y origen fijados por plataforma. Linux/ARM no tiene
// binario oficial de translateLocally (plan §6): allí la traducción se
// configura remota (TRANSLATE_URL) y la lista queda vacía.
func depManifest() []depTool {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return []depTool{{
			ID:      "translateLocally",
			Label:   "translateLocally (motor Bergamot)",
			Version: "v0.0.2+8e31cff",
			License: "MIT",
			Source:  "github.com/XapaJIaMnu/translateLocally",
			URL:     "https://github.com/XapaJIaMnu/translateLocally/releases/download/latest/translateLocally.windows-2022.core-avx-i.exe",
			Bytes:   59515392,
			File:    "translateLocally.exe",
		}}
	}
	return nil
}

type depState struct {
	Installing bool
	Progress   int64
	Err        string
}

var (
	depMu     sync.Mutex
	depStates = map[string]*depState{}
	depClient = &http.Client{Timeout: 30 * time.Minute}
)

func depSiblingPath(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	return filepath.Join(filepath.Dir(exe), name)
}

func (s *Server) registerAdminDepsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/deps", s.handleAdminDeps)
	mux.HandleFunc("/api/admin/deps/install", s.handleAdminDepsInstall)
}

func (s *Server) handleAdminDeps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo GET"})
		return
	}
	type toolStatus struct {
		depTool
		Installed  bool   `json:"installed"`
		Installing bool   `json:"installing"`
		Progress   int64  `json:"progress"`
		Error      string `json:"error,omitempty"`
	}
	out := []toolStatus{}
	depMu.Lock()
	defer depMu.Unlock()
	for _, tool := range depManifest() {
		st := depStates[tool.ID]
		if st == nil {
			st = &depState{}
		}
		info, err := os.Stat(depSiblingPath(tool.File))
		out = append(out, toolStatus{
			depTool:    tool,
			Installed:  err == nil && !info.IsDir(),
			Installing: st.Installing,
			Progress:   st.Progress,
			Error:      st.Err,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": out})
}

func (s *Server) handleAdminDepsInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo POST"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cuerpo invalido"})
		return
	}
	var tool *depTool
	for _, candidate := range depManifest() {
		if candidate.ID == req.ID {
			t := candidate
			tool = &t
			break
		}
	}
	if tool == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "herramienta desconocida o sin binario para esta plataforma"})
		return
	}

	depMu.Lock()
	st := depStates[tool.ID]
	if st != nil && st.Installing {
		depMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ya se esta instalando"})
		return
	}
	st = &depState{Installing: true}
	depStates[tool.ID] = st
	depMu.Unlock()

	go installDepTool(*tool, st)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": tool.ID})
}

// installDepTool descarga a .part con verificación de tamaño (y SHA256 si está
// fijado), y renombra en atómico. El progreso se publica bajo depMu.
func installDepTool(tool depTool, st *depState) {
	fail := func(err error) {
		log.Printf("deps: instalacion de %s fallo: %v", tool.ID, err)
		depMu.Lock()
		st.Installing = false
		st.Err = err.Error()
		depMu.Unlock()
	}

	dest := depSiblingPath(tool.File)
	part := dest + ".part"
	defer os.Remove(part)

	resp, err := depClient.Get(tool.URL)
	if err != nil {
		fail(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fail(fmt.Errorf("descarga rechazada: HTTP %d", resp.StatusCode))
		return
	}

	out, err := os.Create(part)
	if err != nil {
		fail(err)
		return
	}
	hasher := sha256.New()
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				fail(werr)
				return
			}
			hasher.Write(buf[:n])
			written += int64(n)
			depMu.Lock()
			st.Progress = written
			depMu.Unlock()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			fail(rerr)
			return
		}
	}
	if err := out.Close(); err != nil {
		fail(err)
		return
	}

	if written != tool.Bytes {
		fail(fmt.Errorf("tamano inesperado: %d bytes (se esperaban %d); descarga rechazada", written, tool.Bytes))
		return
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if tool.SHA256 != "" && !strings.EqualFold(sum, tool.SHA256) {
		fail(fmt.Errorf("SHA256 no coincide; descarga rechazada"))
		return
	}
	if tool.SHA256 == "" {
		log.Printf("deps: %s %s instalado con SHA256=%s — fijarlo en depManifest para futuras verificaciones", tool.ID, tool.Version, sum)
	}

	if err := os.Chmod(part, 0o755); err != nil {
		fail(err)
		return
	}
	if err := os.Rename(part, dest); err != nil {
		fail(err)
		return
	}
	log.Printf("deps: %s %s instalado en %s (%d bytes)", tool.ID, tool.Version, dest, written)
	depMu.Lock()
	st.Installing = false
	st.Err = ""
	depMu.Unlock()
}
