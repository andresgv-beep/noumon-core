# Carga de contenido: Studio

La carga manual de contenido para Documentos, Cabinet y Moments vive en
**Noumon Studio**. El Panel de Control ya no contiene formularios de creación o
edición ni publica directamente en el pool.

Studio aporta el ciclo editorial completo que no tenía el formulario heredado:

- borrador privado y autoguardado;
- propietario, revisiones y conflictos de edición;
- perfiles separados para Documento, Cabinet y Moments;
- pistas de audio, capítulos, subtítulos, portadas y avatares según el perfil;
- vista previa antes de publicar;
- publicación, republicación, retirada, archivo y borrado definitivo seguro.

El Panel conserva únicamente las operaciones administrativas globales:

- catálogo Kiwix y cola de descargas;
- inventario y eliminación de contenido ya materializado en el pool;
- almacenamiento, colecciones, usuarios, capacidades y acceso.

## Contrato vigente

Las creaciones usan `/api/studio/*` y los permisos del autor. Las antiguas rutas
administrativas `/api/admin/upload` y `/api/admin/media/update` están retiradas.
Los sidecars del pool continúan siendo compatibles con el escáner y las
superficies lectoras; Studio los materializa al publicar.

Este archivo se mantiene para dejar constancia de la migración y evitar que se
reintroduzca un segundo editor en el Panel.
