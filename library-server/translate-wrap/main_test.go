package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEngineCommandUsesModelsDir(t *testing.T) {
	oldBin, oldModels := bin, modelsDir
	t.Cleanup(func() { bin, modelsDir = oldBin, oldModels })
	modelsDir = t.TempDir()
	bin = "translateLocally"
	cmd := engineCommand("-l")
	if cmd.Dir != modelsDir {
		t.Fatalf("directorio de trabajo = %q; esperado %q", cmd.Dir, modelsDir)
	}
	key := "XDG_DATA_HOME"
	if os.PathSeparator == '\\' {
		key = "APPDATA"
	}
	found := false
	for _, entry := range cmd.Env {
		if entry == key+"="+modelsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("falta %s en el entorno del motor", key)
	}
}

func TestLinuxEngineUsesStateToolsAndExplicitOverride(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("rutas Linux")
	}
	root := t.TempDir()
	t.Setenv("NOUMON_LIBRARY_DATA", root)
	t.Setenv("TRANSLATE_BIN", "")
	path := filepath.Join(root, "tools", "translateLocally")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := defaultTranslateBin(); got != path {
		t.Fatalf("bin=%s esperado=%s", got, path)
	}
	t.Setenv("TRANSLATE_BIN", "/explicit/engine")
	if got := defaultTranslateBin(); got != "/explicit/engine" {
		t.Fatalf("no se respeto TRANSLATE_BIN: %s", got)
	}
}

func TestLinuxEngineIsHeadlessEvenWithoutModelsOverride(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("entorno Linux")
	}
	oldBin, oldModels := bin, modelsDir
	t.Cleanup(func() { bin, modelsDir = oldBin, oldModels })
	bin, modelsDir = "translateLocally", ""
	t.Setenv("QT_QPA_PLATFORM", "xcb")
	cmd := engineCommand("-l")
	count := 0
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "QT_QPA_PLATFORM=") {
			count++
			if entry != "QT_QPA_PLATFORM=offscreen" {
				t.Fatal(entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("plataformas Qt: %d", count)
	}
}

func TestMigrateLegacyModelsCopiesMissingAndPreservesExisting(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "old")
	destination := filepath.Join(base, "new")
	if err := os.MkdirAll(filepath.Join(source, "pair"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(destination, "pair"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pair", "model.bin"), []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pair", "keep.bin"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "pair", "keep.bin"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	copied, err := migrateLegacyModels(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 {
		t.Fatalf("copiados = %d; esperado 1", copied)
	}
	for path, want := range map[string]string{
		filepath.Join(source, "pair", "model.bin"):      "model",
		filepath.Join(destination, "pair", "model.bin"): "model",
		filepath.Join(destination, "pair", "keep.bin"):  "new",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; esperado %q", path, got, err, want)
		}
	}
}
