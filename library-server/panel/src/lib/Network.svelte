<script>
  import { onMount } from 'svelte'
  import { getNetwork, setLanAccess } from './api.js'

  let data = $state(null)
  let error = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let saveError = $state('')

  async function load() {
    loading = true; error = ''
    try {
      data = await getNetwork()
    } catch (e) {
      error = e.message || 'no se pudo leer el estado de red'
    } finally {
      loading = false
    }
  }
  onMount(load)

  async function toggle() {
    if (saving || !data) return
    const next = !data.lanAccess
    if (!next && !confirm('Los demás equipos de la red perderán el acceso a la biblioteca. ¿Continuar?')) return
    saving = true; saveError = ''
    try {
      await setLanAccess(next)
      // El servidor se reinicia para aplicar el cambio: esperar a que vuelva.
      await new Promise((resolve) => setTimeout(resolve, 1200))
      for (let attempt = 0; attempt < 60; attempt++) {
        try {
          const fresh = await getNetwork()
          data = fresh
          saving = false
          return
        } catch (e) {}
        await new Promise((resolve) => setTimeout(resolve, 1000))
      }
      saveError = 'El servidor tarda más de lo esperado en reiniciarse.'
    } catch (e) {
      saveError = e.message || 'No se pudo cambiar el acceso.'
    } finally {
      saving = false
    }
  }
</script>

<div class="toolbar">
  <span class="cnt">Acceso en red local</span>
  <span class="grow"></span>
  <button class="btn" onclick={load} disabled={loading}>↻ Actualizar</button>
</div>

{#if data}
  <div class="setcard">
    <div class="net-title">
      <h4>Publicar en la red local</h4>
      <span class="badge {data.published ? 'b-signal' : 'b-mute'}">{data.published ? 'publicado' : 'solo este equipo'}</span>
    </div>
    <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
      {#if data.published}
        La biblioteca es visible para los demás dispositivos de tu red (casa o aula).
      {:else}
        Ahora mismo la biblioteca solo se puede abrir desde este equipo.
      {/if}
    </div>

    {#if data.configurable}
      <div class="net-actions">
        <button class="btn btn-primary" onclick={toggle} disabled={saving}>
          {#if saving}Aplicando…{:else if data.lanAccess}Dejar de publicar{:else}Publicar en la red local{/if}
        </button>
        {#if saving}<span class="net-wait">El servidor se reinicia; los lectores conectados reintentarán solos.</span>{/if}
      </div>
    {:else}
      <div class="setrow" style="color:var(--ink-faint);font-size:11.5px">
        La dirección de escucha está fijada por el entorno del servidor (BIND); se cambia allí.
      </div>
    {/if}
    {#if saveError}<div class="net-error">{saveError}</div>{/if}
  </div>

  {#if data.published && (data.addresses || []).length}
    <div class="setcard">
      <h4>Direcciones para los demás equipos</h4>
      <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
        Escribe una de estas direcciones en el navegador de otro dispositivo, o en la app Noumon al conectar con el servidor.
      </div>
      {#each data.addresses as address (address)}
        <div class="setrow"><code>{address}</code></div>
      {/each}
    </div>
  {/if}

  <div class="setcard">
    <h4>Si otro equipo no conecta</h4>
    <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
      Comprueba que ambos están en la misma red y que el cortafuegos del servidor
      permite el puerto {data.port}. En Linux con ufw: <code>sudo ufw allow {data.port}/tcp</code>.
    </div>
  </div>
{:else if loading}
  <div class="empty">Leyendo el estado de red…</div>
{:else if error}
  <div class="empty"><div class="big">No se pudo leer el estado de red</div>{error}</div>
{/if}

<style>
  .net-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .net-title h4 { margin-bottom: 10px; }
  .net-actions { display: flex; align-items: center; gap: 12px; margin-top: 12px; }
  .net-wait { color: var(--ink-faint); font-size: 11.5px; }
  .net-error { margin-top: 10px; padding: 8px 10px; border: 1px solid var(--crit-border); border-radius: 7px; background: var(--crit-dim); color: var(--crit); font-size: 12px; }
</style>
