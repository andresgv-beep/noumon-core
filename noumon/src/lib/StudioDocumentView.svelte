<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import { t, relTime } from './i18n.svelte.js';

  let { document, preview = false, onOpenItem, onToc } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};

  function collectHeadings(blocks, result = [], depth = 0) {
    if (!Array.isArray(blocks) || depth > 12) return result;
    for (const block of blocks) {
      if (block?.type === 'heading' && String(block.text || '').trim()) {
        result.push({
          id: `studio-section-${document.id}-${block.id}`,
          level: Math.min(3, Math.max(1, block.level || 2)),
          text: String(block.text).trim(),
        });
      }
      collectHeadings(block?.children || block?.blocks, result, depth + 1);
      for (const column of block?.columns || []) collectHeadings(column, result, depth + 1);
    }
    return result;
  }

  // El índice de la página no vive en el documento: se entrega al lector, que lo
  // muestra en su barra lateral derecha (el mismo mecanismo que el resto de
  // contenidos). Así la página se ve como una página real, sin cajas flotantes.
  $effect(() => {
    onToc?.(collectHeadings(content().blocks));
  });
</script>

<div class="document-layout">
  <article
    class="page"
    class:preview
    class:compact={presentation().contentWidth === 'compact'}
    class:wide={presentation().contentWidth === 'wide'}
    class:editorial={presentation().contentWidth === 'editorial'}
    class:sans={presentation().fontPreset === 'sans'}
  >
    <header>
      <span>{document.classification?.workType || content().classification?.workType || t('documents.article')}</span>
      <h1>{document.title}</h1>
      {#if document.summary}<p class="lead">{document.summary}</p>{/if}
      <div class="meta">
        {document.authorLabel || t('documents.localAuthor')}
        {#if document.published || document.updated} · {relTime(document.published || document.updated)}{/if}
      </div>
    </header>

    {#each content().blocks || [] as block (block.id)}
      <StudioBlockView {block} documentId={document.id} {onOpenItem} />
    {/each}

    {#if document.tags?.length}
      <footer>{#each document.tags as tag}<span>{tag}</span>{/each}</footer>
    {/if}
  </article>
</div>

<style>
  .document-layout{width:100%}
  /* Página a ras, como cualquier página del navegador: sin marco de tarjeta
     (borde/sombra/fondo propio) y llenando el ancho de lectura. */
  .page{width:100%;max-width:1120px;box-sizing:border-box;margin:0 auto;padding:clamp(24px,3.2vw,52px) clamp(20px,3.2vw,60px) 64px;color:var(--ink);font-family:var(--font-read,Georgia,serif);line-height:1.75}
  .page.sans{font-family:var(--font,system-ui,sans-serif)}
  .page.compact{max-width:760px}.page.wide{max-width:1320px}.page.editorial{max-width:1500px}
  .page.preview{padding-top:clamp(20px,2.6vw,40px)}
  header{border-bottom:1px solid var(--border);padding-bottom:28px;margin-bottom:34px}
  header>span{font-family:var(--font,system-ui,sans-serif);font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{font-size:clamp(30px,5vw,52px);line-height:1.08;margin:8px 0 16px}
  .lead{font-size:18px;color:var(--muted);line-height:1.55}
  .meta{font-family:var(--font,system-ui,sans-serif);font-size:12px;color:var(--faint)}
  footer{display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .preview header{padding-bottom:20px;margin-bottom:26px}
  .preview h1{font-size:30px}
</style>
