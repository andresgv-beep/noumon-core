# Subidas en la app nativa: por qué llegaban vacías y cómo se arregló

## El síntoma

En la app de escritorio (Moments/Panel), al subir un vídeo o cambiar el logo de
un canal:

- El **vídeo** se quedaba en 0:00 con el spinner girando (no reproducía).
- El **avatar/logo** daba `avatar inválido: no se pudo leer la imagen`.

En el navegador / PWA / acceso remoto por LAN, **todo funcionaba bien**.

## La causa raíz (diagnóstico 2026-07-19)

Los ficheros se guardaban en el pool con **0 bytes**. El sidecar `.json`
(metadatos) salía correcto; solo el binario (vídeo/imagen) llegaba vacío.

Cadena de evidencia:

1. `downloads/Moments/General/<vídeo>.mp4` = **0 bytes** en disco. Al pedir un
   rango, el Core respondía `416` con `content-range: bytes 0-0/0` (= "0 bytes").
2. Subir el MISMO fichero **directo al Core** (`:8090`, saltando la app):
   **funciona** — probado 221 B, 40 MB y con avatar PNG, todos guardados bien.
   ⇒ el Core y el frontend están sanos.
3. La diferencia: la app de escritorio es **Wails v2 (WebView2)**. Todas las
   peticiones del webview pasan por su `AssetServer` → reverse-proxy → Core.

**El problema:** **WebView2 no entrega el cuerpo del POST multipart al
`AssetServer`** de Wails (`GetContent()` llega vacío para `fetch` con
`FormData`). El Core recibe una subida sin body → guarda 0 bytes. Los POST con
body JSON pequeño (login, etc.) sí llegan; el multipart de subida, no.

Es una **limitación de plataforma** del webview nativo, no un bug del Core ni
del frontend. Afecta por igual a las apps nativas (Windows/WebView2 confirmado;
macOS/WKWebView y Linux/WebKitGTK comparten el patrón, mismo riesgo). El acceso
por navegador/PWA NO está afectado (va por HTTP real, con body).

## El arreglo: subir DIRECTO al Core

Una petición del webview a una **URL absoluta del Core** (`http://127.0.0.1:8090`)
es **red real** — NO la intercepta el `AssetServer`, así que **sí lleva el body**.
Por eso las subidas ahora esquivan el proxy de Wails y van directas.

Tres piezas (todas con comentario que apunta a este doc):

1. **Shell** (`library-desktop/gateway.go`): inyecta
   `window.__NOUMON_LIBRARY_CORE__` con la URL real del Core (además del
   `__NOUMON_LIBRARY_SERVER__=""` que mantiene el resto relativo).
2. **Frontend** (`library-server/panel/src/lib/api.js`, `uploadContent`): en modo
   shell, pide un **media-token de un solo uso** por el canal normal (cookie, vía
   proxy) y hace el POST del `FormData` **directo** a
   `${core}/api/admin/upload?st=<token>`. La cookie no cruza orígenes; el token sí
   autentica (`currentUser` lo resuelve al mismo usuario admin).
3. **Core** (`library-server/core/main.go`): la subida directa es cross-origin, así
   que:
   - **CSRF**: se exime del check de Origin a las peticiones autenticadas SOLO por
     token (`requestTokenOnly`: Bearer o `?st=`, y SIN cookie de sesión). Un token
     no es una credencial ambiente: una web hostil no lo conoce, luego no hay CSRF.
     Se exige la ausencia de cookie a propósito, para que un `?st=` basura no
     desactive el check cuando sí hay cookie.
   - **CORS**: se refleja el `Origin` (sin `Allow-Credentials`) para esas mismas
     peticiones, así el JS puede leer la respuesta `{ok}`.

`multipart/form-data` es un content-type "simple" de CORS → la subida directa NO
dispara preflight.

## Regla para el futuro

**Cualquier POST con body (multipart) desde el webview nativo debe ir directo al
Core, no por el proxy de Wails.** Si añades una subida nueva, reutiliza
`uploadContent`/`updateContent` (ya lo hacen). No confíes en que el `AssetServer`
reenvíe el body: no lo hace para `fetch` con FormData.
