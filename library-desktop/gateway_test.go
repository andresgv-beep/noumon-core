package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
