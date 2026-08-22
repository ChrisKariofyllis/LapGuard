<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchAutoDrain, putAutoDrainConfig, respondAutoDrain } from './api';
  import type { AutoDrainStatus } from './types';

  let status = $state<AutoDrainStatus | null>(null);
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let saving = $state(false);
  let responding = $state(false);

  let enabled = $state(false);
  let threshold = $state(10);
  let preMinutes = $state(30);
  let timeoutMinutes = $state(10);

  const awaiting = $derived(Boolean(status?.awaiting_response));
  const stateBadge = $derived.by(() => {
    switch (status?.state) {
      case 'AUTO_DRAIN_IDLE':
        return 'lg-badge--ok';
      case 'AUTO_DRAIN_WARNING_SENT':
      case 'AUTO_DRAIN_AWAITING_RESPONSE':
        return 'lg-badge--warn';
      case 'AUTO_DRAIN_EXECUTING':
      case 'AUTO_DRAIN_TIMEOUT':
        return 'lg-badge--err';
      case 'AUTO_DRAIN_ABORTED':
        return 'lg-badge--info';
      default:
        return '';
    }
  });

  function apply(next: AutoDrainStatus) {
    status = next;
    enabled = Boolean(next.enabled);
    threshold = next.battery_threshold_percent;
    preMinutes = next.pre_notification_minutes;
    timeoutMinutes = next.response_timeout_minutes;
  }

  async function refresh(signal?: AbortSignal) {
    try {
      apply(await fetchAutoDrain(signal));
      error = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to load auto-drain status';
    }
  }

  async function save() {
    saving = true;
    notice = null;
    try {
      const next = await putAutoDrainConfig({
        enabled,
        battery_threshold_percent: threshold,
        pre_notification_minutes: preMinutes,
        response_timeout_minutes: timeoutMinutes,
        notification_services: status?.notification_services ?? ['ntfy'],
        on_user_no: 'continue_on_battery',
      });
      apply(next);
      notice = enabled
        ? 'Saved. Drain still needs a notification, docker.stop_enabled, and YES or timeout.'
        : 'Smart automatic drain is off.';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to save auto-drain config';
    } finally {
      saving = false;
    }
  }

  async function respond(action: 'yes' | 'no') {
    responding = true;
    notice = null;
    try {
      apply(await respondAutoDrain(action));
      notice = action === 'yes' ? 'Save+Stop confirmed.' : 'Continuing on battery.';
      error = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to send auto-drain response';
    } finally {
      responding = false;
    }
  }

  onMount(() => {
    const controller = new AbortController();
    void refresh(controller.signal);
    const id = window.setInterval(() => {
      void refresh();
    }, 2000);
    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  });
</script>

<section class="lg-card">
  <div class="flex flex-wrap items-start justify-between gap-2">
    <div>
      <h2 class="text-[22px] font-medium tracking-tight">Smart automatic drain</h2>
      <p class="mt-1 text-sm text-mist">
        Optional low-battery drain. Off by default. Never runs without a notification, and host
        commands still require <span class="font-mono">docker.stop_enabled</span>,
        <span class="font-mono">safety.dry_run=false</span>, and
        <span class="font-mono">actions.real_enabled=true</span>.
      </p>
    </div>
    <span class="lg-badge font-mono {status?.enabled ? 'lg-badge--warn' : 'lg-badge--err'}">
      {status?.enabled ? 'Experimental' : 'Disabled'}
    </span>
  </div>

  <p class="mt-3 rounded-xl border border-amber/40 bg-amber/10 px-3 py-2 text-sm text-amber">
    ntfy cannot POST back to this host. Use the dashboard YES / NO buttons. A timeout is treated as YES.
    There is no UPS integration and no root escalation.
  </p>

  {#if error}
    <p class="mt-3 rounded-xl border border-rose/40 bg-rose/10 px-3 py-2 text-sm text-rose">{error}</p>
  {/if}
  {#if notice}
    <p class="mt-3 text-xs text-mist">{notice}</p>
  {/if}

  <div class="mt-4 grid gap-3 sm:grid-cols-3">
    <article class="lg-card-nested">
      <p class="lg-stat-label">State</p>
      <p class="mt-2">
        <span class="lg-badge font-mono {stateBadge}">{status?.state ?? '—'}</span>
      </p>
      {#if status?.reason}
        <p class="mt-1 text-[11px] text-mist">{status.reason}</p>
      {/if}
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Threshold</p>
      <p class="lg-stat-value mt-2">{status?.battery_threshold_percent ?? '—'}%</p>
      <p class="mt-1 text-[11px] text-mist">
        {status?.discharging ? 'Discharging' : 'Not discharging'}
        {#if status?.battery_percent !== undefined && status?.battery_percent !== null}
          · {status.battery_percent}%
        {/if}
      </p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Response wait</p>
      <p class="lg-stat-value mt-2">
        {#if awaiting}
          {status?.seconds_remaining ?? 0}s
        {:else}
          {status?.response_timeout_minutes ?? '—'} min
        {/if}
      </p>
      {#if status?.commands_executed}
        <p class="mt-1 text-[11px] text-rose">Commands executed</p>
      {:else}
        <p class="mt-1 text-[11px] text-mist">commands_executed=false</p>
      {/if}
    </article>
  </div>

  {#if awaiting}
    <div class="mt-4 flex flex-wrap gap-2">
      <button
        type="button"
        class="lg-btn lg-btn-danger"
        disabled={responding}
        onclick={() => void respond('yes')}
      >
        YES · Save+Stop
      </button>
      <button
        type="button"
        class="lg-btn lg-btn-secondary"
        disabled={responding}
        onclick={() => void respond('no')}
      >
        NO · Let run
      </button>
    </div>
  {/if}

  <form
    class="mt-4 grid gap-3 sm:grid-cols-2"
    onsubmit={(event) => {
      event.preventDefault();
      void save();
    }}
  >
    <label class="flex items-center gap-2 text-sm">
      <input type="checkbox" bind:checked={enabled} />
      Enable auto-drain
    </label>
    <label class="text-xs text-mist">
      Battery threshold (%)
      <input
        class="lg-input mt-1 font-mono"
        type="number"
        min="1"
        max="100"
        bind:value={threshold}
      />
    </label>
    <label class="text-xs text-mist">
      Message lead time (min)
      <input
        class="lg-input mt-1 font-mono"
        type="number"
        min="1"
        max="1440"
        bind:value={preMinutes}
      />
    </label>
    <label class="text-xs text-mist">
      Response timeout (min)
      <input
        class="lg-input mt-1 font-mono"
        type="number"
        min="1"
        max="1440"
        bind:value={timeoutMinutes}
      />
    </label>
    <div class="sm:col-span-2">
      <button
        type="submit"
        class="lg-btn lg-btn-primary"
        disabled={saving}
      >
        {saving ? 'Saving…' : 'Save auto-drain'}
      </button>
    </div>
  </form>
</section>
