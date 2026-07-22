// Noumon — shim (library-api)
//
// Envuelve kiwix-serve (motor headless, GPL) y expone una API REST JSON limpia
// a los clientes (UI Svelte, panel Noumon, CLI). Ver DESIGN §2, §7.
//
// Licencia limpia: este binario solo habla HTTP con kiwix-serve, NO linka libzim.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andresgv-beep/noumon/download"
)

// Server mantiene la conexión con el motor y la config del shim.
type Server struct {
	kiwix   *url.URL
	proxy   *httputil.ReverseProxy
	http    *http.Client
	token   string  // token de sesión Noumon. Vacío = auth desactivada (dev LAN).
	store   *Store  // capa de gestión persistida (favoritos, notas, historial)
	geo     *sql.DB // índice FTS5 de geocoding (plugin Maps); nil si no hay geo.db
	geoPath string
	mapsDir string
	geoMu   sync.RWMutex

	// lanPrivate: el proceso escucha en la red pero la biblioteca NO está
	// publicada (paquete servidor headless con lanAccess=false). El middleware
	// cierra el plano de lectura a los remotos y deja abierto el de
	// administración: despublicar nunca deja fuera al admin (§Red).
	lanPrivate bool

	// Cachés del camino caliente de /media (RENDIMIENTO-STREAMING §7): el vídeo
	// y el PDF generan cientos de Ranges por reproducción y cada uno revalidaba
	// sesión + permiso contra SQLite, serializado en UNA conexión. Se cachean
	// los INSUMOS (sesión y mapa de acceso) con TTL corto; el gate se sigue
	// ejecutando en cada petición, solo que contra memoria.
	sessCacheMu sync.RWMutex
	sessCache   map[string]sessionCacheEntry

	accessCacheMu  sync.RWMutex
	accessCache    map[string]accessCfg
	accessCachedAt time.Time

	// Control de carga (§6, rate-limit v1): kiwixSem limita las peticiones
	// simultáneas al motor; searchGate limita búsquedas globales concurrentes
	// (/api/search y /api/images son fan-out caro sobre Xapian → DoS barato).
	// searchWaiters acota la cola de espera del gate (v1.1): los que no caben
	// en el gate esperan slot (event-driven, el canal despierta al liberar);
	// si la cola también está llena, 429 inmediato.
	kiwixSem      chan struct{}
	searchGate    chan struct{}
	searchWaiters atomic.Int32

	// Cache LRU de respuestas de búsqueda: repetir una búsqueda es instantáneo,
	// no consume slot del gate ni golpea el motor (mata el "frío" de Xapian,
	// DESIGN §1.4). TTL corto por si la biblioteca cambia en caliente.
	searchCache *lruCache

	// Traductor (TRANSLATE.md): motor opcional detrás del shim. translate es su
	// URL base ("" = desactivado → la UI oculta el toggle). translateGate acota
	// las traducciones concurrentes (CPU-caras, igual que la búsqueda global).
	translate        string
	translateGate    chan struct{}
	translateWaiters atomic.Int32

	// Motor ZIM nativo (zim_native.go, ZIM-ENGINE.md §5.2). nil = camino kiwix
	// de siempre. Toggle: ZIM_ENGINE=kiwix (default) | native; ambos caminos
	// conviven hasta el final del plan de retirada — rollback = 1 env var.
	zimNative *nativeZims
	zimAdmin  *adminZim
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// siblingDir devuelve una carpeta junto al binario (o relativa al cwd como fallback).
func siblingDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), name)
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return name
}

func main() {
	// Subcomando de datos: construir el índice de geocoding y salir.
	//   library-shim buildgeo <overpass.json> <geo.db> [geonames-cp.txt] [prefijos-cp]
	// prefijos-cp: provincias de los códigos postales, separados por coma
	// (por defecto Catalunya: "08,17,25,43"). Parametrizado para packs de región.
	if len(os.Args) >= 2 && os.Args[1] == "buildgeo" {
		if len(os.Args) < 4 {
			log.Fatal("uso: library-shim buildgeo <overpass.json> <geo.db> [geonames-cp.txt] [prefijos-cp]")
		}
		geonames := ""
		if len(os.Args) >= 5 {
			geonames = os.Args[4]
		}
		prefixes := "08,17,25,43"
		if len(os.Args) >= 6 {
			prefixes = os.Args[5]
		}
		if err := buildGeoIndex(os.Args[2], os.Args[3], geonames, prefixes); err != nil {
			log.Fatalf("buildgeo: %v", err)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "buildgeonames" {
		if len(os.Args) != 4 {
			log.Fatal("uso: library-shim buildgeonames <cities500.zip> <geo.db>")
		}
		if _, err := buildGeoNamesCitiesIndex(os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("buildgeonames: %v", err)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "indexstreets" {
		if len(os.Args) != 5 {
			log.Fatal("uso: library-shim indexstreets <map.pmtiles> <streets.db> <geo.db>")
		}
		last := int64(-1)
		count, err := buildStreetIndexFromPMTiles(context.Background(), os.Args[2], os.Args[3], os.Args[4], func(done, total, streets int64, zoom int) {
			if done/1000 != last/1000 || done == total {
				log.Printf("calles: %d/%d teselas, %d entradas, z%d", done, total, streets, zoom)
				last = done
			}
		})
		if err != nil {
			log.Fatalf("indexstreets: %v", err)
		}
		log.Printf("calles: indice listo con %d entradas", count)
		return
	}

	kiwixURL := env("KIWIX_URL", "http://127.0.0.1:8080")
	port := env("PORT", "8090")
	bind := env("BIND", "127.0.0.1") // dev: solo loopback. Container/NAS: 0.0.0.0 (LAN, §6).
	token := strings.TrimSpace(os.Getenv("NOUMON_TOKEN"))
	if err := validateMachineToken(token); err != nil {
		log.Fatal(err)
	}

	ku, err := url.Parse(kiwixURL)
	if err != nil {
		log.Fatalf("KIWIX_URL inválida: %v", err)
	}

	// Pool de almacenamiento (POOL-CONTRACT.md): raíz única de datos. Si POOL_ROOT
	// está definido, las rutas se derivan de él; cada env suelta sigue como override
	// puntual; sin POOL_ROOT se conservan los defaults de hoy (cero regresión, §5).
	poolRoot := os.Getenv("POOL_ROOT")
	dbPath := resolvePoolPath("DB_PATH", poolRoot, "db/library.db", "data/library.db")
	store, err := openStore(dbPath)
	if err != nil {
		log.Fatalf("no se pudo abrir la base de datos (%s): %v", dbPath, err)
	}

	// Índice de geocoding del plugin Maps (opcional: solo si existe geo.db).
	var geo *sql.DB
	geoPath := resolvePoolPath("GEO_DB", poolRoot, "maps/geo.db", filepath.Join(siblingDir("mapdata"), "geo.db"))
	mapsDir := filepath.Dir(geoPath)
	if _, statErr := os.Stat(geoPath); statErr == nil {
		if g, gerr := sql.Open("sqlite", geoPath); gerr == nil {
			geo = g
			log.Printf("maps: geocoder activo (%s)", geoPath)
		}
	}

	// Concurrencia máxima contra el motor (kiwix), escalada a la máquina:
	// 2×núcleos, acotado a [6, 32]. En un Xeon la búsqueda global vive de la
	// paralelización (15 ZIMs en ~3s); en la Pi el disco manda y menos es más.
	// Override manual con KIWIX_CONCURRENCY (en la Pi con ZIMs grandes: 4-6).
	kiwixConc := runtime.NumCPU() * 2
	if kiwixConc < 6 {
		kiwixConc = 6
	}
	if kiwixConc > 32 {
		kiwixConc = 32
	}
	if v, err := strconv.Atoi(env("KIWIX_CONCURRENCY", "")); err == nil && v > 0 {
		kiwixConc = v
	}
	log.Printf("motor: concurrencia máxima %d (KIWIX_CONCURRENCY para ajustar)", kiwixConc)

	// Puerta de búsquedas globales, escalada como el semáforo: núcleos/2,
	// acotado [2, 6]. Pi 4/5 (4 núcleos) → 2, como el valor fijo original;
	// máquinas grandes suben solas. Override con SEARCH_CONCURRENCY.
	// Defectos provisionales hasta medir en hardware real (DISCIPLINE v2.1).
	searchConc := runtime.NumCPU() / 2
	if searchConc < 2 {
		searchConc = 2
	}
	if searchConc > 6 {
		searchConc = 6
	}
	if v, err := strconv.Atoi(env("SEARCH_CONCURRENCY", "")); err == nil && v > 0 {
		searchConc = v
	}
	log.Printf("búsqueda: máx. %d globales simultáneas + cola de %d (SEARCH_CONCURRENCY para ajustar)",
		searchConc, searchQueueMax)

	// Traductor opcional (sidecar detrás del shim, patrón Maps). Vacío = off.
	// Concurrencia baja: modelos NMT son CPU-caros; en la Pi compiten con Xapian.
	translateURL := strings.TrimRight(env("TRANSLATE_URL", ""), "/")
	translateConc := runtime.NumCPU() / 2
	if translateConc < 1 {
		translateConc = 1
	}
	if translateConc > 4 {
		translateConc = 4
	}
	if v, err := strconv.Atoi(env("TRANSLATE_CONCURRENCY", "")); err == nil && v > 0 {
		translateConc = v
	}

	s := &Server{
		kiwix:         ku,
		proxy:         httputil.NewSingleHostReverseProxy(ku),
		http:          &http.Client{Timeout: 30 * time.Second},
		token:         token,
		store:         store,
		geo:           geo,
		geoPath:       geoPath,
		mapsDir:       mapsDir,
		kiwixSem:      make(chan struct{}, kiwixConc),
		searchGate:    make(chan struct{}, searchConc),
		searchCache:   newLRUCache(128, 10*time.Minute),
		translate:     translateURL,
		translateGate: make(chan struct{}, translateConc),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/libraries", s.handleLibraries)
	mux.HandleFunc("/api/libraries/", s.handleLibrarySub) // /{id}/search, /{id}/fulltext
	mux.HandleFunc("/api/search", s.handleGlobalSearch)   // búsqueda global cross-ZIM (home)
	// /api/images se registra más abajo (necesita md para las portadas/logos de vídeos)
	mux.HandleFunc("/api/favorites", s.handleFavorites) // gestión persistida (SQLite)
	mux.HandleFunc("/api/notes", s.handleNotes)
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/recent", s.handleRecent)
	mux.HandleFunc("/api/tags", s.handleTags)
	mux.HandleFunc("/api/translate", s.handleTranslate)                    // traducir segmentos (POST)
	mux.HandleFunc("/api/translate/languages", s.handleTranslateLanguages) // detección tipo Maps
	mux.HandleFunc("/api/maps/geocode", s.handleGeocode)                   // plugin Maps: buscar calle/lugar

	// ── adminMux: la ÚNICA puerta de administración ────────────────────────
	// Todas las rutas administrativas se registran aquí y se montan detrás de
	// `requireAdmin` (auth.go). El Panel esconde los botones; esto es la
	// cerradura. Ruta admin nueva → se registra en `adminMux`, y ya está
	// protegida sin tener que acordarse de nada.
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/api/admin/service", s.handleServiceControl)
	mapAdmin := newMapManager(mapsDir, geoPath)
	mux.HandleFunc("/api/maps/search", s.handleMapSearch(mapAdmin))
	adminMux.HandleFunc("/api/admin/maps", mapAdmin.handleList)
	adminMux.HandleFunc("/api/admin/maps/download", mapAdmin.handleDownload)
	adminMux.HandleFunc("/api/admin/maps/cancel", mapAdmin.handleCancel)
	adminMux.HandleFunc("/api/admin/maps/activate", mapAdmin.handleActivate)
	adminMux.HandleFunc("/api/admin/maps/delete", mapAdmin.handleDelete)
	adminMux.HandleFunc("/api/admin/maps/geocoder", mapAdmin.handleGeocoder)
	adminMux.HandleFunc("/api/admin/maps/index", mapAdmin.handleStreetIndex)
	adminMux.HandleFunc("/api/admin/maps/index/cancel", mapAdmin.handleStreetIndexCancel)
	mux.HandleFunc("/api/maps/config", mapAdmin.handlePublicConfig)
	mux.HandleFunc("/api/maps/tiles/", mapAdmin.handlePublicTile)
	mux.HandleFunc("/api/maps/nearby", mapAdmin.handleNearby)

	// Motor de descargas (catálogo ZIM y descargas manuales del admin). DB propia
	// para no estrangular library.db con SetMaxOpenConns(1). DOWNLOAD_ROOT es la
	// carpeta del pool concedida por Noumon (contrato DOWNLOADS-CONTRACT.md §5).
	downloadsDB := resolvePoolPath("DOWNLOADS_DB", poolRoot, "db/downloads.db", filepath.Join(filepath.Dir(dbPath), "downloads.db"))
	ddb, derr := sql.Open("sqlite", downloadsDB)
	if derr != nil {
		log.Fatalf("no se pudo abrir downloads.db (%s): %v", downloadsDB, derr)
	}
	dlParallel := 2
	if v, err := strconv.Atoi(env("DOWNLOAD_CONCURRENCY", "")); err == nil && v > 0 {
		dlParallel = v
	}
	downloadRoot := resolvePoolPath("DOWNLOAD_ROOT", poolRoot, "downloads", filepath.Join(filepath.Dir(dbPath), "downloads"))
	if absRoot, aerr := filepath.Abs(downloadRoot); aerr == nil {
		downloadRoot = absRoot
	}
	os.MkdirAll(downloadRoot, 0o755)

	// Carpeta de ZIMs del pool + gestión (admin_zim.go). Se crea aquí, antes del
	// motor de descargas, para engancharle el auto-registro: al terminar una
	// descarga del catálogo de kiwix se registra sola.
	zimDir := resolvePoolPath("ZIM_DIR", poolRoot, "zim", "")
	libraryXML := env("LIBRARY_XML", "")
	if libraryXML == "" && zimDir != "" {
		libraryXML = filepath.Join(zimDir, "library.xml")
	}
	az := &adminZim{libraryXML: libraryXML, zimDir: zimDir, store: store} // gestión nativa: sin kiwix-manage
	s.zimAdmin = az

	// Motor ZIM: NATIVO por defecto (retirada de kiwix, §8 — 2026-07-14). El
	// camino kiwix queda solo como rollback explícito (ZIM_ENGINE=kiwix) durante
	// una versión estable más; después se poda (kget/kiwixSem/proxy).
	if engine := env("ZIM_ENGINE", "native"); engine != "kiwix" {
		s.zimNative = newNativeZims(az)
		az.native = s.zimNative // back-ref: exponer el job de indexado FTS en el listado
		// Alta/baja de ZIM desde el Panel → invalidar el registro nativo (§23):
		// cierra los archives desregistrados y reabre los nuevos a la próxima
		// petición. Con kiwix este hook es nil (kiwix -M recarga solo).
		az.onLibraryChange = s.zimNative.invalidate
		log.Printf("motor ZIM: NATIVO (zim-engine, Go puro) · rollback: ZIM_ENGINE=kiwix")
	} else {
		log.Printf("motor ZIM: kiwix (proxy) — modo LEGADO, requiere kiwix-serve corriendo")
	}

	// Generador de sidecar: al llegar un job a `done`, escribe la ficha .json
	// junto al fichero para que Library lo muestre offline (ver sidecar.go).
	sw := &sidecarWriter{root: downloadRoot}

	// onEvent del motor: los ZIM del catálogo (owner_kind "kiwix") se auto-registran
	// al terminar; el resto (media) escribe sidecar como hasta ahora.
	onEvent := func(job download.Job) {
		if job.OwnerKind == "kiwix" {
			if job.Status == download.StatusDone {
				if err := az.registerDownloaded(job.DestPath); err != nil {
					log.Printf("auto-registro %s: %v", filepath.Base(job.DestPath), err)
				} else {
					log.Printf("auto-registrado: %s", filepath.Base(job.DestPath))
				}
			}
			return
		}
		sw.onJobEvent(job)
	}

	mgr, mErr := download.NewManager(ddb, dlParallel, onEvent)
	if mErr != nil {
		log.Fatalf("download manager: %v", mErr)
	}
	if err := mgr.ResumeIncomplete(); err != nil {
		log.Printf("download: ResumeIncomplete: %v", err)
	}
	// Reconciliar descargas oficiales terminadas antes de esta version. Asi una
	// actualizacion no obliga a descargar de nuevo colecciones como TED.
	if jobs, err := mgr.ListByOwner("kiwix"); err != nil {
		log.Printf("confianza ZIM: no se pudo reconciliar el catalogo: %v", err)
	} else {
		for _, job := range jobs {
			if job.Status == download.StatusDone {
				if err := az.reconcileDownloaded(job.DestPath); err != nil {
					log.Printf("confianza ZIM %s: %v", filepath.Base(job.DestPath), err)
				}
			}
		}
	}
	dl := &downloadDeps{mgr: mgr, root: downloadRoot}
	dl.registerDownloadRoutes(adminMux) // cola de importación: administrativa
	log.Printf("descargas: raíz %s · %d en paralelo (DOWNLOAD_ROOT/DOWNLOAD_CONCURRENCY para ajustar)", downloadRoot, dlParallel)

	// Mitad lectora: escanea las fichas del pool y sirve los ficheros locales
	// (ver media.go). Comparte la raíz con las descargas.
	md := &mediaDeps{root: downloadRoot}
	s.registerMediaRoutes(mux, md)                                                                           // /api/media + /media/* — detrás del gate de acceso
	mux.HandleFunc("/api/images", s.handleImageSearch(md))                                                   // imágenes: ZIM + portadas/logos de vídeos
	adminMux.HandleFunc("/api/admin/media/delete", s.handleMediaDelete(md))                                  // borrar item del pool (admin)
	adminMux.HandleFunc("/api/admin/upload", s.handleUpload(&uploadDeps{root: downloadRoot}, md))            // carga manual (Moments/Cabinet)
	adminMux.HandleFunc("/api/admin/media/update", s.handleMediaUpdate(&uploadDeps{root: downloadRoot}, md)) // editar ficha
	s.registerItemRoutes(mux, md)

	// Inventario del pool para el Panel de Control (POOL-CONTRACT.md §6). Read-only.
	// zim/models viven detrás de kiwix/translate; el shim solo los reporta para el
	// Panel (con POOL_ROOT o con ZIM_DIR/MODELS_DIR sueltos en dev).
	pool := &poolInfo{
		root:              poolRoot,
		provider:          env("POOL_PROVIDER", ""),
		configPath:        os.Getenv("NOUMON_LIBRARY_CONFIG"),
		externallyManaged: os.Getenv("NOUMON_LIBRARY_STORAGE_MANAGED") == "environment",
		sections: []sectionSpec{
			{key: "zim", engine: "kiwix", path: zimDir},
			{key: "models", engine: "translate", path: resolvePoolPath("MODELS_DIR", poolRoot, "models", "")},
			{key: "downloads", engine: "media", path: downloadRoot},
			{key: "maps", engine: "maps", path: mapsDir},
			{key: "db", engine: "shim", path: filepath.Dir(dbPath)},
		},
	}
	adminMux.HandleFunc("/api/storage", pool.handleStorage)

	// Publicación en la red local (Panel → Red). Guarda lanAccess en el mismo
	// config.json del pool; el supervisor lo convierte en BIND al reiniciar.
	network := &networkInfo{configPath: os.Getenv("NOUMON_LIBRARY_CONFIG"), bind: bind, port: port}
	adminMux.HandleFunc("/api/admin/network", network.handleNetwork)

	// Escucha amplia con biblioteca despublicada (servidor headless): lectura
	// cerrada a remotos, administración abierta. La decisión la toma el
	// supervisor (coreEnv) y llega como señal explícita, sin releer config.
	if os.Getenv("NOUMON_LAN_PRIVATE") == "1" {
		s.lanPrivate = true
		log.Printf("red: biblioteca NO publicada (solo administración remota); Panel → Red para publicarla")
	}

	// Rutas de gestión de ZIM (az se creó arriba, junto al motor de descargas,
	// para el auto-registro del catálogo).
	az.registerRoutes(adminMux)
	if s.zimNative != nil { // indexado full-text por ZIM (zim_fts_index.go), solo motor nativo
		adminMux.HandleFunc("/api/admin/zim/index", s.zimNative.handleIndex)
		adminMux.HandleFunc("/api/admin/zim/index/all", s.zimNative.handleIndexAll)
		adminMux.HandleFunc("/api/admin/zim/index/cancel", s.zimNative.handleIndexCancel)
	}
	s.registerAdminTranslateRoutes(adminMux)              // modelos de traducción (proxy a translate-wrap)
	s.registerAdminDepsRoutes(adminMux)                   // aprovisionamiento de herramientas (admin_deps.go)
	newAdminCatalog(mgr, zimDir).registerRoutes(adminMux) // catálogo remoto de kiwix → descarga al pool
	s.registerAdminUserRoutes(adminMux)                   // alta/baja de cuentas (auth.go)
	s.registerAccessRoutes(adminMux)                      // acceso por colección: nivel + edad (access.go)

	// Identidad: PÚBLICA. Login y register tienen que ser alcanzables sin sesión.
	s.registerAuthRoutes(mux)

	// Montaje de la cerradura. Los patrones de adminMux son absolutos, así que
	// no hay StripPrefix: solo se enruta el subárbol hacia el guard.
	guard := s.requireAdmin(adminMux)
	mux.Handle("/api/admin/", guard)     // zim, catalog, translate, users, collections/access, media/delete
	mux.Handle("/api/storage", guard)    // inventario y rutas fisicas del servidor
	mux.Handle("/api/downloads", guard)  // cola de importación (exacta)
	mux.Handle("/api/downloads/", guard) // …/clear, …/{id}/pause|resume|cancel

	if s.userCount() == 0 {
		log.Printf("SETUP PENDIENTE: no hay usuarios. El PRIMER registro en /api/auth/register será ADMIN. Hazlo antes de exponer Library.")
	}

	log.Printf("gestión ZIM: NATIVA (sin kiwix-manage) · library=%s", libraryXML)

	mux.HandleFunc("/content/", s.handleContent) // passthrough directo (streaming, §7)
	mux.HandleFunc("/catalog/v2/illustration/", s.handleIllustration)
	// Plugin Maps (offline): página + assets estáticos y los tiles PMTiles (con range).
	mapsFS := http.StripPrefix("/maps/", http.FileServer(http.Dir(siblingDir("maps-www"))))
	mux.HandleFunc("/maps/", func(w http.ResponseWriter, r *http.Request) {
		// El HTML no lleva nombre versionado: WebView2 debe revalidarlo tras cada
		// instalacion. Fuentes, sprites y vendor siguen aprovechando su cache.
		if r.URL.Path == "/maps/" || strings.HasSuffix(r.URL.Path, "/index.html") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		mapsFS.ServeHTTP(w, r)
	})
	mux.Handle("/mapdata/", mapDataHandler(mapsDir))
	// Panel de Control: segunda superficie (app aparte), servida como estáticos.
	// no-cache en el HTML para que los rebuilds se reflejen sin hard-refresh; los
	// assets van con hash en el nombre, así que su caché sí es segura.
	panelFS := http.StripPrefix("/panel/", http.FileServer(http.Dir(siblingDir("www-panel"))))
	mux.HandleFunc("/panel/", func(w http.ResponseWriter, r *http.Request) {
		setTopLevelIsolation(w)
		if !strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		panelFS.ServeHTTP(w, r)
	})
	// Lector (SPA): superficie principal, servida en / como estáticos (www-client).
	// Mismo origen que la API → sin CORS ni el parche cross-origin (cookie SameSite,
	// ?st= del PDF/media). Fallback SPA: cualquier ruta desconocida → index.html.
	// no-cache en el HTML para reflejar rebuilds sin hard-refresh; los assets van
	// con hash en el nombre → su caché es segura.
	clientDir := siblingDir("www-client")
	clientFS := http.FileServer(http.Dir(clientDir))
	clientIndex := filepath.Join(clientDir, "index.html")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setTopLevelIsolation(w)
		// ¿Existe el fichero pedido? http.Dir.Open bloquea el path traversal.
		if r.URL.Path != "/" {
			if f, err := http.Dir(clientDir).Open(r.URL.Path); err == nil {
				st, serr := f.Stat()
				f.Close()
				if serr == nil && !st.IsDir() {
					if !strings.Contains(r.URL.Path, "/assets/") {
						w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					}
					clientFS.ServeHTTP(w, r)
					return
				}
			}
		}
		// Raíz o ruta SPA desconocida → index.html.
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		http.ServeFile(w, r, clientIndex)
	})

	if translateURL != "" {
		log.Printf("traductor: motor en %s · máx. %d simultáneas (TRANSLATE_URL/TRANSLATE_CONCURRENCY)", translateURL, translateConc)
	} else {
		log.Printf("traductor: DESACTIVADO (define TRANSLATE_URL para activarlo)")
	}

	addr := bind + ":" + port
	log.Printf("Library Server → http://%s   (motor: %s)", addr, kiwixURL)
	if token == "" {
		log.Printf("auth: DESACTIVADA (dev LAN · postura por defecto §6)")
	} else {
		log.Printf("auth: token Noumon requerido en escrituras (POST)")
	}
	log.Fatal(http.ListenAndServe(addr, s.middleware(mux)))
}

// middleware: logging + auth por canal (§6).
// GET (lectura) pasa siempre en LAN; POST/PUT/DELETE (escritura) exigen token si hay uno configurado.
// adminPlaneAllowed decide qué puede tocar un remoto cuando la biblioteca está
// despublicada (lanPrivate): el Panel y sus rutas de identidad/administración
// siempre (para poder entrar y republicar), y todo lo demás solo con sesión de
// admin o token de máquina. requireAdmin sigue siendo la cerradura real de
// /api/admin/*: aquí solo se decide visibilidad del plano.
func (s *Server) adminPlaneAllowed(r *http.Request) bool {
	p := r.URL.Path
	if p == "/api/health" || p == "/panel" || strings.HasPrefix(p, "/panel/") ||
		strings.HasPrefix(p, "/api/auth/") || strings.HasPrefix(p, "/api/admin/") {
		return true
	}
	if s.hasMachineToken(r) {
		return true
	}
	u := s.currentUser(r)
	return u != nil && u.IsAdmin
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Defensa CSRF para cualquier mutación iniciada por navegador. Clientes de
		// sistema/CLI sin Origin siguen funcionando; los orígenes remotos permitidos
		// se declaran en CLIENT_ORIGINS. Excepción: una petición autenticada SOLO por
		// token explícito (Bearer o ?st=, sin cookie de sesión) es inmune a CSRF — el
		// token no es ambiente, una web hostil no lo conoce ni lo puede adjuntar. Es
		// lo que permite la subida DIRECTA al Core desde el webview de escritorio
		// (Wails/WebView2 no reenvía el body del multipart por su proxy → MOMENTS-UPLOAD.md).
		if requestIsMutation(r) && !requestOriginAllowed(r) && !requestTokenOnly(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origen de la petición no permitido"})
			return
		}

		// Biblioteca despublicada con escucha amplia (servidor headless): los
		// remotos solo alcanzan el plano de administración; la excepción del
		// admin autenticado le mantiene TODO el acceso (nunca se queda fuera).
		if s.lanPrivate && !requestIsLocal(r) && !s.adminPlaneAllowed(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "biblioteca no publicada en la red; pide acceso al administrador"})
			return
		}

		// Identidad estable y aislada para favoritos/notas/historial del invitado.
		// No concede permisos: currentUser sigue siendo nil hasta iniciar sesión.
		r = s.withGuestIdentity(w, r)

		// El cliente separado usa orígenes explícitos. Loopback queda habilitado
		// para desarrollo; producción se configura con CLIENT_ORIGINS.
		if o := r.Header.Get("Origin"); clientOriginAllowed(o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Noumon-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else if o := r.Header.Get("Origin"); o != "" && requestTokenOnly(r) {
			// Subida directa al Core por token (webview de escritorio): reflejar el
			// origen para que el JS lea la respuesta, SIN Allow-Credentials — la auth
			// va por token, no por cookie, así que no se exponen credenciales de más.
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Noumon-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			// Private Network Access: Chromium/WebView2 puede exigir esta cabecera
			// para dejar que un origen (wails.localhost) llegue a una dirección más
			// privada (127.0.0.1). loopback→loopback no debería dispararlo, pero
			// declararlo es inocuo y evita un bloqueo silencioso de la subida.
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Auth por canal (postura de base, NO el permiso de admin: eso es
		// requireAdmin). Si hay token configurado, una escritura debe venir del
		// carril máquina o de un humano con sesión.
		//
		// Dos excepciones necesarias:
		//   · /api/auth/* — login y register SON POST. Si los bloqueamos aquí,
		//     nadie puede entrar nunca (bug real de la versión anterior: poner
		//     NOUMON_TOKEN dejaba la instalación inaccesible).
		//   · sesión válida — el Panel y el lector nunca mandan X-Noumon-Token.
		write := r.Method != http.MethodGet && r.Method != http.MethodHead
		if s.token != "" && write && !strings.HasPrefix(r.URL.Path, "/api/auth/") {
			if !s.hasMachineToken(r) && s.currentUser(r) == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "necesitas iniciar sesión"})
				return
			}
		}

		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// setTopLevelIsolation evita que la SPA o el Panel autenticado se ejecuten
// dentro de un iframe controlado por otra web. No se aplica a /content ni /maps:
// esas superficies sí se incrustan de forma intencionada dentro del lector.
func setTopLevelIsolation(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
}

func validateMachineToken(token string) error {
	if token != "" && len(token) < 32 {
		return fmt.Errorf("NOUMON_TOKEN debe tener al menos 32 caracteres o quedar vacío")
	}
	return nil
}

func requestIsMutation(r *http.Request) bool {
	return r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
}

func requestOriginAllowed(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true // CLI, daemon y clientes antiguos sin cabecera de navegador
	}
	if clientOriginAllowed(origin) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host)
}

// requestTokenOnly indica que la petición se autentica SOLO por token explícito
// (Bearer o ?st=) y NO trae cookie de sesión. Un token así lo adjunta el JS a
// mano: una web hostil no lo conoce, así que la petición es inmune a CSRF. Se
// exige la AUSENCIA de cookie de sesión a propósito: si viniera la cookie
// (credencial ambiente), un ?st= basura no debe saltarse el check de Origin.
// La subida directa al Core es cross-origin, así que no lleva la cookie del
// Core y cae limpiamente en este carril.
func requestTokenOnly(r *http.Request) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	bearer := len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ")
	st := strings.TrimSpace(r.URL.Query().Get("st")) != ""
	if !bearer && !st {
		return false
	}
	if _, err := r.Cookie(sessionCookie); err == nil {
		return false // hay cookie de sesión → no es "solo token"
	}
	return true
}

// mapDataHandler publica exclusivamente los archivos de teselas que consume
// Maps. Evita el listado de la carpeta y que geo.db u otros ficheros internos
// puedan descargarse por conocer su nombre.
func mapDataHandler(mapsDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo lectura"})
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/mapdata/")
		if name == "" || strings.ContainsAny(name, `/\`) || filepath.Base(name) != name ||
			!strings.HasSuffix(strings.ToLower(name), ".pmtiles") {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(mapsDir, name)
		st, err := os.Stat(full)
		if err != nil || !st.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, full)
	})
}

func clientOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	// El atajo loopback (localhost/127.0.0.1 con cualquier puerto) es cómodo para
	// desarrollo, pero con Allow-Credentials:true abre a apps web locales hostiles.
	// H-5: en producción se exige lista explícita (CLIENT_ORIGINS); el comodín de
	// puerto solo se activa con DEV_CORS=1.
	if os.Getenv("DEV_CORS") == "1" {
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			return true
		}
	}
	for _, allowed := range strings.Split(os.Getenv("CLIENT_ORIGINS"), ",") {
		if strings.TrimRight(strings.TrimSpace(allowed), "/") == strings.TrimRight(origin, "/") {
			return true
		}
	}
	return false
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Motor NATIVO: la salud es "¿puedo leer library.xml y hay colecciones?" — NO
	// se pinga a kiwix, que en este modo no participa (§8). El proxy kiwix sigue
	// chequeándose cuando ZIM_ENGINE=kiwix.
	if s.zimNative != nil {
		count, err := s.zimNative.registeredCount()
		engine := "up"
		if err != nil || count == 0 {
			engine = "down"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"shim":        "up",
			"engine":      engine,
			"mode":        "native",
			"collections": count,
		})
		return
	}

	engine := "down"
	if resp, err := s.http.Get(s.kiwix.String() + "/catalog/v2/root.xml"); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			engine = "up"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shim":   "up",
		"engine": engine,
		"mode":   "kiwix",
		"kiwix":  s.kiwix.String(),
	})
}

// handleContent: /content/* — camino nativo (zim_native.go) o passthrough a
// kiwix-serve según el toggle ZIM_ENGINE. El gate de acceso y el aislamiento
// §19 aplican a AMBOS caminos.
func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	// Gate de acceso (usuarios/edad): /content/{zim}/… se sirve solo si el usuario
	// puede ver esa colección. Las ilustraciones (/catalog/v2/illustration/…) no
	// pasan por aquí como /content, así que no se bloquean (iconos inofensivos).
	zimID := contentZim(r.URL.Path)
	if zimID != "" && !s.canSeeZim(s.currentUser(r), zimID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sin acceso a esta colección"})
		return
	}

	isContent := strings.HasPrefix(r.URL.Path, "/content/")
	if isContent {
		// Aislamiento §19: el contenido ZIM es NO confiable aunque venga del
		// catálogo. Cinturón extra: sin service workers desde /content/*.
		if r.Header.Get("Service-Worker") == "script" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "service workers no permitidos en contenido ZIM"})
			return
		}
		interactive := false
		dest := r.Header.Get("Sec-Fetch-Dest")
		mode := r.Header.Get("Sec-Fetch-Mode")
		// Un navegador marca la navegación de la página con Sec-Fetch-Dest
		// document/iframe (o Sec-Fetch-Mode navigate) y así solo el documento —no
		// sus sub-recursos— consulta la confianza. Pero el canal library:// de la
		// app de escritorio (WebView2 sobre esquema propio) NO envía cabeceras
		// Sec-Fetch: en ese caso (ambas vacías) no se puede distinguir documento de
		// sub-recurso, así que se concede el modo interactivo. Es seguro: el gate
		// real es interactiveAllowed (solo ZIM de confianza) y el CSP de un
		// sub-recurso .js/.css no gobierna la ejecución. Sin esto, TED y demás ZIM
		// con JS quedaban en blanco SOLO en la app de escritorio (en navegador no).
		navLike := dest == "document" || dest == "iframe" || mode == "navigate" || (dest == "" && mode == "")
		if zimID != "" && navLike {
			interactive = s.zimAdmin != nil && s.zimAdmin.interactiveAllowed(zimID)
		}
		setContentIsolation(w, interactive)
	}

	if s.zimNative != nil && isContent {
		s.serveZimNative(w, r)
		return
	}
	s.proxy.ServeHTTP(w, r)
}

// handleIllustration aplica el mismo gate e aislamiento que /content. En modo
// nativo sirve el icono directamente desde M/Illustration_48x48@1, evitando la
// antigua caída al proxy Kiwix que no participa en ese modo.
func (s *Server) handleIllustration(w http.ResponseWriter, r *http.Request) {
	rawID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/catalog/v2/illustration/"), "/")
	id, err := url.PathUnescape(rawID)
	if err != nil || id == "" || strings.ContainsAny(id, `/\`) {
		http.NotFound(w, r)
		return
	}
	if !s.canSeeZim(s.currentUser(r), id) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sin acceso a esta colección"})
		return
	}
	if s.zimNative != nil {
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Path = "/content/" + id + "/M/Illustration_48x48@1"
		u.RawPath = ""
		clone.URL = &u
		s.handleContent(w, clone)
		return
	}
	setContentIsolation(w, false)
	s.proxy.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
