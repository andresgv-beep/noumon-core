<script>
  import { onMount } from 'svelte'
  import { browseMomentsDirectories } from './api.js'
  import { t } from './i18n.svelte.js'

  let { initialPath = '', onChoose, onClose } = $props()
  let dialog
  let data = $state(null)
  let loading = $state(false)
  let error = $state('')
  let filter = $state('')
  let requested = 0
  const directories = $derived((data?.directories || []).filter((d) => d.name.toLocaleLowerCase().includes(filter.toLocaleLowerCase())))

  async function browse(path = '') {
    const request = ++requested
    loading = true; error = ''; filter = ''
    try {
      const next = await browseMomentsDirectories(path)
      if (request === requested) data = next
    } catch (e) {
      if (request === requested) error = t(e.code || 'folders.failed')
    } finally { if (request === requested) loading = false }
  }
  onMount(() => { dialog.showModal(); browse(initialPath); return () => { requested++ } })
  function cancel(event) { event.preventDefault(); onClose() }
</script>

<dialog bind:this={dialog} oncancel={cancel} aria-labelledby="folder-picker-title">
  <header>
    <div><h2 id="folder-picker-title">{t('picker.title')}</h2><p>{t('picker.subtitle')}</p></div>
    <button type="button" class="close" onclick={onClose} aria-label={t('picker.close')}>×</button>
  </header>
  <div class="explorer">
    <aside aria-label={t('picker.locations')}>
      <h3>{t('picker.locations')}</h3>
      <button type="button" class:active={!data?.path} onclick={() => browse()}>{t('picker.allLocations')}</button>
      {#each data?.locations || [] as location (location.path)}
        <button type="button" class:active={data.path === location.path} title={location.path} onclick={() => browse(location.path)}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 4h16v16H4zM4 15h16M16 18h1" /></svg>
          <span>{location.labelKey ? t(location.labelKey, { name: location.name }) : location.name}</span>
        </button>
      {/each}
    </aside>
    <section>
      <div class="navigation">
        <button type="button" class="btn" disabled={loading || !data?.path} onclick={() => browse(data?.parent || '')} title={t('picker.up')} aria-label={t('picker.up')}>↑</button>
        <div class="current" title={data?.path || ''}>{data?.path || t('picker.locations')}</div>
        <button type="button" class="btn" disabled={loading} onclick={() => browse(data?.path || '')} title={t('picker.refresh')} aria-label={t('picker.refresh')}>↻</button>
      </div>
      {#if error}<div class="picker-error" role="alert">{error}</div>{/if}
      {#if loading}
        <div class="empty">{t('common.loading')}</div>
      {:else if data && !data.path}
        <div class="location-grid">
          {#each data.locations as location (location.path)}
            <button type="button" class="location" onclick={() => browse(location.path)}>
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 4h16v16H4zM4 15h16M16 18h1" /></svg>
              <b>{location.labelKey ? t(location.labelKey, { name: location.name }) : location.name}</b><small>{location.path}</small>
            </button>
          {/each}
        </div>
      {:else if data}
        <input class="filter" aria-label={t('picker.filter')} placeholder={t('picker.filter')} bind:value={filter} />
        <div class="directories" aria-label={t('picker.folders')}>
          {#each directories as directory (directory.path)}
            <button type="button" class="directory" onclick={() => browse(directory.path)}>
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7V4h7l2 3h9v13H3z" /></svg><span>{directory.name}</span><span class="arrow">›</span>
            </button>
          {:else}<div class="empty">{filter ? t('picker.noMatches') : t('picker.empty')}</div>{/each}
        </div>
      {/if}
    </section>
  </div>
  <footer>
    <div class="selection"><small>{t('picker.selected')}</small><span>{data?.path || t('picker.none')}</span>{#if data?.selectionHint}<small>{t(data.selectionHint)}</small>{/if}</div>
    <div class="actions"><button type="button" class="btn" onclick={onClose}>{t('picker.cancel')}</button>
      <button type="button" class="btn btn-primary" disabled={loading || !!error || !data?.selectable} onclick={() => onChoose(data.path)}>{t('picker.choose')}</button></div>
  </footer>
</dialog>

<style>
  dialog { margin: auto; max-height: calc(100dvh - 32px); width: min(900px, calc(100vw - 32px)); max-width: none; padding: 0; border: 1px solid var(--line); border-radius: 12px; color: var(--ink); background: var(--canvas); box-shadow: 0 24px 80px #0008; }
  dialog::backdrop { background: #0009; backdrop-filter: blur(3px); }
  header { display: flex; justify-content: space-between; align-items: start; padding: 22px 24px; border-bottom: 1px solid var(--line); }
  h2 { margin: 0; font-size: 18px; } p { margin: 7px 0 0; color: var(--ink-faint); font-size: 12px; line-height: 1.5; }
  .close { font-size: 26px; padding: 0 6px; color: var(--ink-mute); }
  .explorer { display: grid; grid-template-columns: 210px minmax(0, 1fr); height: min(430px, 52vh); }
  aside { padding: 16px 10px; overflow: auto; border-right: 1px solid var(--line); background: var(--canvas-soft); }
  h3 { margin: 0 10px 12px; font-size: 11px; text-transform: uppercase; color: var(--ink-faint); letter-spacing: .06em; }
  aside button { display: flex; align-items: center; gap: 9px; width: 100%; padding: 10px; border-radius: 6px; color: var(--ink-dim); text-align: left; font-size: 12px; }
  aside button span { overflow-wrap: anywhere; } aside button:hover, aside button.active { background: var(--sel); color: var(--ink); }
  svg { width: 20px; height: 20px; flex: none; fill: none; stroke: currentColor; stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; }
  section { min-width: 0; display: flex; flex-direction: column; overflow: hidden; }
  .navigation { display: flex; align-items: center; gap: 10px; padding: 12px; border-bottom: 1px solid var(--line); }
  .current { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-family: monospace; }
  .filter { margin: 12px; padding: 9px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--ink); background: var(--canvas-soft); }
  .directories { overflow: auto; padding: 0 12px 12px; }
  .directory { display: flex; align-items: center; gap: 12px; width: 100%; padding: 12px 10px; text-align: left; color: var(--ink); border-bottom: 1px solid var(--line); border-radius: 4px; }
  .directory svg { color: var(--info, #8b9cff); } .directory:hover, .location:hover { background: var(--sel); } .directory span { overflow-wrap: anywhere; } .arrow { margin-left: auto; color: var(--ink-faint); }
  .location-grid { padding: 16px; overflow: auto; display: grid; gap: 12px; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); align-content: start; }
  .location { display: flex; flex-direction: column; align-items: start; gap: 9px; padding: 18px; border: 1px solid var(--line); border-radius: 8px; text-align: left; color: var(--ink); }
  .location b { font-size: 12px; } .location small { color: var(--ink-faint); font-size: 11px; overflow-wrap: anywhere; }
  footer { display: flex; align-items: center; gap: 16px; padding: 18px 24px; border-top: 1px solid var(--line); }
  .selection { flex: 1; min-width: 0; display: grid; gap: 5px; } .selection small { color: var(--ink-faint); font-size: 11px; } .selection span { font-size: 12px; overflow-wrap: anywhere; }
  .actions { display: flex; gap: 9px; flex-shrink: 0; } .picker-error { margin: 12px; color: var(--crit); font-size: 12px; line-height: 1.5; }
  @media(max-width: 620px) { .explorer { grid-template-columns: 1fr; height: 54vh; } aside { display: none; } footer { flex-direction: column; align-items: stretch; padding: 14px; } .actions { justify-content: end; } header { padding: 16px; } }
</style>
