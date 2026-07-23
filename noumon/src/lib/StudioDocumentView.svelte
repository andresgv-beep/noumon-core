<script>
  import StudioBlockView from './StudioBlockView.svelte';
  import { t, relTime } from './i18n.svelte.js';

  let { document, preview = false, onOpenItem } = $props();

  const content = () => document?.content || {};
  const presentation = () => content().presentation || {};

</script>

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

<style>
  .page{width:100%;max-width:760px;box-sizing:border-box;margin:0 auto;background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);padding:clamp(32px,7vw,82px);color:var(--ink);font-family:var(--font-serif,Georgia,serif);line-height:1.75}
  .page.sans{font-family:var(--font-sans,system-ui,sans-serif)}
  .page.compact{max-width:600px}.page.wide{max-width:1050px}.page.editorial{max-width:900px}
  .page.preview{padding:clamp(28px,5vw,58px);box-shadow:var(--shadow-soft)}
  header{border-bottom:1px solid var(--border);padding-bottom:28px;margin-bottom:34px}
  header>span{font-family:var(--font-sans,system-ui,sans-serif);font-size:10px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{font-size:clamp(30px,5vw,52px);line-height:1.08;margin:8px 0 16px}
  .lead{font-size:18px;color:var(--muted);line-height:1.55}
  .meta{font-family:var(--font-sans,system-ui,sans-serif);font-size:12px;color:var(--faint)}
  footer{display:flex;gap:6px;flex-wrap:wrap;border-top:1px solid var(--border);margin-top:50px;padding-top:22px}
  footer span{font-family:var(--font-sans,system-ui,sans-serif);font-size:11px;padding:4px 9px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .preview header{padding-bottom:20px;margin-bottom:26px}
  .preview h1{font-size:30px}
</style>
