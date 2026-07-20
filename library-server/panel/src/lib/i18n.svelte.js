// i18n del Panel de Control — mismo patrón ligero que el cliente Noumon
// (noumon/src/lib/i18n.svelte.js): sin dependencias, estado de módulo con runes,
// diccionario plano. Añadir un idioma = bloque nuevo en messages.js + LANGS.

import { messages } from './messages.js'

export const LANGS = [
  { code: 'es', label: 'Español', flag: '🇪🇸' },
  { code: 'en', label: 'English', flag: '🇬🇧' },
]

const STORE_KEY = 'noumon-panel-lang'

function detectInitial() {
  try {
    const saved = localStorage.getItem(STORE_KEY)
    if (saved && messages[saved]) return saved
  } catch (e) {}
  try {
    const b = (navigator.language || 'es').slice(0, 2).toLowerCase()
    if (messages[b]) return b
  } catch (e) {}
  return 'es'
}

// Estado reactivo compartido. Leer i18n.locale en un template lo suscribe al cambio.
export const i18n = $state({ locale: detectInitial() })

export function setLocale(code) {
  if (!messages[code]) return
  i18n.locale = code
  try { localStorage.setItem(STORE_KEY, code) } catch (e) {}
}

// t(clave, params?) → cadena traducida. Fallback: idioma actual → español → la clave.
export function t(key, params) {
  const dict = messages[i18n.locale] || messages.es
  let s = dict[key]
  if (s == null) s = messages.es[key]
  if (s == null) return key
  if (params) for (const k in params) s = s.replaceAll('{' + k + '}', params[k])
  return s
}
