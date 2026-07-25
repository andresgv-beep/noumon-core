<script>
  import Icon from './Icon.svelte';
  import StudioImage from './StudioImage.svelte';
  import {
    STUDIO_INFO_RATIOS, STUDIO_INFO_SIDES, STUDIO_MAX_INFO_CARDS,
    studioInfoFocus, studioInfoRatioValue,
  } from './studioContent.js';
  import { t } from './i18n.svelte.js';

  let {
    documentId,
    cards = [],
    selectedCardID = '',
    uploading = false,
    onSelect,
    onAdd,
    onRemoveCard,
    onMoveCard,
    onChooseImage,
    onRemoveImage,
    onChange,
  } = $props();

  function setSide(card, side) {
    if (card.side === side) return;
    card.side = side;
    onChange?.();
  }

  function setRatio(card, value) {
    card.imageRatio = value;
    onChange?.();
  }

  function setFocus(card, axis, value) {
    card[axis] = Math.min(100, Math.max(0, Number(value) || 0));
    onChange?.();
  }

  function addRow(card) {
    if (card.rows.length >= 40) return;
    card.rows.push({ label: '', value: '' });
    onChange?.();
  }

  function removeRow(card, index) {
    card.rows.splice(index, 1);
    onChange?.();
  }

  function moveRow(card, index, delta) {
    const destination = index + delta;
    if (destination < 0 || destination >= card.rows.length) return;
    const [row] = card.rows.splice(index, 1);
    card.rows.splice(destination, 0, row);
    onChange?.();
  }
</script>

<section class="info-card-editor" aria-label={t('studio.infoCards')}>
  <header>
    <span>
      <b>{t('studio.infoCards')}</b>
      <small>{t('studio.infoCardsDesc')}</small>
    </span>
    <button type="button" onclick={() => onAdd?.()} disabled={cards.length >= STUDIO_MAX_INFO_CARDS}>
      <Icon name="plus" size={12} /> {t('studio.infoCardAdd')}
    </button>
  </header>

  {#if !cards.length}
    <p class="no-cards">{t('studio.infoCardsEmpty')}</p>
  {:else}
    <div class="card-tabs" aria-label={t('studio.infoCards')}>
      {#each cards as item, index (item.id)}
        <button
          type="button"
          class:active={item.id === selectedCardID || (!selectedCardID && index === 0)}
          onclick={() => onSelect?.(item.id, true)}
        >{index + 1}<small>{t(`studio.infoCardSide.${item.side || 'right'}`)}</small></button>
      {/each}
    </div>
    {@const card = cards.find((item) => item.id === selectedCardID) || cards[0]}
    {@const cardIndex = cards.indexOf(card)}
    {@const ratio = studioInfoRatioValue(card.imageRatio)}
    <article class="card">
      <div class="card-head">
        <b>{t('studio.infoCardNumber', { number: cardIndex + 1 })}</b>
        <div class="card-actions">
          <button type="button" onclick={() => onMoveCard?.(card.id, -1)} disabled={cardIndex === 0} title={t('studio.infoCardMoveCardUp')} aria-label={t('studio.infoCardMoveCardUp')}>↑</button>
          <button type="button" onclick={() => onMoveCard?.(card.id, 1)} disabled={cardIndex === cards.length - 1} title={t('studio.infoCardMoveCardDown')} aria-label={t('studio.infoCardMoveCardDown')}>↓</button>
          <button type="button" class="danger" onclick={() => onRemoveCard?.(card.id)} title={t('studio.infoCardRemoveCard')} aria-label={t('studio.infoCardRemoveCard')}>×</button>
        </div>
      </div>

      <div class="sides" role="group" aria-label={t('studio.infoCardSide')}>
        <span class="control-label">{t('studio.infoCardSide')}</span>
        <div>
          {#each STUDIO_INFO_SIDES as side}
            <button
              type="button"
              class:active={card.side === side}
              aria-pressed={card.side === side}
              onclick={() => setSide(card, side)}
            >{t(`studio.infoCardSide.${side}`)}</button>
          {/each}
        </div>
      </div>

      <button
        type="button"
        class="card-image"
        class:ready={!!card.assetId}
        onclick={() => onChooseImage?.(card.id)}
        disabled={uploading}
      >
        {#if card.assetId}
          <span class="frame" class:cropped={!!ratio} style:aspect-ratio={ratio || null} style:--info-focus={studioInfoFocus(card)}>
            <StudioImage {documentId} assetId={card.assetId} alt={t('documents.infoCardImage')} />
          </span>
        {:else}
          <Icon name="image" size={18} />
          <span>{uploading ? t('studio.uploadingImage') : t('studio.infoCardAddImage')}</span>
        {/if}
      </button>

      {#if card.assetId}
        <div class="image-actions">
          <button type="button" onclick={() => onChooseImage?.(card.id)} disabled={uploading}>{t('studio.infoCardReplaceImage')}</button>
          <button type="button" class="danger" onclick={() => onRemoveImage?.(card.id)}>{t('studio.infoCardRemoveImage')}</button>
        </div>

        <div class="image-frame" role="group" aria-label={t('studio.infoCardFraming')}>
          <span class="control-label">{t('studio.infoCardFraming')}</span>
          <div class="ratios">
            {#each STUDIO_INFO_RATIOS as option}
              <button
                type="button"
                class:active={(card.imageRatio || 'natural') === option}
                aria-pressed={(card.imageRatio || 'natural') === option}
                onclick={() => setRatio(card, option)}
              >{t(`studio.infoCardRatio.${option}`)}</button>
            {/each}
          </div>
          <!-- El centraje sólo tiene sentido cuando hay recorte: sin encuadre se
               ve la imagen entera y no hay nada que descartar. -->
          <div class="focus" class:disabled={!ratio}>
            <span class="control-label">{t('studio.infoCardFocus')}</span>
            <label>
              <small>{t('studio.infoCardFocusX')}</small>
              <input
                type="range" min="0" max="100" step="1" disabled={!ratio}
                value={card.imageFocusX ?? 50}
                oninput={(event) => setFocus(card, 'imageFocusX', event.currentTarget.value)}
              />
            </label>
            <label>
              <small>{t('studio.infoCardFocusY')}</small>
              <input
                type="range" min="0" max="100" step="1" disabled={!ratio}
                value={card.imageFocusY ?? 50}
                oninput={(event) => setFocus(card, 'imageFocusY', event.currentTarget.value)}
              />
            </label>
            <button
              type="button" class="reset" disabled={!ratio}
              onclick={() => { setFocus(card, 'imageFocusX', 50); setFocus(card, 'imageFocusY', 50); }}
            >{t('studio.infoCardFocusReset')}</button>
          </div>
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
        <button type="button" onclick={() => addRow(card)} disabled={card.rows.length >= 40}>
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
              <button type="button" onclick={() => moveRow(card, index, -1)} disabled={index === 0} title={t('studio.infoCardMoveUp')} aria-label={t('studio.infoCardMoveUp')}>↑</button>
              <button type="button" onclick={() => moveRow(card, index, 1)} disabled={index === card.rows.length - 1} title={t('studio.infoCardMoveDown')} aria-label={t('studio.infoCardMoveDown')}>↓</button>
              <button type="button" class="danger" onclick={() => removeRow(card, index)} title={t('studio.infoCardRemoveRow')} aria-label={t('studio.infoCardRemoveRow')}>×</button>
            </div>
          </div>
        {/each}
        {#if card.rows.length === 0}
          <p>{t('studio.infoCardEmpty')}</p>
        {/if}
      </div>
    </article>
  {/if}
</section>

<style>
  .info-card-editor{display:grid;gap:8px;padding-top:9px;border-top:1px solid var(--border)}
  header{display:flex;align-items:center;justify-content:space-between;gap:8px}
  header>span{display:flex;flex-direction:column;gap:2px}
  header b,.rows-heading b,.card-head b{color:var(--ink);font-size:10.5px}
  header small,.rows-heading small{color:var(--faint);font-size:8.5px;line-height:1.35}
  .no-cards{margin:0;padding:9px;border:1px dashed var(--border);border-radius:var(--r-sm);color:var(--faint);font-size:9px;text-align:center}
  .card-tabs{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:4px}
  .card-tabs button{min-width:0;display:flex;flex-direction:column;align-items:center;gap:1px;padding:6px 3px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:10px}
  .card-tabs button small{max-width:100%;overflow:hidden;color:var(--faint);font-size:7.5px;text-overflow:ellipsis;white-space:nowrap}
  .card-tabs button:hover,.card-tabs button.active{border-color:var(--accent-line);color:var(--ink)}
  .card-tabs button.active{background:var(--accent-weak)}
  .card{display:grid;gap:7px;padding:8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--panel-2,var(--card))}
  .card-head{display:flex;align-items:center;justify-content:space-between;gap:8px}
  .card-actions{display:flex;gap:2px}
  .card-actions button{width:22px;height:20px;display:grid;place-items:center;border:0;border-radius:3px;background:var(--card);color:var(--faint);font-size:10px}
  .card-actions button:hover:not(:disabled){color:var(--ink)}
  .sides{display:flex;align-items:center;justify-content:space-between;gap:8px}
  .sides>div{display:flex;gap:3px}
  .sides button{padding:4px 9px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:8.5px}
  .sides button:hover:not(:disabled),.ratios button:hover:not(:disabled){border-color:var(--accent-line);color:var(--ink)}
  .sides button.active,.ratios button.active{border-color:var(--accent-line);background:var(--accent-weak);color:var(--accent-2)}
  .card-image{width:100%;min-height:90px;display:grid;place-items:center;gap:6px;overflow:hidden;border:1px dashed var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9.5px}
  .card-image:hover{border-color:var(--accent-line);color:var(--ink)}.card-image.ready{border-style:solid}
  /* La previsualización usa el mismo encuadre y punto focal que la ficha
     publicada, para que lo que se ajusta aquí sea lo que se ve allí. */
  .frame{display:block;width:100%}
  .frame :global(img),.frame :global(.placeholder){display:block;width:100%;height:auto;max-height:220px;min-height:70px;border-radius:0;object-fit:contain;object-position:var(--info-focus,50% 50%)}
  .frame.cropped :global(img),.frame.cropped :global(.placeholder){height:100%;max-height:none;min-height:0;object-fit:cover}
  .image-actions{display:grid;grid-template-columns:1fr 1fr;gap:5px}
  .image-actions button,.rows-heading button,header>button{min-width:0;padding:6px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9px}
  .image-actions button:hover,.rows-heading button:hover,header>button:hover:not(:disabled){border-color:var(--accent-line);color:var(--ink)}
  header>button,.rows-heading button{display:flex;align-items:center;gap:4px}
  .image-frame{display:grid;gap:6px;padding:7px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--raise)}
  .control-label{color:var(--faint);font-size:8.5px;letter-spacing:.06em;text-transform:uppercase}
  .ratios{display:grid;grid-template-columns:repeat(5,1fr);gap:3px}
  .ratios button{min-width:0;padding:5px 2px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:8.5px}
  .focus{display:grid;gap:3px}
  .focus.disabled{opacity:.4}
  .focus label{display:grid;grid-template-columns:14px minmax(0,1fr);align-items:center;gap:6px}
  .focus small{color:var(--faint);font-size:8.5px}
  .focus input[type="range"]{width:100%;min-width:0;padding:0;accent-color:var(--accent)}
  .focus .reset{justify-self:end;padding:3px 7px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:8.5px}
  .focus .reset:hover:not(:disabled){border-color:var(--accent-line);color:var(--ink)}
  label{display:grid;gap:4px;color:var(--muted);font-size:9.5px}
  textarea,input{width:100%;min-width:0;box-sizing:border-box;padding:7px 8px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--ink);font:10.5px/1.4 var(--font);outline:0;resize:vertical}
  textarea:focus,input:focus{border-color:var(--accent-line);box-shadow:0 0 0 2px var(--accent-weak)}
  .rows-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}
  .rows-heading>span{display:flex;align-items:baseline;gap:5px}
  .rows{display:grid;gap:6px;max-height:280px;overflow:auto;padding-right:2px}
  .rows>p{margin:0;padding:9px;border:1px dashed var(--border);border-radius:var(--r-sm);color:var(--faint);font-size:9px;text-align:center}
  .row{display:grid;grid-template-columns:minmax(0,.7fr) minmax(0,1fr);gap:5px;padding:6px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--raise)}
  .row textarea{min-height:34px}.row-actions{grid-column:1/-1;display:flex;justify-content:flex-end;gap:2px}
  .row-actions button{width:22px;height:20px;display:grid;place-items:center;border:0;border-radius:3px;background:var(--card);color:var(--faint);font-size:10px}
  .row-actions button:hover:not(:disabled){color:var(--ink)}button.danger{color:#df7474}
  button:disabled{cursor:not-allowed;opacity:.35}
  :global(:root[data-skin="retro"]) :is(.card,.card-image,textarea,input,.row,.rows>p,.no-cards,.image-actions button,.rows-heading button,header>button){border-radius:0}
</style>
