<script>
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte.js';
  import { studioPages } from './studioContent.js';

  let {
    document,
    activePageID = '',
    onSelect,
    onCreate,
    onRename,
    onMove,
    onRemove,
  } = $props();

  let renamingPageID = $state('');
  let titleDraft = $state('');
  let titleInput = $state();

  async function beginRename(page) {
    renamingPageID = page.id;
    titleDraft = page.title;
    await tick();
    titleInput?.focus();
    titleInput?.select();
  }

  function cancelRename() {
    renamingPageID = '';
    titleDraft = '';
  }

  function commitRename(page) {
    const title = titleDraft.trim();
    if (!title) {
      cancelRename();
      return;
    }
    onRename?.(page.id, title);
    cancelRename();
  }

  function renameKeydown(event, page) {
    if (event.key === 'Enter') {
      event.preventDefault();
      commitRename(page);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancelRename();
    }
  }
</script>

<section class="page-manager" aria-label={t('studio.pages')}>
  <header>
    <span>
      <b>{t('studio.pages')}</b>
      <small>{t('studio.pagesCount', { count: studioPages(document).length })}</small>
    </span>
    <button
      type="button"
      class="add-page"
      onclick={() => onCreate?.()}
      disabled={studioPages(document).length >= 100}
      title={studioPages(document).length >= 100 ? t('studio.pageLimitReached') : t('studio.addPage')}
      aria-label={studioPages(document).length >= 100 ? t('studio.pageLimitReached') : t('studio.addPage')}
    ><Icon name="plus" size={14} /></button>
  </header>

  <div class="page-list">
    {#each studioPages(document) as page, index (page.id)}
      <div class="page-row" class:active={page.id === activePageID}>
        <span class="page-order">{index + 1}</span>
        {#if renamingPageID === page.id}
          <input
            class="page-title-input"
            bind:this={titleInput}
            bind:value={titleDraft}
            maxlength="240"
            aria-label={t('studio.pageTitle')}
            onkeydown={(event) => renameKeydown(event, page)}
            onblur={() => commitRename(page)}
          />
        {:else}
          <button
            type="button"
            class="page-select"
            onclick={() => onSelect?.(page.id)}
            aria-current={page.id === activePageID ? 'page' : undefined}
            title={page.title}
          >
            <b>{page.title}</b>
            <small>{t('studio.blocksCount', { count: page.blocks?.length || 0 })}</small>
          </button>
        {/if}
        <div class="page-actions">
          <button
            type="button"
            onclick={() => beginRename(page)}
            title={t('studio.renamePage')}
            aria-label={t('studio.renamePage')}
          ><Icon name="edit" size={12} /></button>
          <button
            type="button"
            onclick={() => onMove?.(page.id, -1)}
            disabled={index === 0}
            title={t('studio.movePageUp')}
            aria-label={t('studio.movePageUp')}
          >↑</button>
          <button
            type="button"
            onclick={() => onMove?.(page.id, 1)}
            disabled={index === studioPages(document).length - 1}
            title={t('studio.movePageDown')}
            aria-label={t('studio.movePageDown')}
          >↓</button>
          <button
            type="button"
            class="remove-page"
            onclick={() => onRemove?.(page.id)}
            disabled={studioPages(document).length <= 1}
            title={studioPages(document).length <= 1 ? t('studio.keepOnePage') : t('studio.removePage')}
            aria-label={studioPages(document).length <= 1 ? t('studio.keepOnePage') : t('studio.removePage')}
          >×</button>
        </div>
      </div>
    {/each}
  </div>

  <button
    type="button"
    class="new-page"
    onclick={() => onCreate?.()}
    disabled={studioPages(document).length >= 100}
    title={studioPages(document).length >= 100 ? t('studio.pageLimitReached') : t('studio.addPage')}
  ><Icon name="plus" size={13} /> {t('studio.addPage')}</button>
</section>

<style>
  .page-manager{display:grid;gap:8px;padding-bottom:9px;border-bottom:1px solid var(--border)}
  header{display:flex;align-items:center;justify-content:space-between;gap:8px}
  header>span{min-width:0;display:flex;flex-direction:column;gap:1px}
  header b{color:var(--ink);font-size:10.5px}
  header small{color:var(--faint);font-size:8.5px}
  button{border:0;font:inherit}
  .add-page{width:26px;height:26px;display:grid;place-items:center;border-radius:var(--r-sm);background:var(--accent-weak);color:var(--accent-2)}
  .page-list{display:grid;gap:4px;max-height:240px;overflow:auto}
  .page-row{min-width:0;display:grid;grid-template-columns:18px minmax(0,1fr) auto;align-items:center;gap:4px;padding:4px;border:1px solid transparent;border-radius:var(--r-sm);background:var(--card)}
  .page-row.active{border-color:var(--accent-line);background:var(--accent-weak)}
  .page-order{color:var(--faint);font-size:8.5px;text-align:center}
  .page-select{min-width:0;display:flex;flex-direction:column;align-items:flex-start;gap:1px;padding:2px;background:transparent;color:var(--muted);text-align:left}
  .page-select b{width:100%;overflow:hidden;color:var(--ink);font-size:10px;text-overflow:ellipsis;white-space:nowrap}
  .page-select small{color:var(--faint);font-size:8px}
  .page-title-input{min-width:0;width:100%;padding:5px 6px;border:1px solid var(--accent-line);border-radius:var(--r-sm);background:var(--panel);color:var(--ink);font-size:10px;outline:0}
  .page-actions{display:flex;align-items:center;gap:1px;opacity:0;transition:opacity .12s}
  .page-row:hover .page-actions,.page-row:focus-within .page-actions,.page-row.active .page-actions{opacity:1}
  .page-actions button{width:20px;height:20px;display:grid;place-items:center;border-radius:3px;background:transparent;color:var(--faint);font-size:11px}
  .page-actions button:hover:not(:disabled){background:var(--raise);color:var(--ink)}
  .page-actions button:disabled{opacity:.25}.page-actions .remove-page:hover:not(:disabled){color:#df7474}
  .new-page{min-height:28px;display:flex;align-items:center;justify-content:center;gap:5px;border:1px dashed var(--border);border-radius:var(--r-sm);background:transparent;color:var(--muted);font-size:9.5px}
  .new-page:hover:not(:disabled){border-color:var(--accent-line);color:var(--ink)}
  button:disabled{cursor:not-allowed}
  :global(:root[data-skin="retro"]) .page-actions{opacity:1}
</style>
