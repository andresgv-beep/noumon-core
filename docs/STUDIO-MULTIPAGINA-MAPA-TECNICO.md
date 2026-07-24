# Multipágina en Studio — mapa técnico previo (auditoría)

Documento de **Fable (auditoría)**, 2026-07-24. No es la especificación: es el
mapa del terreno real del código para que la spec (que mantiene GPT en
`NOUMON-STUDIO-ESPECIFICACION.md`) se escriba sobre hechos y no sobre memoria.
Objetivo del feature: documento multipágina con menú de contenidos lateral,
ficha lateral (imagen + filas dato/valor) y enlaces desde texto seleccionado a
otras páginas. Referencia visual: artículo de enciclopedia — **prohibido
nombrar la marca** en spec/código/UI/commits (ver política de marcas del
proyecto): se dice "menú de contenidos", "ficha lateral", "documento
multipágina".

## 1. Hechos del código (verificados hoy)

### Modelo y validación (servidor)
- El contenido es un único JSON (`content`) dentro de `StudioDocument`;
  el snapshot de cada revisión guarda el documento entero en
  `studio_revisions.snapshot_json` (`studio_store.go`). **Consecuencia:
  multipágina es evolución de JSON + validador; no hay migración SQL del
  contenido.**
- `studioSchemaVersion = 1` con comprobación de igualdad estricta
  (`studio_model.go:205`). `validateStudioInput` se llama desde **8 sitios**:
  crear (`studio.go:174`), actualizar (`studio.go:319`), publicar
  (`studio_publication.go:34`), publicación de media
  (`studio_media_publication.go:73`), assets (`studio_assets.go:771`),
  **backfill de arranque** (`studio_store.go:272`), **restore de revisión**
  (`studio_store.go:568`) y tests. **Consecuencia: los snapshots v1 viven para
  siempre en `studio_revisions` y el backfill/restore los re-valida → el
  validador debe aceptar v1 Y v2 indefinidamente** (normalizando v1→v2 en
  lectura), no basta con subir la constante.
- Límites actuales: 1000 bloques/doc, 2 MB de content, profundidad 4,
  1M runas de texto, IDs de bloque únicos por documento (`state.ids` global
  al documento). `studioIDRE` admite `[A-Za-z0-9._:-]` — **sin `#` ni `/`**.
- Extracción en validación: `PlainText` (un solo blob por documento),
  `Links` (solo de bloques `itemRef`), `Assets` (imágenes + media). Publish
  ancla assets con `ensureStudioAssets(tx, id, valid.Assets, true)`.

### Publicación, FTS y enlaces (servidor)
- Publicar = una transacción: UPDATE de estado + `replaceStudioPublishedLinks`
  + `replaceStudioPublishedFTS` + `content_origins` + gate de colección
  (nace 'login') (`studio_publication.go:16-95`). La lectura pública sale del
  snapshot (`publishedStudioSnapshot`), nunca del borrador, y `ownerUserId`
  jamás sale por la API pública.
- FTS: `studio_published_fts` es FTS5 con **una fila por documento**
  (document_id UNINDEXED, title, summary, plain_text, tags, work_type,
  topics, author_label; `store.go:188`). Existe un **rebuild total al
  arranque** (`studio_store.go:261` borra todo y re-inserta desde snapshots)
  → cambiar el esquema FTS tiene camino de reindexado ya hecho.
- `searchPublishedStudioDocuments` devuelve `ItemID: "studio:"+id` sin noción
  de fragmento/página (`studio_publication.go:466+`).
- `studio_published_links(source_document_id, target_item_id)` solo se escribe
  en publish/unpublish/backfill; los borradores jamás. Relations/related
  puntúa a nivel documento.

### Cliente
- Pestañas: `setItem(tb, itemId, open, ...)` (`App.svelte:319`); direcciones
  `library://item/<itemId>`; `parseLibraryAddress` no conoce fragmentos.
  **Página → nuevo campo en la pestaña + extensión de la dirección +
  `pushHistory` para que atrás/adelante funcione entre páginas.**
- Editor (`Studio.svelte`): `selected.content.blocks` se usa directamente en
  muchos puntos (render del lienzo, `findBlockLocationIn` raíz, `addBlock`,
  `imageSelected`, drag&drop, `collectHeadings`). **Riesgo de dispersión: hay
  que introducir UN accesor (bloques de la página activa) y migrar todos los
  usos; en la revisión buscaré `content.blocks` residuales.**
- Autoguardado serializado (changeVersion + savePromise) con
  `mergeSavedEnvelope` (solo funde el sobre; el cuerpo en edición no se
  reemplaza) — multipágina no lo toca si el content sigue siendo un JSON.
- **Recovery IndexedDB guarda el content con la forma vieja**: tras migrar a
  v2, una copia de recuperación v1 debe normalizarse al aplicarse (misma
  normalización v1→v2 que el servidor, implementada una vez y compartida
  conceptualmente: server en Go, cliente en JS, con tests espejo).
- Render publicado: `DocumentPage` → `StudioDocumentView` (render del
  snapshot) + `onToc` → índice de encabezados en la barra lateral DERECHA del
  lector (`Reader.svelte .toc-col`, recién montado). **El menú de contenidos
  (páginas) es OTRA cosa y va aparte (izquierda); el Índice sigue siendo los
  encabezados de la página abierta.**
- Texto enriquecido: pseudo-markdown `**`/`*` con **escape ANTES de
  formatear** (`inline()` en StudioBlockView/CanvasBlock). Esta es la defensa
  anti-XSS. Cualquier enlace en texto debe ser extensión de esa sintaxis,
  **jamás HTML almacenado**.

## 2. Decisiones que la spec debe fijar (con mi recomendación)

1. **Forma v2**: `content = { schemaVersion:2, presentation, classification,
   pages:[{ id, title, blocks:[...] }], ficha? }`. Normalización v1→v2:
   envolver `blocks` como única página (id derivado estable, p.ej. `p1`).
   Reversible: v2 con una sola página y sin ficha degrada a v1 sin pérdida.
2. **IDs de página**: mismo régimen `studioIDRE`; unicidad de IDs de bloque
   **global al documento** (se mantiene `state.ids` compartido) para que las
   anclas de encabezado y los enlaces sobrevivan a mover bloques de página.
3. **Límites**: los actuales (1000 bloques, 2 MB, 1M runas) son del
   **documento entero**, no por página; añadir tope de páginas (p.ej. 100).
4. **Enlaces internos en texto**: sintaxis dentro del pseudo-markdown, p.ej.
   `[[page:<pageId>|texto]]` y `[[item:<itemId>|texto]]`. El validador:
   - colecciona primero los IDs de página, después exige que cada `page:`
     resuelva (o lo marca roto — decidir: rechazar al publicar vs publicar
     con marca de roto; recomiendo **rechazar al publicar, avisar en
     borrador**);
   - los `item:` alimentan `state.links` → mismo pipeline
     `studio_published_links` de siempre (backlinks/related gratis).
   - El render escapa primero y luego sustituye la sintaxis por `<a>` con
     handler interno (nunca href externo).
5. **FTS por página**: pasar a **una fila por página** añadiendo
   `page_id UNINDEXED` y `page_title` al esquema; `DELETE WHERE document_id=?`
   sigue valiendo; el rebuild de arranque reindexa lo viejo. El resultado
   federado lleva la página para el deep-link. (Cambio de esquema FTS =
   recrear tabla en migración + rebuild, camino ya existente.)
6. **Ficha lateral**: estructura a nivel de documento (no un bloque), con
   campos whitelisted: imagen (assetId por el pipeline de assets existente),
   pie, y filas `{ etiqueta, valor, ¿enlace page:/item:? }` con límites de
   longitud y número (p.ej. 40 filas). El validador la recorre para
   PlainText/Assets/Links igual que los bloques.
7. **Routing**: pestaña con `pageId` opcional; dirección
   `library://item/<itemId>/<pageId>` (o query) — decidir formato exacto en
   la spec; historial y favoritos siguen anclando al documento.
8. **Menú de contenidos**: en el publicado, sidebar izquierdo plegable con
   páginas/subpáginas; en móvil colapsa. El Índice (derecha) no cambia de
   sitio ni de botón.

## 3. Invariantes que NO se pueden romper (mi checklist en cada fase)

1. Escapar antes de formatear; nada de HTML libre almacenado o renderizado.
2. Snapshot inmutable y **atómico**: todas las páginas de una revisión
   publican juntas en una transacción.
3. FTS y destacados/relacionados leen SOLO snapshots publicados; restore no
   contamina (ya cazamos esa fuga una vez — test en vivo otra vez).
4. Borradores jamás escriben `studio_published_links`.
5. `ownerUserId` nunca sale por la API pública.
6. El validador acepta v1 para siempre (backfill de arranque y restore lo
   exigen); documentos v1 abren, guardan, restauran y republican.
7. Los límites globales no se multiplican por página.
8. Gate de acceso por colección intacto (nace 'login').
9. Autoguardado/recovery: ninguna respuesta vieja pisa teclas nuevas; una
   copia de recuperación v1 se aplica normalizada, no corrompe un doc v2.
10. Nada de marcas de terceros en spec/código/UI/commits.

## 4. Fases con criterio de cierre (propuesta a contrastar con la spec)

- **F1 — Modelo v2 + normalización v1→v2** (server+cliente, sin UI nueva):
  validador con páginas, accesor único de bloques en el editor.
  Cierre: `go test ./...` verde; doc v1 real abre/edita/guarda/restaura/
  republica; recovery v1 aplicada sobre v2 correcta.
- **F2 — Gestor de páginas en el editor**: crear/renombrar/reordenar/borrar
  (borrar con recolocación o confirmación), autosave intacto.
  Cierre: 409 optimista y recovery probados cambiando de página en medio.
- **F3 — Menú de contenidos + routing**: editor y publicado; dirección con
  página; atrás/adelante; deep-link desde URL.
- **F4 — Ficha lateral**: campos + imagen por assets; render publicada y
  preview; móvil (debajo del contenido).
- **F5 — Enlaces en texto**: sintaxis, validación de destinos, UI de
  selección + "Enlazar página", enlaces rotos visibles en borrador.
- **F6 — FTS por página + búsqueda federada con deep-link + E2E**
  (temas claro/oscuro/retro, móvil, teclado, instaladores).

Cada fase deja Studio utilizable. Cada fase pasa por el circuito de siempre:
GPT programa → Fable audita (tests Go + verificación en vivo) → commit →
Ctrl+R (o core.exe si toca Go).
