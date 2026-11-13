package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceControlRequiresSupervisor(t *testing.T) {
	t.Setenv("LIBRARY_SUPERVISED", "")
	restartRequested.Store(false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/service", nil)
	(&Server{}).handleServiceControl(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d; esperado %d", recorder.Code, http.StatusConflict)
	}
}

func TestServiceControlStopRequestsDistinctExit(t *testing.T) {
	t.Setenv("LIBRARY_SUPERVISED", "1")
	t.Setenv("LIBRARY_CAN_STOP", "1")
	restartRequested.Store(false)
	exited := make(chan int, 1)
	originalExit := exitProcess
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = originalExit; restartRequested.Store(false) })

	recorder := httptest.NewRecorder()
	(&Server{}).handleServiceControl(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/service", strings.NewReader(`{"action":"stop"}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	// Una orden concurrente no debe aceptar una acción diferente de la pendiente.
	duplicate := httptest.NewRecorder()
	(&Server{}).handleServiceControl(duplicate, httptest.NewRequest(http.MethodPost, "/api/admin/service", strings.NewReader(`{"action":"restart"}`)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("orden concurrente = %d", duplicate.Code)
	}
	select {
	case code := <-exited:
		if code != supervisedStopExitCode {
			t.Fatalf("exit = %d; esperado %d", code, supervisedStopExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no se solicito la parada")
	}
}

func TestServiceControlRejectsInvalidActionsAndOldSupervisor(t *testing.T) {
	t.Setenv("LIBRARY_SUPERVISED", "1")
	t.Setenv("LIBRARY_CAN_STOP", "")
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"action":"stop"}`, http.StatusConflict},
		{`{"action":"unknown"}`, http.StatusBadRequest},
		{`{`, http.StatusBadRequest},
	} {
		t.Run(tc.body, func(t *testing.T) {
			restartRequested.Store(false)
			recorder := httptest.NewRecorder()
			(&Server{}).handleServiceControl(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/service", strings.NewReader(tc.body)))
			if recorder.Code != tc.status || restartRequested.Load() {
				t.Fatalf("status=%d, salida programada=%v", recorder.Code, restartRequested.Load())
			}
		})
	}
}

func TestServiceControlRequestsReservedExit(t *testing.T) {
	t.Setenv("LIBRARY_SUPERVISED", "1")
	restartRequested.Store(false)
	exited := make(chan int, 1)
	originalExit := exitProcess
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = originalExit; restartRequested.Store(false) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/service", nil)
	(&Server{}).handleServiceControl(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d; esperado %d", recorder.Code, http.StatusAccepted)
	}
	select {
	case code := <-exited:
		if code != supervisedRestartExitCode {
			t.Fatalf("exit = %d; esperado %d", code, supervisedRestartExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("no se solicito la salida al supervisor")
	}
}
