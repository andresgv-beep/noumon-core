const surfaces = { cabinet: 'cabinet', moments: 'moments', depot: 'depot', programas: 'depot' }

// Browser directory pickers expose relative paths, never the laptop's full path.
export function planLocalImport(files, surface, collection) {
  const entries = [], skipped = []
  const seen = new Set()
  for (const file of files) {
    const parts = (file.webkitRelativePath || file.name).split('/')
    if (parts.some((part) => !part || part.startsWith('.'))) { skipped.push(file.name); continue }
    const appIndex = parts.findIndex((part) => surfaces[part.toLowerCase()])
    let target = surface
    let relative = parts
    if (appIndex >= 0) {
      const detected = surfaces[parts[appIndex].toLowerCase()]
      if (surface !== 'auto' && surface !== detected) throw new Error('import.local.wrongSurface')
      target = detected
      relative = parts.slice(appIndex + 1)
    }
    if (target === 'auto') { skipped.push(file.name); continue }
    if (relative.length === 1) relative = [collection.trim(), ...relative]
    if (relative.length < 2 || relative.some((part) => !part || part.startsWith('.') || /[\\:\r\n\0]/.test(part))) {
      throw new Error('import.local.invalidPath')
    }
    const path = relative.join('/')
    const key = (target + '/' + path).toLowerCase()
    if (seen.has(key)) { skipped.push(file.name); continue }
    seen.add(key)
    entries.push({ file, surface: target, path, loaded: 0, state: 'pending', error: '' })
  }
  // Preserve all original metadata; primary media and covers arrive first.
  entries.sort((a, b) => Number(a.path.toLowerCase().endsWith('.json')) - Number(b.path.toLowerCase().endsWith('.json')))
  return { entries, skipped }
}

export function uploadLocalFile(entry, onProgress, signal) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const query = new URLSearchParams({ surface: entry.surface, path: entry.path, size: String(entry.file.size) })
    xhr.open('POST', '/api/admin/import/upload?' + query)
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.withCredentials = true
    const abort = () => xhr.abort()
    const finish = (fn, value) => { signal.removeEventListener('abort', abort); fn(value) }
    signal.addEventListener('abort', abort, { once: true })
    xhr.upload.onprogress = (event) => onProgress(event.loaded)
    xhr.onload = () => {
      let data = {}
      try { data = JSON.parse(xhr.responseText) } catch {}
      if (xhr.status >= 200 && xhr.status < 300) finish(resolve, data)
      else finish(reject, new Error(data.errorCode || (xhr.status === 401 || xhr.status === 403 ? 'import.local.auth' : 'import.local.failed')))
    }
    xhr.onerror = () => finish(reject, new Error('import.local.network'))
    xhr.onabort = () => finish(reject, new Error('import.local.cancelled'))
    if (signal.aborted) { finish(reject, new Error('import.local.cancelled')); return }
    xhr.send(entry.file)
  })
}
