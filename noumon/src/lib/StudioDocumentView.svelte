<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import StudioImage from './StudioImage.svelte';
  import { studioDocumentBlocks, studioPage } from './studioContent.js';
  import { t, relTime } from './i18n.svelte.js';

  let { document, pageId = '', preview = false, expanded = false, onOpenItem, onToc } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};
  const activePage = () => studioPage(document, pageId);
  const pageTitle = () => activePage()?.title || document.title;
  const pageBlocks = () => studioDocumentBlocks(document, pageId);
  const cover = () => pageBlocks().find((block) => block?.type === 'image' && block.role === 'cover') || null;
  const bodyBlocks = () => pageBlocks().filter((block) => block?.role !== 'cover');

  function pageTextSize(field) {
    const value = Number(presentation()[field]);
    return Number.isInteger(value) && value >= 10 && value <= 96
      ? `${value}px`
      : undefined;
  }

  function pageTextAlign(field) {
    const value = presentation()[field];
    return ['left', 'center', 'right'].includes(value) ? value : undefined;
  }

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
    onToc?.(collectHeadings(pageBlocks()));
  });
</script>

<div class="document-layout">
  <article
    class="page"
    class:preview
    class:expanded
    class:compact={presentation().contentWidth === 'compact'}
    class:wide={presentation().contentWidth === 'wide'}
    class:editorial={presentation().contentWidth === 'editorial'}
    class:sans={presentation().fontPreset === 'sans'}
  >
    <header>
      <span>{document.classification?.workType || content().classification?.workType || t('documents.article')}</span>
      <h1 style:font-size={pageTextSize('titleFontSize')} style:text-align={pageTextAlign('titleTextAlign')}>{pageTitle()}</h1>
      {#if document.summary}<p class="lead" style:font-size={pageTextSize('summaryFontSize')} style:text-align={pageTextAlign('summaryTextAlign')}>{document.summary}</p>{/if}
      <div class="meta">
        {document.authorLabel || t('documents.localAuthor')}
        {#if document.published || document.updated} · {relTime(document.published || document.updated)}{/if}
      </div>
    </header>

    {#if cover()}
      <figure class="cover">
        <StudioImage documentId={document.id} assetId={cover().assetId} alt={cover().alt || pageTitle()} display="poster" />
        {#if cover().caption}<figcaption>{cover().caption}</figcaption>{/if}
      </figure>
    {/if}

    {#each bodyBlocks() as block (block.id)}
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
  .page.preview{max-width:912px}.page.preview.compact{max-width:744px}.page.preview.wide{max-width:980px}.page.preview.editorial{max-width:1180px}
  .page.preview.expanded{max-width:1000px}.page.preview.expanded.compact{max-width:820px}.page.preview.expanded.wide{max-width:1120px}.page.preview.expanded.editorial{max-width:1340px}
  header{border-bottom:1px solid var(--border);padding-bottom:28px;margin-bottom:34px}
  header>span{font-family:var(--font,system-ui,sans-serif);font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{font-size:clamp(30px,5vw,52px);line-height:1.08;margin:8px 0 16px}
  .lead{font-size:18px;color:var(--muted);line-height:1.55}
  .meta{font-family:var(--font,system-ui,sans-serif);font-size:12px;color:var(--faint)}
  .cover{margin:0 0 38px}.cover :global(img),.cover :global(.placeholder){width:100%;max-height:560px;object-fit:cover;border-radius:var(--r-md)}.cover figcaption{margin-top:8px;color:var(--muted);font:12px var(--font,system-ui,sans-serif);text-align:center}
  footer{display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .preview header{padding-bottom:20px;margin-bottom:26px}
  .preview h1{font-size:30px}
</style>
