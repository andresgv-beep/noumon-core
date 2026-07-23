<script>
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioDocumentView from './StudioDocumentView.svelte';
  import { t, relTime } from './i18n.svelte.js';
  import {
    saveStudioRecovery, loadStudioRecovery, clearStudioRecovery,
  } from './studioRecovery.js';
  import { itemSearch } from './libraryApi.js';
  import {
    listStudioDocuments, getStudioDocument, createStudioDocument,
    updateStudioDocument, archiveStudioDocument, publishStudioDocument,
    unpublishStudioDocument, getStudioCapabilities, uploadStudioAsset,
    listStudioRevisions, restoreStudioRevision,
  } from './studioApi.js';

  let { onOpenItem } = $props();

  let documents = $state([]);
  let selected = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let saved = $state(false);
  let offline = $state(false);
  let error = $state('');
  let dirty = $state(false);
  let canPublish = $state(false);
  let uploadingImage = $state(false);
  let showRevisions = $state(false);
  let revisions = $state([]);
  let revisionsLoading = $state(false);
  let restoringRevision = $state(null);
  let linkPicker = $state(false);
  let linkQuery = $state('');
  let linkResults = $state([]);
  let linkLoading = $state(false);
  let imageInput = $state(null);
  let saveTimer;
  let savedTimer;
  let savePromise = null;
  let changeVersion = 0;
  let openingSequence = 0;
  let linkSearchTimer;
  let linkAbort;
  let blockSequence = 1;

  const content = () => selected?.content || { schemaVersion: 1, presentation: {}, classification: {}, blocks: [] };

  onMount(() => {
    load();
    const beforeUnload = (event) => {
      if (!dirty) return;
      event.preventDefault();
      event.returnValue = '';
    };
    const keydown = (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault();
        saveNow();
      }
    };
    window.addEventListener('beforeunload', beforeUnload);
    window.addEventListener('keydown', keydown);
    return () => {
      clearTimeout(saveTimer);
      clearTimeout(savedTimer);
      clearTimeout(linkSearchTimer);
      linkAbort?.abort();
      window.removeEventListener('beforeunload', beforeUnload);
      window.removeEventListener('keydown', keydown);
    };
  });

  function normalizeDocument(doc) {
    doc.content = typeof doc.content === 'string' ? JSON.parse(doc.content) : doc.content;
    doc.metadata = typeof doc.metadata === 'string' ? JSON.parse(doc.metadata) : doc.metadata;
    return doc;
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const capabilities = await getStudioCapabilities();
      canPublish = !!capabilities.canPublish;
      documents = await listStudioDocuments('all');
      const first = documents.find((item) => item.status === 'draft') || documents[0];
      if (first) await openDocument(first.id);
    } catch (e) {
      error = e.code || e.message;
    }
    loading = false;
  }

  function blankContent() {
    return {
      schemaVersion: 1,
      classification: { workType: 'article', topics: [], audience: [] },
      presentation: { contentWidth: 'reading', fontPreset: 'editorial' },
      blocks: [
        { id: nextBlockId(), type: 'heading', level: 1, text: t('studio.untitled') },
        { id: nextBlockId(), type: 'paragraph', text: '' },
      ],
    };
  }

  function nextBlockId() {
    return `block-${Date.now().toString(36)}-${blockSequence++}`;
  }

  async function newDocument() {
    if (!await flushCurrent()) return;
    error = '';
    try {
      const doc = normalizeDocument(await createStudioDocument({
        templateKey: 'document',
        title: t('studio.untitled'),
        language: '',
        tags: [],
        metadata: {},
        content: blankContent(),
      }));
      openingSequence++;
      documents = [{ ...doc }, ...documents];
      selected = doc;
      dirty = false;
      offline = false;
      changeVersion = 0;
      revisions = [];
      closeLinkPicker();
      if (showRevisions) loadRevisions(doc.id);
    } catch (e) {
      error = e.code || e.message;
    }
  }

  async function openDocument(id) {
    if (selected?.id === id) return;
    if (!await flushCurrent()) return;
    const requestSequence = ++openingSequence;
    error = '';
    try {
      const doc = normalizeDocument(await getStudioDocument(id));
      if (requestSequence !== openingSequence) return;
      selected = doc;
      dirty = false;
      offline = false;
      changeVersion = 0;
      revisions = [];
      closeLinkPicker();
      const recovery = await loadStudioRecovery(id);
      if (requestSequence !== openingSequence || selected?.id !== id) return;
      if (recovery?.document && recovery.baseRevision === doc.revision) {
        selected = normalizeDocument(recovery.document);
        selected.revision = doc.revision;
        dirty = true;
        changeVersion++;
        error = 'studio.recovered';
        scheduleSave();
      }
      if (showRevisions) loadRevisions(id);
    } catch (e) {
      error = e.code || e.message;
    }
  }

  function touch() {
    if (!selected || selected.status === 'archived') return;
    dirty = true;
    offline = false;
    saved = false;
    changeVersion++;
    saveStudioRecovery(selected);
    scheduleSave();
  }

  function scheduleSave(delay = 1200) {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(saveNow, delay);
  }

  function documentInput(document) {
    return {
      templateKey: document.templateKey,
      title: document.title.trim() || t('studio.untitled'),
      summary: document.summary || '',
      language: document.language || '',
      authorLabel: document.authorLabel || '',
      tags: Array.isArray(document.tags) ? document.tags : [],
      metadata: document.metadata || {},
      content: document.content,
      baseRevision: document.revision,
    };
  }

  async function saveNow() {
    clearTimeout(saveTimer);
    if (savePromise) {
      const previousOK = await savePromise;
      if (previousOK && dirty) return saveNow();
      return previousOK && !dirty;
    }
    if (!selected || !dirty || selected.status === 'archived') return true;

    const documentId = selected.id;
    const version = changeVersion;
    const input = JSON.parse(JSON.stringify(documentInput(selected)));
    saving = true;
    offline = false;
    error = '';
    savePromise = (async () => {
      try {
        const updated = normalizeDocument(await updateStudioDocument(documentId, input));
        documents = documents.map((item) => item.id === updated.id ? { ...item, ...updated } : item);
        if (showRevisions) loadRevisions(documentId);
        if (selected?.id !== documentId) return true;

        if (changeVersion === version) {
          selected = updated;
          dirty = false;
          saved = true;
          await clearStudioRecovery(documentId);
          clearTimeout(savedTimer);
          savedTimer = setTimeout(() => { saved = false; }, 1800);
        } else {
          // El servidor ha guardado la instantánea enviada, pero el usuario
          // siguió escribiendo durante la petición. Conservamos esos cambios y
          // solo adelantamos su baseRevision para el siguiente guardado.
          selected.revision = updated.revision;
          selected.updated = updated.updated;
          dirty = true;
          saveStudioRecovery(selected);
          scheduleSave(0);
        }
        return true;
      } catch (e) {
        offline = !e.status;
        if (e.status === 409) error = 'studio.conflict';
        else if (offline) error = 'studio.offline';
        else error = e.code || e.message;
        return false;
      } finally {
        saving = false;
        savePromise = null;
      }
    })();
    return savePromise;
  }

  async function flushCurrent() {
    clearTimeout(saveTimer);
    if (savePromise && !await savePromise) return false;
    if (!dirty) return true;
    return saveNow();
  }

  async function loadRevisions(documentId = selected?.id) {
    if (!documentId) return;
    revisionsLoading = true;
    try {
      const loaded = await listStudioRevisions(documentId);
      if (selected?.id === documentId) revisions = loaded;
    } catch (e) {
      if (selected?.id === documentId) error = e.code || e.message;
    } finally {
      if (selected?.id === documentId) revisionsLoading = false;
    }
  }

  function toggleRevisions() {
    showRevisions = !showRevisions;
    if (showRevisions) loadRevisions();
  }

  async function restoreRevision(revision) {
    if (!selected || restoringRevision || revision.revision === selected.revision) return;
    if (!confirm(t('studio.restoreConfirm', { revision: revision.revision }))) return;
    if (!await flushCurrent()) return;
    const documentId = selected.id;
    restoringRevision = revision.revision;
    error = '';
    try {
      const restored = normalizeDocument(await restoreStudioRevision(
        documentId, revision.revision, selected.revision,
      ));
      if (selected?.id !== documentId) return;
      selected = restored;
      documents = documents.map((item) =>
        item.id === restored.id ? { ...item, ...restored } : item);
      dirty = false;
      offline = false;
      changeVersion = 0;
      saved = true;
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => { saved = false; }, 1800);
      await clearStudioRecovery(documentId);
      await loadRevisions(documentId);
    } catch (e) {
      if (e.status === 409) error = 'studio.conflict';
      else error = e.code || e.message;
    } finally {
      restoringRevision = null;
    }
  }

  function addBlock(type) {
    const block = { id: nextBlockId(), type };
    if (type === 'heading') Object.assign(block, { level: 2, text: t('studio.headingPlaceholder') });
    else if (type === 'bulletList' || type === 'orderedList') block.items = [t('studio.listPlaceholder')];
    else if (type === 'table') block.rows = [[t('studio.tableHeader'), t('studio.tableHeader')], ['', '']];
    else if (type === 'callout') Object.assign(block, { title: '', text: '' });
    else if (type === 'divider') {}
    else block.text = '';
    selected.content.blocks.push(block);
    touch();
  }

  function toggleLinkPicker() {
    if (linkPicker) closeLinkPicker();
    else linkPicker = true;
  }

  function closeLinkPicker() {
    linkPicker = false;
    clearTimeout(linkSearchTimer);
    linkAbort?.abort();
    linkQuery = '';
    linkResults = [];
    linkLoading = false;
  }

  function searchLinkTargets(value) {
    linkQuery = value;
    clearTimeout(linkSearchTimer);
    linkAbort?.abort();
    const query = value.trim();
    if (query.length < 2) {
      linkResults = [];
      linkLoading = false;
      return;
    }
    linkLoading = true;
    linkSearchTimer = setTimeout(async () => {
      linkAbort = new AbortController();
      try {
        const results = await itemSearch(query, { signal: linkAbort.signal });
        if (linkQuery.trim() === query) {
          linkResults = results
            .filter((item) => item.itemId !== `studio:${selected?.id}`)
            .slice(0, 12);
        }
      } catch (e) {
        if (e?.name !== 'AbortError' && linkQuery.trim() === query) linkResults = [];
      } finally {
        if (linkQuery.trim() === query) linkLoading = false;
      }
    }, 250);
  }

  function insertItemReference(item) {
    if (!selected || !item?.itemId) return;
    selected.content.blocks.push({
      id: nextBlockId(),
      type: 'itemRef',
      itemId: item.itemId,
      titleSnapshot: item.title || item.itemId,
      kindSnapshot: item.kind || 'item',
    });
    touch();
    closeLinkPicker();
  }

  function chooseImage() {
    if (!selected || selected.status === 'archived' || uploadingImage) return;
    imageInput?.click();
  }

  async function imageSelected(event) {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = '';
    if (!file || !selected) return;
    const documentId = selected.id;
    uploadingImage = true;
    error = '';
    try {
      const asset = await uploadStudioAsset(documentId, file);
      if (selected?.id !== documentId) return;
      selected.content.blocks.push({
        id: nextBlockId(), type: 'image', assetId: asset.id,
        caption: '', alt: '',
      });
      touch();
    } catch (e) {
      error = e.code || e.message;
    } finally {
      uploadingImage = false;
    }
  }

  function removeBlock(index) {
    selected.content.blocks.splice(index, 1);
    touch();
  }

  function moveBlock(index, delta) {
    const target = index + delta;
    if (target < 0 || target >= selected.content.blocks.length) return;
    const [block] = selected.content.blocks.splice(index, 1);
    selected.content.blocks.splice(target, 0, block);
    touch();
  }

  function listText(block) {
    return (block.items || []).join('\n');
  }

  function setListText(block, value) {
    block.items = value.split('\n').slice(0, 500);
    touch();
  }

  function tableText(block) {
    return (block.rows || []).map((row) => row.join(' | ')).join('\n');
  }

  function setTableText(block, value) {
    block.rows = value.split('\n').slice(0, 100).map((row) => row.split('|').slice(0, 20).map((cell) => cell.trim()));
    touch();
  }

  function tagsText() {
    return (selected?.tags || []).join(', ');
  }

  function setTags(value) {
    selected.tags = value.split(',').map((tag) => tag.trim()).filter(Boolean).slice(0, 50);
    touch();
  }

  async function archiveSelected() {
    if (!selected) return;
    if (!await flushCurrent()) return;
    try {
      const archived = await archiveStudioDocument(selected.id);
      selected.status = archived.status;
      selected.revision = archived.revision;
      documents = documents.map((item) => item.id === selected.id ? { ...item, ...archived } : item);
      dirty = false;
      await clearStudioRecovery(selected.id);
      if (showRevisions) loadRevisions(selected.id);
    } catch (e) {
      error = e.code || e.message;
    }
  }

  async function togglePublication() {
    if (!selected || !canPublish) return;
    if (!await flushCurrent()) return;
    try {
      const updated = selected.publishedRevision
        ? await unpublishStudioDocument(selected.id)
        : await publishStudioDocument(selected.id);
      selected = { ...selected, ...updated };
      documents = documents.map((item) => item.id === updated.id ? { ...item, ...updated } : item);
    } catch (e) {
      error = e.code || e.message;
    }
  }
</script>

<section class="studio">
  <aside class="drafts">
    <div class="draft-head">
      <div>
        <span class="eyebrow">{t('studio.workspace')}</span>
        <h1>{t('studio.title')}</h1>
      </div>
      <button class="primary icon-only" title={t('studio.newDocument')} onclick={newDocument}>
        <Icon name="plus" size={17} />
      </button>
    </div>
    <button class="new-wide" onclick={newDocument}><Icon name="plus" size={15} /> {t('studio.newDocument')}</button>
    <div class="draft-list scroll thin">
      {#if loading}
        <div class="empty">{t('common.loading')}</div>
      {:else if documents.length === 0}
        <div class="empty">{t('studio.empty')}</div>
      {/if}
      {#each documents as doc (doc.id)}
        <button class="draft" class:active={selected?.id === doc.id} onclick={() => openDocument(doc.id)}>
          <Icon name={doc.status === 'archived' ? 'trash' : 'note'} size={16} />
          <span>
            <b>{doc.title}</b>
            <small>{doc.status === 'archived' ? t('studio.archived') : relTime(doc.updated)}</small>
          </span>
        </button>
      {/each}
    </div>
  </aside>

  {#if selected}
    <main class="editor scroll thin">
      <div class="editor-top">
        <div class="save-state">
          {#if saving}{t('common.saving')}{:else if offline}{t('studio.offline')}{:else if saved}<Icon name="check" size={13} /> {t('studio.saved')}{:else if dirty}{t('studio.unsaved')}{:else}{t('studio.draft')}{/if}
        </div>
        <div class="top-actions">
          <button class:active={showRevisions} onclick={toggleRevisions}>{t('studio.revisions')}</button>
          <button onclick={saveNow} disabled={!dirty || saving}>{t('common.save')}</button>
          {#if canPublish}
            <button class="publish" onclick={togglePublication} disabled={saving || selected.status === 'archived'}>
              {selected.publishedRevision ? t('studio.unpublish') : t('studio.publish')}
            </button>
          {/if}
          <button class="danger" onclick={archiveSelected} disabled={selected.status === 'archived'}>{t('studio.archive')}</button>
        </div>
      </div>

      {#if error}
        <div class="error">{t(error)}</div>
      {/if}

      {#if showRevisions}
        <section class="revision-panel">
          <div class="revision-head">
            <b>{t('studio.revisions')}</b>
            <span>{revisions.length}</span>
          </div>
          {#if revisionsLoading}
            <div class="revision-empty">{t('common.loading')}</div>
          {:else if revisions.length === 0}
            <div class="revision-empty">{t('studio.revisionsEmpty')}</div>
          {:else}
            <div class="revision-list">
              {#each revisions as revision (revision.revision)}
                <div class="revision-row">
                  <div>
                    <b>{t('studio.revisionNumber', { revision: revision.revision })}</b>
                    <span>{revision.title}</span>
                    <small>
                      {revision.editorLabel || t('documents.localAuthor')} · {relTime(revision.created)}
                    </small>
                  </div>
                  <div class="revision-actions">
                    {#if revision.revision === selected.revision}
                      <span class="revision-badge">{t('studio.revisionCurrent')}</span>
                    {:else if revision.revision === selected.publishedRevision}
                      <span class="revision-badge published-badge">{t('studio.revisionPublished')}</span>
                    {/if}
                    <button
                      disabled={restoringRevision || revision.revision === selected.revision}
                      onclick={() => restoreRevision(revision)}
                    >
                      {restoringRevision === revision.revision ? t('common.saving') : t('studio.restore')}
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </section>
      {/if}

      <div class="meta-card">
        <input class="title-input" value={selected.title} oninput={(e) => { selected.title = e.currentTarget.value; touch(); }} aria-label={t('studio.documentTitle')} />
        <textarea class="summary" rows="2" placeholder={t('studio.summaryPlaceholder')} value={selected.summary || ''} oninput={(e) => { selected.summary = e.currentTarget.value; touch(); }}></textarea>
        <div class="meta-row">
          <label>{t('studio.author')}<input value={selected.authorLabel || ''} oninput={(e) => { selected.authorLabel = e.currentTarget.value; touch(); }} /></label>
          <label>{t('studio.language')}<input value={selected.language || ''} placeholder="es" oninput={(e) => { selected.language = e.currentTarget.value; touch(); }} /></label>
          <label>{t('studio.tags')}<input value={tagsText()} placeholder={t('studio.tagsPlaceholder')} oninput={(e) => setTags(e.currentTarget.value)} /></label>
        </div>
        <div class="meta-row">
          <label>{t('studio.width')}
            <select value={content().presentation?.contentWidth || 'reading'} onchange={(e) => { selected.content.presentation.contentWidth = e.currentTarget.value; touch(); }}>
              <option value="reading">{t('studio.widthReading')}</option>
              <option value="compact">{t('studio.widthCompact')}</option>
              <option value="wide">{t('studio.widthWide')}</option>
              <option value="editorial">{t('studio.widthEditorial')}</option>
            </select>
          </label>
          <label>{t('studio.typography')}
            <select value={content().presentation?.fontPreset || 'editorial'} onchange={(e) => { selected.content.presentation.fontPreset = e.currentTarget.value; touch(); }}>
              <option value="editorial">{t('studio.fontEditorial')}</option>
              <option value="sans">{t('studio.fontSans')}</option>
            </select>
          </label>
          <label>{t('studio.workType')}<input value={content().classification?.workType || ''} placeholder="article" oninput={(e) => { selected.content.classification.workType = e.currentTarget.value; touch(); }} /></label>
        </div>
      </div>

      <div class="block-toolbar">
        <span>{t('studio.addBlock')}</span>
        <button onclick={() => addBlock('heading')}>H</button>
        <button onclick={() => addBlock('paragraph')}>¶</button>
        <button onclick={() => addBlock('quote')}>❝</button>
        <button onclick={() => addBlock('bulletList')}>• {t('studio.list')}</button>
        <button onclick={() => addBlock('orderedList')}>1. {t('studio.list')}</button>
        <button onclick={() => addBlock('table')}>{t('studio.table')}</button>
        <button onclick={() => addBlock('code')}>{t('studio.code')}</button>
        <button onclick={() => addBlock('callout')}>{t('studio.callout')}</button>
        <button class:active={linkPicker} onclick={toggleLinkPicker}>{t('studio.internalLink')}</button>
        <button onclick={chooseImage} disabled={uploadingImage}>
          <Icon name="image" size={14} />
          {uploadingImage ? t('studio.uploadingImage') : t('studio.image')}
        </button>
        <button onclick={() => addBlock('divider')}>—</button>
        <input class="file-input" bind:this={imageInput} type="file" accept=".jpg,.jpeg,.png,.gif,.webp,image/jpeg,image/png,image/gif,image/webp" onchange={imageSelected} />
      </div>

      {#if linkPicker}
        <div class="link-picker">
          <input
            value={linkQuery}
            placeholder={t('studio.internalLinkSearch')}
            aria-label={t('studio.internalLinkSearch')}
            oninput={(event) => searchLinkTargets(event.currentTarget.value)}
          />
          {#if linkLoading}
            <div class="link-empty">{t('common.loading')}</div>
          {:else if linkQuery.trim().length < 2}
            <div class="link-empty">{t('studio.internalLinkHint')}</div>
          {:else if linkResults.length === 0}
            <div class="link-empty">{t('studio.internalLinkEmpty')}</div>
          {:else}
            <div class="link-results">
              {#each linkResults as item (item.itemId)}
                <button onclick={() => insertItemReference(item)}>
                  <span>{item.title}</span>
                  <small>{item.kind}{#if item.subtitle} · {item.subtitle}{/if}</small>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <div class="blocks">
        {#each content().blocks as block, index (block.id)}
          <article class="block">
            <div class="block-controls">
              <span>{t(`studio.block.${block.type}`)}</span>
              <button title={t('studio.moveUp')} onclick={() => moveBlock(index, -1)}>↑</button>
              <button title={t('studio.moveDown')} onclick={() => moveBlock(index, 1)}>↓</button>
              <button title={t('studio.removeBlock')} onclick={() => removeBlock(index)}><Icon name="trash" size={14} /></button>
            </div>
            {#if block.type === 'heading'}
              <div class="heading-edit">
                <select value={block.level || 2} onchange={(e) => { block.level = Number(e.currentTarget.value); touch(); }}>
                  <option value="1">H1</option><option value="2">H2</option><option value="3">H3</option>
                </select>
                <input value={block.text || ''} oninput={(e) => { block.text = e.currentTarget.value; touch(); }} />
              </div>
            {:else if block.type === 'bulletList' || block.type === 'orderedList'}
              <textarea rows="4" value={listText(block)} placeholder={t('studio.oneItemPerLine')} oninput={(e) => setListText(block, e.currentTarget.value)}></textarea>
            {:else if block.type === 'table'}
              <textarea rows="5" value={tableText(block)} placeholder={t('studio.tableHelp')} oninput={(e) => setTableText(block, e.currentTarget.value)}></textarea>
            {:else if block.type === 'image'}
              <div class="image-editor">
                <StudioImage documentId={selected.id} assetId={block.assetId} alt={block.alt || ''} compact />
                <div class="image-fields">
                  <input value={block.caption || ''} placeholder={t('studio.imageCaption')} oninput={(e) => { block.caption = e.currentTarget.value; touch(); }} />
                  <input value={block.alt || ''} placeholder={t('studio.imageAlt')} oninput={(e) => { block.alt = e.currentTarget.value; touch(); }} />
                </div>
              </div>
            {:else if block.type === 'callout'}
              <div class="callout-fields">
                <input value={block.title || ''} placeholder={t('studio.calloutTitle')} oninput={(e) => { block.title = e.currentTarget.value; touch(); }} />
                <textarea rows="4" value={block.text || ''} placeholder={t('studio.richTextHelp')} oninput={(e) => { block.text = e.currentTarget.value; touch(); }}></textarea>
              </div>
            {:else if block.type === 'code'}
              <textarea class="code-editor" rows="7" value={block.text || ''} placeholder={t('studio.codePlaceholder')} oninput={(e) => { block.text = e.currentTarget.value; touch(); }}></textarea>
            {:else if block.type === 'divider'}
              <hr />
            {:else}
              <textarea rows={block.type === 'quote' ? 3 : 5} value={block.text || ''} placeholder={t('studio.richTextHelp')} oninput={(e) => { block.text = e.currentTarget.value; touch(); }}></textarea>
            {/if}
          </article>
        {/each}
      </div>
    </main>

    <aside class="preview scroll thin">
      <div class="preview-label">{t('studio.preview')}</div>
      <StudioDocumentView document={selected} preview {onOpenItem} />
    </aside>
  {:else}
    <div class="welcome">
      <Icon name="edit" size={30} />
      <h2>{t('studio.welcome')}</h2>
      <p>{t('studio.welcomeBody')}</p>
      <button class="primary" onclick={newDocument}>{t('studio.newDocument')}</button>
      {#if error}<div class="error">{t(error)}</div>{/if}
    </div>
  {/if}
</section>

<style>
  .studio{height:100%;min-width:0;display:grid;grid-template-columns:240px minmax(430px,1fr) minmax(320px,.8fr);background:var(--ground);color:var(--ink)}
  .drafts{min-width:0;background:var(--panel);border-right:1px solid var(--border);display:flex;flex-direction:column;padding:18px 12px 12px}
  .draft-head{display:flex;align-items:flex-start;justify-content:space-between;gap:10px;padding:0 5px 14px}
  .eyebrow,.preview-label{display:block;color:var(--accent-2);font-size:10px;font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{margin:3px 0 0;font-size:21px}
  button{border:1px solid transparent;border-radius:var(--r-md);padding:7px 10px;color:var(--ink-dim);background:var(--raise)}
  button:hover:not(:disabled){color:var(--ink);border-color:var(--border)}
  button:disabled{opacity:.45}
  .primary{background:var(--accent);color:#fff}
  .publish{background:color-mix(in srgb,var(--accent) 22%,var(--panel));color:var(--accent-2);border-color:var(--accent-line)}
  .icon-only{width:32px;height:32px;padding:0;display:grid;place-items:center}
  .new-wide{display:flex;align-items:center;justify-content:center;gap:7px;margin:0 4px 12px}
  .draft-list{display:flex;flex-direction:column;gap:3px;overflow:auto}
  .draft{display:flex;align-items:flex-start;gap:9px;text-align:left;width:100%;background:transparent;padding:10px 9px}
  .draft.active{background:color-mix(in srgb,var(--accent) 14%,var(--panel));border-color:var(--accent-line)}
  .draft span{display:flex;min-width:0;flex-direction:column}
  .draft b{font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .draft small{font-size:11px;color:var(--muted)}
  .empty{padding:18px 9px;color:var(--muted);font-size:13px}
  .editor{overflow:auto;padding:18px clamp(18px,3vw,44px) 60px;border-right:1px solid var(--border)}
  .editor-top{height:32px;display:flex;align-items:center;justify-content:space-between;margin-bottom:14px}
  .save-state{display:flex;align-items:center;gap:5px;font-size:12px;color:var(--muted)}
  .top-actions{display:flex;gap:6px}.danger{color:#df7474}
  .top-actions button.active{border-color:var(--accent-line);color:var(--accent-2);background:color-mix(in srgb,var(--accent) 12%,var(--raise))}
  .error{margin:0 0 12px;padding:9px 11px;border:1px solid color-mix(in srgb,#e45 45%,var(--border));background:color-mix(in srgb,#e45 8%,var(--panel));border-radius:var(--r-md);font-size:12px;color:#e48282}
  .revision-panel{margin:0 0 12px;padding:12px;background:var(--panel);border:1px solid var(--border);border-radius:var(--r-lg)}
  .revision-head{display:flex;align-items:center;justify-content:space-between;color:var(--ink);font-size:12px;margin-bottom:8px}
  .revision-head span{color:var(--muted)}
  .revision-list{display:flex;flex-direction:column;gap:5px;max-height:260px;overflow:auto}
  .revision-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:9px 10px;border-radius:var(--r-md);background:var(--raise)}
  .revision-row>div:first-child{display:flex;min-width:0;flex-direction:column;gap:2px}
  .revision-row b{font-size:12px}.revision-row span{font-size:11px;color:var(--ink-dim);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.revision-row small{font-size:10px;color:var(--faint)}
  .revision-actions{display:flex;align-items:center;gap:6px;flex:none}
  .revision-badge{padding:3px 6px;border-radius:var(--r-pill);background:var(--panel-2);color:var(--muted)!important;font-size:9px!important;text-transform:uppercase}
  .published-badge{color:var(--accent-2)!important;background:color-mix(in srgb,var(--accent) 12%,var(--panel-2))}
  .revision-empty{padding:14px;text-align:center;color:var(--muted);font-size:11px}
  .meta-card,.block{background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow-soft);padding:16px}
  input,textarea,select{width:100%;box-sizing:border-box;background:var(--panel-2);border:1px solid var(--border);border-radius:var(--r-md);padding:8px 10px;color:var(--ink);outline:none}
  input:focus,textarea:focus,select:focus{border-color:var(--accent);box-shadow:0 0 0 2px var(--accent-line)}
  textarea{resize:vertical;line-height:1.5}
  .title-input{font-size:23px;font-weight:700;background:transparent;border-color:transparent;padding-left:2px}
  .summary{margin-top:5px;background:transparent}
  .meta-row{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;margin-top:12px}
  label{font-size:11px;color:var(--muted);display:flex;flex-direction:column;gap:5px}
  .block-toolbar{position:sticky;top:0;z-index:2;margin:17px 0 10px;padding:8px;display:flex;align-items:center;flex-wrap:wrap;gap:5px;background:color-mix(in srgb,var(--panel) 92%,transparent);backdrop-filter:blur(12px);border:1px solid var(--border);border-radius:var(--r-lg)}
  .block-toolbar span{padding:0 7px;font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.06em}
  .block-toolbar button{display:flex;align-items:center;gap:5px}
  .block-toolbar button.active{border-color:var(--accent-line);color:var(--accent-2);background:color-mix(in srgb,var(--accent) 12%,var(--raise))}
  .file-input{display:none}
  .link-picker{margin:-4px 0 12px;padding:10px;background:var(--panel);border:1px solid var(--accent-line);border-radius:var(--r-lg);box-shadow:var(--shadow-soft)}
  .link-results{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:5px;margin-top:8px;max-height:230px;overflow:auto}
  .link-results button{display:flex;min-width:0;flex-direction:column;align-items:flex-start;text-align:left;background:var(--raise)}
  .link-results span{width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--ink);font-size:12px}
  .link-results small{width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--faint);font-size:10px}
  .link-empty{padding:14px;text-align:center;color:var(--muted);font-size:11px}
  .blocks{display:flex;flex-direction:column;gap:10px}
  .block{padding:10px}
  .block-controls{display:flex;align-items:center;justify-content:flex-end;gap:3px;margin-bottom:6px}
  .block-controls span{margin-right:auto;padding-left:3px;font-size:10px;text-transform:uppercase;color:var(--faint)}
  .block-controls button{padding:4px 7px;background:transparent}
  .heading-edit{display:flex;gap:7px}.heading-edit select{width:66px;flex:none}.heading-edit input{font-weight:650}
  .image-editor{display:grid;grid-template-columns:minmax(140px,220px) 1fr;gap:10px;align-items:start}
  .image-fields{display:flex;flex-direction:column;gap:8px}
  .callout-fields{display:flex;flex-direction:column;gap:8px}
  .code-editor{font-family:var(--font-mono,ui-monospace,monospace)}
  .preview{overflow:auto;background:var(--panel-2);padding:20px clamp(16px,2vw,30px) 60px}
  .preview-label{margin-bottom:13px;color:var(--muted)}
  .welcome{grid-column:2/4;display:grid;place-items:center;align-content:center;text-align:center;color:var(--muted);padding:40px}.welcome h2{color:var(--ink);margin-bottom:3px}.welcome p{max-width:440px}
  @media(max-width:1100px){.studio{grid-template-columns:210px 1fr}.preview{display:none}}
  @media(max-width:720px){.studio{grid-template-columns:1fr}.drafts{display:none}.editor{border:0}.meta-row,.meta-row:last-child,.link-results{grid-template-columns:1fr}}
</style>
