#!/bin/sh
# make-server-linux.sh — genera los .deb del SERVIDOR Noumon desde Linux.
# Equivalente nativo de make-linux-package.ps1 (que es para Windows).
#
# Los binarios Go se cross-compilan con CGO_ENABLED=0 (sin dependencia de
# libc), asi que desde esta maquina salen amd64 Y arm64 sin toolchain extra.
# El .deb lo arma scripts/mkdeb (Go puro), sin dpkg-deb.
#
# Requisito previo: las interfaces web compiladas. Desde la raiz del repo:
#   (cd noumon && corepack npm ci && corepack npm run build)
#   (cd library-server/panel && corepack npm ci && corepack npm run build)
# (o npm a secas si esta instalado; el panel deja su build en
# library-server/core/www-panel y el cliente en noumon/dist).
#
# Uso:
#   sh scripts/make-server-linux.sh                # amd64 + arm64, version fecha.hora
#   sh scripts/make-server-linux.sh arm64          # una sola arquitectura
#   sh scripts/make-server-linux.sh 1.2.0 arm64    # version explicita
#
# El paquete instala ademas una entrada de menu "Noumon Panel de Control"
# (con el icono del engranaje) que abre http://localhost:8090/panel/ en el
# navegador — util cuando el servidor corre en una maquina con escritorio.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DIST="$ROOT/library-desktop/dist"
LINUX_ASSETS="$ROOT/scripts/linux"
PMTILES_VERSION=v1.30.2

VERSION=''
ARCHES=''
for arg in "$@"; do
  case "$arg" in
    amd64|arm64) ARCHES="$ARCHES $arg" ;;
    all) ARCHES='amd64 arm64' ;;
    *) VERSION="$arg" ;;
  esac
done
[ -n "$VERSION" ] || VERSION=$(date +%Y.%m.%d.%H%M)
[ -n "$ARCHES" ] || ARCHES='amd64 arm64'

WWW_CLIENT="$ROOT/noumon/dist"
WWW_PANEL="$ROOT/library-server/core/www-panel"
MAPS_WWW="$ROOT/library-server/core/maps-www"
for dir in "$WWW_CLIENT" "$WWW_PANEL" "$MAPS_WWW"; do
  if [ ! -f "$dir/index.html" ]; then
    echo "Falta $dir/index.html — compila las interfaces web primero (ver cabecera)." >&2
    exit 1
  fi
done

mkdir -p "$DIST"

for a in $ARCHES; do
  echo "== noumon server linux/$a v$VERSION =="
  STAGING=$(mktemp -d)
  CONTROL=$(mktemp -d)
  BIN="$STAGING/opt/noumon/bin"
  mkdir -p "$BIN"

  # 1. Binarios Go cross-compilados (CGO_ENABLED=0).
  for mod in core:core supervisor:library-supervisor translate-wrap:translate-wrap; do
    src=${mod%%:*}; out=${mod##*:}
    (cd "$ROOT/library-server/$src" && \
      GOOS=linux GOARCH=$a CGO_ENABLED=0 go build -o "$BIN/$out" .)
  done

  # pmtiles oficial: go install lo deja en GOPATH/bin (nativo) o bin/linux_<arch> (cross).
  GOOS=linux GOARCH=$a CGO_ENABLED=0 go install github.com/protomaps/go-pmtiles@$PMTILES_VERSION
  GOPATH=$(go env GOPATH)
  if [ -x "$GOPATH/bin/linux_$a/go-pmtiles" ]; then
    cp "$GOPATH/bin/linux_$a/go-pmtiles" "$BIN/pmtiles"
  else
    cp "$GOPATH/bin/go-pmtiles" "$BIN/pmtiles"
  fi
  chmod 755 "$BIN/pmtiles"

  # 2. Interfaces web: exactamente las mismas que sirve el todo-en-uno.
  cp -R "$WWW_CLIENT" "$BIN/www-client"
  cp -R "$WWW_PANEL" "$BIN/www-panel"
  cp -R "$MAPS_WWW" "$BIN/maps-www"

  # 3. Unidad systemd (dpkg exige LF; tr protege frente a checkouts CRLF).
  mkdir -p "$STAGING/lib/systemd/system"
  tr -d '\r' < "$LINUX_ASSETS/noumon.service" > "$STAGING/lib/systemd/system/noumon.service"

  # 4. Acceso de menu al Panel de Control con su icono.
  mkdir -p "$STAGING/usr/share/applications" "$STAGING/usr/share/pixmaps"
  cp "$LINUX_ASSETS/noumon-panel.desktop" "$STAGING/usr/share/applications/noumon-panel.desktop"
  cp "$ROOT/icons/noumon-control-panel.png" "$STAGING/usr/share/pixmaps/noumon-panel.png"

  # 5. control + scripts de mantenimiento.
  SIZE_KB=$(du -sk "$STAGING" | cut -f1)
  cat > "$CONTROL/control" <<EOF
Package: noumon
Version: $VERSION
Architecture: $a
Maintainer: Noumon <andresgv7455@gmail.com>
Section: web
Priority: optional
Installed-Size: $SIZE_KB
Description: Biblioteca offline Noumon (servidor y panel)
 Servidor de biblioteca offline: colecciones ZIM, mapas y traduccion local.
 Interfaz en http://localhost:8090 y panel en http://localhost:8090/panel/.
EOF
  for script in postinst prerm postrm; do
    tr -d '\r' < "$LINUX_ASSETS/$script" > "$CONTROL/$script"
  done

  # 6. Armar el .deb.
  DEB="$DIST/noumon_${VERSION}_$a.deb"
  (cd "$ROOT/scripts/mkdeb" && go run . -staging "$STAGING" -control "$CONTROL" -out "$DEB")
  cp "$DEB" "$DIST/noumon_$a.deb"
  (cd "$DIST" && sha256sum "noumon_${VERSION}_$a.deb" > "noumon_${VERSION}_$a.deb.sha256")
  echo "OK -> $DEB"

  rm -rf "$STAGING" "$CONTROL"
done
