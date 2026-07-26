package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
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

// resolveShellTarget devuelve el destino inicial del shell. notice lleva un
// aviso para la pantalla de conexión cuando la configuración guardada no
// sirve: en una app Frameless un log.Fatal es una muerte invisible (doble clic
// y no aparece nada), así que gateway.json ilegible o inválido NUNCA es fatal —
// se trata como "sin configurar" y el usuario vuelve a escribir la dirección.
// Solo NOUMON_LIBRARY_SERVER inválida sigue siendo error: es un contrato
// explícito del operador, no estado guardado que pueda corromperse solo.
func resolveShellTarget() (target *url.URL, remote, configured bool, notice setupNotice, err error) {
	remote = distributionMode == "remote"
	if raw := strings.TrimSpace(os.Getenv("NOUMON_LIBRARY_SERVER")); raw != "" {
		target, err = normalizeRemoteTarget(raw)
		return target, true, err == nil, noticeNone, err
	}
	if !remote {
		target, err = url.Parse(localCoreURL)
		return target, false, err == nil, noticeNone, err
	}

	raw, readErr := os.ReadFile(shellConfigPath())
	if os.IsNotExist(readErr) {
		return nil, true, false, noticeNone, nil
	}
	if readErr != nil {
		log.Printf("gateway.json ilegible, se pide la direccion de nuevo: %v", readErr)
		return nil, true, false, noticeUnreadableConfig, nil
	}
	var cfg gatewayConfig
	if jsonErr := json.Unmarshal(raw, &cfg); jsonErr != nil {
		log.Printf("gateway.json invalido, se pide la direccion de nuevo: %v", jsonErr)
		return nil, true, false, noticeInvalidConfig, nil
	}
	target, targetErr := normalizeRemoteTarget(cfg.Target)
	if targetErr != nil {
		log.Printf("direccion guardada invalida, se pide de nuevo: %v", targetErr)
		return nil, true, false, noticeInvalidTarget, nil
	}
	return target, true, true, noticeNone, nil
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
			serveDisconnected(w, s.remote, s.targetString(), textsFor(r))
			return
		}
		http.Error(w, "Library Server no disponible", http.StatusServiceUnavailable)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		// Blindaje: si el servidor emitiera un Location ABSOLUTO hacia sí mismo
		// (http://ip-del-servidor/...), el webview navegaría fuera del origen del
		// gateway — se perderían la inyección de globals, el mismo-origen y la
		// barra de ventana. Se reescribe a ruta relativa para que la navegación
		// (y cualquier fetch que siga la redirección) se quede dentro del proxy.
		// Un Location hacia OTRO host se deja intacto: no es contenido nuestro.
		if location := response.Header.Get("Location"); location != "" &&
			response.StatusCode >= 300 && response.StatusCode < 400 {
			if parsed, err := url.Parse(location); err == nil && parsed.Host == target.Host && parsed.IsAbs() {
				parsed.Scheme, parsed.Host, parsed.User = "", "", nil
				relative := parsed.String()
				if relative == "" {
					relative = "/"
				}
				response.Header.Set("Location", relative)
			}
		}
		// Los webview de Wails sirven la app por un esquema propio y NO siguen
		// redirecciones HTTP en las navegaciones: pintan el cuerpo del 302
		// ("Found.") tal cual. Ocurre p. ej. en la raíz de una colección ZIM,
		// que redirige a su portada. Se convierte la redirección en una página
		// mínima con meta refresh: el webview sí navega a la URL destino, y al
		// llegar allí las rutas relativas del artículo funcionan igual que en
		// un navegador. Solo para navegaciones (Accept: text/html); las
		// llamadas fetch de la SPA conservan la redirección original.
		if location := response.Header.Get("Location"); location != "" &&
			response.StatusCode >= 300 && response.StatusCode < 400 &&
			response.Request != nil &&
			strings.Contains(response.Request.Header.Get("Accept"), "text/html") {
			if response.Body != nil {
				response.Body.Close()
			}
			target := template.HTMLEscapeString(location)
			body := []byte(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=` + target + `"><a href="` + target + `">Abriendo…</a>`)
			response.StatusCode = http.StatusOK
			response.Status = http.StatusText(http.StatusOK)
			response.Header.Del("Location")
			response.Header.Set("Content-Type", "text/html; charset=utf-8")
			response.Header.Set("Cache-Control", "no-store")
			response.Body = io.NopCloser(bytes.NewReader(body))
			response.ContentLength = int64(len(body))
			response.Header.Set("Content-Length", strconv.Itoa(len(body)))
			return nil
		}
		// El ErrorHandler de arriba cubre "no se alcanza el servidor". Falta el
		// caso contrario: el servidor SÍ contesta, pero con error, y entonces es
		// el webview quien pinta. Para las navegaciones que Wails trata como
		// índice eso significa su defaultindex.html ("index.html not found") en
		// un 404, o una ventana en blanco en cualquier otro error. Se adelanta el
		// shell con una página propia —o con la del servidor, si mandó una— y se
		// normaliza el código a 200, que es lo que el webview respeta.
		if response.StatusCode >= 400 && isIndexNavigation(response.Request) {
			return rewriteNavigationError(response)
		}
		if !isClientDocument(response) {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		response.Body.Close()
		// __NOUMON_LIBRARY_CORE__ = URL REAL del Core (no vacía como SERVER). El
		// Studio la usa para subir binarios (multipart) DIRECTO al Core: el
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

// isIndexNavigation aísla exactamente las respuestas que el webview mangonea:
// una NAVEGACIÓN (no un fetch de la SPA) hacia una ruta que Wails considera
// índice. La condición de la ruta replica a propósito, palabra por palabra,
// assetserver.isRuntimeInjectionMatch: si Wails cambiara ese criterio, este
// sería el sitio donde volver a alinearlo.
func isIndexNavigation(r *http.Request) bool {
	// Solo GET: el assetserver deriva cualquier otro método directo al handler
	// sin mirar el cuerpo, así que ahí no hay nada que adelantarse a arreglar —
	// y colarle un cuerpo a un HEAD sería inventarse una respuesta.
	if r == nil || r.Method != http.MethodGet {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	return strings.HasSuffix(path, "/") || strings.HasSuffix(path, "/index.html")
}

// isAppEntry: la puerta de entrada de una de las dos SPA. Un error ahí no es
// "esta página no existe", es "el servidor no está sirviendo la aplicación", y
// son dos mensajes muy distintos para el usuario.
func isAppEntry(path string) bool {
	switch path {
	case "", "/", "/index.html", "/panel/", "/panel/index.html":
		return true
	}
	return false
}

func rewriteNavigationError(response *http.Response) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	response.Body.Close()
	// Si el servidor mandó su propia página HTML, dice más que cualquier
	// sustituto nuestro: se conserva tal cual y solo se normaliza el código para
	// que el webview no la tire. Solo se suplanta lo que no es una página:
	// un "404 page not found" en texto plano, o un cuerpo vacío.
	request := response.Request
	if !strings.Contains(response.Header.Get("Content-Type"), "text/html") || len(bytes.TrimSpace(body)) == 0 {
		body = noumonErrorPage(response.StatusCode, request.URL.Path,
			isAppEntry(request.URL.Path), textsFor(request))
		response.Header.Set("Content-Type", "text/html; charset=utf-8")
		// El Director borra el Accept-Encoding, así que el cuerpo debería llegar
		// sin comprimir. Pero este arreglo existe precisamente porque un servidor
		// se comportó de forma inesperada: si además viniera comprimido, dejar el
		// Content-Encoding puesto haría que el webview intentara descomprimir un
		// HTML plano y pintara basura — otra vez código en la ventana.
		response.Header.Del("Content-Encoding")
	}
	response.StatusCode = http.StatusOK
	response.Status = http.StatusText(http.StatusOK)
	response.Header.Set("Cache-Control", "no-store")
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

func isClientDocument(response *http.Response) bool {
	if response.Request == nil || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
		return false
	}
	path := response.Request.URL.Path
	// El index del PANEL también es un SPA del shell y recibe los mismos globals.
	// Aunque el Panel ya no crea contenido, mantener una única inyección evita
	// contratos distintos entre las dos SPA. Sus subrecursos
	// (/panel/assets/*.js|css) NO son text/html, así que no matchean.
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
