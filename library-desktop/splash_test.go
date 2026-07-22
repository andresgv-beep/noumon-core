package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// El arranque es una animación con reloj propio. Esta prueba fija lo que la
// hace funcionar, porque son cosas que se rompen sin que nada falle a la vista.
func TestBootPageContract(t *testing.T) {
	rec := httptest.NewRecorder()
	serveSplash(rec, false, "")
	body := rec.Body.String()

	// El <meta refresh> recargaba la página entera cada segundo: la animación
	// no llegaba a empezar nunca. Ahora se sondea /api/health con fetch.
	if strings.Contains(body, "http-equiv=\"refresh\"") {
		t.Error("el arranque no puede recargarse solo: reiniciaría la animación en bucle")
	}
	if !strings.Contains(body, "/api/health") {
		t.Error("falta el sondeo de salud: nada haría entrar en la interfaz")
	}
	// La rodadura del lateral al centro se ve SIEMPRE, aunque el servidor
	// conteste al instante (decisión de Andrés: es la firma del arranque).
	if !strings.Contains(body, "roll-in") {
		t.Error("falta la rodadura: el logo debe entrar desde el lateral")
	}
	// El relevo: sin la marca, el cliente entraría de golpe sin fundido.
	if !strings.Contains(body, "noumon-boot") {
		t.Error("falta la marca de relevo que funde el arranque con el tema del usuario")
	}
	// Texto que el usuario lee si la espera se alarga.
	if !strings.Contains(body, "Conectando con") {
		t.Error("falta el aviso de conexión")
	}
}

func TestBootPageNamesRemoteTarget(t *testing.T) {
	rec := httptest.NewRecorder()
	serveSplash(rec, true, "https://nas.casa:8090")
	if body := rec.Body.String(); !strings.Contains(body, "nas.casa") {
		t.Error("en modo remoto hay que decir CON QUIÉN se está conectando")
	}
}

// El destino remoto llega de la configuración del usuario y se pinta en la
// página: tiene que ir escapado o sería una inyección de HTML.
func TestBootPageEscapesTarget(t *testing.T) {
	rec := httptest.NewRecorder()
	serveSplash(rec, true, `<script>alert(1)</script>`)
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("el destino se pintó sin escapar: inyección de HTML")
	}
}
