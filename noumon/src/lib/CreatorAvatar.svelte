<script>
  import Logo from './Logo.svelte';
  let { src = '', name = '', size = 28 } = $props();
  let failed = $state(false);
  $effect(() => { src; failed = false; });
</script>

{#if src && !failed}
  <img {src} alt={name} loading="lazy" onerror={() => { failed = true; }} />
{:else}
  <span class="fallback"><Logo {size} /></span>
{/if}

<style>
  img { display: block; width: 100%; height: 100%; object-fit: cover; border-radius: inherit; }
  .fallback { display: flex; align-items: center; justify-content: center; width: 100%; height: 100%; background: var(--card); border-radius: inherit; }
</style>
