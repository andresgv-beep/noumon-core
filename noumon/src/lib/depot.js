// depot.js — lógica compartida del apartado Depot (listado y ficha): sistemas,
// tamaños y qué archivo le toca al equipo que mira la página.

export const OS_NAME = { win: 'Windows', linux: 'Linux', mac: 'macOS' };

// Columnas de la matriz: tres sistemas y "arm", que es lo que de verdad
// distingue a una Raspberry Pi o a un portátil ARM.
export const MATRIX = [
  { key: 'win', short: 'win' },
  { key: 'linux', short: 'lnx' },
  { key: 'mac', short: 'mac' },
  { key: 'arm', short: 'arm' },
];

export const programOf = (item) => item?.program || { files: [], screenshots: [] };

export function covers(item, key) {
  return (programOf(item).files || []).some((file) => (key === 'arm' ? file.arch === 'arm64' : file.os === key));
}

export function totalSize(item) {
  return (programOf(item).files || []).reduce((sum, file) => sum + (file.size || 0), 0);
}

export function formatSize(bytes) {
  if (!bytes) return '—';
  const mb = bytes / (1024 * 1024);
  if (mb >= 1024) return `${(mb / 1024).toFixed(1).replace('.', ',')} GB`;
  if (mb >= 10) return `${Math.round(mb)} MB`;
  if (mb >= 1) return `${mb.toFixed(1).replace('.', ',')} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function formatsOf(item) {
  return [...new Set((programOf(item).files || []).map((file) => file.format).filter(Boolean))];
}

// Colores de las etiquetas de formato: un tono fijo por familia mezclado con la
// tinta y el borde del tema, para que se lean igual en claro, oscuro y retro.
const FORMAT_HUE = { exe: '#4b6fd6', msi: '#4b6fd6', deb: '#c25a30' };
export function formatStyle(format) {
  const hue = FORMAT_HUE[format];
  if (!hue) return 'border-color:var(--border);color:var(--muted)';
  return `border-color:color-mix(in srgb,${hue} 55%,var(--border));color:color-mix(in srgb,${hue} 62%,var(--ink))`;
}

// Qué sistema y arquitectura tiene quien mira. Es una pista para proponer la
// descarga, no un permiso: el resto de archivos siguen a un clic.
export function detectDevice() {
  const nav = typeof navigator === 'undefined' ? {} : navigator;
  const platform = String(nav.userAgentData?.platform || nav.platform || '').toLowerCase();
  const agent = String(nav.userAgent || '').toLowerCase();
  let os = '';
  if (platform.includes('win') || agent.includes('windows')) os = 'win';
  else if (platform.includes('mac') || agent.includes('mac os')) os = 'mac';
  else if (platform.includes('linux') || agent.includes('linux')) os = 'linux';
  const arch = /aarch64|arm64|armv8/.test(platform + ' ' + agent) ? 'arm64' : 'x64';
  return { os, arch };
}

const FORMAT_PREFERENCE = ['exe', 'msi', 'deb', 'zip', 'rar'];
export function bestFileFor(item, device) {
  const files = programOf(item).files || [];
  const rank = (file) => {
    const pref = FORMAT_PREFERENCE.indexOf(file.format);
    return pref < 0 ? FORMAT_PREFERENCE.length : pref;
  };
  return files
    .filter((file) => file.os === device.os && file.arch === device.arch)
    .sort((a, b) => rank(a) - rank(b))[0] || null;
}

// Columnas de la matriz que hacen falta: win, lnx y arm siempre (las del
// mockup); mac solo si algún programa lo trae.
export function matrixFor(items) {
  const hasMac = items.some((item) => covers(item, 'mac'));
  return MATRIX.filter((m) => m.key !== 'mac' || hasMac);
}

// Libre = licencia de software libre reconocible; el resto de licencias son
// programas gratuitos de código cerrado (lo único que tiene sentido repartir).
const LIBRE = /^(a?gpl|lgpl|mpl|apache|mit|bsd|isc|epl|eupl|cc0|unlicense|zlib|artistic|wtfpl|0bsd)\b/i;
export function licenseKind(license) {
  const value = String(license || '').trim();
  if (!value) return '';
  return LIBRE.test(value) || /software libre|free software|open source|c[oó]digo abierto/i.test(value) ? 'libre' : 'gratuita';
}

export function shortDate(iso, locale) {
  if (!iso) return '';
  const date = new Date(`${iso}T12:00:00`);
  if (Number.isNaN(date.getTime())) return iso;
  return new Intl.DateTimeFormat(locale || 'es', { day: 'numeric', month: 'short' }).format(date).replace('.', '');
}
