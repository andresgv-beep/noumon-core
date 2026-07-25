<script>
  import { t } from './i18n.svelte.js';
  import { getPublishedDocument, getPublishedDocumentRelations } from './studioApi.js';
  import StudioDocumentView from './StudioDocumentView.svelte';
  import { studioPage, studioPages } from './studioContent.js';

  let { tab, onOpenItem, onToc, onPages, onPageResolved } = $props();
  let document = $state(null);
  let loading = $state(true);
  let error = $state(false);
  let backlinks = $state([]);
  let related = $state([]);
  let loadSequence = 0;

  async function loadDocument(id, sequence) {
    try {
      const loadedDocument = await getPublishedDocument(id);
      if (sequence !== loadSequence) return;
      document = loadedDocument;
      try {
        const relations = await getPublishedDocumentRelations(id);
        if (sequence !== loadSequence) return;
        backlinks = relations.backlinks || [];
        related = relations.related || [];
      } catch {
        if (sequence !== loadSequence) return;
        backlinks = [];
        related = [];
      }
    }
    catch (e) {
      if (sequence !== loadSequence) return;
      error = true;
    }
    if (sequence !== loadSequence) return;
    loading = false;
  }

  // Reader se reutiliza al cambiar entre pestañas internas. onMount solo cargaba
  // la primera página y podía dejar visible un snapshot publicado antiguo (por
  // ejemplo, una portada ya retirada). La identidad del item gobierna la carga.
  $effect(() => {
    const id = String(tab.itemId || tab.open?.itemId || '').replace(/^studio:/, '');
    const sequence = ++loadSequence;
    document = null;
    backlinks = [];
    related = [];
    error = false;
    loading = true;
    onToc?.([]);
    onPages?.([], '');
    if (id) void loadDocument(id, sequence);
    else {
      error = true;
      loading = false;
    }
    return () => {
      if (loadSequence === sequence) loadSequence++;
    };
  });

  $effect(() => {
    const pages = studioPages(document);
    const page = studioPage(document, tab.pageId);
    onPages?.(pages, page?.id || '');
    if (page && tab.pageId !== page.id) onPageResolved?.(page.id);
  });
</script>

<div class="surface scroll thin">
  {#if loading}
    <div class="state">{t('common.loading')}</div>
  {:else if error || !document}
    <div class="state">{t('documents.loadError')}</div>
  {:else}
    <StudioDocumentView {document} pageId={tab.pageId} {onOpenItem} {onToc} />
    {#if backlinks.length}
      <section class="backlinks">
        <span>{t('documents.linksHere')}</span>
        <div>
          {#each backlinks as item (item.id)}
            <button onclick={() => onOpenItem?.(`studio:${item.id}`)}>
              <b>{item.title}</b>
              <small>{item.summary || t('documents.noSummary')}</small>
            </button>
          {/each}
        </div>
      </section>
    {/if}
    {#if related.length}
      <section class="backlinks related">
        <span>{t('documents.related')}</span>
        <div>
          {#each related as item (item.id)}
            <button onclick={() => onOpenItem?.(`studio:${item.id}`)}>
              <b>{item.title}</b>
              <small>{item.summary || t('documents.noSummary')}</small>
            </button>
          {/each}
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  .surface{flex:1;min-width:0;height:100%;overflow:auto;background:var(--panel-2);padding:clamp(22px,5vw,70px)}
  .state{padding:70px;text-align:center;color:var(--muted)}
  .backlinks{max-width:760px;margin:24px auto 0;padding:18px;border-radius:var(--r-lg);background:var(--panel);border:1px solid var(--border)}
  .backlinks>span{display:block;margin-bottom:9px;color:var(--accent-2);font-size:10px;font-weight:700;letter-spacing:.09em;text-transform:uppercase}
  .backlinks>div{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:7px}
  .backlinks button{display:flex;min-width:0;flex-direction:column;align-items:flex-start;gap:4px;padding:11px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--raise);color:var(--ink);text-align:left}
  .backlinks button:hover{border-color:var(--accent-line)}
  .backlinks b{font-size:12px}.backlinks small{color:var(--muted);font-size:11px;line-height:1.4}
</style>
