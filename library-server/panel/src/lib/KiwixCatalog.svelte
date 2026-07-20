<script>
  import { onMount } from 'svelte'
  import { catalogCategories, catalogEntries, catalogDownload } from './api.js'
  import { bytes, num } from './fmt.js'
  import { t } from './i18n.svelte.js'

  let cats = $state([])
  let selCat = $state('wikipedia')
  let selLang = $state('')
  let q = $state('')
  let entries = $state([])
  let loading = $state(false)
  let busy = $state({})
  let flash = $state('')

  const LANGS = [
    { c: '', n: 'cat.langAll' }, { c: 'spa', n: 'cat.langSpa' }, { c: 'eng', n: 'cat.langEng' },
    { c: 'cat', n: 'cat.langCat' }, { c: 'fra', n: 'cat.langFra' },
  ]

  async function loadEntries() {
    loading = true; flash = ''
    try {
      entries = await catalogEntries({ category: selCat, lang: selLang, q })
    } catch (e) {
      flash = t('cat.unreachable', { err: e.message || '' })
      entries = []
    } finally {
      loading = false
    }
  }

  onMount(async () => {
    try { cats = await catalogCategories() } catch {}
    await loadEntries()
  })

  function pickCat(c) { selCat = c; loadEntries() }
  function pickLang(l) { selLang = l; loadEntries() }

  async function download(e) {
    busy = { ...busy, [e.filename]: true }; flash = t('cat.queueing', { name: e.filename })
    try {
      const r = await catalogDownload(e.zimUrl, e.filename)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      flash = t('cat.queued', { title: e.title })
    } catch (err) {
      flash = t('cat.queueFail', { err: err.message })
    } finally {
      busy = { ...busy, [e.filename]: false }
    }
  }
</script>

<div class="ccnote">
  {t('cat.notePre')} <code>.zim</code> {t('cat.noteMid')}
  <b>{t('cat.noteStrong')}</b> {t('cat.notePost')}
</div>

<!-- Categorías -->
<div class="chips">
  {#each cats as c (c)}
    <button class="chip" class:on={selCat === c} onclick={() => pickCat(c)}>{c}</button>
  {/each}
</div>

<!-- Idioma + búsqueda -->
<div class="searchbar">
  <div class="search">
    <input placeholder={t('cat.searchPh')} bind:value={q} onkeydown={(e) => e.key === 'Enter' && loadEntries()} />
  </div>
  <button class="go" onclick={loadEntries} disabled={loading}>{loading ? '…' : t('cat.search')}</button>
</div>
<div class="chips">
  {#each LANGS as l (l.c)}
    <button class="chip" class:on={selLang === l.c} onclick={() => pickLang(l.c)}>{t(l.n)}</button>
  {/each}
  {#if flash}<span style="margin-left:auto;font-size:12px;color:var(--ink-mute);align-self:center">{flash}</span>{/if}
</div>

{#if loading}
  <div class="empty">{t('cat.loading')}</div>
{:else if entries.length}
  <div class="label">{t('cat.results', { n: entries.length, cat: selCat })}</div>
  {#each entries as e (e.filename)}
    {@const flav = (e.filename.replace(/\.zim$/i, '').match(/_(maxi|mini|nopic|nodet|novid)(?:_|$)/i) || [])[1]}
    <div class="row" style="grid-template-columns:1fr auto">
      <div style="min-width:0">
        <div class="cname">
          {e.title}
          {#if e.language}<span class="badge b-info">{e.language}</span>{/if}
          {#if flav}<span class="badge b-mute">{flav}</span>{/if}
          {#if e.inPool}<span class="badge b-signal">{t('cat.inPool')}</span>{/if}
        </div>
        <div class="cpath">{e.filename}{#if e.articles} · {t('cat.articles', { n: num(e.articles) })}{/if}</div>
      </div>
      {#if e.inPool}
        <button class="btn" disabled>✓ {t('cat.downloaded')}</button>
      {:else}
        <button class="btn btn-primary" disabled={busy[e.filename]} onclick={() => download(e)}>
          {busy[e.filename] ? t('cat.busyQueueing') : e.bytes ? t('cat.downloadSize', { size: bytes(e.bytes) }) : t('cat.download')}
        </button>
      {/if}
    </div>
  {/each}
{:else}
  <div class="empty">{t('cat.noResults')}</div>
{/if}
