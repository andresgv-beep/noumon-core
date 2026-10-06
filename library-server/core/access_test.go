package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// El agujero original: el gate de edad protegía los ZIM (/content/*) pero el
// contenido descargado se servía a cualquiera. Estos tests lo fijan.

func accessTestServer(t *testing.T, root string) (*Server, *mediaDeps) {
	t.Helper()
	st, err := openStore(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	t.Cleanup(func() { st.db.Close() })
	return &Server{store: st}, &mediaDeps{root: root}
}

func setAccess(t *testing.T, s *Server, collection, level string, minAge int) {
	t.Helper()
	_, err := s.store.db.Exec(
		`INSERT INTO collection_access (collection_id, access, min_age, updated) VALUES (?,?,?,?)
		 ON CONFLICT(collection_id) DO UPDATE SET access=excluded.access, min_age=excluded.min_age`,
		collectionIDForMedia(collection), level, minAge, time.Now().Unix())
	if err != nil {
		t.Fatalf("setAccess: %v", err)
	}
	s.invalidateAccessCache() // el test escribe por SQL directo; el PUT real invalida igual
}

func getAs(h http.Handler, path string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAccessCacheExpiryBuildsOnceForConcurrentReaders(t *testing.T) {
	s, _ := accessTestServer(t, t.TempDir())
	setAccess(t, s, "Publica", "open", 0)
	id := collectionIDForMedia("Publica")
	if !s.canSeeCached(nil, s.accessMap(), id) { // primera construcción
		t.Fatal("la colección abierta no se cargó")
	}
	before := s.accessBuilds.Load()
	s.accessCacheMu.Lock()
	s.accessCachedAt = time.Now().Add(-accessCacheTTL - time.Second)
	s.accessCacheMu.Unlock()

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if !s.canSeeCached(nil, s.accessMap(), id) {
				errs <- "lector no vio la colección abierta"
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
	if got := s.accessBuilds.Load() - before; got != 1 {
		t.Fatalf("reconstrucciones concurrentes = %d; quiero exactamente 1", got)
	}
}

func TestConcurrentWarmMediaRangesStayOffSQLite(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Moments/Canal", "video.mp4", strings.Repeat("0123456789", 100),
		sidecar{Template: "video", Title: "Vídeo", Source: "moments"})
	s, media := accessTestServer(t, root)
	setAccess(t, s, "Moments/Canal", "login", 0)
	cookie := sessionFor(t, s, "streamer", 30, false)
	mux := http.NewServeMux()
	s.registerMediaRoutes(mux, media)

	requestRange := func() int {
		r := httptest.NewRequest(http.MethodGet, "/media/Moments/Canal/video.mp4", nil)
		r.Header.Set("Range", "bytes=0-31")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if code := requestRange(); code != http.StatusPartialContent {
		t.Fatalf("Range de calentamiento: %d", code)
	}
	sessionMisses := s.sessMisses.Load()
	accessBuilds := s.accessBuilds.Load()

	start := make(chan struct{})
	var wg sync.WaitGroup
	codes := make(chan int, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes <- requestRange()
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusPartialContent {
			t.Errorf("Range concurrente: %d", code)
		}
	}
	if got := s.sessMisses.Load(); got != sessionMisses {
		t.Fatalf("los Range calientes tocaron resolución SQLite: misses antes=%d después=%d", sessionMisses, got)
	}
	if got := s.accessBuilds.Load(); got != accessBuilds {
		t.Fatalf("los Range calientes reconstruyeron permisos: builds antes=%d después=%d", accessBuilds, got)
	}
}

// Los BYTES: /media/<ruta> de una colección bloqueada no se sirven ni al anónimo
// ni a un usuario sin edad. Esto es lo que estaba abierto de par en par.
func TestMediaFileRespectsCollectionAccess(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Adultos", "peli.mp4", "fake mp4",
		sidecar{Template: "video", Title: "Peli"})
	s, md := accessTestServer(t, root)

	mux := http.NewServeMux()
	s.registerMediaRoutes(mux, md)
	const file = "/media/Adultos/peli.mp4"

	// Sin fila de acceso → blocked por defecto.
	if code := getAs(mux, file, nil).Code; code != http.StatusForbidden {
		t.Fatalf("anónimo · colección bloqueada: quiero 403, tengo %d", code)
	}

	// open + edad mínima 18: el crío (12) no pasa, el adulto (40) sí.
	setAccess(t, s, "Adultos", "open", 18)
	nino := sessionFor(t, s, "critico", 12, false)
	adulto := sessionFor(t, s, "andres", 40, false)

	if code := getAs(mux, file, nino).Code; code != http.StatusForbidden {
		t.Fatalf("usuario de 12 en colección 18+: quiero 403, tengo %d", code)
	}
	if code := getAs(mux, file, nil).Code; code != http.StatusForbidden {
		t.Fatalf("anónimo en colección con edad mínima: quiero 403, tengo %d", code)
	}
	if code := getAs(mux, file, adulto).Code; code != http.StatusOK {
		t.Fatalf("usuario de 40 en colección 18+: quiero 200, tengo %d", code)
	}
}

// La LISTA no debe enseñar ni el título de lo que no se puede abrir.
func TestMediaListFiltersByAccess(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Publico", "doc.pdf", "fake pdf", sidecar{Template: "pdf", Title: "Manual"})
	seedCollection(t, root, "Adultos", "peli.mp4", "fake mp4", sidecar{Template: "video", Title: "Peli"})
	s, md := accessTestServer(t, root)
	setAccess(t, s, "Publico", "open", 0)
	setAccess(t, s, "Adultos", "open", 18)

	mux := http.NewServeMux()
	s.registerMediaRoutes(mux, md)

	decode := func(rec *httptest.ResponseRecorder) []mediaItem {
		var payload struct {
			Items []mediaItem `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return payload.Items
	}

	items := decode(getAs(mux, "/api/media", nil))
	if len(items) != 1 || items[0].Title != "Manual" {
		t.Fatalf("anónimo debería ver solo 'Manual', veo: %+v", items)
	}

	nino := sessionFor(t, s, "critico", 12, false)
	if items := decode(getAs(mux, "/api/media", nino)); len(items) != 1 {
		t.Fatalf("usuario de 12: quiero 1 item, tengo %d", len(items))
	}

	adulto := sessionFor(t, s, "andres", 40, false)
	if items := decode(getAs(mux, "/api/media", adulto)); len(items) != 2 {
		t.Fatalf("usuario de 40: quiero 2 items, tengo %d", len(items))
	}
}

// Pedir la colección por su ID (que es adivinable: base64 del nombre) era la
// puerta de atrás del filtro de la lista.
func TestCollectionByIDRespectsAccess(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Adultos", "peli.mp4", "fake mp4", sidecar{Template: "video", Title: "Peli"})
	s, md := accessTestServer(t, root)
	s.kiwixSem = make(chan struct{}, 1)
	s.searchGate = make(chan struct{}, 1)

	mux := http.NewServeMux()
	s.registerItemRoutes(mux, md)

	id := collectionIDForMedia("Adultos")
	if code := getAs(mux, "/api/collections/"+id, nil).Code; code != http.StatusForbidden {
		t.Fatalf("GET colección bloqueada por ID: quiero 403, tengo %d", code)
	}
	if code := getAs(mux, "/api/collections/"+id+"/items", nil).Code; code != http.StatusForbidden {
		t.Fatalf("GET items de colección bloqueada: quiero 403, tengo %d", code)
	}
}

// /api/items/{id}/open devuelve la URL del fichero: si no se comprueba aquí, el
// usuario se lleva la dirección exacta de lo que no puede ver.
func TestItemOpenRespectsAccess(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Adultos", "peli.mp4", "fake mp4", sidecar{Template: "video", Title: "Peli"})
	s, md := accessTestServer(t, root)

	items, err := md.scan("")
	if err != nil || len(items) != 1 {
		t.Fatalf("scan: %v (%d items)", err, len(items))
	}
	item := mediaToItem(items[0])

	mux := http.NewServeMux()
	s.registerItemRoutes(mux, md)

	if code := getAs(mux, "/api/items/"+item.ID+"/open", nil).Code; code != http.StatusForbidden {
		t.Fatalf("open de item bloqueado: quiero 403, tengo %d", code)
	}

	setAccess(t, s, "Adultos", "open", 0)
	if code := getAs(mux, "/api/items/"+item.ID+"/open", nil).Code; code != http.StatusOK {
		t.Fatalf("open de item abierto: quiero 200, tengo %d", code)
	}
}

// La resolución viva de un itemRef usa este endpoint. Un 403 puede indicar que
// el snapshot enlaza contenido restringido, pero su respuesta no puede incluir
// el título vigente ni ningún otro metadato del Item.
func TestItemResolutionDoesNotLeakRestrictedMetadata(t *testing.T) {
	root := t.TempDir()
	seedCollection(t, root, "Privado", "informe.pdf", "fake pdf",
		sidecar{Template: "pdf", Title: "Título vivo confidencial"})
	s, md := accessTestServer(t, root)
	items, err := md.scan("")
	if err != nil || len(items) != 1 {
		t.Fatalf("scan: %v (%d items)", err, len(items))
	}
	item := mediaToItem(items[0])
	mux := http.NewServeMux()
	s.registerItemRoutes(mux, md)

	blocked := getAs(mux, "/api/items/"+item.ID, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("resolución restringida: quiero 403, tengo %d", blocked.Code)
	}
	if strings.Contains(blocked.Body.String(), item.Title) ||
		strings.Contains(blocked.Body.String(), item.CollectionID) {
		t.Fatalf("la resolución restringida filtró metadatos: %s", blocked.Body.String())
	}

	setAccess(t, s, "Privado", "open", 0)
	visible := getAs(mux, "/api/items/"+item.ID, nil)
	if visible.Code != http.StatusOK || !strings.Contains(visible.Body.String(), item.Title) {
		t.Fatalf("la resolución autorizada no devolvió el Item: %d %s",
			visible.Code, visible.Body.String())
	}
}

// Un fichero suelto en la raíz de DOWNLOAD_ROOT: su "colección" es ".", y tiene
// que resolverse igual que la calcula media.go/toItem (si no, el gate miraría
// una fila que no existe y se abriría o cerraría por accidente).
func TestMediaCollectionForRelMatchesToItem(t *testing.T) {
	cases := map[string]string{
		"Biblioteca/Libros/linux.pdf": "Biblioteca/Libros",
		"Publico/doc.pdf":             "Publico",
		"suelto.pdf":                  ".",
	}
	for rel, want := range cases {
		if got := mediaCollectionForRel(rel); got != want {
			t.Fatalf("mediaCollectionForRel(%q) = %q, quiero %q", rel, got, want)
		}
	}
}

// Cabinet y Moments se deciden enteros: el apartado lo dice la ficha, no la
// carpeta (Noumon DL guarda en YouTube/ y Archives/). Una fila vieja de carpeta
// no abre nada si el apartado está cerrado, y lo que mezcla apartados sigue
// por carpeta.
func surfaceTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	writeSidecar(t, root, "Archives/cdl/libro.json", "Libro", "cabinet")
	writeSidecar(t, root, "Archives/otro/libro.json", "Otro", "cabinet")
	writeSidecar(t, root, "YouTube/Canal/video.json", "Video", "moments")
	writeSidecar(t, root, "Mezcla/a.json", "A", "cabinet")
	writeSidecar(t, root, "Mezcla/b.json", "B", "moments")
	s, media := accessTestServer(t, root)
	s.media = media
	return s
}

func insertAccess(t *testing.T, s *Server, id, access string, age, dl int) {
	t.Helper()
	if _, err := s.store.db.Exec(`INSERT INTO collection_access (collection_id, access, min_age, allow_download, updated) VALUES (?,?,?,?,?)`,
		id, access, age, dl, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	s.invalidateAccessCache()
}

func TestSurfaceAccessGovernsItsFolders(t *testing.T) {
	s := surfaceTestServer(t)
	insertAccess(t, s, surfaceAccessCabinet, "blocked", 0, 0)
	insertAccess(t, s, collectionIDForMedia("Archives/cdl"), "open", 0, 0) // fila antigua: ya no manda
	insertAccess(t, s, collectionIDForMedia("Mezcla"), "open", 0, 0)

	if s.canSeeCollectionID(nil, collectionIDForMedia("Archives/cdl")) {
		t.Fatal("una fila de carpeta abrió una colección de un Cabinet bloqueado")
	}
	if s.canSeeMediaPath(nil, "Archives/otro/libro.pdf") {
		t.Fatal("otra colección de Cabinet no heredó el bloqueo del apartado")
	}
	if s.canSeeMediaPath(nil, "YouTube/Canal/video.mp4") {
		t.Fatal("Moments sin fila debe estar bloqueado")
	}
	if !s.canSeeCollectionID(nil, collectionIDForMedia("Mezcla")) {
		t.Fatal("una carpeta que mezcla apartados perdió su propio acceso")
	}

	insertAccess(t, s, surfaceAccessMoments, "open", 0, 0)
	if !s.canSeeMediaPath(nil, "YouTube/Canal/video.mp4") {
		t.Fatal("abrir Moments no abrió sus colecciones")
	}
}

func TestMigrateSurfaceAccessTakesMostRestrictive(t *testing.T) {
	s := surfaceTestServer(t)
	insertAccess(t, s, collectionIDForMedia("Archives/cdl"), "open", 0, 1)
	insertAccess(t, s, collectionIDForMedia("Archives/otro"), "login", 12, 1)
	insertAccess(t, s, collectionIDForMedia("YouTube/Canal"), "open", 0, 1)
	insertAccess(t, s, surfaceAccessMoments, "blocked", 0, 0) // ya decidido: no se toca

	if err := s.migrateSurfaceAccess(); err != nil {
		t.Fatal(err)
	}
	got := s.accessMap()
	if c := got[surfaceAccessCabinet]; c.Access != "login" || c.MinAge != 12 || !c.AllowDownload {
		t.Fatalf("Cabinet migrado = %+v, quiero login/12/descarga", c)
	}
	if c := got[surfaceAccessMoments]; c.Access != "blocked" {
		t.Fatalf("la migración pisó el acceso ya decidido de Moments: %+v", c)
	}
}
