<script>
  import { onMount } from 'svelte';
  import { t } from './i18n.svelte.js';
  import { getPublishedDocument, getPublishedDocumentRelations } from './studioApi.js';
  import StudioDocumentView from './StudioDocumentView.svelte';

  let { tab, onOpenItem } = $props();
  let document = $state(null);
  let loading = $state(true);
  let error = $state(false);
  let backlinks = $state([]);
  let related = $state([]);

  onMount(async () => {
    const id = String(tab.itemId || tab.open?.itemId || '').replace(/^studio:/, '');
    try {
      document = await getPublishedDocument(id);
      try {
        const relations = await getPublishedDocumentRelations(id);
        backlinks = relations.backlinks || [];
        related = relations.related || [];
      } catch {
        backlinks = [];
        related = [];
      }
    }
    catch (e) { error = true; }
    loading = false;
  });
</script>

<div class="surface scroll thin">
  {#if loading}
    <div class="state">{t('common.loading')}</div>
  {:else if error || !document}
    <div class="state">{t('documents.loadError')}</div>
  {:else}
    <StudioDocumentView {document} {onOpenItem} />
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
  .surface{height:100%;overflow:auto;background:var(--panel-2);padding:clamp(22px,5vw,70px)}
  .state{padding:70px;text-align:center;color:var(--muted)}
  .backlinks{max-width:760px;margin:24px auto 0;padding:18px;border-radius:var(--r-lg);background:var(--panel);border:1px solid var(--border)}
  .backlinks>span{display:block;margin-bottom:9px;color:var(--accent-2);font-size:10px;font-weight:700;letter-spacing:.09em;text-transform:uppercase}
  .backlinks>div{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:7px}
  .backlinks button{display:flex;min-width:0;flex-direction:column;align-items:flex-start;gap:4px;padding:11px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--raise);color:var(--ink);text-align:left}
  .backlinks button:hover{border-color:var(--accent-line)}
  .backlinks b{font-size:12px}.backlinks small{color:var(--muted);font-size:11px;line-height:1.4}
</style>
