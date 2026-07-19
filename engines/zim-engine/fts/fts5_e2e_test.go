package fts

// Tests de extremo a extremo del motor FTS5 (MIGRACION-FTS5.md): Build sobre un
// Archive de mentira con artículos HTML en español, Open verificado, y las
// propiedades de búsqueda que la migración NO puede perder respecto a bleve:
// recall morfológico (stemming), insensibilidad a tildes, boost de título,
// snippets con <mark>, DocCount y total. Son la versión sintética de los tests
// dorados; la paridad contra un ZIM REAL se comprueba con zimtool (fase 5).

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresgv-beep/zim-engine/zim"
)

// mockEntry: una entrada HTML de artículo.
type mockEntry struct {
	path  string // sin namespace: "Gato"
	title string
	html  string
}

func (m mockEntry) Key() zim.EntryKey                    { return zim.EntryKey{Namespace: 'C', Path: m.path} }
func (m mockEntry) FullPath() string                     { return "C/" + m.path }
func (m mockEntry) Title() string                        { return m.title }
func (m mockEntry) IsRedirect() bool                     { return false }
func (m mockEntry) RedirectTarget() (zim.EntryKey, bool) { return zim.EntryKey{}, false }
func (m mockEntry) MimeType() string                     { return "text/html" }
func (m mockEntry) Open(context.Context) (io.ReadCloser, zim.BlobInfo, error) {
	return io.NopCloser(strings.NewReader(m.html)), zim.BlobInfo{}, nil
}

// mockArchive implementa lo que Build/Open tocan; el resto hereda panics del
// zim.Archive nil embebido (si Build empieza a llamar algo nuevo, el test lo
// grita en vez de fingir).
type mockArchive struct {
	zim.Archive
	entries []mockEntry
	uuid    [16]byte
}

func (a mockArchive) EntryCount() uint32 { return uint32(len(a.entries)) }
func (a mockArchive) UUID() [16]byte     { return a.uuid }
func (a mockArchive) Capabilities() zim.Capabilities {
	return zim.Capabilities{NewNamespaces: true}
}
func (a mockArchive) EntryAtIndex(i uint32) (zim.Entry, error) {
	return a.entries[i], nil
}

func art(paragraphs ...string) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for _, p := range paragraphs {
		sb.WriteString("<p>" + p + "</p>")
	}
	sb.WriteString("</body></html>")
	return sb.String()
}

func testArchive() mockArchive {
	var u [16]byte
	u[0] = 0x42
	return mockArchive{uuid: u, entries: []mockEntry{
		{"Gato", "Gato", art(
			"El gato doméstico es un mamífero carnívoro.",
			"Los gatos conviven con los humanos desde hace milenios.")},
		{"Perro", "Perro", art(
			"El perro es el mejor amigo del hombre.",
			"Algunos perros persiguen gatos por instinto.")},
		{"Canción", "Canción", art(
			"Una canción es una composición musical con letra.",
			"Las canciones populares se transmiten oralmente.")},
		{"Saturno", "Saturno", art(
			"Saturno es el planeta de los anillos.")},
	}}
}

func buildAndOpen(t *testing.T, storeBody bool) (*Index, mockArchive) {
	t.Helper()
	a := testArchive()
	dir := filepath.Join(t.TempDir(), "idx")
	n, err := Build(context.Background(), a, dir, BuildOptions{
		Language: "es", StoreBody: storeBody, BatchSize: 2, // lotes minúsculos a propósito
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if n != len(a.entries) {
		t.Fatalf("Build indexó %d, quería %d", n, len(a.entries))
	}
	idx, err := Open(dir, a)
	if err != nil {
		t.Fatalf("Open tras Build: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx, a
}

// Recall morfológico: "gatos" (plural) debe encontrar el artículo "Gato" — es
// la promesa del stemming que bleve daba y FTS5+analyzer tiene que mantener.
func TestE2EStemmingRecall(t *testing.T) {
	idx, _ := buildAndOpen(t, true)
	hits, total, err := idx.Search("gatos", 10)
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 || len(hits) < 2 {
		t.Fatalf("'gatos' debería casar Gato y Perro: hits=%d total=%d", len(hits), total)
	}
	if hits[0].Path != "C/Gato" {
		t.Errorf("primer hit de 'gatos' = %s, quería C/Gato (boost de título)", hits[0].Path)
	}
}

// Tildes: la consulta sin tilde encuentra el artículo con tilde, y el título
// devuelto conserva su forma ORIGINAL (el fold vive en el tokenizador y en las
// columnas _st, nunca en el texto almacenado — el arreglo de los snippets).
func TestE2EAccentInsensitiveKeepsOriginals(t *testing.T) {
	idx, _ := buildAndOpen(t, true)
	hits, _, err := idx.Search("cancion", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("'cancion' (sin tilde) no encontró 'Canción'")
	}
	if hits[0].Title != "Canción" {
		t.Errorf("título devuelto %q perdió la tilde: el fold contaminó el almacenado", hits[0].Title)
	}
	if !strings.Contains(hits[0].Snippet, "<mark>") {
		t.Errorf("snippet sin resaltado: %q", hits[0].Snippet)
	}
	if !strings.Contains(hits[0].Snippet, "canción") && !strings.Contains(hits[0].Snippet, "canciones") {
		t.Errorf("snippet no contiene el término original con tilde: %q", hits[0].Snippet)
	}
}

// Boost de título ×3: un artículo con el término EN EL TÍTULO gana a uno que
// solo lo menciona en el cuerpo.
func TestE2ETitleBoost(t *testing.T) {
	idx, _ := buildAndOpen(t, true)
	hits, _, err := idx.Search("perro", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Path != "C/Perro" {
		t.Fatalf("'perro' debería devolver C/Perro primero, fue: %+v", hits)
	}
}

// Score positivo y descendente: el contrato de Hit.Score no cambia con bm25()
// (que internamente es negativo). Si esto se rompe, el prior de enlaces de
// MEJORAS-BUSQUEDA.md multiplicaría con el signo cambiado.
func TestE2EScoresPositiveDescending(t *testing.T) {
	idx, _ := buildAndOpen(t, true)
	hits, _, err := idx.Search("gatos", 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range hits {
		if h.Score <= 0 {
			t.Errorf("hit %d con score no positivo: %f", i, h.Score)
		}
		if i > 0 && hits[i-1].Score < h.Score {
			t.Errorf("scores desordenados: %f < %f", hits[i-1].Score, h.Score)
		}
	}
}

// Sin StoreBody: la búsqueda funciona igual (la FTS5 contentless indexa todo)
// pero el snippet queda vacío — el shim lo rellena del .zim, como siempre.
func TestE2ENoStoreBody(t *testing.T) {
	idx, _ := buildAndOpen(t, false)
	hits, _, err := idx.Search("anillos", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Path != "C/Saturno" {
		t.Fatalf("'anillos' sin StoreBody debería encontrar Saturno: %+v", hits)
	}
	if hits[0].Snippet != "" {
		t.Errorf("snippet debería ser vacío sin StoreBody, fue %q", hits[0].Snippet)
	}
}

func TestE2EDocCountAndManifest(t *testing.T) {
	idx, a := buildAndOpen(t, true)
	n, err := idx.DocCount()
	if err != nil || n != uint64(len(a.entries)) {
		t.Fatalf("DocCount = %d (%v), quería %d", n, err, len(a.entries))
	}
	m := idx.Manifest()
	if m.Schema != SchemaVersion || m.Indexed != len(a.entries) || m.Failed != 0 {
		t.Fatalf("manifiesto raro: %+v", m)
	}
}

// La sintaxis de FTS5 en manos del usuario no puede romper la consulta: los
// tokens van entrecomillados, así que operadores y comillas son literales.
func TestE2EQuerySyntaxCannotInject(t *testing.T) {
	idx, _ := buildAndOpen(t, true)
	for _, q := range []string{
		`gato AND`, `NOT perro`, `"gato`, `(((`, `gato*`, `-`, `col0:x`, `""`,
	} {
		if _, _, err := idx.Search(q, 5); err != nil {
			t.Errorf("Search(%q) devolvió error de sintaxis: %v", q, err)
		}
	}
	// Y una consulta sin ningún token no explota ni devuelve todo el corpus.
	hits, total, err := idx.Search("¡¿!?", 5)
	if err != nil || len(hits) != 0 || total != 0 {
		t.Errorf("consulta vacía: hits=%d total=%d err=%v", len(hits), total, err)
	}
}
