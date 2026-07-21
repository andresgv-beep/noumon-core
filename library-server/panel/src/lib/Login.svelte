<script>
  import { authRegister, authLogin } from './api.js'
  import { i18n, t, setLocale, LANGS } from './i18n.svelte.js'
  import { theme, toggleTheme } from './theme.svelte.js'

  let { setupNeeded = false, onDone } = $props()

  let username = $state('')
  let password = $state('')
  let confirm = $state('')
  let setupToken = $state('')
  let err = $state('')
  let busy = $state(false)

  async function submit(e) {
    e?.preventDefault()
    err = ''
    if (setupNeeded && password !== confirm) {
      err = t('login.mismatch')
      return
    }
    busy = true
    try {
      const r = setupNeeded
        ? await authRegister(username.trim(), password, 0, setupToken.trim()) // local: opcional; LAN: obligatorio
        : await authLogin(username.trim(), password)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      onDone?.()
    } catch (e2) {
      err = e2.message || 'no se pudo'
    } finally {
      busy = false
    }
  }
</script>

<div class="login-wrap">
  <form class="login-card" onsubmit={submit}>
    <div class="login-ic">
      <svg class="ic" viewBox="0 0 24 24" style="width:24px;height:24px">
        <path d="M4 5h6a2 2 0 012 2v12a3 3 0 00-3-3H4z" /><path d="M20 5h-6a2 2 0 00-2 2v12a3 3 0 013-3h5z" />
      </svg>
    </div>
    <h2>{setupNeeded ? t('login.setupTitle') : t('login.title')}</h2>
    <p class="login-sub">{setupNeeded ? t('login.setupSub') : t('login.sub')}</p>

    <label>{t('login.user')}<input bind:value={username} autocomplete="username" required /></label>
    <label>{t('login.password')}<input type="password" bind:value={password} autocomplete={setupNeeded ? 'new-password' : 'current-password'} required /></label>
    {#if setupNeeded}
      <label>{t('login.repeat')}<input type="password" bind:value={confirm} autocomplete="new-password" required /></label>
      <label>{t('login.setupCode')} <span>{t('login.setupCodeHint')}</span><input type="password" bind:value={setupToken} autocomplete="one-time-code" /></label>
    {/if}

    {#if err}<div class="login-err">{err}</div>{/if}
    <button class="btn btn-primary" type="submit" disabled={busy} style="justify-content:center;height:38px">
      {busy ? '…' : setupNeeded ? t('login.createEnter') : t('login.enter')}
    </button>

    <div class="login-langs">
      {#each LANGS as l (l.code)}
        <button type="button" class="lchip" class:on={i18n.locale === l.code} onclick={() => setLocale(l.code)}>{l.flag} {l.label}</button>
      {/each}
      <button type="button" class="lchip" title={theme.mode === 'dark' ? t('theme.toLight') : t('theme.toDark')} onclick={toggleTheme}>
        {theme.mode === 'dark' ? '☀' : '🌙'}
      </button>
    </div>
  </form>
</div>

<style>
  .login-wrap { flex: 1; display: flex; align-items: center; justify-content: center; padding: 24px; }
  .login-card { width: 100%; max-width: 340px; display: flex; flex-direction: column; gap: 12px; background: var(--canvas); border: 1px solid var(--line); border-radius: 6px; padding: 26px 24px; }
  .login-ic { width: 46px; height: 46px; border-radius: 6px; background: var(--window-bg); border: 1px solid var(--line-bright); color: var(--ink-dim); display: grid; place-items: center; }
  .login-card h2 { font-size: 17px; color: var(--ink); }
  .login-sub { font-size: 12px; color: var(--ink-mute); line-height: 1.5; margin-bottom: 4px; }
  .login-card label { display: flex; flex-direction: column; gap: 5px; font-size: 11px; letter-spacing: .06em; text-transform: uppercase; color: var(--ink-faint); }
  .login-card label span { font-size: 9px; letter-spacing: 0; text-transform: none; color: var(--ink-mute); }
  .login-card input { background: var(--window-bg); border: 1px solid var(--line-bright); border-radius: 4px; padding: 9px 11px; font-size: 13px; color: var(--ink); text-transform: none; letter-spacing: normal; }
  .login-card input:focus { border-color: var(--signal-border); }
  .login-err { font-size: 12px; color: var(--crit); background: var(--crit-dim); border: 1px solid var(--crit-border); border-radius: 4px; padding: 8px 10px; }
  .login-langs { display: flex; justify-content: center; gap: 6px; margin-top: 4px; }
  .lchip { padding: 4px 10px; border-radius: 4px; border: 1px solid var(--line); background: var(--window-bg); color: var(--ink-mute); font-size: 11.5px; font-weight: 600; }
  .lchip:hover { color: var(--ink); border-color: var(--line-bright); }
  .lchip.on { background: var(--sel); border-color: var(--sel-border); color: var(--ink); }
</style>
