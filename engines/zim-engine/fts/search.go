package fts

import (
	"database/sql"
	"fmt"

	"github.com/andresgv-beep/zim-engine/analyzer"
	"github.com/andresgv-beep/zim-engine/zim"
)

// Hit: un resultado full-text. Path es el FullPath de la entrada ("C/Saturno"), que
// el shim recorta a la ruta pública. Snippet ya viene resaltado con <mark>…</mark>
// alrededor de los términos (el shim lo reutiliza o lo limpia, como con kiwix).
type Hit struct {
	Path    string
	Title   string
	Snippet string
	Score   float64
}

// Index: un índice FTS5 abierto para consulta. Seguro para uso concurrente
// (database/sql gestiona su pool; el .db se abre en solo lectura).
type Index struct {
	db       *sql.DB
	manifest Manifest
	lang     string // normalizado, del manifiesto: gobierna stemming de consulta
}

// Open abre un índice ya construido en dir y VERIFICA que corresponde a ese
// archive (FTS-AUDIT BUG-1). Es la cerradura del camino de apertura: escribir un
// manifiesto no es verificarlo, igual que esconder un botón no es un permiso.
// Sin esta comprobación, copiar el .db equivocado al pool sirve resultados
// fantasma sin una sola queja — y todo el diseño de índices distribuibles
// (INDEXER.md) depende de que esto no pueda pasar.
//
// Errores:
//   - manifiesto ausente  → build interrumpido o índice pre-manifiesto: reindexar
//   - Matches falla       → índice de otro ZIM, otro entryCount u otro esquema
//     (los índices bleve de la era anterior caen aquí: schema v1 ≠ v2 → reindexar)
func Open(dir string, a zim.Archive) (*Index, error) {
	// Reconcile primero (INDEXER-CRASH-SAFETY.md, Capa 2): tras un arranque sucio
	// puede haber un build completo sin cambiar (.new) que hay que promover, o
	// restos de un swap a medias que limpiar. En estado sano son un par de stat()
	// sin efecto. Así "abrir" cura un corte anterior sin intervención.
	if err := Reconcile(dir); err != nil {
		return nil, fmt.Errorf("reconciliar índice tras arranque sucio: %w", err)
	}
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("índice sin manifiesto válido (¿build a medias?): %w", err)
	}
	if err := m.Matches(a); err != nil {
		return nil, err
	}
	db, err := openReadDB(dir)
	if err != nil {
		return nil, err
	}
	return &Index{db: db, manifest: m, lang: analyzer.NormLang(m.Language)}, nil
}

// Manifest devuelve el manifiesto con el que se abrió el índice (ya verificado).
func (i *Index) Manifest() Manifest { return i.manifest }

func (i *Index) Close() error { return i.db.Close() }

// DocCount devuelve cuántos artículos hay indexados.
func (i *Index) DocCount() (uint64, error) {
	var n uint64
	err := i.db.QueryRow(`SELECT count(*) FROM docs`).Scan(&n)
	return n, err
}

// Search ejecuta la consulta full-text y devuelve los hits ordenados por rank
// (BM25) y el total de coincidencias. El match sobre título va potenciado ×3
// vía los pesos de bm25() — mismo boost que el titleQ de la era bleve. Esto
// deja el orden fino al scoring del shim, igual que hoy con kiwix.
//
// OJO con el signo: bm25() de FTS5 devuelve valores NEGATIVOS (más negativo =
// más relevante; se ordena ASC). Aquí se voltea a positivo (-bm25) en el SQL,
// de una vez y para siempre: Hit.Score es "más grande = mejor", como con bleve,
// y el prior de enlaces de MEJORAS-BUSQUEDA.md podrá multiplicar sobre un score
// positivo sin trampas de signo.
func (i *Index) Search(query string, limit int) ([]Hit, uint64, error) {
	if limit <= 0 {
		limit = 10
	}
	if buildMatchQuery(i.lang, query, matchAnd) == "" { // sin ningún token (solo puntuación…)
		return nil, 0, nil
	}
	origToks := analyzer.Tokenize(query)
	stemToks := analyzer.Analyze(i.lang, query)

	// AND primero (precisión); si no casa nada, OR (rescate de recall). Para una
	// consulta de un solo término AND==OR, así que no hay segunda pasada.
	for _, mode := range [...]matchMode{matchAnd, matchOr} {
		match := buildMatchQuery(i.lang, query, mode)
		hits, total, err := i.runMatch(match, limit, origToks, stemToks)
		if err != nil {
			return nil, 0, err
		}
		if total > 0 || mode == matchOr {
			return hits, total, nil
		}
	}
	return nil, 0, nil
}

// runMatch ejecuta una expresión MATCH concreta: arma los hits (con snippet
// propio si el índice guardó body) y el total de coincidencias.
//
// Pesos bm25 por columna en el orden del esquema: title, body, title_st,
// body_st. Título ×3 en ambas formas.
func (i *Index) runMatch(match string, limit int, origToks, stemToks []string) ([]Hit, uint64, error) {
	rows, err := i.db.Query(`
		SELECT d.path, d.title, COALESCE(d.body, ''), -bm25(fts, 3.0, 1.0, 3.0, 1.0) AS score
		FROM fts JOIN docs d ON d.id = fts.rowid
		WHERE fts MATCH ?
		ORDER BY score DESC
		LIMIT ?`, match, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("fts match: %w", err)
	}
	defer rows.Close()

	hits := make([]Hit, 0, limit)
	for rows.Next() {
		var h Hit
		var body string
		if err := rows.Scan(&h.Path, &h.Title, &body, &h.Score); err != nil {
			return nil, 0, err
		}
		// Snippet propio sobre el body original almacenado (fts5.go). Sin
		// StoreBody body viene vacío y el snippet queda "" — el shim lo rellena
		// leyendo el .zim, mismo contrato que con bleve sin term vectors.
		h.Snippet = makeSnippet(i.lang, body, origToks, stemToks)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Total de coincidencias (no solo las devueltas), para el "N resultados" de
	// la UI — bleve lo regalaba en res.Total; FTS5 lo cobra con un count aparte.
	var total uint64
	if err := i.db.QueryRow(`SELECT count(*) FROM fts WHERE fts MATCH ?`, match).Scan(&total); err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}
