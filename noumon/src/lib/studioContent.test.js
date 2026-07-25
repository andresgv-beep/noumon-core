import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createStudioPage,
  moveStudioPage,
  normalizeStudioContent,
  normalizeStudioDocument,
  removeStudioPage,
  renameStudioPage,
  studioDocumentBlocks,
  studioPage,
} from './studioContent.js';

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

test('normalizes the document info card without changing page content', () => {
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
      pages: [{ id: 'p1', title: 'Inicio', blocks: [] }],
    },
  });

  assert.deepEqual(document.content.infoCard, {
    assetId: 'asset-card',
    caption: 'Retrato',
    rows: [{ label: 'Autor', value: 'Equipo local' }],
  });
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
