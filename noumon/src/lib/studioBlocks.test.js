import test from 'node:test';
import assert from 'node:assert/strict';
import {
  findStudioBlock, studioBlockByID, studioBlockContains, removeStudioBlock,
  duplicateStudioBlock, moveStudioBlockBefore, moveStudioBlockIntoColumn,
  moveStudioBlockToEnd, moveStudioBlockToRoot, appendStudioBlockToColumn,
  studioTextControl, studioBlockDefaultTextSize, clampStudioTextSize,
} from './studioBlocks.js';

const ids = (blocks) => blocks.map((block) => block.id);

// Un documento con las tres formas de anidar que admite el modelo.
function tree() {
  return [
    { id: 'a', type: 'paragraph' },
    {
      id: 'cols',
      type: 'columns',
      columns: [
        [{ id: 'c1', type: 'paragraph' }],
        [{ id: 'c2', type: 'paragraph' }],
      ],
    },
    { id: 'list', type: 'bulletList', children: [{ id: 'li', type: 'paragraph' }] },
    { id: 'b', type: 'paragraph' },
  ];
}

test('encuentra bloques anidados en columnas y en hijos, con su contenedor', () => {
  const blocks = tree();

  assert.equal(findStudioBlock(blocks, 'a').container, blocks);
  assert.equal(findStudioBlock(blocks, 'c2').block.id, 'c2');
  assert.equal(findStudioBlock(blocks, 'c2').container, blocks[1].columns[1]);
  assert.equal(findStudioBlock(blocks, 'li').container, blocks[2].children);
  assert.equal(findStudioBlock(blocks, 'nada'), null);
  assert.equal(findStudioBlock(blocks, ''), null);
});

test('quitar alcanza a los bloques anidados, no sólo a los de primer nivel', () => {
  const blocks = tree();

  assert.equal(removeStudioBlock(blocks, 'c1').id, 'c1');
  assert.deepEqual(blocks[1].columns[0], []);
  assert.equal(removeStudioBlock(blocks, 'inexistente'), null);
});

test('duplicar renueva las identidades de todos los niveles, no sólo la de arriba', () => {
  const blocks = tree();
  let n = 0;
  const copy = duplicateStudioBlock(blocks, 'cols', () => `nuevo-${++n}`);

  assert.equal(ids(blocks)[2], copy.id, 'la copia va justo debajo del original');
  assert.equal(copy.id, 'nuevo-1');
  // Repetir un id rompe las listas por clave y las anclas de las fichas.
  assert.deepEqual(
    [copy.columns[0][0].id, copy.columns[1][0].id],
    ['nuevo-2', 'nuevo-3'],
  );
  assert.equal(blocks[1].columns[0][0].id, 'c1', 'el original no se toca');
});

test('mover hacia abajo en la misma lista cae donde el usuario ha soltado', () => {
  const blocks = tree();
  // 'a' es el primero; soltarlo sobre 'b' (el último) debe dejarlo justo antes.
  moveStudioBlockBefore(blocks, 'a', 'b');

  assert.deepEqual(ids(blocks), ['cols', 'list', 'a', 'b']);
});

test('mover hacia arriba en la misma lista también', () => {
  const blocks = tree();
  moveStudioBlockBefore(blocks, 'b', 'cols');

  assert.deepEqual(ids(blocks), ['a', 'b', 'cols', 'list']);
});

test('un bloque no se puede soltar dentro de sí mismo', () => {
  const blocks = tree();

  assert.equal(moveStudioBlockBefore(blocks, 'cols', 'c1'), null);
  assert.equal(moveStudioBlockIntoColumn(blocks, 'cols', 'cols', 0), null);
  // Y sigue entero donde estaba, no medio desenganchado.
  assert.deepEqual(ids(blocks), ['a', 'cols', 'list', 'b']);
  assert.equal(blocks[1].columns[0][0].id, 'c1');
});

test('soltarlo sobre sí mismo no lo mueve ni lo pierde', () => {
  const blocks = tree();

  assert.equal(moveStudioBlockBefore(blocks, 'a', 'a'), null);
  assert.deepEqual(ids(blocks), ['a', 'cols', 'list', 'b']);
});

test('mover a una columna lo saca de donde estaba', () => {
  const blocks = tree();
  moveStudioBlockIntoColumn(blocks, 'a', 'cols', 1);

  assert.deepEqual(ids(blocks), ['cols', 'list', 'b']);
  assert.deepEqual(ids(blocks[0].columns[1]), ['c2', 'a']);
});

test('si la columna de destino ya no existe, el bloque acaba al final y no se pierde', () => {
  const blocks = tree();
  const moved = moveStudioBlockIntoColumn(blocks, 'a', 'cols', 7);

  assert.equal(moved.id, 'a');
  assert.deepEqual(ids(blocks), ['cols', 'list', 'b', 'a']);
});

test('sacar a la raíz sólo hace algo si el bloque estaba anidado', () => {
  const blocks = tree();

  assert.equal(moveStudioBlockToRoot(blocks, 'a'), null, 'ya estaba en la raíz');
  assert.equal(moveStudioBlockToRoot(blocks, 'c1').id, 'c1');
  assert.deepEqual(ids(blocks), ['a', 'cols', 'list', 'b', 'c1']);
  assert.deepEqual(blocks[1].columns[0], []);
});

test('mover al final saca el bloque de su columna', () => {
  const blocks = tree();
  moveStudioBlockToEnd(blocks, 'c2');

  assert.deepEqual(ids(blocks), ['a', 'cols', 'list', 'b', 'c2']);
  assert.deepEqual(blocks[1].columns[1], []);
});

test('añadir a una columna avisa cuando no existe en vez de tragárselo', () => {
  const blocks = tree();

  assert.equal(appendStudioBlockToColumn(blocks, 'cols', 0, { id: 'x' }), true);
  assert.equal(appendStudioBlockToColumn(blocks, 'cols', 9, { id: 'y' }), false);
  assert.equal(appendStudioBlockToColumn(blocks, 'list', 0, { id: 'z' }), false);
  assert.deepEqual(ids(blocks[1].columns[0]), ['c1', 'x']);
});

test('contiene reconoce la descendencia por columnas y por hijos', () => {
  const blocks = tree();

  assert.equal(studioBlockContains(studioBlockByID(blocks, 'cols'), 'c2'), true);
  assert.equal(studioBlockContains(studioBlockByID(blocks, 'list'), 'li'), true);
  assert.equal(studioBlockContains(studioBlockByID(blocks, 'cols'), 'li'), false);
  assert.equal(studioBlockContains(null, 'a'), false);
});

test('la barra de texto ofrece el tamaño guardado, y si no el del tipo de bloque', () => {
  const blocks = [
    { id: 'h', type: 'heading', level: 1 },
    { id: 'p', type: 'paragraph', fontSize: 21, textAlign: 'center' },
    { id: 'raro', type: 'paragraph', fontSize: 400 },
  ];

  assert.equal(studioTextControl(blocks, 'h', {}).size, 42);
  assert.deepEqual(studioTextControl(blocks, 'p', {}), { size: 21, align: 'center', canAlign: true });
  assert.equal(studioTextControl(blocks, 'raro', {}).size, 15, 'un valor fuera de rango no se acata');
});

test('la barra no aparece donde no hay texto que medir', () => {
  const blocks = [
    { id: 'div', type: 'divider' },
    { id: 'sp', type: 'spacer' },
    { id: 'img', type: 'image', imageSize: 'full' },
    { id: 'flotada', type: 'image', imageSize: 'small', imageAlign: 'left' },
  ];

  assert.equal(studioTextControl(blocks, 'div', {}), null);
  assert.equal(studioTextControl(blocks, 'sp', {}), null);
  assert.equal(studioTextControl(blocks, 'img', {}), null, 'a ancho completo no envuelve texto');
  assert.ok(studioTextControl(blocks, 'flotada', {}), 'flotada sí');
  assert.equal(studioTextControl(blocks, '', {}), null);
});

test('una tabla se mide pero no se alinea en bloque', () => {
  const control = studioTextControl([{ id: 't', type: 'table' }], 't', {});

  assert.equal(control.size, 13);
  assert.equal(control.canAlign, false);
});

test('el título y la entradilla leen su tamaño de la presentación, no del árbol', () => {
  const presentation = { titleFontSize: 50, summaryTextAlign: 'center' };

  assert.equal(studioTextControl([], '@title', presentation).size, 50);
  assert.equal(studioTextControl([], '@summary', presentation).size, 17, 'sin fijar, el suyo');
  assert.equal(studioTextControl([], '@summary', presentation).align, 'center');
});

test('los tamaños se recortan al rango admitido', () => {
  assert.equal(clampStudioTextSize(0, 15), 15, 'un 0 no es una talla, se usa el respaldo');
  assert.equal(clampStudioTextSize(5, 15), 10);
  assert.equal(clampStudioTextSize(500, 15), 96);
  assert.equal(clampStudioTextSize('21', 15), 21);
  assert.equal(studioBlockDefaultTextSize({ type: 'heading', level: 9 }), 25, 'un nivel raro no da undefined');
});
