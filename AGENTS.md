# Instrucciones de continuidad

Antes de modificar Studio, Documentos, Cabinet, Moments o la búsqueda federada,
lee por completo:

- `docs/CONTINUIDAD-STUDIO.md`
- `docs/NOUMON-STUDIO-ESPECIFICACION.md`
- `docs/STUDIO-MULTIPAGINA-MAPA-TECNICO.md`

Reglas de trabajo:

1. No crear commits hasta que el usuario haya probado visualmente el cambio y
   dé permiso explícito.
2. Todo cambio del modelo Go exige compilar e instalar el `core.exe` nuevo
   antes de validar la interfaz o la base real.
3. Todo texto nuevo de interfaz debe añadirse simultáneamente en español e
   inglés.
4. No tocar ni incluir en commits la carpeta local no versionada `.claude/`.
5. No usar marcas de terceros para nombrar funciones propias. Se conservan,
   sin embargo, nombres, identificadores y datos factuales de integraciones
   externas cuando sean necesarios para interoperar.

