// Formateo compartido del Panel.

import { i18n } from './i18n.svelte.js'

export function bytes(n) {
  if (!n || n < 0) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0, v = n
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${u[i]}`
}

export function num(n) {
  return (n || 0).toLocaleString(i18n.locale === 'en' ? 'en-GB' : 'es-ES')
}

// Metadatos de presentación por sección del pool (icono/color/etiqueta).
// `labelKey` se resuelve con t() en la vista para que reaccione al idioma.
export const SECTION_META = {
  zim:       { labelKey: 'pool.zim', glyph: 'W', color: 'var(--info)' },
  models:    { labelKey: 'pool.models', glyph: '文', color: 'var(--magenta)' },
  downloads: { labelKey: 'pool.downloads', glyph: '▼', color: 'var(--orange)' },
  maps:      { labelKey: 'pool.maps', glyph: '◈', color: 'var(--signal)' },
  db:        { labelKey: 'pool.db', glyph: '≡', color: 'var(--ink-mute)' },
}
