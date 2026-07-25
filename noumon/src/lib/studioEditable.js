// Edición de texto en el lienzo del Studio.
//
// El problema que resuelve este módulo: un contenteditable cuyo contenido se
// pinta desde el estado (`{@html inline(block.text)}` o `{item}`) entra en un
// bucle destructivo. Al teclear, el manejador guarda el texto en el estado; el
// estado cambia, Svelte vuelve a escribir el contenido del elemento, y reponer
// el contenido coloca el cursor al principio. La siguiente tecla se inserta ahí,
// y la siguiente antes que esa: el texto sale escrito al revés.
//
// La solución es que, mientras se escribe, el dueño del DOM sea el navegador y
// no Svelte. Estas acciones sólo reescriben el elemento cuando el valor que
// llega es distinto del que el elemento ya representa, es decir cuando el cambio
// viene de fuera (deshacer, cambiar de bloque, restaurar una revisión). Si el
// cambio viene de teclear, el DOM ya está correcto y no se toca, así que el
// cursor se queda donde el usuario lo dejó.

export function escapeHTML(value) {
  return String(value || '').replace(/[&<>"']/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[character]);
}

/** Texto con marcado ligero (**negrita**, *cursiva*) a HTML. */
export function inline(value) {
  return escapeHTML(value)
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
}

/**
 * El camino inverso: del HTML editado de vuelta al texto con marcado.
 * Usa los números de nodeType (3 texto, 1 elemento) en vez de la constante
 * global Node para no depender del navegador y poder probarse.
 */
export function inlineText(node) {
  if (node.nodeType === 3) return node.nodeValue || '';
  if (node.nodeType !== 1) return '';
  const content = [...node.childNodes].map(inlineText).join('');
  if (node.tagName === 'STRONG' || node.tagName === 'B') return `**${content}**`;
  if (node.tagName === 'EM' || node.tagName === 'I') return `*${content}*`;
  if (node.tagName === 'BR') return '\n';
  return content;
}

/** Acción para un contenteditable con marcado ligero. */
export function richText(node, value) {
  node.innerHTML = inline(value);
  return {
    update(next) {
      if (inlineText(node) === String(next ?? '')) return;
      node.innerHTML = inline(next);
    },
  };
}

/** Acción para un contenteditable de texto plano. */
export function plainText(node, value) {
  node.textContent = String(value ?? '');
  return {
    update(next) {
      if (node.innerText === String(next ?? '')) return;
      node.textContent = String(next ?? '');
    },
  };
}
