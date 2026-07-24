<script>
  import StudioCanvasBlock from './StudioCanvasBlock.svelte';
  import StudioImage from './StudioImage.svelte';
  import StudioItemReference from './StudioItemReference.svelte';
  import { t } from './i18n.svelte.js';

  let {
    block,
    documentId,
    selected = false,
    activeBlockID = '',
    nested = false,
    onSelect,
    onChange,
    onDuplicate,
    onRemove,
    onDragStart,
    onDragEnd,
    onDrop,
    onDropIntoColumn,
    onAddToColumn,
    onChooseImage,
    onMoveToRoot,
    onOpenItem,
  } = $props();

  function escapeHTML(value) {
    return String(value || '').replace(/[&<>"']/g, (character) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    })[character]);
  }

  function inline(value) {
    return escapeHTML(value)
      .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
  }

  function inlineText(node) {
    if (node.nodeType === Node.TEXT_NODE) return node.nodeValue || '';
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const content = [...node.childNodes].map(inlineText).join('');
    if (node.tagName === 'STRONG' || node.tagName === 'B') return `**${content}**`;
    if (node.tagName === 'EM' || node.tagName === 'I') return `*${content}*`;
    if (node.tagName === 'BR') return '\n';
    return content;
  }

  function setText(event, field = 'text') {
    block[field] = inlineText(event.currentTarget);
    onChange?.();
  }

  function setListItem(event, index) {
    block.items[index] = event.currentTarget.innerText;
    onChange?.();
  }

  function setTableCell(event, row, column) {
    block.rows[row][column] = event.currentTarget.innerText;
    onChange?.();
  }

  function select(event) {
    event?.stopPropagation();
    onSelect?.(block.id);
  }

  function setColumnLayout(layout, event) {
    event.stopPropagation();
    block.layout = layout;
    onChange?.();
  }

  function addColumnBlock(columnIndex, type, event) {
    event.stopPropagation();
    onAddToColumn?.(block.id, columnIndex, type);
  }

  function chooseColumnImage(columnIndex, event) {
    event.stopPropagation();
    onChooseImage?.(block.id, columnIndex);
  }

  function setColumnCount(count, event) {
    event.stopPropagation();
    const columns = [...(block.columns || [])];
    while (columns.length < count) columns.push([]);
    if (columns.length > count) {
      const overflow = columns.slice(count).flat();
      columns.length = count;
      columns[count - 1].push(...overflow);
    }
    block.columns = columns;
    if (count === 1) block.layout = 'full';
    else if (['full', 'half-left', 'half-right'].includes(block.layout)) block.layout = 'equal';
    onChange?.();
  }

  function tableColumnCount() {
    return Math.max(1, ...(block.rows || []).map((row) => row.length));
  }

  function addTableRow(event) {
    event.stopPropagation();
    block.rows ||= [];
    block.rows.push(Array(tableColumnCount()).fill(''));
    onChange?.();
  }

  function removeTableRow(event) {
    event.stopPropagation();
    if ((block.rows || []).length <= 1) return;
    block.rows.pop();
    onChange?.();
  }

  function addTableColumn(event) {
    event.stopPropagation();
    block.rows ||= [[], []];
    for (const row of block.rows) row.push('');
    onChange?.();
  }

  function removeTableColumn(event) {
    event.stopPropagation();
    if (tableColumnCount() <= 1) return;
    for (const row of block.rows || []) row.pop();
    onChange?.();
  }
</script>

<!-- The block surface selects itself while its real controls stay independently interactive. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="canvas-block"
  class:selected={selected || activeBlockID === block.id}
  class:nested
  role="group"
  aria-label={t(`studio.block.${block.type}`)}
  onclick={select}
  onfocusin={select}
  ondragover={(event) => event.preventDefault()}
  ondrop={(event) => {
    event.preventDefault();
    event.stopPropagation();
    onDrop?.(block.id);
  }}
>
  <button
    class="handle"
    draggable="true"
    title={t('studio.dragBlock')}
    aria-label={t('studio.dragBlock')}
    ondragstart={(event) => onDragStart?.(block.id, event)}
    ondragend={() => onDragEnd?.()}
  >⠿</button>
  <div class="actions">
    {#if nested}
      <button title={t('studio.moveToBody')} aria-label={t('studio.moveToBody')} onclick={(event) => { event.stopPropagation(); onMoveToRoot?.(block.id); }}>↗</button>
    {/if}
    <button title={t('studio.duplicateBlock')} aria-label={t('studio.duplicateBlock')} onclick={(event) => { event.stopPropagation(); onDuplicate?.(block.id); }}>⧉</button>
    <button title={t('studio.removeBlock')} aria-label={t('studio.removeBlock')} onclick={(event) => { event.stopPropagation(); onRemove?.(block.id); }}>×</button>
  </div>

  {#if block.type === 'heading'}
    {@const level = Math.min(3, Math.max(1, block.level || 2))}
    {#if level === 1}
      <h1 contenteditable="true" oninput={setText}>{@html inline(block.text)}</h1>
    {:else if level === 2}
      <h2 contenteditable="true" oninput={setText}>{@html inline(block.text)}</h2>
    {:else}
      <h3 contenteditable="true" oninput={setText}>{@html inline(block.text)}</h3>
    {/if}
  {:else if block.type === 'paragraph'}
    <p contenteditable="true" oninput={setText}>{@html inline(block.text)}</p>
  {:else if block.type === 'quote'}
    <blockquote contenteditable="true" oninput={setText}>{@html inline(block.text)}</blockquote>
  {:else if block.type === 'bulletList' || block.type === 'orderedList'}
    {#if block.type === 'bulletList'}
      <ul>{#each block.items || [] as item, itemIndex}<li contenteditable="true" oninput={(event) => setListItem(event, itemIndex)}>{item}</li>{/each}</ul>
    {:else}
      <ol>{#each block.items || [] as item, itemIndex}<li contenteditable="true" oninput={(event) => setListItem(event, itemIndex)}>{item}</li>{/each}</ol>
    {/if}
  {:else if block.type === 'table'}
    <div class="table-editor">
      <div class="table-controls" aria-label={t('studio.tableControls')}>
        <span>{t('studio.tableSize', { rows: (block.rows || []).length, columns: tableColumnCount() })}</span>
        <div>
          <button onclick={addTableRow}>＋ {t('studio.tableRow')}</button>
          <button disabled={(block.rows || []).length <= 1} onclick={removeTableRow}>− {t('studio.tableRow')}</button>
          <button onclick={addTableColumn}>＋ {t('studio.tableColumn')}</button>
          <button disabled={tableColumnCount() <= 1} onclick={removeTableColumn}>− {t('studio.tableColumn')}</button>
        </div>
      </div>
      <div class="table-scroll">
        <table>
          <tbody>
            {#each block.rows || [] as row, rowIndex}
              <tr>
                {#each row as cell, columnIndex}
                  {#if rowIndex === 0}
                    <th contenteditable="true" oninput={(event) => setTableCell(event, rowIndex, columnIndex)}>{cell}</th>
                  {:else}
                    <td contenteditable="true" oninput={(event) => setTableCell(event, rowIndex, columnIndex)}>{cell}</td>
                  {/if}
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else if block.type === 'image'}
    <figure>
      <StudioImage {documentId} assetId={block.assetId} alt={block.alt || ''} />
      <figcaption contenteditable="true" oninput={(event) => setText(event, 'caption')}>{block.caption || t('studio.imageCaption')}</figcaption>
    </figure>
  {:else if block.type === 'code'}
    <pre contenteditable="true" oninput={setText}>{block.text || ''}</pre>
  {:else if block.type === 'callout'}
    <aside class="callout">
      <strong contenteditable="true" oninput={(event) => setText(event, 'title')}>{@html inline(block.title || t('studio.calloutTitle'))}</strong>
      <p contenteditable="true" oninput={setText}>{@html inline(block.text)}</p>
    </aside>
  {:else if block.type === 'columns'}
    {#if !nested}
      <div class="column-layout-tools" aria-label={t('studio.columns.layout')}>
        <span>{t('studio.columns.layout')}</span>
        <div class="column-count">
          <button
            class:active={(block.columns || []).length === 1}
            onclick={(event) => setColumnCount(1, event)}
          >{t('studio.block.oneColumn')}</button>
          <button
            class:active={(block.columns || []).length === 2}
            onclick={(event) => setColumnCount(2, event)}
          >{t('studio.block.twoColumns')}</button>
          <button
            class:active={(block.columns || []).length === 3}
            onclick={(event) => setColumnCount(3, event)}
          >{t('studio.block.threeColumns')}</button>
        </div>
        {#if (block.columns || []).length === 2}
          <div class="column-ratios">
        <button
          class:active={!block.layout || block.layout === 'equal'}
          title={t('studio.columns.equal')}
          onclick={(event) => setColumnLayout('equal', event)}
        >1:1</button>
        <button
          class:active={block.layout === 'lead-left'}
          title={t('studio.columns.leadLeft')}
          onclick={(event) => setColumnLayout('lead-left', event)}
        >2:1</button>
        <button
          class:active={block.layout === 'lead-right'}
          title={t('studio.columns.leadRight')}
          onclick={(event) => setColumnLayout('lead-right', event)}
        >1:2</button>
          </div>
        {:else if (block.columns || []).length === 1}
          <div class="column-ratios">
            <button
              class:active={!block.layout || block.layout === 'full'}
              onclick={(event) => setColumnLayout('full', event)}
            >{t('studio.columns.full')}</button>
            <button
              class:active={block.layout === 'half-left'}
              onclick={(event) => setColumnLayout('half-left', event)}
            >{t('studio.columns.halfLeft')}</button>
            <button
              class:active={block.layout === 'half-right'}
              onclick={(event) => setColumnLayout('half-right', event)}
            >{t('studio.columns.halfRight')}</button>
          </div>
        {/if}
      </div>
    {/if}
    <div
      class="columns"
      class:single={(block.columns || []).length === 1}
      class:three={(block.columns || []).length === 3}
      class:lead-left={block.layout === 'lead-left'}
      class:lead-right={block.layout === 'lead-right'}
      class:half-left={block.layout === 'half-left'}
      class:half-right={block.layout === 'half-right'}
    >
      {#each block.columns || [] as column, columnIndex}
        <div
          class="column"
          class:empty={column.length === 0}
          role="group"
          aria-label={t('studio.columnNumber', { number: columnIndex + 1 })}
          ondragover={(event) => event.preventDefault()}
          ondrop={(event) => {
            event.preventDefault();
            event.stopPropagation();
            onDropIntoColumn?.(block.id, columnIndex);
          }}
        >
          <div class="column-heading">
            <b>{t('studio.columnNumber', { number: columnIndex + 1 })}</b>
            <small>{t('studio.columnDrop')}</small>
          </div>
          {#each column as child (child.id)}
            <StudioCanvasBlock
              block={child}
              {documentId}
              nested
              {activeBlockID}
              selected={activeBlockID === child.id}
              {onSelect}
              {onChange}
              {onDuplicate}
              {onRemove}
              {onDragStart}
              {onDragEnd}
              {onDrop}
              {onDropIntoColumn}
              {onAddToColumn}
              {onChooseImage}
              {onMoveToRoot}
              {onOpenItem}
            />
          {/each}
          <div class="column-add" aria-label={t('studio.addToColumn')}>
            <span>＋ {t('studio.addToColumn')}</span>
            <div>
              <button title={t('studio.block.paragraph')} onclick={(event) => addColumnBlock(columnIndex, 'paragraph', event)}><b>¶</b>{t('studio.block.paragraph')}</button>
              <button title={t('studio.block.heading')} onclick={(event) => addColumnBlock(columnIndex, 'heading', event)}><b>H</b>{t('studio.block.heading')}</button>
              <button title={t('studio.block.image')} onclick={(event) => chooseColumnImage(columnIndex, event)}><b>▧</b>{t('studio.block.image')}</button>
              <button title={t('studio.block.callout')} onclick={(event) => addColumnBlock(columnIndex, 'callout', event)}><b>!</b>{t('studio.block.callout')}</button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {:else if block.type === 'itemRef'}
    <StudioItemReference
      itemId={block.itemId}
      titleSnapshot={block.titleSnapshot}
      kindSnapshot={block.kindSnapshot}
      {onOpenItem}
    />
  {:else if block.type === 'divider'}
    <hr />
  {/if}
</div>

<style>
  .canvas-block{position:relative;margin:2px -9px;padding:7px 9px;border:1px solid transparent;border-radius:var(--r-sm);transition:border-color .12s,background .12s}
  .canvas-block:not(.nested):hover,.canvas-block.selected{border-color:var(--accent-line);background:color-mix(in srgb,var(--accent) 5%,transparent)}
  .canvas-block.nested{margin:2px 0;padding:7px 9px}
  [contenteditable="true"]{outline:0}
  [contenteditable="true"]:empty::before{content:attr(data-placeholder);color:var(--faint)}
  .handle{position:absolute;left:-25px;top:9px;width:22px;height:24px;color:var(--faint);font-size:12px;opacity:0;cursor:grab}
  .nested>.handle{left:-17px;width:18px}
  .actions{position:absolute;z-index:2;right:5px;top:-24px;display:flex;gap:2px;padding:3px;border-radius:var(--r-sm);background:var(--raise);border:1px solid var(--border);opacity:0}
  .actions button{padding:1px 5px;color:var(--muted);font-size:11px}.actions button:hover{color:var(--ink)}
  .canvas-block:hover>.handle,.canvas-block:hover>.actions,.canvas-block.selected>.handle,.canvas-block.selected>.actions{opacity:1}
  h1{font-size:clamp(30px,5vw,42px);line-height:1.1;letter-spacing:-.03em}
  h2{font-size:25px;line-height:1.2;letter-spacing:-.02em}
  h3{font-size:18px;line-height:1.3}
  p,li{color:var(--ink-dim);font-size:14.5px;line-height:1.68;white-space:pre-wrap}
  ul,ol{padding-left:22px}
  blockquote{padding:13px 18px;border-left:3px solid var(--accent);color:var(--muted);font-size:17px;font-style:italic}
  .callout{padding:14px 16px;border-left:3px solid var(--accent);background:var(--raise);color:var(--muted);font-size:13px;line-height:1.55}
  .callout strong{display:block;color:var(--ink);margin-bottom:3px}.callout p{margin:0;font-size:13px}
  pre{padding:13px 15px;overflow:auto;background:var(--ground);color:var(--link);font:12px/1.6 var(--mono);white-space:pre-wrap}
  figure{margin:0}figcaption{margin-top:7px;text-align:center;color:var(--muted);font:11px var(--font)}
  .column-layout-tools{display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin:0 0 12px;padding:8px 10px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--raise);font-family:var(--font)}
  .column-layout-tools>span{margin-right:auto;color:var(--ink);font-size:10px;font-weight:700;letter-spacing:.05em;text-transform:uppercase}
  .column-count,.column-ratios{display:flex;gap:3px}
  .column-ratios{padding-left:6px;border-left:1px solid var(--border)}
  .column-layout-tools button{min-height:27px;padding:4px 9px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font:10px var(--font)}
  .column-layout-tools button:hover,.column-layout-tools button.active{border-color:var(--accent-line);background:var(--accent-weak);color:var(--ink)}
  .columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:clamp(15px,2.4vw,28px)}
  .columns.single{grid-template-columns:minmax(0,1fr)}
  .columns.single.half-left,.columns.single.half-right{grid-template-columns:minmax(0,50%)}
  .columns.single.half-left{justify-content:start}.columns.single.half-right{justify-content:end}
  .columns.three{grid-template-columns:repeat(3,minmax(0,1fr))}
  .columns.lead-left{grid-template-columns:minmax(0,2fr) minmax(0,1fr)}
  .columns.lead-right{grid-template-columns:minmax(0,1fr) minmax(0,2fr)}
  .column{min-width:0;min-height:150px;padding:10px;border:1px solid color-mix(in srgb,var(--accent) 32%,var(--border));border-radius:var(--r-md);background:color-mix(in srgb,var(--raise) 55%,transparent);transition:border-color .12s,background .12s}
  .column:hover,.column:focus-within{border-color:var(--accent-line);background:color-mix(in srgb,var(--accent) 6%,var(--raise))}
  .column-heading{display:flex;align-items:center;justify-content:space-between;gap:8px;margin:-2px 0 8px;padding-bottom:7px;border-bottom:1px solid var(--border);font-family:var(--font)}
  .column-heading b{color:var(--ink);font-size:10px}.column-heading small{overflow:hidden;color:var(--muted);font-size:9px;text-overflow:ellipsis;white-space:nowrap}
  .column-add{display:grid;gap:6px;margin-top:10px;padding-top:8px;border-top:1px dashed var(--accent-line);font-family:var(--font)}
  .column-add>span{color:var(--muted);font-size:9px;font-weight:650}
  .column-add>div{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:4px}
  .column-add button{min-width:0;min-height:27px;display:flex;align-items:center;gap:5px;padding:4px 6px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--muted);font-size:9px;text-align:left}
  .column-add button b{flex:none;color:var(--accent-2);font-size:10px}
  .column-add button:hover{border-color:var(--accent-line);color:var(--accent-2)}
  .table-editor{overflow:hidden;border:1px solid color-mix(in srgb,var(--accent) 35%,var(--border));border-radius:var(--r-md);background:var(--card)}
  .table-controls{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 10px;border-bottom:1px solid var(--border);background:var(--raise);font-family:var(--font)}
  .table-controls>span{color:var(--muted);font-size:9px;font-weight:650}.table-controls>div{display:flex;flex-wrap:wrap;gap:4px}
  .table-controls button{padding:4px 7px;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--card);color:var(--ink-dim);font-size:9px}
  .table-controls button:hover:not(:disabled){border-color:var(--accent-line);color:var(--accent-2)}.table-controls button:disabled{opacity:.35}
  .table-scroll{overflow:auto}table{width:100%;border-collapse:collapse;font:12px var(--font)}
  th,td{min-width:110px;border:1px solid color-mix(in srgb,var(--ink) 18%,var(--border));padding:10px 11px;text-align:left}
  th:first-child,td:first-child{border-left:0}th:last-child,td:last-child{border-right:0}tr:last-child td{border-bottom:0}
  th{background:color-mix(in srgb,var(--accent) 7%,var(--raise));color:var(--ink)}td{color:var(--ink-dim)}
  th:focus,td:focus{background:var(--accent-weak);box-shadow:inset 0 0 0 1px var(--accent-line)}
  hr{border:0;border-top:1px solid var(--border)}
  @media(max-width:700px){
    .columns,.columns.single,.columns.single.half-left,.columns.single.half-right,.columns.three,.columns.lead-left,.columns.lead-right{grid-template-columns:1fr;justify-content:stretch}
    .handle,.actions{display:none}
    .table-controls{align-items:flex-start;flex-direction:column}
  }
</style>
