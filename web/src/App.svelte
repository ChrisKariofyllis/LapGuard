<script lang="ts">
  import { onMount } from 'svelte';
  import DashboardTab from './lib/tabs/DashboardTab.svelte';
  import SafetyTab from './lib/tabs/SafetyTab.svelte';
  import AutoDrainTab from './lib/tabs/AutoDrainTab.svelte';
  import NotificationsTab from './lib/tabs/NotificationsTab.svelte';
  import SettingsTab from './lib/tabs/SettingsTab.svelte';
  import { fetchCapabilities, fetchDiscover, fetchTelemetry } from './lib/api';
  import { getAPIToken, isLoopbackPage, setAPIToken } from './lib/auth';
  import { statusLabel, statusTone } from './lib/format';
  import type { Capabilities, Telemetry } from './lib/types';

  const POLL_MS = 2000;
  const TAB_KEY = 'lapguard-tab';

  type TabId = 'dashboard' | 'safety' | 'auto-drain' | 'notifications' | 'settings';

  const TABS: { id: TabId; label: string }[] = [
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'safety', label: 'Safety' },
    { id: 'auto-drain', label: 'Auto-Drain' },
    { id: 'notifications', label: 'Notifications' },
    { id: 'settings', label: 'Settings' },
  ];

  function parseTab(raw: string | null): TabId {
    switch (raw) {
      case 'safety':
      case 'auto-drain':
      case 'notifications':
      case 'settings':
      case 'dashboard':
        return raw;
      default:
        return 'dashboard';
    }
  }

  let tab = $state<TabId>('dashboard');
  let telemetry = $state<Telemetry | null>(null);
  let capabilities = $state<Capabilities | null>(null);
  let error = $state<string | null>(null);
  let updatedAt = $state<Date | null>(null);
  let scanning = $state(false);
  let apiToken = $state(getAPIToken());

  const battery = $derived(telemetry?.battery);
  const present = $derived(battery?.present ?? false);
  const capacity = $derived(battery?.capacity_percent);
  const tone = $derived(statusTone(battery?.status, present));

  function setTab(next: TabId) {
    tab = next;
    sessionStorage.setItem(TAB_KEY, next);
    const hash = `#${next}`;
    if (window.location.hash !== hash) {
      history.replaceState(null, '', hash);
    }
  }

  async function refresh(signal?: AbortSignal) {
    try {
      const [tel, caps] = await Promise.all([fetchTelemetry(signal), fetchCapabilities(signal)]);
      telemetry = tel;
      capabilities = caps;
      error = null;
      updatedAt = new Date();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to reach LapGuard';
    }
  }

  async function rescan() {
    scanning = true;
    try {
      await fetchDiscover();
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Discovery failed';
    } finally {
      scanning = false;
    }
  }

  onMount(() => {
    document.documentElement.classList.add('dark');
    document.documentElement.style.colorScheme = 'dark';

    const fromHash = parseTab(window.location.hash.replace(/^#/, '') || null);
    const fromStore = parseTab(sessionStorage.getItem(TAB_KEY));
    setTab(window.location.hash ? fromHash : fromStore);

    const onHash = () => setTab(parseTab(window.location.hash.replace(/^#/, '') || null));
    window.addEventListener('hashchange', onHash);

    const controller = new AbortController();
    refresh(controller.signal);
    const id = window.setInterval(() => {
      void refresh();
    }, POLL_MS);

    return () => {
      controller.abort();
      window.clearInterval(id);
      window.removeEventListener('hashchange', onHash);
    };
  });

  const toneDot: Record<string, string> = {
    charge: 'bg-mint',
    discharge: 'bg-amber',
    full: 'bg-mint',
    idle: 'bg-sky',
    missing: 'bg-rose',
  };
</script>

<div class="min-h-dvh bg-canvas text-snow">
  <header class="lg-nav sticky top-0 z-40">
    <div class="mx-auto flex h-full max-w-[1280px] items-center gap-3 px-4 sm:px-6">
      <div class="flex min-w-0 shrink-0 items-center gap-2">
        <img
          src="/lapguard-logo.jpg"
          alt="LapGuard"
          class="h-7 w-7 rounded-md object-cover"
          width="28"
          height="28"
        />
        <div class="hidden leading-tight sm:block">
          <p class="text-sm font-medium tracking-tight">LapGuard</p>
          <p class="font-mono text-[11px] text-mist">{capabilities?.version ?? '…'}</p>
        </div>
      </div>

      <div
        class="flex min-w-0 flex-1 items-center justify-start overflow-x-auto sm:justify-center"
        role="tablist"
        aria-label="LapGuard sections"
      >
        {#each TABS as item}
          <button
            type="button"
            class="lg-tab shrink-0"
            role="tab"
            id="tab-{item.id}"
            aria-selected={tab === item.id}
            aria-controls="panel-{item.id}"
            tabindex={tab === item.id ? 0 : -1}
            onclick={() => setTab(item.id)}
          >
            {item.label}
          </button>
        {/each}
      </div>

      <div class="flex shrink-0 items-center gap-2">
        <span class="lg-badge hidden items-center gap-1.5 font-mono sm:inline-flex">
          <span class={`h-1.5 w-1.5 rounded-full ${toneDot[tone]}`}></span>
          {capacity === undefined ? '—' : `${capacity}%`}
          · {statusLabel(battery?.status, present)}
        </span>
        <span class="lg-badge font-mono">{telemetry?.provider ?? capabilities?.provider ?? '—'}</span>
      </div>
    </div>
  </header>

  <main class="mx-auto w-full max-w-[1280px] px-4 py-6 sm:px-6">
    {#if error}
      <div class="mb-5 rounded-xl border border-rose/40 bg-rose/10 px-4 py-3 text-sm text-rose">
        {error}. Start the Go API on 127.0.0.1:8585 if it is not running.
      </div>
    {/if}

    {#if capabilities?.auth_enabled && tab !== 'settings'}
      <section class="lg-card mb-5 text-sm">
        <p class="font-medium">Token setup</p>
        <p class="mt-1 text-mist">
          {#if isLoopbackPage()}
            This browser is on loopback, so settings save without a token. Paste a token only if you will use Tailscale or
            another remote client.
          {:else}
            Remote access: PUT/POST need a Bearer token. GET telemetry stays readable without one.
          {/if}
          Full auth flags live in Settings.
        </p>
        <input
          class="lg-input mt-2 font-mono"
          type="password"
          autocomplete="off"
          placeholder={isLoopbackPage() ? 'Optional Bearer token for remote API calls' : 'Bearer token'}
          bind:value={apiToken}
          oninput={() => setAPIToken(apiToken)}
        />
      </section>
    {:else if capabilities?.auth_warning && tab !== 'settings'}
      <p class="mb-5 text-xs text-mist">{capabilities.auth_warning}</p>
    {/if}

    <div id="panel-dashboard" role="tabpanel" aria-labelledby="tab-dashboard" hidden={tab !== 'dashboard'}>
      {#if tab === 'dashboard'}
        <DashboardTab {telemetry} {capabilities} {updatedAt} {scanning} onrescan={() => void rescan()} />
      {/if}
    </div>

    <div id="panel-safety" role="tabpanel" aria-labelledby="tab-safety" hidden={tab !== 'safety'}>
      {#if tab === 'safety'}
        <SafetyTab />
      {/if}
    </div>

    <div id="panel-auto-drain" role="tabpanel" aria-labelledby="tab-auto-drain" hidden={tab !== 'auto-drain'}>
      {#if tab === 'auto-drain'}
        <AutoDrainTab />
      {/if}
    </div>

    <div id="panel-notifications" role="tabpanel" aria-labelledby="tab-notifications" hidden={tab !== 'notifications'}>
      {#if tab === 'notifications'}
        <NotificationsTab />
      {/if}
    </div>

    <div id="panel-settings" role="tabpanel" aria-labelledby="tab-settings" hidden={tab !== 'settings'}>
      {#if tab === 'settings'}
        <SettingsTab />
      {/if}
    </div>
  </main>
</div>
