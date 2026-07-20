<script>
  import { onMount } from 'svelte'
  import { getLanguages, getStorage, translateAvailable, translateDownload, translateRemove, getDeps, installDep } from './api.js'
  import { t } from './i18n.svelte.js'
  import { bytes } from './fmt.js'

  let langs = $state(null)
  let modelsSection = $state(null)
  let loading = $state(true)
  let flash = $state('')

  // aprovisionamiento del motor (admin_deps.go)
  let tool = $state(null) // {installed, installing, progress, bytes, error, ...} | null
  let activating = $state(false)

  // panel "añadir idioma"
  let showAvail = $state(false)
  let avail = $state([])
  let availLoading = $state(false)
  let busy = $state({}) // id → true

  const NAME = {
    en: 'inglés', es: 'español', ca: 'catalán', fr: 'francés', de: 'alemán',
    it: 'italiano', pt: 'portugués', gl: 'gallego', eu: 'euskera', nl: 'neerlandés',
    ru: 'ruso', uk: 'ucraniano', pl: 'polaco', ar: 'árabe', zh: 'chino', cs: 'checo',
    bg: 'búlgaro', el: 'griego', et: 'estonio', is: 'islandés', mk: 'macedonio',
    mt: 'maltés', nb: 'noruego', nn: 'noruego', sl: 'esloveno', sq: 'albanés',
    tr: 'turco', hbs: 'serbocroata', eng: 'inglés',
  }
  const nm = (c) => NAME[c] || c

  async function load() {
    loading = true
    langs = await getLanguages()
    if (!langs?.available) {
      const deps = await getDeps()
      tool = deps.find((d) => d.id === 'translateLocally') || null
    } else {
      tool = null
    }
    try {
      const st = await getStorage()
      modelsSection = (st.sections || []).find((s) => s.key === 'models') || null
    } catch { modelsSection = null }
    loading = false
  }
  onMount(load)

  // Instalación del motor: arranca y sondea /api/admin/deps hasta terminar;
  // después espera a que translate-wrap lo detecte (available:true).
  async function doInstallEngine() {
    if (!tool || tool.installing) return
    flash = ''
    try {
      const r = await installDep(tool.id)
      if (!r.ok && r.status !== 202) throw new Error((await r.json().catch(() => ({}))).error || 'error')
    } catch (e) {
      flash = t('trans.installError', { err: e.message })
      return
    }
    tool = { ...tool, installing: true, progress: 0, error: '' }
    const poll = setInterval(async () => {
      const deps = await getDeps()
      const next = deps.find((d) => d.id === 'translateLocally')
      if (!next) return
      tool = next
      if (!next.installing) {
        clearInterval(poll)
        if (next.installed && !next.error) waitForEngine()
      }
    }, 1000)
  }

  // El binario ya está: translate-wrap lo detecta solo (reintento en caliente o
  // reinicio del supervisor). Sondea hasta que el motor responda.
  function waitForEngine() {
    activating = true
    const poll = setInterval(async () => {
      langs = await getLanguages()
      if (langs?.available) {
        clearInterval(poll)
        activating = false
        tool = null
        load()
      }
    }, 3000)
  }

  async function toggleAvail() {
    showAvail = !showAvail
    if (showAvail && !avail.length) {
      availLoading = true
      try {
        const r = await translateAvailable()
        avail = r.models || []
      } catch (e) {
        flash = t('trans.availFail', { err: e.message || '' })
      } finally {
        availLoading = false
      }
    }
  }

  async function doDownload(m) {
    busy = { ...busy, [m.id]: true }; flash = t('trans.downloadingModel', { id: m.id })
    try {
      const r = await translateDownload(m.id)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('trans.modelAdded', { id: m.id })
      await load()
      avail = avail.map((x) => (x.id === m.id ? { ...x, installed: true } : x))
    } catch (e) {
      flash = t('trans.modelAddFail', { id: m.id, err: e.message })
    } finally {
      busy = { ...busy, [m.id]: false }
    }
  }

  async function doRemove(pair) {
    const id = `${pair.from}-${pair.to}-tiny` // pares instalados hoy son tiny
    if (!confirm(t('trans.confirmRemove', { from: pair.from, to: pair.to }))) return
    busy = { ...busy, [id]: true }; flash = ''
    try {
      const r = await translateRemove(id)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('trans.removed', { from: pair.from, to: pair.to })
      await load()
      avail = avail.map((x) => (x.id === id ? { ...x, installed: false } : x))
    } catch (e) {
      flash = t('trans.removeFail', { err: e.message })
    } finally {
      busy = { ...busy, [id]: false }
    }
  }

  const pairs = $derived(langs?.pairs || [])
  const availSorted = $derived([...avail].sort((a, b) => Number(a.installed) - Number(b.installed) || a.id.localeCompare(b.id)))
  const installPct = $derived(tool?.bytes ? Math.min(100, Math.round(((tool.progress || 0) / tool.bytes) * 100)) : 0)
</script>

<div class="toolbar">
  <span class="cnt">
    {t('trans.title')} ·
    {#if langs?.available}{t('trans.installedCount', { n: pairs.length })}{:else}{t('trans.engineOff')}{/if}
  </span>
  <span class="grow"></span>
  {#if flash}<span style="font-size:12px;color:var(--ink-mute)">{flash}</span>{/if}
  {#if langs?.available}
    <button class="btn btn-primary" onclick={toggleAvail}>{showAvail ? t('trans.closeAvail') : t('trans.addLang')}</button>
  {/if}
  <button class="btn" onclick={load} disabled={loading}>↻</button>
</div>

{#if loading}
  <div class="empty">{t('trans.reading')}</div>
{:else if !langs?.available}
  {#if activating || (tool?.installed && !tool?.error)}
    <div class="setcard install-card">
      <div class="big">{t('trans.activating')}</div>
      <div class="dnote">{t('trans.activatingHint')}</div>
    </div>
  {:else if tool?.installing}
    <div class="setcard install-card">
      <div class="big">{t('trans.installing', { p: installPct })}</div>
      <div class="bar"><i style="width:{installPct}%"></i></div>
      <div class="dnote">{bytes(tool.progress || 0)} / {bytes(tool.bytes)}</div>
    </div>
  {:else if tool}
    <div class="setcard install-card">
      <div class="big">{t('trans.missingTitle')}</div>
      <div class="dnote" style="max-width:520px">{t('trans.missingBody')}</div>
      {#if tool.error}
        <div class="dnote" style="color:var(--crit)">{t('trans.installError', { err: tool.error })}</div>
      {/if}
      <div>
        <button class="btn btn-primary" onclick={doInstallEngine}>
          {tool.error ? t('trans.retry') : t('trans.installBtn')}
        </button>
      </div>
      <div class="dnote">{t('trans.installHint', { label: tool.label, version: tool.version, license: tool.license, size: bytes(tool.bytes), source: tool.source })}</div>
    </div>
  {:else}
    <div class="empty">
      <div class="big">{t('trans.missingTitle')}</div>
      {t('trans.remoteNote')}
    </div>
  {/if}
{:else}
  {#if showAvail}
    <div class="setcard">
      <h4>{t('trans.availTitle')}</h4>
      {#if availLoading}
        <div style="color:var(--ink-faint);font-size:12.5px">{t('trans.availQuerying')}</div>
      {:else}
        {#each availSorted as m (m.id)}
          <div class="row" style="grid-template-columns:1fr auto;margin-bottom:6px;padding:9px 11px">
            <div style="min-width:0">
              <div class="cname" style="font-size:13px">{nm(m.from)} → {nm(m.to)} <span class="badge b-mute">{m.type}</span></div>
              <div class="cpath">{m.id}</div>
            </div>
            {#if m.installed}
              <span class="badge b-signal">{t('trans.installed')}</span>
            {:else}
              <button class="btn btn-primary" disabled={busy[m.id]} onclick={() => doDownload(m)}>
                {busy[m.id] ? t('trans.downloading') : t('trans.download')}
              </button>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}

  <div class="label">{t('trans.installedLabel')}</div>
  {#if pairs.length}
    {#each pairs as p (p.from + '-' + p.to)}
      <div class="row" style="grid-template-columns:40px 1fr auto">
        <div class="cic" style="background:var(--magenta);color:#0a0a0c">文</div>
        <div style="min-width:0">
          <div class="cname">{nm(p.from)} → {nm(p.to)}</div>
          <div class="cpath">{p.from}-{p.to}-tiny</div>
        </div>
        <button class="btn" disabled={busy[`${p.from}-${p.to}-tiny`]} onclick={() => doRemove(p)}>
          {busy[`${p.from}-${p.to}-tiny`] ? '…' : t('trans.remove')}
        </button>
      </div>
    {/each}
  {:else}
    <div class="empty">{t('trans.noModels')}</div>
  {/if}

  <div class="setcard" style="margin-top:12px">
    <div class="setrow">
      <span>{t('trans.onDisk')}</span>
      <code>{modelsSection?.exists ? bytes(modelsSection.bytes) : '—'}</code>
    </div>
  </div>
{/if}

<style>
  .install-card { display: flex; flex-direction: column; gap: 10px; max-width: 560px; margin: 24px auto; padding: 20px 22px; }
  .install-card .big { font-size: 14px; color: var(--ink); font-weight: 600; }
  .install-card .dnote { font-size: 12px; color: var(--ink-mute); line-height: 1.5; }
  .install-card .bar { margin-top: 0; }
</style>
