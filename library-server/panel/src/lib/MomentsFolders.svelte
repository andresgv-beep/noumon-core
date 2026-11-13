<script>
  import { onMount } from 'svelte'
  import { getMomentsFolders, addMomentsFolder, removeMomentsFolder, rescanMoments } from './api.js'
  import { t } from './i18n.svelte.js'

  let data = $state(null)
  let path = $state('')
  let name = $state('')
  let busy = $state(false)
  let error = $state('')

  async function load() { data = await getMomentsFolders() }
  async function run(action) {
    if (busy) return
    busy = true; error = ''
    try { await action(); await load() }
    catch (e) { error = t(e.code || 'folders.failed') }
    finally { busy = false }
  }
  onMount(() => run(async () => {}))
  function add(event) {
    event.preventDefault()
    run(async () => { await addMomentsFolder(path, name); path = ''; name = '' })
  }
  function remove(folder) {
    if (confirm(t('folders.confirmRemove', { name: folder.name }))) run(() => removeMomentsFolder(folder.id))
  }
</script>

<div class="toolbar">
  <span class="cnt">{t('folders.title')}</span><span class="grow"></span>
  <button class="btn" disabled={busy} onclick={() => run(rescanMoments)}>{busy ? t('folders.working') : t('folders.rescan')}</button>
</div>
{#if error}<div class="folder-error" role="alert">{error}</div>{/if}
{#if data}
  <div class="setcard">
    <h4>{t('folders.local')}</h4>
    <p>{t('folders.intro')}</p>
    <code class="folder-path">{data.defaultPath}</code>
    <p>{t('folders.auto', { seconds: data.intervalSeconds })}</p>
  </div>
  <div class="setcard">
    <h4>{t('folders.add')}</h4>
    <p>{t('folders.externalHint')}</p>
    <form onsubmit={add}>
      <label>{t('folders.path')}<input required bind:value={path} placeholder={data.defaultPath.startsWith('/') ? '/mnt/videos' : 'D:\\Videos'} disabled={busy} /></label>
      <label>{t('folders.name')}<input bind:value={name} maxlength="200" disabled={busy} /></label>
      <button class="btn btn-primary" disabled={busy || !path.trim()}>{t('folders.addButton')}</button>
    </form>
  </div>
  {#each data.folders as folder (folder.id)}
    <div class="setcard">
      <div class="folder-head"><h4>{folder.name}</h4><span class="badge {folder.available ? 'b-signal' : 'b-warn'}">{folder.available ? t('folders.available') : t('folders.offline')}</span></div>
      <code class="folder-path">{folder.path}</code>
      <div class="folder-head"><p>{t('folders.count', { n: folder.count })}</p><button class="btn" disabled={busy} onclick={() => remove(folder)}>{t('folders.remove')}</button></div>
    </div>
  {/each}
  <p class="folder-note">{t('folders.templates')}</p>
{:else if busy}
  <div class="empty">{t('common.loading')}</div>
{/if}

<style>
  p, .folder-note { color: var(--ink-faint); font-size: 12px; line-height: 1.6; }
  .folder-path { display: block; overflow-wrap: anywhere; padding: 10px 0; }
  form { display: grid; gap: 14px; margin-top: 16px; }
  label { display: grid; gap: 6px; font-size: 12px; }
  input { padding: 9px 12px; border: 1px solid var(--line); background: var(--canvas); color: var(--ink); border-radius: 6px; width: 100%; box-sizing: border-box; }
  form .btn { justify-self: start; }
  .folder-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .folder-head h4 { margin-bottom: 0; }
  .folder-error { padding: 12px; margin-bottom: 14px; color: var(--crit); background: var(--crit-dim); border-radius: 6px; }
</style>
