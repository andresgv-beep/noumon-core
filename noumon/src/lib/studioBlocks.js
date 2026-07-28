// Operaciones sobre el árbol de bloques de una página: buscar, quitar, duplicar
// y mover. Todas reciben la lista de bloques y la modifican en el sitio; ninguna
// sabe de Svelte, del DOM ni de qué bloque está seleccionado.
//
// Un bloque puede anidar de tres maneras distintas (`columns`, `children` y
// `blocks`), y esa irregularidad es la fuente de la mayoría de los fallos al
// arrastrar: por eso el recorrido está en un solo sitio y se prueba aquí.

// Los hijos de un bloque, vengan por donde vengan.
function childLists(block) {
  const lists = [];
  for (const column of block?.columns || []) lists.push(column);
  for (const children of [block?.children, block?.blocks]) {
    if (Array.isArray(children)) lists.push(children);
  }
  return lists;
}

// Devuelve dónde vive el bloque —su lista y su posición dentro de ella—, no sólo
// el bloque: quitarlo o insertar a su lado necesita el contenedor.
export function findStudioBlock(blocks, blockID) {
  if (!blockID) return null;
  for (let index = 0; index < (blocks || []).length; index += 1) {
    const block = blocks[index];
    if (block.id === blockID) return { block, container: blocks, index };
    for (const list of childLists(block)) {
      const nested = findStudioBlock(list, blockID);
      if (nested) return nested;
    }
  }
  return null;
}

export function studioBlockByID(blocks, blockID) {
  return findStudioBlock(blocks, blockID)?.block || null;
}

// ¿blockID está dentro de block (o es él mismo)? Es la guarda que impide soltar
// un bloque dentro de sí mismo, que desengancharía esa rama del documento.
export function studioBlockContains(block, blockID) {
  if (!block) return false;
  if (block.id === blockID) return true;
  return childLists(block).some((list) => list.some((child) => studioBlockContains(child, blockID)));
}

export function removeStudioBlock(blocks, blockID) {
  const location = findStudioBlock(blocks, blockID);
  if (!location) return null;
  const [removed] = location.container.splice(location.index, 1);
  return removed;
}

// Copia el bloque justo debajo del original con identidades nuevas en todos sus
// niveles. Repetir un id rompe las listas por clave del lienzo y deja las fichas
// laterales ancladas a dos bloques a la vez.
export function duplicateStudioBlock(blocks, blockID, nextId) {
  const location = findStudioBlock(blocks, blockID);
  if (!location) return null;
  const copy = JSON.parse(JSON.stringify(location.block));
  const renewIDs = (block) => {
    block.id = nextId();
    for (const list of childLists(block)) list.forEach(renewIDs);
  };
  renewIDs(copy);
  location.container.splice(location.index + 1, 0, copy);
  return copy;
}

// Saca el bloque de donde esté para poder reinsertarlo. Se hace ANTES de buscar
// el destino a propósito: si los dos comparten lista, quitar el de arriba corre
// un puesto al de abajo, y calcular el destino antes lo dejaría descolocado.
function detach(blocks, draggedID, destinationID) {
  if (!draggedID || draggedID === destinationID) return null;
  const location = findStudioBlock(blocks, draggedID);
  if (!location) return null;
  // Soltarlo dentro de su propia descendencia lo separaría del documento.
  if (studioBlockContains(location.block, destinationID)) return null;
  const [block] = location.container.splice(location.index, 1);
  return block;
}

export function moveStudioBlockBefore(blocks, draggedID, targetID) {
  const block = detach(blocks, draggedID, targetID);
  if (!block) return null;
  const target = findStudioBlock(blocks, targetID);
  if (!target) {
    // El destino se ha ido con el movimiento: antes que perder el bloque, al
    // final del documento.
    blocks.push(block);
    return block;
  }
  target.container.splice(target.index, 0, block);
  return block;
}

export function moveStudioBlockIntoColumn(blocks, draggedID, columnsBlockID, columnIndex) {
  const destination = studioBlockByID(blocks, columnsBlockID);
  if (!destination) return null;
  if (studioBlockContains(studioBlockByID(blocks, draggedID), columnsBlockID)) return null;
  const block = detach(blocks, draggedID, columnsBlockID);
  if (!block) return null;
  const column = studioBlockByID(blocks, columnsBlockID)?.columns?.[columnIndex];
  if (!column) {
    blocks.push(block);
    return block;
  }
  column.push(block);
  return block;
}

export function moveStudioBlockToEnd(blocks, draggedID) {
  const block = detach(blocks, draggedID, '');
  if (!block) return null;
  blocks.push(block);
  return block;
}

// Saca un bloque de la columna en la que esté y lo deja al final del documento.
export function moveStudioBlockToRoot(blocks, blockID) {
  const location = findStudioBlock(blocks, blockID);
  if (!location || location.container === blocks) return null;
  const [block] = location.container.splice(location.index, 1);
  blocks.push(block);
  return block;
}

export function appendStudioBlockToColumn(blocks, columnsBlockID, columnIndex, block) {
  const column = studioBlockByID(blocks, columnsBlockID)?.columns?.[columnIndex];
  if (!column) return false;
  column.push(block);
  return true;
}

export const STUDIO_MIN_TEXT_SIZE = 10;
export const STUDIO_MAX_TEXT_SIZE = 96;

export function clampStudioTextSize(value, fallback) {
  return Math.max(
    STUDIO_MIN_TEXT_SIZE,
    Math.min(STUDIO_MAX_TEXT_SIZE, Math.round(Number(value) || fallback)),
  );
}

function storedTextSize(value, fallback) {
  const size = Number(value);
  return Number.isInteger(size) && size >= STUDIO_MIN_TEXT_SIZE && size <= STUDIO_MAX_TEXT_SIZE
    ? size
    : fallback;
}

export function studioPresentationTextSize(presentation, field, fallback) {
  return storedTextSize(presentation?.[field], fallback);
}

// Tamaño con el que se pinta un bloque cuando el autor no ha elegido ninguno.
export function studioBlockDefaultTextSize(block) {
  if (block?.type === 'heading') return ({ 1: 42, 2: 25, 3: 18 })[block.level || 2] ?? 25;
  if (block?.type === 'quote') return 17;
  if (block?.type === 'code' || block?.type === 'table' || block?.type === 'callout') return 13;
  return 15;
}

const SIZEABLE_TYPES = [
  'heading', 'paragraph', 'quote', 'bulletList', 'orderedList', 'code', 'callout', 'table',
];

// Qué ofrecer en la barra de texto para lo que hay seleccionado, o null si eso
// no se puede medir ni alinear. El título y la entradilla son del documento, no
// bloques, y guardan su tamaño en la presentación.
export function studioTextControl(blocks, selectedBlockID, presentation) {
  if (selectedBlockID === '@title') {
    return {
      size: studioPresentationTextSize(presentation, 'titleFontSize', 34),
      align: presentation?.titleTextAlign || 'left',
      canAlign: true,
    };
  }
  if (selectedBlockID === '@summary') {
    return {
      size: studioPresentationTextSize(presentation, 'summaryFontSize', 17),
      align: presentation?.summaryTextAlign || 'left',
      canAlign: true,
    };
  }
  const block = studioBlockByID(blocks, selectedBlockID);
  // Una imagen sólo lleva texto medible cuando va flotada al lado del párrafo.
  const supported = SIZEABLE_TYPES.includes(block?.type)
    || (block?.type === 'image'
      && ['medium', 'small'].includes(block.imageSize)
      && ['left', 'right'].includes(block.imageAlign));
  if (!supported) return null;
  return {
    size: storedTextSize(block.fontSize, studioBlockDefaultTextSize(block)),
    align: ['left', 'center', 'right'].includes(block.textAlign) ? block.textAlign : 'left',
    // Una tabla se alinea por celdas, no en bloque.
    canAlign: block.type !== 'table',
  };
}
