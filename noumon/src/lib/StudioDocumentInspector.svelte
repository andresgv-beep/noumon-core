<script>
  import Icon from './Icon.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioInfoCardEditor from './StudioInfoCardEditor.svelte';
  import StudioPageManager from './StudioPageManager.svelte';
  import { t } from './i18n.svelte.js';

  let { state = {} } = $props();

  const document = () => state.selected;
  const presentation = () => document()?.content?.presentation || {};
  const classification = () => document()?.content?.classification || {};
  const tagsText = () => (document()?.tags || []).join(', ');

  function setPresentation(field, value) {
    if (!document()) return;
    document().content.presentation ||= {};
    document().content.presentation[field] = value;
    state.changeDocument?.();
  }

  function setClassification(field, value) {
    if (!document()) return;
    document().content.classification ||= {};
    document().content.classification[field] = value;
    state.changeDocument?.();
  }
</script>

{#if document()}
  <div class="inspector">
    {#if state.brokenPageLinks?.length}
      <div class="broken-links">
        <b>{t('studio.pageLinkBrokenTitle')}</b>
        <small>{t('studio.pageLinkBrokenHint')}</small>
        {#each state.brokenPageLinks as link (link.id)}
          <span>
            <code>{link.id}</code>
            <small>{link.count}</small>
            <button
              title={t('studio.pageLinkRemoveHint')}
              onclick={() => state.removeBrokenPageLink?.(link.id)}
            >{t('studio.pageLinkRemove')}</button>
          </span>
        {/each}
      </div>
    {/if}

    {#if state.activeSection === 'pages'}
      <StudioPageManager
        document={document()}
        activePageID={state.activePageID}
        onSelect={state.selectPage}
        onCreate={state.addPage}
        onRename={state.renamePage}
        onMove={state.reorderPage}
        onRemove={state.deletePage}
      />
    {:else if state.activeSection === 'insert'}
      <section class="panel">
        <h3>{t('studio.pageLinks')}</h3>
        <button
          class="link-tool"
          class:active={!!state.pageLinkSelection}
          onclick={() => state.capturePageLinkSelection?.()}
        >
          <b>↗</b>{t('studio.pageLink')}
        </button>
        {#if state.pageLinkSelection}
          <div class="page-link-picker">
            <small>{t('studio.pageLinkSelection', { text: state.pageLinkSelection.label })}</small>
            <div>
              {#each state.pages || [] as page (page.id)}
                <button onclick={() => state.applyPageLink?.(page.id)}>
                  <b>{page.title}</b>
                  <small>{page.id}</small>
                </button>
              {/each}
            </div>
            <button class="cancel" onclick={() => state.cancelPageLink?.()}>{t('common.cancel')}</button>
          </div>
        {:else if state.pageLinkMessage}
          <small class="link-message">{t(state.pageLinkMessage)}</small>
        {:else}
          <small class="link-hint">{t('studio.pageLinkHint')}</small>
        {/if}
        <h3>{t('studio.insertBlock')}</h3>
        <div class="block-grid">
          <button onclick={() => state.addBlock?.('paragraph')}><b>¶</b>{t('studio.block.paragraph')}</button>
          <button onclick={() => state.addBlock?.('heading')}><b>H</b>{t('studio.block.heading')}</button>
          <button onclick={() => state.chooseImage?.()} disabled={state.uploadingImage}><b>▧</b>{t('studio.block.image')}</button>
          <button onclick={() => state.addBlock?.('columns', { columnCount: 1 })}><b>▯</b>{t('studio.block.oneColumn')}</button>
          <button onclick={() => state.addBlock?.('columns', { columnCount: 2 })}><b>▥</b>{t('studio.block.twoColumns')}</button>
          <button onclick={() => state.addBlock?.('columns', { columnCount: 3 })}><b>▥</b>{t('studio.block.threeColumns')}</button>
          <button onclick={() => state.addBlock?.('table')}><b>⊞</b>{t('studio.block.table')}</button>
          <button onclick={() => state.addBlock?.('quote')}><b>❝</b>{t('studio.block.quote')}</button>
          <button onclick={() => state.addBlock?.('callout')}><b>!</b>{t('studio.block.callout')}</button>
          <button onclick={() => state.addBlock?.('code')}><b>&lt;/&gt;</b>{t('studio.block.code')}</button>
          <button onclick={() => state.addBlock?.('bulletList')}><b>≔</b>{t('studio.block.bulletList')}</button>
          <button onclick={() => state.addBlock?.('divider')}><b>—</b>{t('studio.block.divider')}</button>
        </div>

        <h3>{t('studio.internalLink')}</h3>
        <button class="link-tool" class:active={state.linkPicker} onclick={() => state.toggleLinkPicker?.()}>
          <Icon name="book" size={14} />{t('studio.internalLink')}
        </button>
        {#if state.linkPicker}
          <div class="link-picker">
            <input
              value={state.linkQuery || ''}
              placeholder={t('studio.internalLinkSearch')}
              aria-label={t('studio.internalLinkSearch')}
              oninput={(event) => state.searchLinkTargets?.(event.currentTarget.value)}
            />
            {#if state.linkLoading}
              <small>{t('common.loading')}</small>
            {:else if state.linkResults?.length}
              <div class="link-results">
                {#each state.linkResults as item (item.itemId)}
                  <button onclick={() => state.insertItemReference?.(item)}>
                    <b>{item.title}</b><small>{item.kind}</small>
                  </button>
                {/each}
              </div>
            {:else}
              <small>{t('studio.internalLinkHint')}</small>
            {/if}
          </div>
        {/if}
      </section>
    {:else if state.activeSection === 'design'}
      <section class="panel">
        <h3>{t('studio.pageDesign')}</h3>
        <div class="style-options">
          {#each [
            ['reading', t('studio.widthReading'), '760 px'],
            ['wide', t('studio.widthWide'), '980 px'],
            ['editorial', t('studio.widthEditorial'), '1180 px'],
            ['compact', t('studio.widthCompact'), '620 px'],
          ] as option}
            <button
              class:active={presentation().contentWidth === option[0]}
              onclick={() => setPresentation('contentWidth', option[0])}
            ><span>{option[1]}</span><small>{option[2]}</small></button>
          {/each}
        </div>

        <h3>{t('studio.navFrame')}</h3>
        <div class="style-options">
          {#each [
            ['none', t('studio.frameNone')],
            ['framed', t('studio.frameSquare')],
            ['rounded', t('studio.frameRounded')],
          ] as option}
            <button
              class:active={(presentation().navFrame || 'none') === option[0]}
              onclick={() => setPresentation('navFrame', option[0])}
            ><span>{option[1]}</span></button>
          {/each}
        </div>

        <h3>{t('studio.typography')}</h3>
        <div class="style-options">
          <button class:active={presentation().fontPreset !== 'sans'} onclick={() => setPresentation('fontPreset', 'editorial')}>
            <span>{t('studio.fontEditorial')}</span><small>Serif</small>
          </button>
          <button class:active={presentation().fontPreset === 'sans'} onclick={() => setPresentation('fontPreset', 'sans')}>
            <span>{t('studio.fontSans')}</span><small>Sans</small>
          </button>
        </div>
      </section>
    {:else if state.activeSection === 'metadata'}
      <section class="panel">
        <h3>{t('studio.metadata')}</h3>
        <div class="metadata-grid">
          <label>{t('studio.author')}<input value={document().authorLabel || ''} oninput={(event) => { document().authorLabel = event.currentTarget.value; state.changeDocument?.(); }} /></label>
          <label>{t('studio.language')}<input value={document().language || ''} placeholder="es" oninput={(event) => { document().language = event.currentTarget.value; state.changeDocument?.(); }} /></label>
          <label>{t('studio.tags')}<input value={tagsText()} placeholder={t('studio.tagsPlaceholder')} oninput={(event) => state.setTags?.(event.currentTarget.value)} /></label>
          <label>{t('studio.workType')}<input value={classification().workType || ''} placeholder="article" oninput={(event) => setClassification('workType', event.currentTarget.value)} /></label>
        </div>
      </section>
    {:else if state.activeSection === 'cover'}
      <section class="panel">
        <h3>{t('studio.section.cover')}</h3>
        <button class="document-cover" class:ready={!!state.documentCover} onclick={() => state.chooseDocumentCover?.()} disabled={state.uploadingImage}>
          {#if state.documentCover}
            <StudioImage documentId={document().id} assetId={state.documentCover.assetId} alt={t('studio.section.cover')} compact />
          {:else}
            <b>＋</b><span>{state.uploadingImage ? t('studio.uploadingImage') : t('studio.addCover')}</span>
          {/if}
        </button>
        {#if state.documentCover}
          <div class="cover-actions">
            <button onclick={() => state.chooseDocumentCover?.()} disabled={state.uploadingImage}>{t('studio.replaceCover')}</button>
            <button class="remove" onclick={() => state.removeDocumentCover?.()}>{t('studio.removeCover')}</button>
          </div>
        {/if}
      </section>
    {:else if state.activeSection === 'cards'}
      <StudioInfoCardEditor
        documentId={document().id}
        cards={state.infoCards || []}
        selectedCardID={state.selectedInfoCardID || ''}
        uploading={state.uploadingImage}
        onSelect={state.selectInfoCard}
        onAdd={state.addInfoCard}
        onRemoveCard={state.removeInfoCard}
        onMoveCard={state.moveInfoCard}
        onChooseImage={state.chooseInfoCardImage}
        onRemoveImage={state.removeInfoCardImage}
        onChange={state.changeDocument}
      />
    {/if}
  </div>
{/if}

<style>
  .inspector{min-height:0}
  .panel{display:grid;gap:9px}
  h3{margin:5px 0 1px;color:var(--faint);font-size:9px;font-weight:650;letter-spacing:.14em;text-transform:uppercase}
  h3:first-child{margin-top:0}
  .block-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:5px}
  .block-grid button{min-height:34px;display:flex;align-items:center;gap:7px;padding:7px 8px;border:0;border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px;text-align:left}
  .block-grid button:hover{background:var(--raise);color:var(--ink)}
  .block-grid button b{color:var(--accent-2);font-size:12px}
  .style-options{display:grid;gap:5px}
  .style-options button{width:100%;display:flex;align-items:center;justify-content:space-between;gap:6px;min-height:34px;padding:7px 9px;border:0;border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px}
  .style-options button:hover{background:var(--raise);color:var(--ink)}
  .style-options button.active{background:var(--accent-weak);color:var(--ink);box-shadow:inset 0 0 0 1px var(--accent-line)}
  .style-options small{color:var(--faint);font-size:9px}
  .metadata-grid{display:grid;gap:8px}
  .metadata-grid label{display:flex;flex-direction:column;gap:4px;color:var(--muted);font-size:9.5px;letter-spacing:.03em}
  .metadata-grid input,.link-picker input{width:100%;min-width:0;padding:8px 9px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--ink);font-size:10.5px;outline:0}
  .metadata-grid input:focus,.link-picker input:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .link-tool{width:100%;display:flex;align-items:center;gap:7px;padding:8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10.5px}
  .link-tool>b{color:var(--accent-2);font-size:12px}
  .link-tool.active{border-color:var(--accent-line);color:var(--accent-2)}
  .link-hint,.link-message{display:block;color:var(--faint);font-size:9.5px;line-height:1.45}
  .link-message{color:#df7474}
  .page-link-picker{display:grid;gap:7px;padding:8px;border:1px solid var(--accent-line);border-radius:var(--r-sm);background:var(--card)}
  .page-link-picker>small{overflow:hidden;color:var(--muted);font-size:9.5px;line-height:1.4;text-overflow:ellipsis;white-space:nowrap}
  .page-link-picker>div{display:grid;gap:4px;max-height:210px;overflow:auto}
  .page-link-picker>div button{display:flex;align-items:center;justify-content:space-between;gap:6px;padding:7px;border-radius:var(--r-sm);background:var(--raise);color:var(--ink);text-align:left}
  .page-link-picker>div button:hover{box-shadow:inset 0 0 0 1px var(--accent-line)}
  .page-link-picker>div b{overflow:hidden;font-size:10px;text-overflow:ellipsis;white-space:nowrap}
  .page-link-picker>div small{color:var(--faint);font:8px var(--mono)}
  .page-link-picker .cancel{justify-self:end;color:var(--muted);font-size:9px}
  .broken-links{display:grid;gap:5px;padding:8px;border-left:2px solid #df7474;background:color-mix(in srgb,#df7474 8%,transparent)}
  .broken-links>b{color:#df7474;font-size:9.5px}.broken-links>small{color:var(--muted);font-size:8.5px;line-height:1.4}
  .broken-links>span{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:4px 6px;color:var(--ink);font-size:9px}
  .broken-links code{overflow:hidden;text-overflow:ellipsis}.broken-links span small{color:var(--faint)}
  .broken-links span button{grid-column:1/-1;padding:6px;border:1px solid color-mix(in srgb,#df7474 36%,var(--border));border-radius:var(--r-sm);background:var(--card);color:#df9a9a;font-size:8.5px}
  .broken-links span button:hover{border-color:#df7474;color:#f0b0b0}
  .link-picker{display:grid;gap:7px;padding:8px;border:1px solid var(--accent-line);border-radius:var(--r-sm);background:var(--card)}
  .link-picker>small{color:var(--faint);font-size:9.5px}
  .link-results{display:grid;gap:4px;max-height:220px;overflow:auto}
  .link-results button{display:flex;flex-direction:column;align-items:flex-start;padding:7px;border-radius:var(--r-sm);background:var(--raise);color:var(--ink);text-align:left}
  .link-results b{font-size:10px}.link-results small{color:var(--faint);font-size:8.5px}
  .document-cover{width:100%;min-height:116px;display:grid;place-items:center;gap:8px;overflow:hidden;border:1px dashed var(--border);border-radius:var(--r-md);background:var(--card);color:var(--muted)}
  .document-cover:hover{border-color:var(--accent-line);color:var(--ink)}
  .document-cover.ready{border-style:solid}.document-cover b{font-size:24px;color:var(--accent-2)}.document-cover span{font-size:11px}
  .document-cover :global(img),.document-cover :global(.placeholder){width:100%;height:150px;object-fit:cover;border-radius:0}
  .cover-actions{display:grid;grid-template-columns:1fr 1fr;gap:6px}
  .cover-actions button{min-width:0;padding:7px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9.5px}
  .cover-actions button:hover{border-color:var(--accent-line);color:var(--ink)}.cover-actions .remove{color:#df7474}
</style>
