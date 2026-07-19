package fts

// Set de evaluación de relevancia (MEJORAS-BUSQUEDA.md §0): una red de seguridad
// para el ranking. Construye un índice sobre un corpus sintético que reproduce el
// caso real que motivó el AND ("historia de napoleón" NO debe traer artículos con
// solo "historia" — los "Pokémon"), lanza el lote de consultas de
// testdata/eval_queries.tsv y falla si hit@3 o MRR bajan del umbral registrado.
//
// No sustituye a los tests dorados contra un ZIM real (zimtool); es el guardarraíl
// automático que corre en cada `go test`.

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Umbrales registrados: si un cambio de ranking/consulta los baja, el test falla.
// Suben (nunca bajan) según se afina el motor (p. ej. tras el prior de enlaces).
const (
	evalMinHitAt3 = 1.0  // las 10 consultas curadas deben tener su esperado en top-3
	evalMinMRR    = 0.90 // casi todas en el puesto 1 (boost de título)
)

// evalArchive: corpus de prueba. Incluye a propósito DOS "decoys" con la palabra
// común "historia(s)" pero SIN "napoleón" (Clases de Historia, Campeonato de
// historias) — el equivalente sintético de los artículos Pokémon que el OR
// colaba y el AND debe excluir.
func evalArchive() mockArchive {
	var u [16]byte
	u[0] = 0x77
	return mockArchive{uuid: u, entries: []mockEntry{
		{"Napoleón Bonaparte", "Napoleón Bonaparte", art(
			"Napoleón Bonaparte fue un militar y estadista francés, emperador de los franceses.",
			"La historia de Napoleón y las guerras napoleónicas transformaron Europa.")},
		{"Revolución Francesa", "Revolución Francesa", art(
			"La Revolución Francesa fue un proceso social y político que cambió la historia de Francia.",
			"De la revolución surgió más tarde Napoleón.")},
		{"Clases de Historia", "Clases de Historia", art(
			"Las clases de historia enseñan los acontecimientos del pasado en la escuela.",
			"Es una materia escolar común en la educación.")},
		{"Campeonato de historias de entrenadores", "Campeonato de historias de entrenadores", art(
			"Un campeonato de historias de entrenadores es un evento con relatos populares.")},
		{"Energía solar", "Energía solar", art(
			"La energía solar aprovecha la radiación del sol para generar electricidad limpia.")},
		{"Sistema operativo", "Sistema operativo", art(
			"Un sistema operativo es el software que gestiona los recursos de una computadora.")},
		{"Gato", "Gato", art(
			"El gato doméstico es un mamífero carnívoro que convive con los humanos.")},
	}}
}

func buildEval(t *testing.T) *Index {
	t.Helper()
	a := evalArchive()
	dir := filepath.Join(t.TempDir(), "idx")
	if _, err := Build(context.Background(), a, dir, BuildOptions{Language: "es", StoreBody: true, BatchSize: 4}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx, err := Open(dir, a)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

type evalCase struct{ query, want string }

func readEvalCases(t *testing.T) []evalCase {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "eval_queries.tsv"))
	if err != nil {
		t.Fatalf("abrir eval_queries.tsv: %v", err)
	}
	defer f.Close()
	var cases []evalCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("línea mal formada (falta TAB): %q", line)
		}
		cases = append(cases, evalCase{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("leer TSV: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("el set de evaluación está vacío")
	}
	return cases
}

// rankOf devuelve la posición (1-based) de want en los hits, o 0 si no está.
func rankOf(hits []Hit, want string) int {
	for i, h := range hits {
		if h.Path == want {
			return i + 1
		}
	}
	return 0
}

func TestRelevanceEval(t *testing.T) {
	idx := buildEval(t)
	cases := readEvalCases(t)

	hit3 := 0
	rrSum := 0.0
	for _, c := range cases {
		hits, _, err := idx.Search(c.query, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", c.query, err)
		}
		rank := rankOf(hits, c.want)
		switch {
		case rank == 0:
			t.Errorf("[%q] esperado %q NO aparece en los resultados", c.query, c.want)
		case rank > 3:
			t.Errorf("[%q] esperado %q en el puesto %d (fuera de top-3)", c.query, c.want, rank)
		default:
			hit3++
			rrSum += 1.0 / float64(rank)
		}
	}

	hitAt3 := float64(hit3) / float64(len(cases))
	mrr := rrSum / float64(len(cases))
	t.Logf("relevancia: hit@3=%.3f  MRR=%.3f  (%d consultas)", hitAt3, mrr, len(cases))
	if hitAt3 < evalMinHitAt3 {
		t.Errorf("hit@3=%.3f por debajo del umbral %.3f", hitAt3, evalMinHitAt3)
	}
	if mrr < evalMinMRR {
		t.Errorf("MRR=%.3f por debajo del umbral %.3f", mrr, evalMinMRR)
	}
}

// TestRelevanceAndExcludesDecoys fija la regresión que motivó todo: una consulta
// multipalabra NO puede traer artículos que solo tengan la palabra común.
func TestRelevanceAndExcludesDecoys(t *testing.T) {
	idx := buildEval(t)
	hits, _, err := idx.Search("historia de napoleón", 10)
	if err != nil {
		t.Fatal(err)
	}
	decoys := []string{"C/Clases de Historia", "C/Campeonato de historias de entrenadores"}
	for _, d := range decoys {
		if r := rankOf(hits, d); r != 0 {
			t.Errorf("decoy %q apareció (puesto %d) para 'historia de napoleón'; el AND debía excluirlo", d, r)
		}
	}
	// …pero sí debe traer el artículo que cubre ambos términos.
	if rankOf(hits, "C/Napoleón Bonaparte") == 0 {
		t.Error("'historia de napoleón' debería traer C/Napoleón Bonaparte")
	}
}
