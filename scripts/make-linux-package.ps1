# make-linux-package.ps1 — genera los .deb de Linux (amd64 y arm64) desde Windows.
#
# Fase 2/3 de DISTRIBUCION.md. No necesita dpkg ni WSL: los binarios Go se
# cross-compilan (CGO_ENABLED=0, sin dependencias de libc) y el .deb lo arma
# scripts\mkdeb (ar + tar.gz en Go puro).
#
# Requisito previo: build.ps1 -Mode all-in-one ya ejecutado, porque las
# interfaces web (www-client, www-panel, maps-www) se toman de
# library-desktop\bin. El servidor Linux sirve exactamente las mismas.
#
# Uso:
#   .\scripts\make-linux-package.ps1                    # version = fecha yyyy.MM.dd
#   .\scripts\make-linux-package.ps1 -Version 1.2.0
#   .\scripts\make-linux-package.ps1 -Arch amd64        # solo una arquitectura

param(
  [string]$Version,
  [ValidateSet('amd64', 'arm64', 'all')]
  [string]$Arch = 'all'
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Desktop = Join-Path $Root 'library-desktop'
$DesktopBin = Join-Path $Desktop 'bin'
$Dist = Join-Path $Desktop 'dist'
$LinuxAssets = Join-Path $Root 'scripts\linux'
if (-not $Version) { $Version = Get-Date -Format 'yyyy.MM.dd' }

foreach ($required in @('www-client', 'www-panel', 'maps-www')) {
  if (-not (Test-Path -LiteralPath (Join-Path $DesktopBin $required))) {
    throw "Falta library-desktop\bin\$required. Ejecuta build.ps1 -Mode all-in-one primero."
  }
}

$env:GOTELEMETRY = 'off'
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP 'noumon-go-build-cache' }
New-Item -ItemType Directory -Force -Path $Dist | Out-Null

# dpkg exige LF; el repo puede tener CRLF segun la configuracion de git.
function Copy-AsLF([string]$Source, [string]$Destination) {
  $text = [IO.File]::ReadAllText($Source) -replace "`r`n", "`n"
  [IO.File]::WriteAllText($Destination, $text)
}

$arches = if ($Arch -eq 'all') { @('amd64', 'arm64') } else { @($Arch) }
foreach ($a in $arches) {
  Write-Host "== linux/$a ==" -ForegroundColor Cyan
  $staging = Join-Path $env:TEMP "noumon-deb-$a"
  $control = Join-Path $env:TEMP "noumon-deb-$a-control"
  foreach ($d in @($staging, $control)) {
    if (Test-Path -LiteralPath $d) { Remove-Item -LiteralPath $d -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $d | Out-Null
  }
  $bin = Join-Path $staging 'opt\noumon\bin'
  New-Item -ItemType Directory -Force -Path $bin | Out-Null

  $env:GOOS = 'linux'; $env:GOARCH = $a; $env:CGO_ENABLED = '0'
  try {
    Push-Location (Join-Path $Root 'library-server\core')
    try { go build -o (Join-Path $bin 'core') . } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw "core linux/$a fallo" }
    Push-Location (Join-Path $Root 'library-server\supervisor')
    try { go build -o (Join-Path $bin 'library-supervisor') . } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw "supervisor linux/$a fallo" }
    Push-Location (Join-Path $Root 'library-server\translate-wrap')
    try { go build -o (Join-Path $bin 'translate-wrap') . } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw "translate-wrap linux/$a fallo" }

    # pmtiles oficial cross-compilado; go install lo deja en GOPATH\bin\linux_<arch>
    go install github.com/protomaps/go-pmtiles@v1.30.2
    if ($LASTEXITCODE -ne 0) { throw "go-pmtiles linux/$a fallo" }
    $gopath = (go env GOPATH).Trim()
    $pmtiles = Join-Path $gopath "bin\linux_$a\go-pmtiles"
    if (-not (Test-Path -LiteralPath $pmtiles)) { throw "No aparecio $pmtiles" }
    Copy-Item -LiteralPath $pmtiles -Destination (Join-Path $bin 'pmtiles') -Force
  } finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
  }

  foreach ($www in @('www-client', 'www-panel', 'maps-www')) {
    Copy-Item -Recurse -LiteralPath (Join-Path $DesktopBin $www) -Destination (Join-Path $bin $www)
  }

  $unitDir = Join-Path $staging 'lib\systemd\system'
  New-Item -ItemType Directory -Force -Path $unitDir | Out-Null
  Copy-AsLF (Join-Path $LinuxAssets 'noumon.service') (Join-Path $unitDir 'noumon.service')

  $sizeKB = [int](((Get-ChildItem -LiteralPath $staging -Recurse -File | Measure-Object Length -Sum).Sum) / 1024)
  $controlText = @(
    'Package: noumon',
    "Version: $Version",
    "Architecture: $a",
    'Maintainer: Noumon <andresgv7455@gmail.com>',
    'Section: web',
    'Priority: optional',
    "Installed-Size: $sizeKB",
    'Description: Biblioteca offline Noumon (servidor y panel)',
    ' Servidor de biblioteca offline: colecciones ZIM, mapas y traduccion local.',
    ' Interfaz en http://localhost:8090 y panel en http://localhost:8090/panel/.',
    ''
  ) -join "`n"
  [IO.File]::WriteAllText((Join-Path $control 'control'), $controlText)
  foreach ($script in @('postinst', 'prerm', 'postrm')) {
    Copy-AsLF (Join-Path $LinuxAssets $script) (Join-Path $control $script)
  }

  $deb = Join-Path $Dist "noumon_${Version}_$a.deb"
  Push-Location (Join-Path $Root 'scripts\mkdeb')
  try { go run . -staging $staging -control $control -out $deb } finally { Pop-Location }
  if ($LASTEXITCODE -ne 0) { throw "mkdeb linux/$a fallo" }
  # Copia con nombre estable para releases/latest/download
  Copy-Item -LiteralPath $deb -Destination (Join-Path $Dist "noumon_$a.deb") -Force
  $hash = (Get-FileHash -LiteralPath $deb -Algorithm SHA256).Hash
  Set-Content -LiteralPath "$deb.sha256" -Value "$hash  noumon_${Version}_$a.deb" -Encoding ascii
  Write-Host "OK -> $deb" -ForegroundColor Green

  Remove-Item -LiteralPath $staging, $control -Recurse -Force
}
