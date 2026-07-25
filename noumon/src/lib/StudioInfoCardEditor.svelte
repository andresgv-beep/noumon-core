<script>
  import Icon from './Icon.svelte';
  import StudioImage from './StudioImage.svelte';
  import { t } from './i18n.svelte.js';

  let {
    documentId,
    card,
    uploading = false,
    onChooseImage,
    onRemoveImage,
    onChange,
  } = $props();

  function addRow() {
    if (!card || card.rows.length >= 40) return;
    card.rows.push({ label: '', value: '' });
    onChange?.();
  }

  function removeRow(index) {
    card?.rows.splice(index, 1);
    onChange?.();
  }

  function moveRow(index, delta) {
    const destination = index + delta;
    if (!card || destination < 0 || destination >= card.rows.length) return;
    const [row] = card.rows.splice(index, 1);
    card.rows.splice(destination, 0, row);
    onChange?.();
  }
</script>

<section class="info-card-editor" aria-label={t('studio.infoCard')}>
  <header>
    <span>
      <b>{t('studio.infoCard')}</b>
      <small>{t('studio.infoCardDesc')}</small>
    </span>
  </header>

  <button
    type="button"
    class="card-image"
    class:ready={!!card.assetId}
    onclick={() => onChooseImage?.()}
    disabled={uploading}
  >
    {#if card.assetId}
      <StudioImage {documentId} assetId={card.assetId} alt={t('documents.infoCardImage')} compact />
    {:else}
      <Icon name="image" size={18} />
      <span>{uploading ? t('studio.uploadingImage') : t('studio.infoCardAddImage')}</span>
    {/if}
  </button>
  {#if card.assetId}
    <div class="image-actions">
      <button type="button" onclick={() => onChooseImage?.()} disabled={uploading}>{t('studio.infoCardReplaceImage')}</button>
      <button type="button" class="danger" onclick={() => onRemoveImage?.()}>{t('studio.infoCardRemoveImage')}</button>
    </div>
  {/if}

  <label>
    <span>{t('studio.infoCardCaption')}</span>
    <textarea
      rows="2"
      maxlength="1000"
      value={card.caption}
      placeholder={t('studio.infoCardCaptionPlaceholder')}
      oninput={(event) => { card.caption = event.currentTarget.value; onChange?.(); }}
    ></textarea>
  </label>

  <div class="rows-heading">
    <span>
      <b>{t('studio.infoCardRows')}</b>
      <small>{card.rows.length}/40</small>
    </span>
    <button type="button" onclick={addRow} disabled={card.rows.length >= 40}>
      <Icon name="plus" size={12} /> {t('studio.infoCardAddRow')}
    </button>
  </div>

  <div class="rows">
    {#each card.rows as row, index}
      <div class="row">
        <input
          maxlength="120"
          value={row.label}
          placeholder={t('studio.infoCardLabel')}
          aria-label={t('studio.infoCardLabel')}
          oninput={(event) => { row.label = event.currentTarget.value; onChange?.(); }}
        />
        <textarea
          rows="2"
          maxlength="4000"
          value={row.value}
          placeholder={t('studio.infoCardValue')}
          aria-label={t('studio.infoCardValue')}
          oninput={(event) => { row.value = event.currentTarget.value; onChange?.(); }}
        ></textarea>
        <div class="row-actions">
          <button type="button" onclick={() => moveRow(index, -1)} disabled={index === 0} title={t('studio.infoCardMoveUp')} aria-label={t('studio.infoCardMoveUp')}>↑</button>
          <button type="button" onclick={() => moveRow(index, 1)} disabled={index === card.rows.length - 1} title={t('studio.infoCardMoveDown')} aria-label={t('studio.infoCardMoveDown')}>↓</button>
          <button type="button" class="danger" onclick={() => removeRow(index)} title={t('studio.infoCardRemoveRow')} aria-label={t('studio.infoCardRemoveRow')}>×</button>
        </div>
      </div>
    {/each}
    {#if card.rows.length === 0}
      <p>{t('studio.infoCardEmpty')}</p>
    {/if}
  </div>
</section>

<style>
  .info-card-editor{display:grid;gap:8px;padding-top:9px;border-top:1px solid var(--border)}
  header>span{display:flex;flex-direction:column;gap:2px}
  header b,.rows-heading b{color:var(--ink);font-size:10.5px}
  header small,.rows-heading small{color:var(--faint);font-size:8.5px;line-height:1.35}
  .card-image{width:100%;min-height:90px;display:grid;place-items:center;gap:6px;overflow:hidden;border:1px dashed var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9.5px}
  .card-image:hover{border-color:var(--accent-line);color:var(--ink)}.card-image.ready{border-style:solid}
  .card-image :global(img),.card-image :global(.placeholder){width:100%;height:110px;min-height:0;border-radius:0;object-fit:cover}
  .image-actions{display:grid;grid-template-columns:1fr 1fr;gap:5px}
  .image-actions button,.rows-heading button{min-width:0;padding:6px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9px}
  .image-actions button:hover,.rows-heading button:hover{border-color:var(--accent-line);color:var(--ink)}
  label{display:grid;gap:4px;color:var(--muted);font-size:9.5px}
  textarea,input{width:100%;min-width:0;box-sizing:border-box;padding:7px 8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--ink);font:10.5px/1.4 var(--font);outline:0;resize:vertical}
  textarea:focus,input:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .rows-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}
  .rows-heading>span{display:flex;align-items:baseline;gap:5px}
  .rows-heading button{display:flex;align-items:center;gap:4px}
  .rows{display:grid;gap:6px;max-height:280px;overflow:auto;padding-right:2px}
  .rows>p{margin:0;padding:9px;border:1px dashed var(--border);border-radius:var(--r-sm);color:var(--faint);font-size:9px;text-align:center}
  .row{display:grid;grid-template-columns:minmax(0,.7fr) minmax(0,1fr);gap:5px;padding:6px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--raise)}
  .row textarea{min-height:34px}.row-actions{grid-column:1/-1;display:flex;justify-content:flex-end;gap:2px}
  .row-actions button{width:22px;height:20px;display:grid;place-items:center;border:0;border-radius:3px;background:var(--card);color:var(--faint);font-size:10px}
  .row-actions button:hover:not(:disabled){color:var(--ink)}button.danger{color:#df7474}
  button:disabled{cursor:not-allowed;opacity:.35}
  :global(:root[data-skin="retro"]) :is(.card-image,textarea,input,.row,.rows>p,.image-actions button,.rows-heading button){border-radius:0}
</style>
