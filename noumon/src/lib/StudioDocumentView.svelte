<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioInfoCard from './StudioInfoCard.svelte';
  import {
    studioDocumentBlocks, studioPage, studioPages, studioPageInfoCards, studioInfoCardHasContent,
    studioInfoCardsByAnchor,
  } from './studioContent.js';
  import { t, relTime } from './i18n.svelte.js';

  let {
    document, pageId = '', preview = false, expanded = false,
    onOpenItem, onOpenPage, onToc,
  } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};
  const activePage = () => studioPage(document, pageId);
  const pageIDs = () => studioPages(document).map((page) => page.id);
  const pageTitle = () => activePage()?.title || document.title;
  // Fichas de ESTA página. Una ficha sin nada dentro no se publica, aunque en el
  // editor siga visible para poder rellenarla.
  const infoCards = () => studioPageInfoCards(document, pageId).filter(studioInfoCardHasContent);
  const hasInfoCard = () => infoCards().length > 0;
  const cardSlots = () => studioInfoCardsByAnchor(infoCards(), bodyBlocks().length);
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

<div
  class="document-layout"
  class:has-info-card={hasInfoCard()}
  class:compact={presentation().contentWidth === 'compact'}
  class:wide={presentation().contentWidth === 'wide'}
  class:editorial={presentation().contentWidth === 'editorial'}
>
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

    <!-- Las fichas viven DENTRO de la página, flotadas al lado que les toque: el
         texto las envuelve, como en cualquier artículo de enciclopedia. Cada una
         se emite junto al bloque en el que está anclada, porque un flotado
         empieza donde aparece en el flujo: así una ficha puede ir a media página
         y no siempre arriba del todo. -->
    {#if !bodyBlocks().length}
      {#each infoCards() as card (card.id)}
        <aside class="info-slot" class:left={card.side === 'left'}>
          <StudioInfoCard documentId={document.id} {card} compact={preview} />
        </aside>
      {/each}
    {/if}

    {#each bodyBlocks() as block, index (block.id)}
      {#each cardSlots().get(index) || [] as card (card.id)}
        <aside class="info-slot" class:left={card.side === 'left'}>
          <StudioInfoCard documentId={document.id} {card} compact={preview} />
        </aside>
      {/each}
      <StudioBlockView
        {block}
        documentId={document.id}
        pageIDs={pageIDs()}
        {onOpenItem}
        {onOpenPage}
      />
    {/each}

    {#if document.tags?.length}
      <footer>{#each document.tags as tag}<span>{tag}</span>{/each}</footer>
    {/if}
  </article>
</div>

<style>
  .document-layout{width:100%}
  /* La ficha pertenece al flujo del artículo y envuelve el texto, pero nunca
     cambia el ancho de página elegido por el autor. */
  .info-slot{position:relative;z-index:1;float:right;width:clamp(260px,32%,340px);margin:4px 0 22px clamp(20px,2.4vw,34px)}
  .info-slot.left{float:left;margin:4px clamp(20px,2.4vw,34px) 22px 0}
  /* Los bloques con cuerpo propio (figuras, tablas, citas, avisos) se estrechan
     junto a la ficha en vez de deslizarse por debajo: flow-root crea contexto
     de bloque sin recortar nada. Párrafos y títulos sí la envuelven. */
  .page :global(figure),.page :global(.table-scroll),.page :global(blockquote),.page :global(.callout),.page :global(.columns){display:flow-root}
  /* Página a ras, como cualquier página del navegador: sin marco de tarjeta
     (borde/sombra/fondo propio) y llenando el ancho de lectura. */
  /* overflow-wrap explícito: el editor lo recibe gratis del navegador (regla de
     agente de usuario sobre contenteditable), la página publicada no. Sin él una
     cadena larga sin espacios no se puede partir, se desborda a lo ancho y el
     navegador la empuja por debajo de la ficha. */
  .page{width:100%;max-width:760px;box-sizing:border-box;margin:0 auto;padding:clamp(24px,3.2vw,52px) clamp(20px,3.2vw,60px) 64px;color:var(--ink);font-family:var(--font-read,Georgia,serif);line-height:1.75;overflow-wrap:break-word}
  .page.sans{font-family:var(--font,system-ui,sans-serif)}
  .page.compact{max-width:620px}.page.wide{max-width:980px}.page.editorial{max-width:1180px}
  .page.preview{padding-top:clamp(20px,2.6vw,40px)}
  header{border-bottom:1px solid var(--border);padding-bottom:18px;margin-bottom:24px}
  header>span{font-family:var(--font,system-ui,sans-serif);font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  /* Título de artículo, no de portada: prominente pero proporcionado al texto. */
  h1{font-size:clamp(28px,3.2vw,40px);line-height:1.12;margin:6px 0 12px}
  .lead{font-size:18px;color:var(--muted);line-height:1.55}
  .meta{font-family:var(--font,system-ui,sans-serif);font-size:12px;color:var(--faint)}
  .cover{margin:0 0 38px}.cover :global(img),.cover :global(.placeholder){width:100%;max-height:560px;object-fit:cover;border-radius:var(--r-md)}.cover figcaption{margin-top:8px;color:var(--muted);font:12px var(--font,system-ui,sans-serif);text-align:center}
  footer{clear:both;display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .preview header{padding-bottom:20px;margin-bottom:26px}
  .preview h1{font-size:30px}
  @media(max-width:820px){
    /* Sin sitio para envolver: la ficha deja de flotar y ocupa el ancho de la
       página, pero sigue dentro del artículo. */
    .info-slot,.info-slot.left{float:none;width:100%;margin:0 0 28px}
  }
</style>
