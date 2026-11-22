import test from 'node:test'
import assert from 'node:assert/strict'
import { planLocalImport } from './localImport.js'
import { messages } from './messages.js'

const file = (path) => ({ name: path.split('/').at(-1), webkitRelativePath: path, size: 1024 })

test('copies a shared media folder into its three sections with metadata last', () => {
  const plan = planLocalImport([
    file('downloads/Depot/VLC/card.json'), file('downloads/Moments/Channel/video.mp4'),
    file('downloads/Cabinet/Books/book.pdf'), file('downloads/Depot/VLC/setup.exe'),
    file('downloads/Depot/VLC/cover.png'), file('downloads/.private/key'),
  ], 'auto', 'Imported')
  assert.equal(plan.skipped.length, 1)
  assert.deepEqual(plan.entries.map(({ surface, path }) => [surface, path]), [
    ['moments', 'Channel/video.mp4'], ['cabinet', 'Books/book.pdf'],
    ['depot', 'VLC/setup.exe'], ['depot', 'VLC/cover.png'], ['depot', 'VLC/card.json'],
  ])
})

test('supports a selected collection and individual files without flattening paths', () => {
  assert.equal(planLocalImport([file('My channel/subfolder/video.mp4')], 'moments', 'Imported').entries[0].path, 'My channel/subfolder/video.mp4')
  assert.equal(planLocalImport([{ name: 'book.pdf', size: 20 }], 'cabinet', 'Books').entries[0].path, 'Books/book.pdf')
  assert.equal(planLocalImport([file('Cabinet/book.pdf')], 'auto', 'Imported').entries[0].path, 'Imported/book.pdf')
})

test('rejects wrong sections and traversal, and avoids duplicate queue keys', () => {
  assert.throws(() => planLocalImport([file('Moments/Channel/video.mp4')], 'cabinet', 'Books'), /wrongSurface/)
  assert.throws(() => planLocalImport([{ name: 'book.pdf' }], 'cabinet', '../Books'), /invalidPath/)
  assert.equal(planLocalImport([file('Books/a.pdf'), file('Books/a.pdf')], 'cabinet', '').entries.length, 1)
  assert.equal(planLocalImport([file('Unknown/a.pdf')], 'auto', '').entries.length, 0)
})

test('local import interface and error messages exist in Spanish and English', () => {
  const keys = Object.keys(messages.es).filter((key) => key.startsWith('import.local.')).sort()
  assert.deepEqual(keys, Object.keys(messages.en).filter((key) => key.startsWith('import.local.')).sort())
  for (const key of keys) { assert.ok(messages.es[key]); assert.ok(messages.en[key]) }
})
