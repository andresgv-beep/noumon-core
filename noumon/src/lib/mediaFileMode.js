// Las URLs protegidas llevan query de autenticacion y pueden tener fragmento.
// El formato depende del nombre/ruta del recurso, no del final de esa query.
export function mediaFileMode(file) {
  const candidates = [file?.name || '', String(file?.url || '').split(/[?#]/)[0]];
  for (const candidate of candidates) {
    if (/\.(mp4|webm|m4v|mov)$/i.test(candidate)) return 'video';
    if (/\.(mp3|ogg|oga|flac|m4a|wav)$/i.test(candidate)) return 'audio';
    if (/\.(jpe?g|png|gif|webp)$/i.test(candidate)) return 'image';
    if (/\.pdf$/i.test(candidate)) return 'pdf';
  }
  return 'unsupported';
}

export function mediaFileName(url) {
  const path = String(url || '').split(/[?#]/)[0];
  const name = path.slice(path.lastIndexOf('/') + 1);
  try { return decodeURIComponent(name); } catch { return name; }
}
