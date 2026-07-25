# Continuidad de Noumon Studio

Última actualización: 2026-07-25.

Este documento es la memoria de relevo para continuar Studio desde otro equipo
sin reconstruir decisiones a partir del historial de conversación. Describe el
estado comprobado del código, las trampas conocidas y el orden recomendado de
trabajo. No sustituye a la especificación.

## 1. Punto exacto del repositorio

La rama de referencia es `main`. En el momento de escribir esta memoria,
`HEAD` y `origin/main` apuntan a:

```text
8d26e17 Buscador: la linea de origen del resultado vuelve a decir algo util
19f5f4b Studio: el buscador indexa por pagina y lleva al sitio exacto
9891129 Studio: enlaces internos entre paginas y contenidos
```

El árbol estaba limpio salvo por `.claude/`, carpeta local del usuario que no
se debe tocar ni añadir a Git. Esta memoria y `AGENTS.md` quedan deliberadamente
sin commit para que el usuario los revise y decida cuándo incorporarlos.

## 2. Estado funcional consolidado

Studio ya es el único taller de creación de Noumon:

- `Documentos` publica páginas de conocimiento local.
- `Cabinet` publica PDF, lectura, galería, audio y vídeo mediante plantillas
  especializadas.
- `Moments` publica vídeo con su ficha especializada.
- El Panel conserva administración, catálogo Kiwix y cola, pero no un editor
  creativo paralelo.
- Los borradores son privados; la lectura pública usa snapshots publicados
  inmutables.
- Archivar y eliminar definitivamente contenido, revisiones y assets ya están
  implementados. La eliminación definitiva exige archivar primero.
- Español e inglés se desarrollan a la vez.

La evolución multipágina definida en
`STUDIO-MULTIPAGINA-MAPA-TECNICO.md` está completada de F1 a F6:

1. modelo v2 y normalización permanente de contenido v1;
2. gestor de páginas;
3. menú de contenidos, routing, historial y deep-link;
4. fichas laterales por página;
5. enlaces internos entre páginas y contenidos;
6. FTS por página y búsqueda federada con apertura exacta.

La búsqueda comprobada en la instalación real combina correctamente:

- documentos publicados de Studio;
- artículos de colecciones ZIM;
- contenido de Cabinet y Moments.

Cada resultado conserva una línea de procedencia útil. Los resultados de
Studio llevan `pageId` y `pageTitle`; al abrirlos se navega a la página exacta.

## 3. Verificación de F6 ya realizada

La migración FTS se ejecutó sobre la base real, no solo sobre un `t.TempDir()`:

- esquema con `document_id`, `page_id`, `title`, `page_title`, `summary`,
  `plain_text`, `tags`, `work_type`, `topics` y `author_label`;
- ocho páginas publicadas indexadas en aquella instalación;
- cero filas de Cabinet o Moments dentro del FTS documental;
- el backfill reconstruye desde snapshots publicados;
- editar un borrador sin republicar no cambia títulos, fragmentos ni texto
  público;
- las superficies de media quedan fuera del índice documental y continúan
  entrando en la búsqueda por su proveedor normal.

Pruebas ejecutadas en verde:

- `go test ./...`;
- `go vet ./...`;
- suite Studio con detector `-race`;
- 24 pruebas del cliente;
- build de producción del cliente;
- `gofmt` y `git diff --check`.

También se comprobó la instalación real:

- el hash del `core.exe` instalado coincidía con el binario compilado;
- el servicio arrancó y `/api/health` devolvió 200;
- vista móvil a 390 x 844 sin desbordamiento horizontal;
- búsqueda mediante teclado sin error de ejecución;
- búsqueda visual de `bosques primarios` encontró el documento de Studio;
- búsqueda de `c. tangana` combinó artículos ZIM y el vídeo de Moments.

Los respaldos creados en esa máquina fueron:

```text
C:\Program Files\Noumon\bin\core.exe.pre-f6-20260725-203000
C:\Program Files\Noumon\bin\core.exe.pre-f6-mediafix-20260725-203600
C:\Program Files\Noumon\bin\www-client.pre-f6-20260725-203000
C:\ProgramData\Noumon\data\db\backup-pre-f6-20260725-203000
```

Son referencias de la instalación anterior; no se debe asumir que existirán en
el equipo nuevo.

## 4. Trampas que ya costaron horas

### 4.1 Modelo Go y binario instalado

Tocar el modelo Go obliga a recompilar e instalar `core.exe`. Un binario viejo
deserializa el contenido con su `struct` antiguo y lo vuelve a guardar sin los
campos que desconoce. La pérdida es silenciosa y parece un fallo de la
interfaz.

Después de añadir un campo:

1. compilar;
2. detener el servicio;
3. respaldar base, WAL y SHM;
4. instalar el binario;
5. arrancar el servicio;
6. verificar directamente en
   `C:\ProgramData\Noumon\data\db\library.db` que la clave nueva persiste.

No basta con verla temporalmente en pantalla.

### 4.2 Campos editables de Svelte

Todo campo editable nuevo debe usar `use:richText` o `use:plainText` de
`studioEditable.js`, dejando vacío el elemento editable en la plantilla. Si el
contenido se vuelve a pintar desde el estado, el cursor se altera y el texto
puede escribirse al revés. No probar esto con palíndromos o cadenas visualmente
ambiguas: usar texto como `abc123`.

### 4.3 Anclaje de fichas

Las fichas laterales se anclan a posiciones entre bloques del cuerpo de la
página. No se anclan dentro de columnas. Soltarlas sobre un bloque anidado no
debe reinterpretarse como soporte existente.

### 4.4 Autoguardado

El autoguardado está serializado mediante versión de cambio y promesas. Una
respuesta antigua nunca debe reemplazar el cuerpo que el usuario sigue
editando. Guardar no puede:

- remontar el editor;
- desplazar el scroll;
- hacer parpadear el lienzo;
- perder la página activa;
- sustituir texto reciente por una respuesta anterior.

Recovery IndexedDB v1 debe normalizarse a v2 antes de aplicarse.

### 4.5 SQLite con una conexión

El almacén usa `SetMaxOpenConns(1)`. No se puede abrir una consulta anidada
dentro de un `rows.Next()` activo. Primero se recolectan IDs o datos, se cierra
el cursor y después se lanzan las consultas dependientes.

### 4.6 Marcas e interoperabilidad

Las funciones propias se llaman `Documentos`, `documento multipágina`, `menú
de contenidos`, `ficha lateral`, `artículo ZIM`, etc. No se usa una marca de
terceros como nombre genérico del producto.

No hay que romper referencias externas legítimas. Por ejemplo,
`selCat = 'wikipedia'` en el catálogo Kiwix es un identificador factual de un
servicio de terceros y debe conservarse. También pueden aparecer nombres
factuales dentro del contenido o catálogo importado.

### 4.7 Código local ajeno

`.claude/` pertenece al usuario. Debe permanecer fuera de las modificaciones y
de los commits salvo petición explícita.

## 5. Decisiones de producto que no deben reabrirse

- Studio vive en Noumon, mientras que los datos y permisos viven en Server.
- `Documentos`, `Cabinet` y `Moments` son superficies hermanas de lectura.
- Studio no es una cuarta biblioteca pública.
- La edición de Documentos usa un lienzo flexible de bloques.
- Cabinet y Moments usan formularios especializados; sus antiguas entradas de
  sidebar no deben ser botones decorativos ni duplicar controles ya presentes.
- Las herramientas del documento viven dentro del sidebar contextual principal
  de Studio, no en un segundo sidebar flotante.
- El scroll del panel de herramientas es independiente del lienzo.
- Ocultar el sidebar concede espacio al entorno de trabajo, pero no modifica el
  ancho semántico elegido para la página. Lectura, amplia, editorial y compacta
  conservan sus medidas.
- El menú de contenidos publicado vive en el lateral izquierdo y el índice de
  encabezados en el derecho. Son funciones distintas.
- Las fichas laterales pueden administrarse desde el sidebar y localizarse
  mediante un selector compacto en la barra superior.
- Preview y publicación comparten componentes.
- Nada de HTML libre almacenado: primero se escapa y después se aplica el
  pseudo-markdown seguro.
- Los límites de páginas, bloques, texto y tamaño son globales al documento.
- La colección `Documentos` nace con acceso `login`, no público anónimo.
- La interfaz siempre se implementa en español e inglés en la misma fase.
- Primero se consolida publicación local. Community Creator se abordará solo
  después de estabilidad, verificación y adopción.

## 6. Qué queda realmente pendiente

No confundir dos hojas de ruta:

- la hoja multipágina F1-F6 ya está cerrada;
- la `Fase 6 — madurez editorial` de la especificación general sigue abierta.

La siguiente etapa recomendable es una fase de madurez y saneamiento:

1. auditar el ciclo completo desde una instalación limpia y desde una
   actualización real;
2. definir retención o compactación del historial: durante edición intensiva
   se han observado más de doscientas revisiones;
3. añadir duplicación de documento y plantillas reutilizables, no solo
   duplicación de bloques;
4. decidir si hace falta una papelera con periodo de retención. Archivar,
   recuperar y purgar manualmente ya existen;
5. revisar accesibilidad completa: teclado, foco, lector de pantalla,
   claro/oscuro y Modern/Retro en todas las plantillas;
6. comprobar los instaladores nativos en Windows y Linux, incluido el caso
   symlink real en Linux;
7. especificar por separado libros por capítulos y exportación si se priorizan;
8. incorporar aprobación o permisos por grupos únicamente si el uso real lo
   exige.

Community Creator permanece fuera del trabajo inmediato. La arquitectura debe
seguir conservando IDs estables, clasificación portable, hashes, procedencia y
snapshots inmutables para no cerrarle la puerta, pero todavía no se implementa
catálogo público, identidad del creador, firma, moderación, revocación ni P2P.

## 7. Orden de reanudación recomendado

Al continuar desde otro equipo:

1. ejecutar `git status --short` y confirmar el `HEAD`;
2. leer este documento, la especificación y el mapa multipágina;
3. comprobar que `.claude/` sigue fuera de alcance;
4. ejecutar pruebas Go y cliente antes de modificar nada;
5. confirmar si se trabaja contra una instalación local y qué binario está
   realmente activo;
6. escoger una sola unidad de madurez editorial;
7. implementar, probar y mostrarla al usuario;
8. solicitar revisión de Claude/Fable;
9. crear el commit únicamente después de la prueba visual y la autorización
   expresa del usuario.

Flujo habitual del proyecto:

```text
Codex implementa
  → usuario prueba visualmente
  → Claude/Fable audita código, tests e instalación
  → se corrigen hallazgos
  → usuario autoriza commit
```

