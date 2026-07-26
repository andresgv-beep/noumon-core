import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createStudioInfoCard,
  createStudioPage,
  moveStudioInfoCard,
  moveStudioPage,
  normalizeStudioContent,
  normalizeStudioDocument,
  removeStudioPage,
  renameStudioPage,
  studioDocumentBlocks,
  studioDocumentHeading,
  studioInfoCardsByAnchor,
  studioNavGroups,
  studioPage,
  studioPageInfoCards,
} from './studioContent.js';

test('keeps side cards on their own page and creates new pages clean', () => {
  const document = normalizeStudioDocument({
    templateKey: 'document',
    title: 'Archivo local',
    metadata: {},
    content: { schemaVersion: 2, pages: [{ id: 'p1', title: 'Inicio', blocks: [] }] },
  });

  const right = createStudioInfoCard(studioPage(document, 'p1'));
  const left = createStudioInfoCard(studioPage(document, 'p1'), 'left');
  right.caption = 'Ficha derecha';
  left.caption = 'Ficha izquierda';

  const second = createStudioPage(document, 'Anexo');
  assert.deepEqual(second.infoCards, []);
  assert.deepEqual(second.blocks, []);
  assert.deepEqual(studioPageInfoCards(document, second.id), []);

  const first = studioPageInfoCards(document, 'p1');
  assert.deepEqual(first.map((card) => card.side), ['right', 'left']);
  assert.notEqual(right.id, left.id);
});

test('anchors side cards to a block so they can move down the page', () => {
  const document = normalizeStudioDocument({
    templateKey: 'document',
    title: 'Archivo local',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [{
        id: 'p1',
        title: 'Inicio',
        blocks: [{ id: 'a', type: 'paragraph' }, { id: 'b', type: 'paragraph' }, { id: 'c', type: 'paragraph' }],
      }],
    },
  });
  const page = studioPage(document, 'p1');
  const card = createStudioInfoCard(page);
  assert.equal(card.anchor, 0);

  assert.equal(moveStudioInfoCard(page, card.id, 1, 3), true);
  assert.equal(card.anchor, 1);
  assert.equal(studioInfoCardsByAnchor([card], 3).get(1)[0].id, card.id);

  // No baja más allá del último bloque ni sube por encima del primero.
  moveStudioInfoCard(page, card.id, 1, 3);
  assert.equal(moveStudioInfoCard(page, card.id, 1, 3), false);
  assert.equal(card.anchor, 2);
  moveStudioInfoCard(page, card.id, -1, 3);
  moveStudioInfoCard(page, card.id, -1, 3);
  assert.equal(moveStudioInfoCard(page, card.id, -1, 3), false);
  assert.equal(card.anchor, 0);

  // Un anclaje que apunta más allá del final acompaña al último bloque en vez
  // de desaparecer de la página.
  card.anchor = 99;
  assert.equal(studioInfoCardsByAnchor([card], 3).get(2)[0].id, card.id);
});

test('shows a pre-migration info card on the first page without normalizing', () => {
  // El lector publicado no normaliza el contenido: tiene que resolver por su
  // cuenta los documentos guardados antes de las fichas por página.
  const document = {
    id: 'doc-1',
    templateKey: 'document',
    content: {
      schemaVersion: 2,
      infoCard: { assetId: 'asset-card', caption: 'Retrato', rows: [] },
      pages: [
        { id: 'p1', title: 'Inicio', blocks: [] },
        { id: 'p2', title: 'Anexo', blocks: [] },
      ],
    },
  };

  assert.equal(studioPageInfoCards(document, 'p1').length, 1);
  assert.equal(studioPageInfoCards(document, 'p1')[0].caption, 'Retrato');
  assert.deepEqual(studioPageInfoCards(document, 'p2'), []);
});

test('normalizes a legacy document into one stable page without losing blocks', () => {
  const legacy = {
    schemaVersion: 1,
    classification: { workType: 'manual' },
    presentation: { contentWidth: 'wide' },
    blocks: [{ id: 'intro', type: 'paragraph', text: 'Contenido' }],
  };
  const document = {
    templateKey: 'document',
    title: 'Manual local',
    content: normalizeStudioContent(legacy, 'Manual local', 'document'),
  };

  assert.equal(document.content.schemaVersion, 2);
  assert.equal(document.content.blocks, undefined);
  assert.deepEqual(document.content.pages.map(({ id, title }) => ({ id, title })), [
    { id: 'p1', title: 'Manual local' },
  ]);
  assert.strictEqual(studioDocumentBlocks(document)[0], legacy.blocks[0]);
  assert.equal(legacy.schemaVersion, 1);
  assert.equal(legacy.blocks.length, 1);
});

test('keeps pages and resolves the requested active page', () => {
  const document = {
    templateKey: 'technical',
    title: 'Guía',
    content: normalizeStudioContent({
      schemaVersion: 2,
      pages: [
        { id: 'inicio', title: 'Inicio', blocks: [{ id: 'a', type: 'paragraph', text: 'A' }] },
        { id: 'anexo', title: 'Anexo', blocks: [{ id: 'b', type: 'paragraph', text: 'B' }] },
      ],
    }, 'Guía', 'technical'),
  };

  assert.equal(studioPage(document, 'anexo').title, 'Anexo');
  assert.equal(studioDocumentBlocks(document, 'anexo')[0].id, 'b');
  assert.equal(studioDocumentBlocks(document, 'inexistente')[0].id, 'a');
});

test('adopts the legacy document info card as the first card of the first page', () => {
  const document = normalizeStudioDocument({
    templateKey: 'document',
    title: 'Archivo local',
    metadata: {},
    content: {
      schemaVersion: 2,
      infoCard: {
        assetId: 'asset-card',
        caption: 'Retrato',
        rows: [{ label: 'Autor', value: 'Equipo local' }],
      },
      pages: [
        { id: 'p1', title: 'Inicio', blocks: [] },
        { id: 'p2', title: 'Anexo', blocks: [] },
      ],
    },
  });

  assert.equal(document.content.infoCard, undefined);
  assert.deepEqual(document.content.pages[0].infoCards, [{
    id: 'card-1',
    side: 'right',
    anchor: 0,
    assetId: 'asset-card',
    caption: 'Retrato',
    imageRatio: 'natural',
    imageFocusX: 50,
    imageFocusY: 50,
    rows: [{ label: 'Autor', value: 'Equipo local' }],
  }]);
  // La ficha antigua era común a todas las páginas; ahora sólo acompaña a la
  // primera y el resto nace limpio.
  assert.deepEqual(document.content.pages[1].infoCards, []);
  assert.equal(document.content.pages[0].id, 'p1');
});

test('does not migrate Cabinet or Moments payloads to the document schema', () => {
  const media = normalizeStudioContent(
    { schemaVersion: 1, blocks: [] },
    'Audio',
    'cabinet.audio',
  );

  assert.equal(media.schemaVersion, 1);
  assert.deepEqual(media.blocks, []);
  assert.equal(media.pages, undefined);
});

test('does not downgrade an unknown future document schema', () => {
  const future = {
    schemaVersion: 3,
    pages: [{ id: 'future', title: 'Future', blocks: [] }],
    futureField: true,
  };

  assert.strictEqual(
    normalizeStudioContent(future, 'Future', 'document'),
    future,
  );
});

test('applies the same V1 normalization to a recovered document envelope', () => {
  const recovered = normalizeStudioDocument({
    id: 'doc-recovery',
    templateKey: 'document',
    title: 'Copia recuperada',
    metadata: '{}',
    content: JSON.stringify({
      schemaVersion: 1,
      blocks: [{ id: 'recovered', type: 'paragraph', text: 'Sin guardar' }],
    }),
  });

  assert.deepEqual(recovered.metadata, {});
  assert.equal(recovered.content.schemaVersion, 2);
  assert.equal(recovered.content.pages[0].id, 'p1');
  assert.equal(recovered.content.pages[0].blocks[0].text, 'Sin guardar');
});

test('creates, renames, and reorders pages without changing their identities', () => {
  const document = normalizeStudioDocument({
    templateKey: 'document',
    title: 'Manual',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [
        { id: 'p1', title: 'Inicio', blocks: [] },
        { id: 'p2', title: 'Apéndice', blocks: [] },
      ],
    },
  });

  const page = createStudioPage(document, 'Referencias', 'p3');
  assert.equal(page.id, 'p3');
  assert.equal(renameStudioPage(document, 'p3', 'Fuentes'), true);
  assert.equal(renameStudioPage(document, 'p3', '  '), false);
  assert.equal(moveStudioPage(document, 'p3', -1), true);
  assert.deepEqual(document.content.pages.map(({ id, title }) => [id, title]), [
    ['p1', 'Inicio'],
    ['p3', 'Fuentes'],
    ['p2', 'Apéndice'],
  ]);
});

test('deletes a page with deterministic neighbour selection but preserves the final page', () => {
  const document = normalizeStudioDocument({
    templateKey: 'document',
    title: 'Manual',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [
        { id: 'p1', title: 'Primera', blocks: [] },
        { id: 'p2', title: 'Segunda', blocks: [{ id: 'private', type: 'paragraph', text: 'Borrador' }] },
        { id: 'p3', title: 'Tercera', blocks: [] },
      ],
    },
  });

  const middle = removeStudioPage(document, 'p2');
  assert.equal(middle.removed.id, 'p2');
  assert.equal(middle.nextPage.id, 'p3');
  const last = removeStudioPage(document, 'p3');
  assert.equal(last.nextPage.id, 'p1');
  assert.equal(removeStudioPage(document, 'p1'), null);
  assert.deepEqual(document.content.pages.map((page) => page.id), ['p1']);
});

test('keeps edits from multiple pages across an in-flight save snapshot and recovery round trip', () => {
  const document = normalizeStudioDocument({
    id: 'doc-pages',
    templateKey: 'document',
    title: 'Cuaderno',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [
        { id: 'p1', title: 'Uno', blocks: [{ id: 'a', type: 'paragraph', text: 'Antes' }] },
        { id: 'p2', title: 'Dos', blocks: [{ id: 'b', type: 'paragraph', text: '' }] },
      ],
    },
  });

  studioDocumentBlocks(document, 'p1')[0].text = 'Guardado en vuelo';
  const inFlight = JSON.parse(JSON.stringify(document));
  studioDocumentBlocks(document, 'p2')[0].text = 'Escrito mientras guardaba';

  assert.equal(studioDocumentBlocks(inFlight, 'p2')[0].text, '');
  const recovered = normalizeStudioDocument(JSON.parse(JSON.stringify(document)));
  assert.equal(studioDocumentBlocks(recovered, 'p1')[0].text, 'Guardado en vuelo');
  assert.equal(studioDocumentBlocks(recovered, 'p2')[0].text, 'Escrito mientras guardaba');
});

test('el encabezado publicado es el título del documento, no el de la página', () => {
  // El fallo real: la primera página nace con nombre propio ("Documento sin
  // título" heredado del relleno de la interfaz) y ese nombre encabezaba el
  // artículo, así que el título escrito por el usuario no aparecía NUNCA.
  const document = normalizeStudioDocument({
    id: 'd1',
    templateKey: 'document',
    title: 'Prueba de indexación Noumon',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [
        { id: 'p1', title: 'Documento sin título', blocks: [] },
        { id: 'p2', title: 'Segunda parte', blocks: [] },
      ],
    },
  });

  assert.equal(studioDocumentHeading(document, 'p1'), 'Prueba de indexación Noumon');
  // Cambiar de página no cambia de artículo: el encabezado se queda.
  assert.equal(studioDocumentHeading(document, 'p2'), 'Prueba de indexación Noumon');
});

test('sin título de documento, el encabezado cae al de la página', () => {
  const document = normalizeStudioDocument({
    id: 'd2',
    templateKey: 'document',
    title: '   ',
    metadata: {},
    content: {
      schemaVersion: 2,
      pages: [{ id: 'p1', title: 'Capítulo uno', blocks: [] }],
    },
  });

  assert.equal(studioDocumentHeading(document, 'p1'), 'Capítulo uno');
});

test('groups pages into menu sections without ever dropping one', () => {
  const pages = [
    { id: 'p1', title: 'Portada', section: '' },
    { id: 'p2', title: 'Instalación', section: 'Documentos' },
    { id: 'p3', title: 'Uso diario', section: 'Documentos' },
    { id: 'p4', title: 'Capturas', section: 'Fotos' },
    { id: 'p5', title: 'Sin grupo otra vez', section: '' },
  ];
  const groups = studioNavGroups(pages);

  assert.deepEqual(groups.map((group) => group.section), ['', 'Documentos', 'Fotos', '']);
  assert.deepEqual(groups.map((group) => group.pages.length), [1, 2, 1, 1]);
  // La garantía que sostiene todo el menú: agrupar no puede perder páginas.
  assert.deepEqual(
    groups.flatMap((group) => group.pages.map((page) => page.id)),
    pages.map((page) => page.id),
  );
});
