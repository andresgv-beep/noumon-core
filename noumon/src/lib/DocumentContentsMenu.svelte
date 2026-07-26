<script>
  import { studioNavGroups } from './studioContent.js';
  import { t } from './i18n.svelte.js';

  let { pages = [], activePageID = '', title = '', onSelect } = $props();

  let groups = $derived(studioNavGroups(pages));
  // La numeración es del documento, no del grupo: la página 4 sigue siendo la 4
  // aunque estrene sección.
  let numberOf = $derived(new Map(pages.map((page, index) => [page.id, index + 1])));
</script>

<!--
  Navegación del documento, no de la aplicación. Vive DENTRO de la banda de la
  página: sin fondo de panel ni borde contra el armazón, para que se lea como
  parte del documento y no como una barra del navegador. Se queda pegada al
  desplazar, que es lo que se espera de un índice en un documento largo.
-->
<nav class="page-nav" aria-label={title || t('documents.contentsMenu')}>
  <div class="nav-head">
    <b>{title || t('documents.contentsMenu')}</b>
    <small>{t('documents.contentsCount', { count: pages.length })}</small>
  </div>
  {#each groups as group, groupIndex (group.section + groupIndex)}
    {#if group.section}
      <div class="nav-section" class:first={groupIndex === 0}>{group.section}</div>
    {/if}
    {#each group.pages as page (page.id)}
      <button
        type="button"
        class="page-link"
        class:active={page.id === activePageID}
        aria-current={page.id === activePageID ? 'page' : undefined}
        onclick={() => onSelect?.(page.id)}
      >
        <span>{numberOf.get(page.id)}</span>
        <b>{page.title}</b>
      </button>
    {/each}
  {/each}
</nav>

<style>
  .page-nav{position:sticky;top:24px;align-self:start;display:grid;gap:2px;min-width:0;font-family:var(--font,system-ui,sans-serif)}
  .nav-head{display:flex;flex-direction:column;gap:2px;margin-bottom:8px;padding:0 9px 8px;border-bottom:1px solid var(--border)}
  .nav-head b{overflow-wrap:break-word;color:var(--faint);font-size:9.5px;font-weight:700;letter-spacing:.1em;text-transform:uppercase}
  .nav-head small{color:var(--faint);font-size:9.5px}
  /* La sección abre grupo: su propio título y una separación por encima. La
     primera no necesita separarse de nada. */
  .nav-section{margin:14px 0 4px;padding:10px 9px 0;border-top:1px solid var(--border);color:var(--muted);font-size:9.5px;font-weight:700;letter-spacing:.1em;text-transform:uppercase}
  .nav-section.first{margin-top:0;padding-top:0;border-top:0}
  .page-link{width:100%;min-width:0;display:grid;grid-template-columns:16px minmax(0,1fr);align-items:start;gap:8px;padding:7px 9px;border:0;border-left:2px solid transparent;border-radius:0;background:transparent;color:var(--muted);text-align:left}
  .page-link:hover{color:var(--ink);background:color-mix(in srgb,var(--ink) 4%,transparent)}
  .page-link.active{border-left-color:var(--accent);color:var(--ink)}
  .page-link span{padding-top:1px;color:var(--faint);font-size:9.5px;text-align:right}
  .page-link b{overflow-wrap:break-word;font-size:12px;font-weight:520;line-height:1.4}
  .page-link.active b{font-weight:650}
  @media(max-width:900px){
    /* Sin sitio al lado: el índice se coloca encima del artículo. */
    .page-nav{position:static;margin-bottom:22px}
  }
</style>
