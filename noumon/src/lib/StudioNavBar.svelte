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
</script>

<div class="studio-nav">
  <button
    class="nav-icon mobile-side"
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
      title={state.publishDisabled ? t('studio.publishUnavailable') : ''}
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
  .mobile-side{display:none}
  .back{display:flex;align-items:center;gap:7px;height:34px;padding:0 12px;border:1px solid var(--ui-edge);border-radius:var(--r-md);background:var(--ui-face);color:var(--ink);font-size:12px;white-space:nowrap}
  .back:hover{background:var(--raise)}
  .identity{flex:1;min-width:0;display:flex;align-items:baseline;gap:10px;padding:0 4px}
  .identity strong{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13.5px;font-weight:650}
  .save-state{display:flex;align-items:center;gap:6px;color:var(--muted);font-size:11px;white-space:nowrap}
  .save-state i{width:7px;height:7px;border-radius:var(--r-round);background:#6fd39a}
  .save-state[data-state="saving"] i,.save-state[data-state="changes"] i{background:#e9b86b}
  .save-state[data-state="error"]{color:#e77d88}.save-state[data-state="error"] i{background:#e77d88}
  .context-tools{display:flex;align-items:center;gap:2px;padding:4px;border-radius:var(--r-md);background:var(--ui-face);border:1px solid var(--ui-edge)}
  .context-tools button{min-width:27px;height:26px;padding:0 6px;border-radius:var(--r-sm);color:var(--muted);font-size:11px}
  .context-tools button:hover{background:var(--raise);color:var(--ink)}
  .action{height:34px;padding:0 13px;border-radius:var(--r-md);font-size:12px;font-weight:650;white-space:nowrap}
  .action.ghost{background:var(--ui-face);border:1px solid var(--ui-edge);color:var(--ink)}
  .action.primary{background:var(--accent);color:#fff;box-shadow:0 4px 16px var(--accent-weak)}
  .action:disabled{background:var(--raise);color:var(--faint);box-shadow:none;cursor:not-allowed}
  .account{width:36px;height:36px;display:grid;place-items:center;border-radius:var(--r-md)}
  .account:hover{background:var(--panel)}
  .account span{width:28px;height:28px;display:grid;place-items:center;border-radius:var(--r-round);color:#fff;font-size:11px;font-weight:650;border:1px solid rgba(255,255,255,.14)}
  @media(max-width:700px){
    .mobile-side{display:grid}.back span,.context-tools{display:none}
    .studio-nav{gap:6px;padding-inline:8px}.back{width:32px;padding:0;justify-content:center}
    .identity strong{font-size:12.5px}.save-state>span{display:none}
    .action{padding-inline:9px}.account{display:none}
  }
</style>
