<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchConfig, putConfig, testNotification } from './api';
  import type { AppConfig, DockerConfig, NotificationsConfig, ShutdownConfig } from './types';

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
  let webhookConfigured = $state(false);
  let chatConfigured = $state(false);

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
    notes = cfg.notes ?? [];
    execution = cfg.execution?.notifications ?? 'unconfigured';
  }

  async function load() {
    loading = true;
    try {
      applyView(await fetchConfig());
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

  onMount(() => {
    void load();
  });
</script>

<section class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
  <div class="flex flex-wrap items-start justify-between gap-2">
    <div>
      <h2 class="text-sm font-medium">Configuration</h2>
      <p class="mt-1 text-xs text-mist">
        Stored at <span class="font-mono">~/.config/lapguard/config.json</span> with mode 0600. Secrets are never written to logs.
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
        <span class="rounded-full bg-amber/15 px-2.5 py-0.5 font-mono text-[11px] text-amber">Stored only</span>
      </div>
      <p class="mt-1 text-xs text-mist">
        Thresholds are validated and persisted. LapGuard will not power off the machine yet. Warning and critical percents are also used for battery notifications.
      </p>

      <div class="mt-3 grid gap-3 sm:grid-cols-3">
        <label class="flex items-center gap-2 text-sm text-snow sm:col-span-3">
          <input type="checkbox" bind:checked={shutdown.enabled} />
          Enable automatic shutdown
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
      <button
        type="button"
        class="mt-3 rounded-full border border-line px-3 py-1 text-xs text-mist opacity-50"
        disabled
        title="Host shutdown is not implemented yet"
      >
        Shut down now
      </button>
    </div>

    <div class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-medium">Docker drain</h3>
        <span class="rounded-full bg-amber/15 px-2.5 py-0.5 font-mono text-[11px] text-amber">Stored only</span>
      </div>
      <p class="mt-1 text-xs text-mist">
        Stop-before-shutdown is stored here. Containers are not stopped in this milestone.
      </p>

      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="flex items-center gap-2 text-sm text-snow">
          <input type="checkbox" bind:checked={docker.stop_enabled} />
          Stop containers before shutdown
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
        class="mt-3 rounded-full border border-line px-3 py-1 text-xs text-mist opacity-50"
        disabled
        title="Docker control is not implemented yet"
      >
        Stop Docker containers
      </button>
    </div>
  </div>
</section>
