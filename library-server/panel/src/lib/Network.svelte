<script>
  import { onMount } from 'svelte'
  import { getNetwork, setLanAccess } from './api.js'
  import { t } from './i18n.svelte.js'

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
      error = e.message || t('net.loadFail')
    } finally {
      loading = false
    }
  }
  onMount(load)

  async function toggle() {
    if (saving || !data) return
    const next = !data.lanAccess
    if (!next && !confirm(t('net.confirmUnpublish'))) return
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
      saveError = t('net.restartSlow')
    } catch (e) {
      saveError = e.message || t('net.toggleFail')
    } finally {
      saving = false
    }
  }
</script>

<div class="toolbar">
  <span class="cnt">{t('net.title')}</span>
  <span class="grow"></span>
  <button class="btn" onclick={load} disabled={loading}>↻ {t('net.refresh')}</button>
</div>

{#if data}
  <div class="setcard">
    <div class="net-title">
      <h4>{t('net.publishTitle')}</h4>
      <span class="badge {data.published ? 'b-signal' : 'b-mute'}">{data.published ? t('net.badgePublished') : t('net.badgeLocalOnly')}</span>
    </div>
    <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
      {#if data.published}
        {t('net.publishedDesc')}
      {:else}
        {t('net.localOnlyDesc')}
      {/if}
    </div>

    {#if data.configurable}
      <div class="net-actions">
        <button class="btn btn-primary" onclick={toggle} disabled={saving}>
          {#if saving}{t('net.applying')}{:else if data.lanAccess}{t('net.unpublish')}{:else}{t('net.publish')}{/if}
        </button>
        {#if saving}<span class="net-wait">{t('net.waitNote')}</span>{/if}
      </div>
    {:else}
      <div class="setrow" style="color:var(--ink-faint);font-size:11.5px">
        {t('net.bindNote')}
      </div>
    {/if}
    {#if saveError}<div class="net-error">{saveError}</div>{/if}
  </div>

  {#if data.published && (data.addresses || []).length}
    <div class="setcard">
      <h4>{t('net.addressesTitle')}</h4>
      <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
        {t('net.addressesHint')}
      </div>
      {#each data.addresses as address (address)}
        <div class="setrow"><code>{address}</code></div>
      {/each}
    </div>
  {/if}

  <div class="setcard">
    <h4>{t('net.troubleTitle')}</h4>
    <div class="setrow" style="color:var(--ink-faint);font-size:11.5px;padding-top:0">
      {t('net.troubleBody', { port: data.port })} <code>sudo ufw allow {data.port}/tcp</code>.
    </div>
  </div>
{:else if loading}
  <div class="empty">{t('net.reading')}</div>
{:else if error}
  <div class="empty"><div class="big">{t('net.loadFailTitle')}</div>{error}</div>
{/if}

<style>
  .net-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .net-title h4 { margin-bottom: 10px; }
  .net-actions { display: flex; align-items: center; gap: 12px; margin-top: 12px; }
  .net-wait { color: var(--ink-faint); font-size: 11.5px; }
  .net-error { margin-top: 10px; padding: 8px 10px; border: 1px solid var(--crit-border); border-radius: 7px; background: var(--crit-dim); color: var(--crit); font-size: 12px; }
</style>
