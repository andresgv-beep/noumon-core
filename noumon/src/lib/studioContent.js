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
          infoCards: normalizeStudioInfoCards(page?.infoCards),
        }))
      : [{ id: 'p1', title: String(documentTitle || 'Page 1'), blocks: [], infoCards: [] }];
    const normalized = {
      ...source,
      schemaVersion: STUDIO_CONTENT_SCHEMA_VERSION,
      pages: adoptLegacyInfoCard(pages, source.infoCard),
    };
    delete normalized.blocks;
    delete normalized.infoCard;
    return normalized;
  }
  if (Number(source.schemaVersion) !== 1) return source;
  const normalized = {
    ...source,
    schemaVersion: STUDIO_CONTENT_SCHEMA_VERSION,
    pages: adoptLegacyInfoCard([{
      id: 'p1',
      title: String(documentTitle || 'Page 1'),
      blocks: Array.isArray(source.blocks) ? source.blocks : [],
      infoCards: [],
    }], source.infoCard),
  };
  delete normalized.blocks;
  delete normalized.infoCard;
  return normalized;
}

/**
 * Las fichas vivían en el documento (`content.infoCard`, una sola y común a
 * todas las páginas). Ahora viven en cada página y son varias, así que la ficha
 * antigua se adopta como primera ficha de la primera página. Sólo se adopta si
 * esa página no tiene ya fichas propias: si las tiene, la migración ya ocurrió.
 */
function adoptLegacyInfoCard(pages, legacy) {
  if (!pages.length || pages[0].infoCards.length) return pages;
  if (!studioInfoCardHasContent(legacy)) return pages;
  pages[0].infoCards = [normalizeStudioInfoCard(legacy, 0)];
  return pages;
}

// Encuadre de la imagen de la ficha: 'natural' respeta la forma original de la
// imagen (no recorta); el resto la recorta a una proporción fija.
export const STUDIO_INFO_RATIOS = ['natural', 'wide', 'square', 'portrait', 'tall'];

const STUDIO_INFO_RATIO_VALUES = {
  wide: '16 / 9',
  square: '1 / 1',
  portrait: '3 / 4',
  tall: '2 / 3',
};

function normalizeFocus(value) {
  const number = Math.round(Number(value));
  return Number.isFinite(number) ? Math.min(100, Math.max(0, number)) : 50;
}

/** Proporción CSS del encuadre, o '' cuando la imagen va sin recortar. */
export function studioInfoRatioValue(ratio) {
  return STUDIO_INFO_RATIO_VALUES[ratio] || '';
}

/** Punto focal como valor de object-position: qué parte se conserva al recortar. */
export function studioInfoFocus(card) {
  return `${normalizeFocus(card?.imageFocusX)}% ${normalizeFocus(card?.imageFocusY)}%`;
}

/** Lado del artículo al que se pega la ficha. */
export const STUDIO_INFO_SIDES = ['right', 'left'];
export const STUDIO_MAX_INFO_CARDS = 4;

/** ¿La ficha tiene algo que mostrar? Una ficha vacía no se publica. */
export function studioInfoCardHasContent(card) {
  return !!(
    card?.assetId ||
    String(card?.caption || '').trim() ||
    card?.rows?.some((row) => String(row?.label || '').trim() || String(row?.value || '').trim())
  );
}

export function normalizeStudioInfoCards(cards) {
  return (Array.isArray(cards) ? cards : [])
    .slice(0, STUDIO_MAX_INFO_CARDS)
    .map((card, index) => normalizeStudioInfoCard(card, index));
}

export function normalizeStudioInfoCard(infoCard, index = 0) {
  const source = infoCard && typeof infoCard === 'object' && !Array.isArray(infoCard)
    ? infoCard
    : {};
  return {
    // Identidad estable dentro de la página: sirve de clave en el editor y de
    // destino al subir una imagen. Se deriva del orden cuando falta, para que
    // normalizar dos veces el mismo contenido dé el mismo resultado.
    id: /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(String(source.id || ''))
      ? String(source.id)
      : `card-${index + 1}`,
    side: STUDIO_INFO_SIDES.includes(source.side) ? source.side : 'right',
    // Posición dentro de la página: índice del bloque junto al que empieza a
    // flotar. 0 es arriba del todo. Un flotado arranca donde aparece en el
    // flujo, así que esto es lo que permite bajar la ficha por el artículo.
    anchor: Math.max(0, Math.round(Number(source.anchor)) || 0),
    assetId: String(source.assetId || ''),
    caption: String(source.caption || ''),
    imageRatio: STUDIO_INFO_RATIOS.includes(source.imageRatio) ? source.imageRatio : 'natural',
    imageFocusX: normalizeFocus(source.imageFocusX),
    imageFocusY: normalizeFocus(source.imageFocusY),
    rows: Array.isArray(source.rows)
      ? source.rows.slice(0, 40).map((row) => ({
          label: String(row?.label || ''),
          value: String(row?.value || ''),
        }))
      : [],
  };
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

/**
 * Encabezado de la página publicada. Es SIEMPRE el título del documento: es el
 * que se escribe en el lienzo, y el que acompañan la entradilla, el autor y la
 * fecha en esa misma cabecera.
 *
 * El título de página es otra cosa: nombra la página en el menú de contenidos.
 * Encabezar con él dejaba invisible el título del usuario, porque la primera
 * página nace con nombre propio y ese nombre ganaba siempre. Sólo se usa como
 * último recurso, si el documento no tiene título.
 */
export function studioDocumentHeading(document, pageId = '') {
  const title = String(document?.title || '').trim();
  if (title) return title;
  return String(studioPage(document, pageId)?.title || '');
}

export function studioDocumentBlocks(document, pageId = '') {
  if (!isDocumentTemplate(document?.templateKey)) {
    return Array.isArray(document?.content?.blocks) ? document.content.blocks : [];
  }
  const page = studioPage(document, pageId);
  return Array.isArray(page?.blocks) ? page.blocks : [];
}

/**
 * Fichas de una página, resolviendo también los documentos anteriores a las
 * fichas por página. Hace falta porque el lector publicado no normaliza el
 * contenido: sólo el editor lo hace al abrir el documento.
 */
export function studioPageInfoCards(document, pageId = '') {
  const page = studioPage(document, pageId);
  if (!page) return [];
  const own = normalizeStudioInfoCards(page.infoCards);
  if (own.length) return own;
  const legacy = document?.content?.infoCard;
  if (studioPages(document)[0] === page && studioInfoCardHasContent(legacy)) {
    return [normalizeStudioInfoCard(legacy, 0)];
  }
  return [];
}

export function createStudioInfoCard(page, side = 'right') {
  if (!page) return null;
  if (!Array.isArray(page.infoCards)) page.infoCards = [];
  if (page.infoCards.length >= STUDIO_MAX_INFO_CARDS) return null;
  const used = new Set(page.infoCards.map((card) => card.id));
  const random = globalThis.crypto?.randomUUID?.().replaceAll('-', '')
    || `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`;
  let id = `card-${random}`;
  let suffix = 2;
  while (used.has(id)) id = `card-${random}-${suffix++}`;
  const card = normalizeStudioInfoCard({ id, side });
  page.infoCards.push(card);
  return card;
}

export function removeStudioInfoCard(page, cardId) {
  const index = (page?.infoCards || []).findIndex((card) => card.id === cardId);
  if (index < 0) return false;
  page.infoCards.splice(index, 1);
  return true;
}

/** Sube o baja la ficha por la página moviendo su anclaje un bloque. */
export function moveStudioInfoCard(page, cardId, delta, blockCount) {
  const card = (page?.infoCards || []).find((candidate) => candidate.id === cardId);
  if (!card) return false;
  const current = studioInfoCardAnchor(card, blockCount);
  const next = clampAnchor(current + Math.sign(Number(delta) || 0), blockCount);
  if (next === current) return false;
  card.anchor = next;
  return true;
}

function clampAnchor(anchor, blockCount) {
  const last = Math.max(0, (Number(blockCount) || 0) - 1);
  return Math.min(Math.max(0, Math.round(Number(anchor)) || 0), last);
}

/** Anclaje efectivo: si el bloque de referencia ya no existe, acompaña al último. */
export function studioInfoCardAnchor(card, blockCount) {
  return clampAnchor(card?.anchor, blockCount);
}

/**
 * Reparte las fichas por posición: devuelve un mapa índice-de-bloque → fichas
 * que empiezan a flotar ahí. Las de la izquierda van primero para que floten en
 * su orden. Lo usan por igual el editor y la página publicada.
 */
export function studioInfoCardsByAnchor(cards, blockCount) {
  const slots = new Map();
  for (const card of cards) {
    const at = studioInfoCardAnchor(card, blockCount);
    if (!slots.has(at)) slots.set(at, []);
    slots.get(at).push(card);
  }
  for (const group of slots.values()) {
    group.sort((a, b) => (a.side === 'left' ? 0 : 1) - (b.side === 'left' ? 0 : 1));
  }
  return slots;
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
  // Página nueva = página limpia: sin bloques y sin fichas heredadas.
  const page = { id, title: String(title || '').trim(), blocks: [], infoCards: [] };
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
