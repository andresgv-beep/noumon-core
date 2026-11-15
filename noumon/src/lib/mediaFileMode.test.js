import test from 'node:test';
import assert from 'node:assert/strict';
import { mediaFileMode, mediaFileName } from './mediaFileMode.js';

test('protected files retain their viewer type with tokens and fragments', () => {
  for (const [extension, mode] of [['pdf','pdf'], ['mp4','video'], ['mp3','audio'], ['jpg','image']]) {
    assert.equal(mediaFileMode({ url: `/media/file.${extension}?st=read-token#page=2` }), mode);
  }
});

test('uses an original filename when the URL has no file extension', () => {
  assert.equal(mediaFileMode({ name: 'Manual.PDF', url: '/media/resource?st=read-token' }), 'pdf');
  assert.equal(mediaFileMode({ url: '/media/resource?name=manual.pdf' }), 'unsupported');
});

test('viewer filenames omit authentication and keep encoded filename characters', () => {
  assert.equal(mediaFileName('/media/Manual%20A%2BB%23C.pdf?st=private-token#page=2'), 'Manual A+B#C.pdf');
  assert.equal(mediaFileName('/media/manual.pdf?st=private-token'), 'manual.pdf');
});
