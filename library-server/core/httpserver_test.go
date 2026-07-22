package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// El servidor tiene que reciclar sockets abandonados, pero SIN poner un tope a
// la duración de una respuesta: un vídeo largo o un ZIM grande tardan más que
// cualquier plazo razonable.
func TestHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":8090", http.NewServeMux())

	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v; DEBE quedarse a cero: mide la respuesta entera "+
			"y cortaría un vídeo o un ZIM a la mitad. Para clientes ociosos está IdleTimeout.",
			srv.WriteTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %v; a cero: mide el cuerpo entero y rompería subidas grandes.",
			srv.ReadTimeout)
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("sin ReadHeaderTimeout, unas cabeceras a medio enviar retienen un socket indefinidamente")
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("sin IdleTimeout, el keep-alive de un cliente que se fue nunca se recicla")
	}
}

// Una respuesta lenta y larga (el caso "película") debe completarse entera: es
// la garantía que se perdería el día que alguien añada WriteTimeout.
func TestSlowLongResponseIsNotCutOff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 6; i++ {
			w.Write([]byte("xxxx"))
			w.(http.Flusher).Flush()
			time.Sleep(120 * time.Millisecond) // ritmo de reproducción, no de descarga
		}
	})
	srv := newHTTPServer("127.0.0.1:0", mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)
	defer srv.Close()

	resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("la respuesta larga se cortó: %v", err)
	}
	if len(body) != 24 {
		t.Fatalf("llegaron %d bytes de 24: la respuesta se truncó", len(body))
	}
}
