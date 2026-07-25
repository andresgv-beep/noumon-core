import assert from 'node:assert/strict';
import test from 'node:test';

import {
  normalizeStudioContent,
  normalizeStudioDocument,
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
