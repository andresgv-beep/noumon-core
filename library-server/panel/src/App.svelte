<script>
  import { onMount } from 'svelte'
  import { authMe, authLogout, authLogoutAll, authRefresh, getHealth, getServiceStatus, restartLibraryServer } from './lib/api.js'
  import { i18n, t, setLocale, LANGS } from './lib/i18n.svelte.js'
  import Login from './lib/Login.svelte'
  import Storage from './lib/Storage.svelte'
  import Collections from './lib/Collections.svelte'
  import Translation from './lib/Translation.svelte'
  import Import from './lib/Import.svelte'
  import Users from './lib/Users.svelte'
  import Maps from './lib/Maps.svelte'
  import Network from './lib/Network.svelte'

  let tab = $state('storage')
  let health = $state({ shim: '…', engine: '…' })
  let me = $state(null) // {setupNeeded, user}
  let loading = $state(true)
  let service = $state({ supervised: false })
  let restarting = $state(false)
  let desktop = $state(false)

  const TABS = [
    { id: 'storage', key: 'nav.storage', icon: 'M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3' },
    { id: 'collections', key: 'nav.collections', icon: 'M4 5h5v14H4zM10 5h4v14h-4zM16 6l4 13' },
    { id: 'translation', key: 'nav.translation', icon: 'M4 5h8M8 3v2M6 5c0 4-1.5 7-4 9M5 9c1.5 3 4 4 6 4M13 20l4-9 4 9M14.5 17h5' },
    { id: 'maps', key: 'nav.maps', icon: 'M4 6l5-2 6 2 5-2v14l-5 2-6-2-5 2zM9 4v14M15 6v14' },
    { id: 'import', key: 'nav.import', icon: 'M12 4v11M8 11l4 4 4-4M5 20h14' },
    { id: 'users', key: 'nav.users', icon: 'M9 11a3.5 3.5 0 100-7 3.5 3.5 0 000 7zM3 20c0-3.3 2.7-5 6-5s6 1.7 6 5M17 8l2 2 3-3' },
    { id: 'network', key: 'nav.network', icon: 'M12 20h.01M8.5 16.5a5 5 0 017 0M5 13a10 10 0 0114 0M2 9.5a15 15 0 0120 0' },
  ]
  const tabKey = $derived((TABS.find((x) => x.id === tab) || TABS[0]).key)

  async function loadHealth() {
    health = await getHealth()
    if (health.shim === 'up') service = await getServiceStatus()
  }

  async function refreshAuth() {
    me = await authMe()
    if (me.user) await authRefresh()
    if (me.user?.isAdmin) loadHealth()
  }
  async function logout() { await authLogout(); await refreshAuth() }
  async function logoutAll() { await authLogoutAll(); await refreshAuth() }

  async function restartServer() {
    if (restarting || !confirm(t('head.confirmRestart'))) return
    restarting = true
    try {
      await restartLibraryServer()
      health = { shim: 'restarting', engine: 'restarting' }
      await new Promise((resolve) => setTimeout(resolve, 1200))
      for (let attempt = 0; attempt < 60; attempt++) {
        const next = await getHealth()
        if (next.shim === 'up') { location.reload(); return }
        await new Promise((resolve) => setTimeout(resolve, 1000))
      }
    } catch (e) {}
    restarting = false
  }

  onMount(() => {
    desktop = typeof window.runtime?.WindowMinimise === 'function'
    refreshAuth().finally(() => (loading = false))
    const timer = setInterval(() => { if (me?.user?.isAdmin && !restarting) loadHealth() }, 5000)
    return () => clearInterval(timer)
  })

  const isAdmin = $derived(me?.user?.isAdmin)

  // Estado humano del chip de salud; el detalle técnico va al tooltip.
  const status = $derived.by(() => {
    if (health.shim === 'restarting') return { key: 'head.statusRestarting', tone: 'warn' }
    if (health.shim === '…') return { key: 'head.statusWait', tone: 'warn' }
    if (health.shim !== 'up') return { key: 'head.statusDown', tone: 'down' }
    if (health.engine === 'up') return { key: 'head.statusOk', tone: 'ok' }
    if ((health.collections ?? 0) === 0) return { key: 'head.statusNoContent', tone: 'ok' }
    return { key: 'head.statusNoEngine', tone: 'warn' }
  })

  function minimiseWindow() { window.runtime?.WindowMinimise?.() }
  function toggleMaximiseWindow() { window.runtime?.WindowToggleMaximise?.() }
  function closeWindow() { window.runtime?.Quit?.() }
</script>

<div class="win">
  <div class="tbar" class:desktop>
    <span class="tic"><svg viewBox="0 0 24 24" style="width:15px;height:15px"><path d="M4 5h6a2 2 0 012 2v12a3 3 0 00-3-3H4z" /><path d="M20 5h-6a2 2 0 00-2 2v12a3 3 0 013-3h5z" /></svg></span>
    <b>Noumon</b><span class="sep">·</span>panel de control
    {#if isAdmin}<span class="sep">·</span><span style="color:var(--ink-dim)">{me.user.username}</span>{/if}
    {#if desktop}
      <div class="window-controls">
        <button class="window-button" title={t('win.minimise')} aria-label={t('win.minimise')} onclick={minimiseWindow}>
          <svg viewBox="0 0 10 10" aria-hidden="true"><line x1="1.5" y1="5" x2="8.5" y2="5" /></svg>
        </button>
        <button class="window-button" title={t('win.maximise')} aria-label={t('win.maximise')} onclick={toggleMaximiseWindow}>
          <svg viewBox="0 0 10 10" aria-hidden="true"><rect x="1.5" y="1.5" width="7" height="7" rx="1" /></svg>
        </button>
        <button class="window-button close" title={t('win.close')} aria-label={t('win.close')} onclick={closeWindow}>
          <svg viewBox="0 0 10 10" aria-hidden="true"><line x1="1.5" y1="1.5" x2="8.5" y2="8.5" /><line x1="8.5" y1="1.5" x2="1.5" y2="8.5" /></svg>
        </button>
      </div>
    {/if}
  </div>

  {#if loading}
    <div class="empty" style="flex:1;display:grid;place-items:center">{t('common.loading')}</div>
  {:else if me.setupNeeded}
    <Login setupNeeded onDone={refreshAuth} />
  {:else if !me.user}
    <Login onDone={refreshAuth} />
  {:else if !me.user.isAdmin}
    <div class="empty" style="flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px">
      <div class="big">{t('adminOnly.title')}</div>
      <div>{t('adminOnly.body', { user: me.user.username })}</div>
      <button class="btn" onclick={logout}>{t('head.logout')}</button>
    </div>
  {:else}
    <div class="shell">
      <aside class="side">
        <div class="slogo">
          <svg class="ic" viewBox="0 0 24 24"><path d="M4 5h6a2 2 0 012 2v12a3 3 0 00-3-3H4z" /><path d="M20 5h-6a2 2 0 00-2 2v12a3 3 0 013-3h5z" /></svg>
          Noumon
        </div>
        <nav class="snav">
          {#each TABS as item (item.id)}
            <button class="sitem" class:on={tab === item.id} onclick={() => (tab = item.id)}>
              <svg class="ic" viewBox="0 0 24 24"><path d={item.icon} /></svg>{t(item.key)}
            </button>
          {/each}
        </nav>
        <div class="sfoot">
          <div class="langs">
            {#each LANGS as l (l.code)}
              <button class="lchip" class:on={i18n.locale === l.code} title={l.label} onclick={() => setLocale(l.code)}>{l.flag} {l.code.toUpperCase()}</button>
            {/each}
          </div>
          Noumon<br />© 2026
        </div>
      </aside>

      <div class="main">
        <div class="mhead">
          <span class="crumb"><b>{t('head.crumb')}</b><span class="sep">›</span>{t(tabKey)}</span>
          <span class="grow"></span>
          <span class="hchip" title={t('head.statusDetail', { shim: health.shim, engine: health.engine })}>
            <span class="head-dot" class:off={status.tone === 'down'} class:warn={status.tone === 'warn'}></span>{t(status.key)}
          </span>
          {#if service.supervised}
            <button class="btn restart" disabled={restarting} onclick={restartServer}>{restarting ? t('head.restarting') : t('head.restart')}</button>
          {/if}
          <button class="btn" title={t('head.logoutAllTitle')} onclick={logoutAll}>{t('head.logoutAll')}</button>
          <button class="btn" onclick={logout}>{t('head.logout')}</button>
        </div>

        <div class="content scroll">
          <div class="section scroll">
            {#if tab === 'storage'}
              <Storage />
            {:else if tab === 'collections'}
              <Collections onImport={() => (tab = 'import')} />
            {:else if tab === 'translation'}
              <Translation />
            {:else if tab === 'maps'}
              <Maps />
            {:else if tab === 'import'}
              <Import />
            {:else if tab === 'users'}
              <Users me={me.user} />
            {:else if tab === 'network'}
              <Network />
            {/if}
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .tbar .tic { display: grid; place-items: center; color: var(--ink-dim); margin-right: 9px; }
  .tbar .tic svg { stroke: currentColor; stroke-width: 1.6; fill: none; stroke-linecap: round; stroke-linejoin: round; }
  .tbar.desktop { --wails-draggable: drag; }
  .tbar.desktop :is(button, .window-controls) { --wails-draggable: no-drag; }
  .window-controls { display: flex; align-items: center; gap: 2px; margin-left: auto; }
  .window-button { width: 34px; height: 30px; border-radius: 5px; display: grid; place-items: center; color: var(--ink-mute); transition: background .12s, color .12s; }
  .window-button svg { width: 10px; height: 10px; stroke: currentColor; stroke-width: 1.3; fill: none; stroke-linecap: round; }
  .window-button rect { stroke-width: 1.1; }
  .window-button:hover { background: var(--canvas-soft); color: var(--ink); }
  .window-button.close:hover { background: #e5484d; color: #fff; }

  .mhead .restart { color: var(--warn); border-color: var(--warn-border); }
  .mhead .btn { padding: 6px 12px; font-size: 12px; }

  .langs { display: flex; gap: 5px; margin-bottom: 9px; }
  .lchip { padding: 4px 9px; border-radius: 4px; border: 1px solid var(--line); background: var(--canvas); color: var(--ink-mute); font-size: 11px; font-weight: 600; }
  .lchip:hover { color: var(--ink); border-color: var(--line-bright); }
  .lchip.on { background: var(--sel); border-color: var(--sel-border); color: var(--ink); }
</style>
