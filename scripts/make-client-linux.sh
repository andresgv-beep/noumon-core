#!/bin/sh
# make-client-linux.sh — genera el .deb del cliente de escritorio Linux
# (noumon-client_<version>_<arch>.deb). Fase "cliente de escritorio Linux"
# de DISTRIBUCION.md.
#
# A diferencia del servidor (make-linux-package.ps1, CGO_ENABLED=0 y
# cross-compilable), la ventana nativa usa Wails + GTK/WebKit: necesita cgo,
# asi que SOLO compila para la arquitectura de la maquina donde se ejecuta.
# Para arm64 hay que correr este script en una maquina arm64 (p. ej. la Pi).
#
# Requisitos (Debian/Ubuntu):
#   sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev
# y Go >= 1.26.
#
# Uso:
#   sh scripts/make-client-linux.sh              # version = yyyy.MM.dd.HHMM
#   sh scripts/make-client-linux.sh 1.2.0        # version explicita
#
# La version por defecto incluye la hora: dos builds del mismo dia son
# versiones distintas y apt no descarta la reinstalacion por "misma version".
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DESKTOP="$ROOT/library-desktop"
DIST="$DESKTOP/dist"
LINUX_ASSETS="$ROOT/scripts/linux"
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

echo "== noumon-client linux/$ARCH v$VERSION =="

STAGING=$(mktemp -d)
CONTROL=$(mktemp -d)
trap 'rm -rf "$STAGING" "$CONTROL"' EXIT

# 1. Compilar la ventana nativa en modo gateway remoto. Al primer arranque
#    pide la URL del servidor y la guarda en ~/.config/Noumon/gateway.json.
mkdir -p "$STAGING/usr/bin"
(
  cd "$DESKTOP"
  go build -tags 'desktop production webkit2_41' \
    -ldflags '-X main.distributionMode=remote' \
    -o "$STAGING/usr/bin/noumon-client" .
)

# 2. Entrada de menu e icono (pixmaps admite cualquier tamano de PNG).
mkdir -p "$STAGING/usr/share/applications" "$STAGING/usr/share/pixmaps"
cp "$LINUX_ASSETS/noumon-client.desktop" "$STAGING/usr/share/applications/noumon-client.desktop"
cp "$ROOT/icons/noumon_icon_client.png" "$STAGING/usr/share/pixmaps/noumon-client.png"

# 3. control: dependencias de runtime de GTK/WebKit (nombres clasicos y t64).
SIZE_KB=$(du -sk "$STAGING" | cut -f1)
cat > "$CONTROL/control" <<EOF
Package: noumon-client
Version: $VERSION
Architecture: $ARCH
Maintainer: Noumon <andresgv7455@gmail.com>
Section: web
Priority: optional
Installed-Size: $SIZE_KB
Depends: libgtk-3-0 | libgtk-3-0t64, libwebkit2gtk-4.1-0 | libwebkit2gtk-4.1-0t64
Description: Cliente de escritorio Noumon (gateway remoto)
 Ventana nativa que se conecta a un Library Server de la red.
 Al primer arranque pide la direccion del servidor y la guarda en
 ~/.config/Noumon/gateway.json. No incluye servidor: para servir
 contenido instala el paquete "noumon".
EOF

# 4. Armar el .deb con el mismo mkdeb del servidor (Go puro).
mkdir -p "$DIST"
DEB="$DIST/noumon-client_${VERSION}_$ARCH.deb"
(cd "$ROOT/scripts/mkdeb" && go run . -staging "$STAGING" -control "$CONTROL" -out "$DEB")

cp "$DEB" "$DIST/noumon-client_$ARCH.deb"
(cd "$DIST" && sha256sum "noumon-client_${VERSION}_$ARCH.deb" > "noumon-client_${VERSION}_$ARCH.deb.sha256")
echo "OK -> $DEB"
