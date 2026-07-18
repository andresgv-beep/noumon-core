// zim_fts_index.go — construir el índice full-text de un ZIM bajo demanda desde el
// Panel (INDEXER.md §4). Espejo del indexado de calles de Maps (maps_street_index.go):
// un job en 2º plano, con progreso, cancelable, y un modo "indexar todos" (cola).
// El motor (fts.Build, ya probado y con build atómico/reconcile) hace el trabajo;
// aquí solo se cablea el job + el estado que pinta el Panel.
//
// Se llavea por FICHERO (.zim), no por el id del <book> del library.xml: así se
// evita la dualidad id-público vs UUID que ya rompió el trust interactivo.
package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/andresgv-beep/zim-engine/fts"
)

type ftsIndexJob struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Status  string `json:"status"` // indexing | done | error | cancelled
	Scanned int    `json:"scanned"`
	Indexed int    `json:"indexed"`
	Total   int    `json:"total"`
	Error   string `json:"error,omitempty"`
}

// zimHasIndex: hay índice utilizable si existe su manifiesto (la verificación real
// contra el ZIM la hace fts.Open al buscar). Barato, como el os.Stat de Maps.
func zimHasIndex(zimPath string) bool {
	st, err := os.Stat(filepath.Join(ftsDirFor(zimPath), fts.ManifestName))
	return err == nil && !st.IsDir()
}

// indexJobCopy: snapshot del job para el listado del Panel (o nil si no hay).
func (n *nativeZims) indexJobCopy() *ftsIndexJob {
	n.idxMu.Lock()
	defer n.idxMu.Unlock()
	if n.idxJob == nil {
		return nil
	}
	c := *n.idxJob
	return &c
}

func (n *nativeZims) zimTitle(file string) string {
	if books, err := n.az.readLibrary(); err == nil {
		for _, b := range books {
			if filepath.Base(b.Path) == file {
				if b.Title != "" {
					return b.Title
				}
				return b.Name
			}
		}
	}
	return strings.TrimSuffix(file, filepath.Ext(file))
}

// validIndexFile: nombre simple .zim, sin rutas (anti-traversal), y que exista.
func (n *nativeZims) validIndexFile(file string) bool {
	if file == "" || filepath.Base(file) != file || !strings.HasSuffix(strings.ToLower(file), ".zim") {
		return false
	}
	st, err := os.Stat(filepath.Join(n.az.zimDir, file))
	return err == nil && !st.IsDir()
}

// startIndex arranca el job de un fichero. Devuelve (arrancado, mensaje de error).
func (n *nativeZims) startIndex(file string) (bool, string) {
	na, err := n.get(strings.TrimSuffix(file, filepath.Ext(file)))
	if err != nil {
		return false, "colección no encontrada"
	}
	n.idxMu.Lock()
	if n.idxJob != nil && n.idxJob.Status == "indexing" {
		n.idxMu.Unlock()
		return false, "ya hay un índice construyéndose"
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.idxCancel = cancel
	n.idxJob = &ftsIndexJob{File: file, Name: n.zimTitle(file), Status: "indexing", Total: int(na.arc.EntryCount())}
	n.idxMu.Unlock()
	go n.runIndex(ctx, file, na)
	return true, ""
}

func (n *nativeZims) runIndex(ctx context.Context, file string, na nativeArchive) {
	lang, _ := na.arc.Metadata("Language")
	_, err := fts.Build(ctx, na.arc, ftsDirFor(na.path), fts.BuildOptions{
		Language: lang,
		OnProgress: func(p fts.Progress) {
			n.idxMu.Lock()
			if n.idxJob != nil && n.idxJob.File == file {
				n.idxJob.Scanned, n.idxJob.Indexed, n.idxJob.Total = int(p.Scanned), p.Indexed, int(p.Total)
			}
			n.idxMu.Unlock()
		},
	})

	cancelled := ctx.Err() != nil
	n.idxMu.Lock()
	if n.idxJob != nil && n.idxJob.File == file {
		switch {
		case cancelled:
			n.idxJob.Status, n.idxJob.Error = "cancelled", "indexación cancelada"
		case err != nil:
			n.idxJob.Status, n.idxJob.Error = "error", err.Error()
		default:
			n.idxJob.Status = "done"
		}
	}
	n.idxMu.Unlock()

	if err == nil && !cancelled {
		// El índice recién construido debe "aparecer": soltar el abierto viejo para
		// que ftsFor lo reabra y verifique a la siguiente búsqueda (tres pisos).
		id := strings.TrimSuffix(file, filepath.Ext(file))
		n.ftsMu.Lock()
		if st, ok := n.fts[id]; ok {
			if st.idx != nil {
				st.idx.Close()
			}
			delete(n.fts, id)
		}
		delete(n.ftsErr, id)
		n.ftsMu.Unlock()
	}

	if cancelled {
		n.idxMu.Lock()
		n.idxQueue = nil
		n.idxMu.Unlock()
		return
	}
	n.nextInQueue()
}

// nextInQueue: modo "indexar todos" — arranca el siguiente de la cola, saltando los
// que no arranquen (p. ej. no resolubles) sin parar la tanda.
func (n *nativeZims) nextInQueue() {
	n.idxMu.Lock()
	if len(n.idxQueue) == 0 {
		n.idxMu.Unlock()
		return
	}
	next := n.idxQueue[0]
	n.idxQueue = n.idxQueue[1:]
	n.idxMu.Unlock()
	if _, msg := n.startIndex(next); msg != "" {
		n.nextInQueue()
	}
}

// POST /api/admin/zim/index {file} — indexar UN ZIM (manual, a elección).
func (n *nativeZims) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo POST"})
		return
	}
	var input struct {
		File string `json:"file"`
	}
	if jsonDecodeSmall(w, r, &input) != nil || !n.validIndexFile(input.File) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fichero no válido"})
		return
	}
	n.idxMu.Lock()
	n.idxQueue = nil // un indexado manual solo hace ese
	n.idxMu.Unlock()
	if ok, msg := n.startIndex(input.File); !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "indexación iniciada"})
}

// POST /api/admin/zim/index/all — modo auto: indexar todos los que aún no tienen
// índice, en cola, uno a uno (fts.Build ya cede internamente por contexto).
func (n *nativeZims) handleIndexAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo POST"})
		return
	}
	n.idxMu.Lock()
	running := n.idxJob != nil && n.idxJob.Status == "indexing"
	n.idxMu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ya hay un índice construyéndose"})
		return
	}
	var files []string
	if books, err := n.az.readLibrary(); err == nil {
		for _, b := range books {
			file := filepath.Base(b.Path)
			if !zimHasIndex(filepath.Join(n.az.zimDir, file)) {
				files = append(files, file)
			}
		}
	}
	if len(files) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "todo indexado", "count": 0})
		return
	}
	n.idxMu.Lock()
	n.idxQueue = files[1:]
	n.idxMu.Unlock()
	if ok, msg := n.startIndex(files[0]); !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "indexación iniciada", "count": len(files)})
}

// POST /api/admin/zim/index/cancel — cancela el job actual y vacía la cola.
func (n *nativeZims) handleIndexCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo POST"})
		return
	}
	n.idxMu.Lock()
	n.idxQueue = nil
	if n.idxCancel != nil {
		n.idxCancel()
	}
	n.idxMu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelando"})
}
