# Vídeos de Moments desde carpetas

El flujo editorial de Studio se conserva. Además, Moments detecta vídeos sin
ficha en `DOWNLOAD_ROOT/Moments`, incluidas sus subcarpetas. La proyección es
virtual: no escribe JSON ni modifica el vídeo. Una ficha ya existente conserva
prioridad, título, autoría, descripción y recursos.

El administrador puede añadir carpetas de otros discos en **Panel → Carpetas
de Moments**. Deben ser carpetas del servidor, legibles por la cuenta que ejecuta
Noumon. Los archivos permanecen en el disco seleccionado. No se admite la raíz
completa de un disco ni carpetas que se solapen con el pool u otra fuente.

La configuración vive en `moments_folders` dentro de `library.db`. Las URLs
virtuales usan un identificador persistente de la fuente y la ruta relativa del
vídeo. Se conservan al reiniciar o desconectar físicamente el disco. Retirar la
fuente desde el Panel no elimina sus archivos. Un disco ausente no interrumpe
las demás colecciones.

El catálogo comprueba cambios al consultarse, con caché de 30 segundos. Moments
actualiza su listado mientras está abierto; **Escanear ahora** invalida la caché
del servidor. Las búsquedas y las colecciones usan el mismo catálogo.

Los vídeos detectados heredan los permisos de Moments, incluyendo restricciones
de edad y descargas. El streaming conserva Range. Las carpetas externas solo
sirven archivos de vídeo y no siguen enlaces simbólicos. No se borran desde la
acción de eliminación editorial del Panel.

El nombre del archivo proporciona el título; sin avatar del creador se usa el
logo de Noumon. La miniatura conserva el fallback existente al primer fotograma.
Se reconocen MP4, M4V, WebM, MOV, MKV, AVI, OGV, MPG, MPEG, TS, MTS y M2TS. No se
transcodifica: la reproducción depende de los códecs que admita el navegador.
