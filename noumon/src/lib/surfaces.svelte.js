// surfaces.svelte.js — qué superficies (Documentos, Cabinet, Moments) tienen algo
// que el usuario actual pueda ver. El lateral y "Tus sitios" esconden las demás:
// un anónimo no debe encontrarse una entrada que abre una página vacía porque
// todo su contenido es "solo cuentas" o está bloqueado. Decide el servidor; aquí
// solo se guarda su respuesta.
import { serverFetch } from './connection.js';

const KEYS = ['documents', 'cabinet', 'moments'];

// Empiezan ocultas: aparecer y desaparecer al arrancar es peor que tardar un
// instante en aparecer.
export const surfaces = $state({ documents: false, cabinet: false, moments: false });

export async function refreshSurfaces() {
  try {
    const r = await serverFetch('/api/surfaces');
    // Un servidor anterior no conoce la ruta: se muestran todas, como antes.
    if (r.status === 404) {
      for (const key of KEYS) surfaces[key] = true;
      return;
    }
    if (!r.ok) return;
    const data = await r.json();
    for (const key of KEYS) surfaces[key] = data?.[key] === true;
  } catch (e) {
    // Sin conexión se conserva lo último que se supo.
  }
}
