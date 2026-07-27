<script>
  import DocumentContentsMenu from './DocumentContentsMenu.svelte';
  import StudioBlockView from './StudioBlockView.svelte';
  import StudioInfoCard from './StudioInfoCard.svelte';
  import {
    studioDocumentBlocks, studioDocumentHeading, studioPage, studioPages, studioPageInfoCards,
    studioInfoCardHasContent, studioInfoCardsByAnchor,
  } from './studioContent.js';

  let {
    document, pageId = '', preview = false, expanded = false,
    onOpenItem, onOpenPage, onToc,
  } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};
  const activePage = () => studioPage(document, pageId);
  const pageIDs = () => studioPages(document).map((page) => page.id);
  // El encabezado del artículo es el título del DOCUMENTO; el de la página
  // nombra la página en el menú de contenidos y no encabeza nada.
  const heading = () => studioDocumentHeading(document, pageId);
  // Fichas de ESTA página. Una ficha sin nada dentro no se publica, aunque en el
  // editor siga visible para poder rellenarla.
  const infoCards = () => studioPageInfoCards(document, pageId).filter(studioInfoCardHasContent);
  const hasInfoCard = () => infoCards().length > 0;
  const cardSlots = () => studioInfoCardsByAnchor(infoCards(), bodyBlocks());
  const pageBlocks = () => studioDocumentBlocks(document, pageId);
  const bodyBlocks = () => pageBlocks();

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
  class:has-nav={studioPages(document).length > 1}
  class:nav-right={presentation().navSide === 'right'}
  style:--nav-w={`${Math.min(320, Math.max(150, Number(presentation().navWidth) || 196))}px`}
  class:compact={presentation().contentWidth === 'compact'}
  class:wide={presentation().contentWidth === 'wide'}
  class:editorial={presentation().contentWidth === 'editorial'}
>
  {#if studioPages(document).length > 1}
    <DocumentContentsMenu
      pages={studioPages(document)}
      activePageID={activePage()?.id || ''}
      title={content().navTitle || ''}
      frame={presentation().navFrame || 'none'}
      fontSize={presentation().navFontSize || 0}
      numbers={presentation().navNumbers !== false}
      count={presentation().navCount !== false}
      onSelect={onOpenPage}
    />
  {/if}
  <article
    class="page"
    class:preview
    class:expanded
    class:compact={presentation().contentWidth === 'compact'}
    class:wide={presentation().contentWidth === 'wide'}
    class:editorial={presentation().contentWidth === 'editorial'}
    class:sans={presentation().fontPreset === 'sans'}
  >
    <!-- La página no impone nada: ni encabezado, ni entradilla, ni línea de
         autor. Muestra solo los bloques que puso el autor, porque con este
         editor se monta lo que uno quiera y no necesariamente un artículo.
         El título, el resumen y el autor siguen existiendo como metadatos: los
         usan la biblioteca, la pestaña, el menú de páginas y el buscador, que
         los indexa del modelo y no de lo que aquí se pinte. -->
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
      {#each cardSlots().get(block.id) || [] as card (card.id)}
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
  /* Índice y artículo forman una sola banda centrada, como una web de
     documentación. El artículo conserva su ancho de lectura y el índice se le
     suma a la izquierda, en vez de vivir pegado al borde de la ventana. */
  .document-layout.has-nav{display:flex;align-items:flex-start;justify-content:center;gap:clamp(18px,3vw,44px)}
  /* Arranca por debajo del titulo, no a la altura del borde superior: pegado
     arriba competia con el encabezado del articulo. Es un desplazamiento fijo,
     asi que en una pagina que no empiece por un titulo quedara algo baja. */
  .document-layout.has-nav>:global(.page-nav){width:var(--nav-w,196px);flex:none;margin-top:clamp(104px,11vw,176px)}
  /* El lado se resuelve con el orden de la banda, no moviendo nada. */
  .document-layout.has-nav.nav-right>:global(.page-nav){order:2}
  /* margin:0 en la banda: `.page` lleva `margin:0 auto`, y en flex los márgenes
     automáticos absorben todo el espacio libre, empujando el artículo lejos del
     índice. Centrar la banda entera es cosa de justify-content, no del margen. */
  /* min-width:0 es obligatorio en un flex item: sin el no puede encogerse por
     debajo del ancho minimo de su contenido, y una tabla o una cadena larga
     desbordan la banda y sacan barra horizontal. */
  .document-layout.has-nav>.page{flex:0 1 auto;min-width:0;margin:0}
  @media(max-width:900px){
    /* En estrecho el índice se apila encima y la banda vuelve a una columna. */
    .document-layout.has-nav{display:block}
    .document-layout.has-nav>:global(.page-nav){width:auto;margin:0 auto;padding:0 clamp(20px,3.2vw,60px)}
  }
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
  footer{clear:both;display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  @media(max-width:820px){
    /* Sin sitio para envolver: la ficha deja de flotar y ocupa el ancho de la
       página, pero sigue dentro del artículo. */
    .info-slot,.info-slot.left{float:none;width:100%;margin:0 0 28px}
  }
</style>
