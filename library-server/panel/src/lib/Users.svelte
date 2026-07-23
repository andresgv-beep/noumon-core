<script>
  import { onMount } from 'svelte'
  import { listUsers, createUser, deleteUser, resetPassword, getStudioCapabilities, setStudioCapabilities } from './api.js'
  import { t } from './i18n.svelte.js'

  let { me } = $props()

  // Política de contraseñas (espejo de validatePassword en el servidor): 10
  // caracteres + al menos uno no alfanumérico. El servidor es la autoridad; esto
  // solo da feedback inmediato para no ir y volver al backend por un error obvio.
  const pwProblem = (pw) => {
    if ((pw || '').length < 10) return t('users.pwMin')
    if (!/[^\p{L}\p{N}]/u.test(pw)) return t('users.pwSpecial')
    return ''
  }
  // Generador de temporal que cumple la política (para el reset por olvido).
  const genTemp = () => {
    const set = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    const specials = '!@#$%&*?'
    let out = ''
    const rnd = (n) => Math.floor(Math.random() * n)
    for (let i = 0; i < 11; i++) out += set[rnd(set.length)]
    // Insertar un especial en posición aleatoria (garantiza la regla).
    const pos = rnd(out.length + 1)
    return out.slice(0, pos) + specials[rnd(specials.length)] + out.slice(pos)
  }

  // Estado del reset por fila.
  let resetId = $state(null)   // id del usuario cuyo form de reset está abierto
  let rpw = $state('')
  let rbusy = $state(false)
  let rflash = $state('')
  let rdone = $state('')       // muestra la temporal recién puesta (para copiarla)

  function openReset(u) {
    resetId = u.id; rpw = ''; rflash = ''; rdone = ''
  }
  function fillTemp() {
    rpw = genTemp(); rflash = ''
  }
  async function doReset(u) {
    rflash = ''
    const prob = pwProblem(rpw)
    if (prob) { rflash = prob; return }
    rbusy = true
    try {
      const r = await resetPassword(u.id, rpw)
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || t('users.failed'))
      rdone = rpw           // deja la clave a la vista para copiarla y pasársela al usuario
      rpw = ''
    } catch (e) { rflash = e.message } finally { rbusy = false }
  }

  let users = $state([])
  let loading = $state(true)
  let err = $state('')
  let flash = $state('')
  let studioCaps = $state({})
  let studioBusy = $state({})
  let deletePlan = $state(null)
  let deleteBusy = $state(false)

  // formulario de alta
  let show = $state(false)
  let nu = $state('')
  let np = $state('')
  let nc = $state('')
  let na = $state(18)
  let nadmin = $state(false)
  let creating = $state(false)

  async function load() {
    loading = true; err = ''
    try {
      users = await listUsers()
      const entries = await Promise.all(users.map(async (u) => {
        if (u.isAdmin) return [u.id, { canAuthor: true, canPublish: true }]
        try { return [u.id, await getStudioCapabilities(u.id)] }
        catch { return [u.id, { canAuthor: false, canPublish: false, quotaBytes: 2147483648 }] }
      }))
      studioCaps = Object.fromEntries(entries)
    } catch (e) { err = e.message } finally { loading = false }
  }
  onMount(load)

  async function add() {
    flash = ''
    const prob = pwProblem(np)
    if (prob) { flash = prob; return }
    if (np !== nc) { flash = t('users.pwMismatch'); return }
    creating = true
    try {
      const r = await createUser({ username: nu.trim(), password: np, age: nadmin ? 0 : Number(na), isAdmin: nadmin })
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || 'error')
      nu = ''; np = ''; nc = ''; na = 18; nadmin = false; show = false
      await load()
    } catch (e) { flash = e.message } finally { creating = false }
  }

  async function del(u) {
    if (!confirm(t('users.confirmDelete', { name: u.username }))) return
    const r = await deleteUser(u.id)
    const body = await r.json().catch(() => ({}))
    if (r.status === 409 && body.errorCode === 'users.studio_strategy_required') {
      const targets = eligibleTransferTargets(u)
      deletePlan = {
        user: u,
        details: body.details || {},
        strategy: 'custody',
        transferTo: targets[0]?.id || '',
      }
      return
    }
    if (!r.ok) { flash = body.error || t(body.errorCode || 'users.failed'); return }
    await load()
  }

  function eligibleTransferTargets(source) {
    return users.filter((candidate) =>
      candidate.id !== source.id &&
      (candidate.isAdmin || !!studioCaps[candidate.id]?.canAuthor)
    )
  }

  async function confirmStudioDelete() {
    if (!deletePlan || deleteBusy) return
    if (deletePlan.strategy === 'transfer' && !deletePlan.transferTo) {
      flash = t('users.studioTransferTargetRequired')
      return
    }
    if (deletePlan.strategy === 'withdraw' &&
      !confirm(t('users.studioWithdrawConfirm', { name: deletePlan.user.username }))) return
    deleteBusy = true
    flash = ''
    try {
      const response = await deleteUser(deletePlan.user.id, {
        studioStrategy: deletePlan.strategy,
        transferTo: deletePlan.strategy === 'transfer' ? deletePlan.transferTo : '',
      })
      const body = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(t(body.errorCode || 'users.failed'))
      deletePlan = null
      await load()
    } catch (e) {
      flash = e.message
    } finally {
      deleteBusy = false
    }
  }

  const initials = (n) => (n || '?').slice(0, 2).toUpperCase()

  async function changeStudio(u, field, value) {
    if (u.isAdmin || studioBusy[u.id]) return
    const current = studioCaps[u.id] || { canAuthor: false, canPublish: false, quotaBytes: 2147483648 }
    const next = { ...current, [field]: value }
    if (field === 'canAuthor' && !value) next.canPublish = false
    if (field === 'canPublish' && value) next.canAuthor = true
    studioBusy = { ...studioBusy, [u.id]: true }
    try {
      const response = await setStudioCapabilities(u.id, next)
      if (!response.ok) throw new Error((await response.json().catch(() => ({}))).errorCode || t('users.failed'))
      studioCaps = { ...studioCaps, [u.id]: await response.json() }
    } catch (e) {
      flash = e.message
    } finally {
      studioBusy = { ...studioBusy, [u.id]: false }
    }
  }
</script>

<div class="toolbar">
  <span class="cnt"><b>{users.length}</b> {t('users.count')}</span>
  <span class="grow"></span>
  {#if flash}<span style="font-size:12px;color:var(--crit)">{flash}</span>{/if}
  <button class="btn btn-primary" onclick={() => (show = !show)}>{show ? t('users.close') : t('users.addUser')}</button>
</div>

{#if show}
  <div class="setcard">
    <h4>{t('users.newAccount')}</h4>
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:10px">
      <input class="uinput" placeholder={t('users.username')} bind:value={nu} />
      {#if nadmin}
        <div style="align-self:center;font-size:11.5px;color:var(--ink-faint)">{t('users.adminNote')}</div>
      {:else}
        <input class="uinput" type="number" min="0" max="120" placeholder={t('users.age')} bind:value={na} />
      {/if}
      <input class="uinput" type="password" placeholder={t('users.password')} bind:value={np} />
      <input class="uinput" type="password" placeholder={t('users.repeatPassword')} bind:value={nc} />
    </div>
    <div class="pwhint" style="margin-bottom:10px">{t('users.pwHint')}</div>
    <div class="setrow" style="padding-top:0">
      <label style="display:flex;align-items:center;gap:8px;font-size:12.5px;color:var(--ink-dim);cursor:pointer">
        <input type="checkbox" bind:checked={nadmin} /> {t('users.adminCheck')}
      </label>
      <button class="btn btn-primary" disabled={creating || !nu || !np || !nc} onclick={add}>{creating ? '…' : t('users.create')}</button>
    </div>
  </div>
{/if}

{#if loading}
  <div class="empty">{t('users.reading')}</div>
{:else if err}
  <div class="empty"><div class="big">{t('users.loadFail')}</div>{err}</div>
{:else}
  {#each users as u (u.id)}
    <div class="row" style="grid-template-columns:40px 1fr auto">
      <div class="cic" style="background:{u.isAdmin ? 'var(--signal)' : 'var(--info-dim)'};color:{u.isAdmin ? '#0a0a0c' : 'var(--info)'}">{initials(u.username)}</div>
      <div style="min-width:0">
        <div class="cname">
          {u.username}
          {#if u.isAdmin}<span class="badge b-signal">{t('users.badgeAdmin')}</span>{:else}<span class="badge b-mute">{t('users.badgeAge', { age: u.age })}</span>{/if}
          {#if me && u.id === me.id}<span class="badge b-info">{t('users.badgeYou')}</span>{/if}
        </div>
        <div class="cpath">{u.isAdmin ? t('users.adminDesc') : t('users.ageDesc', { age: u.age })}</div>
        <div class="studio-perms">
          <span>{t('users.studio')}</span>
          <label>
            <input type="checkbox" checked={u.isAdmin || !!studioCaps[u.id]?.canAuthor}
              disabled={u.isAdmin || studioBusy[u.id]}
              onchange={(e) => changeStudio(u, 'canAuthor', e.currentTarget.checked)} />
            {t('users.canAuthor')}
          </label>
          <label>
            <input type="checkbox" checked={u.isAdmin || !!studioCaps[u.id]?.canPublish}
              disabled={u.isAdmin || studioBusy[u.id]}
              onchange={(e) => changeStudio(u, 'canPublish', e.currentTarget.checked)} />
            {t('users.canPublish')}
          </label>
        </div>
      </div>
      <div style="display:flex;gap:6px">
        <button class="btn" onclick={() => (resetId === u.id ? (resetId = null) : openReset(u))}>
          {resetId === u.id ? t('users.close') : t('users.reset')}
        </button>
        {#if !me || u.id !== me.id}
          <button class="btn" onclick={() => del(u)}>{t('users.delete')}</button>
        {/if}
      </div>
    </div>

    {#if deletePlan?.user.id === u.id}
      <div class="setcard ownership-card" style="margin:-4px 0 8px">
        <h4>{t('users.studioDeleteTitle', { name: u.username })}</h4>
        <p class="tmphint">
          {t('users.studioDeleteImpact', {
            documents: deletePlan.details.documents || 0,
            published: deletePlan.details.published || 0,
            assets: deletePlan.details.assets || 0,
          })}
        </p>
        <label class="ownership-option">
          <input type="radio" bind:group={deletePlan.strategy} value="custody" />
          <span><b>{t('users.studioCustody')}</b><small>{t('users.studioCustodyDesc')}</small></span>
        </label>
        <label class="ownership-option">
          <input type="radio" bind:group={deletePlan.strategy} value="transfer" />
          <span><b>{t('users.studioTransfer')}</b><small>{t('users.studioTransferDesc')}</small></span>
        </label>
        {#if deletePlan.strategy === 'transfer'}
          <select class="uinput transfer-target" bind:value={deletePlan.transferTo}>
            <option value="">{t('users.studioChooseTarget')}</option>
            {#each eligibleTransferTargets(u) as candidate}
              <option value={candidate.id}>{candidate.username}</option>
            {/each}
          </select>
        {/if}
        <label class="ownership-option danger-option">
          <input type="radio" bind:group={deletePlan.strategy} value="withdraw" />
          <span><b>{t('users.studioWithdraw')}</b><small>{t('users.studioWithdrawDesc')}</small></span>
        </label>
        <div class="setrow ownership-actions">
          <span class="grow"></span>
          <button class="btn" onclick={() => (deletePlan = null)} disabled={deleteBusy}>{t('common.cancel')}</button>
          <button class="btn btn-primary" onclick={confirmStudioDelete} disabled={deleteBusy}>
            {deleteBusy ? '…' : t('users.studioResolveDelete')}
          </button>
        </div>
      </div>
    {/if}

    {#if resetId === u.id}
      <div class="setcard" style="margin:-4px 0 8px">
        {#if rdone}
          <h4>{t('users.tempSetTitle')}</h4>
          <p class="tmphint">{t('users.tempGiveTo')} <b>{u.username}</b>. {t('users.tempCanChange')}</p>
          <div class="tmpbox">
            <code>{rdone}</code>
            <button class="btn" onclick={() => navigator.clipboard?.writeText(rdone)}>{t('users.copy')}</button>
          </div>
          <div class="setrow" style="padding-top:8px">
            <span class="grow"></span>
            <button class="btn btn-primary" onclick={() => { resetId = null; rdone = '' }}>{t('users.done')}</button>
          </div>
        {:else}
          <h4>{t('users.resetTitle', { name: u.username })}</h4>
          <p class="tmphint">{t('users.resetHint')}</p>
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px">
            <input class="uinput" style="flex:1" type="text" placeholder={t('users.tempPlaceholder')} bind:value={rpw} />
            <button class="btn" onclick={fillTemp}>{t('users.generate')}</button>
          </div>
          <div class="pwhint">{t('users.pwHintShort')}</div>
          <div class="setrow" style="padding-top:8px">
            {#if rflash}<span style="font-size:12px;color:var(--crit)">{rflash}</span>{/if}
            <span class="grow"></span>
            <button class="btn btn-primary" disabled={rbusy || !rpw} onclick={() => doReset(u)}>{rbusy ? '…' : t('users.reset')}</button>
          </div>
        {/if}
      </div>
    {/if}
  {/each}
  <div class="empty" style="padding:16px 24px;font-size:11.5px">
    {t('users.footFirst')} <b>{t('users.footAgeWord')}</b> {t('users.footRest')}
  </div>
{/if}

<style>
  .uinput { background: var(--window-bg); border: 1px solid var(--line-bright); border-radius: 7px; padding: 8px 10px; font-size: 13px; color: var(--ink); }
  .uinput:focus { border-color: var(--signal-border); }
  .pwhint { font-size: 11px; color: var(--ink-faint); line-height: 1.4; }
  .tmphint { font-size: 12px; color: var(--ink-dim); line-height: 1.5; margin-bottom: 8px; }
  .tmpbox { display: flex; align-items: center; gap: 8px; }
  .tmpbox code { flex: 1; background: var(--window-bg); border: 1px solid var(--line-bright); border-radius: 7px; padding: 8px 10px; font-size: 13px; color: var(--signal); letter-spacing: .02em; user-select: all; }
  .studio-perms{display:flex;align-items:center;gap:10px;margin-top:7px;font-size:11px;color:var(--ink-faint)}
  .studio-perms>span{font-weight:650;color:var(--ink-dim)}
  .studio-perms label{display:flex;align-items:center;gap:5px;cursor:pointer}
  .ownership-card{border-color:var(--warn-border)}
  .ownership-option{display:flex;align-items:flex-start;gap:9px;padding:9px;border:1px solid var(--line);border-radius:7px;margin-top:7px;cursor:pointer}
  .ownership-option:has(input:checked){border-color:var(--signal-border);background:var(--signal-dim)}
  .ownership-option input{margin-top:3px}
  .ownership-option span{display:flex;flex-direction:column;gap:2px;font-size:12px;color:var(--ink)}
  .ownership-option small{font-size:11px;color:var(--ink-faint);line-height:1.4}
  .danger-option:has(input:checked){border-color:var(--crit);background:var(--crit-dim)}
  .transfer-target{width:100%;margin-top:7px}
  .ownership-actions{padding-top:10px}
</style>
