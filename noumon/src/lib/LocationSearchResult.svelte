<script>
  import Icon from './Icon.svelte';
  import MiniMap from './MiniMap.svelte';
  import { t } from './i18n.svelte.js';

  let { locationState, onRadiusChange, onOpenMap } = $props();
  let radiusKm = $state(2.5);
  let selectedPoi = $state(null);
  let radiusTimer;
  let lastRequested = 2500;

  let result = $derived(locationState?.result);
  // Diez, en dos columnas de cinco. El servidor manda hasta 18, asi que quedarse
  // en seis era recorte nuestro y dejaba media columna vacia.
  const POI_ROWS = 5;
  let visiblePois = $derived((result?.pois || [])
    .filter((p) => p.distance <= radiusKm * 1000)
    .slice(0, POI_ROWS * 2));
  let mapResult = $derived(result ? { ...result, radius: Math.round(radiusKm * 1000) } : null);

  $effect(() => {
    if (locationState?.radius == null) return;
    radiusKm = locationState.radius / 1000;
    lastRequested = locationState.radius;
  });

  function radiusLabel(value) {
    return value === 0 ? '0 km' : `${String(value).replace('.', ',')} km`;
  }
  function distanceLabel(meters) {
    return meters < 1000 ? `${meters} m` : `${(meters / 1000).toFixed(1).replace('.', ',')} km`;
  }
  // Icono propio por categoría, con el alfiler de siempre para lo que no encaje
  // en ninguna: mejor un alfiler que un hueco.
  const POI_ICONS = new Set([
    'restaurant', 'cafe', 'bar', 'fuel', 'shop', 'health',
    'lodging', 'parking', 'bank', 'transport', 'culture', 'park',
  ]);
  function poiIcon(poi) {
    return POI_ICONS.has(poi.categoryCode) ? `poi-${poi.categoryCode}` : 'pin';
  }
  // Abre Maps en una pestaña nueva, centrado aquí y con la marca puesta. El
  // nombre y la categoría viajan para que el globo diga qué es; si no, llegas a
  // un alfiler sin explicación.
  function openInMaps(point, categoryCode) {
    if (!point) return;
    onOpenMap?.({
      lat: point.lat,
      lon: point.lon,
      name: point.name || '',
      categoryCode: categoryCode ?? point.categoryCode ?? '',
    });
  }
  function categoryLabel(poi) {
    const key = `home.map.category.${poi.categoryCode || 'other'}`;
    const translated = t(key);
    return translated === key ? poi.category : translated;
  }
  function requestRadius(value) {
    const meters = Math.round(Number(value) * 1000);
    if (meters === lastRequested) return;
    lastRequested = meters;
    onRadiusChange?.(meters);
  }
  function onRadiusInput(event) {
    radiusKm = Number(event.currentTarget.value);
    clearTimeout(radiusTimer);
    radiusTimer = setTimeout(() => requestRadius(radiusKm), 180);
  }
  function onRadiusCommit() {
    clearTimeout(radiusTimer);
    requestRadius(radiusKm);
  }
</script>

{#if result?.location && result?.map}
  <section class="geo" aria-labelledby="geo-title">
    <div class="geo-copy">
      <div class="geo-kind"><Icon name="pin" size={15} /> {t('home.map.location')}</div>
      <h2 id="geo-title">{result.location.name}{result.location.houseNumber ? ` ${result.location.houseNumber}` : ''}</h2>
      {#if result.location.context}<p>{result.location.context}</p>{/if}
      {#if result.location.approximate}<span class="approx">{t('home.map.approximate')}</span>{/if}
      <!-- El minimapa es una vista fija: para moverse por él o mirar alrededor
           hace falta Maps, y hasta ahora había que abrirlo y buscar otra vez. -->
      {#if onOpenMap}
        <button class="open-map" onclick={() => openInMaps(result.location, '')}>
          <Icon name="map" size={14} />{t('home.map.openInMaps')}
        </button>
      {/if}
      <label class="radius-label" for="library-map-radius">
        <span>{t('home.map.nearby')} <output for="library-map-radius">{radiusLabel(radiusKm)}</output></span>
        <input id="library-map-radius" type="range" min="0" max="5" step="0.5" value={radiusKm}
          oninput={onRadiusInput} onchange={onRadiusCommit} />
        <span class="range-ends"><span>0 km</span><span>5 km</span></span>
      </label>

      <!-- Los lugares acompañan al mapa en vez de ir debajo a todo lo ancho. Eran
           dos bloques apilados y el mapa tenía que estirarse a lo largo para
           llenar su fila; en columna, el mapa crece de alto y deja de ser una
           franja. Además la lista y el punto que la representa se miran a la vez. -->
      <div class="nearby">
        <div class="nearby-head">
          <h3 id="nearby-title">{t('home.map.places')}</h3>
          <span>{#if locationState.status === 'loading'}{t('home.map.updating')}{:else}{t('home.map.placeCount', { n: visiblePois.length, radius: radiusLabel(radiusKm) })}{/if}</span>
        </div>
        {#if radiusKm === 0}
          <p class="nearby-empty">{t('home.map.zeroRadius')}</p>
        {:else if visiblePois.length}
          <div class="poi-grid">
            {#each visiblePois as poi}
              <!-- Dos acciones distintas y por eso dos botones hermanos, no uno
                   dentro de otro: pulsar la fila lo señala en el minimapa de al
                   lado, y el icono lo abre en Maps para poder moverse por él. -->
              <div class="poi-row" class:selected={selectedPoi === poi}>
                <button class="poi" onclick={() => selectedPoi = poi} aria-pressed={selectedPoi === poi}>
                  <span class="poi-icon"><Icon name={poiIcon(poi)} size={15} /></span>
                  <!-- El nombre se recorta con puntos suspensivos si no cabe; el
                       completo queda al alcance del ratón en vez de perderse. -->
                  <span class="poi-copy"><b title={poi.name}>{poi.name}</b><small>{categoryLabel(poi)} · {distanceLabel(poi.distance)}</small></span>
                </button>
                {#if onOpenMap}
                  <button
                    class="poi-open"
                    title={t('home.map.openInMaps')}
                    aria-label={t('home.map.openPlaceInMaps', { name: poi.name })}
                    onclick={() => openInMaps(poi)}
                  ><Icon name="map" size={14} /></button>
                {/if}
              </div>
            {/each}
          </div>
        {:else if locationState.status === 'loading'}
          <p class="nearby-empty">{t('home.map.updating')}</p>
        {:else}
          <p class="nearby-empty">{t('home.map.noPlaces')}</p>
        {/if}
        {#if selectedPoi}
          <p class="selected-info"><Icon name="map" size={14} /> {t('home.map.selected', { name: selectedPoi.name, distance: distanceLabel(selectedPoi.distance) })}</p>
        {/if}
      </div>
    </div>
    <div class="map-wrap">
      <MiniMap result={mapResult} {selectedPoi} />
    </div>
  </section>
{/if}

<style>
  /* Dos columnas de verdad: antes el mapa iba en posición absoluta sobre la
     sección, así que su alto lo fijaba un min-height y salía siempre apaisado
     pasara lo que pasara al lado. Como celda de la rejilla crece con la columna
     de texto y se acerca al cuadrado solo. */
  .geo{width:calc(100% - 80px);max-width:1300px;margin:14px auto 0;display:grid;grid-template-columns:minmax(290px,1fr) minmax(0,1.05fr);gap:clamp(20px,2.6vw,40px);align-items:stretch}
  .geo-copy{min-width:0;padding:20px 0 22px 54px}
  .geo-kind{display:flex;align-items:center;gap:7px;color:var(--accent-2);font-size:11px;font-weight:650;letter-spacing:.8px;text-transform:uppercase;margin-bottom:8px}
  h2{font-size:26px;line-height:1.2;font-weight:650;color:var(--ink);letter-spacing:-.3px}
  .geo-copy p{color:var(--muted);font-size:13.5px;margin-top:6px}
  .approx{display:inline-block;margin-top:8px;padding:4px 8px;border-radius:var(--r-sm);background:color-mix(in srgb,var(--accent) 12%,transparent);color:var(--accent-2);font-size:10.5px}
  .open-map{display:inline-flex;align-items:center;gap:7px;margin-top:14px;padding:7px 11px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--card);color:var(--ink-dim);font-size:12px}
  .open-map:hover{border-color:var(--accent-line);color:var(--accent-2)}
  .radius-label{display:block;max-width:260px;margin-top:24px;color:var(--ink-dim);font-size:12.5px}
  .radius-label>span:first-child{display:flex;align-items:baseline;justify-content:space-between;gap:12px}
  .radius-label output{color:var(--ink);font-weight:600}
  .radius-label input{width:100%;accent-color:var(--accent);cursor:pointer;margin-top:9px}
  .range-ends{display:flex;justify-content:space-between;color:var(--faint);font-size:10.5px;margin-top:1px}
  /* Sin el degradado de los cuatro bordes. Se comía diez píxeles de mapa por
     cada lado y lo dejaba deshaciéndose en la página en vez de parecer un mapa;
     ahora que es una celda con su sitio, el borde limpio dice lo mismo mejor y
     con el mismo lenguaje que las tarjetas del resto de la aplicación. */
  /* Forma propia, no la que le imponga la lista de al lado. Estirándolo a lo alto
     de la fila volvía a deformarse: apaisado cuando la columna era corta y
     estrecho y alto cuando la lista crecía. Con proporción fija siempre se lee
     como un mapa. */
  .map-wrap{position:relative;align-self:start;aspect-ratio:4/3;min-height:300px;overflow:hidden;border-radius:var(--r-lg);border:1px solid var(--border)}
  .nearby{margin-top:24px}
  .nearby-head{display:flex;align-items:baseline;justify-content:space-between;gap:14px;margin-bottom:8px}
  .nearby-head h3{font-size:14px;font-weight:650;color:var(--ink)}
  .nearby-head span{font-size:11px;color:var(--faint)}
  /* Cinco por columna y salta a la de al lado. En flujo por columnas y no por
     filas porque la lista va ordenada por distancia: así se lee hacia abajo,
     que es como se lee una lista, en vez de en zigzag. */
  .poi-grid{display:grid;grid-auto-flow:column;grid-template-rows:repeat(5,auto);grid-auto-columns:minmax(0,1fr);gap:2px 12px}
  .poi-row{min-width:0;display:flex;align-items:center;border-radius:var(--r-md);transition:background .12s}
  .poi-row:hover,.poi-row.selected{background:var(--card)}
  .poi{flex:1;min-width:0;display:flex;align-items:center;gap:9px;text-align:left;padding:8px}
  /* Presente pero callado: diez filas con su botón a plena luz serían diez
     llamadas de atención compitiendo con los nombres. Se enciende al pasar por
     la fila y al llegar con el teclado. */
  .poi-open{flex:none;display:grid;place-items:center;width:30px;height:30px;margin-right:4px;border-radius:var(--r-sm);color:var(--faint);opacity:.35;transition:opacity .12s,color .12s}
  .poi-row:hover .poi-open{opacity:1}
  .poi-open:hover,.poi-open:focus-visible{opacity:1;color:var(--accent-2);background:var(--raise)}
  .poi-icon{display:grid;place-items:center;width:31px;height:31px;flex:none;border-radius:var(--r-md);background:color-mix(in srgb,var(--accent) 12%,transparent);color:var(--accent-2)}
  .poi-copy{min-width:0;display:flex;flex-direction:column}
  .poi-copy b,.poi-copy small{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .poi-copy b{font-size:12.5px;font-weight:600;color:var(--ink-dim)}
  .poi-copy small{font-size:10.5px;color:var(--muted)}
  .nearby-empty{color:var(--muted);font-size:12.5px;padding:9px 0}
  .selected-info{display:flex;align-items:center;gap:6px;color:var(--muted);font-size:11px;padding:8px 4px 0}
  /* La lista vuelve a una columna antes que el resto: medido, a 1010px de ancho
     las dos columnas quedan en 185px y nombres como "Mad Mad Vegan - Barcelona"
     se cortan. La banda de dos columnas sigue teniendo sentido hasta 980. */
  @media(max-width:1200px){
    .poi-grid{grid-auto-flow:row;grid-template-rows:none;grid-template-columns:minmax(0,1fr)}
  }
  /* Dos columnas solo cuando hay sitio: por debajo de esto quedan dos columnas
     flacas, el mapa se queda diminuto y los nombres de los sitios se parten. */
  @media(max-width:980px){
    /* Apilado, y el mapa primero: es el ancla visual, y detrás de una lista de
       seis sitios habría que buscarlo. */
    .geo{width:100%;grid-template-columns:1fr;gap:18px}
    .geo-copy{padding:0 24px 18px}
    /* Alto fijo en vez de proporción: a todo lo ancho, 4:3 son seiscientos y
       pico píxeles de mapa y el resto de la búsqueda queda fuera de pantalla. */
    .map-wrap{order:-1;aspect-ratio:auto;height:250px;min-height:0;border-radius:0;border-left:0;border-right:0}
    /* A una columna: en paralelo, con esta anchura, los nombres se parten. */
    .poi-grid{grid-auto-flow:row;grid-template-rows:none;grid-template-columns:minmax(0,1fr)}
  }
</style>
