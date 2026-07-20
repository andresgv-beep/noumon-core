<script>
  // Formulario de carga manual / edición. source define la app y qué campos se
  // muestran: moments (vídeo) · cabinet (documento). Crea vía /api/admin/upload;
  // si recibe `item`, edita su ficha vía /api/admin/media/update (sin cambiar el fichero).
  import { uploadContent, updateContent } from './api.js'
  import { t } from './i18n.svelte.js'

  let { source = 'moments', item = null, onDone } = $props()
  const editing = !!item
  const src = editing ? item.source : source
  const isVideo = $derived(src === 'moments')

  let file = $state(null)
  let cover = $state(null)
  let channelAvatar = $state(null)
  let fileName = $state('')
  let title = $state(item?.title || '')
  let author = $state(item?.author || '')
  let collection = $state('')
  let tags = $state((item?.tags || []).join(', '))
  let description = $state(item?.description || '')
  let date = $state(item?.date || '')
  let duration = $state(item?.duration || '')
  let language = $state(item?.language || '')
  let contributor = $state(item?.contributor || '')
  let license = $state(item?.license || '')
  let access = $state(editing ? '' : 'blocked') // alta segura; '' en edición = mantener nivel actual

  let busy = $state(false)
  let err = $state('')

  function pickFile(e) {
    file = e.currentTarget.files?.[0] || null
    fileName = file?.name || ''
    if (file && !title.trim()) title = file.name.replace(/\.[^.]+$/, '').replace(/[_-]+/g, ' ').trim()
  }
  function pickCover(e) { cover = e.currentTarget.files?.[0] || null }
  function pickAvatar(e) { channelAvatar = e.currentTarget.files?.[0] || null }

  async function submit(e) {
    e?.preventDefault()
    if (!editing && !file) { err = t('upload.errNoFile'); return }
    if (!title.trim()) { err = t('upload.errNoTitle'); return }
    busy = true; err = ''
    const fields = { source: src, title, author, tags, description, access }
    if (!editing) fields.collection = collection
    if (isVideo) { fields.duration = duration }
    else { fields.date = date; fields.language = language; fields.contributor = contributor; fields.license = license }
    try {
      const files = { cover, channel_avatar: channelAvatar }
      const resp = editing
        ? await updateContent(item.id, fields, files)
        : await uploadContent(fields, { ...files, file })
      if (!resp.ok) { const b = await resp.json().catch(() => ({})); throw new Error(b.error || t('upload.saveFail')) }
      onDone?.()
    } catch (e) { err = e.message } finally { busy = false }
  }
</script>

<form class="uform" onsubmit={submit}>
  <div class="ulabel">{editing ? t('upload.editTitle') : t('upload.newTitle')} · {isVideo ? 'Moments' : 'Cabinet'}</div>

  {#if editing}
    <div class="editfile"><b>{t('upload.fileLabel')}</b> {item.media}<span class="hint"> · {t('upload.fileNoChange')}</span></div>
  {:else}
    <label class="drop">
      <input type="file" accept={isVideo ? 'video/*' : '.pdf,.epub,.mp3,.ogg,.flac,.m4a,.wav,.jpg,.jpeg,.png,.gif,.webp,.txt,.md'} onchange={pickFile} />
      <svg class="ic" viewBox="0 0 24 24" style="width:22px;height:22px"><path d="M12 15V3M7 8l5-5 5 5M4 15v4a2 2 0 002 2h12a2 2 0 002-2v-4" /></svg>
      <span>{fileName || (isVideo ? t('upload.dropVideo') : t('upload.dropDoc'))}</span>
    </label>
  {/if}

  <div class="ugrid">
    <label class="fld" style="grid-column:1 / -1">{isVideo ? t('upload.name') : t('upload.title')}<input type="text" bind:value={title} placeholder={isVideo ? t('upload.phVideoTitle') : t('upload.phDocTitle')} /></label>

    <label class="fld">{isVideo ? t('upload.channelAuthor') : t('upload.author')}<input type="text" bind:value={author} /></label>
    {#if !editing}
      <label class="fld">{t('upload.collection')}<input type="text" bind:value={collection} placeholder={t('upload.phCollection')} /></label>
    {/if}

    {#if isVideo}
      <label class="fld">{t('upload.duration')}<input type="number" min="0" bind:value={duration} placeholder={t('upload.phOptional')} /></label>
      <label class="fld">{t('upload.tags')}<input type="text" bind:value={tags} placeholder={t('upload.phCommaTags')} /></label>
    {:else}
      <label class="fld">{t('upload.year')}<input type="text" bind:value={date} placeholder={t('upload.phYear')} /></label>
      <label class="fld">{t('upload.language')}<input type="text" bind:value={language} placeholder={t('upload.phLanguage')} /></label>
      <label class="fld">{t('upload.categories')}<input type="text" bind:value={tags} placeholder={t('upload.phCommaCats')} /></label>
      <label class="fld">{t('upload.license')}<input type="text" bind:value={license} placeholder={t('upload.phLicense')} /></label>
      <label class="fld">{t('upload.contributor')}<input type="text" bind:value={contributor} placeholder={t('upload.phContributor')} /></label>
    {/if}

    <label class="fld" style="grid-column:1 / -1">{t('upload.description')}<textarea rows="2" bind:value={description}></textarea></label>

    <label class="fld">{t('upload.visibility')}
      <select bind:value={access}>
        {#if editing}<option value="">{t('upload.accessKeep')}</option>{/if}
        <option value="open">{t('upload.accessOpen')}</option>
        <option value="login">{t('upload.accessLogin')}</option>
        <option value="blocked">{t('upload.accessBlocked')}</option>
      </select>
    </label>
    <label class="fld">{isVideo ? t('upload.thumb') : t('upload.cover')} {editing ? t('upload.change') : t('upload.optional')}
      <input type="file" accept="image/*" onchange={pickCover} />
    </label>
    {#if isVideo}
      <label class="fld">{t('upload.channelLogo')} {editing ? t('upload.change') : t('upload.optional')}
        <input type="file" accept="image/*" onchange={pickAvatar} />
      </label>
    {/if}
  </div>

  {#if err}<div class="uerr">{err}</div>{/if}

  <div class="uactions">
    <button type="button" class="btn" onclick={() => onDone?.()}>{t('upload.cancel')}</button>
    <button type="submit" class="btn btn-primary" disabled={busy}>
      {busy ? t('upload.saving') : editing ? t('upload.saveChanges') : t('upload.publishTo', { app: isVideo ? 'Moments' : 'Cabinet' })}
    </button>
  </div>
</form>

<style>
  .uform { background: var(--canvas); border: 1px solid var(--signal-border); border-radius: 11px; padding: 16px; margin-bottom: 14px; }
  .ulabel { font-size: 12.5px; font-weight: 600; color: var(--ink-dim); margin-bottom: 14px; }
  .drop { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 18px; border: 1px dashed var(--line-strong); border-radius: 9px; color: var(--ink-mute); font-size: 12.5px; text-align: center; cursor: pointer; margin-bottom: 14px; }
  .drop:hover { border-color: var(--signal-border); color: var(--ink-dim); }
  .drop input[type=file] { display: none; }
  .drop .ic { color: var(--signal); }
  .editfile { font-size: 12px; color: var(--ink-dim); background: var(--window-bg); border: 1px solid var(--line-bright); border-radius: 8px; padding: 9px 11px; margin-bottom: 14px; }
  .editfile .hint { color: var(--ink-faint); }
  .ugrid { display: grid; grid-template-columns: 1fr 1fr; gap: 11px; }
  .fld { display: flex; flex-direction: column; gap: 5px; font-size: 11.5px; color: var(--ink-mute); }
  .fld input, .fld textarea, .fld select { background: var(--window-bg); border: 1px solid var(--line-bright); border-radius: 7px; padding: 8px 10px; font-size: 12.5px; color: var(--ink); font-family: inherit; resize: vertical; }
  .fld input:focus, .fld textarea:focus, .fld select:focus { border-color: var(--signal-border); outline: none; }
  .fld select { cursor: pointer; }
  .fld input[type=file] { padding: 6px; color: var(--ink-mute); font-size: 12px; }
  .uerr { margin-top: 12px; font-size: 12px; color: var(--crit); }
  .uactions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px; }
  @media (max-width: 620px) { .ugrid { grid-template-columns: 1fr; } }
</style>
