<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import { t, relTime } from './i18n.svelte.js';

  let { document, preview = false, onOpenItem } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};

  function collectHeadings(blocks, result = []) {
    for (const block of blocks || []) {
      if (block?.type === 'heading' && String(block.text || '').trim()) {
        result.push({
          id: `studio-section-${document.id}-${block.id}`,
          level: Math.min(3, Math.max(1, block.level || 2)),
          text: String(block.text).trim(),
        });
      }
      collectHeadings(block?.children || block?.blocks, result);
      for (const column of block?.columns || []) collectHeadings(column, result);
    }
    return result;
  }

  const headings = () => collectHeadings(content().blocks);

  function openHeading(id) {
    const reduceMotion = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    globalThis.document?.getElementById(id)?.scrollIntoView({
      behavior: reduceMotion ? 'auto' : 'smooth',
      block: 'start',
    });
  }
</script>

<div class="document-layout">
  {#if headings().length > 0}
    <aside class="page-index">
      <span>{t('documents.pageIndex')}</span>
      <nav aria-label={t('documents.pageIndex')}>
        {#each headings() as heading (heading.id)}
          <button
            class:level-two={heading.level === 2}
            class:level-three={heading.level === 3}
            onclick={() => openHeading(heading.id)}
          >{heading.text}</button>
        {/each}
      </nav>
    </aside>
  {/if}

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
  .document-layout{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:start;column-gap:24px;width:100%}
  .page{grid-column:2;width:100%;max-width:800px;box-sizing:border-box;margin:0;background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);padding:clamp(32px,6vw,76px);color:var(--ink);font-family:var(--font-read,Georgia,serif);line-height:1.75}
  .page.sans{font-family:var(--font,system-ui,sans-serif)}
  .page.compact{max-width:640px}.page.wide{max-width:1100px}.page.editorial{max-width:1320px}
  .page.preview{padding:clamp(28px,5vw,58px);box-shadow:var(--shadow-soft)}
  header{border-bottom:1px solid var(--border);padding-bottom:28px;margin-bottom:34px}
  header>span{font-family:var(--font,system-ui,sans-serif);font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{font-size:clamp(30px,5vw,52px);line-height:1.08;margin:8px 0 16px}
  .lead{font-size:18px;color:var(--muted);line-height:1.55}
  .meta{font-family:var(--font,system-ui,sans-serif);font-size:12px;color:var(--faint)}
  footer{display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .preview header{padding-bottom:20px;margin-bottom:26px}
  .preview h1{font-size:30px}
  .page-index{position:sticky;top:24px;grid-column:1;justify-self:end;width:190px;padding:14px;border:1px solid var(--border);border-radius:var(--r-lg);background:color-mix(in srgb,var(--panel) 94%,transparent);box-shadow:var(--shadow-soft);font-family:var(--font,system-ui,sans-serif)}
  .page-index>span{display:block;padding:2px 8px 9px;color:var(--accent-2);font-size:10px;font-weight:700;letter-spacing:.09em;text-transform:uppercase}
  .page-index nav{display:flex;flex-direction:column;gap:2px}
  .page-index button{width:100%;padding:7px 8px;border:0;border-radius:var(--r-sm);background:transparent;color:var(--muted);font-size:12px;line-height:1.35;text-align:left;cursor:pointer}
  .page-index button:hover,.page-index button:focus-visible{background:var(--raise);color:var(--ink)}
  .page-index button.level-two{padding-left:16px}
  .page-index button.level-three{padding-left:26px;font-size:11px}
  @media(max-width:1120px){
    .document-layout{grid-template-columns:minmax(0,1fr)}
    .page{grid-column:1}
    .page-index{position:static;grid-column:1;justify-self:center;width:min(760px,100%);box-sizing:border-box;margin:0 auto 14px}
  }
</style>
