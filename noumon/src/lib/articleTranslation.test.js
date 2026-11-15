import test from 'node:test';
import assert from 'node:assert/strict';
import { parseHTML } from 'linkedom';
import { collectArticleSegments, createTranslationRequest } from './articleTranslation.js';

function fixture(html) {
  return parseHTML(`<html><head></head><body>${html}</body></html>`).document;
}

test('collects cards, table cells, nested lists and inline links once', () => {
  const doc = fixture(`<div class="card"><div>Card title</div><span>Card text</span></div>
    <table><caption>Facts</caption><tr><th>Country</th><td>Spain</td></tr></table>
    <ul><li>Outer text<ul><li>Inner text</li></ul></li></ul>
    <p>Read the <a href="/manual">manual</a>.</p><h5>Small heading</h5>`);
  const source = collectArticleSegments(doc);
  const text = source.segments.map(s => s.text);
  for (const expected of ['Card title', 'Card text', 'Facts', 'Country', 'Spain', 'Outer text', 'Inner text', 'Small heading']) {
    assert.equal(text.filter(t => t.trim() === expected).length, 1, expected);
  }
  assert.ok(text.includes('Read the <a href="/manual">manual</a>.'));
  const original = doc.body.innerHTML;
  source.apply(source.segments.map(s => ({ ...s, text: s.text.replace(/text/g, 'texto').replace('Read the', 'Lee el') })));
  assert.equal(doc.querySelector('a').getAttribute('href'), '/manual');
  assert.match(doc.querySelector('ul').textContent, /Outer textoInner texto/);
  source.restore();
  assert.equal(doc.body.innerHTML, original);
  // A second language is applied from the same source, never from a translation.
  source.apply(source.segments.map(s => ({ ...s, text: s.text.replace('Card title', 'Título') })));
  source.restore();
  assert.equal(doc.body.innerHTML, original);
});

test('keeps excluded content and handles escaped direct text as text, not markup', () => {
  const doc = fixture(`<div>Hello &amp; welcome <p>World</p></div>
    <pre>code example</pre><script>hidden script</script><style>hidden style</style>
    <span translate="no">Keep name</span><div class="notranslate">Keep code</div>
    <p>Read <code>example()</code> here <sup class="reference">[1]</sup></p>`);
  const source = collectArticleSegments(doc);
  assert.ok(source.segments.some(s => s.text.includes('Hello &amp; welcome')));
  const all = source.segments.map(s => s.text).join(' ');
  for (const skip of ['code example', 'hidden script', 'hidden style', 'Keep name', 'Keep code', 'example()', '[1]']) assert.ok(!all.includes(skip), skip);
  source.apply(source.segments.map(s => ({ ...s, text: s.text.replace('Hello &amp; welcome', 'Hola &amp; &lt;mundo&gt;') })));
  assert.match(doc.body.textContent, /Hola & <mundo>/);
  assert.equal(doc.querySelector('mundo'), null);
});

test('obsolete requests cannot apply after switching languages or showing original', () => {
  const requests = createTranslationRequest();
  const first = requests.start();
  assert.equal(first.isCurrent(), true);
  const second = requests.start();
  assert.equal(first.signal.aborted, true);
  assert.equal(first.isCurrent(), false);
  assert.equal(second.isCurrent(), true);
  requests.cancel();
  assert.equal(second.signal.aborted, true);
  assert.equal(second.isCurrent(), false);
});

test('medical index separators never reach the engine and inline spacing survives trimming', () => {
  const doc = fixture('<table><tr><td><div></div><a href="/a">Cardiología</a> • <a href="/b">Pediatría</a> • Labio leporino • <a href="/c">Dermatología</a></td></tr></table>');
  const source = collectArticleSegments(doc);
  assert.ok(source.segments.every(s => /\p{L}/u.test(s.text)));
  assert.equal(source.segments.filter(s => s.text.trim() === '•').length, 0);
  assert.ok(source.segments.some(s => s.text === 'Labio leporino'));
  source.apply(source.segments.map(s => ({ ...s, text: s.text.replace('Cardiología','Cardiology').replace('Pediatría','Pediatrics').replace('Labio leporino','Cleft lip').replace('Dermatología','Dermatology').trim() })));
  assert.equal(doc.querySelector('td').textContent, 'Cardiology • Pediatrics • Cleft lip • Dermatology');
  source.restore();
  assert.match(doc.querySelector('td').textContent, /Cardiología • Pediatría • Labio leporino • Dermatología/);
});
