export const STUDIO_CONTENT_SCHEMA_VERSION = 2;

export function isDocumentTemplate(templateKey) {
  return !String(templateKey || '').startsWith('cabinet.')
    && !String(templateKey || '').startsWith('moments.');
}

export function normalizeStudioContent(content, documentTitle = '', templateKey = 'document') {
  const source = content && typeof content === 'object' && !Array.isArray(content)
    ? content
    : {};
  if (!isDocumentTemplate(templateKey)) {
    return {
      ...source,
      schemaVersion: Number(source.schemaVersion) || 1,
      blocks: Array.isArray(source.blocks) ? source.blocks : [],
    };
  }
  if (Number(source.schemaVersion) === STUDIO_CONTENT_SCHEMA_VERSION) {
    const pages = Array.isArray(source.pages) && source.pages.length
      ? source.pages.map((page, index) => ({
          ...page,
          id: String(page?.id || `p${index + 1}`),
          title: String(page?.title || documentTitle || `Page ${index + 1}`),
          blocks: Array.isArray(page?.blocks) ? page.blocks : [],
        }))
      : [{ id: 'p1', title: String(documentTitle || 'Page 1'), blocks: [] }];
    const normalized = {
      ...source,
      schemaVersion: STUDIO_CONTENT_SCHEMA_VERSION,
      pages,
    };
    delete normalized.blocks;
    return normalized;
  }
  if (Number(source.schemaVersion) !== 1) return source;
  const normalized = {
    ...source,
    schemaVersion: STUDIO_CONTENT_SCHEMA_VERSION,
    pages: [{
      id: 'p1',
      title: String(documentTitle || 'Page 1'),
      blocks: Array.isArray(source.blocks) ? source.blocks : [],
    }],
  };
  delete normalized.blocks;
  return normalized;
}

export function normalizeStudioDocument(document) {
  if (!document || typeof document !== 'object') return document;
  document.content = typeof document.content === 'string'
    ? JSON.parse(document.content)
    : document.content;
  document.metadata = typeof document.metadata === 'string'
    ? JSON.parse(document.metadata)
    : document.metadata;
  if (!document.metadata || Array.isArray(document.metadata)) document.metadata = {};
  document.content = normalizeStudioContent(
    document.content,
    document.title,
    document.templateKey,
  );
  return document;
}

export function studioPages(document) {
  if (!isDocumentTemplate(document?.templateKey)) return [];
  return Array.isArray(document?.content?.pages) ? document.content.pages : [];
}

export function studioPage(document, pageId = '') {
  const pages = studioPages(document);
  return pages.find((page) => page.id === pageId) || pages[0] || null;
}

export function studioDocumentBlocks(document, pageId = '') {
  if (!isDocumentTemplate(document?.templateKey)) {
    return Array.isArray(document?.content?.blocks) ? document.content.blocks : [];
  }
  const page = studioPage(document, pageId);
  return Array.isArray(page?.blocks) ? page.blocks : [];
}

export function createStudioPage(document, title, requestedId = '') {
  const pages = studioPages(document);
  if (!pages.length && document?.content) document.content.pages = [];
  const used = new Set(pages.map((page) => page.id));
  let id = String(requestedId || '').trim();
  if (!/^[A-Za-z0-9._:-]{1,128}$/.test(id) || used.has(id)) {
    const random = globalThis.crypto?.randomUUID?.().replaceAll('-', '')
      || `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`;
    id = `page-${random}`;
    let suffix = 2;
    while (used.has(id)) id = `page-${random}-${suffix++}`;
  }
  const page = { id, title: String(title || '').trim(), blocks: [] };
  document.content.pages.push(page);
  return page;
}

export function renameStudioPage(document, pageId, title) {
  const page = studioPages(document).find((candidate) => candidate.id === pageId);
  const nextTitle = String(title || '').trim();
  if (!page || !nextTitle || nextTitle.length > 240) return false;
  if (page.title === nextTitle) return false;
  page.title = nextTitle;
  return true;
}

export function moveStudioPage(document, pageId, delta) {
  const pages = studioPages(document);
  const from = pages.findIndex((page) => page.id === pageId);
  const to = from + Math.sign(Number(delta) || 0);
  if (from < 0 || to < 0 || to >= pages.length || from === to) return false;
  const [page] = pages.splice(from, 1);
  pages.splice(to, 0, page);
  return true;
}

export function removeStudioPage(document, pageId) {
  const pages = studioPages(document);
  if (pages.length <= 1) return null;
  const index = pages.findIndex((page) => page.id === pageId);
  if (index < 0) return null;
  const [removed] = pages.splice(index, 1);
  return {
    removed,
    nextPage: pages[Math.min(index, pages.length - 1)],
  };
}
