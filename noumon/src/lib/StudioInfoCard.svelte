<script>
  import StudioImage from './StudioImage.svelte';
  import { t } from './i18n.svelte.js';

  let { documentId, card = {}, compact = false } = $props();
  let rows = $derived(
    (Array.isArray(card?.rows) ? card.rows : [])
      .filter((row) => String(row?.label || '').trim() || String(row?.value || '').trim()),
  );
</script>

<aside class="info-card" class:compact aria-label={t('documents.infoCard')}>
  {#if card.assetId}
    <figure>
      <StudioImage
        {documentId}
        assetId={card.assetId}
        alt={card.caption || t('documents.infoCardImage')}
        compact
      />
      {#if card.caption}<figcaption>{card.caption}</figcaption>{/if}
    </figure>
  {:else if card.caption}
    <p class="caption-only">{card.caption}</p>
  {/if}

  {#if rows.length}
    <dl>
      {#each rows as row}
        <div>
          {#if row.label}<dt>{row.label}</dt>{/if}
          {#if row.value}<dd>{row.value}</dd>{/if}
        </div>
      {/each}
    </dl>
  {/if}
</aside>

<style>
  .info-card{align-self:start;min-width:0;overflow:hidden;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card);color:var(--ink);font-family:var(--font,system-ui,sans-serif);box-shadow:var(--shadow-soft)}
  figure{margin:0;background:var(--raise)}
  figure :global(img),figure :global(.placeholder){width:100%;max-height:320px;min-height:150px;border-radius:0;object-fit:cover}
  figcaption,.caption-only{margin:0;padding:9px 12px;color:var(--muted);font-size:11px;line-height:1.45;text-align:center}
  .caption-only{border-bottom:1px solid var(--border)}
  dl{margin:0;padding:8px 11px 11px}
  dl div{display:grid;grid-template-columns:minmax(74px,.42fr) minmax(0,1fr);gap:9px;padding:7px 2px;border-bottom:1px solid var(--border)}
  dl div:last-child{border-bottom:0}
  dt,dd{min-width:0;margin:0;overflow-wrap:anywhere;font-size:11.5px;line-height:1.45}
  dt{font-weight:700;color:var(--ink)}
  dd{color:var(--muted)}
  .compact{box-shadow:none}
  :global(:root[data-skin="retro"]) .info-card{border-radius:0;box-shadow:var(--shadow)}
</style>
