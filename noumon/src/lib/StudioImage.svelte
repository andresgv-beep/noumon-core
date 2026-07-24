<script>
  import { getStudioAssetBlob } from './studioApi.js';

  let { documentId, assetId, alt = '', compact = false, display = 'original' } = $props();
  let src = $state('');
  let failed = $state(false);

  $effect(() => {
    const doc = String(documentId || '');
    const asset = String(assetId || '');
    let cancelled = false;
    let objectURL = '';
    src = '';
    failed = false;
    if (doc && asset) {
      getStudioAssetBlob(doc, asset)
        .then((blob) => {
          if (cancelled) return;
          objectURL = URL.createObjectURL(blob);
          src = objectURL;
        })
        .catch(() => {
          if (!cancelled) failed = true;
        });
    }
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  });
</script>

{#if src}
  <img {src} {alt} class:compact class:poster={display === 'poster'} />
{:else}
  <div class="placeholder" class:failed aria-hidden="true"></div>
{/if}

<style>
  img,.placeholder{display:block;width:100%;min-height:120px;border-radius:var(--r-md);background:var(--raise);object-fit:contain}
  img.compact{max-height:260px}
  img.poster{height:clamp(230px,34vw,520px);min-height:0;object-fit:cover}
  .placeholder{background:linear-gradient(110deg,var(--raise),var(--panel-2),var(--raise));background-size:220% 100%;animation:pulse 1.4s linear infinite}
  .placeholder.failed{animation:none;opacity:.5}
  @keyframes pulse{to{background-position:-220% 0}}
  @media(prefers-reduced-motion:reduce){.placeholder{animation:none}}
</style>
