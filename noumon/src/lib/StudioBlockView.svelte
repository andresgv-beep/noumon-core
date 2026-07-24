<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioItemReference from './StudioItemReference.svelte';

  let { block, documentId, onOpenItem } = $props();

  function escapeHTML(value) {
    return String(value || '').replace(/[&<>"']/g, (char) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    })[char]);
  }

  function inline(value) {
    return escapeHTML(value)
      .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
  }

  function headingId() {
    return `studio-section-${documentId}-${block.id}`;
  }
</script>

{#if block.type === 'heading'}
  {@const level = Math.min(3, Math.max(1, block.level || 2))}
  {#if level === 1}<h1 id={headingId()}>{@html inline(block.text)}</h1>{:else if level === 2}<h2 id={headingId()}>{@html inline(block.text)}</h2>{:else}<h3 id={headingId()}>{@html inline(block.text)}</h3>{/if}
{:else if block.type === 'paragraph'}
  <p>{@html inline(block.text)}</p>
{:else if block.type === 'quote'}
  <blockquote>{@html inline(block.text)}</blockquote>
{:else if block.type === 'bulletList'}
  <ul>{#each block.items || [] as item}<li>{@html inline(item)}</li>{/each}</ul>
{:else if block.type === 'orderedList'}
  <ol>{#each block.items || [] as item}<li>{@html inline(item)}</li>{/each}</ol>
{:else if block.type === 'table'}
  <div class="table-scroll"><table><tbody>{#each block.rows || [] as row, rowIndex}<tr>{#each row as cell}{#if rowIndex === 0}<th>{@html inline(cell)}</th>{:else}<td>{@html inline(cell)}</td>{/if}{/each}</tr>{/each}</tbody></table></div>
{:else if block.type === 'image'}
  <figure>
    <StudioImage {documentId} assetId={block.assetId} alt={block.alt || ''} />
    {#if block.caption}<figcaption>{@html inline(block.caption)}</figcaption>{/if}
  </figure>
{:else if block.type === 'code'}
  <pre><code>{block.text || ''}</code></pre>
{:else if block.type === 'callout'}
  <aside class="callout">
    {#if block.title}<b>{@html inline(block.title)}</b>{/if}
    {#if block.text}<p>{@html inline(block.text)}</p>{/if}
    {#each block.children || block.blocks || [] as child (child.id)}
      <StudioBlockView block={child} {documentId} {onOpenItem} />
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
      <div>{#each column as child (child.id)}<StudioBlockView block={child} {documentId} {onOpenItem} />{/each}</div>
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
{/if}

<style>
  h1{font-size:clamp(28px,4vw,44px);line-height:1.12;margin:38px 0 12px}
  h2{font-size:26px;line-height:1.25;margin:38px 0 9px}
  h3{font-size:19px;margin:30px 0 7px}
  p{white-space:pre-wrap}
  blockquote{margin:28px 0;border-left:3px solid var(--accent);padding:10px 20px;background:var(--raise);color:var(--ink-dim)}
  figure{margin:30px 0}
  figcaption{margin-top:8px;text-align:center;color:var(--muted);font-family:var(--font,system-ui,sans-serif);font-size:12px}
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
  @media(max-width:680px){
    .columns,.columns.single,.columns.single.half-left,.columns.single.half-right,.columns.three,.columns.lead-left,.columns.lead-right{grid-template-columns:1fr;justify-content:stretch}
  }
</style>
