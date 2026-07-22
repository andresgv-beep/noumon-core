package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

func TestMediaCatalogInvalidateDuringBuildCannotPublishStaleSnapshot(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, root, "Cabinet/Libros/a.json", "Libro A", "cabinet")
	m := &mediaDeps{root: root}

	scanned := make(chan struct{})
	release := make(chan struct{})
	var hookCalls atomic.Int32
	m.afterScan = func() {
		if hookCalls.Add(1) == 1 {
			close(scanned) // el primer snapshot ya se construyó, pero no se publicó
			<-release
		}
	}

	type result struct {
		items []mediaItem
		err   error
	}
	done := make(chan result, 1)
	go func() {
		items, err := m.itemsFor("")
		done <- result{items: items, err: err}
	}()
	<-scanned
	writeSidecar(t, root, "Cabinet/Libros/b.json", "Libro B", "cabinet")
	m.invalidate()
	close(release)

	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	if len(r.items) != 2 {
		t.Fatalf("se publicó un snapshot viejo: items=%d, quiero 2", len(r.items))
	}
	if builds := m.catalogBuilds.Load(); builds != 2 {
		t.Fatalf("reconstrucciones=%d, quiero 2 (vieja descartada + nueva)", builds)
	}
}

func TestMediaCatalogIndexesProviderIDAndCollectionMetadata(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, root, "Moments/Canal/a.json", "Vídeo", "moments")
	writeSidecar(t, root, "Cabinet/Libros/b.json", "Libro", "cabinet")
	meta, _ := json.Marshal(collectionMeta{Title: "Biblioteca fina", Source: "cabinet"})
	if err := os.WriteFile(filepath.Join(root, "Cabinet", "Libros", "collection.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	m := &mediaDeps{root: root}
	catalog, err := m.catalogSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(catalog.providerItems("moments")); got != 1 {
		t.Fatalf("items Moments=%d, quiero 1", got)
	}
	if got := len(catalog.providerItems("cabinet")); got != 1 {
		t.Fatalf("items Cabinet=%d, quiero 1", got)
	}
	if got := catalog.collections["Cabinet/Libros"].Title; got != "Biblioteca fina" {
		t.Fatalf("título cacheado=%q", got)
	}
	item := catalog.providerItems("cabinet")[0]
	id := itemIDForMedia(strings.Trim(strings.TrimPrefix(item.MediaURL, "/media/"), "/"))
	indexed, ok, err := m.itemForID(id)
	if err != nil || !ok || indexed.Title != "Libro" {
		t.Fatalf("índice por ID: ok=%v err=%v item=%+v", ok, err, indexed)
	}
}

func TestSurfaceItemsUsesMediaSnapshotWithoutZIMCatalog(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, root, "Cabinet/Libros/b.json", "Libro", "cabinet")
	meta, _ := json.Marshal(collectionMeta{Title: "Colección desde RAM", Source: "cabinet"})
	if err := os.WriteFile(filepath.Join(root, "Cabinet", "Libros", "collection.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	s, media := accessTestServer(t, root)
	setAccess(t, s, "Cabinet/Libros", "open", 0)
	// s.kiwix queda nil deliberadamente: el endpoint no debe tocar el catálogo ZIM.
	h := s.handleSurfaceItems(media)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/api/items/surface?provider=cabinet", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("surface: %d %s", w.Code, w.Body.String())
	}
	var payload struct {
		Items []struct {
			Title       string `json:"title"`
			SectionName string `json:"sectionName"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Title != "Libro" || payload.Items[0].SectionName != "Colección desde RAM" {
		t.Fatalf("respuesta inesperada: %+v", payload.Items)
	}
}
