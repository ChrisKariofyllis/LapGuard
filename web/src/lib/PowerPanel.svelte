<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchEvents, fetchPower } from './api';
  import type { PowerEvent, PowerStatus } from './types';

  let power = $state<PowerStatus | null>(null);
  let events = $state<PowerEvent[]>([]);
  let error = $state<string | null>(null);

  const sourceLabel = $derived.by(() => {
    switch (power?.source) {
      case 'AC':
        return 'On AC';
      case 'BATTERY':
        return 'On battery';
      default:
        return 'Power source unknown';
    }
  });

  const sourceClass = $derived.by(() => {
    switch (power?.source) {
      case 'AC':
        return 'text-mint';
      case 'BATTERY':
        return 'text-amber';
      default:
        return 'text-rose';
    }
  });

  function formatEvent(type: string): string {
    switch (type) {
      case 'AC_CONNECTED':
        return 'AC connected';
      case 'AC_DISCONNECTED':
        return 'AC disconnected';
      case 'AC_UNKNOWN':
        return 'AC state unknown';
      default:
        return type;
    }
  }

  function formatTime(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) {
      return iso;
    }
    return d.toLocaleString();
  }

  function formatDuration(ms: number | undefined): string {
    if (ms === undefined) {
      return '';
    }
    const sec = Math.round(ms / 1000);
    if (sec < 60) {
      return `${sec}s on battery`;
    }
    const min = Math.round(sec / 60);
    return `${min}m on battery`;
  }

  async function refresh(signal?: AbortSignal) {
    try {
      const [pwr, log] = await Promise.all([fetchPower(signal), fetchEvents(50, undefined, signal)]);
      power = pwr;
      events = log.events;
      error = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to load power status';
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
      <h2 class="text-[22px] font-medium tracking-tight">Power source</h2>
      <p class="mt-1 text-sm text-mist">
        Mains adapters are discovered from sysfs <span class="font-mono">type=Mains</span>. Names are not hardcoded.
      </p>
    </div>
        <span class="lg-badge font-mono uppercase tracking-wider {sourceClass}">
          {power ? sourceLabel : '…'}
        </span>
  </div>

  {#if error}
    <p class="mt-3 text-sm text-rose">{error}</p>
  {/if}

  {#if (power?.adapters.length ?? 0) === 0}
    <p class="mt-3 rounded-xl border border-amber/30 bg-amber/10 px-3 py-2 text-sm text-amber">
      No mains adapter detected. AC power-loss events cannot be recorded on this machine.
    </p>
  {:else}
    <ul class="mt-3 flex flex-wrap gap-2">
      {#each power?.adapters ?? [] as adapter}
        <li
          class={`rounded-full px-3 py-1 font-mono text-[11px] ${adapter.readable && adapter.online ? 'bg-mint/10 text-mint' : adapter.readable ? 'bg-amber/10 text-amber' : 'bg-rose/10 text-rose'}`}
        >
          {adapter.name}
          {#if !adapter.readable}
            · online unreadable
          {:else if adapter.online}
            · online
          {:else}
            · offline
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if power?.reason && power.source === 'UNKNOWN'}
    <p class="mt-2 text-xs text-mist">{power.reason}</p>
  {/if}

  <p class="mt-3 font-mono text-[11px] text-mist">
    watcher {power?.watcher.running ? 'running' : 'idle'}
    · poll {power?.watcher.interval_seconds ?? '—'}s
    · debounce {power?.watcher.debounce_seconds ?? '—'}s
  </p>

  <div class="mt-4">
    <h3 class="text-xs font-medium uppercase tracking-[0.16em] text-mist">Recent power events</h3>
    {#if events.length === 0}
      <p class="mt-2 text-sm text-mist">No AC connect or disconnect events yet. The startup baseline is not logged.</p>
    {:else}
      <ul class="mt-2 space-y-2">
        {#each events as event}
          <li class="lg-card-nested">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <p class="text-sm font-medium">{formatEvent(event.type)}</p>
              <p class="font-mono text-[11px] text-mist">{formatTime(event.timestamp)}</p>
            </div>
            <p class="mt-1 font-mono text-[11px] text-mist">
              {event.source}
              {#if event.duration_ms !== undefined}
                · {formatDuration(event.duration_ms)}
              {/if}
            </p>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>
