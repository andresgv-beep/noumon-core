import assert from 'node:assert/strict';
import test from 'node:test';

import { inline, inlineText, plainText, richText } from './studioEditable.js';

// Doble mínimo del DOM: sólo lo que estas acciones usan. Basta para fijar la
// regla que importa —cuándo se reescribe el elemento y cuándo no—, que es lo que
// mandaba el cursor al principio y hacía que el texto saliera al revés.
class FakeNode {
  constructor() {
    this.writes = 0;
    this._html = '';
    this._text = '';
    this.nodeType = 1;
    this.tagName = 'P';
  }

  get childNodes() {
    return this._text ? [{ nodeType: 3, nodeValue: this._text }] : [];
  }

  set innerHTML(value) {
    this._html = value;
    this._text = value.replace(/<[^>]*>/g, '');
    this.writes += 1;
  }

  get innerHTML() { return this._html; }

  set textContent(value) {
    this._text = value;
    this._html = value;
    this.writes += 1;
  }

  get textContent() { return this._text; }

  get innerText() { return this._text; }

  /** Lo que el usuario acaba de teclear: cambia el DOM sin pasar por Svelte. */
  type(value) {
    this._text = value;
    this._html = value;
  }
}

test('does not rewrite a rich field while the user types into it', () => {
  const node = new FakeNode();
  const action = richText(node, 'Hola');
  const initialWrites = node.writes;

  node.type('Hola que tal');
  // Svelte reacciona al estado que acaba de guardar el manejador de entrada.
  action.update('Hola que tal');

  assert.equal(node.writes, initialWrites, 'reescribir el elemento al teclear mueve el cursor al principio');
  assert.equal(node.innerText, 'Hola que tal');
});

test('rewrites a rich field when the value changes from outside', () => {
  const node = new FakeNode();
  const action = richText(node, 'Hola');
  const initialWrites = node.writes;

  // Deshacer, cambiar de bloque o restaurar una revisión: el DOM está obsoleto.
  action.update('Texto restaurado');

  assert.equal(node.writes, initialWrites + 1);
  assert.equal(node.innerText, 'Texto restaurado');
});

test('applies the same rule to plain-text fields', () => {
  const node = new FakeNode();
  const action = plainText(node, 'uno');
  const initialWrites = node.writes;

  node.type('uno dos');
  action.update('uno dos');
  assert.equal(node.writes, initialWrites);

  action.update('tres');
  assert.equal(node.writes, initialWrites + 1);
  assert.equal(node.innerText, 'tres');
});

test('keeps light markup round-tripping between text and HTML', () => {
  assert.equal(inline('**fuerte** y *suave*'), '<strong>fuerte</strong> y <em>suave</em>');
  assert.equal(inline('<script>'), '&lt;script&gt;');
  assert.equal(
    inlineText({
      nodeType: 1,
      tagName: 'P',
      childNodes: [
        { nodeType: 3, nodeValue: 'texto ' },
        { nodeType: 1, tagName: 'STRONG', childNodes: [{ nodeType: 3, nodeValue: 'fuerte' }] },
      ],
    }),
    'texto **fuerte**',
  );
});
