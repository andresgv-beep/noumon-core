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
const studioInlineLinkRE = /\[\[(page|item):([A-Za-z0-9][A-Za-z0-9._:-]{0,127})\|([^\]\r\n]+)\]\]/g;
const studioEmptyInlineLinkRE = /\[\[(page|item):([A-Za-z0-9][A-Za-z0-9._:-]{0,127})\|\]\]/g;

function formattedText(value) {
  return escapeHTML(value)
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
}

export function studioInlineLinks(value) {
  const source = String(value || '');
  const links = [...source.matchAll(studioInlineLinkRE)].map((match) => ({
    syntax: match[0],
    kind: match[1],
    id: match[2],
    label: match[3],
  }));
  links.push(...[...source.matchAll(studioEmptyInlineLinkRE)].map((match) => ({
    syntax: match[0],
    kind: match[1],
    id: match[2],
    label: '',
    invalid: true,
  })));
  return links;
}

export function collectStudioInlineLinks(value, result = []) {
  if (typeof value === 'string') {
    result.push(...studioInlineLinks(value));
    return result;
  }
  if (Array.isArray(value)) {
    for (const child of value) collectStudioInlineLinks(child, result);
    return result;
  }
  if (value && typeof value === 'object') {
    for (const child of Object.values(value)) collectStudioInlineLinks(child, result);
  }
  return result;
}

/**
 * Convierte los enlaces a una página en texto normal dentro de todo el
 * documento. Conserva la etiqueta visible y limpia también la forma vacía que
 * generaban versiones anteriores del editor.
 */
export function removeStudioPageLinks(value, pageID) {
  let removed = 0;

  function unlinkText(text) {
    const replaceLink = (syntax, kind, id, label = '') => {
      if (kind !== 'page' || id !== pageID) return syntax;
      removed++;
      return label;
    };
    return text
      .replace(studioInlineLinkRE, replaceLink)
      .replace(
        studioEmptyInlineLinkRE,
        (syntax, kind, id) => replaceLink(syntax, kind, id, ''),
      );
  }

  function visit(current) {
    if (typeof current === 'string') return unlinkText(current);
    if (Array.isArray(current)) {
      for (let index = 0; index < current.length; index++) {
        current[index] = visit(current[index]);
      }
      return current;
    }
    if (current && typeof current === 'object') {
      for (const key of Object.keys(current)) current[key] = visit(current[key]);
    }
    return current;
  }

  return { value: visit(value), removed };
}

/** Texto con marcado ligero y enlaces internos a HTML seguro. */
export function inline(value, { pageIDs = [] } = {}) {
  // Una etiqueta vacía no tiene nada que representar y no debe dejar visible
  // el marcado interno de una versión antigua del editor.
  const source = String(value || '').replace(studioEmptyInlineLinkRE, '');
  const knownPages = new Set(pageIDs || []);
  let cursor = 0;
  let output = '';
  for (const match of source.matchAll(studioInlineLinkRE)) {
    output += formattedText(source.slice(cursor, match.index));
    const [, kind, id, label] = match;
    const broken = kind === 'page' && !knownPages.has(id);
    const classes = `studio-inline-link studio-inline-link-${kind}${broken ? ' is-broken' : ''}`;
    output += `<a class="${classes}" href="#studio-${kind}-${encodeURIComponent(id)}" data-studio-link-kind="${kind}" data-studio-link-id="${escapeHTML(id)}"${broken ? ' aria-invalid="true"' : ''}>${formattedText(label)}</a>`;
    cursor = match.index + match[0].length;
  }
  return output + formattedText(source.slice(cursor));
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
  if (node.tagName === 'A') {
    const kind = node.dataset?.studioLinkKind;
    const id = node.dataset?.studioLinkId;
    if ((kind === 'page' || kind === 'item') &&
      /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(id || '')) {
      if (!content.trim()) return content;
      return `[[${kind}:${id}|${content}]]`;
    }
  }
  if (node.tagName === 'BR') return '\n';
  return content;
}

function richTextOptions(value) {
  if (value && typeof value === 'object') {
    return {
      text: String(value.text ?? ''),
      pageIDs: Array.isArray(value.pageIDs) ? value.pageIDs : [],
    };
  }
  return { text: String(value ?? ''), pageIDs: [] };
}

/** Acción para un contenteditable con marcado ligero. */
export function richText(node, value) {
  let options = richTextOptions(value);
  let pageSignature = options.pageIDs.join('\u0000');
  node.innerHTML = inline(options.text, options);
  return {
    update(next) {
      const nextOptions = richTextOptions(next);
      const nextPageSignature = nextOptions.pageIDs.join('\u0000');
      if (inlineText(node) === nextOptions.text && pageSignature === nextPageSignature) return;
      options = nextOptions;
      pageSignature = nextPageSignature;
      node.innerHTML = inline(options.text, options);
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
