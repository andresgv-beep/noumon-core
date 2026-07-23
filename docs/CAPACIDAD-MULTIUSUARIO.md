# Capacidad multiusuario — metodología y cifras honestas

> Estado: metodología definida y herramienta lista. **Las tablas de resultados
> están vacías a propósito**: se rellenan midiendo hardware real. Ninguna cifra
> de este documento debe publicarse si no salió de una medición reproducible.

## 0. Por qué este documento

"Soporta N usuarios" es la frase más fácil de decir y la más difícil de sostener.
Sin decir *haciendo qué* y *con qué criterio de éxito*, el número no significa
nada: 20 personas leyendo contenido documental y 20 viendo vídeo 1080p son cargas que se
diferencian en dos órdenes de magnitud.

Noumon se instala en casa de gente que no puede pedir soporte. Prometer de más
es peor que prometer poco, así que aquí se fija un criterio estricto, se mide, y
se publica lo que salga.

## 1. El criterio

**Un espectador está bien atendido si el vídeo no se le corta.**

La métrica principal no es la latencia media ni las peticiones por segundo: es el
número de **cortes** (*underruns*), veces que el reproductor agota su búfer y
tiene que parar a recargar. Un servidor con latencia media excelente que provoca
un corte cada dos minutos es un servidor que la gente percibe como roto.

La capacidad publicada de una máquina es **el mayor número de espectadores
simultáneos con CERO cortes** durante la ventana de medida. No el número en el
que "va regular"; el número en el que va bien.

## 2. La herramienta

`scripts/bench` simula espectadores reales. La diferencia clave con un martillo
de carga: cada espectador virtual **se frena cuando llena su búfer**, igual que
un `<video>` de verdad. Descargar a máxima velocidad mediría el ancho de banda
del disco, no la experiencia de nadie.

```bash
cd scripts/bench

# ¿aguanta esta máquina 4 espectadores sin cortes?
go run . -host 192.168.1.50:8090 -users 4 -dur 5m -user admin -pass ...

# ¿cuál es su techo? sube la carga hasta el primer corte
go run . -host 192.168.1.50:8090 -ramp -dur 2m -user admin -pass ...
```

Opciones que importan:

| Opción | Para qué |
|---|---|
| `-bitrate` | 3 Mbps ≈ 720p (por defecto), 6 ≈ 1080p. Sube esto antes que los usuarios si tu contenido es pesado. |
| `-searchers` | Cuántos espectadores además buscan. Es donde aparecen los 429 (ver §4). |
| `-dur` | Ventana de medida. Menos de 2 minutos no detecta la degradación lenta. |
| `-ramp` | Encuentra el techo automáticamente en vez de adivinarlo. |

### Reglas para que la medida valga algo

1. **Medir por la red real, nunca en localhost.** En localhost no existen ni el
   wifi ni la latencia, que es justo donde está el cuello de botella real.
2. **Ventanas de 5 minutos** para la cifra que se publique. Las de 30 segundos
   valen para iterar, no para prometer.
3. **Disco frío y disco caliente dan resultados distintos.** Anotar cuál se midió;
   la caché de páginas del sistema operativo puede duplicar el resultado.
4. **Anotar el almacenamiento.** Tarjeta SD, SSD por USB y NVMe no juegan la
   misma liga, y en la Pi es probablemente la variable que más manda.

## 3. Resultados

### Raspberry Pi

Objetivo declarado: **4 espectadores simultáneos**. No se persigue más; la Pi es
el escenario doméstico y familiar, no el de un aula.

| Modelo | RAM | Almacenamiento | Red | Espectadores sin cortes | Fecha |
|---|---|---|---|---|---|
| _(pendiente)_ | | | | | |

### Equipo de sobremesa / mini-PC

| CPU | RAM | Almacenamiento | Red | Espectadores sin cortes | Fecha |
|---|---|---|---|---|---|
| _(pendiente — medición por red real)_ | | | | | |

#### Sondeo preliminar — NO PUBLICABLE

Medido en **localhost**, sin red de por medio, sobre un fichero de 36 MB que cabe
entero en la caché del sistema operativo. Sirve para saber que el servidor no es
el cuello de botella; **no** para prometer nada a nadie.

- Máquina: Intel i7-14650HX (24 hilos), 32 GB RAM, Windows.
- Carga: espectadores a 6 Mbps (1080p), tramos de 512 KB.

| Espectadores | Agregado | Tramo p50 | Tramo p99 | Cortes |
|---|---|---|---|---|
| 20 | 193 Mbps | 3 ms | 47 ms | 0 |
| 60 | 577 Mbps | 25 ms | 114 ms | 0 |
| 100 | 962 Mbps | 49 ms | 186 ms | 0 |
| 160 | 1540 Mbps | 78 ms | 355 ms | 0 |

No se encontró techo. La lectura útil: **este servidor satura por sí solo un
enlace de gigabit**, así que en cualquier red doméstica o de aula el cuello será
la red o el disco mucho antes que Noumon. Por eso la cifra que se publique tiene
que medirse por red real: es la red la que pone el límite, y eso localhost no lo
puede ver.

Búsqueda global, 24 consultas simultáneas con términos distintos (para esquivar
la caché): **24 respuestas 200, ninguna 429**, la más lenta 2,6 s. El 429 del §4
es más difícil de provocar de lo que sugiere el diseño, porque las búsquedas
terminan rápido y la cola se vacía sola. El riesgo real está en la Pi, donde cada
búsqueda tarda segundos y sí puede llenarse la cola.

### Notas de cada medición

_(Aquí van las observaciones cualitativas: qué se saturó primero, si la CPU
estaba ociosa mientras el disco no daba más, etc. Es lo que permite recomendar
la mejora correcta a quien se quede corto.)_

## 4. Techos conocidos por diseño

Cosas que ya sabemos por el código, para no confundirlas con un fallo:

- **Búsqueda global.** El servidor se autoconfigura a `núcleos/2` búsquedas
  simultáneas (mínimo 2, máximo 6) más una cola de 4. En una Pi de 4 núcleos son
  2 + 4: el séptimo que busque a la vez recibe un **429**. Ajustable con
  `SEARCH_CONCURRENCY`, pero subirlo sin medir puede empeorar el conjunto: en la
  Pi el disco manda y más paralelismo es más contención.
  Es el único límite que devuelve un error visible en vez de ir más despacio.
- **Lectura de artículos y PDF.** Limitada a `2×núcleos` operaciones de motor
  (mínimo 6, máximo 32, `KIWIX_CONCURRENCY`). Al llenarse, la gente **hace cola**
  unas décimas y entra; nadie ve un error.
- **Conexiones del navegador.** Sin HTTPS se habla HTTP/1.1, donde cada navegador
  permite ~6 conexiones por origen **compartidas entre todas sus ventanas**. Esto
  NO afecta a usuarios en equipos distintos (cada equipo tiene su propio cupo),
  pero sí hace que abrir 6+ ventanas con vídeo en una sola máquina se atasque.
  Es un límite del navegador, no del servidor: durante el atasco el servidor
  sigue respondiendo con normalidad a cualquier otro dispositivo.
- **SQLite con una sola conexión.** Ya no está en el camino caliente del
  streaming: sesión, permisos y catálogo se sirven desde memoria
  (ver `RENDIMIENTO-STREAMING-MULTIUSUARIO.md`). `/api/admin/cache/metrics`
  permite confirmar durante la prueba que sigue siendo así.

## 5. Durante la medición, mirar también

Con la carga en marcha, `GET /api/admin/cache/metrics` (sesión de admin) debe
mostrar, una vez caliente:

- `session.misses` casi plano — si sube con cada petición, la caché de sesión no
  está haciendo su trabajo.
- `access.builds` subiendo como mucho cada 15 segundos, no por petición.
- `catalog.builds` estable — si crece, algo está invalidando el catálogo en bucle.

Si esos tres están bien y aun así hay cortes, el cuello **no es el servidor**:
mirar disco y red antes de tocar una línea de código.
