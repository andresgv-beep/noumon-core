<script>
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { t, relTime } from './i18n.svelte.js';
  import { listPublishedDocuments } from './studioApi.js';
  import { serverUrl } from './connection.js';

  let { onOpenItem } = $props();
  let documents = $state([]);
  let query = $state('');
  let selectedTopic = $state('');
  let loading = $state(true);
  let denied = $state(false);
  let topics = $derived.by(() => {
    const counts = new Map();
    for (const doc of documents) {
      for (const topic of doc.classification?.topics || []) {
        counts.set(topic, (counts.get(topic) || 0) + 1);
      }
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .slice(0, 12);
  });
  // Portada de respaldo: sin ella, una estantería visual nace llena de huecos
  // grises, porque casi ningún documento tiene imagen. El tono sale del id, así
  // que es estable —el mismo documento tiene siempre el mismo color— y dos
  // documentos seguidos rara vez se parecen.
  function coverHue(id) {
    let hash = 0;
    for (const char of String(id)) hash = (hash * 31 + char.codePointAt(0)) % 360;
    return hash;
  }

  function coverInitial(title) {
    const clean = String(title || '').trim();
    return clean ? [...clean][0].toLocaleUpperCase() : '·';
  }

  function coverUrl(doc) {
    return doc.coverAssetId
      ? serverUrl(`/api/studio/documents/${encodeURIComponent(doc.id)}/assets/${encodeURIComponent(doc.coverAssetId)}`)
      : '';
  }

  // Seis en recientes: con tres por fila son dos filas completas, sin la fila
  // coja que dejaban cuatro. Van rotando solas segun publicas.
  let recent = $derived(documents.slice(0, 6));
  let featured = $derived(documents.filter((doc) => doc.featured).slice(0, 6));
  let filtered = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    return documents.filter((doc) =>
      (!selectedTopic || (doc.classification?.topics || []).includes(selectedTopic)) &&
      (!needle || [doc.title, doc.summary, doc.authorLabel, ...(doc.tags || []), ...(doc.classification?.topics || [])]
        .join(' ').toLocaleLowerCase().includes(needle)));
  });

  onMount(async () => {
    try {
      documents = await listPublishedDocuments();
    } catch (e) {
      denied = e.status === 401 || e.status === 403;
    }
    loading = false;
  });
</script>

<section class="documents scroll thin">
  <header class="hero">
    <div class="mark"><Icon name="note" size={25} /></div>
    <div>
      <span>{t('documents.knowledgeBase')}</span>
      <h1>{t('menu.documents')}</h1>
      <p>{t('documents.subtitle')}</p>
    </div>
  </header>

  <div class="search">
    <Icon name="search" size={17} />
    <input bind:value={query} placeholder={t('documents.search')} />
  </div>

  {#if loading}
    <div class="state">{t('common.loading')}</div>
  {:else if denied}
    <div class="state"><Icon name="lock" /> <h2>{t('documents.loginTitle')}</h2><p>{t('documents.loginBody')}</p></div>
  {:else if documents.length === 0}
    <div class="state"><Icon name="note" /> <h2>{t('documents.emptyTitle')}</h2><p>{t('documents.emptyBody')}</p></div>
  {:else}
    {#if topics.length}
      <nav class="topics" aria-label={t('documents.topics')}>
        <span>{t('documents.topics')}</span>
        <button class:active={!selectedTopic} onclick={() => (selectedTopic = '')}>{t('documents.allTopics')}</button>
        {#each topics as [topic, count] (topic)}
          <button class:active={selectedTopic === topic} onclick={() => (selectedTopic = topic)}>
            {topic}<small>{count}</small>
          </button>
        {/each}
      </nav>
    {/if}

    {#if !query.trim() && !selectedTopic && recent.length}
      {#if featured.length}
        <section class="recent featured">
          <div class="section-title"><span>{t('documents.featured')}</span></div>
          <div class="recent-grid">
            {#each featured as doc (doc.id)}
              <button onclick={() => onOpenItem?.(`studio:${doc.id}`)}>
                <span class="r-body">
                  <small>{doc.classification?.workType || t('documents.article')}</small>
                  <b>{doc.title}</b>
                  <span>{doc.summary || t('documents.noSummary')}</span>
                </span>
                <span class="r-cover">
                  {#if coverUrl(doc)}
                    <img src={coverUrl(doc)} alt="" loading="lazy" />
                  {:else}
                    <i style:--hue={coverHue(doc.id)}>{coverInitial(doc.title)}</i>
                  {/if}
                </span>
              </button>
            {/each}
          </div>
        </section>
      {/if}
      <section class="recent">
        <div class="section-title"><span>{t('documents.recent')}</span></div>
        <div class="recent-grid">
          {#each recent as doc (doc.id)}
            <button onclick={() => onOpenItem?.(`studio:${doc.id}`)}>
              <span class="r-body">
                <small>{doc.classification?.workType || t('documents.article')} · {relTime(doc.published || doc.updated)}</small>
                <b>{doc.title}</b>
                <span>{doc.summary || t('documents.noSummary')}</span>
              </span>
              <span class="r-cover">
                {#if coverUrl(doc)}
                  <img src={coverUrl(doc)} alt="" loading="lazy" />
                {:else}
                  <i style:--hue={coverHue(doc.id)}>{coverInitial(doc.title)}</i>
                {/if}
              </span>
            </button>
          {/each}
        </div>
      </section>
    {/if}

    <div class="count"><span>{t('documents.allPages')}</span><span>{filtered.length} {t('documents.published')}</span></div>
    <div class="grid">
      {#each filtered as doc (doc.id)}
        <button class="doc" onclick={() => onOpenItem?.(`studio:${doc.id}`)}>
          <!-- La portada manda: es lo que reconoces antes de leer el título.
               Sin imagen propia se dibuja una de respaldo, para que la parrilla
               no tenga huecos. -->
          {#if coverUrl(doc)}
            <img class="doc-cover" src={coverUrl(doc)} alt="" loading="lazy" />
          {:else}
            <div class="doc-cover fallback" style:--hue={coverHue(doc.id)} aria-hidden="true">
              <span>{coverInitial(doc.title)}</span>
            </div>
          {/if}
          <div class="doc-top">
            <span class="type">{doc.classification?.workType || t('documents.article')}</span>
            <small>{relTime(doc.published || doc.updated)}</small>
          </div>
          <h2>{doc.title}</h2>
          <p>{doc.summary || t('documents.noSummary')}</p>
          <div class="byline">
            <span>{doc.authorLabel || t('documents.localAuthor')}</span>
            <span class="arrow">→</span>
          </div>
          {#if doc.tags?.length}
            <div class="tags">{#each doc.tags.slice(0, 4) as tag}<span>{tag}</span>{/each}</div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</section>

<style>
  /* flex:1 y min-width:0 como Cabinet y Moments: es un elemento flex del armazón
     del lector, y sin estirarse se quedaba del ancho de su contenido, pegado a la
     izquierda y con su propia barra de desplazamiento a media pantalla. */
  .documents{flex:1;min-width:0;height:100%;overflow:auto;padding:clamp(28px,5vw,64px);background:var(--ground)}
  .hero{max-width:1050px;margin:0 auto 28px;display:flex;align-items:center;gap:18px}
  .mark{width:52px;height:52px;display:grid;place-items:center;border-radius:var(--r-lg);color:var(--accent-2);background:color-mix(in srgb,var(--accent) 18%,var(--panel));border:1px solid var(--accent-line)}
  .hero span{font-size:11px;color:var(--accent-2);font-weight:700;letter-spacing:.12em;text-transform:uppercase}
  h1{margin:2px 0 0;font-size:clamp(28px,4vw,44px);line-height:1}
  .hero p{margin:8px 0 0;color:var(--muted)}
  .search{max-width:1050px;margin:0 auto 24px;display:flex;align-items:center;gap:10px;padding:0 14px;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel);color:var(--muted)}
  .search input{flex:1;border:0;outline:0;background:transparent;color:var(--ink);padding:13px 0;font-size:14px}
  .topics{max-width:1050px;margin:0 auto 24px;display:flex;align-items:center;gap:7px;flex-wrap:wrap}
  .topics>span,.section-title,.count{color:var(--faint);font-size:10px;text-transform:uppercase;letter-spacing:.08em}
  .topics button{display:flex;align-items:center;gap:6px;padding:6px 10px;border:1px solid var(--border);border-radius:var(--r-pill);background:var(--panel);color:var(--muted);font-size:11px}
  .topics button.active{border-color:var(--accent-line);background:color-mix(in srgb,var(--accent) 12%,var(--panel));color:var(--accent-2)}
  .topics small{font-size:9px;opacity:.7}
  .recent{max-width:1050px;margin:0 auto 28px}
  .section-title{margin-bottom:9px}
  .recent-grid{display:grid;grid-template-columns:repeat(auto-fill,272px);justify-content:start;gap:10px}
  /* Apaisada, como el boceto: texto a la izquierda y portada a la derecha
     ocupando todo el alto, a sangre contra el borde. */
  .recent-grid button{min-width:0;min-height:96px;display:grid;grid-template-columns:minmax(0,1fr) 88px;align-items:stretch;overflow:hidden;padding:0;border:1px solid var(--border);border-radius:var(--r-lg);background:var(--panel);color:var(--ink);text-align:left}
  .r-body{min-width:0;display:flex;flex-direction:column;align-items:flex-start;padding:11px 12px}
  .r-cover{min-width:0;display:block;overflow:hidden}
  .r-cover img{width:100%;height:100%;object-fit:cover;display:block}
  .recent-grid button:hover{border-color:var(--accent-line)}
  .recent-grid small{color:var(--accent-2);font-size:9px;text-transform:uppercase;letter-spacing:.05em}
  .recent-grid b{margin:5px 0 4px;overflow-wrap:break-word;font-size:13.5px;line-height:1.25}
  .recent-grid span{color:var(--muted);font-size:11px;line-height:1.4;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
  .count{max-width:1050px;margin:0 auto 10px;display:flex;justify-content:space-between;gap:10px}
  .grid{max-width:1050px;margin:0 auto;display:grid;grid-template-columns:repeat(auto-fill,minmax(196px,1fr));gap:11px}
  /* La portada ocupa el ancho completo de la tarjeta y manda sobre el texto. */
  /* La portada ocupa todo el alto a la derecha, a sangre contra el borde. */
  .doc-cover{width:calc(100% + 28px);height:104px;margin:-14px -14px 10px;object-fit:cover;border-radius:var(--r-lg) var(--r-lg) 0 0;background:var(--raise)}
  /* Respaldo: tono estable derivado del id, inicial grande y marca de agua. */
  .doc-cover.fallback,.recent-grid i{display:grid;place-items:center;background:linear-gradient(150deg,hsl(var(--hue) 42% 26%),hsl(calc(var(--hue) + 38) 38% 15%));color:hsl(var(--hue) 60% 86%)}
  .doc-cover.fallback span{font:600 32px/1 var(--font-read,Georgia,serif);opacity:.62}
  .recent-grid i{width:100%;height:100%;font:600 20px/1 var(--font-read,Georgia,serif);font-style:normal;opacity:.62}
  .doc{min-height:0;padding:14px;overflow:hidden;display:flex;flex-direction:column;text-align:left;background:var(--card);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow-soft);color:var(--ink);transition:transform .14s,border-color .14s}
  .doc:hover{transform:translateY(-2px);border-color:var(--accent-line)}
  .doc-top,.byline{display:flex;align-items:center;justify-content:space-between;gap:10px}
  .type{font-size:10px;font-weight:700;color:var(--accent-2);text-transform:uppercase;letter-spacing:.08em}.doc small{color:var(--faint)}
  .doc h2{margin:9px 0 6px;font-size:15px;line-height:1.25;overflow-wrap:break-word}.doc p{margin:0;color:var(--muted);line-height:1.45;font-size:11.5px;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
  .byline{margin-top:auto;padding-top:12px;color:var(--ink-dim);font-size:11px}.arrow{color:var(--accent-2);font-size:15px}
  .tags{display:flex;gap:4px;flex-wrap:wrap;margin-top:8px;overflow:hidden;max-height:20px}.tags span{font-size:10px;padding:3px 7px;border-radius:var(--r-pill);background:var(--raise);color:var(--muted)}
  /* Piel retro: esquinas en punta, igual que el resto de la interfaz. */
  :global(:root[data-skin="retro"]) :is(.doc,.doc-cover,.recent-grid button){border-radius:0}
  .state{max-width:650px;margin:70px auto;text-align:center;color:var(--muted)}.state :global(.ic){color:var(--accent-2)}.state h2{color:var(--ink);margin-bottom:4px}.state p{margin-top:0}
  @media(max-width:900px){.recent-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
  @media(max-width:560px){.recent-grid{grid-template-columns:1fr}}
</style>
