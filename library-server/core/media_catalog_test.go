package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// escribe un sidecar mínimo válido (fuente propia) en root/rel.
func writeSidecar(t *testing.T, root, rel, title, source string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	sc := map[string]any{"title": title, "media": "f.bin", "source": source}
	raw, _ := json.Marshal(sc)
	if err := os.WriteFile(abs, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(abs), "f.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMediaCatalogCacheAndInvalidate(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, root, "Moments/General/a.json", "Video A", "moments")
	writeSidecar(t, root, "Cabinet/Libros/b.json", "Libro B", "cabinet")
	m := &mediaDeps{root: root}

	all, err := m.itemsFor("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("esperaba 2 items, hay %d", len(all))
	}

	// Un sidecar nuevo NO aparece hasta invalidar (caché fresca dentro del TTL)…
	writeSidecar(t, root, "Cabinet/Libros/c.json", "Libro C", "cabinet")
	cached, _ := m.itemsFor("")
	if len(cached) != 2 {
		t.Fatalf("la caché debería seguir sirviendo 2 items, sirve %d", len(cached))
	}
	// …y aparece justo después de invalidar (mutación propia: subir/editar/borrar).
	m.invalidate()
	fresh, _ := m.itemsFor("")
	if len(fresh) != 3 {
		t.Fatalf("tras invalidar esperaba 3 items, hay %d", len(fresh))
	}

	// El acotado por colección respeta carpeta e hijas, como el escaneo antiguo.
	cab, _ := m.itemsFor("Cabinet")
	if len(cab) != 2 {
		t.Fatalf("Cabinet debería tener 2 items, tiene %d", len(cab))
	}
	sub, _ := m.itemsFor("Cabinet/Libros")
	if len(sub) != 2 {
		t.Fatalf("Cabinet/Libros debería tener 2 items, tiene %d", len(sub))
	}
	none, _ := m.itemsFor("Cabinet/Otra")
	if len(none) != 0 {
		t.Fatalf("colección inexistente debería dar 0 items, da %d", len(none))
	}
}

func TestMediaCatalogStampede(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		writeSidecar(t, root, filepath.ToSlash(filepath.Join("Moments", "C", string(rune('a'+i))+".json")), "V", "moments")
	}
	m := &mediaDeps{root: root}

	// 50 lectores simultáneos: todos deben terminar con el catálogo completo y
	// sin carreras (el -race del CI vigila la exclusión mutua).
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := m.itemsFor("")
			if err != nil {
				errs <- err
				return
			}
			if len(items) != 20 {
				errs <- os.ErrInvalid
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("lector concurrente falló: %v", err)
	}
}
