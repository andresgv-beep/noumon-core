# Memoria de cambios — library-server/supervisor

**Fecha:** 2026-07-20
**Origen:** revisión de código de la copia `noumon-core-main` (commit a559ecc)
**Archivos tocados:** `library-server/supervisor/main.go` y `main_test.go`. Nada más del árbol.
**Verificado:** `gofmt` limpio, `go vet` limpio, 11/11 tests con `-race` (7 previos + 4 nuevos), compilación cruzada Windows amd64 y Linux ARM64.

Los archivos entregados son versiones completas para sustituir a los del repo;
comparar con `git diff` antes de integrar.

---

## Cambio 1 — Reinicio administrativo (código 75) pisado por el reset de uptime

### Problema

En `runLoop`, al salir Core con `restartExitCode` (75) se fijaba
`delay = 300ms`, pero unas líneas después el bloque
`if time.Since(started) > 30*time.Second { delay = time.Second }` lo
sobreescribía. El caso normal de un reinicio administrativo es justo ese: el
servidor lleva tiempo vivo cuando el Panel guarda ajustes. Resultado: el
reinicio "rápido" esperaba siempre 1s, nunca los 300ms previstos.

### Solución

- Nueva función pura `restartDelay(current, uptime, adminRestart) (sleep, next)`
  que concentra toda la decisión de espera y backoff:
  - `adminRestart` se evalúa **antes** que el uptime → siempre 300ms de espera
    y backoff rearmado a 1s.
  - Uptime > 30s (proceso sano) → rearma a 1s.
  - Resto: duerme `current` y arma `current*2` con techo en 30s.
- `runLoop` ya no manipula `delay` en el `case err := <-wait:`; solo marca
  `adminRestart = true` y al final del bucle llama a `restartDelay`.
- La semántica del backoff en caídas encadenadas no cambia: 1→2→4→8→16→30.

## Cambio 2 — Rotación de logs por tamaño

### Problema

`supervisor.log`, `core.log` y `translate.log` se abrían con `O_APPEND` sin
límite. En una instalación de servicio Windows corriendo meses, ProgramData
acumula logs sin cota.

### Solución

- Nuevo helper `openRotatedLog(path, limit)`: al abrir, si el fichero alcanza
  `limit` lo aparta a `path+".old"` (sustituyendo la generación anterior) y
  empieza limpio. Una sola generación → uso acotado a ~2× por proceso.
- Nueva constante `maxLogSize = 5 << 20` (5 MB).
- Lo usan `processLog` (logs de hijos, se reabre en cada relanzamiento) y la
  apertura de `supervisor.log` en `main()`.

### Limitación conocida (documentada en el código)

Los hijos escriben directamente sobre el descriptor heredado, así que la
rotación solo ocurre al (re)abrir. Un Core que viva meses sin reiniciarse puede
superar los 5 MB hasta su siguiente relanzamiento; rotar por debajo del hijo no
es viable en Windows. Se aceptó como coste del diseño sin dependencias.

## Cambio 3 — config.json corrupto degradaba en silencio

### Problema

`coreEnv` hacía `cfg, _ = readSupervisorConfig(configPath)`. Con un
`config.json` corrupto, `contentRoot` y `lanAccess` volvían a cero sin dejar
rastro: el pool caía al directorio de estado y para el usuario "desaparecía la
biblioteca" sin ninguna pista en el log.

### Solución

- `coreEnv` ahora captura el error de lectura/parseo y escribe en
  `supervisor.log`: `config.json ilegible, se usan valores por defecto: <err>`.
- Se ignora `os.ErrNotExist` (fichero ausente es el estado normal de una
  instalación nueva).
- El comportamiento de fallback no cambia; solo se hace visible.
- Nota: `translateModelsDir` lee el mismo config y sigue callando su error a
  propósito, para no duplicar la misma línea de log en cada arranque.

## Tests añadidos (`main_test.go`)

| Test | Cubre |
| --- | --- |
| `TestRestartDelayAdminRestartNotOverriddenByUptime` | Cambio 1: 300ms + rearme a 1s con uptime de horas |
| `TestRestartDelayBackoffAndReset` | Secuencia 1→2→4→8→16→30 y reset con uptime sano |
| `TestOpenRotatedLogRotatesBySize` | Cambio 2: rota al superar el límite, no rota por debajo, `.old` conserva el contenido viejo |
| `TestCoreEnvCorruptConfigFallsBackToStateRoot` | Cambio 3: con config corrupto el entorno cae a los valores por defecto correctos |

## Pendiente fuera de este parche

- `runLoop` sigue sin test de integración (relanzamiento real, código 75 de
  extremo a extremo, parada limpia por contexto). Viable con el patrón de
  binario ayudante (`os.Args[0]` + env var). La lógica de espera ya quedó
  cubierta al extraerla a `restartDelay`.
- `stopProcessTree` (Unix): el `Kill()` de emergencia tras los 10s mata solo al
  proceso líder, no al grupo (`syscall.Kill(-pid, SIGKILL)` sería lo simétrico
  al SIGTERM). Menor: Core no lanza hijos hoy.
- `mergeEnv` compara claves en mayúsculas también en Linux (correcto para
  Windows, teóricamente impreciso en Unix). Inofensivo con las claves actuales.
