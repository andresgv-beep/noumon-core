<script>
  // Depot: el almacén de programas. Es el mockup aprobado ("catálogo técnico"):
  // filas de inventario, matriz de sistemas, monoespaciada para versiones y
  // pesos. Usa los tokens del tema, así que sigue luz, piel y acento.
  import { onMount } from 'svelte';
  import { getSurfaceItems } from './libraryApi.js';
  import { serverUrl } from './connection.js';
  import { t, i18n } from './i18n.svelte.js';
  import {
    OS_NAME, covers, totalSize, formatSize, formatsOf, formatStyle, programOf,
    matrixFor, licenseKind, shortDate,
  } from './depot.js';

  let { onOpenItem } = $props();
  let items = $state([]);
  let loading = $state(true);
  let failed = $state(false);
  let q = $state('');
  let shelf = $state('');
  let system = $state('');
  let license = $state('');
  let sort = $state('name');

  const SHELVES = ['multimedia', 'office', 'graphics', 'internet', 'utilities', 'system', 'development', 'education', 'games'];

  onMount(async () => {
    try {
      items = await getSurfaceItems('depot');
    } catch (e) {
      failed = true;
    }
    loading = false;
  });

  const iconOf = (item) => (item?.preview?.kind === 'image' && item.preview.url ? serverUrl(item.preview.url) : '');
  const shelfLabel = (key) => (SHELVES.includes(key) ? t('studio.depot.shelf.' + key) : key);
  const pad = (n) => String(n).padStart(2, '0');

  const columns = $derived(matrixFor(items));
  const query = $derived(q.trim().toLowerCase().replace(/^\./, ''));
  const filtered = $derived(!!(shelf || system || license || query));
  const shown = $derived.by(() => {
    const list = items.filter((item) => {
      if (shelf && programOf(item).shelf !== shelf) return false;
      if (system && !covers(item, system)) return false;
      if (license && licenseKind(item.license) !== license) return false;
      if (!query) return true;
      return (item.title || '').toLowerCase().includes(query) ||
        (item.description || '').toLowerCase().includes(query) ||
        formatsOf(item).includes(query);
    });
    if (sort === 'size') return list.slice().sort((a, b) => totalSize(b) - totalSize(a));
    if (sort === 'recent') {
      return list.slice().sort((a, b) => (programOf(b).published || '').localeCompare(programOf(a).published || ''));
    }
    return list.slice().sort((a, b) => (a.title || '').localeCompare(b.title || ''));
  });
  const fresh = $derived(items
    .filter((item) => programOf(item).published)
    .slice()
    .sort((a, b) => programOf(b).published.localeCompare(programOf(a).published))
    .slice(0, 4));
  const shelves = $derived(SHELVES
    .map((key) => ({ key, n: items.filter((item) => programOf(item).shelf === key).length }))
    .filter((entry) => entry.n > 0));

  function availability(item) {
    const list = columns.filter((m) => covers(item, m.key)).map((m) => OS_NAME[m.key] || 'ARM');
    return t('depot.availableFor', { list: list.join(', ') });
  }

  function reset() {
    q = '';
    shelf = '';
    system = '';
    license = '';
  }
</script>

<div class="depot">
  <header class="bar">
    <div class="bar-in">
      <div class="brand">
        <svg viewBox="0 0 24 24" width="26" height="26" aria-hidden="true"><path d="M12 2.8l8.2 4.6v9.2L12 21.2l-8.2-4.6V7.4z"/><path d="M3.8 7.4L12 12l8.2-4.6M12 12v9.2"/><path d="M7.9 5.1l8.2 4.6" stroke-dasharray="2 2"/></svg>
        <span>depot</span>
      </div>
      <label class="search">
        <span class="prompt" aria-hidden="true">&gt;</span>
        <input type="search" bind:value={q} placeholder={t('depot.search')} aria-label={t('depot.searchLabel')} />
      </label>
      <span class="count">{t('depot.count', { n: items.length, size: formatSize(items.reduce((sum, item) => sum + totalSize(item), 0)) })}</span>
    </div>
  </header>

  {#if loading}
    <div class="state">…</div>
  {:else if failed}
    <div class="state">{t('depot.loadError')}</div>
  {:else if items.length === 0}
    <div class="state">{t('depot.empty')}</div>
  {:else}
    <div class="body">
      <nav class="side" aria-label={t('depot.shelf')}>
        <div class="group">
          <span class="cap">{t('depot.shelf')}</span>
          <button class="shelf" class:on={!shelf} aria-pressed={!shelf} onclick={() => (shelf = '')}>
            <span>{t('depot.all')}</span><i></i><b>{pad(items.length)}</b>
          </button>
          {#each shelves as entry (entry.key)}
            <button class="shelf" class:on={shelf === entry.key} aria-pressed={shelf === entry.key} onclick={() => (shelf = entry.key)}>
              <span>{shelfLabel(entry.key)}</span><i></i><b>{pad(entry.n)}</b>
            </button>
          {/each}
        </div>
        <div class="group">
          <span class="cap">{t('depot.system')}</span>
          <div class="systems" role="group" aria-label={t('depot.system')}>
            <button class:on={!system} aria-pressed={!system} onclick={() => (system = '')}>{t('depot.allSystems')}</button>
            {#each columns as m (m.key)}
              <button class:on={system === m.key} aria-pressed={system === m.key} onclick={() => (system = m.key)}>{m.short}</button>
            {/each}
          </div>
        </div>
        <div class="group">
          <span class="cap">{t('depot.license')}</span>
          <div class="licenses" role="group" aria-label={t('depot.license')}>
            {#each [['', 'depot.licAll'], ['libre', 'depot.licFree'], ['gratuita', 'depot.licFreeware']] as [key, label] (key)}
              <button class:on={license === key} aria-pressed={license === key} onclick={() => (license = key)}><i></i>{t(label)}</button>
            {/each}
          </div>
        </div>
      </nav>

      <main class="main">
        {#if !filtered && fresh.length}
          <section class="fresh">
            <div class="head"><h2 class="cap">{t('depot.recent')}</h2><span class="rule"></span></div>
            <div class="tiles">
              {#each fresh as item (item.id)}
                <button class="tile" onclick={() => onOpenItem?.(item.id)}>
                  <span class="tile-top">
                    <span class="icon small">{#if iconOf(item)}<img src={iconOf(item)} alt="" loading="lazy" />{:else}{(item.title || '?').charAt(0)}{/if}</span>
                    <span class="mono date">{shortDate(programOf(item).published, i18n.locale)}</span>
                  </span>
                  <span class="tile-copy">
                    <span class="tile-name">{item.title}</span>
                    {#if programOf(item).version}<span class="mono ver-accent">v{programOf(item).version}</span>{/if}
                  </span>
                </button>
              {/each}
            </div>
          </section>
        {/if}

        <section class="catalog">
          <div class="head">
            <h2 class="cap">{shelf ? shelfLabel(shelf) : t('depot.catalog')}</h2>
            <span class="mono faint">{pad(shown.length)} / {pad(items.length)}</span>
            <span class="rule"></span>
            <label class="cap sort">{t('depot.sort')}
              <select bind:value={sort}>
                <option value="name">{t('depot.sortName')}</option>
                <option value="recent">{t('depot.sortRecent')}</option>
                <option value="size">{t('depot.sortSize')}</option>
              </select>
            </label>
          </div>

          <div class="cols" aria-hidden="true">
            <span class="c-icon"></span>
            <span class="cap c-main">{t('depot.colProgram')}</span>
            <span class="cap c-ver">{t('depot.colVersion')}</span>
            <span class="cap c-matrix" style="grid-template-columns:repeat({columns.length},1fr)">{#each columns as m (m.key)}<span>{m.short}</span>{/each}</span>
            <span class="cap c-size">{t('depot.colSize')}</span>
          </div>

          {#if shown.length}
            <div class="rows">
              {#each shown as item (item.id)}
                {@const kind = licenseKind(item.license)}
                <button class="row" onclick={() => onOpenItem?.(item.id)}>
                  <span class="icon c-icon">{#if iconOf(item)}<img src={iconOf(item)} alt="" loading="lazy" />{:else}{(item.title || '?').charAt(0)}{/if}</span>
                  <span class="c-main">
                    <span class="name-line">
                      <b class="name">{item.title}</b>
                      {#each formatsOf(item) as format (format)}<span class="fmt" style={formatStyle(format)}>{format}</span>{/each}
                    </span>
                    {#if item.description}<span class="desc">{item.description}</span>{/if}
                  </span>
                  <span class="c-ver">
                    <span class="mono">{programOf(item).version || '—'}</span>
                    {#if item.license}<span class="mono lic" class:libre={kind === 'libre'}>{item.license}</span>{/if}
                  </span>
                  <span class="c-matrix" role="img" aria-label={availability(item)} style="grid-template-columns:repeat({columns.length},1fr)">
                    {#each columns as m (m.key)}<i class:on={covers(item, m.key)}></i>{/each}
                  </span>
                  <span class="mono c-size">{formatSize(totalSize(item))}</span>
                </button>
              {/each}
            </div>
          {:else}
            <div class="none">
              <span class="mono">{t('depot.noResults')}</span>
              {#if filtered}<button onclick={reset}>{t('depot.clearFilters')}</button>{/if}
            </div>
          {/if}
        </section>
      </main>
    </div>
  {/if}
</div>

<style>
  .depot{flex:1;min-width:0;height:100%;overflow-y:auto;background-color:var(--ground);color:var(--ink);letter-spacing:var(--tracking);
    background-image:linear-gradient(color-mix(in srgb,var(--ink) 3.5%,transparent) 1px,transparent 1px),linear-gradient(90deg,color-mix(in srgb,var(--ink) 3.5%,transparent) 1px,transparent 1px);
    background-size:24px 24px}
  .mono{font-family:var(--mono)}
  .faint{color:var(--muted);font-size:11px}
  .cap{font-family:var(--mono);font-size:11px;letter-spacing:1.4px;text-transform:uppercase;color:var(--muted);font-weight:400;margin:0}
  .rule{flex:1;min-width:20px;border-bottom:1px solid var(--border-soft)}
  .head{display:flex;flex-wrap:wrap;align-items:baseline;gap:12px}

  .bar{position:sticky;top:0;z-index:2;background:color-mix(in srgb,var(--ground) 94%,transparent);border-bottom:1px solid var(--border-soft)}
  .bar-in{max-width:1240px;margin:0 auto;padding:0 28px;min-height:60px;display:flex;flex-wrap:wrap;align-items:center;gap:12px 20px;box-sizing:border-box}
  .brand{display:flex;align-items:center;gap:10px;font:700 17px var(--mono);letter-spacing:.6px}
  .brand svg{fill:none;stroke:var(--logo);stroke-width:1.7;stroke-linejoin:round}
  .search{flex:1 1 260px;min-width:0;max-width:520px;display:flex;align-items:center;gap:10px;height:40px;padding:0 12px;box-sizing:border-box;border-radius:var(--r-sm);background:var(--panel-2);border:1px solid var(--border)}
  .search:focus-within{border-color:var(--accent-line)}
  .prompt{color:var(--link);font:14px var(--mono)}
  .search input{flex:1;min-width:0;height:100%;border:0;outline:0;background:transparent;color:var(--ink);font:13px var(--mono)}
  .search input::placeholder{color:var(--muted)}
  .count{margin-left:auto;flex:none;font:12px var(--mono);color:var(--muted);white-space:nowrap}
  .state{padding:60px 28px;text-align:center;color:var(--muted);font:13px var(--mono)}

  .body{max-width:1240px;margin:0 auto;padding:28px 28px 72px;box-sizing:border-box;display:flex;flex-wrap:wrap;gap:32px;align-items:flex-start}
  .side{flex:1 1 200px;max-width:220px;min-width:0;display:flex;flex-direction:column;gap:26px}
  .group{display:flex;flex-direction:column;gap:8px}
  .shelf{display:flex;align-items:baseline;gap:6px;min-height:30px;padding:0 0 0 10px;border-left:2px solid var(--border-soft);color:var(--ink-dim);font-size:14px;text-align:left}
  .shelf:hover{color:var(--ink)}
  .shelf.on{border-left-color:var(--accent);color:var(--ink);font-weight:650}
  .shelf i{flex:1;min-width:12px;border-bottom:1px dotted var(--border);transform:translateY(-4px)}
  .shelf b{flex:none;font:400 12px var(--mono);color:var(--muted)}
  .systems{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px}
  .systems button{height:34px;border-radius:var(--r-sm);border:1px solid var(--border);color:var(--ink-dim);font:12px var(--mono)}
  .systems button:hover{color:var(--ink)}
  .systems button.on{background:var(--accent-weak);border-color:var(--accent-line);color:var(--link)}
  .licenses{display:flex;flex-direction:column;gap:2px}
  .licenses button{display:flex;align-items:center;gap:10px;min-height:30px;color:var(--ink-dim);font-size:14px;text-align:left}
  .licenses button:hover{color:var(--ink)}
  .licenses i{width:10px;height:10px;box-sizing:border-box;border-radius:50%;border:1.5px solid var(--muted)}
  .licenses button.on{color:var(--ink)}
  .licenses button.on i{border-color:var(--accent);background:var(--accent)}

  .main{flex:999 1 640px;min-width:0;display:flex;flex-direction:column;gap:34px}
  .fresh,.catalog{display:flex;flex-direction:column;gap:12px}
  .catalog{gap:10px}
  .tiles{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px}
  .tile{display:flex;flex-direction:column;align-items:stretch;gap:14px;padding:16px;border-radius:var(--r-sm);border:1px solid var(--border-soft);background:var(--panel);box-shadow:var(--shadow);color:var(--ink);text-align:left}
  .tile:hover{border-color:var(--accent-line)}
  .tile-top{display:flex;align-items:center;justify-content:space-between;gap:8px}
  .date{font-size:11px;color:var(--muted)}
  .tile-name{font-size:15px;font-weight:650}
  .tile-copy{display:flex;flex-direction:column;gap:3px;min-width:0}
  .ver-accent{font-size:12px;color:var(--link)}
  .sort{display:flex;align-items:center;gap:8px}
  .sort select{height:30px;padding:0 8px;border-radius:var(--r-sm);background:var(--panel-2);border:1px solid var(--border);color:var(--ink);font:12px var(--mono);letter-spacing:0;text-transform:none}

  .cols,.row{display:flex;flex-wrap:wrap;align-items:center;gap:10px 16px}
  .cols{padding:0 14px 6px;align-items:end}
  .cols .cap{font-size:10px}
  .c-icon{flex:none;width:52px}
  .c-main{flex:1 1 240px;min-width:0}
  .c-ver{flex:none;width:90px}
  .c-matrix{flex:none;width:132px;display:grid;justify-items:center;text-align:center}
  .c-size{flex:none;width:74px;text-align:right}
  .rows{display:flex;flex-direction:column;border-top:1px solid var(--border)}
  .row{padding:12px 14px;border-bottom:1px solid var(--border-soft);color:var(--ink);text-align:left}
  .row:hover{background:var(--panel)}
  .row:hover .name{text-decoration:underline;text-underline-offset:3px}
  .icon{height:52px;overflow:hidden;display:grid;place-items:center;border-radius:var(--r-lg);background:var(--raise);color:var(--ink);font-size:23px;font-weight:700}
  .icon.small{width:44px;height:44px;border-radius:var(--r-md);font-size:20px}
  .icon img{width:100%;height:100%;object-fit:contain}
  .row .c-main{display:flex;flex-direction:column;gap:4px}
  .name-line{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px}
  .name{font-size:15px;font-weight:650}
  .fmt{height:18px;padding:0 5px;display:inline-flex;align-items:center;border:1px solid;border-radius:min(3px,var(--r-sm));font:700 10px var(--mono);letter-spacing:.6px}
  .desc{font-size:13px;line-height:1.45;color:var(--ink-dim);display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
  .row .c-ver{display:flex;flex-direction:column;gap:3px;font-size:13px}
  .lic{font-size:11px;color:var(--muted)}
  .lic.libre{color:color-mix(in srgb,#3aa35f 62%,var(--ink))}
  .row .c-matrix i{width:14px;height:14px;box-sizing:border-box;border:1px dashed var(--border);border-radius:min(2px,var(--r-sm))}
  .row .c-matrix i.on{border-style:solid;border-color:var(--accent);background:var(--accent)}
  .row .c-size{font-size:12px;color:var(--ink-dim)}
  .none{display:flex;flex-direction:column;align-items:flex-start;gap:12px;padding:28px 14px;border-top:1px solid var(--border);border-bottom:1px solid var(--border-soft);color:var(--ink-dim)}
  .none button{height:36px;padding:0 14px;border-radius:var(--r-sm);border:1px solid var(--border);font-size:13px}
  @media(max-width:860px){.cols{display:none}}
</style>
