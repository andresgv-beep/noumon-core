import test from 'node:test';
import assert from 'node:assert/strict';

import {
  collectStudioInlineLinks,
  inline,
  inlineText,
  removeStudioPageLinks,
  studioInlineLinks,
} from './studioEditable.js';

function text(value) {
  return { nodeType: 3, nodeValue: value };
}

function element(tagName, childNodes = [], dataset = {}) {
  return { nodeType: 1, tagName, childNodes, dataset };
}

test('inline renders safe page and item links after escaping content', () => {
  const html = inline(
    'Ir a [[page:p2|**Detalles**]] y [[item:studio:abc|<fuente>]].',
    { pageIDs: ['p1', 'p2'] },
  );
  assert.match(html, /data-studio-link-kind="page"/);
  assert.match(html, /data-studio-link-id="p2"/);
  assert.match(html, /<strong>Detalles<\/strong>/);
  assert.match(html, /data-studio-link-kind="item"/);
  assert.match(html, /&lt;fuente&gt;/);
  assert.doesNotMatch(html, /<fuente>/);
  assert.doesNotMatch(html, /is-broken/);
});

test('inline marks only unresolved page links as broken', () => {
  const html = inline(
    '[[page:missing|Pendiente]] [[item:missing|Contenido]]',
    { pageIDs: ['p1'] },
  );
  assert.match(html, /studio-inline-link-page is-broken/);
  assert.doesNotMatch(
    html.match(/<a class="studio-inline-link studio-inline-link-item[^"]*"/)?.[0] || '',
    /is-broken/,
  );
});

test('inlineText preserves link syntax across contenteditable updates', () => {
  const root = element('DIV', [
    text('Antes '),
    element('A', [text('página dos')], {
      studioLinkKind: 'page',
      studioLinkId: 'p2',
    }),
    text(' después'),
  ]);
  assert.equal(inlineText(root), 'Antes [[page:p2|página dos]] después');
});

test('inlineText unwraps an empty link instead of creating invalid syntax', () => {
  const root = element('DIV', [
    text('Antes'),
    element('A', [], {
      studioLinkKind: 'page',
      studioLinkId: 'p2',
    }),
    text(' despues'),
  ]);
  assert.equal(inlineText(root), 'Antes despues');
});

test('removeStudioPageLinks preserves labels and cleans legacy empty links', () => {
  const document = {
    pages: [{
      blocks: [
        { text: 'Lee [[page:p2|la segunda pagina]] ahora.' },
        { text: 'Roto [[page:p2|]] y [[page:p3|otro destino]].' },
      ],
    }],
  };
  const result = removeStudioPageLinks(document, 'p2');
  assert.equal(result.removed, 2);
  assert.equal(document.pages[0].blocks[0].text, 'Lee la segunda pagina ahora.');
  assert.equal(document.pages[0].blocks[1].text, 'Roto  y [[page:p3|otro destino]].');
});

test('collectors expose legacy empty page links as invalid and rendering hides their syntax', () => {
  assert.deepEqual(studioInlineLinks('[[page:p2|]]'), [{
    syntax: '[[page:p2|]]',
    kind: 'page',
    id: 'p2',
    label: '',
    invalid: true,
  }]);
  assert.equal(inline('Antes [[page:p2|]] despues', { pageIDs: [] }), 'Antes  despues');
});

test('link collectors find nested document links without interpreting other text', () => {
  assert.deepEqual(studioInlineLinks('[[page:p2|Dos]]'), [{
    syntax: '[[page:p2|Dos]]',
    kind: 'page',
    id: 'p2',
    label: 'Dos',
  }]);
  assert.deepEqual(
    collectStudioInlineLinks({
      pages: [{ blocks: [{ text: '[[page:p3|Tres]]' }] }],
      title: 'Sin enlace',
    }).map(({ kind, id, label }) => ({ kind, id, label })),
    [{ kind: 'page', id: 'p3', label: 'Tres' }],
  );
});
