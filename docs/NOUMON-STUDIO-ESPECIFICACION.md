# Noumon Studio — especificación técnica previa

**Estado:** propuesta para revisión, sin implementación  
**Fecha:** 2026-07-21 · actualizada 2026-07-23 con decisiones de interfaz
**Proyecto analizado:** `noumon-core`  
**Objetivo:** incorporar a Noumon un estudio de creación, previsualización y publicación de conocimiento sin convertir el cliente en administrador del servidor.

## 1. Resumen ejecutivo

Noumon Studio será una superficie de autoría dentro del cliente Noumon. Se abrirá en una pestaña propia y permitirá crear documentos enriquecidos, preparar fichas para Cabinet y Moments, guardar borradores privados, previsualizar el resultado definitivo y publicar contenido indexable en Library Server.

La frontera arquitectónica existente se conserva:

```text
Noumon presenta el editor y la previsualización.
Library Server guarda, valida, autoriza, publica e indexa.
El Panel administra usuarios, permisos globales y colecciones.
```

Studio no accederá al sistema de archivos del servidor, no escribirá directamente en el pool y no llamará a endpoints administrativos. El servidor seguirá siendo el único propietario del contenido.

El encaje es viable y aprovecha buena parte del código existente:

- pestañas y vistas internas del cliente;
- autenticación y estado por usuario;
- contratos `Collection`, `Item`, `Preview` y `OpenTarget`;
- plantillas y sidecars de Cabinet y Moments;
- streaming `/media`, Range y control de acceso;
- búsqueda federada;
- SQLite en WAL, adecuado para equipos modestos;
- subida directa al Core desde WebView2, necesaria para multipart.

Sin embargo, Studio necesita un dominio nuevo de autoría. No debe construirse reutilizando sin más `/api/admin/upload`, porque esa API exige administración, publica directamente en el pool y no representa borradores, propietarios, revisiones ni conflictos de edición.

## 2. Principios de producto

1. **CPU y RAM son suficientes.** Studio no depende de IA, GPU ni conexión a Internet.
2. **Servidor como fuente de verdad.** Los borradores y publicaciones pertenecen al servidor y a la cuenta, no a un navegador concreto.
3. **Privado por defecto.** Crear o guardar nunca publica accidentalmente.
4. **Previsualización fiel.** La vista previa usa los mismos componentes de presentación que el contenido publicado.
5. **Permisos explícitos.** Leer, crear y publicar son capacidades diferentes.
6. **Formato portable.** El contenido canónico es estructurado y no depende del motor visual elegido para editarlo.
7. **Seguridad por diseño.** No se almacena ni ejecuta HTML arbitrario aportado por el usuario.
8. **Degradación limpia.** Si Studio no está disponible en un servidor antiguo, el cliente continúa funcionando como lector.
9. **Recursos modestos.** Nada de procesos residentes adicionales ni indexadores pesados para el MVP.

## 3. Alcance del MVP

### Incluido

- nueva pestaña interna `Studio`;
- listado de borradores propios;
- documentos de bloques con texto e imágenes;
- metadatos comunes: título, resumen, autor visible, idioma, etiquetas y portada;
- guardado automático en el servidor;
- revisiones y detección de conflictos;
- previsualización de documento, Cabinet y Moments;
- adjuntos con subida progresiva;
- roles/capacidades de autor y publicador;
- publicación en una colección permitida;
- indexación de documentos publicados;
- apertura del documento publicado como un `Item` normal;
- retirada de publicación sin destruir el borrador.

### Fuera del MVP

- colaboración simultánea tipo Google Docs;
- comentarios y aprobación editorial multinivel;
- plantillas creadas por usuarios;
- maquetación libre por píxeles;
- importación completa de DOCX;
- generación de EPUB/PDF final;
- edición de vídeo o audio;
- IA generativa, resumen, transcripción o etiquetado automático;
- publicación en Internet.

## 4. Estado actual relevante

### 4.1 Cliente

`noumon/src/App.svelte` mantiene el estado de pestañas con `kind`, `view`, historial atrás/adelante y título. `Reader.svelte` enruta las vistas internas y ya tiene carriles específicos para Cabinet, Moments, mapas, ajustes e Items.

Studio encaja en este sistema, pero un documento abierto necesita más identidad que una vista genérica:

```text
kind: "studio"
documentId: "..."
studioMode: "edit" | "preview"
title: "Manual de mantenimiento"
dirty: true | false
```

El esquema `library://` actual puede ampliarse inicialmente sin renombrar el protocolo:

```text
library://studio
library://studio/new?template=technical
library://studio/document/<id>
```

La marca visible seguirá siendo Noumon aunque el protocolo interno conserve `library://` por compatibilidad.

### 4.2 Notas

`NoteEditor.svelte` es un modal de texto plano asociado a un Item. No es una base adecuada para Studio salvo como referencia visual. Las notas deben seguir siendo anotaciones personales ligeras; un documento Studio tiene identidad, bloques, adjuntos, estado, revisiones y publicación.

### 4.3 Cabinet y Moments

El servidor ya contiene:

- `sidecar` con metadatos comunes y campos audiovisuales;
- `templateForExt` para `video`, `audio`, `gallery`, `pdf` y `reader`;
- `collection.json` por carpeta;
- `UploadForm.svelte` en el Panel;
- `/api/admin/upload` y `/api/admin/media/update`;
- renderizadores Cabinet/Moments en el cliente;
- conversión de media a `Item` y búsqueda federada.

Esto permite reutilizar el contrato publicado, no la API administrativa. Studio necesita su propia API autenticada y el servidor materializará la publicación compatible con Cabinet/Moments cuando corresponda.

### 4.4 Permisos

Hoy existen usuarios con `is_admin` y acceso por colección (`open`, `login`, `blocked`, edad y descarga). No existen capacidades de autor/publicador, grupos ni ACL por documento.

Para el MVP se añadirán capacidades explícitas:

- `can_author`: puede crear y editar borradores propios;
- `can_publish`: puede publicar en colecciones que el administrador le haya habilitado;
- `is_admin`: conserva todas las capacidades y la administración global.

Un lector no obtiene permisos de servidor por mostrar u ocultar botones en el cliente. Cada endpoint Studio los comprobará en Core.

## 5. Arquitectura propuesta

```text
┌──────────────────────── Cliente Noumon ────────────────────────┐
│ StudioShell                                                    │
│ ├─ StudioLibrary        borradores y publicados propios        │
│ ├─ StudioEditor         lienzo de bloques                      │
│ ├─ StudioInspector      metadatos, destino y permisos          │
│ ├─ StudioPreview        render real compartido                 │
│ └─ studioApi.js         contrato HTTP                          │
└──────────────────────────────┬──────────────────────────────────┘
                               │ API autenticada
┌──────────────────────────────▼──────────────────────────────────┐
│ Library Server / studio.go                                    │
│ ├─ autorización de autor/publicador                           │
│ ├─ validación y normalización de bloques                       │
│ ├─ borradores, revisiones y publicación en SQLite              │
│ ├─ staging y promoción atómica de adjuntos                     │
│ ├─ proyección a Item / Cabinet / Moments                       │
│ └─ índice FTS5 de contenido Studio                             │
└───────────────┬────────────────────────────┬────────────────────┘
                │                            │
       POOL_ROOT/studio              POOL_ROOT/downloads
       borradores/adjuntos           contenido publicado existente
```

### Decisión fundamental

El documento canónico vive en el dominio Studio. Publicar no convierte el borrador en un fichero sin dueño: crea o actualiza una proyección publicada. De esta forma se puede retirar, corregir y volver a publicar sin perder el historial editorial.

## 6. Modelo de contenido

### 6.1 Documento canónico

El cuerpo se almacenará como JSON versionado y neutral respecto al editor:

```json
{
  "schemaVersion": 1,
  "presentation": {
    "contentWidth": "reading",
    "fontPreset": "editorial"
  },
  "blocks": [
    { "id": "b1", "type": "heading", "level": 1, "text": "Sistema hidráulico" },
    { "id": "b2", "type": "paragraph", "text": "Procedimiento de inspección..." },
    { "id": "b3", "type": "image", "assetId": "asset-123", "caption": "Válvula principal" },
    { "id": "b4", "type": "code", "language": "text", "text": "PRESION_MAX=12" }
  ]
}
```

Bloques permitidos en el MVP:

- `paragraph`;
- `heading` niveles 1–3;
- `bulletList` y `orderedList`;
- `quote`;
- `image` con pie y texto alternativo;
- `table` limitada;
- `code`;
- `callout`;
- `divider`;
- `columns`, limitado a dos columnas y apilado automáticamente en pantallas
  estrechas;
- `itemRef` para enlazar otro Item de Noumon.

No se admite `html`, `script`, iframes arbitrarios, atributos de evento ni URLs `javascript:`. El servidor valida el esquema, limita profundidad/tamaños y extrae el texto indexable a partir de bloques conocidos.

`presentation` permite personalizar la página con presets versionados y
compatibles con todas las pieles: ancho de lectura, ancho amplio o márgenes
editoriales; y tipografía editorial o sans. No acepta CSS, fuentes remotas,
colores arbitrarios ni valores libres. De este modo dos documentos pueden tener
composiciones claramente distintas sin perder accesibilidad, adaptación móvil
ni coherencia con Modern/Retro y claro/oscuro.

Un `itemRef` conservará además de `itemId` una instantánea mínima del título y tipo visibles en el momento de insertarlo:

```json
{
  "id": "b8",
  "type": "itemRef",
  "itemId": "zim:...",
  "titleSnapshot": "Sistema hidráulico",
  "kindSnapshot": "article"
}
```

Al renderizar, el cliente intenta resolver el Item actual. Si ya no existe, muestra una tarjeta atenuada con `Contenido ya no disponible` y el título guardado; si existe pero el lector no tiene acceso, muestra `Contenido restringido`. En ninguno de los dos casos se rompe el documento, su publicación ni el resto de su índice.

### 6.2 Tablas conceptuales

Las migraciones se implementarán de forma idempotente en `store.go` o en un módulo de migración Studio.

```sql
studio_documents (
  id TEXT PRIMARY KEY,
  owner_user_id INTEGER,               -- NULL solo bajo custodia administrativa
  template_key TEXT NOT NULL,
  status TEXT NOT NULL,              -- draft | published | archived
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT '',
  author_label TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  content_json TEXT NOT NULL,
  plain_text TEXT NOT NULL DEFAULT '',
  cover_asset_id TEXT,
  revision INTEGER NOT NULL DEFAULT 1,
  published_revision INTEGER,
  publication_kind TEXT,             -- document | cabinet | moments
  publication_target TEXT,
  created INTEGER NOT NULL,
  updated INTEGER NOT NULL,
  published INTEGER
)

studio_revisions (
  document_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  editor_user_id INTEGER,
  editor_label TEXT NOT NULL DEFAULT '',
  snapshot_json TEXT NOT NULL,
  created INTEGER NOT NULL,
  PRIMARY KEY (document_id, revision)
)

studio_assets (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL,
  owner_user_id INTEGER,
  filename TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  state TEXT NOT NULL,                -- staged | published | deleted
  created INTEGER NOT NULL
)

user_capabilities (
  user_id INTEGER PRIMARY KEY,
  can_author INTEGER NOT NULL DEFAULT 0,
  can_publish INTEGER NOT NULL DEFAULT 0
)

studio_publish_targets (
  user_id INTEGER NOT NULL,
  collection_id TEXT NOT NULL,
  PRIMARY KEY (user_id, collection_id)
)
```

Se usará el ID numérico del usuario como propietario. El nombre de usuario es visible, pero no debe ser clave de propiedad porque podría cambiar en el futuro. `author_label` y `editor_label` son instantáneas históricas: una transferencia de propiedad o eliminación de cuenta no reescribe la autoría mostrada.

### 6.3 Índice

Se añadirá una tabla FTS5 separada para Studio, actualizada explícitamente dentro de la misma operación lógica de guardado/publicación:

```text
title, summary, plain_text, tags, author_label
```

El índice distinguirá:

- borrador: solo aparece en la búsqueda interna de Studio del propietario;
- publicado indexado: aparece en la búsqueda federada para quien pueda verlo;
- publicado sin indexar: se abre mediante enlace/ID, pero no aparece en resultados;
- retirado: deja de aparecer en la búsqueda pública.

La búsqueda actual de media recorre sidecars en disco por consulta. Studio no debe añadir otro recorrido completo del filesystem; FTS5 es más predecible en una Raspberry Pi y escala mejor con documentos largos.

### 6.4 Eliminación de la cuenta propietaria

Una cuenta con documentos Studio no se elimina dejando claves colgantes ni transfiriendo contenido silenciosamente. El Panel obliga a resolver su contenido mediante una de estas acciones:

1. **Transferir a otro autor/publicador** — opción recomendada; cambia el propietario técnico y conserva `author_label`.
2. **Archivar borradores y conservar publicaciones** — los documentos quedan con `owner_user_id = NULL`, bajo custodia administrativa, hasta su reasignación.
3. **Retirar y archivar todo** — elimina la exposición pública, conserva revisiones y permite recuperación administrativa.

El borrado definitivo del contenido es una operación posterior y separada. La eliminación de cuenta, transferencia, retirada y archivado se ejecutan en una transacción: si alguna parte falla, la cuenta permanece. El servidor no permitirá borrar una cuenta con contenido sin enviar una estrategia explícita.

Las publicaciones conservadas no cambian de autor visible. Un administrador puede gestionarlas mientras estén bajo custodia, pero debe reasignarlas antes de continuar su edición ordinaria.

## 7. Plantillas

Las plantillas oficiales definen campos, bloques iniciales, reglas de validación, previsualización y destino. En el MVP son código versionado por Noumon/Server, no documentos editables por usuarios.

### 7.1 Documento

- título;
- resumen;
- autor visible;
- idioma;
- etiquetas;
- portada opcional;
- cuerpo componible por bloques;
- presets de ancho, márgenes y tipografía;
- reordenación, duplicado y eliminación de bloques.

Se publica como Item nativo Studio y se renderiza con `StudioDocument.svelte`.

### 7.2 Artículo técnico

Parte de la plantilla Documento y siembra bloques sugeridos:

- objetivo;
- requisitos;
- procedimiento;
- advertencias;
- resultados;
- referencias.

No obliga al autor a conservarlos.

### 7.3 Relato

- título y subtítulo;
- autor;
- sinopsis;
- portada;
- cuerpo por capítulos o secciones.

En el MVP se publica como documento. El empaquetado como libro/EPUB se deja para una fase posterior.

### 7.4 Cabinet

Reutiliza los campos actuales:

- archivo principal;
- portada;
- título, autor y año;
- descripción;
- idioma;
- categorías/etiquetas;
- contribuidor;
- licencia;
- texto/OCR opcional.

La vista previa debe reutilizar las mismas tarjetas y la misma ficha que `Cabinet.svelte`/`ItemPage.svelte`. Al publicar, el servidor crea una proyección compatible con el sidecar actual dentro de una colección Cabinet autorizada.

### 7.5 Moments

Reutiliza y amplía los campos actuales:

- vídeo;
- miniatura;
- título;
- canal/autor y avatar;
- descripción;
- fecha;
- duración;
- etiquetas;
- subtítulos;
- capítulos `{start, title}`;
- procedencia opcional.

La vista previa usa los componentes visuales de Moments. Studio no edita el vídeo: prepara el archivo, metadatos y presentación.

## 8. Previsualización compartida

No se crearán dos interfaces independientes que imiten el resultado.

Se extraerán componentes presentacionales puros:

```text
CabinetCard.svelte
CabinetItemView.svelte
MomentsCard.svelte
MomentsWatchView.svelte
StudioDocumentView.svelte
```

Cabinet y Moments normales recibirán un `Item` del servidor. StudioPreview construirá un `Item` provisional con el mismo contrato y URLs temporales para los archivos locales seleccionados. Así la diferencia entre vista previa y publicación será mínima y comprobable.

La previsualización no ejecutará HTML del usuario. Renderizará el JSON validado mediante componentes Svelte.

## 9. API propuesta

Todas las rutas siguientes requieren una sesión humana válida. No pertenecen a `adminMux`.

### Capacidades y plantillas

```text
GET /api/studio/capabilities
GET /api/studio/templates
GET /api/studio/publish-targets
```

`capabilities` permite que clientes y servidores de versiones distintas degraden la interfaz correctamente.

### Documentos

```text
GET    /api/studio/documents?status=draft
POST   /api/studio/documents
GET    /api/studio/documents/{id}
PUT    /api/studio/documents/{id}
DELETE /api/studio/documents/{id}
GET    /api/studio/documents/{id}/revisions
POST   /api/studio/documents/{id}/restore/{revision}
```

`PUT` incluirá `baseRevision`. Si no coincide con la revisión actual, devuelve `409 Conflict` y no pisa silenciosamente trabajo realizado desde otro equipo.

### Adjuntos

```text
POST   /api/studio/documents/{id}/assets
GET    /api/studio/documents/{id}/assets/{assetId}
DELETE /api/studio/documents/{id}/assets/{assetId}
```

La lectura comprueba propiedad del borrador o permisos de la publicación. Las rutas físicas nunca aparecen en la API.

### Publicación

```text
POST /api/studio/documents/{id}/publish
POST /api/studio/documents/{id}/unpublish
GET  /api/studio/publications/{id}
```

Ejemplo conceptual:

```json
{
  "baseRevision": 12,
  "kind": "cabinet",
  "targetCollectionId": "col:media:...",
  "indexed": true
}
```

El servidor comprueba de nuevo autoría, `can_publish`, destino permitido, validez de adjuntos y compatibilidad de plantilla. El cliente no puede publicar eligiendo una carpeta física.

## 10. Integración en el cliente

### 10.1 Navegación

Studio se añadirá como destino visible solo si el servidor informa `canAuthor` o `canPublish`.

Entrada recomendada:

- botón `Crear` en el panel de inicio;
- acceso `Studio` en el sidebar;
- nueva pestaña al crear o abrir un borrador.

Abrir Studio no debe reemplazar el artículo que el usuario está consultando: la creación nace en una pestaña nueva para permitir consultar fuentes en paralelo.

### 10.2 Componentes

```text
noumon/src/lib/studio/
├── Studio.svelte
├── StudioLibrary.svelte
├── StudioEditor.svelte
├── StudioToolbar.svelte
├── StudioInspector.svelte
├── StudioPreview.svelte
├── StudioPublish.svelte
├── TemplatePicker.svelte
├── blocks/
│   ├── BlockEditor.svelte
│   ├── BlockRenderer.svelte
│   └── ...
└── studioApi.js
```

No se elegirá todavía una librería de editor como parte de esta especificación. El formato canónico será propio y estable; el motor visual podrá evaluarse por tamaño, accesibilidad, licencia y compatibilidad Svelte 5. Si se usa una dependencia, no debe definir el formato persistido de Noumon.

### 10.3 Estado y autoguardado

- cambios locales inmediatos en memoria;
- autoguardado con debounce aproximado de 1–2 segundos;
- indicador `Guardando…`, `Guardado` o `Sin conexión`;
- guardado explícito con `Ctrl+S`;
- `baseRevision` en cada escritura;
- aviso antes de cerrar una pestaña con cambios no enviados;
- copia de recuperación limitada en IndexedDB para sobrevivir a una caída de red o cierre del cliente.

La copia local es recuperación, no fuente de verdad ni modo completo sin servidor.

### 10.4 Gestión de capacidades en el Panel

La vista Usuarios del Panel incorporará una sección de autoría, no solo dos campos añadidos sin contexto:

- `Puede crear contenido` (`can_author`);
- `Puede publicar contenido` (`can_publish`);
- selector `Puede publicar en`, agrupado por Documentos, Cabinet y Moments;
- consumo y cuota asignada del usuario;
- aviso de documentos pendientes al intentar eliminar la cuenta.

Activar publicación activa también autoría. Desactivar autoría desactiva publicación y retira sus destinos asignados. Un administrador posee ambas capacidades, pero la protección de espacio libre también se le aplica. Cada cambio de capacidades, destinos o cuota queda registrado en la auditoría.

El selector solo enumera colecciones válidas y muestra su nivel de acceso actual; no expone carpetas físicas. El servidor vuelve a validar los destinos en cada publicación aunque el cliente conserve una lista antigua.

### 10.5 Decisiones de interfaz cerradas

Estas decisiones completan y concretan 10.1–10.3. Su contrato visual está en el
[mockup interactivo de Studio](mockups/noumon-studio-concept.html).

**Entrada y ventana**

- Studio aparece como apartado propio del sidebar, encima de
  **BIBLIOTECAS**.
- Se abre en una pestaña dedicada y no reemplaza el contenido que el usuario
  estaba leyendo.
- Su inicio muestra **Crear** —Documento, Cabinet y Moments— y
  **Seguir creando**, con los borradores recientes y su estado.
- El tipo se elige al crear el borrador. Dentro del editor no existe un
  conmutador que transforme un Documento en Cabinet o Moments.

**Shell de Studio**

- La barra superior deja de ser navegación: desaparece el recuadro de
  dirección y su espacio pasa a mostrar título, estado de guardado,
  previsualización y publicación.
- La salida hacia el inicio de Studio permanece visible para que el editor no
  se sienta como una ruta sin retorno.
- La parte fija contiene volver, estado, previsualizar y publicar. La parte
  contextual muestra herramientas de bloques en Documento, ficha/archivo en
  Cabinet y vídeo/miniatura/capítulos en Moments.
- La shell usa superficies rellenas coherentes con Noumon. Los bordes se
  reservan para campos, zonas de arrastre, validaciones y separaciones con
  función.

**Sidebar contextual**

- En Inicio, el sidebar muestra la biblioteca de borradores y un resumen de
  estados.
- Durante la edición cambia al contexto del contenido activo: estructura,
  bloques, diseño, metadatos, revisiones, destino y cuota. No mezcla
  simultáneamente toda la lista de borradores con el inspector del documento.

**Documento personalizable**

- Documento es un lienzo componible, no una plantilla visual cerrada.
- El autor puede insertar, reordenar, duplicar y eliminar los bloques definidos
  en 6.1, incluidos títulos, texto enriquecido, imágenes, tablas, citas,
  avisos, código, listas, separadores y dos columnas.
- Puede escoger presets de ancho, márgenes y tipografía. Son valores
  versionados y seguros; no se admite CSS ni HTML arbitrario.
- La composición se adapta automáticamente a pantallas estrechas y a las
  cuatro combinaciones Modern/Retro y claro/oscuro.

**Cabinet y Moments**

- Cabinet usa un formulario propio para archivo, ficha, portada, descripción,
  licencia y demás metadatos, acompañado por la tarjeta/ficha final real.
- Moments sigue el mismo patrón para vídeo, miniatura, canal, capítulos y
  subtítulos. Studio prepara archivo y presentación; no edita el vídeo.
- Ambos reutilizan los componentes publicados para que la previsualización sea
  contractual y no una imitación.

**Estado editorial**

- Guardar nunca publica: el contenido continúa como borrador privado hasta una
  acción explícita.
- La interfaz distingue `Guardando…`, `Guardado`, `Sin conexión`, error,
  borrador, publicado y publicado con cambios pendientes.
- Publicar se desactiva cuando faltan campos obligatorios y explica qué debe
  completar el autor.
- La futura superficie Articles será otro destino del mismo Documento de
  bloques, no un cuarto editor.

## 11. Almacenamiento y publicación

### 11.1 Borradores

Los JSON y metadatos viven en `library.db`. Los binarios se guardan bajo:

```text
POOL_ROOT/studio/<document-id>/assets/
```

Studio debe aparecer como sección propia en el inventario del pool y en las copias de seguridad.

### 11.2 Publicación de documentos nativos

Un documento enriquecido permanece en las tablas Studio y se expone como un `Item`:

```text
provider: studio
kind: document
open.mode: studio-document
open.itemId: studio:<id>
```

`Reader.svelte` abrirá `StudioDocumentView`. Los assets se servirán por una ruta con gate Studio, cabeceras `nosniff`, CSP apropiada y tipos MIME validados.

### 11.3 Publicación en Cabinet/Moments

Para mantener compatibilidad con las superficies actuales, el servidor materializará de forma atómica el archivo y sidecar en el destino autorizado. El sidecar ganará campos opcionales:

```json
{
  "studio_id": "...",
  "owner_user_id": 7,
  "published_revision": 12
}
```

Estos campos permiten actualizar o retirar la proyección correcta y auditar el origen sin alterar los lectores antiguos.

La operación seguirá el patrón:

```text
validar → escribir temporal → fsync/close → rename atómico → actualizar DB/índice
```

Si falla, el borrador permanece intacto y no aparece una publicación parcial.

### 11.4 Relación con ZIM

Studio no genera un ZIM por documento. Un ZIM es un paquete de distribución prácticamente inmutable; recompilar uno por cada guardado haría lentas la edición, retirada e indexación.

```text
Documento Studio vivo
    → SQLite: bloques, permisos y revisiones
    → FTS5: búsqueda
    → assets del pool
    → Item nativo de Noumon
```

Cabinet y Moments continúan usando fichero y sidecar. Para el lector, todos aparecen en la búsqueda federada aunque internamente no sean ZIM.

En una fase futura y mediante especificación independiente, un administrador podrá **exportar una colección publicada completa a ZIM**. Esa exportación será una instantánea versionada para USB, traslado entre servidores, conservación o distribución offline; no sustituirá los originales editables de Studio.

## 12. Visibilidad e indexación

### MVP recomendado

- **Borrador privado:** solo propietario y administrador; índice privado de Studio.
- **Publicado en colección:** hereda acceso, edad y descarga de la colección; aparece en búsqueda solo para usuarios autorizados.
- **Sin indexar:** mantiene el mismo permiso de lectura, pero no aparece en búsqueda.
- **Retirado:** deja de estar disponible públicamente y vuelve a borrador.

No se implementará una falsa visibilidad por Item sobre `/media` mientras el gate real siga siendo por carpeta/colección. Si más adelante se necesitan publicaciones para usuarios o grupos concretos, deberá añadirse un modelo ACL de Items y aplicarse también a streaming, previews, búsqueda y descargas.

Esta limitación debe ser visible en la UI: en el MVP se publica **en una colección**, no con una lista arbitraria de personas.

## 13. Subidas desde navegador y aplicación nativa

`MOMENTS-UPLOAD.md` documenta que WebView2 no entrega correctamente multipart a través del `AssetServer` de Wails. Studio debe reutilizar el carril directo al Core, pero con un token más estrecho que el administrativo:

```text
POST /api/studio/documents/{id}/upload-token
```

Propiedades recomendadas:

- vida de 5 minutos;
- ligado a usuario, documento y operación `asset-upload`;
- no reutilizable tras completar la subida;
- no concede administración;
- válido solo para el endpoint y documento indicados.

En navegador/PWA same-origin se usa la cookie normal. En shell nativo se obtiene el token por el proxy y el multipart se envía directamente a Core. Debe extraerse un helper de transporte compartido en vez de copiar la lógica del Panel.

## 14. Seguridad

### Obligatorio desde la primera fase

- sesión requerida para toda autoría;
- comprobación de propietario en cada lectura/escritura de borrador;
- capacidades comprobadas en servidor;
- destino de publicación validado contra una lista permitida;
- límites de tamaño total y por archivo;
- streaming a disco, no carga completa en RAM;
- MIME detectado por contenido y extensión permitida;
- imágenes raster saneadas; SVG/HTML no admitidos como portada;
- nombres generados por el servidor, no rutas del cliente;
- anti-traversal y protección frente a symlinks;
- JSON con límites de bloques, texto, tabla y profundidad;
- sin HTML arbitrario;
- assets privados con `nosniff`, CSP y gate de autorización;
- protección CSRF en peticiones con cookie;
- tokens directos de subida con alcance limitado;
- búsqueda filtrada antes de devolver título o snippet;
- borrado lógico inicial y limpieza diferida de assets huérfanos;
- auditoría mínima: creador, editor, publicación, retirada y fecha.

### Riesgo específico

El código actual filtra media por colección y escanea sidecars del filesystem. Una proyección Studio privada no puede colocarse sin más en una colección pública porque `/media/<ruta>` autorizaría por carpeta. Los borradores y documentos privados usarán almacenamiento/rutas Studio, no `/media` público.

## 15. Rendimiento y cuotas en hardware modesto

### 15.1 Cuotas por defecto

Los límites serán configurables globalmente y por usuario, pero la instalación nace con valores conservadores:

| Recurso | Límite inicial |
|---|---:|
| JSON de un documento | 2 MB |
| Imagen individual | 12 MB |
| Imágenes/adjuntos por documento | 100 MB |
| Archivo de Cabinet | 512 MB |
| Vídeo de Moments | 1 GB |
| Contenido total propiedad de un autor | 2 GB |
| Borradores activos por autor | 100 |

Staging y publicaciones cuentan para la cuota; publicar no permite evadirla. Una proyección no contabiliza dos veces el mismo binario canónico. El servidor rechaza nuevas subidas cuando queda menos del 10 % del pool o 1 GB libre, lo que sea mayor, incluso para administradores.

El cliente consulta la capacidad antes de comenzar archivos grandes y muestra consumo, límite y motivo del rechazo. El MVP no redimensiona imágenes automáticamente: si superan el límite, solicita al usuario reducirlas sin gastar CPU ni modificar silenciosamente el original.

### 15.2 Reglas de rendimiento

- SQLite con WAL y una conexión se mantiene para metadatos;
- FTS5 solo indexa texto normalizado, no binarios;
- extracción de texto ocurre al guardar/publicar, no en cada búsqueda;
- imágenes no se reprocesan continuamente durante la edición;
- autoguardado agrupa cambios;
- listado de documentos paginado;
- revisiones compactadas mediante política configurable;
- subida y copia de archivos mediante streaming;
- no se inicia ningún servicio adicional.

En una Raspberry Pi, un documento de texto debe poder guardarse y aparecer en búsquedas sin depender de procesos externos.

## 16. Fases de implementación

### Fase 0 — contratos y migraciones

- añadir capacidades de usuario;
- diseñar en el Panel capacidades, destinos publicables, cuota y resolución de cuenta con contenido;
- añadir tablas Studio y pruebas de migración;
- definir structs y validación del documento de bloques;
- exponer `GET /api/studio/capabilities`;
- actualizar la regla de producto en README: Noumon consume y también crea mediante APIs de usuario, pero no administra el servidor.

### Fase 1 — borradores de texto

- CRUD de documentos propios;
- pestaña Studio;
- selector de plantilla Documento/Artículo técnico/Relato;
- bloques de texto básicos;
- autoguardado y conflictos;
- previsualización de documento;
- sin publicación todavía.

### Fase 2 — imágenes y publicación nativa

- staging de assets;
- transporte directo del shell;
- bloques de imagen;
- Item `studio-document`;
- FTS5 y búsqueda federada;
- publicar, retirar y abrir desde resultados.

### Fase 3 — Cabinet

- plantilla y metadatos Cabinet;
- componentes de preview compartidos;
- destinos permitidos;
- materialización sidecar atómica;
- edición y republicación.

### Fase 4 — Moments

- plantilla Moments;
- vídeo, miniatura y avatar;
- capítulos y subtítulos;
- preview compartida;
- publicación y reproducción final.

### Fase 5 — madurez editorial

- historial navegable y restauración;
- duplicar documentos/plantillas;
- papelera y retención;
- permisos por grupos o flujo de aprobación, si el uso real lo exige;
- libros por capítulos y exportación, mediante una especificación separada.

## 17. Pruebas mínimas

### Servidor

- migración desde una base existente;
- lector sin capacidades no puede crear;
- autor solo ve/edita sus borradores;
- publicador solo usa destinos asignados;
- administrador puede recuperar contenido sin saltarse auditoría;
- conflicto de revisión devuelve 409;
- documento inválido se rechaza;
- HTML/script no se almacena como bloque ejecutable;
- límites de subida y MIME;
- token de subida no sirve para otro documento ni otra ruta;
- publicación/retirada actualiza búsqueda;
- búsqueda privada no filtra títulos/snippets;
- borrar una cuenta con contenido sin estrategia se rechaza;
- transferencia, custodia y retirada por baja de usuario son transaccionales;
- `itemRef` inexistente o restringido degrada sin filtrar ni romper el documento;
- cuotas incluyen staging y publicación y respetan la reserva de espacio libre;
- fallo a mitad de publicación no deja sidecar visible;
- upgrade/downgrade tolera campos sidecar opcionales.

### Cliente

- abrir Studio crea una pestaña independiente;
- atrás/adelante y cierre respetan cambios pendientes;
- autoguardado y recuperación de red;
- conflicto no pisa contenido;
- preview coincide con el componente publicado;
- servidor sin Studio oculta las acciones;
- temas Modern/Retro y claro/oscuro;
- navegación por teclado y lector de pantalla;
- interfaz en español e inglés.

### Flujo nativo

- multipart desde PWA;
- multipart desde `noumon-client.exe` remoto;
- multipart desde todo-en-uno;
- archivo grande sin crecimiento equivalente de RAM;
- compilación e instalación completa según `COMPILACION-NATIVA.md`.

## 18. Criterios de aceptación del MVP

1. Un administrador puede conceder autoría y publicación sin convertir al usuario en administrador.
2. Un autor abre Studio en una pestaña y crea un documento con texto e imágenes.
3. El borrador se recupera desde otro cliente con la misma cuenta.
4. Otro usuario no puede descubrir ni abrir ese borrador.
5. La vista previa usa el aspecto final.
6. Un publicador elige únicamente colecciones permitidas.
7. La publicación aparece en búsquedas autorizadas y se abre como Item.
8. Retirarla elimina su exposición sin borrar el borrador.
9. El flujo funciona sin Internet y sin IA.
10. El consumo de recursos sigue siendo razonable en hardware ARM/x86 modesto.
11. Eliminar una cuenta nunca deja propietarios colgantes ni borra publicaciones silenciosamente.
12. Un enlace a contenido retirado no impide leer el resto del documento.

## 19. Archivos que probablemente cambiarán

### Cliente

- `noumon/src/App.svelte` — nuevo tipo de pestaña e historial Studio;
- `noumon/src/lib/Reader.svelte` — enrutado del estudio y documento publicado;
- `noumon/src/lib/Sidebar.svelte` y `Home.svelte` — entradas Crear/Studio;
- `noumon/src/lib/libraryAddress.js` — direcciones Studio;
- `noumon/src/lib/auth.svelte.js` — capacidades recibidas;
- `noumon/src/lib/Cabinet.svelte`, `Moments.svelte`, `ItemPage.svelte` — extraer presentadores compartidos;
- nuevos módulos bajo `noumon/src/lib/studio/`;
- mensajes de `i18n` en español e inglés.

### Servidor

- `library-server/core/store.go` — migraciones/capacidades;
- nuevo `library-server/core/studio.go` — API y dominio;
- nuevo `library-server/core/studio_store.go` — persistencia;
- nuevo `library-server/core/studio_assets.go` — staging/streaming;
- nuevo `library-server/core/studio_search.go` — FTS;
- `library-server/core/items.go` — proyección a Item y búsqueda federada;
- `library-server/core/main.go` — rutas y sección del pool;
- `library-server/core/sidecar.go` — campos opcionales de procedencia Studio;
- `library-server/core/upload.go` — extraer utilidades compartidas, sin abrir la ruta admin;
- Panel de Control — capacidades de usuario y destinos publicables.
- Panel de Control — cuotas y flujo de eliminación/transferencia de propietarios.

## 20. Decisiones cerradas y pendientes

### Cerradas por esta propuesta

- Studio vive en Noumon como experiencia, pero los datos viven en Server.
- No se reutiliza `/api/admin/upload` como API de autor.
- Borrador privado por defecto.
- Formato canónico de bloques JSON, no HTML.
- Preview y vista final comparten componentes.
- Publicación Cabinet/Moments reutiliza el contrato sidecar.
- Los documentos largos usan FTS5.
- IA completamente opcional y fuera del alcance.
- Studio crea Items vivos; ZIM queda como exportación opcional de colecciones.
- Las cuotas iniciales son conservadoras y configurables.
- La baja de un autor exige transferir, custodiar o retirar su contenido.
- Studio tiene inicio propio, pestaña dedicada y shell sin dirección durante la
  edición.
- Inicio y edición usan sidebars contextuales distintos.
- Documento es un lienzo de bloques personalizable mediante presets seguros.
- Cabinet y Moments usan formularios especializados con preview real, no el
  editor de bloques.
- Articles será un destino futuro del Documento, no otro editor.

### Pendientes antes de programar el editor visual

1. Elegir el motor/editor visual tras una prueba pequeña de accesibilidad, tamaño y Svelte 5.
2. Definir si cualquier autor puede publicar en una colección asignada o si hace falta aprobación.
3. Definir la política de revisiones y papelera.
4. Validar como contrato visual el
   [mockup interactivo de Studio](mockups/noumon-studio-concept.html), que ya
   cubre claro/oscuro y Modern/Retro para Inicio, Documento, Cabinet y Moments.

## 21. Recomendación de arranque

No comenzar por subir vídeos ni por un editor complejo. La primera implementación debe demostrar la frontera completa con el menor riesgo:

```text
capacidad de autor
    → crear documento de texto
    → autoguardar borrador privado
    → previsualizar
    → publicar como Item Studio
    → encontrarlo en búsqueda
    → retirarlo sin perder el borrador
```

Cuando este circuito sea sólido, Cabinet y Moments se añaden como proyecciones y plantillas sobre el mismo sistema, no como tres editores independientes.
