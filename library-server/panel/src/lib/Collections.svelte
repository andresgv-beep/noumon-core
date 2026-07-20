<script>
  import { onMount } from 'svelte'
  import { getAdminZim, getCollections, registerZim, unregisterZim, setZimInteractive, indexZim, indexAllZims, cancelZimIndex, getAccessMap, setAccess } from './api.js'
  import { t } from './i18n.svelte.js'
  import { bytes, num } from './fmt.js'
  import Downloaded from './Downloaded.svelte'

  let { onImport } = $props()

  let sub = $state('kiwix')    // 'kiwix' (ZIM) | 'media' (medios locales del pool)
  let zim = $state(null)       // /api/admin/zim
  let rich = $state({})        // providerItemId → {id, itemCount, description}
  let accessMap = $state({})   // collectionId → {access, minAge, allowDownload}
  let error = $state('')
  let loading = $state(true)
  let busy = $state({})
  let flash = $state('')

  // Vista: filtros, selección y paginación
  let q = $state('')
  let fAccess = $state('')     // '' | open | login | blocked
  let fScripts = $state('')    // '' | on | off
  let fIndex = $state('')      // '' | yes | no
  let fLang = $state('')
  let page = $state(1)
  const PER_PAGE = 8
  let checked = $state({})     // zim.id → true
  let openId = $state('')      // zim.id abierto en el drawer

  const ACCESS = [
    { k: 'open', key: 'access.open', cls: 'b-signal' },
    { k: 'login', key: 'access.login', cls: 'b-info' },
    { k: 'blocked', key: 'access.blocked', cls: 'b-mute' },
  ]
  const accLabel = (a) => t((ACCESS.find((x) => x.k === a) || ACCESS[2]).key)
  const accCls = (a) => (ACCESS.find((x) => x.k === a) || ACCESS[2]).cls

  async function load() {
    loading = true; error = ''
    try {
      zim = await getAdminZim()
    } catch (e) {
      error = e.message; loading = false; return
    }
    try {
      const cols = await getCollections()
      const map = {}
      for (const c of cols) {
        const pid = c.source?.providerItemId
        if (pid) map[pid] = { id: c.id, itemCount: c.itemCount, description: c.description }
      }
      rich = map
    } catch { rich = {} }
    try { accessMap = await getAccessMap() } catch { accessMap = {} }
    loading = false
  }
  onMount(load)

  const richOf = (file) => rich[file.replace(/\.zim$/i, '')] || {}
  const cfgOf = (cid) => (cid && accessMap[cid]) || { access: 'blocked', minAge: 0, allowDownload: false }

  async function saveAccess(cid, access, minAge, allowDownload) {
    minAge = Math.max(0, Math.min(18, Number(minAge) || 0)) // siempre entero limpio
    accessMap = { ...accessMap, [cid]: { access, minAge, allowDownload } } // optimista
    const r = await setAccess(cid, access, minAge, allowDownload)
    if (!r.ok) flash = t('msg.accessFail')
    else if (flash === t('msg.accessFail')) flash = ''
  }
  const changeAccess = (cid, a) => saveAccess(cid, a, cfgOf(cid).minAge, cfgOf(cid).allowDownload)
  const changeAge = (cid, age) => saveAccess(cid, cfgOf(cid).access, Math.max(0, Math.min(18, Number(age) || 0)), cfgOf(cid).allowDownload)
  const toggleDownload = (cid) => saveAccess(cid, cfgOf(cid).access, cfgOf(cid).minAge, !cfgOf(cid).allowDownload)

  async function doRegister(file) {
    busy = { ...busy, [file]: true }; flash = ''
    try {
      const r = await registerZim(file)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('msg.added', { name: file }); await load()
    } catch (e) { flash = t('msg.addFail', { err: e.message }) } finally { busy = { ...busy, [file]: false } }
  }
  async function doUnregister(z, skipConfirm = false) {
    if (!skipConfirm && !confirm(t('msg.confirmRemove', { name: z.title || z.file }))) return
    busy = { ...busy, [z.id]: true }; flash = ''
    try {
      const r = await unregisterZim(z.id)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('msg.removed', { name: z.title || z.file })
      if (openId === z.id) openId = ''
      await load()
    } catch (e) { flash = t('msg.removeFail', { err: e.message }) } finally { busy = { ...busy, [z.id]: false } }
  }

  async function setInteractive(z, enabled) {
    if (z.interactive === enabled) return
    let acknowledge = false
    if (enabled && !z.official) {
      acknowledge = confirm(t('msg.confirmScripts', { name: z.title || z.file }))
      if (!acknowledge) return
    }
    busy = { ...busy, [`interactive:${z.id}`]: true }; flash = ''
    try {
      const r = await setZimInteractive(z.id, enabled, acknowledge)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t(enabled ? 'msg.scriptsOn' : 'msg.scriptsOff', { name: z.title || z.file })
      await load()
    } catch (e) {
      flash = t('msg.scriptsFail', { err: e.message })
    } finally {
      busy = { ...busy, [`interactive:${z.id}`]: false }
    }
  }

  // ── Indexado full-text (buscador) ──────────────────────────────────────────
  const indexJob = $derived(zim?.indexJob || null)
  const indexing = $derived(indexJob?.status === 'indexing')
  const pct = (j) => (j && j.total ? Math.min(100, Math.round((j.scanned / j.total) * 100)) : 0)

  async function refreshZim() { try { zim = await getAdminZim() } catch {} }
  // Mientras hay un job, refresca el estado (progreso) cada 1,5 s.
  $effect(() => {
    if (!indexing) return
    const timer = setInterval(refreshZim, 1500)
    return () => clearInterval(timer)
  })

  async function doIndex(z) {
    busy = { ...busy, [`index:${z.file}`]: true }; flash = ''
    try {
      const r = await indexZim(z.file)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('msg.indexing', { name: z.title || z.file }); await refreshZim()
    } catch (e) { flash = t('msg.indexFail', { err: e.message }) } finally { busy = { ...busy, [`index:${z.file}`]: false } }
  }
  async function doIndexAll() {
    busy = { ...busy, indexAll: true }; flash = ''
    try {
      const r = await indexAllZims()
      const d = await r.json().catch(() => ({}))
      if (!r.ok) throw new Error(d.error || 'error')
      flash = d.count ? t('msg.indexingCount', { n: d.count }) : t('msg.allIndexed'); await refreshZim()
    } catch (e) { flash = t('msg.indexFail', { err: e.message }) } finally { busy = { ...busy, indexAll: false } }
  }
  async function doCancelIndex() { try { await cancelZimIndex(); await refreshZim() } catch {} }

  const registered = $derived(zim?.registered || [])
  const unregistered = $derived(zim?.unregistered || [])
  const canManage = $derived(zim?.canManage ?? false)
  const unindexedCount = $derived(registered.filter((z) => !z.indexed && z.present).length)

  // ── Filtrado + paginación (cliente) ────────────────────────────────────────
  const languages = $derived([...new Set(registered.map((z) => z.language).filter(Boolean))].sort())
  const filtered = $derived(registered.filter((z) => {
    const cfg = cfgOf(richOf(z.file).id)
    if (q && !(z.title || z.file).toLowerCase().includes(q.toLowerCase()) && !z.file.toLowerCase().includes(q.toLowerCase())) return false
    if (fAccess && cfg.access !== fAccess) return false
    if (fScripts === 'on' && !z.interactive) return false
    if (fScripts === 'off' && z.interactive) return false
    if (fIndex === 'yes' && !z.indexed) return false
    if (fIndex === 'no' && z.indexed) return false
    if (fLang && z.language !== fLang) return false
    return true
  }))
  const pages = $derived(Math.max(1, Math.ceil(filtered.length / PER_PAGE)))
  $effect(() => { if (page > pages) page = pages })
  const pageRows = $derived(filtered.slice((page - 1) * PER_PAGE, page * PER_PAGE))

  const checkedIds = $derived(Object.keys(checked).filter((id) => checked[id]))
  const allPageChecked = $derived(pageRows.length > 0 && pageRows.every((z) => checked[z.id]))
  function togglePage() {
    const next = { ...checked }
    const on = !allPageChecked
    for (const z of pageRows) next[z.id] = on
    checked = next
  }
  const byId = $derived(Object.fromEntries(registered.map((z) => [z.id, z])))
  const open = $derived(openId ? byId[openId] : null)

  // ── Acciones en lote ───────────────────────────────────────────────────────
  async function bulkAccess(a) {
    for (const id of checkedIds) {
      const z = byId[id]; if (!z) continue
      const cid = richOf(z.file).id; if (!cid) continue
      await changeAccess(cid, a)
    }
    flash = t('bulk.accessApplied', { access: accLabel(a), n: checkedIds.length })
  }
  async function bulkBlockScripts() {
    for (const id of checkedIds) {
      const z = byId[id]
      if (z?.interactive) await setInteractive(z, false)
    }
  }
  async function bulkIndex() {
    for (const id of checkedIds) {
      const z = byId[id]
      if (z && !z.indexed && z.present) { await doIndex(z); break } // el indexador procesa de uno en uno
    }
  }
  async function bulkRemove() {
    const names = checkedIds.map((id) => byId[id]?.title || byId[id]?.file).filter(Boolean)
    const list = names.slice(0, 6).join('\n') + (names.length > 6 ? '\n…' : '')
    if (!confirm(t('bulk.confirmRemove', { n: names.length, names: list }))) return
    for (const id of checkedIds) { const z = byId[id]; if (z) await doUnregister(z, true) }
    checked = {}
  }
</script>

<div class="stabs">
  <button class="stab" class:on={sub === 'kiwix'} onclick={() => (sub = 'kiwix')}>{t('col.tabZim')}</button>
  <button class="stab" class:on={sub === 'media'} onclick={() => (sub = 'media')}>{t('col.tabMedia')}</button>
</div>

{#if sub === 'media'}
  <Downloaded />
{:else}
<div class="toolbar">
  <div class="search" style="max-width:340px">
    <svg class="ic" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
    <input placeholder={t('col.search')} bind:value={q} oninput={() => (page = 1)} />
  </div>
  <label class="fsel">{t('col.fAccess')}
    <select bind:value={fAccess} onchange={() => (page = 1)}>
      <option value="">{t('common.all')}</option>
      {#each ACCESS as a (a.k)}<option value={a.k}>{t(a.key)}</option>{/each}
    </select>
  </label>
  <label class="fsel">{t('col.fScripts')}
    <select bind:value={fScripts} onchange={() => (page = 1)}>
      <option value="">{t('common.all')}</option><option value="on">{t('scripts.on')}</option><option value="off">{t('scripts.off')}</option>
    </select>
  </label>
  <label class="fsel">{t('col.fIndex')}
    <select bind:value={fIndex} onchange={() => (page = 1)}>
      <option value="">{t('common.all')}</option><option value="yes">{t('index.yes')}</option><option value="no">{t('index.no')}</option>
    </select>
  </label>
  {#if languages.length > 1}
    <label class="fsel">{t('col.fLang')}
      <select bind:value={fLang} onchange={() => (page = 1)}>
        <option value="">{t('common.all')}</option>
        {#each languages as l (l)}<option value={l}>{l.toUpperCase()}</option>{/each}
      </select>
    </label>
  {/if}
  <span class="grow"></span>
  {#if flash}<span style="font-size:12px;color:var(--ink-mute)">{flash}</span>{/if}
  <button class="btn" disabled={!canManage || indexing || unindexedCount === 0 || busy.indexAll} onclick={doIndexAll} title={t('col.indexAllTitle')}>
    {busy.indexAll ? '…' : `${t('col.indexAll')}${unindexedCount ? ` (${unindexedCount})` : ''}`}
  </button>
  <button class="btn" onclick={load} disabled={loading}>↻ {t('common.refresh')}</button>
  {#if onImport}
    <button class="btn btn-primary" onclick={onImport}>
      <svg class="ic" viewBox="0 0 24 24" style="width:14px;height:14px"><path d="M12 4v11M8 11l4 4 4-4M5 20h14" /></svg>
      {t('col.import')}
    </button>
  {/if}
</div>

{#if indexing}
  <div class="idx-banner">
    <div class="idx-head"><b>{t('idx.banner', { name: indexJob.name })}</b><small>{t('idx.meta', { n: num(indexJob.indexed), p: pct(indexJob) })}</small></div>
    <div class="bar" style="margin-top:0"><i style="width:{pct(indexJob)}%"></i></div>
    <button class="btn" onclick={doCancelIndex}>{t('common.cancel')}</button>
  </div>
{/if}

{#if error}
  <div class="empty"><div class="big">{t('col.loadError')}</div>{error}</div>
{:else if loading && !zim}
  <div class="empty">{t('col.reading')}</div>
{:else}
  {#if unregistered.length}
    <div class="label">{t('col.unregLabel')}</div>
    {#each unregistered as u (u.file)}
      <div class="row" style="grid-template-columns:40px 1fr auto">
        <div class="cic" style="background:var(--warn-dim);color:var(--warn)">＋</div>
        <div style="min-width:0">
          <div class="cname">{u.file}</div>
          <div class="cpath">{t('col.unregSub', { size: bytes(u.bytes) })}</div>
        </div>
        <button class="btn btn-primary" disabled={!canManage || busy[u.file]} onclick={() => doRegister(u.file)}>
          {busy[u.file] ? '…' : t('col.add')}
        </button>
      </div>
    {/each}
    <div style="height:12px"></div>
  {/if}

  <div class="dwrap">
    <div class="dmain">
      <div class="tblcard">
        <div class="bulk">
          <input type="checkbox" checked={allPageChecked} onchange={togglePage} style="accent-color:var(--accent)" />
          <span><b style="color:var(--ink-dim)">{checkedIds.length}</b> {t('bulk.selected')}</span>
          {#if checkedIds.length}
            {#each ACCESS as a (a.k)}
              <button class="chip" onclick={() => bulkAccess(a.k)}>{t(a.key)}</button>
            {/each}
            <button class="chip" onclick={bulkBlockScripts}>{t('bulk.blockScripts')}</button>
            <button class="chip" disabled={indexing} onclick={bulkIndex}>{t('bulk.index')}</button>
            <button class="chip" style="color:var(--crit);border-color:var(--crit-border)" onclick={bulkRemove}>{t('bulk.remove')}</button>
          {/if}
          <span class="grow"></span>
          <span>{t('bulk.ofTotal', { a: filtered.length, b: registered.length })}</span>
        </div>
        {#if pageRows.length}
          <table class="tbl">
            <thead>
              <tr>
                <th style="width:34px"></th>
                <th>{t('col.thCollection')}</th>
                <th>{t('col.thLanguage')}</th>
                <th>{t('col.thAccess')}</th>
                <th>{t('col.thScripts')}</th>
                <th>{t('col.thIndex')}</th>
                <th style="text-align:right">{t('col.thItems')}</th>
              </tr>
            </thead>
            <tbody>
              {#each pageRows as z (z.id)}
                {@const info = richOf(z.file)}
                {@const cfg = cfgOf(info.id)}
                <tr class:sel={openId === z.id} onclick={() => (openId = openId === z.id ? '' : z.id)}>
                  <td onclick={(e) => e.stopPropagation()}>
                    <input type="checkbox" checked={!!checked[z.id]} onchange={() => (checked = { ...checked, [z.id]: !checked[z.id] })} style="accent-color:var(--accent)" />
                  </td>
                  <td style="min-width:0">
                    <div style="display:flex;align-items:center;gap:11px;min-width:0">
                      <div class="cic" style="width:34px;height:34px;font-size:13px;background:var(--sel);color:var(--ink-dim)">{(z.title || z.file).charAt(0).toUpperCase()}</div>
                      <div style="min-width:0">
                        <div style="font-weight:600;color:var(--ink);white-space:nowrap;overflow:hidden;text-overflow:ellipsis">
                          {z.title || z.file}
                          {#if !z.present}<span class="badge b-warn" style="margin-left:6px">{t('badge.missing')}</span>{/if}
                          {#if z.trustStale}<span class="badge b-warn" style="margin-left:6px">{t('badge.changed')}</span>{/if}
                        </div>
                        <div class="cpath">{z.file}</div>
                      </div>
                    </div>
                  </td>
                  <td>{#if z.language}<span class="badge b-accent">{z.language.slice(0, 3).toUpperCase()}</span>{/if}</td>
                  <td><span class="badge {accCls(cfg.access)}">{accLabel(cfg.access)}{#if cfg.minAge > 0} · {cfg.minAge}+{/if}</span></td>
                  <td><span class="badge {z.interactive ? 'b-signal' : 'b-mute'}">{z.interactive ? t('scripts.on') : t('scripts.off')}</span></td>
                  <td>
                    {#if indexing && indexJob?.file === z.file}
                      <span class="stdot busy"><i></i>{t('index.busy', { p: pct(indexJob) })}</span>
                    {:else if z.indexed}
                      <span class="stdot ok"><i></i>{t('index.yes')}</span>
                    {:else}
                      <span class="stdot"><i></i>{t('index.no')}</span>
                    {/if}
                  </td>
                  <td class="tnum" style="text-align:right">{info.itemCount ? num(info.itemCount) : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <div class="empty"><div class="big">{registered.length ? t('col.emptyFiltered') : t('col.emptyNone')}</div>{registered.length ? t('col.emptyFilteredSub') : t('col.emptyNoneSub')}</div>
        {/if}
      </div>

      <div class="pager">
        <span>{t('pager.showing', { a: filtered.length ? (page - 1) * PER_PAGE + 1 : 0, b: Math.min(page * PER_PAGE, filtered.length), n: filtered.length })}</span>
        <span class="grow"></span>
        <button class="pbtn" disabled={page <= 1} onclick={() => page--}>‹</button>
        {#each Array.from({ length: pages }, (_, i) => i + 1) as p (p)}
          {#if pages <= 7 || p === 1 || p === pages || Math.abs(p - page) <= 1}
            <button class="pbtn" class:on={p === page} onclick={() => (page = p)}>{p}</button>
          {:else if p === 2 || p === pages - 1}
            <span>…</span>
          {/if}
        {/each}
        <button class="pbtn" disabled={page >= pages} onclick={() => page++}>›</button>
      </div>
    </div>

    {#if open}
      {@const info = richOf(open.file)}
      {@const cid = info.id}
      {@const cfg = cfgOf(cid)}
      <aside class="drawer scroll">
        <div class="dhead">
          <div class="cic" style="background:var(--sel);color:var(--ink-dim)">{(open.title || open.file).charAt(0).toUpperCase()}</div>
          <div style="min-width:0">
            <div class="dtitle">{open.title || open.file}</div>
            <div class="dfile">{open.file}</div>
            <div class="dmeta">
              {#if info.itemCount}{t('drawer.items', { n: num(info.itemCount) })} ·{/if} {bytes(open.bytes)}
              {#if open.language}<span class="badge b-accent">{open.language.slice(0, 3).toUpperCase()}</span>{/if}
              {#if open.official}<span class="badge b-signal">{t('badge.official')}</span>{/if}
            </div>
          </div>
          <button class="dx" title={t('win.close')} onclick={() => (openId = '')}>✕</button>
        </div>

        {#if cid}
          <div class="dsec">
            <div class="label">{t('drawer.access')}</div>
            {#each ACCESS as a (a.k)}
              <label class="dradio">
                <input type="radio" name="dacc" checked={cfg.access === a.k} onchange={() => changeAccess(cid, a.k)} />
                {t(a.key)}
              </label>
            {/each}
            <div class="dfield">{t('drawer.minAge')}
              <input type="number" min="0" max="18" value={cfg.minAge} onchange={(e) => changeAge(cid, e.target.value)} disabled={cfg.access === 'blocked'} />
            </div>
            <label class="dcheck">
              <input type="checkbox" checked={cfg.allowDownload} disabled={cfg.access === 'blocked'} onchange={() => toggleDownload(cid)} />
              {t('drawer.anonDl')}
            </label>
          </div>
        {/if}

        <div class="dsec">
          <div class="label">{t('drawer.scripts')}</div>
          <select class="dselect" disabled={!canManage || !open.present || busy[`interactive:${open.id}`]}
            value={open.interactive ? 'on' : 'off'}
            onchange={(e) => setInteractive(open, e.target.value === 'on')}>
            <option value="on">{t('scripts.on')}</option>
            <option value="off">{t('scripts.off')}</option>
          </select>
          <div class="dnote">{t('drawer.scriptsNote')}{#if !open.official} {t('drawer.notOfficial')}{/if}</div>
        </div>

        <div class="dsec">
          <div class="label">{t('drawer.searchSec')}</div>
          {#if indexing && indexJob?.file === open.file}
            <span class="stdot busy"><i></i>{t('index.busy', { p: pct(indexJob) })} · {t('drawer.articles', { n: num(indexJob.indexed) })}</span>
            <button class="btn" onclick={doCancelIndex}>{t('drawer.cancelIndex')}</button>
          {:else if open.indexed}
            <span class="stdot ok"><i></i>{t('index.yes')}</span>
            <button class="btn" disabled={indexing || busy[`index:${open.file}`]} onclick={() => doIndex(open)}>{t('drawer.reindex')}</button>
          {:else}
            <span class="stdot"><i></i>{t('index.titleOnly')}</span>
            <button class="btn" disabled={!canManage || !open.present || indexing || busy[`index:${open.file}`]} onclick={() => doIndex(open)}>
              {busy[`index:${open.file}`] ? '…' : t('drawer.indexBtn')}
            </button>
          {/if}
        </div>

        <div class="dsec">
          <div class="label">{t('drawer.info')}</div>
          <dl class="dinfo">
            <dt>{t('drawer.uuid')}</dt><dd>{open.id}</dd>
            <dt>{t('drawer.file')}</dt><dd>{open.file}</dd>
            <dt>{t('drawer.size')}</dt><dd>{bytes(open.bytes)}</dd>
            {#if open.name}<dt>{t('drawer.name')}</dt><dd>{open.name}</dd>{/if}
          </dl>
        </div>

        <div class="dsec">
          <button class="btn btn-danger" style="justify-content:center" disabled={!canManage || busy[open.id]} onclick={() => doUnregister(open)}>
            {busy[open.id] ? '…' : t('drawer.remove')}
          </button>
          <div class="dnote">{t('drawer.removeNote')}</div>
        </div>
      </aside>
    {/if}
  </div>

  <div class="empty" style="padding:14px 24px;font-size:11.5px">{t('col.footNote')}</div>
{/if}
{/if}

<style>
  .idx-banner { display: flex; align-items: center; gap: 12px; margin: 0 0 12px; padding: 10px 14px; border: 1px solid var(--warn-border); border-radius: 5px; background: var(--canvas); }
  .idx-head { display: flex; flex-direction: column; gap: 2px; min-width: 180px; }
  .idx-head small { color: var(--ink-faint); font-size: 11px; }
  .idx-banner .bar { flex: 1; }
  .idx-banner .bar i { background: var(--warn); }
</style>
