// Tema claro/oscuro del Panel. Mismo espíritu que i18n.svelte.js: estado de
// módulo con runes, persistencia en localStorage, cero dependencias. El tema
// se aplica como atributo data-theme en <html>; app.css define los tokens de
// cada uno. Por defecto: preferencia del sistema, oscuro si no se sabe.

const STORE_KEY = 'noumon-panel-theme'

function detectInitial() {
  try {
    const saved = localStorage.getItem(STORE_KEY)
    if (saved === 'light' || saved === 'dark') return saved
  } catch (e) {}
  try {
    if (window.matchMedia('(prefers-color-scheme: light)').matches) return 'light'
  } catch (e) {}
  return 'dark'
}

function apply(mode) {
  try { document.documentElement.dataset.theme = mode } catch (e) {}
}

export const theme = $state({ mode: detectInitial() })
apply(theme.mode)

export function setTheme(mode) {
  if (mode !== 'light' && mode !== 'dark') return
  theme.mode = mode
  apply(mode)
  try { localStorage.setItem(STORE_KEY, mode) } catch (e) {}
}

export function toggleTheme() {
  setTheme(theme.mode === 'dark' ? 'light' : 'dark')
}
