<script>
  import Icon from './Icon.svelte';
  import { t, relTime } from './i18n.svelte.js';

  let { state = {} } = $props();

  const draftCount = () => (state.documents || []).filter((doc) => !doc.publishedRevision).length;
  const publishedCount = () => (state.documents || []).filter((doc) => !!doc.publishedRevision).length;
  const kindLabel = (doc) => {
    if (doc?.templateKey?.startsWith('cabinet.')) return t('studio.createCabinet');
    if (doc?.templateKey?.startsWith('moments.')) return t('studio.createMoments');
    return t('studio.createDocument');
  };
</script>

<aside class="studio-side">
  {#if state.mode === 'home'}
    <span class="side-title">{t('studio.myDrafts')}</span>
    <div class="drafts scroll thin">
      {#each (state.documents || []).slice(0, 12) as doc (doc.id)}
        <button class="draft" onclick={() => state.openDocument?.(doc.id)}>
          <b>{doc.title || t('studio.untitled')}</b>
          <small>
            {kindLabel(doc)} · {relTime(doc.updated)}
            {#if doc.publishedRevision}<u>{t('studio.published')}</u>{/if}
          </small>
        </button>
      {/each}
      {#if !state.documents?.length}
        <p class="empty">{t('studio.empty')}</p>
      {/if}
    </div>
    <span class="side-title">{t('studio.status')}</span>
    <span class="side-item"><i class="private"></i>{t('studio.privateDraftCount', { count: draftCount() })}</span>
    <span class="side-item"><i class="published"></i>{t('studio.publishedCount', { count: publishedCount() })}</span>
  {:else}
    <div class="section-kind">
      <span class="glyph">{state.kindGlyph || '✎'}</span>
      <span><b>{state.kindLabel || t('studio.createDocument')}</b><small>{state.kindHint || t('studio.blockEditor')}</small></span>
    </div>
    <span class="side-title">{t('studio.editing')}</span>
    {#each state.sections || [] as section (section.key)}
      <button class="side-item nav-item" class:active={state.activeSection === section.key} onclick={() => state.openSection?.(section.key)}>
        <Icon name={section.icon || 'list'} size={15} />{section.label}
      </button>
    {/each}
    <span class="side-title">{t('studio.history')}</span>
    <button class="side-item nav-item" class:active={state.revisionsOpen} onclick={() => state.toggleRevisions?.()}>
      <Icon name="history" size={15} />{t('studio.revisions')}<span class="badge">{state.revisionCount || 0}</span>
    </button>
    <span class="side-title">{t('studio.publication')}</span>
    <span class="side-item"><Icon name="storage" size={15} />{state.destination || t('studio.destinationDocuments')}</span>
    <span class="side-item"><Icon name="download" size={15} />{state.quotaLabel || t('studio.quotaUnknown')}</span>
    {#if state.unpublish}
      <button class="unpublish" onclick={() => state.unpublish?.()}><Icon name="close" size={14} />{t('studio.unpublish')}</button>
    {/if}
    {#if state.canArchive && state.archive}
      <button class="archive" onclick={() => state.archive?.()}><Icon name="trash" size={14} />{t('studio.archive')}</button>
    {/if}
  {/if}
</aside>

<style>
  .studio-side{height:100%;min-height:0;overflow:hidden;display:flex;flex-direction:column;gap:4px;padding:18px 12px;background:var(--panel);border-right:1px solid var(--border);color:var(--ink)}
  .side-title{margin:10px 7px 5px;color:var(--faint);font-size:9px;font-weight:650;letter-spacing:.14em;text-transform:uppercase}
  .side-title:first-child{margin-top:0}
  .drafts{max-height:52%;overflow:auto;display:flex;flex-direction:column;gap:2px}
  .draft{display:flex;flex-direction:column;align-items:flex-start;gap:2px;width:100%;padding:8px 10px;border-radius:var(--r-md);text-align:left}
  .draft:hover{background:var(--raise)}
  .draft b{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;font-weight:600}
  .draft small{color:var(--muted);font-size:10.5px}.draft u{margin-left:5px;color:var(--accent-2);text-decoration:none}
  .side-item{min-height:34px;display:flex;align-items:center;gap:9px;padding:8px 10px;border-radius:var(--r-md);color:var(--muted);font-size:12px;text-align:left}
  .side-item i{width:7px;height:7px;border-radius:var(--r-round)}.side-item i.private{background:var(--accent)}.side-item i.published{background:#6fd39a}
  button.side-item{width:100%}button.side-item:hover,button.side-item.active{background:var(--raise);color:var(--ink)}
  button.side-item.active{box-shadow:inset 0 0 0 1px var(--accent-line);background:var(--accent-weak)}
  .badge{margin-left:auto;color:var(--faint);font-size:10px}
  .section-kind{display:flex;align-items:center;gap:8px;margin-bottom:8px;padding:9px 10px;border-radius:var(--r-md);background:var(--card)}
  .section-kind .glyph{color:var(--accent-2);font-size:16px}.section-kind span:last-child{display:flex;flex-direction:column}
  .section-kind b{font-size:12px}.section-kind small{color:var(--muted);font-size:10px}
  .archive{display:flex;align-items:center;gap:8px;margin-top:auto;padding:8px 10px;border-radius:var(--r-md);color:#df7474;font-size:11px}
  .archive:hover{background:color-mix(in srgb,#df7474 9%,var(--raise))}
  .unpublish{display:flex;align-items:center;gap:8px;padding:8px 10px;border-radius:var(--r-md);color:var(--muted);font-size:11px}
  .unpublish:hover{background:var(--raise);color:var(--ink)}
  .empty{padding:10px;color:var(--faint);font-size:11px}
</style>
