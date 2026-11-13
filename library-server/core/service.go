package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

const (
	supervisedRestartExitCode = 75
	supervisedStopExitCode    = 76
)

var (
	restartRequested atomic.Bool
	exitProcess      = os.Exit
)

// handleServiceControl solo se monta dentro de adminMux. La interfaz solicita
// reiniciar o detener; no toca procesos. El supervisor interpreta la salida.
func (s *Server) handleServiceControl(w http.ResponseWriter, r *http.Request) {
	supervised := os.Getenv("LIBRARY_SUPERVISED") == "1"
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{
			"supervised":    supervised,
			"stopSupported": supervised && os.Getenv("LIBRARY_CAN_STOP") == "1",
			"pid":           os.Getpid(),
		})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "metodo no permitido"})
		return
	}
	if !supervised {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Library Server no esta bajo supervision"})
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitud invalida"})
		return
	}
	code, status := supervisedRestartExitCode, "reiniciando"
	switch input.Action {
	case "", "restart": // Compatibilidad con los paneles anteriores.
	case "stop":
		if os.Getenv("LIBRARY_CAN_STOP") != "1" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "el supervisor no permite detener el servidor; actualizalo primero"})
			return
		}
		code, status = supervisedStopExitCode, "deteniendo"
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accion desconocida"})
		return
	}
	if !scheduleSupervisedExit(code) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ya hay un cambio de servicio en curso"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": status})
}

func scheduleSupervisedRestart() bool {
	return scheduleSupervisedExit(supervisedRestartExitCode)
}

func scheduleSupervisedExit(code int) bool {
	if !restartRequested.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		exitProcess(code)
	}()
	return true
}
