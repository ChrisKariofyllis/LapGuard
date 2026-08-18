<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchConfig, fetchPower, postDockerDrain, postPowerOff, putConfig, testNotification } from './api';
  import type { ActionsConfig, AppConfig, DockerConfig, NotificationsConfig, ShutdownConfig } from './types';

  let loading = $state(true);
  let saving = $state(false);
  let testing = $state(false);
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let testResult = $state<string | null>(null);
  let testFailed = $state(false);
  let showWebhook = $state(false);
  let notes = $state<string[]>([]);
  let execution = $state('unconfigured');
  let hostExecution = $state('disabled');
  let webhookConfigured = $state(false);
  let chatConfigured = $state(false);
  let powerSource = $state<'AC' | 'BATTERY' | 'UNKNOWN'>('UNKNOWN');
  let acting = $state(false);
  let modal = $state<null | 'poweroff' | 'docker'>(null);
  let confirmText = $state('');
  let actionNotice = $state<string | null>(null);
  let actionFailed = $state(false);

  let notifications = $state<NotificationsConfig>({
    provider: 'none',
    enabled: false,
    dry_run: false,
    webhook_url: '',
    chat_id: '',
  });
  let shutdown = $state<ShutdownConfig>({
    enabled: false,
    warning_threshold: 20,
    critical_threshold: 10,
  });
  let docker = $state<DockerConfig>({
    stop_enabled: false,
    timeout_seconds: 30,
  });
  let actions = $state<ActionsConfig>({
    real_enabled: false,
    require_confirmation: true,
    cooldown_seconds: 60,
    poweroff_timeout_seconds: 30,
    intended_plan: ['sync', 'poweroff'],
    gates: ['real_actions_disabled', 'safety_dry_run'],
    ready: false,
  });

  const thresholdHint = $derived.by(() => {
    if (shutdown.warning_threshold < 0 || shutdown.warning_threshold > 100) {
      return 'Warning percent must be between 0 and 100.';
    }
    if (shutdown.critical_threshold < 0 || shutdown.critical_threshold > 100) {
      return 'Critical percent must be between 0 and 100.';
    }
    if (shutdown.critical_threshold >= shutdown.warning_threshold) {
      return 'Critical percent must be lower than warning percent.';
    }
    return null;
  });

  const canTest = $derived(
    notifications.enabled && notifications.provider !== 'none' && (webhookConfigured || Boolean(notifications.webhook_url)),
  );

  const intendedPlan = $derived.by(() => {
    const plan: string[] = [];
    if (docker.stop_enabled) {
      plan.push('stop_docker');
    }
    plan.push('sync', 'poweroff');
    return plan;
  });
  const poweroffReady = $derived(actions.ready && powerSource === 'BATTERY');
  const dockerReady = $derived(actions.ready);
  const confirmWord = $derived(modal === 'docker' ? 'STOP_DOCKER' : 'POWER_OFF');

  function planLabel(step: string): string {
    switch (step) {
      case 'stop_docker':
        return 'Stop Docker containers';
      case 'sync':
        return 'Flush filesystem buffers';
      case 'poweroff':
        return 'Power off the host';
      default:
        return step.replaceAll('_', ' ');
    }
  }

  function gateLabel(gate: string): string {
    switch (gate) {
      case 'real_actions_disabled':
        return 'Real actions disabled';
      case 'safety_dry_run':
        return 'Safety dry-run is on';
      case 'confirmation_required':
        return 'Confirmation required';
      default:
        return gate.replaceAll('_', ' ');
    }
  }

  function hostBadge(): string {
    if (!actions.real_enabled) {
      return 'Real actions disabled';
    }
    if (hostExecution === 'dry_run') {
      return 'Dry-run';
    }
    if (actions.ready) {
      return 'Manual only';
    }
    return hostExecution;
  }

  function applyView(cfg: AppConfig) {
    webhookConfigured = Boolean(cfg.notifications.webhook_configured);
    chatConfigured = Boolean(cfg.notifications.chat_id_configured);
    notifications = {
      provider: cfg.notifications.provider,
      enabled: cfg.notifications.enabled,
      dry_run: Boolean(cfg.notifications.dry_run),
      webhook_url: '',
      chat_id: '',
    };
    shutdown = { ...cfg.shutdown };
    docker = { ...cfg.docker };
    if (cfg.actions) {
      actions = { ...cfg.actions };
    }
    notes = cfg.notes ?? [];
    execution = cfg.execution?.notifications ?? 'unconfigured';
    hostExecution = cfg.execution?.shutdown ?? 'disabled';
  }

  async function load() {
    loading = true;
    try {
      const [cfg, power] = await Promise.all([fetchConfig(), fetchPower()]);
      applyView(cfg);
      powerSource = power.source;
      error = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to load configuration';
    } finally {
      loading = false;
    }
  }

  async function save() {
    if (thresholdHint) {
      error = thresholdHint;
      return;
    }
    saving = true;
    notice = null;
    try {
      const next = await putConfig({ notifications, shutdown, docker });
      applyView(next);
      error = null;
      notice = 'Settings saved. Secrets stay on disk and are not shown here.';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to save configuration';
    } finally {
      saving = false;
    }
  }

  async function sendTest() {
    testing = true;
    testResult = null;
    testFailed = false;
    try {
      const result = await testNotification();
      testFailed = !result.ok;
      if (result.ok) {
        testResult = result.dry_run
          ? `Dry-run OK (${result.provider ?? notifications.provider}) — no HTTP request was sent.`
          : `Sent via ${result.provider ?? notifications.provider}.`;
      } else {
        testResult = result.error || 'Test notification failed.';
      }
    } catch (err) {
      testFailed = true;
      testResult = err instanceof Error ? err.message : 'Test notification failed.';
    } finally {
      testing = false;
    }
  }

  function openModal(kind: 'poweroff' | 'docker') {
    if (kind === 'poweroff' && !poweroffReady) {
      return;
    }
    if (kind === 'docker' && !dockerReady) {
      return;
    }
    modal = kind;
    confirmText = '';
    actionNotice = null;
    actionFailed = false;
  }

  function closeModal() {
    modal = null;
    confirmText = '';
  }

  async function confirmAction() {
    if (!modal || confirmText !== confirmWord || acting) {
      return;
    }
    acting = true;
    actionNotice = null;
    try {
      const result = modal === 'docker' ? await postDockerDrain() : await postPowerOff();
      actionFailed = !result.ok;
      actionNotice = result.ok
        ? 'Action accepted. Command output is not shown.'
        : result.error || result.detail || 'Action rejected';
      if (result.ok) {
        closeModal();
      }
    } catch (err) {
      actionFailed = true;
      actionNotice = err instanceof Error ? err.message : 'Action failed';
    } finally {
      acting = false;
    }
  }

  onMount(() => {
    void load();
    const id = window.setInterval(() => {
      void fetchPower()
        .then((power) => {
          powerSource = power.source;
        })
        .catch(() => {
          /* keep last known source */
        });
    }, 5000);
    return () => window.clearInterval(id);
  });
</script>


<section class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
  <div class="flex flex-wrap items-start justify-between gap-2">
    <div>
      <h2 class="text-sm font-medium">Configuration</h2>
      <p class="mt-1 text-xs text-mist">
        Stored in the process config file (mode 0600). Default for a user
        install is <span class="font-mono">~/.config/lapguard/config.json</span>;
        the system unit uses <span class="font-mono">/etc/lapguard/config.json</span>.
        Secrets are never written to logs.
      </p>
    </div>
    <button
      type="button"
      class="rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
      onclick={() => void save()}
      disabled={saving || loading || Boolean(thresholdHint)}
    >
      {saving ? 'Saving…' : 'Save settings'}
    </button>
  </div>

  {#if error}
    <p class="mt-3 rounded-xl border border-rose/40 bg-rose/10 px-3 py-2 text-sm text-rose">{error}</p>
  {/if}
  {#if notice}
    <p class="mt-3 rounded-xl border border-mint/30 bg-mint/10 px-3 py-2 text-sm text-mint">{notice}</p>
  {/if}
  {#if notes.length}
    <ul class="mt-3 space-y-1 text-xs text-amber">
      {#each notes as note}
        <li>{note}</li>
      {/each}
    </ul>
  {/if}

  <div class="mt-4 grid gap-4">
    <div class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-medium">Notifications</h3>
        <span class="rounded-full px-2.5 py-0.5 font-mono text-[11px] {execution === 'ready' || execution === 'dry_run' ? 'bg-mint/15 text-mint' : 'bg-amber/15 text-amber'}">{execution}</span>
      </div>
      <p class="mt-1 text-xs text-mist">
        Alerts fire for AC connect/disconnect and battery warning/critical. Delivery is off until you enable it.
      </p>

      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="block text-xs text-mist">
          Provider
          <select
            class="mt-1 w-full rounded-xl border border-line bg-panel px-3 py-2 text-sm text-snow"
            bind:value={notifications.provider}
          >
            <option value="none">None</option>
            <option value="ntfy">ntfy</option>
            <option value="telegram">Telegram</option>
            <option value="discord">Discord</option>
            <option value="webhook">Generic webhook</option>
          </select>
        </label>
        <div class="flex flex-col justify-end gap-2 text-sm text-snow">
          <label class="flex items-center gap-2">
            <input type="checkbox" bind:checked={notifications.enabled} />
            Enable notifications
          </label>
          <label class="flex items-center gap-2">
            <input type="checkbox" bind:checked={notifications.dry_run} />
            Dry-run (log only, no HTTP)
          </label>
        </div>
        <label class="block text-xs text-mist sm:col-span-2">
          Webhook URL
          <div class="mt-1 flex gap-2">
            <input
              class="w-full rounded-xl border border-line bg-panel px-3 py-2 font-mono text-sm text-snow"
              type={showWebhook ? 'url' : 'password'}
              autocomplete="off"
              spellcheck="false"
              placeholder={webhookConfigured ? 'Saved — enter a new URL to replace' : 'https://'}
              bind:value={notifications.webhook_url}
            />
            <button
              type="button"
              class="shrink-0 rounded-xl border border-line px-3 py-2 text-xs text-mist"
              onclick={() => (showWebhook = !showWebhook)}
            >
              {showWebhook ? 'Hide' : 'Show'}
            </button>
          </div>
        </label>
        <label class="block text-xs text-mist sm:col-span-2">
          Chat ID
          <input
            class="mt-1 w-full rounded-xl border border-line bg-panel px-3 py-2 font-mono text-sm text-snow"
            type="text"
            autocomplete="off"
            placeholder={chatConfigured ? 'Saved — enter a new chat ID to replace' : 'Telegram chat id'}
            bind:value={notifications.chat_id}
          />
        </label>
      </div>
      <div class="mt-3 flex flex-wrap items-center gap-3">
        <button
          type="button"
          class="rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
          disabled={testing || loading || !canTest}
          title={canTest ? 'Send a test message through the configured provider' : 'Enable notifications and configure a provider first'}
          onclick={() => void sendTest()}
        >
          {testing ? 'Sending…' : 'Send test notification'}
        </button>
        {#if testResult}
          <p class="text-xs {testFailed ? 'text-rose' : 'text-mint'}">{testResult}</p>
        {/if}
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-medium">Low-battery shutdown</h3>
        <span class="rounded-full bg-amber/15 px-2.5 py-0.5 font-mono text-[11px] text-amber">{hostBadge()}</span>
      </div>
      <p class="mt-1 text-xs text-mist">
        Thresholds are validated and persisted. Automatic low-battery shutdown is not executed in this alpha.
        Manual poweroff is experimental, disabled by default, and requires extra gates.
      </p>

      <div class="mt-3 grid gap-3 sm:grid-cols-3">
        <label class="flex items-center gap-2 text-sm text-snow sm:col-span-3">
          <input type="checkbox" bind:checked={shutdown.enabled} />
          Enable shutdown thresholds (automatic path still does not execute)
        </label>
        <label class="block text-xs text-mist">
          Warning %
          <input
            class="mt-1 w-full rounded-xl border border-line bg-panel px-3 py-2 font-mono text-sm text-snow"
            type="number"
            min="0"
            max="100"
            bind:value={shutdown.warning_threshold}
          />
        </label>
        <label class="block text-xs text-mist">
          Critical %
          <input
            class="mt-1 w-full rounded-xl border border-line bg-panel px-3 py-2 font-mono text-sm text-snow"
            type="number"
            min="0"
            max="100"
            bind:value={shutdown.critical_threshold}
          />
        </label>
      </div>
      {#if thresholdHint}
        <p class="mt-2 text-xs text-rose">{thresholdHint}</p>
      {/if}

      <div class="mt-3 rounded-xl border border-line bg-panel/60 px-3 py-2">
        <p class="text-[11px] uppercase tracking-wide text-mist">Intended plan</p>
        <ul class="mt-1 space-y-0.5 text-xs text-snow">
          {#each intendedPlan as step}
            <li>{planLabel(step)}</li>
          {/each}
        </ul>
        {#if actions.gates.length}
          <p class="mt-2 text-[11px] text-amber">{actions.gates.map(gateLabel).join(' · ')}</p>
        {/if}
        <p class="mt-1 text-[11px] text-mist">Power source: {powerSource}</p>
      </div>

      <button
        type="button"
        class="mt-3 rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
        disabled={!poweroffReady || acting}
        title={poweroffReady ? 'Requires confirmation' : 'Real actions stay disabled until every safety gate is satisfied and AC is disconnected'}
        onclick={() => openModal('poweroff')}
      >
        Shut down now
      </button>
    </div>

    <div class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-medium">Docker drain</h3>
        <span class="rounded-full bg-amber/15 px-2.5 py-0.5 font-mono text-[11px] text-amber">{hostBadge()}</span>
      </div>
      <p class="mt-1 text-xs text-mist">
        Stop-before-shutdown is stored here. Manual drain is experimental and disabled by default.
      </p>

      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="flex items-center gap-2 text-sm text-snow">
          <input type="checkbox" bind:checked={docker.stop_enabled} />
          Include Docker stop in the shutdown plan
        </label>
        <label class="block text-xs text-mist">
          Timeout (seconds)
          <input
            class="mt-1 w-full rounded-xl border border-line bg-panel px-3 py-2 font-mono text-sm text-snow"
            type="number"
            min="0"
            max="3600"
            bind:value={docker.timeout_seconds}
          />
        </label>
      </div>
      <button
        type="button"
        class="mt-3 rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
        disabled={!dockerReady || acting}
        title={dockerReady ? 'Requires confirmation' : 'Real actions stay disabled until every safety gate is satisfied'}
        onclick={() => openModal('docker')}
      >
        Stop Docker containers
      </button>
    </div>
  </div>
</section>

{#if modal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4">
    <div class="w-full max-w-md rounded-2xl border border-line bg-panel px-4 py-4">
      <h3 class="text-sm font-medium">
        {modal === 'docker' ? 'Confirm Docker drain' : 'Confirm host poweroff'}
      </h3>
      <p class="mt-2 text-xs text-mist">
        {modal === 'docker'
          ? 'This will stop running containers. Type STOP_DOCKER to continue.'
          : 'This will power off the machine. Type POWER_OFF to continue.'}
      </p>
      <input
        class="mt-3 w-full rounded-xl border border-line bg-ink-soft px-3 py-2 font-mono text-sm text-snow"
        type="text"
        autocomplete="off"
        spellcheck="false"
        bind:value={confirmText}
      />
      {#if actionNotice && actionFailed}
        <p class="mt-2 text-xs text-rose">{actionNotice}</p>
      {/if}
      <div class="mt-4 flex justify-end gap-2">
        <button
          type="button"
          class="rounded-full border border-line px-3 py-1 text-xs text-mist"
          onclick={closeModal}
          disabled={acting}
        >
          Cancel
        </button>
        <button
          type="button"
          class="rounded-full border border-rose/50 px-3 py-1 text-xs text-rose disabled:opacity-50"
          disabled={acting || confirmText !== confirmWord}
          onclick={() => void confirmAction()}
        >
          {acting ? 'Working…' : 'Confirm'}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if actionNotice && !modal}
  <p class="mt-3 text-xs {actionFailed ? 'text-rose' : 'text-mint'}">{actionNotice}</p>
{/if}
