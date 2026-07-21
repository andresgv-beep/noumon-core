#!/bin/sh
# install.sh — instalador/actualizador de Noumon para Linux (x86-64 y ARM64).
#
#   curl -fsSL https://raw.githubusercontent.com/andresgv-beep/noumon-core/main/scripts/install.sh | sudo sh
#
# Tambien funciona sin red con un .deb local:
#   sudo sh install.sh ./noumon_amd64.deb
#
# Descarga el .deb de la ultima release de GitHub y lo instala con dpkg/apt.
# Actualizar es volver a ejecutarlo: el paquete para el servicio, sustituye
# binarios y rearranca conservando los datos (/var/lib/noumon).

set -e

REPO="andresgv-beep/noumon-core"

if [ "$(id -u)" -ne 0 ]; then
  echo "Ejecuta este instalador como root (sudo)." >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "Arquitectura no soportada: $(uname -m)" >&2; exit 1 ;;
esac

if ! command -v dpkg >/dev/null 2>&1; then
  echo "Este instalador necesita dpkg (Debian, Ubuntu, Mint, Raspberry Pi OS...)." >&2
  exit 1
fi

DEB="$1"
CLEANUP=""
if [ -z "$DEB" ]; then
  URL="https://github.com/$REPO/releases/latest/download/noumon_$ARCH.deb"
  DEB="$(mktemp /tmp/noumon_XXXXXX.deb)"
  CLEANUP="$DEB"
  echo "Descargando $URL ..."
  if command -v curl >/dev/null 2>&1; then
    curl -fL -o "$DEB" "$URL"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$DEB" "$URL"
  else
    echo "Se necesita curl o wget." >&2
    exit 1
  fi
fi

echo "Instalando $DEB ..."
if command -v apt-get >/dev/null 2>&1; then
  apt-get install -y "$DEB" || dpkg -i "$DEB"
else
  dpkg -i "$DEB"
fi

[ -n "$CLEANUP" ] && rm -f "$CLEANUP"

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "$IP" ] || IP="localhost"
echo ""
echo "Noumon listo y publicado en tu red local."
echo "  Biblioteca:  http://$IP:8090"
echo "  Panel admin: http://$IP:8090/panel/"
echo "Abre el Panel desde cualquier equipo de la red: el PRIMER registro se"
echo "convierte en administrador — hazlo tu ahora mismo."
echo "Servicio: systemctl status noumon"
