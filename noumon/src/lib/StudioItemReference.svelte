<script>
  import { t } from './i18n.svelte.js';
  import { resolveItemReference } from './libraryApi.js';

  let { itemId, titleSnapshot = '', kindSnapshot = '', onOpenItem } = $props();
  let state = $state('loading');
  let item = $state(null);

  $effect(() => {
    const target = itemId;
    const controller = new AbortController();
    state = 'loading';
    item = null;

    resolveItemReference(target, { signal: controller.signal })
      .then((resolved) => {
        if (controller.signal.aborted) return;
        state = resolved.state;
        item = resolved.item || null;
      })
      .catch((error) => {
        if (!controller.signal.aborted && error?.name !== 'AbortError') {
          state = 'unavailable';
          item = null;
        }
      });

    return () => controller.abort();
  });

  const label = () => {
    if (state === 'restricted') return t('documents.referenceRestricted');
    if (state === 'missing') return t('documents.referenceMissing');
    if (state === 'unavailable') return t('documents.referenceUnavailable');
    if (state === 'loading') return t('documents.referenceLoading');
    return item?.kind || kindSnapshot || 'Noumon';
  };

  const title = () =>
    state === 'available'
      ? (item?.title || titleSnapshot || itemId)
      : (titleSnapshot || itemId);
</script>

<button
  class="item-ref"
  class:degraded={state !== 'available' && state !== 'loading'}
  disabled={state !== 'available' || !onOpenItem}
  onclick={() => state === 'available' && onOpenItem?.(itemId)}
>
  <small>{label()}</small>
  <b>{title()}</b>
</button>

<style>
  .item-ref{display:flex;width:100%;flex-direction:column;align-items:flex-start;gap:3px;margin:22px 0;padding:14px 16px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--raise);color:var(--ink);text-align:left}
  .item-ref:not(:disabled){cursor:pointer}.item-ref:not(:disabled):hover{border-color:var(--accent-line);background:color-mix(in srgb,var(--accent) 8%,var(--raise))}
  .item-ref:disabled{opacity:.75}.item-ref.degraded{border-style:dashed}
  small{font-family:var(--font-sans,system-ui,sans-serif);color:var(--muted);text-transform:uppercase;font-size:9px;letter-spacing:.1em}
</style>
