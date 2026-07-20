<script>
  import { onMount, onDestroy } from 'svelte'
  import { listDownloads, pauseDownload, resumeDownload, cancelDownload, clearDownloads, getMedia, deleteMedia } from './api.js'
  import { bytes } from './fmt.js'
  import { t } from './i18n.svelte.js'
  import KiwixCatalog from './KiwixCatalog.svelte'
  import UploadForm from './UploadForm.svelte'

  let sub = $state('kiwix')
  let formOpen = $state(false)
  let editItem = $state(null)

  function go(s) { sub = s; formOpen = false; editItem = null }

  // ── Medios del pool (para Moments / Cabinet) ──
  let media = $state([])
  let busyDel = $state({})
  async function loadMedia() { media = await getMedia() }
  const momentsItems = $derived(media.filter((m) => m.source === 'moments'))
  const cabinetItems = $derived(media.filter((m) => m.source === 'cabinet'))
  const KIND = { video: 'import.kindVideo', audio: 'import.kindAudio', gallery: 'import.kindImage', pdf: 'import.kindDoc', reader: 'import.kindDoc' }

  async function del(it) {
    if (!confirm(t('import.confirmDelete', { title: it.title }))) return
    busyDel = { ...busyDel, [it.id]: true }
    try { await deleteMedia(it.id); media = media.filter((x) => x.id !== it.id) }
    catch (e) { alert(t('import.deleteFail')) }
    busyDel = { ...busyDel, [it.id]: false }
  }

  // ── Cola de descargas (catálogo ZIM) ──
  let jobs = $state([])
  let poll
  const ACTIVE = new Set(['queued', 'downloading', 'paused'])
  async function refreshQueue() { jobs = await listDownloads() }
  async function clearQueue() { try { await clearDownloads() } catch (e) {} refreshQueue() }
  const jobName = (j) => (j.dest_path ? j.dest_path.split(/[\\/]/).pop() : j.owner_id || j.url)
  const queueCount = $derived(jobs.filter((j) => ACTIVE.has(j.status)).length)

  onMount(() => {
    refreshQueue(); loadMedia()
    poll = setInterval(refreshQueue, 2000)
  })
  onDestroy(() => clearInterval(poll))
</script>

<div class="stabs">
  <button class="stab" class:on={sub === 'kiwix'} onclick={() => go('kiwix')}>{t('import.tabKiwix')}</button>
  <button class="stab" class:on={sub === 'moments'} onclick={() => go('moments')}>Moments {#if momentsItems.length}<span class="qn">{momentsItems.length}</span>{/if}</button>
  <button class="stab" class:on={sub === 'cabinet'} onclick={() => go('cabinet')}>Cabinet {#if cabinetItems.length}<span class="qn">{cabinetItems.length}</span>{/if}</button>
  <button class="stab" class:on={sub === 'cola'} onclick={() => go('cola')}>{t('import.tabQueue')} {#if queueCount}<span class="qn">{queueCount}</span>{/if}</button>
</div>

{#if sub === 'kiwix'}
  <KiwixCatalog />

{:else if sub === 'moments' || sub === 'cabinet'}
  {@const items = sub === 'moments' ? momentsItems : cabinetItems}
  {@const appName = sub === 'moments' ? 'Moments' : 'Cabinet'}
  <div class="toolbar">
    <span class="cnt"><b>{items.length}</b> {t('import.inApp', { app: appName })}</span>
    <span class="grow"></span>
    {#if !formOpen && !editItem}
      <button class="btn btn-primary" onclick={() => (formOpen = true)}>＋ {t('import.newImport')}</button>
    {/if}
  </div>

  {#if formOpen}
    <UploadForm source={sub} onDone={() => { formOpen = false; loadMedia() }} />
  {:else if editItem}
    {#key editItem.id}
      <UploadForm item={editItem} onDone={() => { editItem = null; loadMedia() }} />
    {/key}
  {/if}

  {#if items.length && !editItem}
    {#each items as it (it.id)}
      <div class="row" style="grid-template-columns:1fr auto">
        <div style="min-width:0">
          <div class="cname">{it.title}</div>
          <div class="cpath" style="font-family:var(--font-sans)">
            <span class="badge b-mute">{t(KIND[it.template] || 'import.kindDoc')}</span>
            {it.collection}{#if it.author} · {it.author}{/if}
          </div>
        </div>
        <div style="display:flex;gap:6px;align-items:center">
          <button class="btn" onclick={() => { editItem = it; formOpen = false }}>✏ {t('import.edit')}</button>
          <button class="btn" onclick={() => del(it)} disabled={busyDel[it.id]}>{busyDel[it.id] ? '…' : '🗑 ' + t('import.delete')}</button>
        </div>
      </div>
    {/each}
  {:else if !formOpen && !editItem}
    <div class="empty">
      <div class="big">{sub === 'moments' ? t('import.emptyMoments') : t('import.emptyCabinet')}</div>
      {sub === 'moments' ? t('import.emptyHintMoments') : t('import.emptyHintCabinet')}
    </div>
  {/if}

{:else}
  <div class="toolbar">
    <span class="cnt"><b>{jobs.length}</b> {t('import.queueCount', { n: queueCount })}</span>
    <span class="grow"></span>
    <button class="btn" onclick={refreshQueue}>↻ {t('import.refresh')}</button>
    <button class="btn" title={t('import.clearTitle')} aria-label={t('import.clearLabel')}
      onclick={clearQueue} disabled={!jobs.some((j) => j.status === 'done' || j.status === 'error' || j.status === 'cancelled')}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v5M14 11v5"/></svg>
    </button>
  </div>
  {#if jobs.length}
    {#each jobs as j (j.id)}
      {@const pct = j.total_bytes > 0 ? Math.round((j.written_bytes / j.total_bytes) * 100) : 0}
      <div class="row" style="grid-template-columns:1fr 90px auto">
        <div style="min-width:0">
          <div class="cname">{jobName(j)}</div>
          <div class="cpath">
            {j.owner_kind} · {bytes(j.written_bytes)}{#if j.total_bytes > 0} / {bytes(j.total_bytes)}{/if}
            {#if j.error_msg} · <span style="color:var(--crit)">{j.error_msg}</span>{/if}
          </div>
          {#if ACTIVE.has(j.status) && j.total_bytes > 0}<div class="bar" style="max-width:320px"><i style="width:{pct}%"></i></div>{/if}
        </div>
        <div class="cmeta">
          <span class="badge {j.status === 'done' ? 'b-signal' : j.status === 'error' ? 'b-warn' : j.status === 'downloading' ? 'b-info' : 'b-mute'}">{j.status}</span>
          {#if j.total_bytes > 0}<br><span style="color:var(--ink-faint)">{pct}%</span>{/if}
        </div>
        <div style="display:flex;gap:6px;justify-self:end">
          {#if j.status === 'downloading' || j.status === 'queued'}
            <button class="btn" title={t('import.pause')} onclick={() => pauseDownload(j.id).then(refreshQueue)}>⏸</button>
          {:else if j.status === 'paused' || j.status === 'error'}
            <button class="btn" title={t('import.resume')} onclick={() => resumeDownload(j.id).then(refreshQueue)}>▶</button>
          {/if}
          {#if j.status !== 'done' && j.status !== 'cancelled'}
            <button class="btn" title={t('import.cancel')} onclick={() => cancelDownload(j.id).then(refreshQueue)}>✕</button>
          {/if}
        </div>
      </div>
    {/each}
  {:else}
    <div class="empty"><div class="big">{t('import.queueEmpty')}</div>{t('import.queueEmptyHint')}</div>
  {/if}
{/if}
