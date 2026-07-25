package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNormalizeRemoteTarget(t *testing.T) {
	t.Parallel()
	valid, err := normalizeRemoteTarget(" https://library.example/ ")
	if err != nil || valid.String() != "https://library.example" {
		t.Fatalf("target valido: %v, %v", valid, err)
	}
	for _, raw := range []string{"", "library.example", "ftp://library.example", "https://user@library.example", "https://library.example/panel", "https://library.example?q=1"} {
		if _, err := normalizeRemoteTarget(raw); err == nil {
			t.Errorf("se acepto target invalido %q", raw)
		}
	}
}

func TestGatewayRewritesHostAndOnlyInjectsClient(t *testing.T) {
	t.Parallel()
	var receivedHost string
	var receivedOrigin string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		receivedOrigin = r.Header.Get("Origin")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><head></head><body>ok</body></html>")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)

	s := &shell{remote: true}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(true)
	server := httptest.NewServer(s)
	defer server.Close()

	root := mustBody(t, server.URL+"/")
	if !strings.Contains(root, "__NOUMON_LIBRARY_SERVER__") || !strings.Contains(root, "__NOUMON_LIBRARY_GATEWAY__=true") {
		t.Fatalf("el documento cliente no recibio la configuracion del shell: %s", root)
	}
	if receivedHost != target.Host {
		t.Fatalf("Host recibido = %q; esperado %q", receivedHost, target.Host)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/test", strings.NewReader("{}"))
	req.Header.Set("Origin", "http://wails.localhost")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if receivedOrigin != target.Scheme+"://"+target.Host {
		t.Fatalf("Origin reenviado = %q; esperado el destino del gateway", receivedOrigin)
	}

	content := mustBody(t, server.URL+"/content/wiki/A/Portada")
	if strings.Contains(content, "__NOUMON_LIBRARY_SERVER__") {
		t.Fatal("el gateway inyecto codigo dentro del contenido ZIM")
	}
}

func TestPanelShellStartsAtPanel(t *testing.T) {
	t.Parallel()
	paths := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><head></head><body>panel</body></html>")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)

	s := &shell{startPath: "/panel/"}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(true)
	server := httptest.NewServer(s)
	defer server.Close()
	_ = mustBody(t, server.URL+"/")
	if path := <-paths; path != "/panel/" {
		t.Fatalf("ruta inicial = %q; esperada /panel/", path)
	}
}

func mustBody(t *testing.T, address string) string {
	t.Helper()
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestSplashDaPasoADesconectadoTrasGracia(t *testing.T) {
	t.Parallel()
	target, _ := url.Parse("http://127.0.0.1:1") // puerto cerrado: nunca conecta
	s := &shell{remote: true}
	s.configured.Store(true)
	s.installProxy(target)

	navigate := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		s.ServeHTTP(rec, req)
		return rec
	}

	// Dentro de la ventana de gracia: splash de "Conectando con...".
	s.bootStarted.Store(time.Now().UnixMilli())
	if body := navigate().Body.String(); !strings.Contains(body, "Conectando con") {
		t.Fatalf("se esperaba el splash, llego: %.120s", body)
	}

	// Agotada la gracia: página de desconexión con opción de cambiar servidor.
	s.bootStarted.Store(time.Now().Add(-connectGrace - time.Second).UnixMilli())
	body := navigate().Body.String()
	if !strings.Contains(body, "Se ha perdido la conexi") || !strings.Contains(body, "Conectar a otro servidor") {
		t.Fatalf("se esperaba la página de desconexión, llego: %.120s", body)
	}

	// Un fetch (sin Accept text/html) nunca recibe HTML con 200: el ping de la
	// página de desconexión no debe confundir el splash con el servidor vivo.
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, healthPath, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("fetch sin html: código %d, se esperaba 503", rec.Code)
	}
}

func TestGatewayConvierteRedireccionesDeNavegacionEnMetaRefresh(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/content/wiki/" {
			http.Redirect(w, r, "/content/wiki/A/Portada", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)

	s := &shell{remote: true}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(true)
	server := httptest.NewServer(s)
	defer server.Close()

	// Navegación (Accept text/html): el webview no sigue 302 en el esquema
	// propio, así que debe llegar 200 con meta refresh al destino.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/content/wiki/", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; se esperaba 200 con meta refresh", response.StatusCode)
	}
	if !strings.Contains(string(body), `url=/content/wiki/A/Portada`) {
		t.Fatalf("falta el destino en el meta refresh: %s", body)
	}

	// Un fetch (sin Accept html) conserva la redirección HTTP original.
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/content/wiki/", nil)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") == "" {
		t.Fatalf("fetch: status = %d Location=%q; se esperaba 302 intacto", response.StatusCode, response.Header.Get("Location"))
	}
}

func TestResolveShellTargetConfigCorruptoNoEsFatal(t *testing.T) {
	// Un gateway.json roto NO puede impedir que la app abra: en una ventana
	// Frameless un log.Fatal es una muerte invisible. Debe tratarse como
	// "sin configurar" con aviso para la pantalla de conexión.
	setConfigDirEnv(t, t.TempDir())
	configPath := shellConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	oldMode := distributionMode
	distributionMode = "remote"
	t.Cleanup(func() { distributionMode = oldMode })
	t.Setenv("NOUMON_LIBRARY_SERVER", "")

	cases := []struct{ name, content string }{
		{"json roto", "{corrupto"},
		{"target invalido", `{"target":"ftp://nas.local"}`},
	}
	for _, c := range cases {
		if err := os.WriteFile(configPath, []byte(c.content), 0o600); err != nil {
			t.Fatal(err)
		}
		target, remote, configured, notice, err := resolveShellTarget()
		if err != nil {
			t.Fatalf("%s: devolvio error fatal: %v", c.name, err)
		}
		if target != nil || !remote || configured {
			t.Fatalf("%s: se esperaba sin configurar; target=%v configured=%v", c.name, target, configured)
		}
		if notice == noticeNone {
			t.Fatalf("%s: falta el aviso para la pantalla de conexion", c.name)
		}
	}
}

func TestSetupMuestraElAvisoDeConfigCorrupta(t *testing.T) {
	t.Parallel()
	s := &shell{remote: true, setupNotice: noticeInvalidConfig}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	s.ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, "no era valida") {
		t.Fatalf("la pantalla de conexion no muestra el aviso: %.200s", body)
	}

	// El aviso se guarda como código justo para poder traducirlo al pintar.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	s.ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, "was not valid") {
		t.Fatalf("el aviso no se tradujo: %.200s", body)
	}
}

// Las pantallas del shell viven antes de que exista SPA, así que su idioma sale
// del Accept-Language. Español e inglés van a la par por regla del proyecto.
func TestPantallasDelShellEnLosDosIdiomas(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		page          func(w *httptest.ResponseRecorder, t shellText)
		marcaES       string
		marcaEN       string
		esperadoLangs bool
	}{
		{
			name:    "arranque",
			page:    func(w *httptest.ResponseRecorder, tx shellText) { serveSplash(w, false, "", tx) },
			marcaES: "Conectando con el servicio local",
			marcaEN: "Connecting to the local",
		},
		{
			name:    "conexion",
			page:    func(w *httptest.ResponseRecorder, tx shellText) { serveSetup(w, noticeNone, tx) },
			marcaES: "Conectar a Noumon Server",
			marcaEN: "Connect to Noumon Server",
		},
		{
			name:    "desconexion",
			page:    func(w *httptest.ResponseRecorder, tx shellText) { serveDisconnected(w, false, "", tx) },
			marcaES: "Se ha perdido la conexi",
			marcaEN: "Lost the connection",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.page(rec, textES)
			body := rec.Body.String()
			if !strings.Contains(body, c.marcaES) {
				t.Errorf("castellano: falta %q", c.marcaES)
			}
			if !strings.Contains(body, `<html lang="es"`) {
				t.Error("castellano: falta lang=\"es\"; un lector de pantalla leeria mal la pagina")
			}

			rec = httptest.NewRecorder()
			c.page(rec, textEN)
			body = rec.Body.String()
			if !strings.Contains(body, c.marcaEN) {
				t.Errorf("ingles: falta %q", c.marcaEN)
			}
			if !strings.Contains(body, `<html lang="en"`) {
				t.Error("ingles: falta lang=\"en\"")
			}
			if strings.Contains(body, c.marcaES) {
				t.Errorf("ingles: se colo texto en castellano (%q)", c.marcaES)
			}
		})
	}
}

// Un diccionario a medias es peor que uno vacío: la pantalla saldría mitad en
// cada idioma sin que nada falle. Esto lo caza al añadir texto nuevo.
func TestLosDosDiccionariosEstanCompletos(t *testing.T) {
	t.Parallel()
	es, en := reflect.ValueOf(textES), reflect.ValueOf(textEN)
	for i := 0; i < es.NumField(); i++ {
		name := es.Type().Field(i).Name
		if es.Field(i).String() == "" {
			t.Errorf("textES.%s vacio", name)
		}
		if en.Field(i).String() == "" {
			t.Errorf("textEN.%s vacio", name)
		}
		if name != "lang" && es.Field(i).String() == en.Field(i).String() {
			t.Errorf("textES.%s y textEN.%s son identicos: falta traducir", name, name)
		}
	}
}

func TestPrefersSpanish(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		header string
		want   bool
	}{
		{"", true}, // sin señal: el idioma de casa
		{"es-ES,es;q=0.9", true},
		{"en-GB,en;q=0.9", false},
		{"en-US,es;q=0.8", false}, // gana la primera reconocida, no la de mayor q
		{"fr-FR,en;q=0.7", false}, // idioma que no tenemos: sigue buscando
		{"*", true},               // comodín: no dice nada, castellano
		{"de-DE", true},           // ninguna reconocida: castellano
	} {
		if got := prefersSpanish(c.header); got != c.want {
			t.Errorf("prefersSpanish(%q) = %v; se esperaba %v", c.header, got, c.want)
		}
	}
}

func TestGatewayReescribeRedireccionAbsolutaDelMismoHost(t *testing.T) {
	t.Parallel()
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/content/wiki/" {
			// Redireccion ABSOLUTA hacia el propio servidor: el caso que
			// escaparia del gateway si no se reescribe.
			w.Header().Set("Location", upstream.URL+"/content/wiki/A/Portada")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)

	s := &shell{remote: true}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(true)
	server := httptest.NewServer(s)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Navegacion: el meta refresh debe apuntar a la ruta RELATIVA.
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/content/wiki/", nil)
	request.Header.Set("Accept", "text/html")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `url=/content/wiki/A/Portada`) || strings.Contains(string(body), upstream.URL) {
		t.Fatalf("el meta refresh no quedo relativo al gateway: %s", body)
	}

	// Fetch: el 302 conserva Location, pero ya reescrito a relativo.
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/content/wiki/", nil)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if got := response.Header.Get("Location"); got != "/content/wiki/A/Portada" {
		t.Fatalf("Location = %q; esperado /content/wiki/A/Portada", got)
	}
}

// navigationBody pide address como lo haría el webview al navegar y devuelve
// estado y cuerpo. lang viaja en Accept-Language ("" = sin cabecera).
func navigationBody(t *testing.T, address, lang string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, address, nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	if lang != "" {
		request.Header.Set("Accept-Language", lang)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	return response.StatusCode, string(body)
}

// shellTo monta un shell listo delante de upstream.
func shellTo(t *testing.T, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	target, _ := url.Parse(upstream.URL)
	s := &shell{}
	s.installProxy(target)
	s.configured.Store(true)
	s.ready.Store(true)
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	return server
}

func TestGatewayNoDejaAlWebviewPintarSuPantallaDeDepuracion(t *testing.T) {
	// Wails trata como índice toda ruta acabada en "/": ante un 404 sustituye la
	// página por su defaultindex.html ("index.html not found"). Le pasó a un
	// usuario real cuando otro proceso ocupaba el 8090 y el shell hablaba con un
	// servidor sin www-client: parecía que el instalador estuviera roto.
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // 404 en texto plano, como un Core sin cliente al lado
	}))
	defer upstream.Close()
	server := shellTo(t, upstream)

	status, body := navigationBody(t, server.URL+"/", "es-ES,es;q=0.9")
	if status != http.StatusOK {
		t.Fatalf("status = %d; con 404 el webview pinta su propia pagina, hace falta 200", status)
	}
	if !strings.Contains(body, "no est&aacute; sirviendo la interfaz") {
		t.Fatalf("no es la pagina del shell: %s", body)
	}

	// Un fetch de la SPA no es una navegación: conserva el error real. De esto
	// depende el sondeo de reintento de la propia página.
	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("fetch: status = %d; se esperaba el 404 intacto", response.StatusCode)
	}
}

func TestGatewayConservaLaPaginaDeErrorDelServidor(t *testing.T) {
	// Si el servidor se explica en HTML, dice más que cualquier sustituto
	// nuestro: solo se normaliza el código para que el webview no la tire.
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<!doctype html><h1>Coleccion retirada</h1>`))
	}))
	defer upstream.Close()
	server := shellTo(t, upstream)

	status, body := navigationBody(t, server.URL+"/content/wiki/", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d; se esperaba 200 para que el webview la respete", status)
	}
	if !strings.Contains(body, "Coleccion retirada") {
		t.Fatalf("se perdio la pagina del servidor: %s", body)
	}
}

func TestPaginaDeErrorHablaElIdiomaDelWebview(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "vaya", http.StatusBadGateway)
	}))
	defer upstream.Close()
	server := shellTo(t, upstream)

	// Una ruta de contenido no es la puerta de la SPA: el mensaje es otro.
	_, spanish := navigationBody(t, server.URL+"/content/wiki/", "es-ES,es;q=0.9")
	if !strings.Contains(spanish, "no existe en el servidor") || !strings.Contains(spanish, ">Volver<") {
		t.Fatalf("castellano: %s", spanish)
	}
	_, english := navigationBody(t, server.URL+"/content/wiki/", "en-GB,en;q=0.9")
	if !strings.Contains(english, "doesn&rsquo;t exist on the server") || !strings.Contains(english, ">Back<") {
		t.Fatalf("ingles: %s", english)
	}
}

// setConfigDirEnv apunta os.UserConfigDir a un directorio temporal en el SO
// donde corra el test.
func setConfigDirEnv(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}
