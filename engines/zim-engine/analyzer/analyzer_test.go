package analyzer

import (
	"reflect"
	"testing"
)

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"canción":   "cancion",
		"über":      "uber",
		"São Paulo": "Sao Paulo",
		"plain":     "plain",
		"":          "",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize("¡Los GATOS, más de 2 vidas!")
	want := []string{"los", "gatos", "más", "de", "2", "vidas"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize = %v, quería %v", got, want)
	}
}

// El stemmer español debe colapsar las variantes morfológicas a la misma raíz:
// es exactamente el recall que la migración no puede perder respecto a bleve.
func TestStemSpanish(t *testing.T) {
	root := Stem("es", "gato")
	for _, w := range []string{"gatos", "gata", "gatas"} {
		if got := Stem("es", w); got != root {
			t.Errorf("Stem(es, %q) = %q, quería la raíz %q", w, got, root)
		}
	}
	// El stemmer español usa las tildes en su algoritmo y las quita él mismo:
	// "canción" y "canciones" deben converger (por eso Fold va DESPUÉS de Stem).
	if a, b := Fold(Stem("es", "canción")), Fold(Stem("es", "canciones")); a != b {
		t.Errorf("canción/canciones no convergen: %q vs %q", a, b)
	}
}

// Idioma sin stemmer = identidad, nunca error: un ZIM en un idioma exótico se
// indexa igual, solo que sin recall morfológico (paridad con bleve standard).
func TestStemUnknownIsIdentity(t *testing.T) {
	if got := Stem("xx", "palabra"); got != "palabra" {
		t.Errorf("Stem(xx) = %q, quería identidad", got)
	}
}

func TestNormLang(t *testing.T) {
	for in, want := range map[string]string{
		"spa": "es", "eng": "en", "ES-419": "es", "  Fr_CA ": "fr",
		"rus": "ru", "chu": "chu", "de": "de",
	} {
		if got := NormLang(in); got != want {
			t.Errorf("NormLang(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestAnalyzePipeline(t *testing.T) {
	// canción → cancion (stem es + fold); GATOS → gat
	got := Analyze("es", "Canción de los GATOS")
	if len(got) != 4 {
		t.Fatalf("Analyze devolvió %v", got)
	}
	if got[0] != Fold(Stem("es", "canción")) {
		t.Errorf("primer token %q no pasó por stem+fold", got[0])
	}
	for _, tok := range got {
		if tok != Fold(tok) {
			t.Errorf("token %q conserva diacríticos tras Analyze", tok)
		}
	}
}
