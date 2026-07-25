<script>
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import StudioCanvasBlock from './StudioCanvasBlock.svelte';
  import StudioDocumentView from './StudioDocumentView.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioInfoCard from './StudioInfoCard.svelte';
  import StudioInfoCardEditor from './StudioInfoCardEditor.svelte';
  import StudioMediaEditor from './StudioMediaEditor.svelte';
  import StudioPageManager from './StudioPageManager.svelte';
  import { t, relTime } from './i18n.svelte.js';
  import {
    saveStudioRecovery, loadStudioRecovery, clearStudioRecovery,
  } from './studioRecovery.js';
  import {
    normalizeStudioDocument, studioDocumentBlocks, studioPage, studioPages,
    createStudioPage, renameStudioPage, moveStudioPage, removeStudioPage,
    createStudioInfoCard, removeStudioInfoCard, moveStudioInfoCard,
    studioInfoCardsByAnchor,
  } from './studioContent.js';
  import { plainText } from './studioEditable.js';
  import { itemSearch } from './libraryApi.js';
  import {
    listStudioDocuments, getStudioDocument, createStudioDocument,
    updateStudioDocument, archiveStudioDocument, purgeStudioDocument, publishStudioDocument,
    unpublishStudioDocument, getStudioCapabilities, uploadStudioAsset,
    listStudioRevisions, restoreStudioRevision,
  } from './studioApi.js';

  let { onOpenItem, onShellChange, sidebarOpen = true } = $props();

  let documents = $state([]);
  let selected = $state(null);
  let mode = $state('home');
  let activeSection = $state('structure');
  let selectedBlockID = $state('');
  let draggingBlockID = $state('');
  let draggingCardID = $state('');
  let loading = $state(true);
  let saving = $state(false);
  let saved = $state(false);
  let offline = $state(false);
  let error = $state('');
  let dirty = $state(false);
  let canPublish = $state(false);
  let quotaBytes = $state(0);
  let creatingTemplate = $state('');
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
  let imageTargetColumn = $state(null);
  let saveTimer;
  let savedTimer;
  let recoveryTimer;
  let retryTimer;
  let savePromise = null;
  let changeVersion = 0;
  let retryAttempt = 0;
  let studioActive = false;
  let openingSequence = 0;
  let linkSearchTimer;
  let linkAbort;
  let blockSequence = 1;
  let activePageID = $state('');

  const content = () => selected?.content || {
    schemaVersion: 2,
    presentation: {},
    classification: {},
    pages: [{ id: 'p1', title: '', blocks: [] }],
  };
  const activeBlocks = () => studioDocumentBlocks(selected, activePageID);
  // Las fichas son de la página activa, no del documento.
  const activePage = () => studioPage(selected, activePageID);
  const infoCards = () => activePage()?.infoCards || [];
  const infoCardVisible = () => infoCards().length > 0;
  const cardSlots = () => studioInfoCardsByAnchor(infoCards(), documentBlocks().length);
  const documentCover = () => activeBlocks().find((block) => block?.type === 'image' && block.role === 'cover') || null;
  const documentBlocks = () => activeBlocks().filter((block) => block?.role !== 'cover');

  function defaultSection(document = selected) {
    if (document?.templateKey === 'cabinet.audio') return 'tracks';
    if (document?.templateKey?.startsWith('cabinet.')) return 'file';
    if (document?.templateKey?.startsWith('moments.')) return 'video';
    return 'structure';
  }

  function firstEditableBlockID(document, pageId = '') {
    return studioDocumentBlocks(document, pageId).find((block) => block?.role !== 'cover')?.id || '';
  }

  function selectInitialPage(document) {
    activePageID = studioPage(document)?.id || '';
  }

  function selectPage(pageId) {
    const page = studioPage(selected, pageId);
    if (!page || page.id === activePageID) return;
    activePageID = page.id;
    selectedBlockID = firstEditableBlockID(selected, page.id);
    closeLinkPicker();
  }

  function addPage() {
    if (!selected || selected.status === 'archived' || studioPages(selected).length >= 100) return;
    const number = studioPages(selected).length + 1;
    const page = createStudioPage(selected, t('studio.newPageTitle', { number }));
    activePageID = page.id;
    selectedBlockID = '';
    closeLinkPicker();
    touch();
  }

  function renamePage(pageId, title) {
    if (!selected || selected.status === 'archived') return;
    if (renameStudioPage(selected, pageId, title)) touch();
  }

  function reorderPage(pageId, delta) {
    if (!selected || selected.status === 'archived') return;
    if (moveStudioPage(selected, pageId, delta)) touch();
  }

  function deletePage(pageId) {
    if (!selected || selected.status === 'archived') return;
    const page = studioPage(selected, pageId);
    const pages = studioPages(selected);
    if (!page || pages.length <= 1) return;
    if (!confirm(t('studio.removePageConfirm', {
      title: page.title,
      count: page.blocks?.length || 0,
    }))) return;
    const result = removeStudioPage(selected, pageId);
    if (!result) return;
    if (activePageID === pageId) {
      activePageID = result.nextPage.id;
      selectedBlockID = firstEditableBlockID(selected, activePageID);
    }
    closeLinkPicker();
    touch();
  }

  function pageTextSize(field, fallback) {
    const value = Number(content().presentation?.[field]);
    return Number.isInteger(value) && value >= 10 && value <= 96 ? value : fallback;
  }

  function setPageTextSize(field, value, fallback) {
    content().presentation ||= {};
    content().presentation[field] = Math.max(10, Math.min(96, Math.round(Number(value) || fallback)));
    touch();
  }

  function blockDefaultTextSize(block) {
    if (block?.type === 'heading') return ({ 1: 42, 2: 25, 3: 18 })[block.level || 2];
    if (block?.type === 'quote') return 17;
    if (block?.type === 'code' || block?.type === 'table' || block?.type === 'callout') return 13;
    if (block?.type === 'image') return 15;
    return 15;
  }

  function selectedTextControl() {
    if (selectedBlockID === '@title') {
      return {
        size: pageTextSize('titleFontSize', 34),
        align: content().presentation?.titleTextAlign || 'left',
        canAlign: true,
      };
    }
    if (selectedBlockID === '@summary') {
      return {
        size: pageTextSize('summaryFontSize', 17),
        align: content().presentation?.summaryTextAlign || 'left',
        canAlign: true,
      };
    }
    const block = findBlockByID(selectedBlockID);
    const supported = ['heading', 'paragraph', 'quote', 'bulletList', 'orderedList', 'code', 'callout', 'table'].includes(block?.type)
      || (block?.type === 'image'
        && ['medium', 'small'].includes(block.imageSize)
        && ['left', 'right'].includes(block.imageAlign));
    if (!supported) return null;
    const value = Number(block.fontSize);
    return {
      size: Number.isInteger(value) && value >= 10 && value <= 96
        ? value
        : blockDefaultTextSize(block),
      align: ['left', 'center', 'right'].includes(block.textAlign) ? block.textAlign : 'left',
      canAlign: block.type !== 'table',
    };
  }

  function setSelectedTextSize(value) {
    const next = Math.max(10, Math.min(96, Math.round(Number(value) || 15)));
    if (selectedBlockID === '@title') {
      setPageTextSize('titleFontSize', next, 34);
      return;
    }
    if (selectedBlockID === '@summary') {
      setPageTextSize('summaryFontSize', next, 17);
      return;
    }
    const block = findBlockByID(selectedBlockID);
    if (!block) return;
    block.fontSize = next;
    touch();
  }

  function setSelectedTextAlign(align) {
    if (!['left', 'center', 'right'].includes(align)) return;
    if (selectedBlockID === '@title' || selectedBlockID === '@summary') {
      content().presentation ||= {};
      const field = selectedBlockID === '@title' ? 'titleTextAlign' : 'summaryTextAlign';
      content().presentation[field] = align;
      touch();
      return;
    }
    const block = findBlockByID(selectedBlockID);
    if (!block || block.type === 'table') return;
    block.textAlign = align;
    touch();
  }

  onMount(() => {
    studioActive = true;
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
    const online = () => {
      if (!dirty || !offline) return;
      retryAttempt = 0;
      scheduleRetry(0);
    };
    window.addEventListener('beforeunload', beforeUnload);
    window.addEventListener('keydown', keydown);
    window.addEventListener('online', online);
    return () => {
      studioActive = false;
      clearTimeout(saveTimer);
      clearTimeout(savedTimer);
      clearTimeout(retryTimer);
      clearTimeout(linkSearchTimer);
      linkAbort?.abort();
      // La copia local se encola primero; si el flush al servidor termina bien,
      // clearStudioRecovery se ejecutará después sobre la misma cola.
      void persistRecoveryNow();
      void flushCurrent();
      window.removeEventListener('beforeunload', beforeUnload);
      window.removeEventListener('keydown', keydown);
      window.removeEventListener('online', online);
    };
  });

  function normalizeDocument(doc) {
    return normalizeStudioDocument(doc);
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const capabilities = await getStudioCapabilities();
      canPublish = !!capabilities.canPublish;
      quotaBytes = capabilities.quotaBytes || 0;
      documents = await listStudioDocuments('all');
    } catch (e) {
      error = e.code || e.message;
    }
    loading = false;
  }

  function templateContent(templateKey, documentTitle) {
    const base = {
      classification: { workType: 'article', topics: [], audience: [] },
      presentation: { contentWidth: 'reading', fontPreset: 'editorial' },
    };
    if (templateKey.startsWith('cabinet.') || templateKey.startsWith('moments.')) {
      return {
        ...base,
        schemaVersion: 1,
        classification: {
          ...base.classification,
          workType: templateKey.replace('.', '-'),
        },
        presentation: { contentWidth: 'wide', fontPreset: 'sans' },
        blocks: [],
      };
    }
    let blocks;
    if (templateKey === 'technical') {
      base.classification = { ...base.classification, workType: 'manual' };
      base.presentation = { contentWidth: 'wide', fontPreset: 'sans' };
      blocks = [
        { id: nextBlockId(), type: 'heading', level: 2, text: t('studio.template.objective') },
        { id: nextBlockId(), type: 'paragraph', text: '' },
        { id: nextBlockId(), type: 'heading', level: 2, text: t('studio.template.procedure') },
        { id: nextBlockId(), type: 'orderedList', items: [t('studio.template.firstStep')] },
        { id: nextBlockId(), type: 'heading', level: 2, text: t('studio.template.references') },
        { id: nextBlockId(), type: 'paragraph', text: '' },
      ];
    } else if (templateKey === 'story') {
      base.classification = { ...base.classification, workType: 'story' };
      blocks = [
        { id: nextBlockId(), type: 'heading', level: 2, text: t('studio.template.chapterOne') },
        { id: nextBlockId(), type: 'paragraph', text: '' },
      ];
    } else {
      blocks = [
        { id: nextBlockId(), type: 'heading', level: 2, text: t('studio.template.introduction') },
        { id: nextBlockId(), type: 'paragraph', text: '' },
      ];
    }
    return {
      ...base,
      schemaVersion: 2,
      pages: [{ id: 'p1', title: documentTitle, blocks, infoCards: [] }],
    };
  }

  function nextBlockId() {
    return `block-${Date.now().toString(36)}-${blockSequence++}`;
  }

  async function newDocument(template) {
    if (!template?.key || creatingTemplate) return;
    if (!await flushCurrent()) return;
    creatingTemplate = template.key;
    error = '';
    try {
      const title = template.key.startsWith('cabinet.')
        ? t('studio.template.cabinetUntitled')
        : template.key.startsWith('moments.')
          ? t('studio.template.momentsUntitled')
          : t(`studio.template.${template.key}Untitled`);
      const doc = normalizeDocument(await createStudioDocument({
        templateKey: template.key,
        title,
        language: '',
        tags: [],
        metadata: {},
        content: templateContent(template.key, title),
      }));
      openingSequence++;
      documents = [{ ...doc }, ...documents];
      selected = doc;
      selectInitialPage(doc);
      mode = 'editor';
      activeSection = defaultSection(doc);
      selectedBlockID = firstEditableBlockID(doc);
      dirty = false;
      offline = false;
      changeVersion = 0;
      revisions = [];
      closeLinkPicker();
      if (showRevisions) loadRevisions(doc.id);
    } catch (e) {
      error = e.code || e.message;
    } finally {
      creatingTemplate = '';
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
      selectInitialPage(doc);
      mode = 'editor';
      activeSection = defaultSection(doc);
      selectedBlockID = firstEditableBlockID(doc);
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
        selectInitialPage(selected);
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
    saved = false;
    changeVersion++;
    scheduleRecovery();
    scheduleSave();
  }

  function scheduleRecovery(delay = 300) {
    clearTimeout(recoveryTimer);
    recoveryTimer = setTimeout(() => {
      recoveryTimer = null;
      if (selected && dirty) void saveStudioRecovery(selected);
    }, delay);
  }

  async function persistRecoveryNow() {
    clearTimeout(recoveryTimer);
    recoveryTimer = null;
    if (selected && dirty) await saveStudioRecovery(selected);
  }

  function scheduleSave(delay = 1200) {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(saveNow, delay);
  }

  function clearRetry() {
    clearTimeout(retryTimer);
    retryTimer = null;
    retryAttempt = 0;
  }

  function scheduleRetry(delay) {
    if (!studioActive || !dirty || !offline) return;
    clearTimeout(retryTimer);
    if (typeof navigator !== 'undefined' && navigator.onLine === false) return;
    const wait = delay ?? Math.min(30000, 1000 * (2 ** retryAttempt));
    retryAttempt = Math.min(retryAttempt + 1, 5);
    retryTimer = setTimeout(() => {
      retryTimer = null;
      if (studioActive && dirty && offline) void saveNow();
    }, wait);
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

  function mergeSavedEnvelope(document, updated) {
    // El cuerpo y los metadatos que está editando el usuario deben conservar
    // su identidad. Reemplazarlos por la respuesta del servidor desmonta los
    // contenteditable, pierde el foco y puede devolver el lienzo hacia arriba.
    for (const field of [
      'revision',
      'status',
      'updated',
      'publishedRevision',
      'publicationKind',
      'publicationTarget',
      'published',
      'coverAssetId',
    ]) {
      document[field] = updated[field];
    }
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
        clearRetry();
        offline = false;
        documents = documents.map((item) => item.id === updated.id ? { ...item, ...updated } : item);
        if (showRevisions) loadRevisions(documentId);
        if (selected?.id !== documentId) return true;

        if (changeVersion === version) {
          mergeSavedEnvelope(selected, updated);
          dirty = false;
          saved = true;
          clearTimeout(recoveryTimer);
          recoveryTimer = null;
          await clearStudioRecovery(documentId);
          clearTimeout(savedTimer);
          savedTimer = setTimeout(() => { saved = false; }, 1800);
        } else {
          // El servidor ha guardado la instantánea enviada, pero el usuario
          // siguió escribiendo durante la petición. Conservamos esos cambios y
          // solo adelantamos su baseRevision para el siguiente guardado.
          mergeSavedEnvelope(selected, updated);
          dirty = true;
          scheduleRecovery(0);
          scheduleSave(0);
        }
        return true;
      } catch (e) {
        offline = !e.status;
        if (e.status === 409) error = 'studio.conflict';
        else if (offline) {
          error = 'studio.offline';
          await persistRecoveryNow();
          scheduleRetry();
        }
        else error = e.code || e.message;
        if (!offline) clearRetry();
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
    if (showRevisions) {
      activeSection = 'history';
      mode = 'editor';
      loadRevisions();
    } else {
      activeSection = defaultSection();
    }
  }

  async function restoreRevision(revision) {
    if (!selected || restoringRevision || revision.revision === selected.revision) return;
    const confirmKey = selected.publishedRevision
      ? 'studio.restorePublishedConfirm'
      : 'studio.restoreConfirm';
    if (!confirm(t(confirmKey, { revision: revision.revision }))) return;
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
      selectInitialPage(restored);
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

  function createBlock(type, options = {}) {
    const block = { id: nextBlockId(), type };
    if (type === 'heading') Object.assign(block, { level: 2, text: t('studio.headingPlaceholder') });
    else if (type === 'bulletList' || type === 'orderedList') block.items = [t('studio.listPlaceholder')];
    else if (type === 'table') block.rows = [[t('studio.tableHeader'), t('studio.tableHeader')], ['', '']];
    else if (type === 'callout') Object.assign(block, { title: '', text: '' });
    else if (type === 'columns') {
      const columnCount = [1, 2, 3].includes(options.columnCount)
        ? options.columnCount
        : 2;
      block.layout = columnCount === 1 ? 'full' : 'equal';
      block.columns = Array.from(
        { length: columnCount },
        () => [createBlock('paragraph')],
      );
    }
    else if (type === 'divider') {}
    else block.text = '';
    return block;
  }

  function addBlock(type, options = {}) {
    const block = createBlock(type, options);
    activeBlocks().push(block);
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
    activeBlocks().push({
      id: nextBlockId(),
      type: 'itemRef',
      itemId: item.itemId,
      titleSnapshot: item.title || item.itemId,
      kindSnapshot: item.kind || 'item',
    });
    touch();
    closeLinkPicker();
  }

  function chooseImage(targetColumn = null) {
    if (!selected || selected.status === 'archived' || uploadingImage) return;
    imageTargetColumn = targetColumn;
    imageInput?.click();
  }

  function chooseDocumentCover() {
    chooseImage({ cover: true });
  }

  function findInfoCard(cardId) {
    return infoCards().find((card) => card.id === cardId) || null;
  }

  function chooseInfoCardImage(cardId) {
    chooseImage({ infoCardId: cardId });
  }

  function removeInfoCardImage(cardId) {
    const card = findInfoCard(cardId);
    if (!card?.assetId) return;
    card.assetId = '';
    touch();
  }

  function addInfoCard() {
    if (!selected || selected.status === 'archived') return;
    if (!createStudioInfoCard(activePage())) return;
    touch();
  }

  function removeInfoCard(cardId) {
    if (!removeStudioInfoCard(activePage(), cardId)) return;
    touch();
  }

  function moveInfoCard(cardId, delta) {
    if (!moveStudioInfoCard(activePage(), cardId, delta, documentBlocks().length)) return;
    selectedBlockID = cardId;
    touch();
  }

  function flipInfoCardSide(cardId) {
    const card = findInfoCard(cardId);
    if (!card) return;
    card.side = card.side === 'left' ? 'right' : 'left';
    selectedBlockID = cardId;
    touch();
  }

  function startInfoCardDrag(cardId, event) {
    draggingCardID = cardId;
    draggingBlockID = '';
    selectedBlockID = cardId;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', cardId);
  }

  // Soltar una ficha sobre un bloque la ancla ahí. Sólo vale para bloques del
  // cuerpo: los anidados (dentro de columnas) no son posiciones de anclaje.
  function anchorInfoCardAt(index) {
    const card = findInfoCard(draggingCardID);
    draggingCardID = '';
    if (!card || index < 0) return true;
    card.anchor = index;
    touch();
    return true;
  }

  function removeDocumentCover() {
    const cover = documentCover();
    if (!cover) return;
    const blocks = activeBlocks();
    const index = blocks.indexOf(cover);
    if (index >= 0) blocks.splice(index, 1);
    touch();
  }

  async function imageSelected(event) {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = '';
    if (!file || !selected) {
      imageTargetColumn = null;
      return;
    }
    const documentId = selected.id;
    const targetColumn = imageTargetColumn;
    imageTargetColumn = null;
    uploadingImage = true;
    error = '';
    try {
      const asset = await uploadStudioAsset(documentId, file);
      if (selected?.id !== documentId) return;
      const imageBlock = {
        id: nextBlockId(), type: 'image', assetId: asset.id,
        caption: '', alt: '', sideText: '',
        imageSize: 'original', imageAlign: 'center',
      };
      if (targetColumn?.infoCardId) {
        const card = findInfoCard(targetColumn.infoCardId);
        if (!card) return;
        card.assetId = asset.id;
        selectedBlockID = '';
        touch();
        return;
      }
      if (targetColumn?.cover) {
        const current = documentCover();
        if (current) {
          current.assetId = asset.id;
          current.imageSize = 'poster';
          current.imageAlign = 'center';
        } else {
          imageBlock.role = 'cover';
          imageBlock.imageSize = 'poster';
          activeBlocks().unshift(imageBlock);
        }
        selectedBlockID = '';
        touch();
        return;
      }
      const target = targetColumn
        ? findBlockByID(targetColumn.blockID)
        : null;
      if (target?.type === 'columns' && target.columns?.[targetColumn.columnIndex]) {
        target.columns[targetColumn.columnIndex].push(imageBlock);
      } else {
        activeBlocks().push(imageBlock);
      }
      selectedBlockID = imageBlock.id;
      touch();
    } catch (e) {
      error = e.code || e.message;
    } finally {
      uploadingImage = false;
    }
  }

  async function uploadMediaAsset(file, purpose) {
    if (!selected || selected.status === 'archived') return null;
    const documentId = selected.id;
    error = '';
    const asset = await uploadStudioAsset(documentId, file, purpose);
    if (selected?.id !== documentId) return null;
    return asset;
  }

  function mediaEditorError(cause) {
    error = cause?.code || cause?.message || 'studio.internal';
  }

  function findBlockLocationIn(blockID, blocks) {
    for (let index = 0; index < (blocks || []).length; index++) {
      const block = blocks[index];
      if (block.id === blockID) return { block, container: blocks, index };
      for (const column of block.columns || []) {
        const nested = findBlockLocationIn(blockID, column);
        if (nested) return nested;
      }
      for (const children of [block.children, block.blocks]) {
        const nested = findBlockLocationIn(blockID, children);
        if (nested) return nested;
      }
    }
    return null;
  }

  function findBlockLocation(blockID) {
    return findBlockLocationIn(blockID, activeBlocks());
  }

  function findBlockByID(blockID) {
    return findBlockLocation(blockID)?.block || null;
  }

  function blockContainsID(block, blockID) {
    if (!block) return false;
    if (block.id === blockID) return true;
    for (const column of block.columns || []) {
      if (column.some((child) => blockContainsID(child, blockID))) return true;
    }
    for (const children of [block.children, block.blocks]) {
      if ((children || []).some((child) => blockContainsID(child, blockID))) return true;
    }
    return false;
  }

  function removeBlock(blockID) {
    const location = findBlockLocation(blockID);
    if (!location) return;
    if (blockContainsID(location.block, selectedBlockID)) selectedBlockID = '';
    location.container.splice(location.index, 1);
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

  async function purgeSelected() {
    if (!selected || selected.status !== 'archived') return;
    if (!confirm(t('studio.purgeConfirm', { title: selected.title }))) return;
    const id = selected.id;
    try {
      await purgeStudioDocument(id);
      await clearStudioRecovery(id);
      documents = documents.filter((item) => item.id !== id);
      selected = null;
      mode = 'home';
      selectedBlockID = '';
      showRevisions = false;
      closeLinkPicker();
    } catch (e) {
      error = e.code || e.message;
    }
  }

  // Guardar una vez no basta para dejar el documento limpio: si el usuario sigue
  // escribiendo durante la petición, saveNow conserva esos cambios, vuelve a
  // marcar dirty y programa otro guardado. Publicar en ese momento publica la
  // revisión anterior, y el guardado que viene detrás sube la revisión: la
  // publicación queda pendiente al instante, justo después de publicar.
  async function flushUntilClean(attempts = 5) {
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      if (!await flushCurrent()) return false;
      if (!dirty) return true;
    }
    return !dirty;
  }

  async function publishSelected() {
    if (!selected || !canPublish) return;
    if (!await flushUntilClean()) return;
    try {
      const updated = await publishStudioDocument(selected.id);
      // Sólo el sobre (revisión, estado, publicación). Reemplazar el documento
      // entero traería de vuelta el contenido del servidor sin normalizar y
      // desmontaría el lienzo que el usuario está editando.
      mergeSavedEnvelope(selected, updated);
      documents = documents.map((item) => item.id === updated.id ? { ...item, ...updated } : item);
    } catch (e) {
      error = e.code || e.message;
    }
  }

  async function unpublishSelected() {
    if (!selected?.publishedRevision || !canPublish) return;
    if (!await flushUntilClean()) return;
    try {
      const updated = await unpublishStudioDocument(selected.id);
      mergeSavedEnvelope(selected, updated);
      documents = documents.map((item) => item.id === updated.id ? { ...item, ...updated } : item);
    } catch (e) {
      error = e.code || e.message;
    }
  }

  function surfaceOf(document = selected) {
    if (document?.templateKey?.startsWith('cabinet.')) return 'cabinet';
    if (document?.templateKey?.startsWith('moments.')) return 'moments';
    return 'document';
  }

  async function createSurface(surface) {
    const key = surface === 'cabinet' ? 'cabinet.pdf'
      : surface === 'moments' ? 'moments.video' : 'document';
    await newDocument({ key });
  }

  async function goStudioHome() {
    if (!await flushCurrent()) return;
    openingSequence++;
    selected = null;
    mode = 'home';
    selectedBlockID = '';
    showRevisions = false;
    closeLinkPicker();
  }

  function togglePreview() {
    if (!selected) return;
    if (surfaceOf() !== 'document') {
      openSection('cover');
      return;
    }
    mode = mode === 'preview' ? 'editor' : 'preview';
  }

  function duplicateBlock(blockID) {
    const location = findBlockLocation(blockID);
    if (!location) return;
    const copy = JSON.parse(JSON.stringify(location.block));
    const renewIDs = (block) => {
      block.id = nextBlockId();
      for (const child of block.children || block.blocks || []) renewIDs(child);
      for (const column of block.columns || []) {
        for (const child of column) renewIDs(child);
      }
    };
    renewIDs(copy);
    location.container.splice(location.index + 1, 0, copy);
    selectedBlockID = copy.id;
    touch();
  }

  function startBlockDrag(blockID, event) {
    draggingBlockID = blockID;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', blockID);
  }

  function endBlockDrag() {
    draggingBlockID = '';
    draggingCardID = '';
  }

  function takeDraggedBlock(destinationBlockID = '') {
    if (!draggingBlockID || draggingBlockID === destinationBlockID) return null;
    const location = findBlockLocation(draggingBlockID);
    if (!location || blockContainsID(location.block, destinationBlockID)) return null;
    const [block] = location.container.splice(location.index, 1);
    return block;
  }

  function dropBeforeBlock(targetBlockID) {
    if (draggingCardID) {
      anchorInfoCardAt(documentBlocks().findIndex((block) => block.id === targetBlockID));
      return;
    }
    const block = takeDraggedBlock(targetBlockID);
    if (!block) return;
    const target = findBlockLocation(targetBlockID);
    if (!target) return;
    target.container.splice(target.index, 0, block);
    draggingBlockID = '';
    selectedBlockID = block.id;
    touch();
  }

  function dropIntoColumn(columnsBlockID, columnIndex) {
    // Una ficha no cabe dentro de una columna: se ancla al bloque de columnas.
    if (draggingCardID) {
      anchorInfoCardAt(documentBlocks().findIndex((block) => block.id === columnsBlockID));
      return;
    }
    const destinationBeforeMove = findBlockByID(columnsBlockID);
    if (!destinationBeforeMove || blockContainsID(findBlockByID(draggingBlockID), columnsBlockID)) return;
    const block = takeDraggedBlock(columnsBlockID);
    if (!block) return;
    const destination = findBlockByID(columnsBlockID);
    if (!destination?.columns?.[columnIndex]) {
      activeBlocks().push(block);
      return;
    }
    destination.columns[columnIndex].push(block);
    draggingBlockID = '';
    selectedBlockID = block.id;
    touch();
  }

  function dropAtRootEnd() {
    if (draggingCardID) {
      anchorInfoCardAt(documentBlocks().length - 1);
      return;
    }
    const block = takeDraggedBlock();
    if (!block) return;
    activeBlocks().push(block);
    draggingBlockID = '';
    selectedBlockID = block.id;
    touch();
  }

  function addToColumn(columnsBlockID, columnIndex, type) {
    const destination = findBlockByID(columnsBlockID);
    if (!destination?.columns?.[columnIndex]) return;
    const block = createBlock(type);
    destination.columns[columnIndex].push(block);
    selectedBlockID = block.id;
    touch();
  }

  function moveBlockToRoot(blockID) {
    const location = findBlockLocation(blockID);
    const blocks = activeBlocks();
    if (!location || location.container === blocks) return;
    const [block] = location.container.splice(location.index, 1);
    blocks.push(block);
    selectedBlockID = block.id;
    touch();
  }

  function runTool(key) {
    if (key === 'bold' || key === 'italic') {
      globalThis.document?.execCommand?.(key);
      return;
    }
    if (key === 'image') {
      chooseImage();
      return;
    }
    if (key === 'link') {
      toggleLinkPicker();
      return;
    }
    const types = {
      heading: 'heading', list: 'bulletList', quote: 'quote', code: 'code',
      columns: 'columns', table: 'table',
    };
    if (types[key]) addBlock(types[key]);
  }

  function openSection(key) {
    activeSection = key;
    mode = 'editor';
    showRevisions = false;
    requestAnimationFrame(() => {
      globalThis.document?.querySelector(`[data-studio-section="${key}"]`)
        ?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }

  function formatQuota(bytes) {
    if (!bytes) return t('studio.quotaUnknown');
    const gb = bytes / (1024 ** 3);
    return t('studio.quotaLimit', { size: gb >= 1 ? `${gb.toFixed(gb >= 10 ? 0 : 1)} GB` : `${Math.round(bytes / (1024 ** 2))} MB` });
  }

  function shellSections() {
    return [];
  }

  function shellTools() {
    if (surfaceOf() !== 'document') return [];
    return [
      { key: 'bold', short: 'B', label: t('studio.tool.bold') },
      { key: 'italic', short: 'I', label: t('studio.tool.italic') },
      { key: 'heading', short: 'H₁', label: t('studio.block.heading') },
      { key: 'list', short: '≔', label: t('studio.block.bulletList') },
      { key: 'quote', short: '❝', label: t('studio.block.quote') },
      { key: 'code', short: '</>', label: t('studio.block.code') },
      { key: 'columns', short: '▥', label: t('studio.block.columns') },
      { key: 'image', short: '▧', label: t('studio.block.image') },
      { key: 'table', short: '⊞', label: t('studio.block.table') },
    ];
  }

  $effect(() => {
    const surface = surfaceOf();
    const publishDisabled = !selected || selected.status === 'archived';
    const publicationPending = !!selected?.publishedRevision &&
      (dirty || selected.revision !== selected.publishedRevision);
    onShellChange?.({
      mode,
      title: selected?.title || t('studio.title'),
      saveState: saving ? 'saving' : offline ? 'error' : dirty ? 'changes' : publicationPending ? 'publication' : 'saved',
      saveLabel: saving ? t('studio.saving') : offline ? t('studio.offlineShort') : dirty ? t('studio.changesPending') : publicationPending ? t('studio.publicationPending') : t('studio.saved'),
      tools: shellTools(),
      textControl: selectedTextControl(),
      canPublish,
      publishDisabled,
      publishLabel: selected?.publishedRevision ? t('studio.updatePublication') : t('studio.publish'),
      documents,
      selected,
      activeSection,
      sections: shellSections(),
      revisionsOpen: showRevisions,
      revisionCount: revisions.length || selected?.revision || 0,
      kindGlyph: surface === 'cabinet' ? '▣' : surface === 'moments' ? '▶' : '✎',
      kindLabel: surface === 'cabinet' ? t('studio.createCabinet') : surface === 'moments' ? t('studio.createMoments') : t('studio.createDocument'),
      kindHint: surface === 'document' ? t('studio.blockEditor') : t('studio.publicationForm'),
      destination: surface === 'cabinet' ? t('studio.destinationCabinet')
        : surface === 'moments' ? t('studio.destinationMoments') : t('studio.destinationDocuments'),
      quotaLabel: formatQuota(quotaBytes),
      canArchive: selected?.status !== 'archived',
      canPurge: selected?.status === 'archived',
      goHome: goStudioHome,
      togglePreview,
      publish: publishSelected,
      unpublish: selected?.publishedRevision ? unpublishSelected : null,
      runTool,
      setTextSize: setSelectedTextSize,
      setTextAlign: setSelectedTextAlign,
      openDocument,
      openSection,
      toggleRevisions,
      archive: archiveSelected,
      purge: purgeSelected,
    });
  });
</script>

<section class="studio-new" class:sidebar-hidden={!sidebarOpen}>
  {#if loading}
    <div class="studio-state">{t('common.loading')}</div>
  {:else if mode === 'home'}
    <main class="studio-home">
      <h2>{t('studio.create')}</h2>
      <div class="create-grid">
        <button class="create-card" onclick={() => createSurface('document')}>
          <span class="create-glyph">✎</span>
          <b>{t('studio.createDocument')}</b>
          <small>{t('studio.createDocumentDesc')}</small>
        </button>
        <button class="create-card" onclick={() => createSurface('cabinet')}>
          <span class="create-glyph">▣</span>
          <b>{t('studio.createCabinet')}</b>
          <small>{t('studio.createCabinetDesc')}</small>
        </button>
        <button class="create-card" onclick={() => createSurface('moments')}>
          <span class="create-glyph">▶</span>
          <b>{t('studio.createMoments')}</b>
          <small>{t('studio.createMomentsDesc')}</small>
        </button>
      </div>

      <h2>{t('studio.continueCreating')}</h2>
      <div class="recent-list">
        {#each documents.slice(0, 12) as doc (doc.id)}
          {@const surface = surfaceOf(doc)}
          <button class="recent-item" onclick={() => openDocument(doc.id)}>
            <span class="recent-glyph">{surface === 'cabinet' ? '▣' : surface === 'moments' ? '▶' : '✎'}</span>
            <span class="recent-meta">
              <b>{doc.title || t('studio.untitled')}</b>
              <small>
                {surface === 'cabinet' ? t('studio.createCabinet') : surface === 'moments' ? t('studio.createMoments') : t('studio.createDocument')}
                · {relTime(doc.updated)}
              </small>
            </span>
            <span class:published={!!doc.publishedRevision} class="recent-state">
              {doc.publishedRevision ? t('studio.published') : t('studio.draft')}
            </span>
          </button>
        {/each}
        {#if documents.length === 0}
          <div class="home-empty">{t('studio.empty')}</div>
        {/if}
      </div>
      {#if error}<div class="studio-error">{t(error)}</div>{/if}
    </main>
  {:else if selected && mode === 'preview'}
    <main class="preview-mode scroll thin">
      <StudioDocumentView document={selected} pageId={activePageID} {onOpenItem} preview expanded={!sidebarOpen} />
    </main>
  {:else if selected && surfaceOf() === 'document'}
    <main class="document-workspace scroll thin">
      <aside class="document-palette" aria-label={t('studio.documentPalette')}>
        <StudioPageManager
          document={selected}
          {activePageID}
          onSelect={selectPage}
          onCreate={addPage}
          onRename={renamePage}
          onMove={reorderPage}
          onRemove={deletePage}
        />
        <StudioInfoCardEditor
          documentId={selected.id}
          cards={infoCards()}
          uploading={uploadingImage}
          onAdd={addInfoCard}
          onRemoveCard={removeInfoCard}
          onMoveCard={moveInfoCard}
          onChooseImage={chooseInfoCardImage}
          onRemoveImage={removeInfoCardImage}
          onChange={touch}
        />
        <h3>{t('studio.insertBlock')}</h3>
        <div class="block-grid">
          <button onclick={() => addBlock('paragraph')}><b>¶</b>{t('studio.block.paragraph')}</button>
          <button onclick={() => addBlock('heading')}><b>H</b>{t('studio.block.heading')}</button>
          <button onclick={chooseImage} disabled={uploadingImage}><b>▧</b>{t('studio.block.image')}</button>
          <button onclick={() => addBlock('columns', { columnCount: 1 })}><b>▯</b>{t('studio.block.oneColumn')}</button>
          <button onclick={() => addBlock('columns', { columnCount: 2 })}><b>▥</b>{t('studio.block.twoColumns')}</button>
          <button onclick={() => addBlock('columns', { columnCount: 3 })}><b>▥</b>{t('studio.block.threeColumns')}</button>
          <button onclick={() => addBlock('table')}><b>⊞</b>{t('studio.block.table')}</button>
          <button onclick={() => addBlock('quote')}><b>❝</b>{t('studio.block.quote')}</button>
          <button onclick={() => addBlock('callout')}><b>!</b>{t('studio.block.callout')}</button>
          <button onclick={() => addBlock('code')}><b>&lt;/&gt;</b>{t('studio.block.code')}</button>
          <button onclick={() => addBlock('bulletList')}><b>≔</b>{t('studio.block.bulletList')}</button>
          <button onclick={() => addBlock('divider')}><b>—</b>{t('studio.block.divider')}</button>
        </div>

        <div class="palette-section" class:active={activeSection === 'design'} data-studio-section="design">
          <h3>{t('studio.pageDesign')}</h3>
          <div class="style-options">
            {#each [
              ['reading', t('studio.widthReading'), '910 px'],
              ['wide', t('studio.widthWide'), '980 px'],
              ['editorial', t('studio.widthEditorial'), '1180 px'],
              ['compact', t('studio.widthCompact'), '740 px'],
            ] as option}
              <button
                class:active={content().presentation?.contentWidth === option[0]}
                onclick={() => { selected.content.presentation.contentWidth = option[0]; touch(); }}
              ><span>{option[1]}</span><small>{option[2]}</small></button>
            {/each}
          </div>

          <h3>{t('studio.typography')}</h3>
          <div class="style-options">
            <button class:active={content().presentation?.fontPreset !== 'sans'} onclick={() => { selected.content.presentation.fontPreset = 'editorial'; touch(); }}>
              <span>{t('studio.fontEditorial')}</span><small>Serif</small>
            </button>
            <button class:active={content().presentation?.fontPreset === 'sans'} onclick={() => { selected.content.presentation.fontPreset = 'sans'; touch(); }}>
              <span>{t('studio.fontSans')}</span><small>Sans</small>
            </button>
          </div>
        </div>

        <div class="palette-section metadata-palette" class:active={activeSection === 'metadata'} data-studio-section="metadata">
          <h3>{t('studio.metadata')}</h3>
          <div class="metadata-grid">
            <label>{t('studio.author')}<input value={selected.authorLabel || ''} oninput={(event) => { selected.authorLabel = event.currentTarget.value; touch(); }} /></label>
            <label>{t('studio.language')}<input value={selected.language || ''} placeholder="es" oninput={(event) => { selected.language = event.currentTarget.value; touch(); }} /></label>
            <label>{t('studio.tags')}<input value={tagsText()} placeholder={t('studio.tagsPlaceholder')} oninput={(event) => setTags(event.currentTarget.value)} /></label>
            <label>{t('studio.workType')}<input value={content().classification?.workType || ''} placeholder="article" oninput={(event) => { selected.content.classification.workType = event.currentTarget.value; touch(); }} /></label>
          </div>
        </div>

        <div class="palette-section cover-palette" data-studio-section="cover">
          <h3>{t('studio.section.cover')}</h3>
          <button class="document-cover" class:ready={!!documentCover()} onclick={chooseDocumentCover} disabled={uploadingImage}>
            {#if documentCover()}
              <StudioImage documentId={selected.id} assetId={documentCover().assetId} alt={t('studio.section.cover')} compact />
            {:else}
              <b>＋</b><span>{uploadingImage ? t('studio.uploadingImage') : t('studio.addCover')}</span>
            {/if}
          </button>
          {#if documentCover()}
            <div class="cover-actions">
              <button onclick={chooseDocumentCover} disabled={uploadingImage}>{t('studio.replaceCover')}</button>
              <button class="remove" onclick={removeDocumentCover}>{t('studio.removeCover')}</button>
            </div>
          {/if}
        </div>

        <button class="link-tool" class:active={linkPicker} onclick={toggleLinkPicker}>
          <Icon name="book" size={14} />{t('studio.internalLink')}
        </button>
        {#if linkPicker}
          <div class="link-picker">
            <input
              value={linkQuery}
              placeholder={t('studio.internalLinkSearch')}
              aria-label={t('studio.internalLinkSearch')}
              oninput={(event) => searchLinkTargets(event.currentTarget.value)}
            />
            {#if linkLoading}
              <small>{t('common.loading')}</small>
            {:else if linkResults.length}
              <div class="link-results">
                {#each linkResults as item (item.itemId)}
                  <button onclick={() => insertItemReference(item)}><b>{item.title}</b><small>{item.kind}</small></button>
                {/each}
              </div>
            {:else}
              <small>{t('studio.internalLinkHint')}</small>
            {/if}
          </div>
        {/if}
      </aside>

      <div
        class="canvas-column"
        class:has-info-card={infoCardVisible()}
        class:wide={content().presentation?.contentWidth === 'wide'}
        class:editorial={content().presentation?.contentWidth === 'editorial'}
        class:compact={content().presentation?.contentWidth === 'compact'}
      >
        {#if error}<div class="studio-error">{t(error)}</div>{/if}
        {#if showRevisions}
          <section class="revision-panel">
            <header><b>{t('studio.revisions')}</b><span>{revisions.length}</span></header>
            {#if revisionsLoading}
              <div>{t('common.loading')}</div>
            {:else}
              <div class="revision-list">
                {#each revisions as revision (revision.revision)}
                  <div class="revision-row">
                    <span>
                      <b>{revision.title}</b>
                      <small>
                        {t('studio.revisionNumber', { revision: revision.revision })} · {relTime(revision.created)}
                        {#if revision.revision === selected.revision} · {t('studio.revisionCurrent')}{/if}
                        {#if revision.revision === selected.publishedRevision} · {t('studio.revisionPublished')}{/if}
                      </small>
                    </span>
                    <button disabled={restoringRevision || revision.revision === selected.revision} onclick={() => restoreRevision(revision)}>
                      {t('studio.restore')}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
          </section>
        {/if}

        <div class="canvas-layout" class:has-info-card={infoCardVisible()}>
          <article
            class="document-canvas"
            class:wide={content().presentation?.contentWidth === 'wide'}
            class:editorial={content().presentation?.contentWidth === 'editorial'}
            class:compact={content().presentation?.contentWidth === 'compact'}
            class:sans={content().presentation?.fontPreset === 'sans'}
            data-studio-section="structure"
          >
          <div class="canvas-title selected">
            <h1
              style:font-size={`${pageTextSize('titleFontSize', 34)}px`}
              style:text-align={content().presentation?.titleTextAlign || 'left'}
              contenteditable="true"
              use:plainText={selected.title}
              onfocus={() => (selectedBlockID = '@title')}
              oninput={(event) => { selected.title = event.currentTarget.innerText; touch(); }}
            ></h1>
          </div>
          <div class="canvas-summary">
            <p
              style:font-size={`${pageTextSize('summaryFontSize', 17)}px`}
              style:text-align={content().presentation?.summaryTextAlign || 'left'}
              contenteditable="true"
              use:plainText={selected.summary || t('studio.summaryPlaceholder')}
              onfocus={() => (selectedBlockID = '@summary')}
              oninput={(event) => { selected.summary = event.currentTarget.innerText; touch(); }}
            ></p>
          </div>

          <!-- Dentro del lienzo y flotadas: se editan en el mismo sitio en el
               que se publican. Cada ficha se emite junto al bloque en el que
               está anclada, porque un flotado empieza donde aparece en el flujo:
               eso es lo que permite bajarla por la página. -->
          {#snippet canvasInfoCard(card)}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="canvas-info-card"
              class:left={card.side === 'left'}
              class:selected={selectedBlockID === card.id}
              onclick={() => (selectedBlockID = card.id)}
            >
              <div class="card-tools">
                <button
                  class="grip"
                  draggable="true"
                  title={t('studio.infoCardDrag')}
                  aria-label={t('studio.infoCardDrag')}
                  ondragstart={(event) => startInfoCardDrag(card.id, event)}
                  ondragend={endBlockDrag}
                >⠿</button>
                <button title={t('studio.infoCardMoveCardUp')} aria-label={t('studio.infoCardMoveCardUp')} onclick={(event) => { event.stopPropagation(); moveInfoCard(card.id, -1); }}>↑</button>
                <button title={t('studio.infoCardMoveCardDown')} aria-label={t('studio.infoCardMoveCardDown')} onclick={(event) => { event.stopPropagation(); moveInfoCard(card.id, 1); }}>↓</button>
                <button title={t('studio.infoCardFlipSide')} aria-label={t('studio.infoCardFlipSide')} onclick={(event) => { event.stopPropagation(); flipInfoCardSide(card.id); }}>⇄</button>
              </div>
              <StudioInfoCard documentId={selected.id} {card} compact />
            </div>
          {/snippet}

          {#if !documentBlocks().length}
            {#each infoCards() as card (card.id)}{@render canvasInfoCard(card)}{/each}
          {/if}

          {#each documentBlocks() as block, blockIndex (block.id)}
            {#each cardSlots().get(blockIndex) || [] as card (card.id)}{@render canvasInfoCard(card)}{/each}
            <StudioCanvasBlock
              {block}
              documentId={selected.id}
              selected={selectedBlockID === block.id}
              activeBlockID={selectedBlockID}
              onSelect={(blockID) => (selectedBlockID = blockID)}
              onChange={touch}
              onDuplicate={duplicateBlock}
              onRemove={removeBlock}
              onDragStart={startBlockDrag}
              onDragEnd={endBlockDrag}
              onDrop={dropBeforeBlock}
              onDropIntoColumn={dropIntoColumn}
              onAddToColumn={addToColumn}
              onChooseImage={(blockID, columnIndex) => chooseImage({ blockID, columnIndex })}
              onMoveToRoot={moveBlockToRoot}
              {onOpenItem}
            />
          {/each}
          <button
            class="add-any"
            ondragover={(event) => event.preventDefault()}
            ondrop={(event) => { event.preventDefault(); dropAtRootEnd(); }}
            onclick={() => addBlock('paragraph')}
          ><b>＋</b>{t('studio.addAnyBlock')}</button>
          </article>
        </div>

      </div>
      <input class="file-input" bind:this={imageInput} type="file" accept=".jpg,.jpeg,.png,.gif,.webp,image/jpeg,image/png,image/gif,image/webp" onchange={imageSelected} />
    </main>
  {:else if selected}
    <main class="publication-workspace scroll thin">
      {#if error}<div class="studio-error">{t(error)}</div>{/if}
      {#if showRevisions}
        <section class="revision-panel media-revisions">
          <header><b>{t('studio.revisions')}</b><span>{revisions.length}</span></header>
          {#if revisionsLoading}
            <div>{t('common.loading')}</div>
          {:else}
            <div class="revision-list">
              {#each revisions as revision (revision.revision)}
                <div class="revision-row">
                  <span>
                    <b>{revision.title}</b>
                    <small>
                      {t('studio.revisionNumber', { revision: revision.revision })} · {relTime(revision.created)}
                      {#if revision.revision === selected.revision} · {t('studio.revisionCurrent')}{/if}
                      {#if revision.revision === selected.publishedRevision} · {t('studio.revisionPublished')}{/if}
                    </small>
                  </span>
                  <button disabled={restoringRevision || revision.revision === selected.revision} onclick={() => restoreRevision(revision)}>
                    {t('studio.restore')}
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </section>
      {/if}
      <StudioMediaEditor
        document={selected}
        onChange={touch}
        onUpload={uploadMediaAsset}
        onError={mediaEditorError}
      />
    </main>
  {/if}
</section>

<style>
  .studio-new{flex:1 1 auto;height:100%;min-height:0;min-width:0;background:var(--ground);color:var(--ink);overflow:hidden}
  .studio-state{height:100%;display:grid;place-items:center;color:var(--muted);font-size:13px}
  .studio-home{height:100%;overflow:auto;padding:clamp(28px,5vw,64px) clamp(20px,6vw,80px) 70px}
  .studio-home>h2,.document-palette h3{margin:0 0 12px;color:var(--faint);font-size:9px;font-weight:650;letter-spacing:.14em;text-transform:uppercase}
  .studio-home>h2:not(:first-child){margin-top:32px}
  .create-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px;max-width:780px}
  .create-card{min-height:150px;display:flex;flex-direction:column;align-items:flex-start;gap:8px;padding:20px 17px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--card);color:var(--ink);text-align:left;box-shadow:var(--shadow-soft);transition:border-color .14s,transform .14s}
  .create-card:hover{border-color:var(--accent-line);transform:translateY(-2px)}
  .create-glyph,.recent-glyph{display:grid;place-items:center;border-radius:var(--r-md);background:var(--accent-weak);color:var(--accent-2)}
  .create-glyph{width:39px;height:39px;margin-bottom:10px;font-size:16px}
  .create-card b{font-size:13.5px}.create-card small{max-width:210px;color:var(--muted);font-size:11.5px;line-height:1.45}
  .recent-list{display:grid;gap:8px;max-width:780px}
  .recent-item{width:100%;display:flex;align-items:center;gap:12px;padding:11px 14px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card);color:var(--ink);text-align:left}
  .recent-item:hover{border-color:var(--accent-line)}
  .recent-glyph{width:31px;height:31px;flex:none;font-size:13px}
  .recent-meta{min-width:0;display:flex;flex:1;flex-direction:column;gap:2px}
  .recent-meta b{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px}.recent-meta small{color:var(--muted);font-size:11px}
  .recent-state{flex:none;padding:3px 9px;border:1px solid var(--border);border-radius:var(--r-pill);color:var(--muted);font-size:10.5px}
  .recent-state.published{border-color:color-mix(in srgb,#6fd39a 45%,var(--border));color:#6fd39a}
  .home-empty{padding:24px;border:1px dashed var(--border);border-radius:var(--r-lg);color:var(--muted);font-size:12px;text-align:center}
  .studio-error{margin:12px 0;padding:9px 11px;border-left:3px solid #df7474;background:color-mix(in srgb,#df7474 9%,var(--panel));color:#df8585;font-size:12px}

  .document-workspace{height:100%;overflow:auto;display:grid;grid-template-columns:244px minmax(0,1500px);align-items:start;justify-content:center;gap:18px;padding:22px clamp(10px,1.5vw,24px) 70px}
  .sidebar-hidden .document-workspace{grid-template-columns:260px minmax(0,1640px);padding-inline:clamp(10px,1.2vw,20px)}
  .document-palette{position:sticky;top:0;display:grid;gap:8px;padding:12px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel);box-shadow:var(--shadow-soft)}
  .document-palette h3{margin:6px 0 1px}.document-palette h3:first-child{margin-top:0}
  .palette-section{display:grid;gap:8px;margin:3px -5px 0;padding:5px;border:1px solid transparent;border-radius:var(--r-md);transition:border-color .14s,background .14s}
  .palette-section.active{border-color:var(--accent-line);background:var(--accent-weak)}
  .block-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:5px}
  .block-grid button{min-height:31px;display:flex;align-items:center;gap:6px;padding:6px 7px;border:0;border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px;text-align:left}
  .block-grid button:hover{background:var(--raise);color:var(--ink)}.block-grid button b{color:var(--accent-2);font-size:12px}
  .style-options{display:grid;gap:5px}
  .style-options button{width:100%;display:flex;align-items:center;justify-content:space-between;gap:6px;min-height:31px;padding:6px 8px;border:0;border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px}
  .style-options button:hover{background:var(--raise);color:var(--ink)}.style-options button.active{background:var(--accent-weak);color:var(--ink)}
  .style-options small{color:var(--faint);font-size:9px}
  .metadata-palette .metadata-grid{grid-template-columns:1fr;gap:7px}
  .metadata-palette label{display:flex;flex-direction:column;gap:4px;color:var(--muted);font-size:9.5px;letter-spacing:.03em}
  .metadata-palette input{width:100%;min-width:0;padding:7px 8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--ink);font-size:10.5px;outline:0}
  .metadata-palette input:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .link-tool{width:100%;display:flex;align-items:center;gap:7px;margin-top:4px;padding:8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px}
  .link-tool.active{border-color:var(--accent-line);color:var(--accent-2)}
  .document-palette .link-picker{margin:0;padding:7px;border:1px solid var(--accent-line);border-radius:var(--r-sm);background:var(--card)}
  .document-palette .link-picker input{padding:7px 8px;font-size:10.5px}.document-palette .link-results{grid-template-columns:1fr;max-height:180px}
  .document-palette .link-results button{padding:7px;background:var(--raise)}
  .canvas-column{width:100%;max-width:912px;min-width:0;margin:0 auto;transition:max-width .2s}
  .canvas-column.wide{max-width:980px}.canvas-column.editorial{max-width:1180px}.canvas-column.compact{max-width:744px}
  /* Con ficha el lienzo sólo crece lo que ocupa la ficha; el texto conserva su
     medida de lectura (ancho publicado + el relleno del lienzo). */
  .canvas-column.has-info-card{max-width:1052px}.canvas-column.has-info-card.wide{max-width:1212px}.canvas-column.has-info-card.editorial{max-width:1392px}.canvas-column.has-info-card.compact{max-width:932px}
  .sidebar-hidden .canvas-column{max-width:1000px}
  .sidebar-hidden .canvas-column.wide{max-width:1120px}.sidebar-hidden .canvas-column.editorial{max-width:1340px}.sidebar-hidden .canvas-column.compact{max-width:820px}
  .sidebar-hidden .canvas-column.has-info-card{max-width:1052px}.sidebar-hidden .canvas-column.has-info-card.wide{max-width:1212px}.sidebar-hidden .canvas-column.has-info-card.editorial{max-width:1392px}.sidebar-hidden .canvas-column.has-info-card.compact{max-width:932px}
  .canvas-layout{min-width:0}
  /* La ficha flota dentro del lienzo, igual que en la página publicada: no es
     una columna hermana, así que el lienzo vuelve a ser una sola caja. */
  /* z-index alto a propósito: los bloques son position:relative, así que se
     pintaban por encima del flotado y sus bordes y botones tapaban la ficha,
     dejándola imposible de señalar. Con esto la ficha queda arriba y recibe el
     clic y el arrastre. */
  .canvas-info-card{position:relative;z-index:3;float:right;width:clamp(240px,32%,320px);margin:2px 0 18px clamp(16px,2vw,28px);border-radius:var(--r-md);outline:1px solid transparent;outline-offset:3px}
  .canvas-info-card.left{float:left;margin:2px clamp(16px,2vw,28px) 18px 0}
  .canvas-info-card:hover{outline-color:var(--accent-line)}
  .canvas-info-card.selected{outline-color:var(--accent)}
  .card-tools{position:absolute;z-index:1;right:4px;top:-25px;display:flex;gap:2px;padding:3px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--raise);opacity:0;transition:opacity .12s}
  .canvas-info-card:hover .card-tools,.canvas-info-card.selected .card-tools{opacity:1}
  .card-tools button{width:22px;height:20px;display:grid;place-items:center;border:0;border-radius:3px;background:transparent;color:var(--muted);font-size:11px}
  .card-tools button:hover{color:var(--ink);background:var(--card)}
  .card-tools .grip{cursor:grab}
  .revision-panel{margin:0 auto 12px;width:100%;padding:12px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel)}
  .revision-panel header{display:flex;align-items:center;justify-content:space-between;margin-bottom:8px;font-size:11px}.revision-panel header span{color:var(--faint)}
  .revision-list{display:grid;gap:5px}.revision-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px 10px;border-radius:var(--r-sm);background:var(--raise)}
  .revision-row>span{min-width:0;display:flex;flex-direction:column}.revision-row b{font-size:11px}.revision-row small{color:var(--faint);font-size:9px}.revision-row button{padding:5px 8px;font-size:10px}
  /* Mismo corte de palabra que la página publicada, sin depender de la regla de
     agente de usuario que Chrome aplica a los contenteditable. */
  .document-canvas{width:100%;min-height:470px;margin:0 auto;padding:36px clamp(28px,4vw,56px);border:1px solid var(--border);border-radius:var(--r-lg);background:var(--card);box-shadow:var(--shadow-soft);font-family:var(--font-read);transition:padding .2s;overflow-wrap:break-word}
  .document-canvas.compact{padding-inline:44px}.document-canvas.sans{font-family:var(--font)}
  .canvas-title,.canvas-summary{margin:2px -9px;padding:7px 9px;border:1px solid transparent;border-radius:var(--r-sm)}
  .canvas-title:hover,.canvas-title:focus-within,.canvas-summary:hover,.canvas-summary:focus-within{border-color:var(--accent-line);background:color-mix(in srgb,var(--accent) 5%,transparent)}
  .canvas-title h1{margin:0;outline:0;color:var(--ink);line-height:1.1;letter-spacing:-.03em}
  .canvas-summary p{min-height:28px;margin:0;outline:0;color:var(--muted);line-height:1.5}
  .add-any{clear:both;width:100%;display:flex;align-items:center;justify-content:center;gap:8px;margin-top:14px;padding:11px;border:1px dashed var(--border);border-radius:var(--r-md);background:transparent;color:var(--faint);font-size:12px}
  .add-any:hover{border-color:var(--accent-line);color:var(--ink)}.add-any b{width:22px;height:22px;display:grid;place-items:center;border-radius:var(--r-sm);background:var(--accent-weak);color:var(--accent-2)}
  .metadata-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}
  .document-cover{width:100%;min-height:180px;display:grid;place-items:center;gap:8px;overflow:hidden;border:1px dashed var(--border);border-radius:var(--r-md);background:var(--card);color:var(--muted)}
  .document-cover:hover{border-color:var(--accent-line);color:var(--ink)}.document-cover.ready{border-style:solid}
  .document-cover b{font-size:24px;color:var(--accent-2)}.document-cover span{font-size:12px}
  .document-cover :global(img),.document-cover :global(.placeholder){width:100%;height:clamp(180px,28vw,360px);object-fit:cover;border-radius:0}
  .cover-actions{display:flex;justify-content:flex-end;gap:7px;margin-top:9px}.cover-actions button{padding:7px 10px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px}
  .cover-actions button:hover{border-color:var(--accent-line);color:var(--ink)}.cover-actions .remove{color:#df7474}
  .cover-palette .document-cover{min-height:92px}
  .cover-palette .document-cover :global(img),.cover-palette .document-cover :global(.placeholder){height:110px;min-height:0}
  .cover-palette .cover-actions{display:grid;grid-template-columns:1fr 1fr;gap:5px;margin-top:0}
  .cover-palette .cover-actions button{min-width:0;padding-inline:5px;font-size:9.5px}

  .preview-mode{height:100%;overflow:auto;padding:28px clamp(18px,5vw,70px) 70px;background:var(--panel-2)}
  .publication-workspace{height:100%;overflow:auto;padding:34px clamp(22px,6vw,80px) 70px}
  .media-revisions{max-width:1320px;margin-bottom:16px}
  .file-input{display:none}

  :global(:root[data-skin="retro"]) .create-card:hover{transform:none}
  :global(:root[data-skin="retro"]) :is(.create-card,.document-canvas,.document-palette){box-shadow:var(--shadow)}
  @media(max-width:1080px){
    .document-workspace{grid-template-columns:1fr;justify-content:stretch}.document-palette{position:static;max-width:980px;width:100%;margin:0 auto}
    .block-grid{grid-template-columns:repeat(5,minmax(0,1fr))}.style-options{grid-template-columns:repeat(3,minmax(0,1fr))}
  }
  @media(max-width:900px){
    .canvas-info-card,.canvas-info-card.left{float:none;width:100%;margin:0 0 20px}
  }
  @media(max-width:700px){
    .studio-home{padding:24px 14px 50px}.create-grid{grid-template-columns:1fr}.create-card{min-height:124px}
    .document-workspace,.publication-workspace{padding:18px 14px 50px}.block-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.style-options{grid-template-columns:1fr}
    .document-canvas,.document-canvas.compact{padding:26px 20px}.metadata-grid{grid-template-columns:1fr}
    .recent-state{display:none}
  }

</style>
