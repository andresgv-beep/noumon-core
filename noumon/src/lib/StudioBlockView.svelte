<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioItemReference from './StudioItemReference.svelte';
  import { inline } from './studioEditable.js';

  let { block, documentId, pageIDs = [], onOpenItem, onOpenPage } = $props();

  function rendered(value) {
    return inline(value, { pageIDs });
  }

  function openInlineLink(event) {
    const link = event.target?.closest?.('[data-studio-link-kind]');
    if (!link || !event.currentTarget?.contains?.(link)) return;
    const kind = link.dataset.studioLinkKind;
    const id = link.dataset.studioLinkId;
    event.preventDefault();
    event.stopPropagation();
    if (kind === 'page') onOpenPage?.(id);
    else if (kind === 'item') onOpenItem?.(id);
  }

  // Misma acotacion que el editor y que el servidor (8-400 px), para que lo
  // que se ve al editar sea exactamente lo que se publica.
  function spacerHeight() {
    const value = Number(block.space);
    return Number.isFinite(value) ? Math.min(400, Math.max(8, Math.round(value))) : 48;
  }

  function headingId() {
    return `studio-section-${documentId}-${block.id}`;
  }

  function imageSize() {
    return ['original', 'medium', 'small', 'poster'].includes(block.imageSize)
      ? block.imageSize
      : 'original';
  }

  function imageAlign() {
    return ['left', 'center', 'right'].includes(block.imageAlign)
      ? block.imageAlign
      : 'center';
  }

  function imageHasSideText() {
    return ['medium', 'small'].includes(imageSize())
      && ['left', 'right'].includes(imageAlign())
      && String(block.sideText || '').trim();
  }

  function textSize() {
    const value = Number(block.fontSize);
    return Number.isInteger(value) && value >= 10 && value <= 96
      ? `${value}px`
      : undefined;
  }

  function textAlign() {
    return ['left', 'center', 'right'].includes(block.textAlign)
      ? block.textAlign
      : undefined;
  }
</script>

<!-- Los enlaces creados con {@html} delegan aquí el clic. display:contents
     conserva el flujo editorial de párrafos y fichas laterales. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="block-view" onclick={openInlineLink}>
{#if block.type === 'heading'}
  {@const level = Math.min(3, Math.max(1, block.level || 2))}
  {#if level === 1}<h1 id={headingId()} style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.text)}</h1>{:else if level === 2}<h2 id={headingId()} style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.text)}</h2>{:else}<h3 id={headingId()} style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.text)}</h3>{/if}
{:else if block.type === 'paragraph'}
  <p style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.text)}</p>
{:else if block.type === 'quote'}
  <blockquote style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.text)}</blockquote>
{:else if block.type === 'bulletList'}
  <ul style:font-size={textSize()} style:text-align={textAlign()}>{#each block.items || [] as item}<li>{@html rendered(item)}</li>{/each}</ul>
{:else if block.type === 'orderedList'}
  <ol style:font-size={textSize()} style:text-align={textAlign()}>{#each block.items || [] as item}<li>{@html rendered(item)}</li>{/each}</ol>
{:else if block.type === 'table'}
  <div class="table-scroll"><table style:font-size={textSize()}><tbody>{#each block.rows || [] as row, rowIndex}<tr>{#each row as cell}{#if rowIndex === 0}<th>{@html rendered(cell)}</th>{:else}<td>{@html rendered(cell)}</td>{/if}{/each}</tr>{/each}</tbody></table></div>
{:else if block.type === 'image'}
  <div
    class={`image-layout image-${imageSize()} align-${imageAlign()}`}
    class:with-side-text={imageHasSideText()}
  >
    <figure>
      <StudioImage
        {documentId}
        assetId={block.assetId}
        alt={block.alt || ''}
        display={imageSize()}
      />
    {#if block.caption}<figcaption>{@html rendered(block.caption)}</figcaption>{/if}
    </figure>
    {#if imageHasSideText()}
      <div class="image-side-text" style:font-size={textSize()} style:text-align={textAlign()}>{@html rendered(block.sideText)}</div>
    {/if}
  </div>
{:else if block.type === 'code'}
  <pre style:font-size={textSize()} style:text-align={textAlign()}><code>{block.text || ''}</code></pre>
{:else if block.type === 'callout'}
  <aside class="callout" style:font-size={textSize()} style:text-align={textAlign()}>
    {#if block.title}<b>{@html rendered(block.title)}</b>{/if}
    {#if block.text}<p>{@html rendered(block.text)}</p>{/if}
    {#each block.children || block.blocks || [] as child (child.id)}
      <StudioBlockView block={child} {documentId} {pageIDs} {onOpenItem} {onOpenPage} />
    {/each}
  </aside>
{:else if block.type === 'columns'}
  <div
    class="columns"
    class:single={(block.columns || []).length === 1}
    class:three={(block.columns || []).length === 3}
    class:lead-left={block.layout === 'lead-left'}
    class:lead-right={block.layout === 'lead-right'}
    class:half-left={block.layout === 'half-left'}
    class:half-right={block.layout === 'half-right'}
  >
    {#each block.columns || [] as column}
      <div>{#each column as child (child.id)}<StudioBlockView block={child} {documentId} {pageIDs} {onOpenItem} {onOpenPage} />{/each}</div>
    {/each}
  </div>
{:else if block.type === 'itemRef'}
  <StudioItemReference
    itemId={block.itemId}
    titleSnapshot={block.titleSnapshot}
    kindSnapshot={block.kindSnapshot}
    {onOpenItem}
  />
{:else if block.type === 'divider'}
  <hr />
{:else if block.type === 'spacer'}
  <!-- Aquí el hueco es hueco de verdad: sin rayado ni controles, solo el aire
       que pidió el autor. El rayado del editor es únicamente para verlo. -->
  <div class="spacer" style:height={`${spacerHeight()}px`} aria-hidden="true"></div>
{/if}
</div>

<style>
  .block-view{display:contents}
  h1{font-size:clamp(28px,4vw,44px);line-height:1.12;margin:38px 0 12px}
  h2{font-size:26px;line-height:1.25;margin:38px 0 9px}
  h3{font-size:19px;margin:30px 0 7px}
  p{white-space:pre-wrap}
  blockquote{margin:28px 0;border-left:3px solid var(--accent);padding:10px 20px;background:var(--raise);color:var(--ink-dim)}
  figure{margin:30px 0;min-width:0}
  .image-layout{width:100%}
  .image-layout.image-medium{width:min(72%,760px)}.image-layout.image-small{width:min(40%,420px)}
  .image-layout.image-poster,.image-layout.image-original{width:100%}
  .image-layout.align-left{margin-right:auto}.image-layout.align-center{margin-inline:auto}.image-layout.align-right{margin-left:auto}
  .image-layout.with-side-text{display:flex;align-items:flex-start;gap:clamp(24px,4vw,52px);width:100%}
  .image-layout.with-side-text figure{flex:0 0 58%;min-width:0}
  .image-layout.with-side-text.image-small figure{flex-basis:40%}
  .image-layout.with-side-text.align-right figure{order:2}
  .image-layout.with-side-text figure{margin-top:30px;margin-bottom:30px}
  .image-side-text{flex:1;min-width:0;overflow-wrap:anywhere;margin:30px 0;color:var(--ink-dim);line-height:1.75;white-space:pre-wrap}
  figcaption{margin-top:8px;overflow-wrap:anywhere;text-align:center;color:var(--muted);font-family:var(--font,system-ui,sans-serif);font-size:12px}
  .table-scroll{overflow:auto;margin:24px 0}
  table{width:100%;border-collapse:collapse;font-family:var(--font,system-ui,sans-serif);font-size:14px}
  th,td{border:1px solid var(--border);padding:9px;text-align:left}
  th{background:var(--raise)}
  pre{overflow:auto;margin:24px 0;padding:16px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel-2);font:13px/1.6 var(--mono,ui-monospace,monospace)}
  code{white-space:pre}
  .callout{margin:24px 0;padding:16px 18px;border-left:4px solid var(--accent);border-radius:var(--r-md);background:var(--raise)}
  .callout>b{display:block;margin-bottom:5px}.callout>p{margin:0}
  .columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:clamp(18px,4vw,42px);margin:26px 0}
  .columns.single{grid-template-columns:minmax(0,1fr)}
  .columns.single.half-left,.columns.single.half-right{grid-template-columns:minmax(0,50%)}
  .columns.single.half-left{justify-content:start}.columns.single.half-right{justify-content:end}
  .columns.three{grid-template-columns:repeat(3,minmax(0,1fr));gap:clamp(16px,2.5vw,30px)}
  .columns.lead-left{grid-template-columns:minmax(0,2fr) minmax(0,1fr)}
  .columns.lead-right{grid-template-columns:minmax(0,1fr) minmax(0,2fr)}
  hr{border:0;border-top:1px solid var(--border);margin:32px 0}
  .spacer{width:100%}
  :global(.studio-inline-link){color:var(--accent-2);text-decoration:underline;text-decoration-thickness:1px;text-underline-offset:3px;cursor:pointer}
  :global(.studio-inline-link:hover){color:var(--ink)}
  :global(.studio-inline-link.is-broken){color:#df7474;text-decoration-style:wavy}
  @media(max-width:680px){
    .columns,.columns.single,.columns.single.half-left,.columns.single.half-right,.columns.three,.columns.lead-left,.columns.lead-right{grid-template-columns:1fr;justify-content:stretch}
    .image-layout.image-medium{width:min(86%,760px)}.image-layout.image-small{width:min(62%,420px)}
    .image-layout.with-side-text{display:grid;width:100%}
    .image-layout.with-side-text figure{width:min(86%,620px);margin-bottom:0}
    .image-layout.with-side-text.align-right figure{order:0;margin-left:auto}
    .image-layout.with-side-text .image-side-text{margin-top:10px}
  }
</style>
