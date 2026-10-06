<script>
  // Un apartado de medios locales (Documentos, Cabinet o Moments), cada uno en
  // su pestaña del Panel. Arriba, quién lo ve —uno por apartado, no por colección: lo que se
  // publique después en cualquier colección suya hereda esa decisión— y debajo
  // todo su contenido, con la opción de ELIMINAR un item del pool (ficha +
  // fichero(s) + portada + pistas). Admin.
  import { onMount } from 'svelte'
  import { getMedia, deleteMedia, getDocumentItems, getAccessMap, setAccess } from './api.js'
  import { t } from './i18n.svelte.js'

  let { surface: surfaceKey = 'cabinet' } = $props()

  let items = $state([])
  let documents = $state([])
  let loading = $state(true)
  let busy = $state({}) // id -> true mientras borra
  let filter = $state('')
  let accessMap = $state({})
  let accessError = $state('')

  const ACCESS = ['open', 'login', 'blocked']
  // Mismas claves que el servidor (access.go). El apartado de cada item lo dice
  // su ficha (source), no la carpeta: Noumon DL guarda en YouTube/ y Archives/.
  const SURFACES = [
    { k: 'documents', id: 'col:studio:documents', nameKey: 'media.surfaceDocuments', source: '' },
    { k: 'cabinet', id: 'surface:cabinet', name: 'Cabinet', source: 'cabinet' },
    { k: 'moments', id: 'surface:moments', name: 'Moments', source: 'moments' },
    { k: 'depot', id: 'surface:depot', name: 'Depot', source: 'depot' },
  ]
  const surface = SURFACES.find((x) => x.k === surfaceKey) || SURFACES[1]

  async function load() {
    loading = true
    const [media, docs, access] = await Promise.all([
      getMedia(),
      getDocumentItems(),
      getAccessMap().catch(() => ({})),
    ])
    items = media
    documents = docs
    accessMap = access
    loading = false
  }
  onMount(load)

  // Sin fila en el servidor un apartado está bloqueado: misma regla que el gate.
  const cfgOf = (id) => accessMap[id] || { access: 'blocked', minAge: 0, allowDownload: false }

  async function saveAccess(id, patch) {
    const next = { ...cfgOf(id), ...patch }
    next.minAge = Math.max(0, Math.min(18, Number(next.minAge) || 0))
    accessMap = { ...accessMap, [id]: next } // optimista
    const r = await setAccess(id, next.access, next.minAge, next.allowDownload)
    accessError = r.ok ? '' : t('msg.accessFail')
  }

  async function del(it) {
    if (!confirm(t('media.confirmDelete', { title: it.title }))) return
    busy = { ...busy, [it.id]: true }
    try {
      await deleteMedia(it.id)
      items = items.filter((x) => x.id !== it.id)
    } catch (e) {
      alert(t('media.deleteFail'))
    }
    busy = { ...busy, [it.id]: false }
  }

  const KIND = { program: 'media.kindProgram', video: 'media.kindVideo', audio: 'media.kindAudio', gallery: 'media.kindImage', pdf: 'media.kindText', reader: 'media.kindText' }
  const kindLabel = (tpl) => t(KIND[tpl] || 'media.kindDoc')

  const matches = (text) => {
    const q = filter.trim().toLowerCase()
    return !q || text.toLowerCase().includes(q)
  }
  const contentOf = (surface) => surface.source
    ? items.filter((it) => it.source === surface.source)
    : documents
  const shownOf = (surface) => contentOf(surface).filter((it) =>
    matches(it.title + ' ' + (it.collection || '') + ' ' + (it.author || '')))
  const cfg = $derived(cfgOf(surface.id))
  const shown = $derived(shownOf(surface))
</script>

<div class="toolbar">
  {#if accessError}<span class="acc-err">{accessError}</span>{/if}
  <span class="grow"></span>
  <input class="search-in" placeholder={t('media.filter')} bind:value={filter} />
  <button class="btn" onclick={load}>↻ {t('media.refresh')}</button>
</div>

{#if loading}
  <div class="empty">{t('media.loading')}</div>
{:else}
  <section class="cell">
    <header class="cell-head">
      <div style="min-width:0">
        <div class="cname">{surface.nameKey ? t(surface.nameKey) : surface.name}</div>
        <div class="cpath">{t('media.items', { n: contentOf(surface).length })}</div>
      </div>
      <select class="acc-in" aria-label={t('drawer.access')} value={cfg.access}
        onchange={(e) => saveAccess(surface.id, { access: e.currentTarget.value })}>
        {#each ACCESS as a (a)}<option value={a}>{t('access.' + a)}</option>{/each}
      </select>
      <label class="acc-age">{t('drawer.minAge')}
        <input class="acc-in" type="number" min="0" max="18" value={cfg.minAge} disabled={cfg.access === 'blocked'}
          onchange={(e) => saveAccess(surface.id, { minAge: e.currentTarget.value })} />
      </label>
      <label class="acc-dl">
        <input type="checkbox" checked={cfg.allowDownload} disabled={cfg.access === 'blocked'}
          onchange={() => saveAccess(surface.id, { allowDownload: !cfg.allowDownload })} />
        {t('drawer.anonDl')}
      </label>
    </header>

    {#each shown as it (it.id)}
      <div class="item">
        <div style="min-width:0">
          <div class="iname">{it.title}</div>
          {#if surface.source}
            <div class="cpath">
              <span class="badge b-mute">{kindLabel(it.template)}</span>
              {it.collection}{#if it.author} · {it.author}{/if}
            </div>
          {/if}
        </div>
        {#if surface.source}
          <button class="btn btn-danger" title={t('media.deleteTitle')} onclick={() => del(it)} disabled={busy[it.id]}>
            {busy[it.id] ? '…' : '🗑 ' + t('media.delete')}
          </button>
        {/if}
      </div>
    {:else}
      <div class="cell-empty">{contentOf(surface).length ? t('media.noMatch') : t('media.sectionEmpty')}</div>
    {/each}
    {#if !surface.source && documents.length}
      <div class="cell-hint">{t('media.documentsHint')}</div>
    {/if}
  </section>
{/if}

<style>
  .search-in { height: 34px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--border); background: var(--card); color: var(--ink); font: inherit; outline: none; min-width: 160px; }
  .search-in:focus { border-color: var(--accent, #3fb950); }
  .btn-danger:hover:not(:disabled) { border-color: var(--crit, #da6b74); color: var(--crit, #da6b74); }
  .cell { border: 1px solid var(--line); border-radius: 8px; background: var(--canvas); margin-bottom: 14px; overflow: hidden; }
  .cell-head { display: grid; grid-template-columns: minmax(0, 1fr) auto auto auto; align-items: center; gap: 13px; padding: 12px 14px; border-bottom: 1px solid var(--line); }
  .item { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 13px; padding: 9px 14px; border-top: 1px solid var(--line); }
  .item:first-of-type { border-top: 0; }
  .iname { font-size: 13.5px; color: var(--ink); }
  .cell-empty, .cell-hint { padding: 12px 14px; font-size: 12.5px; color: var(--ink-faint); }
  .cell-hint { padding-top: 0; }
  .acc-in { height: 32px; padding: 0 8px; border-radius: 8px; border: 1px solid var(--border); background: var(--card); color: var(--ink); font: inherit; font-size: 13px; }
  .acc-age input { width: 64px; margin-left: 6px; }
  .acc-age, .acc-dl { display: flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--ink-mute); white-space: nowrap; }
  .acc-err { color: var(--crit, #da6b74); font-size: 12.5px; }
  @media (max-width: 760px) { .cell-head { grid-template-columns: 1fr; } }
</style>
