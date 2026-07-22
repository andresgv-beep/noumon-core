// Banco de pruebas de multiusuario para Noumon.
//
// Por qué existe: "aguanta N personas" no significa nada si no se dice haciendo
// qué y con qué criterio. Aquí el criterio es el del usuario, no el del
// servidor: un espectador está bien atendido si el vídeo NO se le corta. Por eso
// la métrica principal no son milisegundos sino CORTES (underruns): veces que el
// reproductor se quedó sin búfer y tuvo que parar a recargar.
//
// Cada espectador virtual imita a un reproductor real, que es lo que distingue
// esta medida de un martillo de carga: pide por delante hasta llenar unos
// segundos de búfer y luego SE FRENA, igual que hace un <video>. Descargar a
// toda velocidad mediría el ancho de banda del disco, no la experiencia.
//
// Uso típico:
//
//	# ¿aguanta esta máquina 4 espectadores sin cortes?
//	go run . -host 192.168.1.50:8090 -users 4 -dur 60s -user andres -pass ...
//
//	# ¿cuál es su techo? sube usuarios hasta que aparece el primer corte
//	go run . -host 192.168.1.50:8090 -ramp -dur 45s -user andres -pass ...
//
// Para medir la Pi desde otro equipo, compilar y llevarlo o apuntar por red:
//
//	GOOS=linux GOARCH=arm64 go build -o bench-arm64 .
//
// Se mide SIEMPRE contra la máquina real y por la red real: medir en localhost
// esconde precisamente el cuello de botella que importa (wifi y disco).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	host      = flag.String("host", "127.0.0.1:8090", "host:puerto del servidor Noumon")
	users     = flag.Int("users", 4, "espectadores simultáneos")
	dur       = flag.Duration("dur", 60*time.Second, "duración de cada nivel")
	bitrate   = flag.Float64("bitrate", 3.0, "Mbps por espectador (3≈720p, 6≈1080p)")
	mediaPath = flag.String("path", "", "ruta /media/... a reproducir (vacío = autodetectar)")
	username  = flag.String("user", "", "usuario (necesario si el contenido no es abierto)")
	password  = flag.String("pass", "", "contraseña")
	searchers = flag.Int("searchers", 0, "cuántos de los espectadores además buscan (0 = ninguno)")
	ramp      = flag.Bool("ramp", false, "subir espectadores hasta el primer corte para hallar el techo")
	rampStep  = flag.Int("step", 2, "incremento de espectadores por nivel en -ramp")
	rampMax   = flag.Int("max", 32, "tope de espectadores en -ramp")
)

const (
	chunkBytes    = 512 << 10 // lo que pide un reproductor por tramo
	bufferTargetS = 12.0      // segundos de vídeo que el player mantiene por delante
	rebufferS     = 3.0       // tras un corte, espera a tener esto antes de reanudar
)

func base() string { return "http://" + *host }

// ── Resultado de un nivel ──────────────────────────────────────────────────

type result struct {
	users        int
	stalls       int64 // CORTES: la métrica que importa
	stalledView  int   // espectadores que sufrieron al menos un corte
	bytes        int64
	lat          []time.Duration // latencia por tramo
	searchLat    []time.Duration
	search429    int64
	searchErr    int64
	errs         int64
	wall         time.Duration
	discardChunk int64
}

func pct(v []time.Duration, q float64) time.Duration {
	if len(v) == 0 {
		return 0
	}
	return v[int(float64(len(v)-1)*q)]
}

// ── Espectador virtual ─────────────────────────────────────────────────────

// viewer imita un reproductor: mantiene un búfer en segundos, pide por delante
// hasta llenarlo y se frena cuando está lleno. Si el búfer llega a cero mientras
// "reproduce", eso es un corte y se contabiliza.
func viewer(client *http.Client, path string, size int64, deadline time.Time, res *result, mu *sync.Mutex) {
	chunkSecs := float64(chunkBytes) * 8 / (*bitrate * 1e6)

	var playhead, buffered float64 // segundos
	stalled := false
	myStalls := 0
	offset := int64(0)
	last := time.Now()

	for time.Now().Before(deadline) {
		now := time.Now()
		if !stalled {
			playhead += now.Sub(last).Seconds()
		}
		last = now

		// ¿Se agotó el búfer mientras reproducía? Eso es un corte real.
		if !stalled && playhead > buffered {
			stalled = true
			myStalls++
			playhead = buffered
		}
		if stalled && buffered-playhead >= rebufferS {
			stalled = false
		}

		// Player educado: con el búfer lleno no pide más (como un <video> real).
		if buffered-playhead >= bufferTargetS {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if offset >= size {
			offset = 0 // vuelve al principio: sesión larga sin quedarse sin fichero
		}
		end := offset + chunkBytes - 1
		if end >= size {
			end = size - 1
		}

		req, _ := http.NewRequest(http.MethodGet, base()+path, nil)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
		t0 := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			atomic.AddInt64(&res.errs, 1)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		n, _ := io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lat := time.Since(t0)

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			atomic.AddInt64(&res.errs, 1)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		atomic.AddInt64(&res.bytes, n)
		mu.Lock()
		res.lat = append(res.lat, lat)
		mu.Unlock()

		buffered += chunkSecs
		offset = end + 1
	}

	if myStalls > 0 {
		mu.Lock()
		res.stalledView++
		mu.Unlock()
	}
	atomic.AddInt64(&res.stalls, int64(myStalls))
}

// searcher: algunos espectadores además buscan, que es donde vive el único
// límite que devuelve error visible (429) en vez de simplemente ir más lento.
func searcher(client *http.Client, deadline time.Time, res *result, mu *sync.Mutex) {
	terms := []string{"historia", "agua", "zelda", "españa", "medicina", "arte"}
	i := 0
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		if !time.Now().Before(deadline) {
			return
		}
		q := terms[i%len(terms)]
		i++
		t0 := time.Now()
		resp, err := client.Get(base() + "/api/search?q=" + url.QueryEscape(q))
		if err != nil {
			atomic.AddInt64(&res.searchErr, 1)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lat := time.Since(t0)
		if resp.StatusCode == http.StatusTooManyRequests {
			atomic.AddInt64(&res.search429, 1)
			continue
		}
		mu.Lock()
		res.searchLat = append(res.searchLat, lat)
		mu.Unlock()
	}
}

// ── Un nivel de carga ──────────────────────────────────────────────────────

func level(n int, path string, size int64, cookies []*http.Cookie) result {
	res := result{users: n}
	var mu sync.Mutex
	deadline := time.Now().Add(*dur)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		// Un cliente por espectador: conexiones propias, como equipos distintos.
		jar := &simpleJar{cookies: cookies}
		c := &http.Client{
			Timeout:   60 * time.Second,
			Jar:       jar,
			Transport: &http.Transport{MaxIdleConnsPerHost: 4},
		}
		wg.Add(1)
		go func() { defer wg.Done(); viewer(c, path, size, deadline, &res, &mu) }()
		if i < *searchers {
			wg.Add(1)
			go func() { defer wg.Done(); searcher(c, deadline, &res, &mu) }()
		}
	}
	wg.Wait()
	res.wall = time.Since(start)

	sort.Slice(res.lat, func(a, b int) bool { return res.lat[a] < res.lat[b] })
	sort.Slice(res.searchLat, func(a, b int) bool { return res.searchLat[a] < res.searchLat[b] })
	return res
}

func (r result) line() string {
	mbps := float64(r.bytes) * 8 / r.wall.Seconds() / 1e6
	verdict := "OK sin cortes"
	if r.stalls > 0 {
		verdict = fmt.Sprintf("CORTES: %d (%d de %d espectadores)", r.stalls, r.stalledView, r.users)
	}
	s := fmt.Sprintf("%3d espectadores | %6.1f Mbps | tramo p50 %-7v p95 %-7v p99 %-7v | %s",
		r.users, mbps, pct(r.lat, .5).Round(time.Millisecond),
		pct(r.lat, .95).Round(time.Millisecond), pct(r.lat, .99).Round(time.Millisecond), verdict)
	if len(r.searchLat) > 0 || r.search429 > 0 {
		s += fmt.Sprintf("\n     búsqueda: p95 %v · 429 = %d",
			pct(r.searchLat, .95).Round(time.Millisecond), r.search429)
	}
	if r.errs > 0 {
		s += fmt.Sprintf("\n     errores de red/HTTP: %d", r.errs)
	}
	return s
}

// ── Arranque: login, descubrir contenido, medir ────────────────────────────

type simpleJar struct{ cookies []*http.Cookie }

func (j *simpleJar) SetCookies(_ *url.URL, c []*http.Cookie) {}
func (j *simpleJar) Cookies(_ *url.URL) []*http.Cookie       { return j.cookies }

func login() []*http.Cookie {
	if *username == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"username": *username, "password": *password})
	req, _ := http.NewRequest(http.MethodPost, base()+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", base()) // el servidor exige Origin == Host en escrituras
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "login: %d %s\n", resp.StatusCode, strings.TrimSpace(string(b)))
		os.Exit(1)
	}
	return resp.Cookies()
}

// discover elige el fichero más grande que el usuario pueda ver, prefiriendo
// vídeo: es el contenido que de verdad estresa el streaming.
func discover(cookies []*http.Cookie) (string, int64) {
	c := &http.Client{Timeout: 30 * time.Second, Jar: &simpleJar{cookies: cookies}}
	resp, err := c.Get(base() + "/api/media")
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo listar el pool: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var payload struct {
		Items []struct {
			Template string `json:"template"`
			MediaURL string `json:"media_url"`
		} `json:"items"`
	}
	json.NewDecoder(resp.Body).Decode(&payload)

	best, bestSize := "", int64(0)
	for _, it := range payload.Items {
		if it.MediaURL == "" {
			continue
		}
		size := headSize(c, it.MediaURL)
		if size <= 0 {
			continue
		}
		score := size
		if it.Template == "video" {
			score *= 4 // preferimos vídeo aunque haya un PDF mayor
		}
		if score > bestSize {
			best, bestSize = it.MediaURL, score
		}
	}
	if best == "" {
		fmt.Fprintln(os.Stderr, "no hay contenido visible para este usuario; usa -path o -user/-pass")
		os.Exit(1)
	}
	return best, headSize(c, best)
}

func headSize(c *http.Client, path string) int64 {
	req, _ := http.NewRequest(http.MethodHead, base()+path, nil)
	resp, err := c.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	return resp.ContentLength
}

func main() {
	flag.Parse()
	cookies := login()

	path, size := *mediaPath, int64(0)
	if path == "" {
		path, size = discover(cookies)
	} else {
		size = headSize(&http.Client{Timeout: 30 * time.Second, Jar: &simpleJar{cookies: cookies}}, path)
	}
	if size <= 0 {
		fmt.Fprintln(os.Stderr, "no se pudo leer el tamaño del contenido")
		os.Exit(1)
	}

	dec, _ := url.PathUnescape(path)
	fmt.Printf("Noumon · banco de multiusuario\n")
	fmt.Printf("servidor : %s\n", *host)
	fmt.Printf("contenido: %s (%.1f MB)\n", strings.TrimPrefix(dec, "/media/"), float64(size)/(1<<20))
	fmt.Printf("criterio : cada espectador consume %.1f Mbps; un CORTE es quedarse sin búfer\n\n", *bitrate)

	if !*ramp {
		fmt.Println(level(*users, path, size, cookies).line())
		return
	}

	fmt.Println("Subiendo carga hasta el primer corte...")
	lastClean := 0
	for n := *rampStep; n <= *rampMax; n += *rampStep {
		r := level(n, path, size, cookies)
		fmt.Println(r.line())
		if r.stalls > 0 || r.errs > 0 {
			fmt.Printf("\nTECHO: %d espectadores simultáneos sin cortes (falla a partir de %d).\n", lastClean, n)
			return
		}
		lastClean = n
		time.Sleep(2 * time.Second) // deja respirar cachés y disco entre niveles
	}
	fmt.Printf("\nTECHO: no se encontró; aguantó %d espectadores sin un solo corte.\n", lastClean)
}
