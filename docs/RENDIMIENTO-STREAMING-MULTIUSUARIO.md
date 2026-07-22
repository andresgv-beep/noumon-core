# Rendimiento del streaming en multiusuario (vídeo / PDF)

> Estado (2026-07-22): arreglos aplicados y cubiertos por pruebas automáticas;
> pendiente validar la mejora con carga real de 3–4 clientes. El código confirma
> el coste repetido original, pero el peso exacto de SQLite frente a red, disco y
> cliente debe decidirse con las métricas descritas en el §8.

---

## 0. Implementación actual

- Sesión cacheada durante 8 s; logout, logout global, cambio/reset de contraseña,
  rotación y borrado de usuario invalidan RAM inmediatamente.
- Mapa de acceso cacheado durante 15 s, con invalidación explícita y reconstrucción
  única anti-estampida al vencer el TTL.
- Catálogo RAM inmutable, versionado e indexado por proveedor, colección e ID.
  Una mutación durante un escaneo hace que el snapshot viejo se descarte.
- Moments y Cabinet usan una petición a `/api/items/surface`; sus metadatos de
  colección salen del mismo snapshot, sin consultar Kiwix ni releerlos por visita.
- `GET /api/admin/cache/metrics` expone contadores agregados de hits, misses,
  builds, esperas y tiempo de construcción; nunca tokens, usuarios o rutas.
- `SetMaxOpenConns(1)` se mantiene. Tampoco se añade aún una caché de bytes de
  1 GB: primero se mide esta capa junto a la caché de ficheros del sistema operativo.

---

## 1. El síntoma

Con varios usuarios reproduciendo vídeo (o paginando un PDF grande) **a la vez**,
la reproducción va a tirones / se ralentiza. Con un solo usuario no se nota.

La pista "solo en multiusuario" apunta a un recurso compartido y serializado.
El código mostró SQLite como candidato claro; la validación con métricas debe
separar su impacto del disco, la red y la capacidad de los clientes.

---

## 2. Por qué el vídeo es el caso peor

Un `<video>` (y el visor de PDF por páginas) **no descarga el fichero de una vez**.
El navegador pide el contenido en trozos con cabeceras `Range` (`bytes=0-...`,
luego `bytes=1048576-...`, etc.), y **cada seek del usuario dispara más Ranges**.

Resultado: una reproducción puede generar muchas peticiones HTTP al endpoint
`/media/`, según navegador, formato, duración y seeks. Multiplicado por usuarios,
es un flujo constante de peticiones. Todo lo que cueste "algo" *por petición* se multiplica
por ese factor y se convierte en el cuello de botella. Una imagen o una descarga
normal no tienen este perfil (una petición y ya); el vídeo/PDF sí.

---

## 3. Qué corre en cada petición de Range (el camino caliente)

`GET /media/<ruta>` pasa por `gateMediaFile` (`media.go`) → `handleMediaFile`.
Por **cada** Range se ejecuta:

| Paso | Operación | Coste | Fichero |
|------|-----------|-------|---------|
| 1 | `s.currentUser(r)` → caché token→usuario | RAM normalmente; SQLite en miss | auth.go |
| 2 | `canSeeMediaPath` → mapa de acceso cacheado | RAM normalmente; una query global en miss | access.go |
| 3 | `UPDATE sessions SET last_seen` — **solo** si pasó `sessionTouchStep` (5 min) | escritura, ya throttleada | auth.go:328 |
| 4 | `os.Open` + `Stat` + `http.ServeContent` (con Range) | E/S de disco, barata | media.go:385 |

Los pasos 3 y 4 **no** son el problema:
- El paso 3 ya está mitigado: el `UPDATE` de `last_seen` solo ocurre cada 5 min
  por sesión (`sessionTouchStep`), no en cada Range. Quien escribió el fix ya lo
  vio venir. Bien.
- El paso 4 es lo que el endpoint debe hacer; `ServeContent` gestiona Range e
  `If-Modified-Since` correctamente.

Antes del arreglo, los pasos 1 y 2 eran dos lecturas SQLite por Range para
revalidar resultados que normalmente no cambian durante una reproducción. Ahora
el gate sigue ejecutándose, pero usa los insumos cacheados en el caso normal.

---

## 4. El multiplicador: `SetMaxOpenConns(1)`

`store.go:121` abre la BD con **una sola conexión**:

```go
db.SetMaxOpenConns(1) // SQLite: una conexión evita "database is locked"
// WAL: escrituras que no bloquean lecturas; busy_timeout por si acaso.
PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL;
```

Esto es razonable como decisión general (WAL + una conexión evita los
"database is locked" en la Pi con SD). **Pero** podía convertir el coste del §3
en un cuello de botella:

- WAL permite que varias lecturas no se bloqueen *a nivel de fichero*.
- Con `MaxOpenConns(1)`, el driver de Go **serializa igualmente** todas las
  consultas sobre esa única conexión: van de una en una.

Así, las lecturas de N usuarios podían hacer cola sobre una sola conexión para
preguntar repetidamente "¿este usuario puede ver esto?". Las métricas permiten
comprobar si esa cola era dominante o coexistía con límites de disco/red.

---

## 5. Causa raíz (resumida)

> Se revalida la sesión y el permiso de colección **contra SQLite en cada
> petición de Range**, y todas esas consultas se serializan sobre una única
> conexión. Como el vídeo/PDF generan cientos de Ranges por reproducción, en
> multiusuario la cola de consultas de permisos —no el disco ni el ancho de
> banda— es lo que estrangula el streaming.

Nada de esto es un fallo de seguridad: el gate hace lo correcto. Es un problema
de **coste por petición** en el camino más caliente del servidor.

---

## 6. Precedente en el propio código (esto ya se resolvió una vez)

Junto a `collectionAccess` (access.go) hay este comentario:

> `accessMap` carga TODA la config de acceso de una vez (una query)… los filtros
> de listado/búsqueda llamaban `collectionAccess` por cada item → N+1 sobre
> SQLite, que en la Pi con listados de cientos de items se nota (auditoría O-1).
> Con el mapa en mano, `canSeeCached` resuelve en memoria.

Ese precedente se extendió al camino de `/media`: `collectionAccess` resuelve
ahora contra `accessMap`, que conserva un snapshot en memoria e impide que varios
lectores reconstruyan el mismo mapa simultáneamente.

---

## 7. Diseño aplicado y decisión sobre el Fix C

Principio rector: **no relajar el gate**. Se sigue comprobando sesión y permiso
en cada petición; solo se cambia *contra qué* se comprueban — una caché de vida
corta en memoria en vez del disco. La seguridad es idéntica; el TTL corto
garantiza que un cambio de permisos o una sesión caducada se recojan enseguida.

### Fix A — cachear la resolución de sesión (token → *User) con TTL corto
El mayor ahorro por sí solo.
- Mapa en memoria `token → (*User, expira)` con TTL ~5–10 s, protegido por
  `sync.RWMutex` (o `sync.Map`).
- `currentUser` mira la caché antes de tocar la BD; si hay entrada viva, la usa.
- Durante una reproducción, los cientos de Ranges de ese usuario reusan la misma
  entrada → **0 lecturas SQLite** para la sesión en el caso normal.
- Las revocaciones internas invalidan la caché inmediatamente; el TTL no sustituye
  esa garantía.
- El `UPDATE last_seen` (throttleado a 5 min) puede quedarse como está o moverse
  a que lo dispare la caché al refrescar.

### Fix B — usar `accessMap`/`canSeeCached` en el gate de streaming
Reutiliza lo que YA existe y está probado (§6).
- `canSeeMediaPath` / `canDownloadMediaPath` resuelven contra el mapa de acceso
  cacheado; `collectionAccess` ya no consulta una fila SQLite por Range.
- La config se invalida en el PUT del panel y al sembrar acceso durante uploads.
- Una sola goroutine reconstruye el mapa al vencer; las demás reutilizan el resultado.
- Resultado: **0 lecturas SQLite** para el permiso de colección en el caso normal.

Con A+B, el camino caliente de un Range pasa de **2 lecturas SQLite** a **0** en
el caso normal (todo en memoria). El gate sigue ejecutándose en cada Range.

### Fix C (opcional, mayor calado) — permitir >1 conexión de lectura
Si tras A+B aún se quiere más margen concurrente:
- Con WAL, las lecturas concurrentes son seguras. Se podría subir
  `SetMaxOpenConns` (p. ej. a 4) y dejar las **escrituras** serializadas por otra
  vía (una conexión de escritura dedicada, o un mutex de escritura en Go).
- Riesgo: reintroduce la posibilidad de "database is locked" si no se separa
  lectura/escritura con cuidado. `busy_timeout=5000` ya da colchón.
- **No hacer C sin A+B**: A+B quitan la mayor parte de las consultas, así que
  puede que C ni haga falta. Medir antes.

### Estado de ejecución
1. **Fix A** aplicado, incluida revocación administrativa inmediata.
2. **Fix B** aplicado con single-builder para evitar estampidas periódicas.
3. Catálogo RAM versionado e indexado aplicado para Moments/Cabinet.
4. **Fix C** aplazado hasta medir con 3–4 reproducciones simultáneas.

---

## 8. Cómo verificar que está arreglado

- Reproducir 3–4 vídeos a la vez desde clientes/sesiones distintas y comprobar
  que no hay tirones ni buffering cruzado (que el seek de uno no frene a otro).
- Consultar `GET /api/admin/cache/metrics` antes y después. Confirmar que los hits
  crecen, que `access.builds` aumenta una sola vez por expiración y que los misses
  quedan casi planos después de calentar las cachés.
- Caso PDF: abrir un PDF grande y paginar rápido mientras otro usuario reproduce
  vídeo; ambos deben mantenerse fluidos.

---

## 9. Notas de seguridad (para no romper el gate al optimizar)

- El TTL de sesión es corto (8 s) como red de seguridad para cambios externos.
  Las revocaciones realizadas por la aplicación invalidan RAM inmediatamente.
- La caché de permisos de colección **debe invalidarse** cuando el panel cambia
  el acceso de una colección (ya hay mecanismo: `accessMap` se invalida en el PUT).
- No cachear la decisión final "puede ver X fichero" por ruta completa sin TTL:
  cachear los **insumos** (sesión, mapa de acceso) y recomputar la decisión barata
  en memoria. Así un cambio de permiso o de edad del usuario se refleja al
  siguiente ciclo de caché sin ventanas largas.
- `?dl=1` y las cabeceras de aislamiento (`Referrer-Policy`, CSP sandbox,
  `X-Frame-Options`, `nosniff`) del gate **se mantienen intactas**: no son el
  problema de rendimiento y sí son necesarias.

---

## 10. Resumen en una línea

El camino normal de un Range conserva el gate, pero resuelve sesión y permiso en
RAM; las reconstrucciones son únicas, las revocaciones internas son inmediatas y
el catálogo de Moments/Cabinet es un snapshot versionado compartido. Falta medir
en carga real antes de atribuir toda la mejora o cualquier resto a SQLite.
