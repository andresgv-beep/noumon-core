# Memoria de distribución e instaladores

Esta guía complementa a `COMPILACION-NATIVA.md`. Aquella cubre el ciclo de
desarrollo en esta máquina; esta cubre cómo se genera **lo que se entrega a un
usuario normal**: un instalador que no exige clonar el repo ni tocar PowerShell.

## Regla principal

**Esto no es una fase final.** Cada compilación destinada a máquinas reales debe
regenerar su instalador. Un instalador viejo distribuye binarios viejos; el
repositorio actualizado no actualiza nada por sí solo. El ciclo completo es:

```text
código → build.ps1 all-in-one → make-installer-windows.ps1 → NoumonSetup-<version>.exe
```

El mismo `NoumonSetup-<version>.exe` sirve para **instalar de cero** y para
**actualizar sin conexión**: en una máquina que ya tiene Noumon detiene el
servicio, sustituye binarios, reinstala y arranca el servicio, y conserva
intactos el pool de ZIMs, los índices, los mapas descargados y ProgramData.
Basta llevarlo en un USB y ejecutarlo.

## Fase 1 — Windows (NoumonSetup.exe)

### Generar el instalador

Desde la raíz del repositorio:

```powershell
.\scripts\make-installer-windows.ps1                 # versión = fecha yyyy.MM.dd
.\scripts\make-installer-windows.ps1 -Version 1.2.0  # versión explícita
.\scripts\make-installer-windows.ps1 -SkipBuild      # si ya corriste build.ps1 all-in-one
```

El script compila todo (salvo `-SkipBuild`), regenera el icono, compila
`library-desktop\installer\NoumonSetup.iss` con Inno Setup 6 y deja en
`library-desktop\dist\`:

```text
NoumonSetup-<version>.exe
NoumonSetup-<version>.exe.sha256
```

Requisito único: **Inno Setup 6** (`winget install JRSoftware.InnoSetup`).

### Tipos de instalación

El mismo setup ofrece tres tipos (página "Tipo de instalación"; en silencioso,
`/TYPE=full|server|client`, por defecto `full`):

- **Completa**: cliente todo-en-uno + servicio + Panel. Una sola máquina.
- **Solo servidor**: servicio NoumonServer + Panel de Control, sin ventana de
  cliente. Para el equipo que sirve a la casa/aula.
- **Solo cliente**: únicamente la ventana Noumon en modo *gateway remoto*
  (`noumon-client.exe`, que `make-installer-windows.ps1` compila aparte). Al
  abrirla por primera vez pide la dirección del servidor y la guarda en
  `%AppData%\Noumon\gateway.json`; también respeta `NOUMON_LIBRARY_SERVER`.

Aviso: cambiar de tipo sobre una instalación existente no retira los
componentes del tipo anterior (Inno no borra lo ya copiado). Para pasar p. ej.
de Completa a Solo cliente: desinstalar primero y volver a instalar (los datos
de usuario se conservan siempre).

### Qué hace el instalador en la máquina destino

1. Pide elevación (UAC) y cierra las apps Noumon abiertas si hace falta.
2. Si la máquina no tiene WebView2 y el instalador incluye el bootstrapper
   (ver abajo), lo instala en silencio.
3. Si hay una versión previa: detiene `NoumonServer` y espera a que Windows
   libere los ejecutables (mismo baile que `install-all-in-one.ps1`).
4. Copia `noumon.exe`, `library-control-panel.exe` y `bin\` a
   `C:\Program Files\Noumon`.
5. `library-supervisor.exe install` + `start` (servicio `NoumonServer`,
   arranque automático).
6. Accesos directos en menú Inicio (y escritorio, opcional).
7. Registra el desinstalador en "Aplicaciones instaladas". Desinstalar retira
   servicio y programa pero **conserva los datos del usuario**.

### WebView2 sin conexión

Windows 11 y los Windows 10 actualizados ya traen WebView2. Para máquinas sin
él y sin internet, descargar una vez el *Evergreen Bootstrapper* oficial de
Microsoft y dejarlo en:

```text
library-desktop\redist\MicrosoftEdgeWebView2Setup.exe
```

Si el archivo existe, el .iss lo empaqueta y solo lo ejecuta cuando falta
WebView2. Si no existe, el instalador se compila igual (sin ese refuerzo).
Nota: el *bootstrapper* necesita internet para descargar el runtime; para
equipos 100% aislados usar el *Evergreen Standalone Installer* (~180 MB) en la
misma ruta y nombre.

### Los iconos

- Fuente única: `icons\noumon_icon_client.png` (cliente) y
  `icons\noumon-control-panel.png` (engranaje del panel), 1254×1254.
- `build.ps1` los incrusta en cada exe vía `cmd\iconresource` (acepta PNG y
  genera las tallas 16–256). Los accesos directos heredan el icono del exe.
- `make-installer-windows.ps1` regenera además `assets\noumon.ico` para el
  propio setup. Para cambiar un icono: sustituir el PNG y recompilar; no hay
  nada más que tocar.

### Firma de código (pendiente, recomendado)

Sin certificado, SmartScreen mostrará "editor desconocido" en máquinas ajenas
(en actualizaciones offline de máquinas propias es solo un clic extra). Cuando
haya certificado, se añade el paso `signtool` al final de
`make-installer-windows.ps1`.

## Legalidad del paquete

Sigue aplicando el principio de siempre: **el instalador solo lleva código
propio y binarios permisivos** (Wails MIT, go-pmtiles BSD-3, modernc/sqlite,
deps Go). El contenido (ZIMs), los modelos de traducción (translateLocally),
los datos de mapas (PMTiles) y los iconos de catálogo los descarga el usuario
desde la fuente original vía el Panel. El gate de `THIRD-PARTY-NOTICES.txt`
del build sigue siendo bloqueante: con cola `REVIEW REQUIRED` no se publica.
El bootstrapper de WebView2 es redistribuible por licencia de Microsoft.

## Fase 2 — Linux x86-64 (.deb) y Fase 3 — ARM64/Raspberry Pi

Ambas salen del mismo molde. Desde la raíz del repositorio:

```powershell
.\scripts\make-linux-package.ps1                 # amd64 + arm64, versión = fecha
.\scripts\make-linux-package.ps1 -Arch amd64     # una sola arquitectura
.\scripts\make-linux-package.ps1 -Version 1.2.0
```

Requiere `build.ps1 -Mode all-in-one` previo (las interfaces web se toman de
`library-desktop\bin`). No necesita dpkg ni WSL: los binarios se cross-compilan
con `CGO_ENABLED=0` (sin dependencia de libc) y el .deb lo arma `scripts\mkdeb`
(Go puro). Resultado en `library-desktop\dist\`:

```text
noumon_<version>_amd64.deb (+ .sha256)   noumon_amd64.deb (nombre estable)
noumon_<version>_arm64.deb (+ .sha256)   noumon_arm64.deb (nombre estable)
```

### Qué instala el .deb

- Binarios en `/opt/noumon/bin`: `core`, `library-supervisor`, `translate-wrap`,
  `pmtiles`, más `www-client`, `www-panel` y `maps-www`.
- Unidad `noumon.service`: systemd arranca `library-supervisor run` como el
  usuario de sistema `noumon` (creado en postinst) con
  `NOUMON_LIBRARY_DATA=/var/lib/noumon`; el supervisor cría y reinicia core y
  translate-wrap, igual que en Windows.
- Actualizar = instalar el .deb nuevo encima (postinst hace `systemctl restart`).
  Desinstalar (incluso purge) **conserva `/var/lib/noumon`**.
- En Linux no hay ventana nativa (por ahora): la interfaz es el navegador —
  `http://localhost:8090` y panel en `http://localhost:8090/panel/`. En la Pi
  ese es además el modo natural: la Pi sirve y el resto de dispositivos entran
  por el navegador.

### install.sh

`scripts\install.sh` es el camino corto para el usuario Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/andresgv-beep/noumon-core/main/scripts/install.sh | sudo sh
# o sin red, con el .deb en un USB:
sudo sh install.sh ./noumon_amd64.deb
```

Detecta la arquitectura (x86-64 → amd64, aarch64 → arm64), descarga
`noumon_<arch>.deb` de la **última release de GitHub** del repo
`andresgv-beep/noumon-core` y lo instala. Para que funcione online hay que
subir a cada release los .deb con sus nombres estables (`noumon_amd64.deb`,
`noumon_arm64.deb`) además de los versionados.

### Verificación realizada (2026-07-19)

Smoke test en contenedores Docker `debian:bookworm` limpios: `dpkg -i` sin
errores, usuario y unidad creados, `library-supervisor run` levanta core y
`/api/health` responde `200` con motor nativo — en **amd64** y en **arm64**
(emulado con QEMU). En una instalación virgen `engine:"down"` con 0
colecciones es lo esperado; se levanta al registrar el primer ZIM.

## Pendientes

- Firma de código Windows (certificado) y, si algún día se quiere, ventana
  nativa Linux (Wails/webkit2gtk) y repos apt propios.
- Crear las GitHub Releases y subir los artefactos (`NoumonSetup-*.exe`,
  `noumon_*.deb` + sha256) para activar el camino online de `install.sh`.

Al cerrar cada fase, actualizar este documento en el mismo commit.
