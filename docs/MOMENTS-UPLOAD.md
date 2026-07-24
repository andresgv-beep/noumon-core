# Subidas nativas de Studio

## Por qué el transporte es directo

En el shell de escritorio, el webview no conserva de forma fiable el cuerpo de
un `POST multipart` cuando pasa por el proxy de recursos de la aplicación. Una
subida relativa podía llegar al Core con el fichero vacío aunque los metadatos
sí fueran correctos.

Por ello, los binarios de Studio se envían directamente a la URL real del Core:

1. el shell inyecta `window.__NOUMON_LIBRARY_CORE__`;
2. Studio solicita un token de subida corto, de un solo uso y ligado al
   documento;
3. el cliente envía el `multipart` directamente a la ruta de assets de ese
   documento, sin cookie;
4. el Core vuelve a comprobar identidad, capacidad, propiedad, cuota, tamaño y
   tipo real del fichero antes de promoverlo.

El navegador/PWA con mismo origen continúa usando la sesión normal.

## Regla para futuras subidas

Toda subida binaria nueva del shell debe reutilizar el transporte de Studio. No
debe crear otro endpoint administrativo ni depender del proxy del webview para
reenviar `FormData`.

Las antiguas rutas `/api/admin/upload` y `/api/admin/media/update` y el
formulario creativo del Panel fueron retirados al alcanzar Studio la paridad de
Documentos, Cabinet y Moments.
