# Studio — veredicto de interfaz (2026-07-23)

> **Estado: la interfaz actual de Studio queda RECHAZADA por Andrés.** No se
> parece al contrato visual acordado (`docs/mockups/noumon-studio-concept.html`,
> acordado 2026-07-23) ni cumple la propia especificación (§10.5). El motor de
> servidor, la API y el modelo de datos SÍ están validados y se conservan: lo
> que se rehace es la capa de presentación del cliente.

## Por qué se rechaza (con el contrato en la mano)

La implementación actual incumple decisiones que están ESCRITAS, no gustos:

### 1. Studio está dentro de BIBLIOTECAS — y no es una biblioteca

- **Contrato** (§10.5 y mockup): "Studio aparece como apartado propio del
  sidebar, **encima de BIBLIOTECAS**". El mockup lo dibuja bajo su propio
  rótulo **CREAR**, separado de Navegar y de Bibliotecas.
- **Implementado**: Studio cuelga como una entrada más dentro de BIBLIOTECAS,
  junto a Documentos/Cabinet/Moments. Studio es un TALLER, no una superficie de
  lectura; mezclarlo con las bibliotecas rompe el modelo mental del producto.

### 2. La barra superior sigue siendo un navegador — debía transformarse

- **Contrato** (§10.5 y mockup): "La barra superior **deja de ser navegación**:
  desaparece el recuadro de dirección y su espacio pasa a mostrar título,
  estado de guardado, previsualización y publicación". El mockup la define con
  dos personalidades superpuestas: modo lector (URL, navegación) y modo Studio
  — `[‹ Inicio de Studio] [Título del doc + estado] [herramientas contextuales]
  [Previsualizar] [Publicar]` — con transición entre ambas.
- **Implementado**: la barra de navegación normal sigue ahí con su recuadro de
  URL; Guardar/Publicar/Archivar son botones metidos DENTRO de la vista. No
  existe la barra-herramienta.

### 3. El editor de Documento es un formulario — debía ser un lienzo

Esta es la distancia más grave, y la razón del "no te da libertad":

- **Contrato** (§10.5: "Documento es un **lienzo componible**, no una plantilla
  visual cerrada"; mockup `doc-canvas`): el documento se edita **sobre la
  propia página**, con el aspecto final — bloques `contenteditable` editados en
  su sitio, asa de arrastre (⠿) por bloque, acciones de duplicar/eliminar al
  vuelo, estado seleccionado visible. A un lado, una **paleta** con "Insertar
  bloque" (rejilla de 10 tipos), "Diseño de página" (ancho lectura/amplio/
  márgenes) y "Tipografía" (editorial/moderna).
- **Implementado**: cada bloque es una TARJETA de formulario con un `textarea`
  dentro y botones ↑/↓; la página real solo se ve en una columna de preview
  separada. Editar así es rellenar un patrón, no componer una página. El autor
  no ve lo que hace donde lo hace.

### 4. El sidebar no se vuelve contextual

- **Contrato** (§10.5 y mockup): en el inicio de Studio el sidebar muestra
  **Mis borradores + Estado**; durante la edición cambia al contexto del
  documento — Edición (navegación por secciones), Historial (Revisiones con
  contador), Publicación (Destino, Cuota). "No mezcla simultáneamente toda la
  lista de borradores con el inspector del documento".
- **Implementado**: el sidebar general de la app no cambia; la lista de
  borradores es una columna fija dentro de la vista, siempre visible, y no
  existe el panel contextual de edición (secciones/destino/cuota).

### 5. El inicio de Studio no es "Crear + Seguir creando"

- **Contrato** (mockup `pane-home`): dos zonas — **Crear** con tres tarjetas
  grandes (Documento / Cabinet / Moments, cada una con su descripción) y
  **Seguir creando** con los recientes y su estado (Borrador / Publicado).
- **Implementado**: no hay inicio; se aterriza directamente en el último
  borrador con la lista al lado y un botón "Nuevo documento".

## Lo que SÍ está validado y NO se tira

Las tres revisiones de servidor y la pasada visual dejaron piezas correctas que
la reconstrucción debe REUTILIZAR, no reescribir:

- **Todo el servidor**: API de documentos/revisiones/assets/publicación,
  permisos, snapshots inmutables, grafo de enlaces, FTS (pendiente 1 arreglo,
  ver abajo), baja de autores. Nada de esto cambia por la interfaz.
- **La máquina de autoguardado** (`changeVersion` + promesa única + recovery
  IndexedDB + reintento con backoff): certificada dos veces. La nueva interfaz
  debe montarse ENCIMA de esta lógica, no sustituirla.
- **`StudioDocumentView` / `StudioBlockView`**: son el render de la página
  publicada (con índice automático, columnas, temas verificados en las cuatro
  combinaciones). En el lienzo nuevo, estos mismos componentes son la base
  sobre la que se edita — así la "previsualización contractual" pasa a ser el
  propio editor.
- **El diálogo de plantillas** (`<dialog>` nativo, foco atrapado, Escape): la
  mecánica vale; dónde se dispara cambia (desde la tarjeta "Documento" del
  inicio Crear, si Andrés mantiene las 3 plantillas de texto de §7).
- **`StudioItemReference`** (resolución viva con 4 estados sin fugas) y
  **`StudioImage`**: se conservan tal cual.

## Pendiente de servidor arrastrado (independiente de la interfaz)

- 🔴 ALTO — `restoreStudioRevision` contamina `studio_published_fts` con el
  contenido del borrador restaurado (demostrado en vivo: el título privado
  aparece en la búsqueda pública y el publicado desaparece). Arreglo: no tocar
  la tabla FTS al restaurar cuando el documento está publicado; test con el
  repro publicar-beta → restaurar-alfa → buscar.
- 🟢 Menor — definir el token `--shadow-soft` (hoy computa `none`).

## Puntos que Andrés debe decidir antes de reconstruir

1. Las 3 plantillas de texto (Documento/Técnico/Relato, §7): ¿viven dentro de
   la tarjeta "Documento" del inicio Crear, o desaparecen en favor del lienzo
   libre con bloques?
2. La preview: el mockup la pone como BOTÓN en la barra Studio (modo aparte),
   no como columna permanente. ¿Confirmado?
3. Móvil: el mockup contempla ☰ para el panel lateral en pantalla estrecha.

## Criterio de aceptación de la reconstrucción

La interfaz se dará por buena cuando un lado a lado con
`noumon-studio-concept.html` (abierto en las 4 combinaciones de tema) muestre:
sidebar con CREAR encima de BIBLIOTECAS; barra transformada sin URL en modo
Studio; inicio Crear + Seguir creando; lienzo con bloques editables en su
sitio, asa, duplicar/eliminar y paleta lateral; sidebar contextual con
borradores en inicio y Edición/Historial/Publicación en edición. La validación
final la hace Andrés contra su mockup, no contra esta lista.
