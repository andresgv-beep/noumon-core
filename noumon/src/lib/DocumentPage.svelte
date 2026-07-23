<script>
  import { onMount } from 'svelte';
  import { t, relTime } from './i18n.svelte.js';
  import { getPublishedDocument } from './studioApi.js';
  import StudioImage from './StudioImage.svelte';

  let { tab } = $props();
  let document = $state(null);
  let loading = $state(true);
  let error = $state(false);

  onMount(async () => {
    const id = String(tab.itemId || tab.open?.itemId || '').replace(/^studio:/, '');
    try { document = await getPublishedDocument(id); }
    catch (e) { error = true; }
    loading = false;
  });

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
</script>

<div class="surface scroll thin">
  {#if loading}
    <div class="state">{t('common.loading')}</div>
  {:else if error || !document}
    <div class="state">{t('documents.loadError')}</div>
  {:else}
    <article class="page" class:wide={document.content?.presentation?.contentWidth === 'wide'} class:compact={document.content?.presentation?.contentWidth === 'compact'}>
      <header>
        <span>{document.classification?.workType || t('documents.article')}</span>
        <h1>{document.title}</h1>
        {#if document.summary}<p class="lead">{document.summary}</p>{/if}
        <div class="meta">{document.authorLabel || t('documents.localAuthor')} · {relTime(document.published || document.updated)}</div>
      </header>
      {#each document.content?.blocks || [] as block (block.id)}
        {#if block.type === 'heading'}
          {@const level = Math.min(3, Math.max(1, block.level || 2))}
          {#if level === 1}<h1>{@html inline(block.text)}</h1>{:else if level === 2}<h2>{@html inline(block.text)}</h2>{:else}<h3>{@html inline(block.text)}</h3>{/if}
        {:else if block.type === 'paragraph'}<p>{@html inline(block.text)}</p>
        {:else if block.type === 'quote'}<blockquote>{@html inline(block.text)}</blockquote>
        {:else if block.type === 'bulletList'}
          <ul>{#each block.items || [] as item}<li>{@html inline(item)}</li>{/each}</ul>
        {:else if block.type === 'orderedList'}
          <ol>{#each block.items || [] as item}<li>{@html inline(item)}</li>{/each}</ol>
        {:else if block.type === 'table'}
          <div class="table-scroll"><table><tbody>{#each block.rows || [] as row, rowIndex}<tr>{#each row as cell}{#if rowIndex === 0}<th>{@html inline(cell)}</th>{:else}<td>{@html inline(cell)}</td>{/if}{/each}</tr>{/each}</tbody></table></div>
        {:else if block.type === 'image'}
          <figure>
            <StudioImage documentId={document.id} assetId={block.assetId} alt={block.alt || ''} />
            {#if block.caption}<figcaption>{@html inline(block.caption)}</figcaption>{/if}
          </figure>
        {:else if block.type === 'divider'}<hr />
        {/if}
      {/each}
      {#if document.tags?.length}<footer>{#each document.tags as tag}<span>{tag}</span>{/each}</footer>{/if}
    </article>
  {/if}
</div>

<style>
  .surface{height:100%;overflow:auto;background:var(--panel-2);padding:clamp(22px,5vw,70px)}
  .page{max-width:760px;margin:0 auto;background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);padding:clamp(32px,7vw,82px);color:var(--ink);line-height:1.75}
  .page.compact{max-width:600px}.page.wide{max-width:1050px}
  header{border-bottom:1px solid var(--border);padding-bottom:28px;margin-bottom:34px}header>span{font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{font-size:clamp(30px,5vw,52px);line-height:1.08;margin:8px 0 16px}h2{font-size:26px;line-height:1.25;margin:38px 0 9px}h3{font-size:19px;margin:30px 0 7px}
  p{white-space:pre-wrap}.lead{font-size:18px;color:var(--muted);line-height:1.55}.meta{font-size:12px;color:var(--faint)}
  blockquote{margin:28px 0;border-left:3px solid var(--accent);padding:10px 20px;background:var(--raise);color:var(--ink-dim)}
  figure{margin:30px 0}figcaption{margin-top:8px;text-align:center;color:var(--muted);font-size:12px}
  .table-scroll{overflow:auto;margin:24px 0}table{width:100%;border-collapse:collapse;font-size:14px}th,td{border:1px solid var(--border);padding:9px;text-align:left}th{background:var(--raise)}
  footer{display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}footer span{font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .state{padding:70px;text-align:center;color:var(--muted)}
</style>
