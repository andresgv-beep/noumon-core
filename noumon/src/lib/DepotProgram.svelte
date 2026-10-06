<script>
  // Ficha de un programa de Depot, tal como el mockup aprobado: la descarga que
  // le toca a este equipo (ticket), capturas, qué hace, la matriz sistema ×
  // arquitectura que filtra los archivos, ficha técnica y versiones. Descargar
  // pasa siempre por el permiso del servidor (downloadMedia).
  import { getItem, getSurfaceItems } from './libraryApi.js';
  import { serverUrl } from './connection.js';
  import { downloadMedia } from './auth.svelte.js';
  import { t, i18n } from './i18n.svelte.js';
  import {
    OS_NAME, detectDevice, bestFileFor, formatSize, formatStyle, programOf, totalSize,
    licenseKind, shortDate,
  } from './depot.js';

  let { tab, onOpenItem, onOpenView } = $props();
  let item = $state(null);
  let loading = $state(true);
  let related = $state([]);
  let shot = $state(0);
  let cell = $state('');
  let copied = $state('');
  const device = detectDevice();

  const SHELVES = ['multimedia', 'office', 'graphics', 'internet', 'utilities', 'system', 'development', 'education', 'games'];
  const ARCHS = ['x64', 'arm64', 'x86'];

  async function load(id) {
    loading = true;
    shot = 0;
    cell = '';
    try {
      item = await getItem(id);
    } catch (e) {
      item = null;
    }
    loading = false;
    try {
      const all = await getSurfaceItems('depot');
      const shelf = programOf(item).shelf;
      related = all.filter((other) => other.id !== item?.id && shelf && programOf(other).shelf === shelf).slice(0, 4);
    } catch (e) {
      related = [];
    }
  }

  $effect(() => {
    if (tab?.itemId) load(tab.itemId);
  });

  const program = $derived(programOf(item));
  const files = $derived(program.files || []);
  const shots = $derived(program.screenshots || []);
  const best = $derived(item ? bestFileFor(item, device) : null);
  const icon = $derived(item?.preview?.kind === 'image' && item.preview.url ? serverUrl(item.preview.url) : '');
  const libre = $derived(licenseKind(item?.license) === 'libre');
  const shelfLabel = (key) => (SHELVES.includes(key) ? t('studio.depot.shelf.' + key) : key);
  const pad = (n) => String(n).padStart(2, '0');

  // Matriz: filas = sistemas con archivos (y el de este equipo, para que se vea
  // que falta); columnas = arquitecturas que aparecen.
  const systems = $derived(['win', 'linux', 'mac'].filter((os) => files.some((f) => f.os === os) || os === device.os));
  const archs = $derived(ARCHS.filter((arch) => files.some((f) => f.arch === arch)));
  const filesIn = (os, arch) => files.filter((f) => f.os === os && f.arch === arch);
  const shownFiles = $derived(cell ? files.filter((f) => `${f.os}/${f.arch}` === cell) : files);
  const osLabel = (os) => (OS_NAME[os] || os).toLowerCase();

  // La primera frase va bajo el título, como en el mockup; el texto entero, en
  // "Qué hace".
  const paragraphs = (text) => String(text || '').split(/\n+/).map((p) => p.trim()).filter(Boolean);
  const lead = $derived.by(() => {
    const first = paragraphs(item?.description)[0] || '';
    const match = first.match(/^.*?[.!?](?=\s|$)/);
    return match ? match[0] : first;
  });

  function download(file) {
    if (!file) return;
    const url = `${file.url}${file.url.includes('?') ? '&' : '?'}name=${encodeURIComponent(file.name)}`;
    downloadMedia(serverUrl(url), file.name);
  }

  async function copySha(file) {
    try { await navigator.clipboard?.writeText(file.sha256); } catch (e) {}
    copied = file.sha256;
  }
</script>

<div class="program">
  <header class="bar">
    <div class="bar-in">
      <button class="brand" onclick={() => onOpenView?.('depot')}>
        <svg viewBox="0 0 24 24" width="26" height="26" aria-hidden="true"><path d="M12 2.8l8.2 4.6v9.2L12 21.2l-8.2-4.6V7.4z"/><path d="M3.8 7.4L12 12l8.2-4.6M12 12v9.2"/><path d="M7.9 5.1l8.2 4.6" stroke-dasharray="2 2"/></svg>
        <span>depot</span>
      </button>
      {#if program.shelf}<span class="sep" aria-hidden="true">/</span><span class="crumb">{shelfLabel(program.shelf).toLowerCase()}</span>{/if}
      {#if item}<span class="sep" aria-hidden="true">/</span><span class="crumb here">{item.title.toLowerCase()}</span>{/if}
    </div>
  </header>

  {#if loading}
    <div class="state">…</div>
  {:else if !item}
    <div class="state">{t('depot.notFound')}</div>
  {:else}
    <main class="page">
      <div class="hero">
        <div class="identity">
          <span class="icon">{#if icon}<img src={icon} alt="" />{:else}{(item.title || '?').charAt(0)}{/if}</span>
          <div class="titles">
            <h1>{item.title}</h1>
            {#if lead}<p class="lead">{lead}</p>{/if}
            <div class="meta mono">
              {#if program.version}<span>v<b>{program.version}</b></span>{/if}
              {#if item.authors?.length}<span>{item.authors.join(', ').toLowerCase()}</span>{/if}
              {#if item.license}<span class:libre>{item.license}</span>{/if}
              <span>{t(files.length === 1 ? 'depot.oneFile' : 'depot.manyFiles', { n: files.length })} · {formatSize(totalSize(item))}</span>
            </div>
          </div>
        </div>

        <section class="ticket" aria-label={t('depot.thisDevice')}>
          <div class="stub" aria-hidden="true"><span>{t('depot.thisDevice')}</span></div>
          <div class="ticket-body">
            {#if best}
              <div class="ticket-top">
                <span class="cap">{osLabel(best.os)} · {best.arch}</span>
                <span class="mono faint">{formatSize(best.size)}</span>
              </div>
              <span class="mono ticket-file">{best.name}</span>
              <button class="primary" onclick={() => download(best)}>
                <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M12 3v12m0 0l-4-4m4 4l4-4M5 21h14"/></svg>
                {t('depot.download')}
              </button>
            {:else}
              <span class="cap">{device.os ? `${osLabel(device.os)} · ${device.arch}` : t('depot.thisDevice')}</span>
              <span class="faint">{t('depot.noneForDevice')}</span>
            {/if}
            <a class="mono other" href="#depot-files">{t('depot.otherSystem')}</a>
          </div>
        </section>
      </div>

      <div class="columns">
        <div class="main-col">
          {#if shots.length}
            <section class="block" aria-label={t('depot.screenshot', { n: shot + 1, total: shots.length })}>
              <div class="stage">
                <img src={serverUrl(shots[shot].url)} alt={shots[shot].caption || ''} />
                {#if shots.length > 1}
                  <button class="nav prev" aria-label={t('depot.prevShot')} onclick={() => (shot = (shot + shots.length - 1) % shots.length)}>
                    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M15 18l-6-6 6-6"/></svg>
                  </button>
                  <button class="nav next" aria-label={t('depot.nextShot')} onclick={() => (shot = (shot + 1) % shots.length)}>
                    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M9 18l6-6-6-6"/></svg>
                  </button>
                {/if}
                <span class="mono counter">{pad(shot + 1)} / {pad(shots.length)}</span>
              </div>
              {#if shots[shot].caption}<p class="caption">{shots[shot].caption}</p>{/if}
              {#if shots.length > 1}
                <div class="thumbs">
                  {#each shots as s, index (s.url)}
                    <button class:on={index === shot} aria-pressed={index === shot}
                      aria-label={t('depot.screenshot', { n: index + 1, total: shots.length })}
                      onclick={() => (shot = index)}><img src={serverUrl(s.url)} alt="" loading="lazy" /></button>
                  {/each}
                </div>
              {/if}
            </section>
          {/if}

          {#if item.description}
            <section class="block">
              <div class="head"><h2 class="cap">{t('depot.whatItDoes')}</h2><span class="rule"></span></div>
              {#each paragraphs(item.description) as p}<p class="read">{p}</p>{/each}
            </section>
          {/if}

          <section id="depot-files" class="block">
            <div class="head">
              <h2 class="cap">{t('depot.files')}</h2>
              <span class="mono faint">{pad(shownFiles.length)} / {pad(files.length)}</span>
              <span class="rule"></span>
              {#if cell}<button class="mono chip" onclick={() => (cell = '')}>{t('depot.showAll')}</button>{/if}
            </div>

            {#if archs.length}
              <div class="grid" role="group" aria-label={t('depot.system')} style="grid-template-columns:96px repeat({archs.length},minmax(0,1fr))">
                <span></span>
                {#each archs as arch (arch)}<span class="cap arch-head">{arch}</span>{/each}
                {#each systems as os (os)}
                  <span class="mono os">{osLabel(os)}</span>
                  {#each archs as arch (arch)}
                    {@const hits = filesIn(os, arch)}
                    {@const key = `${os}/${arch}`}
                    <button class="cell" class:off={!hits.length} class:on={cell === key}
                      aria-pressed={cell === key} aria-disabled={!hits.length}
                      onclick={() => { if (hits.length) cell = cell === key ? '' : key; }}>
                      {#each hits as file (file.url)}<span class="fmt" style={formatStyle(file.format)}>{file.format}</span>{/each}
                      {#if !hits.length}<span class="mono unavailable">{t('depot.unavailable')}</span>{/if}
                    </button>
                  {/each}
                {/each}
              </div>
            {/if}

            <div class="files">
              {#each shownFiles as file (file.url)}
                <div class="file">
                  <span class="fmt big" style={formatStyle(file.format)}>{file.format}</span>
                  <span class="fname">
                    <b>{file.label || file.name}</b>
                    {#if file.label}<span class="mono">{file.name}</span>{/if}
                  </span>
                  <span class="mono faint arch">{file.os === 'win' ? 'win' : file.os === 'linux' ? 'lnx' : file.os} · {file.arch === 'x64' ? '64 bits' : file.arch}</span>
                  <span class="mono faint fsize">{formatSize(file.size)}</span>
                  <button class="mono sha" title="SHA-256" aria-label={t('depot.copyShaAria', { file: file.name })} onclick={() => copySha(file)}>
                    {copied === file.sha256 ? t('depot.copied') : 'sha ' + String(file.sha256 || '').slice(0, 8)}
                  </button>
                  <button class="dl" onclick={() => download(file)}>{t('depot.download')}</button>
                </div>
              {/each}
            </div>
            <span class="mono faint">{t('depot.verifyHint')}</span>
          </section>
        </div>

        <aside class="side-col">
          <section class="block">
            <div class="head"><h2 class="cap">{t('depot.spec')}</h2><span class="rule"></span></div>
            <dl class="spec">
              {#each [
                ['depot.version', program.version],
                ['depot.author', (item.authors || []).join(', ')],
                ['depot.license', item.license],
                ['depot.shelf', program.shelf ? shelfLabel(program.shelf).toLowerCase() : ''],
                ['depot.languages', program.languages],
                ['depot.requires', program.requirements],
                ['depot.added', program.published],
                ['depot.website', program.website],
              ].filter(([, value]) => value) as [key, value] (key)}
                <div><dt>{t(key)}</dt><i></i><dd class="mono">{value}</dd></div>
              {/each}
            </dl>
          </section>

          {#if program.version || program.notes}
            <section class="block">
              <div class="head"><h2 class="cap">{t('depot.versions')}</h2><span class="rule"></span></div>
              <ol class="timeline">
                <li>
                  <i class="dot" aria-hidden="true"></i>
                  <div>
                    <div class="mono tl-head">
                      <b>{program.version || '—'}</b>
                      {#if program.published}<span>{program.published}</span>{/if}
                      <span class="tag">{t('depot.current')}</span>
                    </div>
                    {#if program.notes}{#each paragraphs(program.notes) as p}<p class="note">{p}</p>{/each}{/if}
                  </div>
                </li>
              </ol>
            </section>
          {/if}

          {#if related.length}
            <section class="block">
              <div class="head"><h2 class="cap">{t('depot.sameShelf')}</h2><span class="rule"></span></div>
              <div class="related">
                {#each related as other (other.id)}
                  <button class="rel" onclick={() => onOpenItem?.(other.id)}>
                    <span class="rel-icon">{#if other.preview?.kind === 'image' && other.preview.url}<img src={serverUrl(other.preview.url)} alt="" loading="lazy" />{:else}{(other.title || '?').charAt(0)}{/if}</span>
                    <b>{other.title}</b>
                    <span class="mono faint">{programOf(other).version || ''}</span>
                  </button>
                {/each}
              </div>
            </section>
          {/if}
        </aside>
      </div>
    </main>
  {/if}
</div>

<style>
  .program{flex:1;min-width:0;height:100%;overflow-y:auto;background-color:var(--ground);color:var(--ink);letter-spacing:var(--tracking);
    background-image:linear-gradient(color-mix(in srgb,var(--ink) 3.5%,transparent) 1px,transparent 1px),linear-gradient(90deg,color-mix(in srgb,var(--ink) 3.5%,transparent) 1px,transparent 1px);
    background-size:24px 24px}
  .mono{font-family:var(--mono)}
  .faint{color:var(--muted);font-size:12px}
  .cap{font-family:var(--mono);font-size:11px;letter-spacing:1.4px;text-transform:uppercase;color:var(--muted);font-weight:400;margin:0}
  .rule{flex:1;min-width:20px;border-bottom:1px solid var(--border-soft)}
  .head{display:flex;flex-wrap:wrap;align-items:baseline;gap:12px}

  .bar{background:color-mix(in srgb,var(--ground) 94%,transparent);border-bottom:1px solid var(--border-soft)}
  .bar-in{max-width:1240px;margin:0 auto;padding:0 28px;min-height:60px;display:flex;flex-wrap:wrap;align-items:center;gap:10px 16px;box-sizing:border-box}
  .brand{display:flex;align-items:center;gap:10px;min-height:44px;font:700 17px var(--mono);letter-spacing:.6px;color:var(--ink)}
  .brand svg{fill:none;stroke:var(--logo);stroke-width:1.7;stroke-linejoin:round}
  .sep{color:var(--faint);font-family:var(--mono)}
  .crumb{font:13px var(--mono);color:var(--ink-dim)}.crumb.here{color:var(--ink)}
  .state{padding:60px 28px;text-align:center;color:var(--muted);font:13px var(--mono)}

  .page{max-width:1240px;margin:0 auto;padding:36px 28px 80px;box-sizing:border-box;display:flex;flex-direction:column;gap:40px}
  .hero{display:flex;flex-wrap:wrap;gap:28px;align-items:stretch}
  .identity{flex:999 1 520px;min-width:0;display:flex;gap:24px;align-items:center}
  .icon{width:104px;height:104px;flex:none;overflow:hidden;display:grid;place-items:center;border-radius:calc(var(--r-lg) * 1.7);background:var(--raise);font-size:46px;font-weight:700;
    box-shadow:0 0 0 1px var(--border-soft),0 0 0 7px var(--panel),0 0 0 8px var(--border-soft)}
  .icon img{width:100%;height:100%;object-fit:contain}
  .titles{min-width:0;display:flex;flex-direction:column;gap:10px}
  h1{margin:0;font-size:36px;font-weight:700;letter-spacing:-.6px;line-height:1.1}
  .lead{margin:0;font-size:15px;color:var(--ink-dim)}
  .meta{display:flex;flex-wrap:wrap;gap:6px 18px;font-size:12px;color:var(--muted)}
  .meta b{color:var(--link);font-weight:600}
  .meta .libre{color:color-mix(in srgb,#3aa35f 62%,var(--ink))}

  .ticket{flex:1 1 320px;min-width:0;display:flex;flex-direction:row;border-radius:var(--r-sm);background:var(--panel);border:1px solid var(--border);box-shadow:var(--shadow);overflow:hidden}
  .stub{flex:none;width:34px;display:flex;align-items:center;justify-content:center;background:var(--accent-weak);border-right:1px dashed var(--accent-line)}
  .stub span{writing-mode:vertical-rl;transform:rotate(180deg);font:10px var(--mono);letter-spacing:2px;text-transform:uppercase;color:var(--link)}
  .ticket-body{flex:1;min-width:0;padding:16px 18px;display:flex;flex-direction:column;gap:10px}
  .ticket-top{display:flex;align-items:baseline;justify-content:space-between;gap:10px}
  .ticket-file{font-size:14px;overflow-wrap:anywhere}
  .primary{display:flex;align-items:center;justify-content:center;gap:10px;height:46px;border-radius:var(--r-sm);background:var(--accent);border:1px solid var(--accent);color:var(--on-accent,#fff);font-size:15px;font-weight:700}
  .primary svg,.nav svg{fill:none;stroke:currentColor;stroke-width:2.2;stroke-linecap:round;stroke-linejoin:round}
  .primary:hover{filter:brightness(1.06)}
  .other{font-size:12px;color:var(--ink-dim)}

  .columns{display:flex;flex-wrap:wrap;gap:36px;align-items:flex-start}
  .main-col{flex:999 1 620px;min-width:0;display:flex;flex-direction:column;gap:40px}
  .side-col{flex:1 1 280px;min-width:0;display:flex;flex-direction:column;gap:36px}
  .block{display:flex;flex-direction:column;gap:12px}
  .read{margin:0;font-family:var(--font-read);font-size:15px;line-height:1.7;color:var(--ink-dim)}

  .stage{position:relative;aspect-ratio:16/9;overflow:hidden;border-radius:var(--r-sm);background:var(--panel-2);border:1px solid var(--border);display:grid;place-items:center}
  .stage img{width:100%;height:100%;object-fit:contain}
  .nav{position:absolute;top:50%;transform:translateY(-50%);width:44px;height:44px;display:grid;place-items:center;border-radius:var(--r-sm);border:1px solid var(--border);background:color-mix(in srgb,var(--ground) 88%,transparent);color:var(--ink)}
  .prev{left:12px}.next{right:12px}
  .counter{position:absolute;left:14px;bottom:12px;font-size:11px;color:var(--muted)}
  .caption{margin:0;font-size:13px;color:var(--ink-dim)}
  .thumbs{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}
  .thumbs button{aspect-ratio:16/9;overflow:hidden;padding:0;border-radius:min(3px,var(--r-sm));border:1px solid var(--border);background:var(--panel-2)}
  .thumbs button.on{border-color:var(--accent-line);box-shadow:inset 0 -2px 0 var(--accent)}
  .thumbs img{width:100%;height:100%;object-fit:cover}

  .chip{height:28px;padding:0 10px;border-radius:var(--r-sm);border:1px solid var(--border);font-size:11px;color:var(--ink-dim)}
  .grid{display:grid;gap:6px;align-items:stretch}
  .arch-head{font-size:10px;padding:0 2px}
  .os{display:flex;align-items:center;font-size:13px;color:var(--ink-dim)}
  .cell{min-height:52px;padding:8px 12px;display:flex;flex-wrap:wrap;align-items:center;gap:6px;border-radius:var(--r-sm);border:1px solid var(--border);background:var(--panel);text-align:left}
  .cell:hover:not(.off){border-color:var(--accent-line)}
  .cell.on{background:var(--accent-weak);border-color:var(--accent)}
  .cell.off{background:transparent;border-style:dashed;cursor:default}
  .unavailable{font-size:12px;color:var(--muted)}
  .fmt{height:20px;padding:0 6px;display:inline-flex;align-items:center;border:1px solid;border-radius:min(3px,var(--r-sm));font:700 11px var(--mono);letter-spacing:.6px}
  .fmt.big{flex:none;width:44px;height:24px;justify-content:center;padding:0}
  .files{display:flex;flex-direction:column;border-top:1px solid var(--border)}
  .file{display:flex;flex-wrap:wrap;align-items:center;gap:10px 18px;padding:14px 4px;border-bottom:1px solid var(--border-soft)}
  .fname{flex:1 1 220px;min-width:0;display:flex;flex-direction:column;gap:3px}
  .fname b{font-size:14px;font-weight:600;overflow-wrap:anywhere}
  .fname .mono{font-size:12px;color:var(--muted);overflow-wrap:anywhere}
  .arch{flex:none;width:96px}
  .fsize{flex:none;width:60px;text-align:right}
  .sha{flex:none;height:32px;padding:0 10px;border-radius:var(--r-sm);border:1px solid var(--border);font-size:11px;color:var(--ink-dim)}
  .dl{flex:none;height:36px;padding:0 16px;border-radius:var(--r-sm);background:var(--accent);border:1px solid var(--accent);color:var(--on-accent,#fff);font-size:13px;font-weight:700}

  .spec{margin:0;display:flex;flex-direction:column;gap:9px;font-size:13px}
  .spec div{display:flex;align-items:baseline;gap:6px}
  .spec dt{flex:none;color:var(--muted)}
  .spec i{flex:1;min-width:10px;border-bottom:1px dotted var(--border);transform:translateY(-4px)}
  .spec dd{margin:0;min-width:0;text-align:right;font-size:12px;overflow-wrap:anywhere}
  .timeline{margin:0;padding:0;list-style:none}
  .timeline li{display:flex;gap:14px}
  .dot{flex:none;margin-top:4px;width:11px;height:11px;box-sizing:border-box;border-radius:50%;background:var(--accent);border:1.5px solid var(--accent)}
  .tl-head{display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 10px;font-size:11px;color:var(--muted)}
  .tl-head b{font-size:13px;font-weight:600;color:var(--ink)}
  .tag{color:var(--link)}
  .note{margin:4px 0 0;font-size:13px;line-height:1.5;color:var(--ink-dim)}
  .related{display:flex;flex-direction:column}
  .rel{display:flex;align-items:center;gap:12px;padding:8px 6px;border-radius:var(--r-sm);text-align:left;color:var(--ink)}
  .rel:hover{background:var(--panel)}
  .rel b{flex:1;min-width:0;font-size:14px;font-weight:600}
  .rel-icon{width:36px;height:36px;flex:none;overflow:hidden;display:grid;place-items:center;border-radius:var(--r-md);background:var(--raise);font-weight:700}
  .rel-icon img{width:100%;height:100%;object-fit:contain}
</style>
