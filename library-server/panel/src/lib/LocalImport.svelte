<script>
  import { onDestroy } from 'svelte'
  import { t } from './i18n.svelte.js'
  import { bytes } from './fmt.js'
  import { planLocalImport, uploadLocalFile } from './localImport.js'
  import { finishLocalImport } from './api.js'

  let surface = $state('auto')
  let collection = $state(t('import.local.defaultCollection'))
  let entries = $state([])
  let skipped = $state(0)
  let message = $state('')
  let working = $state(false)
  let controller
  let filePicker, folderPicker
  const total = $derived(entries.reduce((sum, entry) => sum + entry.file.size, 0))
  const loaded = $derived(entries.reduce((sum, entry) => sum + entry.loaded, 0))
  const completed = $derived(entries.filter((entry) => entry.state === 'done').length)
  const percent = $derived(total ? Math.min(100, Math.round(loaded / total * 100)) : 0)

  function selectFiles(event) {
    message = ''
    try {
      const plan = planLocalImport(Array.from(event.target.files || []), surface, collection)
      entries = plan.entries
      skipped = plan.skipped.length
      if (!entries.length) message = 'import.local.noFiles'
    } catch (error) { entries = []; message = error.message }
    event.target.value = ''
  }

  async function start() {
    working = true
    message = ''
    controller = new AbortController()
    try {
      for (const entry of entries) {
        if (controller.signal.aborted) break
        if (entry.state === 'done') continue
        entry.state = 'uploading'; entry.error = ''; entry.loaded = 0
        try {
          await uploadLocalFile(entry, (n) => { entry.loaded = n }, controller.signal)
          entry.loaded = entry.file.size
          entry.state = 'done'
        } catch (error) {
          entry.state = 'error'; entry.error = error.message; entry.loaded = 0
          if (controller.signal.aborted || error.message === 'import.local.network' || error.message === 'import.local.auth') break
        }
      }
      // Generate only missing minimal cards; imported JSON and covers win.
      for (const key of ['cabinet', 'moments', 'depot']) {
        const files = entries.filter((entry) => entry.state === 'done' && entry.surface === key).map((entry) => entry.path)
        const pendingMetadata = entries.some((entry) => entry.surface === key && entry.path.toLowerCase().endsWith('.json') && entry.state !== 'done')
        if (files.length) await finishLocalImport(key, files, !pendingMetadata)
      }
      message = controller.signal.aborted ? 'import.local.cancelled' : entries.some((entry) => entry.state !== 'done') ? 'import.local.partial' : 'import.local.finished'
    } catch (error) { message = error.message || 'import.local.failed' }
    finally { working = false }
  }
  onDestroy(() => controller?.abort())
</script>

<svelte:window onbeforeunload={(event) => { if (working) event.preventDefault() }} />

<section class="setcard local-import">
  <h3>{t('import.local.title')}</h3>
  <p class="hint">{t('import.local.hint')}</p>
  <div class="options">
    <label>{t('import.local.surface')}
      <select bind:value={surface} disabled={working || entries.length > 0}>
        <option value="auto">{t('import.local.auto')}</option>
        <option value="cabinet">Cabinet</option>
        <option value="moments">Moments</option>
        <option value="depot">{t('import.local.programs')}</option>
      </select>
    </label>
    {#if surface !== 'auto'}
      <label>{t('import.local.collection')}
        <input bind:value={collection} disabled={working || entries.length > 0} />
      </label>
    {/if}
  </div>
  <input class="picker" type="file" multiple bind:this={filePicker} onchange={selectFiles} aria-label={t('import.local.files')} />
  <input class="picker" type="file" webkitdirectory multiple bind:this={folderPicker} onchange={selectFiles} aria-label={t('import.local.folder')} />
  <div class="actions">
    <button class="btn" disabled={working} onclick={() => folderPicker.click()}>{t('import.local.folder')}</button>
    <button class="btn" disabled={working || surface === 'auto'} onclick={() => filePicker.click()}>{t('import.local.files')}</button>
    {#if entries.length && !working}<button class="btn" onclick={() => { entries = []; skipped = 0; message = '' }}>{t('import.local.clear')}</button>{/if}
  </div>
  <p class="hint">{t('import.local.folderHint')}</p>
  {#if skipped}<p class="hint">{t('import.local.skipped', { n: skipped })}</p>{/if}
  {#if entries.length}
    <div class="summary">{t('import.local.progress', { n: completed, total: entries.length })} · {bytes(loaded)} / {bytes(total)}</div>
    <progress value={loaded} max={total || 1} aria-label={t('import.local.progressLabel')}></progress>
    <span class="hint">{percent}%</span>
    <div class="file-list">
      {#each entries as entry (entry.surface + '/' + entry.path)}
        <div class="import-file">
          <span class="path">{entry.surface} / {entry.path}</span>
          <span class:error={entry.error}>{entry.error ? t(entry.error) : t('import.local.state.' + entry.state)}</span>
        </div>
      {/each}
    </div>
    <div class="actions">
      {#if working}
        <button class="btn" onclick={() => controller.abort()}>{t('import.local.stop')}</button>
      {:else}
        <button class="btn btn-primary" disabled={completed === entries.length} onclick={start}>{t('import.local.start')}</button>
      {/if}
    </div>
  {/if}
  {#if message}<p role="status">{t(message)}</p>{/if}
</section>

<style>
  .local-import{display:flex;flex-direction:column;gap:14px}
  h3,p{margin:0}
  .hint{color:var(--ink-mute);font-size:13px;line-height:1.6}
  .options,.actions{display:flex;flex-wrap:wrap;gap:12px}
  label{display:flex;flex-direction:column;gap:6px;font-size:13px;min-width:200px}
  input,select{min-height:36px;padding:6px 10px;background:var(--canvas);color:var(--ink);border:1px solid var(--line-bright);border-radius:6px}
  .picker{display:none}
  progress{width:100%;height:10px;accent-color:var(--accent)}
  .summary{font-size:13px}
  .file-list{max-height:340px;overflow:auto;border-top:1px solid var(--line)}
  .import-file{display:flex;flex-wrap:wrap;justify-content:space-between;gap:6px 16px;padding:10px 0;border-bottom:1px solid var(--line);font-size:12px}
  .path{overflow-wrap:anywhere;min-width:0;flex:1 1 260px}
  .error{color:var(--crit)}
</style>
