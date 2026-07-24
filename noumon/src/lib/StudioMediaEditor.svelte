<script>
  import StudioImage from './StudioImage.svelte';
  import { t } from './i18n.svelte.js';

  let { document, onChange, onUpload, onError } = $props();
  let uploading = $state('');
  let primaryInput = $state(null);
  let coverInput = $state(null);
  let avatarInput = $state(null);
  let trackInput = $state(null);
  let subtitleInput = $state(null);

  const surface = () => document?.templateKey?.startsWith('moments.') ? 'moments' : 'cabinet';
  const metadata = () => {
    if (!document.metadata || Array.isArray(document.metadata)) document.metadata = {};
    if (!Array.isArray(document.metadata.tracks)) document.metadata.tracks = [];
    if (!Array.isArray(document.metadata.subtitles)) document.metadata.subtitles = [];
    if (!Array.isArray(document.metadata.chapters)) document.metadata.chapters = [];
    if (!document.metadata.collection) document.metadata.collection = 'General';
    return document.metadata;
  };
  const cabinetProfile = () => String(document?.templateKey || '').replace('cabinet.', '');

  function changed() {
    onChange?.();
  }

  function selectCabinetProfile(profile) {
    if (surface() !== 'cabinet' || cabinetProfile() === profile) return;
    document.templateKey = `cabinet.${profile}`;
    metadata().primaryAssetId = '';
    metadata().primaryName = '';
    metadata().tracks = [];
    changed();
  }

  function primaryAccept() {
    switch (document.templateKey) {
      case 'cabinet.pdf': return '.pdf,application/pdf';
      case 'cabinet.audio': return '.mp3,.ogg,.oga,.flac,.m4a,.wav,audio/*';
      case 'cabinet.video':
      case 'moments.video': return '.mp4,.webm,.m4v,.mov,video/*';
      default: return '';
    }
  }

  async function upload(event, purpose, assign) {
    const input = event.currentTarget;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !document || uploading) return;
    uploading = purpose;
    try {
      const asset = await onUpload?.(file, purpose);
      if (asset) {
        assign(asset, file);
        changed();
      }
    } catch (error) {
      onError?.(error);
    } finally {
      uploading = '';
    }
  }

  function addChapter() {
    const chapters = metadata().chapters;
    const last = chapters.at(-1);
    chapters.push({
      start: last ? Number(last.start || 0) + 60 : 0,
      title: t('studio.chapterPlaceholder'),
    });
    changed();
  }

  function removeAt(field, index) {
    metadata()[field].splice(index, 1);
    changed();
  }

  function setTags(value) {
    document.tags = value.split(',').map((tag) => tag.trim()).filter(Boolean).slice(0, 50);
    changed();
  }

  function assetLabel(name, id) {
    if (name) return name;
    return id ? t('studio.assetReady') : '';
  }
</script>

<div class="media-layout">
  <section class="media-form">
    {#if surface() === 'cabinet'}
      <div class="profile-field">
        <span class="field-label">{t('studio.contentProfile')}</span>
        <div class="profile-tabs">
          <button class:active={cabinetProfile() === 'pdf'} onclick={() => selectCabinetProfile('pdf')}>PDF</button>
          <button class:active={cabinetProfile() === 'video'} onclick={() => selectCabinetProfile('video')}>{t('studio.profileVideo')}</button>
          <button class:active={cabinetProfile() === 'audio'} onclick={() => selectCabinetProfile('audio')}>{t('studio.profileAudio')}</button>
        </div>
      </div>
    {/if}

    <div class="field" data-studio-section={surface() === 'moments' ? 'video' : 'file'}>
      <span class="field-label">
        {document.templateKey === 'cabinet.audio' ? t('studio.mainAudio')
          : surface() === 'moments' || document.templateKey === 'cabinet.video'
            ? t('studio.section.video') : t('studio.section.mainFile')}
      </span>
      <button class="drop-zone" class:ready={!!metadata().primaryAssetId} onclick={() => primaryInput?.click()} disabled={!!uploading}>
        <b>{uploading === 'primary' ? t('studio.uploadingFile') : assetLabel(metadata().primaryName, metadata().primaryAssetId) || t('studio.chooseLocalFile')}</b>
        <small>{t('studio.localFileHint')}</small>
      </button>
      <input bind:this={primaryInput} class="hidden-input" type="file" accept={primaryAccept()}
        onchange={(event) => upload(event, 'primary', (asset, file) => {
          metadata().primaryAssetId = asset.id;
          metadata().primaryName = file.name;
        })} />
    </div>

    <div class="fields-two" data-studio-section="metadata">
      <label>{t('studio.documentTitle')}<input value={document.title} oninput={(event) => { document.title = event.currentTarget.value; changed(); }} /></label>
      <label>{surface() === 'moments' ? t('studio.channelAuthor') : t('studio.author')}
        <input value={document.authorLabel || ''} oninput={(event) => { document.authorLabel = event.currentTarget.value; changed(); }} />
      </label>
    </div>
    <label>{surface() === 'moments' ? t('studio.videoDescription') : t('studio.summaryPlaceholder')}
      <textarea rows="5" value={document.summary || ''} oninput={(event) => { document.summary = event.currentTarget.value; changed(); }}></textarea>
    </label>
    <div class="fields-two">
      <label>{t('studio.collection')}<input value={metadata().collection || 'General'} oninput={(event) => { metadata().collection = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.date')}<input value={metadata().date || ''} placeholder="2026" oninput={(event) => { metadata().date = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.language')}<input value={document.language || ''} placeholder="es" oninput={(event) => { document.language = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.tags')}<input value={(document.tags || []).join(', ')} placeholder={t('studio.tagsPlaceholder')} oninput={(event) => setTags(event.currentTarget.value)} /></label>
      <label>{t('studio.contributor')}<input value={metadata().contributor || ''} oninput={(event) => { metadata().contributor = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.license')}<input value={metadata().license || ''} oninput={(event) => { metadata().license = event.currentTarget.value; changed(); }} /></label>
      {#if surface() === 'moments' || document.templateKey === 'cabinet.video'}
        <label>{t('studio.durationSeconds')}<input type="number" min="0" value={metadata().duration || 0} oninput={(event) => { metadata().duration = Number(event.currentTarget.value || 0); changed(); }} /></label>
      {/if}
    </div>

    {#if document.templateKey === 'cabinet.audio'}
      <section class="repeat-field" data-studio-section="tracks">
        <header><span><b>{t('studio.audioTracks')}</b><small>{t('studio.audioTracksHint')}</small></span>
          <button onclick={() => trackInput?.click()} disabled={!!uploading}>＋ {t('studio.addTrack')}</button>
        </header>
        <input bind:this={trackInput} class="hidden-input" type="file" accept=".mp3,.ogg,.oga,.flac,.m4a,.wav,audio/*"
          onchange={(event) => upload(event, 'track', (asset, file) => {
            metadata().tracks.push({ title: file.name.replace(/\.[^.]+$/, ''), assetId: asset.id });
          })} />
        {#each metadata().tracks as track, index}
          <div class="repeat-row">
            <span class="number">{index + 1}</span>
            <input value={track.title} aria-label={t('studio.trackTitle')} oninput={(event) => { track.title = event.currentTarget.value; changed(); }} />
            <button class="remove" onclick={() => removeAt('tracks', index)} aria-label={t('studio.removeEntry')}>×</button>
          </div>
        {/each}
        {#if metadata().tracks.length === 0}<p>{t('studio.noTracks')}</p>{/if}
      </section>
    {/if}

    {#if surface() === 'moments'}
      <section class="repeat-field" data-studio-section="chapters">
        <header><span><b>{t('studio.section.chapters')}</b><small>{t('studio.chaptersHint')}</small></span>
          <button onclick={addChapter}>＋ {t('studio.addChapter')}</button>
        </header>
        {#each metadata().chapters as chapter, index}
          <div class="repeat-row chapter-row">
            <input class="time" type="number" min="0" step="0.1" value={chapter.start} aria-label={t('studio.chapterStart')}
              oninput={(event) => { chapter.start = Number(event.currentTarget.value || 0); changed(); }} />
            <input value={chapter.title} aria-label={t('studio.chapterTitle')} oninput={(event) => { chapter.title = event.currentTarget.value; changed(); }} />
            <button class="remove" onclick={() => removeAt('chapters', index)} aria-label={t('studio.removeEntry')}>×</button>
          </div>
        {/each}
      </section>

      <section class="repeat-field" data-studio-section="subtitles">
        <header><span><b>{t('studio.subtitles')}</b><small>{t('studio.subtitlesHint')}</small></span>
          <button onclick={() => subtitleInput?.click()} disabled={!!uploading}>＋ {t('studio.addSubtitles')}</button>
        </header>
        <input bind:this={subtitleInput} class="hidden-input" type="file" accept=".vtt,text/vtt"
          onchange={(event) => upload(event, 'subtitle', (asset) => {
            metadata().subtitles.push({ lang: document.language || 'es', assetId: asset.id });
          })} />
        {#each metadata().subtitles as subtitle, index}
          <div class="repeat-row subtitle-row">
            <input class="lang" value={subtitle.lang} aria-label={t('studio.subtitleLanguage')} oninput={(event) => { subtitle.lang = event.currentTarget.value; changed(); }} />
            <span>{t('studio.assetReady')}</span>
            <button class="remove" onclick={() => removeAt('subtitles', index)} aria-label={t('studio.removeEntry')}>×</button>
          </div>
        {/each}
      </section>
    {/if}
  </section>

  <aside class="media-preview" data-studio-section="cover">
    <span>{surface() === 'moments' ? t('studio.momentsPreview') : t('studio.cabinetPreview')}</span>
    <div class="image-slots">
      <button class="image-upload cover" onclick={() => coverInput?.click()} disabled={!!uploading}>
        {#if metadata().coverAssetId}
          <StudioImage documentId={document.id} assetId={metadata().coverAssetId} alt={t('studio.section.cover')} compact />
        {:else}
          <b>＋</b><small>{surface() === 'moments' ? t('studio.addThumbnail') : t('studio.addCover')}</small>
        {/if}
      </button>
      <input bind:this={coverInput} class="hidden-input" type="file" accept=".jpg,.jpeg,.png,.gif,.webp,image/*"
        onchange={(event) => upload(event, 'cover', (asset, file) => {
          metadata().coverAssetId = asset.id;
          metadata().coverName = file.name;
        })} />
      {#if surface() === 'moments'}
        <button class="image-upload avatar" onclick={() => avatarInput?.click()} disabled={!!uploading}>
          {#if metadata().channelAvatarAssetId}
            <StudioImage documentId={document.id} assetId={metadata().channelAvatarAssetId} alt={t('studio.channelAvatar')} compact />
          {:else}<b>＋</b><small>{t('studio.channelAvatar')}</small>{/if}
        </button>
        <input bind:this={avatarInput} class="hidden-input" type="file" accept=".jpg,.jpeg,.png,.gif,.webp,image/*"
          onchange={(event) => upload(event, 'avatar', (asset, file) => {
            metadata().channelAvatarAssetId = asset.id;
            metadata().channelAvatarName = file.name;
          })} />
      {/if}
    </div>
    <div class="preview-copy">
      <b>{document.title}</b>
      <small>{document.authorLabel || t('documents.localAuthor')}</small>
      {#if document.summary}<p>{document.summary}</p>{/if}
    </div>
    <p class="preview-hint">{t('studio.previewContract')}</p>
  </aside>
</div>

<style>
  .media-layout{max-width:960px;margin:0 auto;display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:28px;align-items:start}
  .media-form{display:grid;gap:15px}.media-form label{display:flex;flex-direction:column;gap:5px;color:var(--muted);font-size:10px;letter-spacing:.04em}
  .media-form input,.media-form textarea{width:100%;padding:9px 10px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card);color:var(--ink);outline:0}
  .media-form input:focus,.media-form textarea:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .field,.profile-field{display:grid;gap:6px}.field-label{color:var(--muted);font-size:10px;letter-spacing:.04em}
  .fields-two{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px}
  .profile-tabs{display:flex;gap:6px;padding:5px;border-radius:var(--r-md);background:var(--panel)}
  .profile-tabs button{flex:1;padding:8px 10px;border:1px solid transparent;border-radius:var(--r-sm);color:var(--muted);font-size:11px}
  .profile-tabs button:hover{color:var(--ink)}.profile-tabs button.active{border-color:var(--accent-line);background:var(--accent-weak);color:var(--ink)}
  .drop-zone{min-height:104px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:5px;padding:16px;border:1px dashed var(--border);border-radius:var(--r-md);background:var(--card);color:var(--muted);text-align:center}
  .drop-zone:hover{border-color:var(--accent-line);color:var(--ink)}.drop-zone.ready{border-style:solid;border-color:var(--accent-line);background:var(--accent-weak)}
  .drop-zone b{font-size:12px}.drop-zone small{color:var(--faint);font-size:10px}
  .repeat-field{display:grid;gap:8px;padding:12px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel)}
  .repeat-field header{display:flex;align-items:center;justify-content:space-between;gap:12px}
  .repeat-field header>span{display:flex;flex-direction:column;gap:2px}.repeat-field header b{font-size:11px}.repeat-field header small,.repeat-field>p{color:var(--faint);font-size:9.5px}
  .repeat-field header button{padding:6px 8px;border-radius:var(--r-sm);background:var(--card);color:var(--accent-2);font-size:10px}
  .repeat-row{display:grid;grid-template-columns:24px minmax(0,1fr) 28px;gap:6px;align-items:center}
  .repeat-row input{padding:7px 8px}.repeat-row .number{display:grid;place-items:center;color:var(--faint);font:600 10px var(--mono)}
  .repeat-row .remove{height:28px;border-radius:var(--r-sm);color:var(--faint)}.repeat-row .remove:hover{background:color-mix(in srgb,#df7474 12%,transparent);color:#df7474}
  .chapter-row{grid-template-columns:84px minmax(0,1fr) 28px}.subtitle-row{grid-template-columns:70px minmax(0,1fr) 28px}.subtitle-row span{color:var(--muted);font-size:10px}
  .media-preview{position:sticky;top:0;display:grid;gap:10px}.media-preview>span{color:var(--faint);font-size:9px;letter-spacing:.13em;text-transform:uppercase}
  .image-slots{position:relative}.image-upload{overflow:hidden;border:1px dashed var(--border);background:var(--card);color:var(--faint)}
  .image-upload.cover{width:100%;min-height:158px;border-radius:var(--r-lg);display:grid;place-items:center}.image-upload.cover b{font-size:22px}.image-upload small{display:block;font-size:9.5px}
  .image-upload.avatar{position:absolute;left:12px;bottom:-22px;width:54px;height:54px;overflow:hidden;border-radius:50%;background:var(--panel)}
  .image-upload.avatar :global(img),.image-upload.avatar :global(.placeholder){min-height:52px;height:52px;border-radius:50%;object-fit:cover}
  .image-upload.cover :global(img),.image-upload.cover :global(.placeholder){min-height:158px;height:158px;border-radius:0;object-fit:cover}
  .preview-copy{display:flex;flex-direction:column;gap:3px;padding:8px 10px 10px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card)}
  .preview-copy b{font-size:13px}.preview-copy small{color:var(--muted);font-size:10.5px}.preview-copy p{margin:5px 0 0;color:var(--muted);font-size:10.5px;line-height:1.45}
  .preview-hint{margin:0;color:var(--faint);font-size:10.5px;line-height:1.5}.hidden-input{display:none}
  @media(max-width:850px){.media-layout{grid-template-columns:1fr}.media-preview{position:static}.image-slots{max-width:420px}}
  @media(max-width:620px){.fields-two{grid-template-columns:1fr}.profile-tabs{display:grid;grid-template-columns:repeat(3,1fr)}}
</style>
