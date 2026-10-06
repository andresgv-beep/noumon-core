<script>
  // Ficha de un programa para Depot: sus archivos (uno por sistema y formato),
  // la ficha técnica, las capturas y el icono. Solo el admin llega aquí; el
  // servidor lo vuelve a comprobar al guardar y al publicar.
  import StudioImage from './StudioImage.svelte';
  import { t } from './i18n.svelte.js';

  let { document, onChange, onUpload, onError } = $props();
  let uploading = $state('');
  let fileInput = $state(null);
  let shotInput = $state(null);
  let iconInput = $state(null);
  // Tamaño y SHA-256 de lo subido en esta sesión. La ficha publicada los saca
  // del servidor; aquí solo sirven para comprobar de un vistazo lo que se subió.
  let uploaded = $state({});

  const OS = [
    { k: 'win', name: 'Windows' },
    { k: 'linux', name: 'Linux' },
    { k: 'mac', name: 'macOS' },
  ];
  const ARCH = ['x64', 'arm64', 'x86'];
  const SHELVES = ['multimedia', 'office', 'graphics', 'internet', 'utilities', 'system', 'development', 'education', 'games'];

  // Lector PURO: el template lo invoca durante el render (ver StudioMediaEditor).
  const metadata = () => (document?.metadata && !Array.isArray(document.metadata)) ? document.metadata : {};

  function ensureMetadata() {
    if (!document.metadata || Array.isArray(document.metadata)) document.metadata = {};
    const m = document.metadata;
    if (!Array.isArray(m.files)) m.files = [];
    if (!Array.isArray(m.screenshots)) m.screenshots = [];
    if (!m.collection) m.collection = 'General';
    return m;
  }

  $effect(() => {
    if (document) ensureMetadata();
  });

  function changed() {
    onChange?.();
  }

  const extensionOf = (name) => (String(name || '').match(/\.([a-z0-9]+)$/i)?.[1] || '').toLowerCase();

  // Lo que se puede deducir del archivo; lo que no, lo elige el autor. Un .exe o
  // un .msi es de Windows y un .deb de Linux; un zip o un rar no lo dice.
  function guessOS(name) {
    const ext = extensionOf(name);
    if (ext === 'exe' || ext === 'msi') return 'win';
    if (ext === 'deb') return 'linux';
    const lower = name.toLowerCase();
    if (/(^|[^a-z])(win(dows)?|win32|win64)([^a-z]|$)/.test(lower)) return 'win';
    if (/linux/.test(lower)) return 'linux';
    if (/(mac(os)?|osx|darwin)/.test(lower)) return 'mac';
    return '';
  }

  function guessArch(name) {
    const lower = name.toLowerCase();
    if (/(arm64|aarch64)/.test(lower)) return 'arm64';
    if (/(x86_64|amd64|win64|x64)/.test(lower)) return 'x64';
    if (/(i386|i686|win32|x86)/.test(lower)) return 'x86';
    return 'x64';
  }

  async function uploadFiles(event, purpose) {
    const input = event.currentTarget;
    const files = Array.from(input.files || []);
    input.value = '';
    if (!files.length || !document || uploading) return;
    uploading = purpose;
    let added = false;
    try {
      for (const file of files) {
        const asset = await onUpload?.(file, purpose);
        if (!asset) continue;
        const m = ensureMetadata();
        if (purpose === 'package') {
          uploaded = { ...uploaded, [asset.id]: asset };
          m.files.push({ assetId: asset.id, name: file.name, os: guessOS(file.name), arch: guessArch(file.name), label: '' });
        } else {
          m.screenshots.push({ assetId: asset.id, caption: '' });
        }
        added = true;
      }
    } catch (error) {
      onError?.(error);
    } finally {
      uploading = '';
      if (added) changed();
    }
  }

  async function uploadIcon(event) {
    const input = event.currentTarget;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !document || uploading) return;
    uploading = 'cover';
    try {
      const asset = await onUpload?.(file, 'cover');
      if (asset) {
        const m = ensureMetadata();
        m.coverAssetId = asset.id;
        m.coverName = file.name;
        changed();
      }
    } catch (error) {
      onError?.(error);
    } finally {
      uploading = '';
    }
  }

  function removeAt(field, index) {
    ensureMetadata()[field].splice(index, 1);
    changed();
  }

  function moveAt(field, index, delta) {
    const entries = ensureMetadata()[field];
    const target = index + delta;
    if (target < 0 || target >= entries.length) return;
    const [entry] = entries.splice(index, 1);
    entries.splice(target, 0, entry);
    changed();
  }

  function setField(key, value) {
    ensureMetadata()[key] = value;
    changed();
  }

  function setTags(value) {
    document.tags = value.split(',').map((tag) => tag.trim()).filter(Boolean).slice(0, 50);
    changed();
  }

  function fileMeta(file) {
    const asset = uploaded[file.assetId];
    if (!asset) return '';
    const mb = asset.sizeBytes / (1024 * 1024);
    const size = mb >= 1 ? `${mb.toFixed(mb >= 10 ? 0 : 1)} MB` : `${Math.max(1, Math.round(asset.sizeBytes / 1024))} KB`;
    return `${size} · sha ${String(asset.sha256 || '').slice(0, 8)}`;
  }

  // Matriz de la fila de Depot: qué sistemas cubre lo subido hasta ahora.
  const covers = (key) => (metadata().files || []).some((file) =>
    key === 'arm' ? file.arch === 'arm64' : file.os === key);
</script>

<div class="depot-layout">
  <section class="depot-form">
    <section class="repeat-field" data-studio-section="files">
      <header>
        <span><b>{t('studio.depot.files')}</b><small>{t('studio.depot.filesHint')}</small></span>
        <button onclick={() => fileInput?.click()} disabled={!!uploading}>
          ＋ {uploading === 'package' ? t('studio.uploadingFile') : t('studio.depot.addFiles')}
        </button>
      </header>
      <input bind:this={fileInput} class="hidden-input" type="file" multiple
        accept=".exe,.msi,.deb,.zip,.rar" onchange={(event) => uploadFiles(event, 'package')} />
      {#each metadata().files || [] as file, index (file.assetId)}
        <div class="file-row" class:missing={!file.os}>
          <span class="format">{extensionOf(file.name) || '?'}</span>
          <span class="file-name">
            <b>{file.name || t('studio.assetReady')}</b>
            {#if !file.os}<small class="warn">{t('studio.depot.needsSystem')}</small>
            {:else if fileMeta(file)}<small>{fileMeta(file)}</small>{/if}
          </span>
          <select value={file.os} aria-label={t('studio.depot.system')}
            onchange={(event) => { file.os = event.currentTarget.value; changed(); }}>
            <option value="">{t('studio.depot.chooseSystem')}</option>
            {#each OS as os (os.k)}<option value={os.k}>{os.name}</option>{/each}
          </select>
          <select value={file.arch} aria-label={t('studio.depot.arch')}
            onchange={(event) => { file.arch = event.currentTarget.value; changed(); }}>
            {#each ARCH as arch (arch)}<option value={arch}>{arch}</option>{/each}
          </select>
          <input class="label-input" value={file.label || ''} aria-label={t('studio.depot.fileLabel')}
            placeholder={t('studio.depot.fileLabelPlaceholder')}
            oninput={(event) => { file.label = event.currentTarget.value; changed(); }} />
          <span class="entry-actions">
            <button onclick={() => moveAt('files', index, -1)} disabled={index === 0} aria-label={t('studio.moveUp')}>↑</button>
            <button onclick={() => moveAt('files', index, 1)} disabled={index === metadata().files.length - 1} aria-label={t('studio.moveDown')}>↓</button>
            <button class="remove" onclick={() => removeAt('files', index)} aria-label={t('studio.removeEntry')}>×</button>
          </span>
        </div>
      {/each}
      {#if (metadata().files || []).length === 0}<p>{t('studio.depot.noFiles')}</p>{/if}
    </section>

    <div class="fields-two" data-studio-section="metadata">
      <label>{t('studio.documentTitle')}<input value={document.title} oninput={(event) => { document.title = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.author')}<input value={document.authorLabel || ''} oninput={(event) => { document.authorLabel = event.currentTarget.value; changed(); }} /></label>
      <label>{t('studio.depot.version')}<input class="mono" value={metadata().version || ''} placeholder="1.0" oninput={(event) => setField('version', event.currentTarget.value)} /></label>
      <label>{t('studio.license')}<input class="mono" value={metadata().license || ''} placeholder="GPL-3.0" oninput={(event) => setField('license', event.currentTarget.value)} /></label>
      <label>{t('studio.depot.shelf')}
        <select value={metadata().shelf || ''} onchange={(event) => setField('shelf', event.currentTarget.value)}>
          <option value="">{t('studio.depot.chooseShelf')}</option>
          {#each SHELVES as shelf (shelf)}<option value={shelf}>{t('studio.depot.shelf.' + shelf)}</option>{/each}
        </select>
      </label>
      <label>{t('studio.depot.website')}<input class="mono" value={metadata().website || ''} oninput={(event) => setField('website', event.currentTarget.value)} /></label>
      <label>{t('studio.depot.requirements')}<input value={metadata().requirements || ''} oninput={(event) => setField('requirements', event.currentTarget.value)} /></label>
      <label>{t('studio.depot.languages')}<input value={metadata().languages || ''} oninput={(event) => setField('languages', event.currentTarget.value)} /></label>
      <label>{t('studio.collection')}<input value={metadata().collection || 'General'} oninput={(event) => setField('collection', event.currentTarget.value)} /></label>
      <label>{t('studio.tags')}<input value={(document.tags || []).join(', ')} placeholder={t('studio.tagsPlaceholder')} oninput={(event) => setTags(event.currentTarget.value)} /></label>
    </div>
    <label>{t('studio.depot.description')}
      <textarea rows="4" value={document.summary || ''} oninput={(event) => { document.summary = event.currentTarget.value; changed(); }}></textarea>
    </label>
    <label>{t('studio.depot.notes')}
      <textarea rows="2" value={metadata().notes || ''} oninput={(event) => setField('notes', event.currentTarget.value)}></textarea>
    </label>

    <section class="repeat-field" data-studio-section="screenshots">
      <header>
        <span><b>{t('studio.depot.screenshots')}</b><small>{t('studio.depot.screenshotsHint')}</small></span>
        <button onclick={() => shotInput?.click()} disabled={!!uploading}>
          ＋ {uploading === 'screenshot' ? t('studio.uploadingFile') : t('studio.depot.addScreenshots')}
        </button>
      </header>
      <input bind:this={shotInput} class="hidden-input" type="file" multiple
        accept=".jpg,.jpeg,.png,.gif,.webp,image/*" onchange={(event) => uploadFiles(event, 'screenshot')} />
      <div class="shots">
        {#each metadata().screenshots || [] as shot, index (shot.assetId)}
          <figure class="shot">
            <StudioImage documentId={document.id} assetId={shot.assetId} alt={shot.caption || t('studio.depot.screenshots')} compact />
            <input value={shot.caption || ''} aria-label={t('studio.depot.caption')} placeholder={t('studio.depot.caption')}
              oninput={(event) => { shot.caption = event.currentTarget.value; changed(); }} />
            <span class="entry-actions">
              <button onclick={() => moveAt('screenshots', index, -1)} disabled={index === 0} aria-label={t('studio.moveUp')}>←</button>
              <button onclick={() => moveAt('screenshots', index, 1)} disabled={index === metadata().screenshots.length - 1} aria-label={t('studio.moveDown')}>→</button>
              <button class="remove" onclick={() => removeAt('screenshots', index)} aria-label={t('studio.removeEntry')}>×</button>
            </span>
          </figure>
        {/each}
      </div>
    </section>
  </section>

  <aside class="depot-preview" data-studio-section="cover">
    <span class="aside-label">{t('studio.depot.icon')}</span>
    <button class="icon-upload" onclick={() => iconInput?.click()} disabled={!!uploading}>
      {#if metadata().coverAssetId}
        <StudioImage documentId={document.id} assetId={metadata().coverAssetId} alt={t('studio.depot.icon')} compact />
      {:else}
        <b>＋</b><small>{uploading === 'cover' ? t('studio.uploadingFile') : t('studio.depot.addIcon')}</small>
      {/if}
    </button>
    <input bind:this={iconInput} class="hidden-input" type="file" accept=".jpg,.jpeg,.png,.gif,.webp,image/*" onchange={uploadIcon} />

    <span class="aside-label">{t('studio.depot.preview')}</span>
    <div class="preview-row">
      <span class="preview-icon">
        {#if metadata().coverAssetId}
          <StudioImage documentId={document.id} assetId={metadata().coverAssetId} alt="" compact />
        {:else}{(document.title || '?').charAt(0)}{/if}
      </span>
      <span class="preview-copy">
        <b>{document.title}</b>
        <small class="mono">{metadata().version || ''} {metadata().license || ''}</small>
      </span>
      <span class="matrix" aria-hidden="true">
        {#each ['win', 'linux', 'arm'] as key (key)}
          <span class="cell"><i class:on={covers(key)}></i><small>{key === 'linux' ? 'lnx' : key}</small></span>
        {/each}
      </span>
    </div>
    <p class="preview-hint">{t('studio.previewContract')}</p>
  </aside>
</div>

<style>
  .depot-layout{max-width:1000px;margin:0 auto;display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:28px;align-items:start}
  .depot-form{display:grid;gap:15px}
  .depot-form label{display:flex;flex-direction:column;gap:5px;color:var(--muted);font-size:10px;letter-spacing:.04em}
  .depot-form input,.depot-form textarea,.depot-form select{width:100%;padding:9px 10px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card);color:var(--ink);outline:0;font:inherit;font-size:12px}
  .depot-form input:focus,.depot-form textarea:focus,.depot-form select:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .mono{font-family:var(--mono)}
  .fields-two{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px}
  .repeat-field{display:grid;gap:8px;padding:12px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel)}
  .repeat-field header{display:flex;align-items:center;justify-content:space-between;gap:12px}
  .repeat-field header>span{display:flex;flex-direction:column;gap:2px}.repeat-field header b{font-size:11px}
  .repeat-field header small,.repeat-field>p{color:var(--faint);font-size:9.5px}
  .repeat-field header button{padding:6px 8px;border-radius:var(--r-sm);background:var(--card);color:var(--accent-2);font-size:10px}
  /* Fila que se parte sola: el nombre manda y los controles bajan a otra línea
     cuando no caben. Con columnas fijas el nombre se quedaba en una letra por
     línea en cuanto el editor se estrechaba. */
  .file-row{display:flex;flex-wrap:wrap;gap:8px;align-items:center;padding:8px;border-radius:var(--r-md);background:var(--card)}
  .file-row .format{flex:none;width:40px}
  .file-row .file-name{flex:1 1 220px}
  .file-row select{flex:0 0 108px;width:auto}
  .file-row select+select{flex-basis:84px}
  .file-row .label-input{flex:1 1 140px;width:auto}
  .file-row .entry-actions{flex:none;margin-left:auto}
  .file-row.missing{box-shadow:inset 0 0 0 1px color-mix(in srgb,#e0a030 50%,var(--border))}
  .file-row select,.file-row .label-input{padding:6px 8px;font-size:11px}
  .file-row.missing select:first-of-type{border-color:color-mix(in srgb,#e0a030 60%,var(--border))}
  .format{display:grid;place-items:center;height:22px;border:1px solid var(--border);border-radius:min(3px,var(--r-sm));color:var(--muted);font:700 10px var(--mono);letter-spacing:.06em;text-transform:uppercase}
  .file-name{min-width:0;display:flex;flex-direction:column;gap:2px}
  .file-name b{font:500 11.5px var(--mono);overflow-wrap:anywhere}
  .file-name small{color:var(--faint);font:10px var(--mono)}
  .file-name .warn{color:color-mix(in srgb,#e0a030 75%,var(--ink))}
  .entry-actions{display:flex;align-items:center;gap:2px}
  .entry-actions button{width:26px;height:28px;border-radius:var(--r-sm);color:var(--faint)}
  .entry-actions button:hover:not(:disabled){background:var(--raise);color:var(--ink)}.entry-actions button:disabled{opacity:.28}
  .entry-actions .remove:hover{background:color-mix(in srgb,#df7474 12%,transparent)!important;color:#df7474!important}
  .shots{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:10px}
  .shot{margin:0;display:grid;gap:6px;padding:6px;border-radius:var(--r-md);background:var(--card)}
  .shot :global(img),.shot :global(.placeholder){width:100%;aspect-ratio:16/10;height:auto;min-height:0;object-fit:cover;border-radius:var(--r-sm)}
  .shot input{padding:6px 8px;font-size:11px}
  .depot-preview{position:sticky;top:0;display:grid;gap:10px}
  .aside-label{color:var(--faint);font-size:9px;letter-spacing:.13em;text-transform:uppercase}
  .icon-upload{width:96px;height:96px;overflow:hidden;display:grid;place-items:center;border:1px dashed var(--border);border-radius:var(--r-lg);background:var(--card);color:var(--faint)}
  .icon-upload:hover{border-color:var(--accent-line);color:var(--ink)}
  .icon-upload b{font-size:20px}.icon-upload small{display:block;font-size:9.5px}
  .icon-upload :global(img),.icon-upload :global(.placeholder){width:96px;height:96px;min-height:0;object-fit:contain;border-radius:0}
  .preview-row{display:flex;align-items:center;gap:10px;padding:10px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel)}
  .preview-icon{width:40px;height:40px;flex:none;overflow:hidden;display:grid;place-items:center;border-radius:var(--r-md);background:var(--raise);color:var(--ink);font-weight:700}
  .preview-icon :global(img),.preview-icon :global(.placeholder){width:40px;height:40px;min-height:0;object-fit:contain}
  .preview-copy{min-width:0;flex:1;display:flex;flex-direction:column;gap:2px}
  .preview-copy b{font-size:12.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .preview-copy small{color:var(--link);font-size:10px}
  .matrix{display:flex;gap:6px}
  .matrix .cell{display:flex;flex-direction:column;align-items:center;gap:3px}
  .matrix i{width:11px;height:11px;border:1px dashed var(--border);border-radius:min(2px,var(--r-sm))}
  .matrix i.on{border-style:solid;border-color:var(--accent);background:var(--accent)}
  .matrix small{color:var(--muted);font:9px var(--mono)}
  .preview-hint{margin:0;color:var(--faint);font-size:10.5px;line-height:1.5}
  .hidden-input{display:none}
  @media(max-width:850px){.depot-layout{grid-template-columns:1fr}.depot-preview{position:static}}
  @media(max-width:620px){.fields-two{grid-template-columns:1fr}}
</style>
