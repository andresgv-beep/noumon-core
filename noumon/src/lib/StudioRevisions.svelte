<script>
  import { t, relTime } from './i18n.svelte.js';

  let { state } = $props();

  let revisions = $derived(state.revisions || []);
</script>

<!--
  Historial del documento, en la columna de herramientas y no sobre el lienzo.
  Vivía dentro de la columna del editor, entre la franja de metadatos y la
  página, y siendo las tres cajas del mismo panel se leían como una sola pila:
  parecía parte de lo que estabas editando. Aquí es una herramienta más y, sobre
  todo, deja ver la página mientras se recorre el historial.
-->
<div class="revisions">
  {#if state.revisionsLoading}
    <p class="note">{t('common.loading')}</p>
  {:else if !revisions.length}
    <p class="note">{t('studio.revisionsEmpty')}</p>
  {:else}
    {#each revisions as revision (revision.revision)}
      {@const isCurrent = revision.revision === state.currentRevision}
      <div class="revision" class:current={isCurrent}>
        <span>
          <b>{revision.title}</b>
          <small>
            {t('studio.revisionNumber', { revision: revision.revision })} · {relTime(revision.created)}
            {#if isCurrent} · {t('studio.revisionCurrent')}{/if}
            {#if revision.revision === state.publishedRevision} · {t('studio.revisionPublished')}{/if}
          </small>
        </span>
        <button
          disabled={!!state.restoringRevision || isCurrent}
          onclick={() => state.restoreRevision?.(revision)}
        >
          {t('studio.restore')}
        </button>
      </div>
    {/each}
  {/if}
</div>

<style>
  .revisions{display:grid;gap:4px;align-content:start;padding:2px 0 8px}
  .note{margin:0;padding:10px 4px;color:var(--faint);font-size:11.5px}
  /* En columna estrecha el texto manda y el botón se queda con lo justo: sin
     min-width:0 una etiqueta larga estira la fila y saca barra horizontal. */
  .revision{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:7px 9px;border-radius:var(--r-sm);background:var(--raise)}
  .revision span{display:flex;min-width:0;flex-direction:column;gap:1px}
  .revision b{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;font-weight:600}
  .revision small{color:var(--faint);font-size:10.5px}
  .revision button{flex:none;padding:5px 9px;font-size:11px}
  /* La revisión actual no se puede restaurar sobre sí misma: se marca en vez de
     dejar un botón apagado sin explicación. */
  .revision.current{background:color-mix(in srgb,var(--accent) 12%,var(--panel))}
</style>
