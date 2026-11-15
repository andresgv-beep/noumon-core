// Translate disjoint fragments, keeping inline links together. Descend through
// cards and tables without replacing their structural or interactive containers.
const SKIP = 'script, style, noscript, code, pre, kbd, samp, textarea, input, select, svg, math, video, audio, canvas, iframe, [translate="no"], .notranslate, .mw-editsection, .reference';
const BLOCK = 'address, article, aside, blockquote, div, dl, dt, dd, figure, figcaption, footer, form, h1, h2, h3, h4, h5, h6, header, hr, main, nav, ol, p, section, table, caption, thead, tbody, tfoot, tr, td, th, ul, li, details, summary';
export function collectArticleSegments(doc) {
  const segments = [], entries = [];
  function add(node, html) {
    const original = html ? node.innerHTML : node.nodeValue;
    // Punctuation, bullets and numbers are layout/data, not sentences. Tiny
    // models hallucinate prose when asked to translate an isolated separator.
    const visible = html ? node.textContent : original;
    if (!/\p{L}/u.test(visible || '')) return;
    const prefix = html ? '' : original.match(/^[\s•·|–—-]*/u)[0];
    const suffix = html ? '' : original.match(/[\s•·|–—-]*$/u)[0];
    const content = html ? original : original.slice(prefix.length, original.length - suffix.length);
    const text = html ? content : content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    const id = `s${segments.length}`;
    segments.push({ id, text });
    entries.push({ id, node, html, original, prefix, suffix });
  }
  function walk(el) {
    if (el.matches(SKIP)) return;
    if (el !== doc.body && !el.querySelector(`${BLOCK}, ${SKIP}`) && el.textContent.trim()) {
      add(el, true);
      return;
    }
    for (const child of el.childNodes) {
      if (child.nodeType === 3) add(child, false);
      else if (child.nodeType === 1) walk(child);
    }
  }
  if (doc.body) { doc.body.normalize(); walk(doc.body); }
  return {
    segments,
    restore() {
      for (const entry of entries) {
        if (entry.html) entry.node.innerHTML = entry.original;
        else entry.node.nodeValue = entry.original;
      }
    },
    apply(result) {
      const byID = new Map(result.map(({ id, text }) => [id, text]));
      for (const { id, node, html, prefix, suffix } of entries) {
        const text = byID.get(id);
        if (text == null) continue;
        if (html) node.innerHTML = text;
        else {
          const fragment = doc.createElement('div');
          fragment.innerHTML = text;
          // The HTML engine trims fragment boundaries; keep the spaces that
          // separate direct text from neighbouring links and inline elements.
          node.nodeValue = prefix + fragment.textContent.trim() + suffix;
        }
      }
    },
  };
}
// An obsolete request must never overwrite a newer language, even after abort.
export function createTranslationRequest() {
  let current;
  return {
    cancel() { current?.abort(); current = undefined; },
    start() {
      current?.abort();
      const controller = new AbortController();
      current = controller;
      return { signal: controller.signal, isCurrent: () => current === controller && !controller.signal.aborted };
    },
  };
}
