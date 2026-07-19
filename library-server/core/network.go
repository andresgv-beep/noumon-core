// network.go — publicación del servidor en la red local (Panel → Red).
//
// El Core siempre escucha donde diga BIND (main.go); este módulo solo persiste
// la preferencia lanAccess en el config.json compartido con el supervisor y
// pide el reinicio administrativo. Es el supervisor quien, al relanzar Core,
// traduce lanAccess a BIND=0.0.0.0 (salvo que el operador fije BIND por
// entorno, que siempre gana). Mismo contrato que el cambio de pool: guardar +
// código 75.

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
)

type networkInfo struct {
	configPath string
	bind       string
	port       string
}

// handleNetwork: GET estado de publicación, PUT {lanAccess} para cambiarla.
// Montado en adminMux: solo administradores.
func (n *networkInfo) handleNetwork(w http.ResponseWriter, r *http.Request) {
	supervised := os.Getenv("LIBRARY_SUPERVISED") == "1"
	switch r.Method {
	case http.MethodGet:
		cfg, _ := readStorageConfig(n.configPath)
		writeJSON(w, http.StatusOK, map[string]any{
			"lanAccess":    cfg.LanAccess,
			"bind":         n.bind,
			"port":         n.port,
			"published":    n.bind != "127.0.0.1" && n.bind != "localhost" && n.bind != "::1",
			"configurable": n.configPath != "" && os.Getenv("BIND") == "",
			"supervised":   supervised,
			"addresses":    lanAddresses(n.port),
		})
	case http.MethodPut:
		if n.configPath == "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "este servidor no dispone de configuracion persistente"})
			return
		}
		if os.Getenv("BIND") != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "BIND esta fijado por el entorno del servidor; cambialo alli"})
			return
		}
		var input struct {
			LanAccess bool `json:"lanAccess"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON invalido"})
			return
		}
		cfg, _ := readStorageConfig(n.configPath)
		cfg.LanAccess = input.LanAccess
		if err := writeStorageConfig(n.configPath, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no se pudo guardar la configuracion: " + err.Error()})
			return
		}
		status := http.StatusOK
		restarting := false
		if supervised {
			status = http.StatusAccepted
			restarting = scheduleSupervisedRestart()
		}
		writeJSON(w, status, map[string]any{"lanAccess": input.LanAccess, "restartRequired": true, "restarting": restarting})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "metodo no permitido"})
	}
}

// lanAddresses lista las URL en que la biblioteca queda visible desde la LAN:
// una por IPv4 no-loopback de la máquina. Es informativo para el Panel.
func lanAddresses(port string) []string {
	addresses := []string{}
	interfaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return addresses
	}
	for _, addr := range interfaceAddrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ipv4 := ipNet.IP.To4(); ipv4 != nil {
			addresses = append(addresses, "http://"+ipv4.String()+":"+port)
		}
	}
	return addresses
}
