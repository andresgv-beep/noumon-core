<script>
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte.js';
  import { profile, profileInitials, profileGradient } from './profile.svelte.js';

  let {
    state = {},
    sidebarOpen = true,
    user = null,
    onToggleSidebar,
    onAccount,
  } = $props();

  const editing = () => state.mode === 'editor' || state.mode === 'preview';

  function chooseInfoCard(event, cardId) {
    state.selectInfoCard?.(cardId, true);
    event.currentTarget.closest('details')?.removeAttribute('open');
  }

  function createInfoCard(event) {
    state.addInfoCard?.();
    event.currentTarget.closest('details')?.removeAttribute('open');
  }
</script>

<div class="studio-nav">
  <button
    class="nav-icon"
    title={sidebarOpen ? t('nav.hideLibrary') : t('nav.showLibrary')}
    aria-label={sidebarOpen ? t('nav.hideLibrary') : t('nav.showLibrary')}
    onclick={() => onToggleSidebar?.()}
  ><Icon name="panel" /></button>

  {#if editing()}
    <button class="back" onclick={() => state.goHome?.()}>
      <Icon name="back" size={15} />
      <span>{t('studio.backHome')}</span>
    </button>
  {/if}

  <div class="identity">
    <strong>{editing() ? (state.title || t('studio.untitled')) : t('studio.title')}</strong>
    {#if editing()}
      <span class="save-state" data-state={state.saveState || 'saved'}>
        <i></i>
        <span>{state.saveLabel || t('studio.saved')}</span>
      </span>
    {/if}
  </div>

  {#if state.mode === 'editor' && state.sections?.some((section) => section.key === 'cards')}
    <details class="card-jump">
      <summary title={t('studio.infoCards')}>
        <Icon name="list" size={14} />
        <span>{t('studio.infoCards')}</span>
        <b>{state.infoCards?.length || 0}</b>
        <Icon name="chevron" size={12} />
      </summary>
      <div class="card-menu">
        <header>
          <b>{t('studio.infoCards')}</b>
          <small>{t('studio.infoCardsDesc')}</small>
        </header>
        {#each state.infoCards || [] as card, index (card.id)}
          <button
            class:active={state.selectedInfoCardID === card.id}
            onclick={(event) => chooseInfoCard(event, card.id)}
          >
            <span>{index + 1}</span>
            <b>{t('studio.infoCardNumber', { number: index + 1 })}</b>
            <small>{t(`studio.infoCardSide.${card.side || 'right'}`)}</small>
          </button>
        {/each}
        {#if !state.infoCards?.length}
          <p>{t('studio.infoCardsEmpty')}</p>
        {/if}
        {#if (state.infoCards?.length || 0) < 4}
          <button class="new-card" onclick={createInfoCard}>
            <Icon name="plus" size={13} />{t('studio.infoCardAdd')}
          </button>
        {/if}
      </div>
    </details>
  {/if}

  {#if state.mode === 'editor' && state.textControl}
    <div class="text-context" aria-label={t('studio.textFormatting')}>
      <button
        title={t('studio.textSmaller')}
        aria-label={t('studio.textSmaller')}
        onclick={() => state.setTextSize?.(state.textControl.size - 1)}
      >−</button>
      <label>
        <input
          type="number"
          min="10"
          max="96"
          value={state.textControl.size}
          aria-label={t('studio.textSize')}
          onchange={(event) => state.setTextSize?.(event.currentTarget.value)}
        />
        <span>px</span>
      </label>
      <button
        title={t('studio.textLarger')}
        aria-label={t('studio.textLarger')}
        onclick={() => state.setTextSize?.(state.textControl.size + 1)}
      >+</button>
      {#if state.textControl.canAlign}
        <i></i>
        {#each ['left', 'center', 'right'] as align}
          <button
            class="align"
            class:active={state.textControl.align === align}
            title={t(`studio.textAlign.${align}`)}
            aria-label={t(`studio.textAlign.${align}`)}
            onclick={() => state.setTextAlign?.(align)}
          >
            <span class={`align-lines ${align}`}><b></b><b></b><b></b></span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}

  {#if state.mode === 'editor' && state.tools?.length}
    <div class="context-tools" aria-label={t('studio.contextTools')}>
      {#each state.tools as tool (tool.key)}
        <button
          title={tool.label}
          aria-label={tool.label}
          onclick={() => state.runTool?.(tool.key)}
        >{tool.short}</button>
      {/each}
    </div>
  {/if}

  {#if editing()}
    <button class="action ghost" onclick={() => state.togglePreview?.()}>
      {state.mode === 'preview' ? t('studio.edit') : t('studio.preview')}
    </button>
    <button
      class="action primary"
      disabled={!state.canPublish || state.publishDisabled}
      title={state.publishDisabled
        ? (state.publishDisabledReason || t('studio.publishUnavailable'))
        : (state.publishHint || '')}
      onclick={() => state.publish?.()}
    >{state.publishLabel || t('studio.publish')}</button>
  {/if}

  <button class="account" title={t('side.account')} onclick={() => onAccount?.()}>
    <span style:background={profileGradient(profile.color)}>
      {user ? user.username.slice(0, 2).toUpperCase() : profileInitials(profile.name)}
    </span>
  </button>
</div>

<style>
  .studio-nav{height:100%;display:flex;align-items:center;gap:8px;padding:0 16px;background:var(--panel-2);border-bottom:1px solid var(--border);color:var(--ink)}
  button{flex:none}
  .nav-icon{width:32px;height:32px;display:grid;place-items:center;border-radius:var(--r-md);color:var(--ink-dim)}
  .nav-icon:hover{background:var(--panel)}
  .back{display:flex;align-items:center;gap:7px;height:34px;padding:0 12px;border:1px solid var(--ui-edge);border-radius:var(--r-md);background:var(--ui-face);color:var(--ink);font-size:12px;white-space:nowrap}
  .back:hover{background:var(--raise)}
  .identity{flex:1;min-width:0;display:flex;align-items:baseline;gap:10px;padding:0 4px}
  .identity strong{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13.5px;font-weight:650}
  .save-state{display:flex;align-items:center;gap:6px;color:var(--muted);font-size:11px;white-space:nowrap}
  .save-state i{width:7px;height:7px;border-radius:var(--r-round);background:#6fd39a}
  /* Naranja sólo cuando hay algo sin asegurar: escribiendo o a medio guardar. */
  .save-state[data-state="saving"] i,.save-state[data-state="changes"] i{background:#e9b86b}
  /* "Publicación pendiente" NO es un aviso: el borrador está guardado y a salvo,
     lo único que falta es publicar, y eso lo decide el autor. Con el mismo
     naranja que "sin guardar" parecía que el guardado no había funcionado, sobre
     todo en estrecho, donde el texto se oculta y el punto va solo. Anillo, no
     relleno: hay algo esperando, no algo que vaya mal. */
  .save-state[data-state="publication"] i{background:transparent;box-shadow:inset 0 0 0 2px var(--accent)}
  .save-state[data-state="error"]{color:#e77d88}.save-state[data-state="error"] i{background:#e77d88}
  .card-jump{position:relative;flex:none}
  .card-jump summary{height:34px;display:flex;align-items:center;gap:6px;padding:0 9px;border:1px solid var(--ui-edge);border-radius:var(--r-md);background:var(--ui-face);color:var(--muted);font-size:10.5px;cursor:pointer;list-style:none}
  .card-jump summary::-webkit-details-marker{display:none}
  .card-jump summary:hover,.card-jump[open] summary{background:var(--raise);color:var(--ink)}
  .card-jump summary>b{min-width:18px;height:18px;display:grid;place-items:center;border-radius:var(--r-pill);background:var(--accent-weak);color:var(--accent-2);font-size:9px}
  .card-menu{position:absolute;z-index:30;right:0;top:40px;width:230px;display:grid;gap:4px;padding:8px;border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel);box-shadow:var(--shadow)}
  .card-menu header{display:flex;flex-direction:column;padding:3px 5px 6px}.card-menu header b{font-size:10.5px}.card-menu header small{color:var(--faint);font-size:8.5px}
  .card-menu button{min-width:0;display:grid;grid-template-columns:22px minmax(0,1fr) auto;align-items:center;gap:6px;padding:7px;border-radius:var(--r-sm);color:var(--muted);font-size:9.5px;text-align:left}
  .card-menu button:hover,.card-menu button.active{background:var(--raise);color:var(--ink)}
  .card-menu button>span{width:20px;height:20px;display:grid;place-items:center;border-radius:var(--r-sm);background:var(--accent-weak);color:var(--accent-2)}
  .card-menu button>b{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:9.5px}.card-menu button>small{color:var(--faint);font-size:8px}
  .card-menu p{margin:0;padding:8px;color:var(--faint);font-size:9px;text-align:center}
  .card-menu .new-card{grid-template-columns:18px 1fr;margin-top:3px;border-top:1px solid var(--border);border-radius:0;color:var(--accent-2)}
  .context-tools{display:flex;align-items:center;gap:2px;padding:4px;border-radius:var(--r-md);background:var(--ui-face);border:1px solid var(--ui-edge)}
  .context-tools button{min-width:27px;height:26px;padding:0 6px;border-radius:var(--r-sm);color:var(--muted);font-size:11px}
  .context-tools button:hover{background:var(--raise);color:var(--ink)}
  .text-context{display:flex;align-items:center;gap:2px;height:36px;padding:4px;border:1px solid var(--ui-edge);border-radius:var(--r-md);background:var(--ui-face)}
  .text-context>button{min-width:25px;height:26px;padding:0 5px;border-radius:var(--r-sm);color:var(--muted);font-size:12px}
  .text-context>button:hover,.text-context>button.active{background:var(--raise);color:var(--ink)}
  .text-context>i{width:1px;height:18px;margin:0 3px;background:var(--ui-edge)}
  .text-context label{height:26px;display:flex;align-items:center;border:1px solid var(--ui-edge);border-radius:var(--r-sm);background:var(--panel);color:var(--muted)}
  .text-context input{width:36px;padding:0 2px;border:0;outline:0;background:transparent;color:var(--ink);font:11px var(--mono);text-align:right;appearance:textfield}
  .text-context input::-webkit-inner-spin-button{appearance:none}
  .text-context label span{padding-right:5px;font-size:9px}
  .align-lines{width:14px;height:12px;display:flex;flex-direction:column;justify-content:space-between}
  .align-lines b{display:block;height:1px;background:currentColor}.align-lines b:nth-child(2){width:9px}
  .align-lines.left b:nth-child(2){align-self:flex-start}.align-lines.center b:nth-child(2){align-self:center}.align-lines.right b:nth-child(2){align-self:flex-end}
  .action{height:34px;padding:0 13px;border-radius:var(--r-md);font-size:12px;font-weight:650;white-space:nowrap}
  .action.ghost{background:var(--ui-face);border:1px solid var(--ui-edge);color:var(--ink)}
  .action.primary{background:var(--accent);color:#fff;box-shadow:0 4px 16px var(--accent-weak)}
  .action:disabled{background:var(--raise);color:var(--faint);box-shadow:none;cursor:not-allowed}
  .account{width:36px;height:36px;display:grid;place-items:center;border-radius:var(--r-md)}
  .account:hover{background:var(--panel)}
  .account span{width:28px;height:28px;display:grid;place-items:center;border-radius:var(--r-round);color:#fff;font-size:11px;font-weight:650;border:1px solid rgba(255,255,255,.14)}
  @media(max-width:700px){
    .back span,.context-tools,.text-context,.card-jump summary>span{display:none}
    .studio-nav{gap:6px;padding-inline:8px}.back{width:32px;padding:0;justify-content:center}
    .identity strong{font-size:12.5px}.save-state>span{display:none}
    .action{padding-inline:9px}.account{display:none}
  }
</style>
