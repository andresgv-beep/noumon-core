# make-installer-windows.ps1 — genera el instalador NoumonSetup-<version>.exe
#
# Este script es el paso final de cada compilacion destinada a maquinas reales:
#   1. (salvo -SkipBuild) ejecuta build.ps1 -Mode all-in-one
#   2. regenera el icono del instalador desde icons\noumon_icon_client.png
#   3. compila installer\NoumonSetup.iss con Inno Setup (ISCC)
#   4. deja el resultado y su SHA256 en library-desktop\dist\
#
# El exe resultante instala Y actualiza sin conexion: en una maquina con Noumon
# ya instalado detiene el servicio, sustituye binarios y rearranca, conservando
# el pool de datos del usuario.
#
# Uso:
#   .\scripts\make-installer-windows.ps1                    # version = fecha (yyyy.MM.dd)
#   .\scripts\make-installer-windows.ps1 -Version 1.2.0
#   .\scripts\make-installer-windows.ps1 -SkipBuild         # ya compilaste all-in-one

param(
  [string]$Version,
  [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Desktop = Join-Path $Root 'library-desktop'
if (-not $Version) { $Version = Get-Date -Format 'yyyy.MM.dd' }

if (-not $SkipBuild) {
  & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $Desktop 'build.ps1') -Mode all-in-one
  if ($LASTEXITCODE -ne 0) { throw "build.ps1 fallo con codigo $LASTEXITCODE" }
}

foreach ($required in @(
    (Join-Path $Desktop 'noumon-all-in-one.exe'),
    (Join-Path $Desktop 'library-control-panel.exe'),
    (Join-Path $Desktop 'bin\library-supervisor.exe'))) {
  if (-not (Test-Path -LiteralPath $required)) { throw "Falta $required. Ejecuta build.ps1 -Mode all-in-one." }
}

Write-Host 'Icono del instalador...' -ForegroundColor Cyan
Push-Location $Desktop
try {
  $env:GOTELEMETRY = 'off'
  go run ./cmd/iconresource -icon (Join-Path $Root 'icons\noumon_icon_client.png') -ico (Join-Path $Desktop 'assets\noumon.ico')
  if ($LASTEXITCODE -ne 0) { throw 'No se pudo generar assets\noumon.ico' }
} finally { Pop-Location }

$isccCandidates = @(
  "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
  "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
  "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
)
$iscc = $isccCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $iscc) {
  try { $iscc = (Get-Command iscc -ErrorAction Stop).Source } catch {}
}
if (-not $iscc) {
  throw 'Inno Setup 6 no esta instalado. Instalalo con: winget install JRSoftware.InnoSetup'
}

Write-Host "Compilando instalador (version $Version)..." -ForegroundColor Cyan
& $iscc "/DAppVersion=$Version" (Join-Path $Desktop 'installer\NoumonSetup.iss')
if ($LASTEXITCODE -ne 0) { throw "ISCC fallo con codigo $LASTEXITCODE" }

$setup = Join-Path $Desktop "dist\NoumonSetup-$Version.exe"
if (-not (Test-Path -LiteralPath $setup)) { throw "No se genero $setup" }
$hash = (Get-FileHash -LiteralPath $setup -Algorithm SHA256).Hash
Set-Content -LiteralPath "$setup.sha256" -Value "$hash  NoumonSetup-$Version.exe" -Encoding ascii

Write-Host "OK -> $setup" -ForegroundColor Green
Write-Host "SHA256 = $hash" -ForegroundColor Green
