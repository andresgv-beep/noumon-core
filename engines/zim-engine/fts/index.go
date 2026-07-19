package fts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andresgv-beep/zim-engine/analyzer"
	"github.com/andresgv-beep/zim-engine/zim"

	_ "modernc.org/sqlite" // SQLite puro Go, sin CGO — el mismo driver del Core
)

// ErrIncompleteBuild: los errores reales del build superaron el umbral. El índice
// mentiría por omisión, así que no se escribe manifiesto (FTS-AUDIT BUG-2).
var ErrIncompleteBuild = errors.New("fts: build incompleto")

// indexDoc: un artículo listo para insertar. Los campos *_st (stem+fold) se
// calculan en los WORKERS: el stemming es CPU y ahí ya estamos en paralelo; el
// consumidor único solo mete filas.
type indexDoc struct {
	Title   string
	Body    string
	TitleSt string
	BodySt  string
}

// Progress: avance del job de indexado, para pintar en la CLI o el Panel.
type Progress struct {
	Scanned uint32 // entradas recorridas de la path pointer list
	Indexed int    // artículos realmente indexados
	Total   uint32 // total de entradas del .zim
}

// BuildOptions configura el job. Language sale de M/Language del .zim; "" ⇒ sin
// stemmer (solo columnas originales con fold del tokenizador).
//
// StoreBody: guardar el texto del artículo en docs.body permite snippets
// resaltados directamente del índice; con false solo se indexa (la FTS5 es
// contentless: el índice invertido existe igual) y el snippet se regenera
// leyendo el artículo del .zim al mostrar — mismo camino que
// fillMissingPreviews del shim. A diferencia de bleve, aquí el coste de
// StoreBody es UNA copia del texto (docs.body), no term vectors aparte.
type BuildOptions struct {
	Language   string
	BatchSize  int // 0 ⇒ 512; tamaño de transacción de inserción
	StoreBody  bool
	OnProgress func(Progress)
}

// Build crea (reconstruyendo desde cero) el índice FTS5 en dir a partir del
// archive. Devuelve cuántos artículos se indexaron. Cancelable por ctx (§17): un
// job largo se corta limpio si se retira la colección o se apaga el proceso.
func Build(ctx context.Context, a zim.Archive, dir string, opts BuildOptions) (int, error) {
	// ctx interno cancelable: un fallo del insertador debe poder cortar workers
	// y dispatcher aunque el llamante no cancele.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.BatchSize <= 0 {
		opts.BatchSize = 512
	}
	lang := analyzer.NormLang(opts.Language)

	// Publicación atómica (INDEXER-CRASH-SAFETY.md, Capa 1): se construye en un
	// directorio APARTE y solo al final se cambia por el bueno de golpe. Así el
	// índice vivo NUNCA se toca durante el build → un apagón a mitad no lo corrompe
	// ni lo pierde. Reconcile primero, por si un swap anterior quedó a medias:
	// restaura el índice bueno ANTES de tocar nada (si no, lo pisaríamos).
	if err := Reconcile(dir); err != nil {
		return 0, err
	}
	building := buildingDir(dir)
	if err := os.RemoveAll(building); err != nil { // restos de un build anterior
		return 0, err
	}
	if err := os.MkdirAll(building, 0o755); err != nil {
		return 0, err
	}

	db, err := openBuildDB(building)
	if err != nil {
		return 0, err
	}
	// El .db con journal OFF no sobrevive a un corte — da igual: un corte deja
	// building sin manifiesto, que Reconcile reconoce como basura y borra.

	// Namespace de artículos según esquema: 'C' moderno, 'A' legacy (§13).
	artNS := byte('A')
	if a.Capabilities().NewNamespaces {
		artNS = 'C'
	}

	total := a.EntryCount()

	// Pipeline paralelo: lo caro es la extracción (abrir entrada + descomprimir +
	// parsear HTML) y ahora también el stemming, CPU-bound → pool de workers. La
	// inserción SQL va por un ÚNICO consumidor. El dispatch va por RANGOS
	// contiguos de la path pointer list: entradas vecinas comparten cluster, así
	// cada worker explota la LRU en vez de pelearse por descomprimir lo mismo.
	//
	// Nota de determinismo: con inserción reordenada (abajo) el .db es
	// reproducible en contenido para el mismo ZIM y opciones; el layout de
	// páginas de SQLite no se garantiza byte a byte. Mismo criterio que con
	// bleve: el gate de determinismo §5 aplica al índice de títulos, no al FTS.
	const chunk = 64
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8 // el insertador y el disco saturan antes; más workers = RAM inútil
	}

	type doc struct {
		id string
		d  indexDoc
	}
	// Cada chunk viaja con su número de secuencia y el consumidor REORDENA: los
	// docs entran a SQLite en el orden de la path pointer list aunque la
	// extracción sea paralela — inserción determinista, independiente del número
	// de workers. El buffer de pendientes queda acotado por construcción: como
	// mucho hay (workers + cap(jobs)) chunks en vuelo fuera de orden.
	type job struct {
		seq    int
		lo, hi uint32
	}
	type chunkResult struct {
		seq  int
		docs []doc
	}
	jobs := make(chan job, workers)
	results := make(chan chunkResult, workers)

	var scanned atomic.Uint32
	// FTS-AUDIT BUG-2: los errores dejan de tragarse en silencio. Se cuentan por
	// tipo, y al final el build FALLA si los errores reales superan el umbral —
	// un índice a medias que se declara completo es peor que no tener índice.
	var candidates, skipped, failed atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				out := make([]doc, 0, 8)
				for i := j.lo; i < j.hi; i++ {
					scanned.Add(1)
					e, err := a.EntryAtIndex(i)
					if err != nil {
						failed.Add(1) // error de lectura real (dirent ilegible)
						continue
					}
					if e.IsRedirect() || e.Key().Namespace != artNS || !isHTML(e.MimeType()) {
						continue // no es candidato: ni cuenta ni es error
					}
					candidates.Add(1)
					rc, _, err := e.Open(ctx)
					if err != nil {
						if ctx.Err() != nil {
							return // cancelación: no es un fallo del ZIM
						}
						failed.Add(1) // cluster ilegible, límite §16…
						continue
					}
					body := extractText(rc)
					rc.Close()
					if body == "" {
						skipped.Add(1) // sin texto útil (vacías, solo-imagen): legítimo
						continue
					}
					title := e.Title()
					out = append(out, doc{e.FullPath(), indexDoc{
						Title:   title,
						Body:    body,
						TitleSt: strings.Join(analyzer.Analyze(lang, title), " "),
						BodySt:  strings.Join(analyzer.Analyze(lang, body), " "),
					}})
				}
				select {
				case results <- chunkResult{j.seq, out}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		seq := 0
		for lo := uint32(0); lo < total; lo += chunk {
			hi := lo + chunk
			if hi > total {
				hi = total
			}
			select {
			case jobs <- job{seq, lo, hi}:
				seq++
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() { wg.Wait(); close(results) }()

	// Insertador: transacciones de opts.BatchSize docs. docs.body solo si
	// StoreBody; la fila FTS5 (contentless) lleva rowid=docs.id para que la
	// búsqueda pueda hacer JOIN.
	ins := &inserter{db: db, storeBody: opts.StoreBody, batch: opts.BatchSize}

	indexed, lastReport := 0, 0
	next := 0
	pending := map[int][]doc{}
	var indexErr error
consume:
	for r := range results {
		pending[r.seq] = r.docs
		for {
			ds, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			next++
			for _, d := range ds {
				if err := ins.add(d.id, d.d); err != nil {
					// FTS-AUDIT BUG-2: un fallo del insertador no es "un artículo
					// menos" — es el índice roto (disco lleno, .db corrupto).
					indexErr = fmt.Errorf("insert(%s): %w", d.id, err)
					cancel() // corta workers y dispatcher
					break consume
				}
				indexed++
			}
			if opts.OnProgress != nil && indexed-lastReport >= 500 {
				lastReport = indexed
				opts.OnProgress(Progress{Scanned: scanned.Load(), Indexed: indexed, Total: total})
			}
		}
	}
	// Drena results para que los workers no queden bloqueados enviando.
	for range results {
	}

	tally := Tally{
		Candidates: int(candidates.Load()),
		Indexed:    indexed,
		Skipped:    int(skipped.Load()),
		Failed:     int(failed.Load()),
	}

	// Si el insertador falló o el ctx cortó el job, un índice a medias no vale
	// nada: cerrar el .db, borrar el directorio y reportar la causa real.
	abort := func() {
		db.Close()
		os.RemoveAll(building)
	}
	if indexErr != nil {
		abort()
		return indexed, indexErr
	}
	if err := ctx.Err(); err != nil {
		abort()
		return indexed, err
	}

	// CERO artículos indexados = no hay índice que escribir. Colecciones de solo
	// vídeo/JS (p. ej. cursos interactivos) no tienen texto extraíble: la
	// colección queda en búsqueda por título y ya. (Con FTS5 no hay el panic de
	// Close-con-0-docs del Builder de bleve; el criterio de no publicar un
	// índice vacío se conserva igualmente.)
	if indexed == 0 {
		abort()
		return 0, nil
	}

	// Umbral de completitud (FTS-AUDIT BUG-2): si los errores REALES superan el
	// 1% de los candidatos, el índice miente por omisión → no merece manifiesto.
	// (Skipped no cuenta: una página sin texto es legítima, no un fallo.)
	if tally.Candidates > 0 && tally.Failed*100 > tally.Candidates {
		abort()
		return indexed, fmt.Errorf("%w: %d de %d candidatos fallaron (>1%%): índice incompleto, no se escribe manifiesto",
			ErrIncompleteBuild, tally.Failed, tally.Candidates)
	}

	// Cierre = el merge final: 'optimize' consolida los b-trees de FTS5 en su
	// forma más compacta (equivalente al Close del Builder de bleve). Aún en el
	// directorio de construcción, no en el vivo.
	if err := ins.finish(); err != nil {
		abort()
		return indexed, err
	}
	if _, err := db.Exec(`INSERT INTO fts(fts) VALUES('optimize')`); err != nil {
		abort()
		return indexed, fmt.Errorf("optimize: %w", err)
	}
	if err := db.Close(); err != nil {
		return indexed, err
	}

	// El manifiesto va DESPUÉS del cierre: su presencia certifica build completo
	// Y honesto (el tally viaja dentro). Se escribe en building; a partir de
	// aquí, "building con manifiesto" = índice listo para publicar (lo que
	// Reconcile reconoce y promueve si un corte pilla el swap por medio).
	if err := writeManifest(building, newManifest(a, opts, tally)); err != nil {
		return indexed, err
	}

	// Cambio atómico building → dir. AQUÍ, y solo aquí, se sustituye el índice
	// vivo, de una vez y con el nuevo ya entero y sellado (Capa 1).
	if err := promote(building, dir); err != nil {
		return indexed, err
	}

	if opts.OnProgress != nil {
		opts.OnProgress(Progress{Scanned: total, Indexed: indexed, Total: total})
	}
	return indexed, nil
}

// inserter agrupa las inserciones en transacciones de `batch` documentos con
// statements preparados. add/finish son de un solo goroutine (el consumidor).
type inserter struct {
	db        *sql.DB
	storeBody bool
	batch     int

	tx      *sql.Tx
	insDoc  *sql.Stmt
	insFts  *sql.Stmt
	inBatch int
}

func (n *inserter) begin() error {
	tx, err := n.db.Begin()
	if err != nil {
		return err
	}
	insDoc, err := tx.Prepare(`INSERT INTO docs(path, title, body) VALUES(?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	insFts, err := tx.Prepare(`INSERT INTO fts(rowid, title, body, title_st, body_st) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	n.tx, n.insDoc, n.insFts, n.inBatch = tx, insDoc, insFts, 0
	return nil
}

func (n *inserter) add(path string, d indexDoc) error {
	if n.tx == nil {
		if err := n.begin(); err != nil {
			return err
		}
	}
	var body any // NULL si no se almacena; el índice invertido lo lleva igual
	if n.storeBody {
		body = d.Body
	}
	res, err := n.insDoc.Exec(path, d.Title, body)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := n.insFts.Exec(id, d.Title, d.Body, d.TitleSt, d.BodySt); err != nil {
		return err
	}
	n.inBatch++
	if n.inBatch >= n.batch {
		return n.finish()
	}
	return nil
}

func (n *inserter) finish() error {
	if n.tx == nil {
		return nil
	}
	n.insDoc.Close()
	n.insFts.Close()
	err := n.tx.Commit()
	n.tx = nil
	return err
}

func isHTML(mime string) bool {
	return strings.HasPrefix(mime, "text/html")
}

// AnalyzerName devuelve el nombre descriptivo del análisis que se usaría para
// un código de idioma dado. Expuesto para diagnóstico (la CLI lo muestra al
// indexar) y guardado en el manifiesto.
func AnalyzerName(lang string) string {
	l := analyzer.NormLang(lang)
	if analyzer.HasStemmer(l) {
		return "fts5/unicode61+snowball(" + l + ")"
	}
	return "fts5/unicode61"
}
