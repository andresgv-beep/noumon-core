import test, { beforeEach } from 'node:test';
import assert from 'node:assert/strict';

// Reproducir el gateway: base inyectada vacia, sin cookies del WebView.
globalThis.window = { __NOUMON_LIBRARY_SERVER__: '' };
const storage = new Map();
globalThis.localStorage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, String(value)),
  removeItem: (key) => storage.delete(key),
};
const connection = await import('./connection.js');

beforeEach(() => {
  storage.clear();
  delete window.__NOUMON_LIBRARY_SHELL__;
  connection.setServerBase('');
  connection.setMediaToken('');
});

test('gateway adds a media token to both PDF and cover relative URLs', () => {
  window.__NOUMON_LIBRARY_SHELL__ = true;
  connection.setSessionToken('full-session');
  connection.setMediaToken('ephemeral-media');
  const item = connection.resolveServerPayload({
    open: { url: '/media/Cabinet/manual/document.pdf' },
    preview: { url: '/media/Cabinet/manual/cover.jpg' },
    files: [{ url: '/media/Cabinet/manual/document.pdf' }],
  });
  for (const url of [item.open.url, item.preview.url, item.files[0].url]) {
    assert.match(url, /\?st=ephemeral-media$/);
    assert.ok(!url.includes('full-session'));
  }
});

test('keeps media query parameters and document fragments', () => {
  connection.setMediaToken('fresh token');
  const url = connection.serverUrl('/content/book/article?lang=es&st=old#chapter-2');
  const parsed = new URL(url, 'https://local.test');
  assert.equal(parsed.searchParams.get('lang'), 'es');
  assert.deepEqual(parsed.searchParams.getAll('st'), ['fresh token']);
  assert.equal(parsed.hash, '#chapter-2');
});

test('does not send tokens to APIs, unrelated paths, or external resources', () => {
  connection.setMediaToken('ephemeral-media');
  for (const url of ['/api/items', '/media-other/private', '/content-other/file', '/assets/logo.svg',
    'https://other.test/media/a.pdf', '//other.test/media/a.jpg', 'blob:https://local.test/id', 'data:image/png;base64,x']) {
    assert.equal(connection.serverUrl(url), url);
  }
});

test('cross-origin media still receives its ephemeral token', () => {
  connection.setServerBase('http://server.test:8090');
  connection.setMediaToken('remote-media');
  assert.equal(connection.serverUrl('/media/cover.jpg'), 'http://server.test:8090/media/cover.jpg?st=remote-media');
});

test('API bearer authentication remains separate from native media URLs', async () => {
  connection.setSessionToken('full-session');
  connection.setMediaToken('ephemeral-media');
  const originalFetch = globalThis.fetch;
  let captured;
  globalThis.fetch = async (url, options) => { captured = { url, options }; return { ok: true }; };
  try {
    await connection.serverFetch('/api/items');
    assert.equal(captured.url, '/api/items');
    assert.equal(captured.options.headers.get('Authorization'), 'Bearer full-session');
    assert.equal(captured.options.credentials, 'include');
  } finally { globalThis.fetch = originalFetch; }
});

test('anonymous media URLs remain unchanged', () => {
  assert.equal(connection.serverUrl('/media/public.pdf#page=2'), '/media/public.pdf#page=2');
});
