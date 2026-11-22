# Importar contenido desde otro equipo

En el Panel, abrir **Importar → Desde este equipo**. La operación copia los
archivos del equipo donde está abierto el navegador al servidor al que se ha
conectado el Panel. Requiere una cuenta administradora de ese servidor.

- Para copiar una biblioteca publicada, elegir **Detectar por carpetas** y
  seleccionar `Cabinet`, `Moments`, `Depot`, o la carpeta `downloads` que las
  contiene. Se mantienen las colecciones y las subcarpetas.
- Para una colección con otro nombre o para archivos sueltos, elegir primero
  Cabinet, Moments o Programas. Los archivos sueltos van a la colección indicada.
- Se copian medios, instaladores, fichas JSON, portadas, capturas, pistas y
  subtítulos. No se transfieren las cuentas ni los borradores de Studio.
- Los ficheros de fichas se envían al final para que se conserven los metadatos
  originales. Si no hay ficha, se crea una mínima para los formatos reconocidos.
  La arquitectura de un instalador no se adivina.

La selección muestra archivos y tamaño antes de pulsar **Copiar al servidor**.
Mantener abierta la pestaña. Se permite detener la copia: se conservan los
archivos completos y se limpia el temporal del archivo interrumpido. Se puede
reintentar desde la selección actual; ese archivo vuelve a empezar desde cero.
No hay reanudación por bloques ni recuperación de la cola al cerrar el navegador.
Si un fichero ya existe, se conserva y se informa de la colisión.

La escritura se hace en el disco de contenido, no en `/tmp`, con un buffer de
64 KB. La copia es secuencial desde el navegador. Se mantiene la reserva de
espacio libre del servidor. Los destinos se limitan a las tres carpetas del
pool mediante `os.Root`; no se siguen enlaces que salgan de ese árbol.

Validación inicial: pruebas de Core en Windows y binario de pruebas ARM64 en
la Zero 2 W real. Se comprobaron conservación de fichas y portadas, permisos de
administrador, rutas inválidas, enlaces fuera del pool, colisiones y limpieza
de transferencias interrumpidas. Sigue pendiente la prueba visual del usuario
con sus carpetas reales; no se han copiado automáticamente sus contenidos.
