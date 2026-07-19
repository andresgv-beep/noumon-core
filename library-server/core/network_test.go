package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNetworkUpdatePersistsPreservesRootAndRestarts(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"contentRoot":"/data/pool"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIBRARY_SUPERVISED", "1")
	t.Setenv("BIND", "")
	restartRequested.Store(false)
	exited := make(chan int, 1)
	originalExit := exitProcess
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = originalExit; restartRequested.Store(false) })

	network := &networkInfo{configPath: configPath, bind: "127.0.0.1", port: "8090"}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/network", bytes.NewBufferString(`{"lanAccess":true}`))
	network.handleNetwork(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	cfg, err := readStorageConfig(configPath)
	if err != nil || !cfg.LanAccess || cfg.ContentRoot != "/data/pool" {
		t.Fatalf("config inesperada: %+v, err=%v", cfg, err)
	}
	select {
	case code := <-exited:
		if code != supervisedRestartExitCode {
			t.Fatalf("exit = %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("no se solicito reinicio")
	}
}

func TestNetworkUpdateRejectsOperatorBind(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/network", bytes.NewBufferString(`{"lanAccess":true}`))
	(&networkInfo{configPath: filepath.Join(t.TempDir(), "config.json"), bind: "0.0.0.0", port: "8090"}).handleNetwork(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkStatusReportsPublication(t *testing.T) {
	t.Setenv("BIND", "")
	t.Setenv("LIBRARY_SUPERVISED", "")
	recorder := httptest.NewRecorder()
	(&networkInfo{configPath: "", bind: "0.0.0.0", port: "8090"}).handleNetwork(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/network", nil))
	var status struct {
		Published    bool `json:"published"`
		Configurable bool `json:"configurable"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Published || status.Configurable {
		t.Fatalf("estado inesperado: %s", recorder.Body.String())
	}
}

func TestStorageRootUpdateKeepsLanAccess(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"contentRoot":"/old","lanAccess":true}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIBRARY_SUPERVISED", "")

	root := filepath.Join(base, "nuevo-pool")
	body, _ := json.Marshal(storageConfig{ContentRoot: root})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/storage", bytes.NewReader(body))
	(&poolInfo{configPath: configPath}).handleStorage(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	cfg, err := readStorageConfig(configPath)
	if err != nil || !cfg.LanAccess || cfg.ContentRoot != root {
		t.Fatalf("lanAccess se perdio al cambiar el pool: %+v, err=%v", cfg, err)
	}
}
