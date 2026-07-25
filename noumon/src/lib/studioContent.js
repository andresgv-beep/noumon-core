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
