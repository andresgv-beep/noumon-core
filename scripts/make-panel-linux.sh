#!/bin/sh
# make-panel-linux.sh — genera el .deb de la VENTANA nativa del Panel de
# Control (noumon-panel_<version>_<arch>.deb), el equivalente Linux de
# library-control-panel.exe.
#
# Es el mismo shell Wails del cliente pero con interfaceMode=panel y el modo
# local por defecto: proxy al servicio noumon de systemd en localhost:8090.
# La ventana NUNCA arranca ni detiene el servicio; ese es trabajo de systemd.
#
# Como el cliente, usa cgo + GTK/WebKit: SOLO compila para la arquitectura de
# la maquina donde se ejecuta (para arm64, correr este script en la Pi).
#
# Requisitos (Debian/Ubuntu):
#   sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev
#
# La entrada de menu la pone el paquete del servidor (noumon-panel.desktop):
# detecta esta ventana al lanzarse y solo si falta abre el navegador. Por eso
# este paquete no instala .desktop propio.
#
# Uso:
#   sh scripts/make-panel-linux.sh              # version = yyyy.MM.dd.HHMM
#   sh scripts/make-panel-linux.sh 1.2.0        # version explicita
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DESKTOP="$ROOT/library-desktop"
DIST="$DESKTOP/dist"
VERSION=${1:-$(date +%Y.%m.%d.%H%M)}

case $(uname -m) in
  x86_64) ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  *) echo "Arquitectura no soportada: $(uname -m)" >&2; exit 1 ;;
esac

for pkg in gtk+-3.0 webkit2gtk-4.1; do
  if ! pkg-config --exists "$pkg" 2>/dev/null; then
    echo "Falta la libreria de desarrollo '$pkg'." >&2
    echo "Instala: sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev" >&2
    exit 1
  fi
done

echo "== noumon-panel linux/$ARCH v$VERSION =="

STAGING=$(mktemp -d)
CONTROL=$(mktemp -d)
trap 'rm -rf "$STAGING" "$CONTROL"' EXIT

mkdir -p "$STAGING/usr/bin"
(
  cd "$DESKTOP"
  go build -tags 'desktop production webkit2_41' \
    -ldflags '-X main.interfaceMode=panel' \
    -o "$STAGING/usr/bin/noumon-panel" .
)

SIZE_KB=$(du -sk "$STAGING" | cut -f1)
cat > "$CONTROL/control" <<EOF
Package: noumon-panel
Version: $VERSION
Architecture: $ARCH
Maintainer: Noumon <andresgv7455@gmail.com>
Section: web
Priority: optional
Installed-Size: $SIZE_KB
Depends: libgtk-3-0 | libgtk-3-0t64, libwebkit2gtk-4.1-0 | libwebkit2gtk-4.1-0t64
Recommends: noumon
Description: Ventana nativa del Panel de Control de Noumon
 Ventana de escritorio para administrar el servidor Noumon local
 (servicio systemd "noumon" en localhost:8090). La entrada de menu
 "Noumon Panel de Control" del paquete del servidor la usa
 automaticamente cuando esta instalada; sin ella abre el navegador.
EOF

mkdir -p "$DIST"
DEB="$DIST/noumon-panel_${VERSION}_$ARCH.deb"
(cd "$ROOT/scripts/mkdeb" && go run . -staging "$STAGING" -control "$CONTROL" -out "$DEB")

cp "$DEB" "$DIST/noumon-panel_$ARCH.deb"
(cd "$DIST" && sha256sum "noumon-panel_${VERSION}_$ARCH.deb" > "noumon-panel_${VERSION}_$ARCH.deb.sha256")
echo "OK -> $DEB"
