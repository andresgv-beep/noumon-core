<script>
  import { studioNavGroups } from './studioContent.js';
  import { t } from './i18n.svelte.js';

  let {
    pages = [], activePageID = '', title = '', frame = 'none', fontSize = 0,
    numbers = true, count = true, borderWidth = 1, titleSize = 0,
    titleColor = '', textColor = '', titleBg = '', bg = '', onSelect,
  } = $props();

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
<nav
  class="page-nav"
  class:framed={frame === 'framed'}
  class:rounded={frame === 'rounded'}
  style:--nav-font={Number(fontSize) >= 9 && Number(fontSize) <= 24 ? `${Math.round(fontSize)}px` : null}
  style:--nav-cols={numbers ? null : 'minmax(0,1fr)'}
  style:--nav-border={`${Math.min(6, Math.max(0, Number(borderWidth) ?? 1))}px`}
  style:--nav-title-size={Number(titleSize) >= 8 && Number(titleSize) <= 24 ? `${Math.round(titleSize)}px` : null}
  style:--nav-title-color={titleColor || null}
  style:--nav-text-color={textColor || null}
  style:--nav-title-bg={titleBg || null}
  style:--nav-bg={bg || null}
  class:filled={Boolean(bg)}
  class:barred={Boolean(titleBg)}
  aria-label={title || t('documents.contentsMenu')}
>
  <div class="nav-head">
    <b>{title || t('documents.contentsMenu')}</b>
    {#if count}<small>{t('documents.contentsCount', { count: pages.length })}</small>{/if}
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
        {#if numbers}<span>{numberOf.get(page.id)}</span>{/if}
        <b>{page.title}</b>
      </button>
    {/each}
  {/each}
</nav>

<style>
  .page-nav{position:sticky;top:24px;align-self:start;display:grid;gap:2px;min-width:0;background:var(--nav-bg,transparent);font-family:var(--font,system-ui,sans-serif);font-size:var(--nav-font,12px)}
  /* Marco opcional del índice: a ras por defecto, para que parezca parte del
     documento y no una caja aparte. Sólo el borde, sin fondo ni sombra: así el
     índice sigue sobre la página en vez de convertirse en una tarjeta. Con
     marco necesita su propio relleno. */
  .page-nav.framed,.page-nav.rounded{border:var(--nav-border,1px) solid var(--border)}
  /* Un fondo propio (del menú o de su barra de título) también obliga a rellenar:
     sin relleno el color quedaría pegado al texto. Es el mismo relleno que el del
     marco, así que combinarlos no lo duplica. */
  .page-nav.framed,.page-nav.rounded,.page-nav.filled,.page-nav.barred{padding:14px 6px}
  .page-nav.rounded{border-radius:var(--r-lg)}
  :global(:root[data-skin="retro"]) .page-nav.rounded{border-radius:0}
  .nav-head{display:flex;flex-direction:column;gap:2px;margin-bottom:8px;padding:0 9px 8px;border-bottom:1px solid var(--border)}
  /* Con color propio deja de ser un encabezado a ras y pasa a ser una barra: se
     estira hasta el borde del marco anulando el relleno del menú. */
  .page-nav.barred .nav-head{margin:-14px -6px 8px;padding:13px 15px 11px;border-bottom:0;background:var(--nav-title-bg)}
  .page-nav.rounded.barred .nav-head{border-radius:var(--r-lg) var(--r-lg) 0 0}
  :global(:root[data-skin="retro"]) .page-nav.rounded.barred .nav-head{border-radius:0}
  .nav-head b{overflow-wrap:break-word;color:var(--nav-title-color,var(--faint));font-size:var(--nav-title-size,.79em);font-weight:700;letter-spacing:.1em;text-transform:uppercase}
  /* El recuento acompaña al título: si el autor le da color, lo sigue, porque
     sobre una barra de color el gris de la paleta puede desaparecer. */
  .nav-head small{color:var(--nav-title-color,var(--faint));font-size:.79em;opacity:.75}
  /* La sección abre grupo: su propio título y una separación por encima. La
     primera no necesita separarse de nada. */
  .nav-section{margin:14px 0 4px;padding:10px 9px 0;border-top:1px solid var(--border);color:var(--muted);font-size:.79em;font-weight:700;letter-spacing:.1em;text-transform:uppercase}
  .nav-section.first{margin-top:0;padding-top:0;border-top:0}
  /* font-family/size heredados a mano: un <button> NO hereda el tipo de letra del
     documento, el navegador le impone el suyo (13.3px Arial). Sin esto el tamaño
     del menú movía el título, que es un <div>, y dejaba las páginas clavadas. */
  .page-link{width:100%;min-width:0;display:grid;grid-template-columns:var(--nav-cols,16px minmax(0,1fr));align-items:start;gap:8px;padding:7px 9px;border:0;border-left:2px solid transparent;border-radius:0;background:transparent;color:var(--nav-text-color,var(--muted));font-family:inherit;font-size:inherit;text-align:left}
  /* El color del autor manda también al pasar por encima y en la página actual:
     var(--ink) es el de la paleta y sobre un fondo elegido a mano puede no verse.
     La página actual se distingue por el filo y el grosor, no por el color. */
  .page-link:hover{color:var(--nav-text-color,var(--ink));background:color-mix(in srgb,var(--ink) 4%,transparent)}
  .page-link.active{border-left-color:var(--accent);color:var(--nav-text-color,var(--ink))}
  .page-link span{padding-top:1px;color:var(--nav-text-color,var(--faint));font-size:.79em;text-align:right;opacity:.7}
  .page-link b{overflow-wrap:break-word;font-size:1em;font-weight:520;line-height:1.4}
  .page-link.active b{font-weight:650}
  @media(max-width:900px){
    /* Sin sitio al lado: el índice se coloca encima del artículo. */
    .page-nav{position:static;margin-bottom:22px}
  }
</style>
