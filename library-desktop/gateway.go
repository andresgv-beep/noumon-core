package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const gatewayConfigPath = "/__noumon/gateway"

type gatewayConfig struct {
	Target string `json:"target"`
}

func resolveShellTarget() (*url.URL, bool, bool, error) {
	remote := distributionMode == "remote"
	if raw := strings.TrimSpace(os.Getenv("NOUMON_LIBRARY_SERVER")); raw != "" {
		target, err := normalizeRemoteTarget(raw)
		return target, true, err == nil, err
	}
	if !remote {
		target, err := url.Parse(localCoreURL)
		return target, false, err == nil, err
	}

	raw, err := os.ReadFile(shellConfigPath())
	if os.IsNotExist(err) {
		return nil, true, false, nil
	}
	if err != nil {
		return nil, true, false, fmt.Errorf("leer configuracion del gateway: %w", err)
	}
	var cfg gatewayConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, true, false, fmt.Errorf("configuracion del gateway invalida: %w", err)
	}
	target, err := normalizeRemoteTarget(cfg.Target)
	return target, true, err == nil, err
}

func normalizeRemoteTarget(raw string) (*url.URL, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil, fmt.Errorf("escribe la direccion de Library Server")
	}
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, fmt.Errorf("la direccion debe empezar por http:// o https://")
	}
	if target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return nil, fmt.Errorf("usa solo el origen de Library Server, sin ruta, usuario, query ni fragmento")
	}
	target.Path = ""
	return target, nil
}

func shellConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "Noumon", "gateway.json")
}

func saveGatewayTarget(target *url.URL) error {
	path := shellConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(gatewayConfig{Target: target.String()}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func (s *shell) installProxy(target *url.URL) {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
		// El WebView habla con el gateway en su origen interno. Al reenviar al
		// servidor, normalizamos Origin al destino para que la defensa CSRF pueda
		// distinguir este proxy legítimo de una web hostil.
		if r.Header.Get("Origin") != "" {
			r.Header.Set("Origin", target.Scheme+"://"+target.Host)
		}
		// Garantiza que ModifyResponse pueda inspeccionar el HTML de la SPA.
		r.Header.Del("Accept-Encoding")
		// Noumon es un producto LOCAL/LAN, no un servicio de internet detrás de un
		// reverse-proxy (Caddy). El shell de escritorio es un proxy LOCAL de
		// confianza: si dejamos que httputil inyecte X-Forwarded-For con la IP del
		// WebView, el Core cree que el usuario es "remoto" y exige el código de
		// configuración para crear el primer admin — auto-bloqueando la app en la
		// propia máquina. Poner el header a nil le dice a ReverseProxy que NO lo
		// añada, así requestIsLocal reconoce al usuario local. La protección real
		// del alta remota sigue viva para un servidor detrás de un proxy de verdad.
		r.Header["X-Forwarded-For"] = nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		// Para una navegación (recarga, arranque con el servidor caído) el WebView
		// pintaría su página de error interna, sin controles de ventana: servimos
		// la página de desconexión del shell. Las llamadas fetch de la SPA siguen
		// recibiendo el error plano de siempre.
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			serveDisconnected(w, s.remote, s.targetString())
			return
		}
		http.Error(w, "Library Server no disponible", http.StatusServiceUnavailable)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		if !isClientDocument(response) {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		response.Body.Close()
		// __NOUMON_LIBRARY_CORE__ = URL REAL del Core (no vacía como SERVER). El
		// front la usa SOLO para subir contenido (multipart) DIRECTO al Core: el
		// webview de Wails/WebView2 no reenvía el body del POST por su AssetServer,
		// así que las subidas relativas llegan vacías (ficheros de 0 bytes). Una
		// petición a esta URL absoluta es red real (no la intercepta el AssetServer)
		// y sí lleva el body. Ver MOMENTS-UPLOAD.md.
		injection := `<script>window.__NOUMON_LIBRARY_SERVER__="";window.__NOUMON_LIBRARY_SHELL__=true;window.__NOUMON_LIBRARY_CORE__=` + strconv.Quote(s.targetString()) + `;window.__NOUMON_LIBRARY_GATEWAY__=` + strconv.FormatBool(s.remote) + `;</script>`
		body = bytes.Replace(body, []byte("<head>"), []byte("<head>"+injection), 1)
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		response.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}

	s.mu.Lock()
	s.target = target
	s.proxy = proxy
	s.mu.Unlock()
}

func isClientDocument(response *http.Response) bool {
	if response.Request == nil || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
		return false
	}
	path := response.Request.URL.Path
	// El index del PANEL también es un SPA del shell y necesita los globals
	// inyectados (__NOUMON_LIBRARY_CORE__ para subir directo al Core — sin esto la
	// subida cae al proxy de Wails y llega vacía, MOMENTS-UPLOAD.md). Sus
	// subrecursos (/panel/assets/*.js|css) NO son text/html, así que no matchean.
	if path == "/panel" || path == "/panel/" || path == "/panel/index.html" {
		return true
	}
	reserved := []string{"/api", "/content", "/media", "/panel", "/maps", "/mapdata", "/catalog", "/assets", "/pdfjs"}
	for _, prefix := range reserved {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return false
		}
	}
	return true
}

func (s *shell) targetString() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.target == nil {
		return ""
	}
	return s.target.String()
}

func (s *shell) serveGatewayConfig(w http.ResponseWriter, r *http.Request) {
	if !s.remote {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(gatewayConfig{Target: s.targetString()})
		return
	}
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, `{"error":"metodo no permitido"}`, http.StatusMethodNotAllowed)
		return
	}

	var cfg gatewayConfig
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeGatewayError(w, http.StatusBadRequest, "peticion invalida")
		return
	}
	target, err := normalizeRemoteTarget(cfg.Target)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := saveGatewayTarget(target); err != nil {
		writeGatewayError(w, http.StatusInternalServerError, "no se pudo guardar la configuracion")
		return
	}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(false)
	// Ventana de gracia nueva para el destino nuevo: que el usuario vea el
	// splash otros connectGrace segundos antes de volver a la página de
	// desconexión.
	s.bootStarted.Store(0)
	s.startBoot()
	_ = json.NewEncoder(w).Encode(gatewayConfig{Target: target.String()})
}

func writeGatewayError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
