// Tema del shell: DOS ejes independientes (contrato noumon-temas) + acento.
// - luz  (claro/oscuro/sistema)  → data-theme  → paleta de color.
// - piel (moderna/retro)         → data-skin   → forma: radios, tipografía, sombras.
// - acento (un color o vacío)    → --accent inline → derivados via color-mix.
// SOLO afecta a la interfaz: el contenido de los ZIM va en su iframe.
// El anti-parpadeo inicial lo hace un script en index.html (aplica los tres).

const STORE_KEY = 'noumon-theme';
const SKIN_KEY = 'noumon-skin';
const ACCENT_KEY = 'noumon-accent';

export const THEMES = [
  { code: 'system', labelKey: 'settings.themeSystem', icon: 'contrast' },
  { code: 'light', labelKey: 'settings.themeLight', icon: 'sun' },
  { code: 'dark', labelKey: 'settings.themeDark', icon: 'moon' },
];

export const SKINS = [
  { code: 'modern', labelKey: 'settings.skinModern', icon: 'panel' },
  { code: 'retro', labelKey: 'settings.skinRetro', icon: 'book' },
];

// Acentos de serie; el usuario también puede elegir uno libre.
export const ACCENTS = [
  { hex: '#7c6cf0', labelKey: 'settings.accentLila' },
  { hex: '#e8a33d', labelKey: 'settings.accentAmbar' },
  { hex: '#4fd39a', labelKey: 'settings.accentFosforo' },
  { hex: '#4aa8e0', labelKey: 'settings.accentCian' },
];

function saved() {
  try { const v = localStorage.getItem(STORE_KEY); if (v) return v; } catch (e) {}
  return 'dark';
}
function savedSkin() {
  try { const v = localStorage.getItem(SKIN_KEY); if (v === 'retro') return v; } catch (e) {}
  return 'modern';
}
function savedAccent() {
  try {
    const v = localStorage.getItem(ACCENT_KEY);
    if (v && /^#[0-9a-f]{6}$/i.test(v)) return v;
  } catch (e) {}
  return ''; // vacío = acento por defecto de la piel/luz activas
}
function systemDark() {
  try { return matchMedia('(prefers-color-scheme: dark)').matches; } catch (e) { return true; }
}
const resolve = (choice) => (choice === 'system' ? (systemDark() ? 'dark' : 'light') : choice);

// Estado reactivo compartido (runes). `choice` = lo elegido; `resolved` = lo aplicado.
export const theme = $state({
  choice: saved(),
  resolved: resolve(saved()),
  skin: savedSkin(),
  accent: savedAccent(),
});

function apply() {
  theme.resolved = resolve(theme.choice);
  try {
    const root = document.documentElement;
    root.setAttribute('data-theme', theme.resolved);
    root.setAttribute('data-skin', theme.skin);
    if (theme.accent) root.style.setProperty('--accent', theme.accent);
    else root.style.removeProperty('--accent');
  } catch (e) {}
}

export function setTheme(code) {
  theme.choice = code;
  try { localStorage.setItem(STORE_KEY, code); } catch (e) {}
  apply();
}

export function setSkin(code) {
  if (code !== 'modern' && code !== 'retro') return;
  theme.skin = code;
  try { localStorage.setItem(SKIN_KEY, code); } catch (e) {}
  apply();
}

// Un acento casi blanco o casi negro no deja derivado legible (enlaces y marca
// se funden con el fondo): esos extremos se ignoran y se mantiene el anterior.
function accentUsable(hex) {
  const n = parseInt(hex.slice(1), 16);
  const lum = (0.2126 * ((n >> 16) & 255) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255;
  return lum > 0.06 && lum < 0.92;
}

// hex '#rrggbb' o '' para volver al acento por defecto.
export function setAccent(hex) {
  const valid = /^#[0-9a-f]{6}$/i.test(hex || '');
  if (valid && !accentUsable(hex)) return;
  theme.accent = valid ? hex : '';
  try {
    if (theme.accent) localStorage.setItem(ACCENT_KEY, theme.accent);
    else localStorage.removeItem(ACCENT_KEY);
  } catch (e) {}
  apply();
}

// Init + seguir al sistema en vivo cuando la elección es "system".
apply();
try {
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (theme.choice === 'system') apply();
  });
} catch (e) {}
