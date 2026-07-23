<script>
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { t, relTime } from './i18n.svelte.js';
  import { listPublishedDocuments } from './studioApi.js';

  let { onOpenItem } = $props();
  let documents = $state([]);
  let query = $state('');
  let loading = $state(true);
  let denied = $state(false);
  let filtered = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return documents;
    return documents.filter((doc) =>
      [doc.title, doc.summary, doc.authorLabel, ...(doc.tags || [])]
        .join(' ').toLocaleLowerCase().includes(needle));
  });

  onMount(async () => {
    try {
      documents = await listPublishedDocuments();
    } catch (e) {
      denied = e.status === 401 || e.status === 403;
    }
    loading = false;
  });
</script>

<section class="documents scroll thin">
  <header class="hero">
    <div class="mark"><Icon name="note" size={25} /></div>
    <div>
      <span>{t('documents.localWiki')}</span>
      <h1>{t('menu.documents')}</h1>
      <p>{t('documents.subtitle')}</p>
    </div>
  </header>

  <div class="search">
    <Icon name="search" size={17} />
    <input bind:value={query} placeholder={t('documents.search')} />
  </div>

  {#if loading}
    <div class="state">{t('common.loading')}</div>
  {:else if denied}
    <div class="state"><Icon name="lock" /> <h2>{t('documents.loginTitle')}</h2><p>{t('documents.loginBody')}</p></div>
  {:else if documents.length === 0}
    <div class="state"><Icon name="note" /> <h2>{t('documents.emptyTitle')}</h2><p>{t('documents.emptyBody')}</p></div>
  {:else}
    <div class="count">{filtered.length} {t('documents.published')}</div>
    <div class="grid">
      {#each filtered as doc (doc.id)}
        <button class="doc" onclick={() => onOpenItem?.(`studio:${doc.id}`)}>
          <div class="doc-top">
            <span class="type">{doc.classification?.workType || t('documents.article')}</span>
            <small>{relTime(doc.published || doc.updated)}</small>
          </div>
          <h2>{doc.title}</h2>
          <p>{doc.summary || t('documents.noSummary')}</p>
          <div class="byline">
            <span>{doc.authorLabel || t('documents.localAuthor')}</span>
            <span class="arrow">→</span>
          </div>
          {#if doc.tags?.length}
            <div class="tags">{#each doc.tags.slice(0, 4) as tag}<span>{tag}</span>{/each}</div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</section>

<style>
  .documents{height:100%;overflow:auto;padding:clamp(28px,5vw,64px);background:var(--ground)}
  .hero{max-width:1050px;margin:0 auto 28px;display:flex;align-items:center;gap:18px}
  .mark{width:52px;height:52px;display:grid;place-items:center;border-radius:var(--r-lg);color:var(--accent-2);background:color-mix(in srgb,var(--accent) 18%,var(--panel));border:1px solid var(--accent-line)}
  .hero span{font-size:11px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{margin:2px 0 0;font-size:clamp(28px,4vw,44px);line-height:1}
  .hero p{margin:8px 0 0;color:var(--muted)}
  .search{max-width:1050px;margin:0 auto 24px;display:flex;align-items:center;gap:10px;padding:0 14px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel);color:var(--muted)}
  .search input{flex:1;border:0;outline:0;background:transparent;color:var(--ink);padding:13px 0;font-size:14px}
  .count{max-width:1050px;margin:0 auto 10px;color:var(--faint);font-size:11px;text-transform:uppercase;letter-spacing:.08em}
  .grid{max-width:1050px;margin:0 auto;display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:13px}
  .doc{min-height:230px;padding:20px;display:flex;flex-direction:column;text-align:left;background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow-soft);color:var(--ink);transition:transform .14s,border-color .14s}
  .doc:hover{transform:translateY(-2px);border-color:var(--accent-line)}
  .doc-top,.byline{display:flex;align-items:center;justify-content:space-between;gap:10px}
  .type{font-size:10px;font-weight:700;color:var(--accent-2);text-transform:uppercase;letter-spacing:.08em}.doc small{color:var(--faint)}
  .doc h2{margin:23px 0 8px;font-size:20px;line-height:1.25}.doc p{margin:0;color:var(--muted);line-height:1.5;font-size:13px;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden}
  .byline{margin-top:auto;padding-top:22px;color:var(--ink-dim);font-size:12px}.arrow{color:var(--accent-2);font-size:18px}
  .tags{display:flex;gap:5px;flex-wrap:wrap;margin-top:10px}.tags span{font-size:10px;padding:3px 7px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  .state{max-width:650px;margin:70px auto;text-align:center;color:var(--muted)}.state :global(.ic){color:var(--accent-2)}.state h2{color:var(--ink);margin-bottom:4px}.state p{margin-top:0}
</style>
