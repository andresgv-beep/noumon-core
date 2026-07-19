package fts

// Capa SQLite/FTS5 del índice (MIGRACION-FTS5.md). Sustituye a bleve manteniendo
// la API pública del paquete intacta: Build/Open/Search/Hit/Manifest/DocCount.
//
// Esquema — dos tablas, y la decisión clave es que la FTS5 es CONTENTLESS:
//
//	docs  tabla normal: path, title, body (NULL si !StoreBody) y links (prior
//	      de enlaces de MEJORAS-BUSQUEDA.md; hoy 0, mañana un UPDATE barato en
//	      una tabla normal — un UPDATE sobre una fila FTS5 es internamente
//	      delete+reinsert de la fila entera, y con 2M de artículos eso es
//	      reindexar Wikipedia otra vez).
//	fts   FTS5 content='': SOLO el índice invertido, sin texto almacenado. El
//	      rowid casa con docs.id. Esto resuelve de golpe el riesgo de tamaño de
//	      las columnas duplicadas del diseño original (el texto se guarda una
//	      vez en docs, no cuatro) y hace irrelevante el problema de offsets de
//	      snippet() sobre columnas stemizadas: los snippets los generamos
//	      nosotros sobre docs.body (el "plan B" del doc, elegido de entrada).
//
// Columnas de fts: title/body con la forma ORIGINAL (el tokenizador unicode61
// remove_diacritics 2 foldea al tokenizar, tanto el índice como la consulta,
// SIN tocar el texto — por eso docs guarda el original con sus tildes) y
// title_st/body_st con la forma stem+fold del paquete analyzer, para el recall
// morfológico. La consulta se lanza como OR de ambos grupos.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/andresgv-beep/zim-engine/analyzer"
)

// dbName: el fichero del índice dentro del directorio. El directorio (con su
// manifiesto y su swap atómico) sigue siendo la unidad de publicación.
const dbName = "fts.db"

func dbPath(dir string) string { return filepath.Join(dir, dbName) }

const schemaSQL = `
CREATE TABLE docs (
	id    INTEGER PRIMARY KEY,
	path  TEXT NOT NULL,
	title TEXT NOT NULL,
	body  TEXT,
	links INTEGER NOT NULL DEFAULT 0
);
CREATE VIRTUAL TABLE fts USING fts5(
	title, body, title_st, body_st,
	content = '',
	tokenize = 'unicode61 remove_diacritics 2'
);
`

// openBuildDB abre el .db de construcción con las prisas puestas: el corpus es
// estático y el directorio entero se descarta si el build muere (Capa 1 del
// swap atómico), así que la durabilidad intermedia no compra nada.
func openBuildDB(dir string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath(dir))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // un solo escritor; modernc no quiere más
	for _, p := range []string{
		"PRAGMA journal_mode=OFF",
		"PRAGMA synchronous=OFF",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-65536", // 64 MiB de page cache durante el build
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma build: %w", err)
		}
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("crear esquema: %w", err)
	}
	return db, nil
}

// openReadDB abre un índice ya publicado, solo lectura.
func openReadDB(dir string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath(dir)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA query_only=ON"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// matchMode elige cómo se combinan los términos de la consulta entre sí.
type matchMode int

const (
	matchAnd matchMode = iota // todos los términos (precisión) — el defecto
	matchOr                   // cualquier término (recall) — solo como rescate
)

// buildMatchQuery arma la expresión MATCH. Cada término de la consulta se busca
// en su forma ORIGINAL (columnas title/body; unicode61 foldea diacríticos por
// su cuenta) O en su RAÍZ (columnas *_st, pipeline analyzer), y los términos se
// combinan según mode.
//
// matchAnd (defecto): un documento debe contener CADA palabra. El OR puro de
// antes (heredado del MatchQuery de bleve) traía cualquier cosa con UNA sola
// palabra común: "historia de napoleón" devolvía todo lo que tuviera "historia".
// Search cae a matchOr solo si el AND no casa nada, para no perder recall en
// consultas cuyos términos no coexisten en ningún artículo.
//
// Cada token viaja entre comillas dobles: para FTS5 una cadena entrecomillada es
// SIEMPRE un término literal, así que la sintaxis de consulta del usuario (AND,
// NOT, paréntesis, asteriscos) no puede inyectarse ni romper el parser.
func buildMatchQuery(lang, query string, mode matchMode) string {
	orig := analyzer.Tokenize(query)
	if len(orig) == 0 {
		return ""
	}
	stem := analyzer.Analyze(lang, query) // alineado 1:1 con orig (mismo Tokenize)

	quote := func(t string) string { return `"` + strings.ReplaceAll(t, `"`, `""`) + `"` }

	// term(i): término i en original (title/body) con caída a su raíz en *_st si
	// difiere (idioma sin stemmer o palabra ya en raíz ⇒ solo original).
	term := func(i int) string {
		o := `{title body}: ` + quote(orig[i])
		if i < len(stem) && stem[i] != "" && stem[i] != orig[i] {
			return `(` + o + ` OR {title_st body_st}: ` + quote(stem[i]) + `)`
		}
		return o
	}

	sep := " AND "
	if mode == matchOr {
		sep = " OR "
	}
	parts := make([]string, len(orig))
	for i := range orig {
		parts[i] = term(i)
	}
	return strings.Join(parts, sep)
}

// ── Snippets ────────────────────────────────────────────────────────────────

// snippetRadius: palabras de contexto a cada lado del primer término que casa.
const snippetRadius = 12

// snippetScanCap: tope de palabras del body a examinar buscando el término. Un
// artículo de 4 MiB no puede costarnos un stemming palabra a palabra entero
// por resultado; si el término vive más allá, el snippet degrada al arranque
// del artículo, que es lo que hace también la búsqueda-dentro del shim.
const snippetScanCap = 4000

// makeSnippet genera el fragmento resaltado sobre el body ORIGINAL. Busca la
// primera palabra cuyo fold casa con un token original de la consulta o cuyo
// stem+fold casa con un token stemizado (así el snippet funciona también
// cuando el match fue puramente morfológico), recorta una ventana de palabras
// alrededor y envuelve cada palabra que casa en <mark>…</mark> — el mismo
// contrato que devolvía el highlight de bleve, que el shim ya sabe consumir.
func makeSnippet(lang, body string, origToks, stemToks []string) string {
	if body == "" {
		return ""
	}
	origSet := make(map[string]bool, len(origToks))
	for _, t := range origToks {
		origSet[analyzer.Fold(t)] = true
	}
	stemSet := make(map[string]bool, len(stemToks))
	for _, t := range stemToks {
		stemSet[t] = true
	}

	words := strings.Fields(body)
	if len(words) == 0 {
		return ""
	}

	matches := func(w string) bool {
		toks := analyzer.Tokenize(w) // limpia puntuación pegada: "(gatos)," → gatos
		for _, t := range toks {
			if origSet[analyzer.Fold(t)] {
				return true
			}
			if len(stemSet) > 0 && stemSet[analyzer.Fold(analyzer.Stem(lang, t))] {
				return true
			}
		}
		return false
	}

	first := -1
	limit := len(words)
	if limit > snippetScanCap {
		limit = snippetScanCap
	}
	for i := 0; i < limit; i++ {
		if matches(words[i]) {
			first = i
			break
		}
	}

	lo, hi := 0, len(words)
	if first >= 0 {
		lo = first - snippetRadius
		hi = first + snippetRadius + 1
	} else {
		hi = 2*snippetRadius + 1 // sin match localizable: arranque del artículo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(words) {
		hi = len(words)
	}

	var sb strings.Builder
	if lo > 0 {
		sb.WriteString("… ")
	}
	for i := lo; i < hi; i++ {
		if i > lo {
			sb.WriteByte(' ')
		}
		if first >= 0 && matches(words[i]) {
			sb.WriteString("<mark>")
			sb.WriteString(words[i])
			sb.WriteString("</mark>")
		} else {
			sb.WriteString(words[i])
		}
	}
	if hi < len(words) {
		sb.WriteString(" …")
	}
	return sb.String()
}
