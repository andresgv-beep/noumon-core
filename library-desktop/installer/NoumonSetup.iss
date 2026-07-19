; NoumonSetup.iss — instalador de Windows para usuarios finales.
;
; Replica la logica de library-desktop\install-all-in-one.ps1 (que sigue siendo
; la via de desarrollo) y anade lo que un usuario normal necesita: asistente
; grafico, desinstalador, entrada en "Aplicaciones instaladas" y actualizacion
; en sitio sin conexion (parar servicio -> sustituir binarios -> arrancar).
; Los datos del usuario (pool de ZIMs, indices, ProgramData) nunca se tocan.
;
; Se compila con scripts\make-installer-windows.ps1, que pasa /DAppVersion=...
; Requiere haber ejecutado antes build.ps1 -Mode all-in-one.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#define AppName "Noumon"
#define DesktopDir ".."
; Bootstrapper Evergreen de WebView2 (opcional). Si existe en library-desktop\redist,
; se incluye y se ejecuta solo cuando la maquina no tiene WebView2. Microsoft permite
; redistribuir este bootstrapper. Sin el archivo, el instalador se compila igual.
#define RedistWebView2 DesktopDir + "\redist\MicrosoftEdgeWebView2Setup.exe"

[Setup]
AppId={{8F4D2C71-95B3-4A6E-B0D9-3C61E7A5F214}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Noumon
DefaultDirName={autopf}\Noumon
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=yes
RestartApplications=no
SetupIconFile={#DesktopDir}\assets\noumon.ico
UninstallDisplayIcon={app}\noumon.exe
UninstallDisplayName={#AppName}
WizardStyle=modern
Compression=lzma2/max
SolidCompression=yes
OutputDir={#DesktopDir}\dist
OutputBaseFilename=NoumonSetup-{#AppVersion}
VersionInfoDescription=Instalador de Noumon
VersionInfoProductName={#AppName}

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

; Tres instalaciones desde el mismo setup:
;   Completa      -> cliente todo-en-uno + servicio + panel (una sola maquina)
;   Solo servidor -> servicio + panel, sin ventana de cliente (p. ej. el PC
;                    que sirve a la casa/aula; se administra con el panel)
;   Solo cliente  -> ventana Noumon en modo gateway remoto: al abrirla pide la
;                    direccion del servidor (o NOUMON_LIBRARY_SERVER)
[Types]
Name: "full"; Description: "Completa (cliente + servidor en esta maquina)"
Name: "server"; Description: "Solo servidor (servicio + Panel de Control)"
Name: "client"; Description: "Solo cliente (conectar a un servidor remoto)"
Name: "custom"; Description: "Personalizada"; Flags: iscustom

[Components]
Name: "client"; Description: "Cliente Noumon (ventana nativa)"; Types: full client
Name: "server"; Description: "Servidor NoumonServer (servicio + motor + contenidos)"; Types: full server
Name: "panel"; Description: "Library Control Panel (administracion)"; Types: full server

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Components: client

[Files]
; El exe del cliente depende de si hay servidor local: con servidor va el
; todo-en-uno; sin el, el cliente remoto (gateway) que pide la direccion.
Source: "{#DesktopDir}\noumon-all-in-one.exe"; DestDir: "{app}"; DestName: "noumon.exe"; Components: client and server; Flags: ignoreversion
Source: "{#DesktopDir}\noumon-client.exe"; DestDir: "{app}"; DestName: "noumon.exe"; Components: client and not server; Flags: ignoreversion
Source: "{#DesktopDir}\library-control-panel.exe"; DestDir: "{app}"; Components: panel; Flags: ignoreversion
Source: "{#DesktopDir}\bin\*"; DestDir: "{app}\bin"; Components: server; Flags: ignoreversion recursesubdirs createallsubdirs
#if FileExists(RedistWebView2)
Source: "{#RedistWebView2}"; DestDir: "{tmp}"; Flags: deleteafterinstall; Check: not WebView2Installed
#endif

[Icons]
Name: "{autoprograms}\Noumon\Noumon"; Filename: "{app}\noumon.exe"; WorkingDir: "{app}"; Components: client
Name: "{autoprograms}\Noumon\Library Control Panel"; Filename: "{app}\library-control-panel.exe"; WorkingDir: "{app}"; Components: panel
Name: "{autodesktop}\Noumon"; Filename: "{app}\noumon.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
#if FileExists(RedistWebView2)
Filename: "{tmp}\MicrosoftEdgeWebView2Setup.exe"; Parameters: "/silent /install"; StatusMsg: "Instalando Microsoft WebView2..."; Check: not WebView2Installed; Flags: waituntilterminated
#endif
Filename: "{app}\bin\library-supervisor.exe"; Parameters: "install"; StatusMsg: "Registrando el servicio NoumonServer..."; Components: server; Flags: runhidden waituntilterminated
Filename: "{app}\bin\library-supervisor.exe"; Parameters: "start"; StatusMsg: "Arrancando el servicio NoumonServer..."; Components: server; Flags: runhidden waituntilterminated
Filename: "{app}\noumon.exe"; Description: "{cm:LaunchProgram,Noumon}"; Components: client; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\bin\library-supervisor.exe"; Parameters: "uninstall"; RunOnceId: "NoumonSvcUninstall"; Check: SupervisorPresent; Flags: runhidden waituntilterminated

[Code]
function SupervisorPresent(): Boolean;
begin
  Result := FileExists(ExpandConstant('{app}\bin\library-supervisor.exe'));
end;

// WebView2 Evergreen instalado (por maquina o por usuario).
function WebView2Installed(): Boolean;
var
  Version: String;
begin
  Result :=
    RegQueryStringValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) or
    RegQueryStringValue(HKLM, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) or
    RegQueryStringValue(HKCU, 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version);
  Result := Result and (Version <> '') and (Version <> '0.0.0.0');
end;

function ServiceExists(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{cmd}'), '/c sc query NoumonServer >nul 2>&1', '',
    SW_HIDE, ewWaitUntilTerminated, ResultCode) and (ResultCode = 0);
end;

function ServiceStopped(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{cmd}'), '/c sc query NoumonServer | findstr STOPPED >nul', '',
    SW_HIDE, ewWaitUntilTerminated, ResultCode) and (ResultCode = 0);
end;

// Actualizacion en sitio: detener el servicio instalado y esperar a que Windows
// libere los ejecutables antes de sustituirlos (mismo baile que install-all-in-one.ps1).
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Supervisor: String;
  ResultCode, Attempt: Integer;
begin
  Result := '';
  if not ServiceExists() then
    exit;
  Supervisor := ExpandConstant('{app}\bin\library-supervisor.exe');
  if FileExists(Supervisor) then
    Exec(Supervisor, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
  else
    Exec(ExpandConstant('{cmd}'), '/c sc stop NoumonServer', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  for Attempt := 0 to 99 do
  begin
    if ServiceStopped() then
      exit;
    Sleep(300);
  end;
  Result := 'No se pudo detener el servicio NoumonServer para actualizar. Cierra Noumon y vuelve a intentarlo.';
end;
