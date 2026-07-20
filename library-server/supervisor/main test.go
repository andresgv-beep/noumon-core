package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMergeEnvReplacesValues(t *testing.T) {
	result := mergeEnv([]string{"A=one", "B=two"}, map[string]string{"a": "changed", "C": "three"})
	joined := strings.ToUpper(strings.Join(result, "\n"))
	if strings.Count(joined, "A=") != 1 || !strings.Contains(joined, "A=CHANGED") || !strings.Contains(joined, "C=THREE") {
		t.Fatalf("entorno inesperado: %v", result)
	}
}

func TestFindExecutable(t *testing.T) {
	dir := t.TempDir()
	name := "library-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findExecutable(dir, "missing", "library-server"); got != path {
		t.Fatalf("findExecutable = %q; esperado %q", got, path)
	}
}

func TestCoreEnvLoadsConfiguredContentRootAndKeepsStateDB(t *testing.T) {
	base := t.TempDir()
	content := filepath.Join(base, "large-library")
	t.Setenv("NOUMON_LIBRARY_DATA", base)
	t.Setenv("POOL_ROOT", "")
	configPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"contentRoot":"`+strings.ReplaceAll(content, `\`, `\\`)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &supervisor{}
	joined := strings.Join(s.coreEnv(), "\n")
	for _, want := range []string{
		"POOL_ROOT=" + content,
		"POOL_PROVIDER=configured",
		"DB_PATH=" + filepath.Join(base, "data", "db", "library.db"),
		"DOWNLOADS_DB=" + filepath.Join(base, "data", "db", "downloads.db"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q en entorno:\n%s", want, joined)
		}
	}
}

func TestTranslateEnvUsesConfiguredModelsDir(t *testing.T) {
	base := t.TempDir()
	content := filepath.Join(base, "large-library")
	t.Setenv("NOUMON_LIBRARY_DATA", base)
	t.Setenv("POOL_ROOT", "")
	t.Setenv("MODELS_DIR", "")
	configPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"contentRoot":"`+strings.ReplaceAll(content, `\`, `\\`)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join((&supervisor{}).translateEnv(), "\n")
	want := "MODELS_DIR=" + filepath.Join(content, "models")
	if !strings.Contains(joined, want) {
		t.Fatalf("falta %q en entorno:\n%s", want, joined)
	}
}

func TestTranslateEnvRespectsExplicitModelsDir(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom-models")
	t.Setenv("MODELS_DIR", explicit)
	joined := strings.Join((&supervisor{}).translateEnv(), "\n")
	if !strings.Contains(joined, "MODELS_DIR="+explicit) {
		t.Fatalf("no se respeto MODELS_DIR explicito:\n%s", joined)
	}
}

func TestCoreEnvPublishesLanWhenConfigured(t *testing.T) {
	base := t.TempDir()
	t.Setenv("NOUMON_LIBRARY_DATA", base)
	t.Setenv("POOL_ROOT", "")
	t.Setenv("BIND", "")
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"lanAccess":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((&supervisor{}).coreEnv(), "\n")
	if !strings.Contains(joined, "BIND=0.0.0.0") {
		t.Fatalf("lanAccess no publico el servidor:\n%s", joined)
	}
}

func TestCoreEnvOperatorBindWinsOverLanAccess(t *testing.T) {
	base := t.TempDir()
	t.Setenv("NOUMON_LIBRARY_DATA", base)
	t.Setenv("POOL_ROOT", "")
	t.Setenv("BIND", "192.168.1.5")
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"lanAccess":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((&supervisor{}).coreEnv(), "\n")
	if strings.Contains(joined, "BIND=0.0.0.0") || !strings.Contains(joined, "BIND=192.168.1.5") {
		t.Fatalf("el BIND del operador no gano:\n%s", joined)
	}
}

func TestRestartDelayAdminRestartNotOverriddenByUptime(t *testing.T) {
	// Caso real: el servidor lleva horas vivo cuando el admin guarda ajustes.
	sleep, next := restartDelay(time.Second, 2*time.Hour, true)
	if sleep != 300*time.Millisecond {
		t.Fatalf("reinicio administrativo durmio %v; esperado 300ms", sleep)
	}
	if next != time.Second {
		t.Fatalf("backoff tras reinicio administrativo = %v; esperado 1s", next)
	}
}

func TestRestartDelayBackoffAndReset(t *testing.T) {
	// Bucle de caidas: 1s→2s→4s… con techo en 30s.
	delay := time.Second
	var sleeps []time.Duration
	for i := 0; i < 6; i++ {
		var sleep time.Duration
		sleep, delay = restartDelay(delay, time.Second, false)
		sleeps = append(sleeps, sleep)
	}
	want := []time.Duration{1, 2, 4, 8, 16, 30}
	for i, w := range want {
		if sleeps[i] != w*time.Second {
			t.Fatalf("caida %d durmio %v; esperado %v", i, sleeps[i], w*time.Second)
		}
	}
	// Un proceso sano (>30s de uptime) rearma el backoff a 1s.
	sleep, next := restartDelay(delay, time.Minute, false)
	if sleep != time.Second || next != 2*time.Second {
		t.Fatalf("tras uptime sano: sleep=%v next=%v; esperado 1s/2s", sleep, next)
	}
}

func TestOpenRotatedLogRotatesBySize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "core.log")
	if err := os.WriteFile(path, []byte("contenido viejo"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := openRotatedLog(path, 10) // limite diminuto para forzar la rotacion
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("nuevo"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	old, err := os.ReadFile(path + ".old")
	if err != nil || string(old) != "contenido viejo" {
		t.Fatalf("no se roto el log: %v %q", err, old)
	}
	current, _ := os.ReadFile(path)
	if string(current) != "nuevo" {
		t.Fatalf("el log nuevo no empieza limpio: %q", current)
	}
	// Por debajo del limite no rota.
	file, err = openRotatedLog(path, maxLogSize)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if data, _ := os.ReadFile(path); string(data) != "nuevo" {
		t.Fatalf("roto sin superar el limite: %q", data)
	}
}

func TestCoreEnvCorruptConfigFallsBackToStateRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("NOUMON_LIBRARY_DATA", base)
	t.Setenv("POOL_ROOT", "")
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte("{corrupto"), 0o600); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((&supervisor{}).coreEnv(), "\n")
	for _, want := range []string{
		"POOL_ROOT=" + filepath.Join(base, "data"),
		"POOL_PROVIDER=host",
		"DB_PATH=" + filepath.Join(base, "data", "db", "library.db"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q con config corrupto:\n%s", want, joined)
		}
	}
}
