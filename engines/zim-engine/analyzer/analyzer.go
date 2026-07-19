// Paquete analyzer: normalización de texto compartida por el índice FTS5 y (a
// futuro) la búsqueda multiidioma (BUSQUEDA-MULTIIDIOMA.md).
//
// Pipeline canónico: Tokenize (minúsculas, unicode) → Stem (Snowball por idioma)
// → Fold (fuera diacríticos). El ORDEN importa: los stemmers Snowball usan las
// vocales acentuadas dentro de su algoritmo (el español, sin ir más lejos, las
// des-acentúa él mismo al final), así que se stemiza sobre la forma original y
// se foldea DESPUÉS — foldear antes cambiaría el resultado del stemmer y
// romperíamos la paridad con bleve, que hace lo mismo por debajo.
//
// Decisión de dependencias (MIGRACION-FTS5.md, vía a): blevesearch/snowballstem
// a pelo — es el código Go generado de los algoritmos Snowball publicados, CERO
// dependencias transitivas, y es exactamente lo que bleve usa por debajo, así
// que el stemming del índice nuevo es idéntico al del viejo. Queda escondido
// tras Stem() para poder sustituirlo idioma a idioma por implementación propia
// sin tocar a nadie.
package analyzer

import (
	"strings"
	"sync"
	"unicode"

	snowball "github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/arabic"
	"github.com/blevesearch/snowballstem/dutch"
	"github.com/blevesearch/snowballstem/english"
	"github.com/blevesearch/snowballstem/french"
	"github.com/blevesearch/snowballstem/german"
	"github.com/blevesearch/snowballstem/italian"
	"github.com/blevesearch/snowballstem/portuguese"
	"github.com/blevesearch/snowballstem/russian"
	"github.com/blevesearch/snowballstem/spanish"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// stemmers: registro por ISO 639-1. Un idioma fuera de esta tabla stemiza a
// identidad — mismo comportamiento que el analizador standard de bleve, que
// indexa sin stemmer. Los nueve de aquí son los mismos nueve de analyzerFor
// del paquete fts.
var stemmers = map[string]func(env *snowball.Env) bool{
	"es": spanish.Stem,
	"en": english.Stem,
	"fr": french.Stem,
	"de": german.Stem,
	"it": italian.Stem,
	"pt": portuguese.Stem,
	"ru": russian.Stem,
	"ar": arabic.Stem,
	"nl": dutch.Stem,
}

// HasStemmer informa si hay stemmer registrado para el idioma (ya normalizado
// a 639-1). Útil para diagnóstico y para el manifiesto.
func HasStemmer(lang string) bool { _, ok := stemmers[lang]; return ok }

// NormLang normaliza un código de idioma a 639-1: primer subtag, minúsculas, y
// los 639-3 más comunes traducidos ("spa"→"es", "es-419"→"es"). Es la misma
// tabla que usaba fts.normLang; vive aquí porque el idioma es cosa del análisis.
func NormLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	if len(lang) == 2 {
		return lang
	}
	switch lang {
	case "spa":
		return "es"
	case "eng":
		return "en"
	case "fra", "fre":
		return "fr"
	case "deu", "ger":
		return "de"
	case "ita":
		return "it"
	case "por":
		return "pt"
	case "rus":
		return "ru"
	case "ara":
		return "ar"
	case "nld", "dut":
		return "nl"
	}
	return lang
}

// foldPool: transformers que eliminan marcas diacríticas (descompone NFD, tira
// las marcas de combinación Mn, recompone NFC). "canción"→"cancion",
// "über"→"uber". x/text ya era dependencia del módulo; no entra nada nuevo aquí.
//
// Va por sync.Pool y NO como un único transformer de paquete: los transformers
// de x/text son STATEFUL (transform.Chain mantiene buffers internos que
// transform.String muta en cada llamada) y NO son seguros para uso concurrente.
// Build stemiza/foldea desde un pool de workers (index.go), así que un foldT
// compartido petaba con "slice bounds out of range" al pisarse el estado entre
// goroutines. Con el pool cada goroutine toma el suyo y lo devuelve.
var foldPool = sync.Pool{
	New: func() any {
		return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	},
}

// Fold devuelve el texto sin diacríticos. Si la transformación fallara (bytes
// inválidos), devuelve la entrada tal cual: mejor indexar con tilde que perder
// el término.
func Fold(s string) string {
	t := foldPool.Get().(transform.Transformer)
	defer foldPool.Put(t)
	out, _, err := transform.String(t, s) // transform.String hace Reset() al entrar
	if err != nil {
		return s
	}
	return out
}

// Tokenize trocea el texto en tokens en minúsculas: secuencias de letras y
// dígitos unicode, todo lo demás es separador. Equivale al tokenizador unicode
// + lowercase de bleve para el texto que nos ocupa (artículos ya extraídos a
// texto plano). Deliberadamente NO quita stop words: FTS5+BM25 ya deprime los
// términos ultracomunes por estadística, y mantener los tokens permite frases
// exactas y NEAR en el backlog. Diferencia asumida frente a bleve (que sí las
// quita); los tests dorados dirán si mueve el ranking de forma apreciable.
func Tokenize(text string) []string {
	out := make([]string, 0, 16)
	var sb strings.Builder
	flush := func() {
		if sb.Len() > 0 {
			out = append(out, sb.String())
			sb.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return out
}

// Stem stemiza un token YA en minúsculas con el stemmer del idioma. Idioma sin
// stemmer ⇒ identidad. El token entra con sus diacríticos (ver nota de orden en
// la cabecera del paquete).
func Stem(lang, token string) string {
	fn, ok := stemmers[lang]
	if !ok || token == "" {
		return token
	}
	env := snowball.NewEnv(token)
	fn(env)
	return env.Current()
}

// Analyze aplica el pipeline completo a un texto: tokenizar → stemizar →
// foldear. Es lo que alimenta las columnas *_st del índice y la variante
// stemizada de la consulta. lang debe venir ya normalizado (NormLang).
func Analyze(lang, text string) []string {
	toks := Tokenize(text)
	for i, t := range toks {
		toks[i] = Fold(Stem(lang, t))
	}
	return toks
}
