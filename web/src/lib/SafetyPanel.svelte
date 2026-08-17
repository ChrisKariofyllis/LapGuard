<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchSafety, testSafety } from './api';
  import type { SafetyStatus } from './types';

  let safety = $state<SafetyStatus | null>(null);
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let testing = $state(false);

  const stateClass = $derived.by(() => {
    switch (safety?.state) {
      case 'NORMAL':
      case 'AC_CONNECTED':
        return 'text-mint';
      case 'WARNING':
        return 'text-amber';
      case 'CRITICAL':
      case 'SHUTDOWN_PENDING':
        return 'text-rose';
      default:
        return 'text-mist';
    }
  });

  async function refresh(signal?: AbortSignal) {
    try {
      safety = await fetchSafety(signal);
      error = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to load safety status';
    }
  }

  async function simulate(scenario: 'warning' | 'critical') {
    testing = true;
    notice = null;
    try {
      const result = await testSafety(scenario);
      if (result.safety) {
        safety = result.safety;
      }
      notice = result.message || 'Dry run — no commands will be executed';
      error = result.ok ? null : result.error || 'Simulation failed';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Simulation failed';
    } finally {
      testing = false;
    }
  }

  onMount(() => {
    const controller = new AbortController();
    void refresh(controller.signal);
    const id = window.setInterval(() => {
      void refresh();
    }, 5000);
    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  });
</script>

<section class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
  <div class="flex flex-wrap items-start justify-between gap-2">
    <div>
      <h2 class="text-sm font-medium">Safety controller</h2>
      <p class="mt-1 text-xs text-mist">Monitors battery percent while discharging. Host commands are not executed.</p>
    </div>
    <span class="rounded-full bg-amber/15 px-2.5 py-0.5 font-mono text-[11px] text-amber">Dry-run</span>
  </div>

  <p class="mt-3 rounded-xl border border-amber/40 bg-amber/10 px-3 py-2 text-sm text-amber">
    Dry run — no commands will be executed
  </p>

  {#if error}
    <p class="mt-3 rounded-xl border border-rose/40 bg-rose/10 px-3 py-2 text-sm text-rose">{error}</p>
  {/if}
  {#if notice}
    <p class="mt-3 text-xs text-mist">{notice}</p>
  {/if}

  <div class="mt-4 grid gap-3 sm:grid-cols-3">
    <article class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <p class="text-xs text-mist">State</p>
      <p class="mt-1 font-mono text-lg {stateClass}">{safety?.state ?? '—'}</p>
      {#if safety?.reason}
        <p class="mt-1 text-[11px] text-mist">{safety.reason}</p>
      {/if}
    </article>
    <article class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <p class="text-xs text-mist">Warning</p>
      <p class="mt-1 font-mono text-lg">{safety?.warning_threshold ?? '—'}%</p>
    </article>
    <article class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
      <p class="text-xs text-mist">Critical</p>
      <p class="mt-1 font-mono text-lg">{safety?.critical_threshold ?? '—'}%</p>
    </article>
  </div>

  {#if safety?.last_event}
    <p class="mt-3 text-xs text-mist">
      Last event: <span class="font-mono">{safety.last_event.type}</span>
      {#if safety.last_event.percent !== undefined && safety.last_event.percent !== null}
        · {safety.last_event.percent}%
      {/if}
    </p>
  {/if}
  {#if safety?.intended_actions?.length}
    <p class="mt-1 text-xs text-mist">
      Intended (not executed): <span class="font-mono">{safety.intended_actions.join(', ')}</span>
    </p>
  {/if}

  <div class="mt-4 flex flex-wrap gap-2">
    <button
      type="button"
      class="rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
      disabled={testing}
      onclick={() => void simulate('warning')}
    >
      Simulate warning
    </button>
    <button
      type="button"
      class="rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
      disabled={testing}
      onclick={() => void simulate('critical')}
    >
      Simulate critical
    </button>
  </div>
</section>
