# Rendimiento del streaming en multiusuario (vídeo / PDF)

> Estado: diagnóstico + plan. El problema está identificado y confirmado en
> código; el arreglo aún NO está aplicado. Este documento existe para poder
> mantener el multiusuario sin volver a reconstruir el análisis desde cero.

---

## 1. El síntoma

Con varios usuarios reproduciendo vídeo (o paginando un PDF grande) **a la vez**,
la reproducción va a tirones / se ralentiza. Con un solo usuario no se nota.

La pista "solo en multiusuario" es la clave del diagnóstico: apunta a un recurso
**compartido y serializado** que se satura cuando varias reproducciones tiran de
él en paralelo. Ese recurso es SQLite.

---

## 2. Por qué el vídeo es el caso peor

Un `<video>` (y el visor de PDF por páginas) **no descarga el fichero de una vez**.
El navegador pide el contenido en trozos con cabeceras `Range` (`bytes=0-...`,
luego `bytes=1048576-...`, etc.), y **cada seek del usuario dispara más Ranges**.

Resultado: **una sola reproducción genera cientos o miles de peticiones HTTP**
al endpoint `/media/`. Multiplicado por usuarios simultáneos, es un chorro
constante de peticiones. Todo lo que cueste "algo" *por petición* se multiplica
por ese factor y se convierte en el cuello de botella. Una imagen o una descarga
normal no tienen este perfil (una petición y ya); el vídeo/PDF sí.

---

## 3. Qué corre en cada petición de Range (el camino caliente)

`GET /media/<ruta>` pasa por `gateMediaFile` (`media.go`) → `handleMediaFile`.
Por **cada** Range se ejecuta:

| Paso | Operación | Coste | Fichero |
|------|-----------|-------|---------|
| 1 | `s.currentUser(r)` → `SELECT` JOIN sessions+users por token | **1 lectura SQLite** | auth.go:289 |
| 2 | `canSeeMediaPath` → `collectionAccess` → `SELECT` en `collection_access` | **1 lectura SQLite** | access.go:53 / :192 |
| 3 | `UPDATE sessions SET last_seen` — **solo** si pasó `sessionTouchStep` (5 min) | escritura, ya throttleada | auth.go:328 |
| 4 | `os.Open` + `Stat` + `http.ServeContent` (con Range) | E/S de disco, barata | media.go:385 |

Los pasos 3 y 4 **no** son el problema:
- El paso 3 ya está mitigado: el `UPDATE` de `last_seen` solo ocurre cada 5 min
  por sesión (`sessionTouchStep`), no en cada Range. Quien escribió el fix ya lo
  vio venir. Bien.
- El paso 4 es lo que el endpoint debe hacer; `ServeContent` gestiona Range e
  `If-Modified-Since` correctamente.

El problema son los **pasos 1 y 2: dos lecturas a SQLite en cada Range**, para
revalidar permisos que **no cambian durante una reproducción**. Es trabajo
repetido cientos de veces por el mismo resultado.

---

## 4. El multiplicador: `SetMaxOpenConns(1)`

`store.go:121` abre la BD con **una sola conexión**:

```go
db.SetMaxOpenConns(1) // SQLite: una conexión evita "database is locked"
// WAL: escrituras que no bloquean lecturas; busy_timeout por si acaso.
PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL;
```

Esto es razonable como decisión general (WAL + una conexión evita los
"database is locked" en la Pi con SD). **Pero** convierte el problema del §3 en el
cuello de botella real:

- WAL permite que varias lecturas no se bloqueen *a nivel de fichero*.
- Con `MaxOpenConns(1)`, el driver de Go **serializa igualmente** todas las
  consultas sobre esa única conexión: van de una en una.

Así que las 2 lecturas × N usuarios × cientos de Ranges **hacen cola sobre una
sola conexión**. Ahí es donde el vídeo del segundo y tercer usuario empieza a ir
a tirones: no está esperando al disco del vídeo, está esperando su turno para
preguntar "¿este usuario puede ver esto?" — una pregunta cuya respuesta no ha
cambiado desde el Range anterior.

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

Es decir: **el mismo patrón N+1 ya se detectó y se arregló para los listados**
(cachear la config de acceso en memoria con `accessMap` + `canSeeCached`). El
streaming se quedó fuera de ese arreglo y sigue yendo a la BD por Range. La
solución de §7 es, en parte, extender esa misma idea al camino de `/media`.

---

## 7. Plan de arreglo (por orden de impacto / menor riesgo)

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
- Al hacer logout / borrar sesión, invalidar la entrada (o dejar que el TTL corto
  la expire; 5–10 s de ventana es aceptable y coherente con el modelo actual).
- El `UPDATE last_seen` (throttleado a 5 min) puede quedarse como está o moverse
  a que lo dispare la caché al refrescar.

### Fix B — usar `accessMap`/`canSeeCached` en el gate de streaming
Reutiliza lo que YA existe y está probado (§6).
- `canSeeMediaPath` / `canDownloadMediaPath` deben resolver contra el mapa de
  acceso cacheado, no contra `collectionAccess` (que va a la BD).
- La config de acceso cambia poquísimo; el mapa se cachea con invalidación en el
  PUT del panel (mismo mecanismo que ya usa `accessMap` para los listados).
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

### Orden recomendado
1. **Fix A** (sesión cacheada) — el que más quita, riesgo bajo.
2. **Fix B** (permiso cacheado) — reutiliza código existente, riesgo bajo.
3. Medir con 3–4 reproducciones simultáneas. Si va fino, parar aquí.
4. **Fix C** solo si hace falta y con separación lectura/escritura bien hecha.

---

## 8. Cómo verificar que está arreglado

- Reproducir 3–4 vídeos a la vez desde clientes/sesiones distintas y comprobar
  que no hay tirones ni buffering cruzado (que el seek de uno no frene a otro).
- Contar consultas: instrumentar (temporalmente) un contador en `currentUser` y
  `collectionAccess` y confirmar que durante una reproducción quedan casi planos
  tras el primer Range (deberían dispararse una vez y luego servir de caché).
- Caso PDF: abrir un PDF grande y paginar rápido mientras otro usuario reproduce
  vídeo; ambos deben mantenerse fluidos.

---

## 9. Notas de seguridad (para no romper el gate al optimizar)

- El TTL de la caché de sesión debe ser **corto** (5–10 s): es la ventana máxima
  en la que una sesión ya caducada/borrada podría seguir sirviendo. Aceptable
  para /media; no ampliar sin pensarlo.
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

El streaming va a tirones en multiusuario porque revalida sesión + permiso contra
SQLite en cada petición de Range y todas esas consultas se serializan sobre una
única conexión; la solución es cachear en memoria la sesión (TTL corto) y el mapa
de acceso (ya existe para listados), dejando el gate igual de estricto pero sin
tocar el disco en el caso normal.
