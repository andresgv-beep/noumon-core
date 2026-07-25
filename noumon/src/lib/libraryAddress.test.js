import assert from 'node:assert/strict';
import test from 'node:test';

import { formatLibraryAddress, parseLibraryAddress } from './libraryAddress.js';

test('formats and parses a deep link to a Studio page', () => {
  const address = formatLibraryAddress({
    kind: 'item',
    itemId: 'studio:doc-local',
    pageId: 'page-introduction',
    open: { provider: 'studio' },
  });

  assert.equal(address, 'library://item/studio%3Adoc-local/page-introduction');
  assert.deepEqual(parseLibraryAddress(address), {
    kind: 'item',
    itemId: 'studio:doc-local',
    pageId: 'page-introduction',
  });
});

test('keeps document-only item addresses compatible', () => {
  const address = 'library://item/studio%3Adoc-local';
  assert.deepEqual(parseLibraryAddress(address), {
    kind: 'item',
    itemId: 'studio:doc-local',
    pageId: '',
  });
});
