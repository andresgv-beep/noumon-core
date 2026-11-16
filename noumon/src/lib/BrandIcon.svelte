<script>
  // Every surface owns its transparent artwork, including its silhouette. The
  // radius prop used by older callers is intentionally no longer needed.
  let { kind = '', size = 28 } = $props();
  // Bounds of the opaque artwork inside each 256px transparent asset. Match
  // the visible silhouette, not the PNG/WebP canvas; centre the artwork itself.
  const ARTWORK = {
    documents: { src: '/brands/documentos-v2.webp', bounds: [17, 13, 232, 245] },
    moments: { src: '/brands/moments-v2.webp', bounds: [35, 30, 220, 229] },
    depot: { src: '/brands/programas-v3.webp', bounds: [24, 36, 230, 220] },
    cabinet: { src: '/brands/cabinet-v2.webp', bounds: [13, 16, 240, 244] },
  };
  const artwork = $derived(ARTWORK[kind === 'document' ? 'documents' : kind]);
  const framing = $derived.by(() => {
    if (!artwork) return '';
    const [left, top, right, bottom] = artwork.bounds;
    const scale = (256 * .95) / Math.max(right - left, bottom - top);
    const x = (128 - (left + right) / 2) / 256 * scale * 100;
    const y = (128 - (top + bottom) / 2) / 256 * scale * 100;
    return `translate(${x}%, ${y}%) scale(${scale})`;
  });
</script>

{#if artwork}
  <span class="bi" style="width:{size}px;height:{size}px">
    <img src={artwork.src} style:transform={framing} width={size} height={size} alt="" draggable="false" />
  </span>
{/if}

<style>
  .bi{flex:none;display:grid;place-items:center;background:none}
  .bi img{display:block;width:100%;height:100%;object-fit:contain}
</style>
