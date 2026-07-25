<script>
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte.js';

  let { pages = [], activePageID = '', onSelect } = $props();
  let open = $state(true);

  function selectPage(pageId) {
    onSelect?.(pageId);
    if (globalThis.matchMedia?.('(max-width: 900px)').matches) open = false;
  }
</script>

<div class="page-navigation" class:open class:closed={!open}>
  {#if open}
    <aside class="contents-menu scroll thin" aria-label={t('documents.contentsMenu')}>
      <header>
        <span>
          <b>{t('documents.contentsMenu')}</b>
          <small>{t('documents.contentsCount', { count: pages.length })}</small>
        </span>
        <button
          type="button"
          onclick={() => (open = false)}
          title={t('documents.hideContents')}
          aria-label={t('documents.hideContents')}
        ><Icon name="back" size={15} /></button>
      </header>
      <nav aria-label={t('documents.contentsMenu')}>
        {#each pages as page, index (page.id)}
          <button
            type="button"
            class="page-link"
            class:active={page.id === activePageID}
            aria-current={page.id === activePageID ? 'page' : undefined}
            onclick={() => selectPage(page.id)}
          >
            <span>{index + 1}</span>
            <b>{page.title}</b>
          </button>
        {/each}
      </nav>
    </aside>
  {:else}
    <button
      type="button"
      class="show-contents"
      onclick={() => (open = true)}
      title={t('documents.showContents')}
      aria-label={t('documents.showContents')}
    ><Icon name="book" size={17} /></button>
  {/if}
</div>

<style>
  .page-navigation{height:100%;flex:none;position:relative;background:var(--panel)}
  .page-navigation.open{width:250px}.page-navigation.closed{width:46px}
  .contents-menu{width:100%;height:100%;box-sizing:border-box;overflow-y:auto;padding:18px 13px;border-right:1px solid var(--border);background:var(--panel)}
  header{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:12px;padding:0 5px}
  header>span{min-width:0;display:flex;flex-direction:column;gap:2px}
  header b{color:var(--ink);font-size:11px;letter-spacing:.08em;text-transform:uppercase}
  header small{color:var(--faint);font-size:9.5px}
  header button,.show-contents{display:grid;place-items:center;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted)}
  header button{width:27px;height:27px;flex:none}
  nav{display:grid;gap:3px}
  .page-link{width:100%;min-width:0;display:grid;grid-template-columns:22px minmax(0,1fr);align-items:start;gap:7px;padding:8px 9px;border:1px solid transparent;border-radius:var(--r-sm);background:transparent;color:var(--muted);text-align:left}
  .page-link:hover{background:var(--raise);color:var(--ink)}
  .page-link.active{border-color:var(--accent-line);background:var(--accent-weak);color:var(--ink)}
  .page-link span{padding-top:1px;color:var(--faint);font-size:9px;text-align:right}
  .page-link b{overflow:hidden;font-size:11.5px;font-weight:550;line-height:1.35;text-overflow:ellipsis}
  .show-contents{width:32px;height:32px;margin:12px 7px;color:var(--accent-2)}
  header button:hover,.show-contents:hover{border-color:var(--accent-line);color:var(--ink)}
  :global(:root[data-skin="retro"]) :is(.contents-menu,.show-contents,header button){border-radius:0}
  @media(max-width:900px){
    .page-navigation{position:absolute;z-index:8;left:0;top:0;bottom:0;height:auto;background:transparent}
    .page-navigation.open{width:min(290px,84vw);box-shadow:10px 0 28px color-mix(in srgb,#000 32%,transparent)}
    .page-navigation.closed{width:0}
    .show-contents{position:absolute;left:8px;top:8px;margin:0;background:var(--panel);box-shadow:var(--shadow)}
  }
</style>
