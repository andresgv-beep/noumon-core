import assert from 'node:assert/strict';
import test from 'node:test';

import { messages } from './messages.js';

test('keeps the Spanish and English interface dictionaries in exact parity', () => {
  assert.deepEqual(
    Object.keys(messages.en).sort(),
    Object.keys(messages.es).sort(),
  );
});

test('ships every page-manager message in both interface languages', () => {
  const keys = [
    'studio.pages',
    'studio.pagesCount',
    'studio.blocksCount',
    'studio.addPage',
    'studio.pageLimitReached',
    'studio.newPageTitle',
    'studio.pageTitle',
    'studio.renamePage',
    'studio.movePageUp',
    'studio.movePageDown',
    'studio.removePage',
    'studio.keepOnePage',
    'studio.removePageConfirm',
  ];
  for (const language of ['es', 'en']) {
    for (const key of keys) {
      assert.equal(typeof messages[language][key], 'string', `${language}.${key}`);
      assert.notEqual(messages[language][key].trim(), '', `${language}.${key}`);
    }
  }
});

test('ships every info-card message in both interface languages', () => {
  const keys = [
    'studio.infoCard',
    'studio.infoCardDesc',
    'studio.infoCardAddImage',
    'studio.infoCardReplaceImage',
    'studio.infoCardRemoveImage',
    'studio.infoCardCaption',
    'studio.infoCardCaptionPlaceholder',
    'studio.infoCardRows',
    'studio.infoCardAddRow',
    'studio.infoCardLabel',
    'studio.infoCardValue',
    'studio.infoCardMoveUp',
    'studio.infoCardMoveDown',
    'studio.infoCardRemoveRow',
    'studio.infoCardEmpty',
    'documents.infoCard',
    'documents.infoCardImage',
  ];
  for (const language of ['es', 'en']) {
    for (const key of keys) {
      assert.equal(typeof messages[language][key], 'string', `${language}.${key}`);
      assert.notEqual(messages[language][key].trim(), '', `${language}.${key}`);
    }
  }
});
