<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchCapabilities, fetchTelemetry } from './lib/api';
  import {
    abs,
    fmtInt,
    fmtNumber,
    healthTone,
    statusLabel,
    statusTone,
    type StatusTone,
  } from './lib/format';
  import type { Capabilities, Telemetry } from './lib/types';

  const POLL_MS = 2000;
  const THEME_KEY = 'lapguard-theme';

  let dark = $state(true);
  let telemetry = $state<Telemetry | null>(null);
  let capabilities = $state<Capabilities | null>(null);
  let error = $state<string | null>(null);
  let updatedAt = $state<Date | null>(null);

  const battery = $derived(telemetry?.battery);
  const present = $derived(battery?.present ?? false);
  const capacity = $derived(battery?.capacity_percent);
  const tone = $derived(statusTone(battery?.status, present));
  const power = $derived(abs(battery?.power_w));

  function applyTheme(next: boolean) {
    dark = next;
    document.documentElement.classList.toggle('dark', next);
    document.documentElement.style.colorScheme = next ? 'dark' : 'light';
    localStorage.setItem(THEME_KEY, next ? 'dark' : 'light');
  }

  async function refresh(signal?: AbortSignal) {
    try {
      const [tel, caps] = await Promise.all([
        fetchTelemetry(signal),
        fetchCapabilities(signal),
      ]);
      telemetry = tel;
      capabilities = caps;
      error = null;
      updatedAt = new Date();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to reach LapGuard';
    }
  }

  onMount(() => {
    const saved = localStorage.getItem(THEME_KEY);
    applyTheme(saved ? saved !== 'light' : true);

    const controller = new AbortController();
    refresh(controller.signal);
    const id = window.setInterval(() => {
      void refresh();
    }, POLL_MS);

    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  });

  const ring = $derived.by(() => {
    const pct = Math.max(0, Math.min(100, capacity ?? 0));
    const radius = 88;
    const circ = 2 * Math.PI * radius;
    return {
      pct,
      radius,
      circ,
      dash: circ * (pct / 100),
    };
  });

  const toneClass: Record<StatusTone, string> = {
    charge: 'text-mint',
    discharge: 'text-amber',
    full: 'text-mint',
    idle: 'text-sky',
    missing: 'text-rose',
  };

  const ringClass: Record<StatusTone, string> = {
    charge: 'stroke-mint',
    discharge: 'stroke-amber',
    full: 'stroke-mint',
    idle: 'stroke-sky',
    missing: 'stroke-rose',
  };
</script>

<div class="min-h-dvh px-4 pb-10 pt-[max(1.25rem,env(safe-area-inset-top))] sm:px-6">
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-5">
    <header class="flex items-start justify-between gap-3">
      <div>
        <p class="font-mono text-[11px] uppercase tracking-[0.22em] text-mist">Laptop power manager</p>
        <h1 class="mt-1 text-3xl font-semibold tracking-tight text-snow">
          LapGuard
        </h1>
        <p class="mt-1 text-sm text-mist">
          {capabilities ? `${capabilities.version} · ${capabilities.listen}` : 'connecting…'}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span
          class="rounded-full border border-line px-3 py-1 font-mono text-[11px] uppercase tracking-wider text-mist dark:border-line"
        >
          {telemetry?.provider ?? capabilities?.provider ?? '—'}
        </span>
        <button
          type="button"
          class="rounded-full border border-line px-3 py-1 text-sm text-mist transition hover:border-mist hover:text-snow"
          onclick={() => applyTheme(!dark)}
        >
          {dark ? 'Light' : 'Dark'}
        </button>
      </div>
    </header>

    {#if error}
      <div class="rounded-2xl border border-rose/40 bg-rose/10 px-4 py-3 text-sm text-rose">
        {error}. Start the Go API on 127.0.0.1:8585 if it is not running.
      </div>
    {/if}

    <section
      class="rounded-3xl border border-line bg-panel/80 p-6 shadow-[0_20px_60px_rgba(0,0,0,0.28)] backdrop-blur dark:bg-panel/80"
    >
      <div class="flex flex-col items-center gap-6 sm:flex-row sm:items-center sm:justify-between">
        <div class="relative grid place-items-center">
          <svg width="220" height="220" viewBox="0 0 220 220" class="drop-shadow-sm">
            <circle cx="110" cy="110" r={ring.radius} class="fill-none stroke-line" stroke-width="14" />
            <circle
              cx="110"
              cy="110"
              r={ring.radius}
              class={`fill-none ${ringClass[tone]}`}
              stroke-width="14"
              stroke-linecap="round"
              stroke-dasharray={`${ring.dash} ${ring.circ}`}
              transform="rotate(-90 110 110)"
            />
          </svg>
          <div class="absolute inset-0 grid place-items-center text-center">
            <div>
              <p class="font-mono text-5xl font-medium tracking-tight">
                {capacity === undefined ? '—' : capacity}<span class="text-2xl text-mist">%</span>
              </p>
              <p class={`mt-1 text-sm font-medium ${toneClass[tone]}`}>
                {statusLabel(battery?.status, present)}
              </p>
            </div>
          </div>
        </div>

        <div class="w-full flex-1 space-y-4">
          <div>
            <p class="font-mono text-[11px] uppercase tracking-[0.18em] text-mist">Instantaneous power</p>
            <p class="mt-1 font-mono text-4xl font-medium tracking-tight">
              {fmtNumber(power, 2)}
              <span class="text-lg text-mist">W</span>
            </p>
            <p class="mt-1 text-sm text-mist">
              {#if !present}
                No pack detected. Auto-fallback to mock is available with <span class="font-mono">--provider mock</span>.
              {:else if tone === 'discharge'}
                Drawing from the battery
              {:else if tone === 'charge'}
                Charging the pack
              {:else}
                Power is stable
              {/if}
            </p>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div class="rounded-2xl border border-line bg-ink-soft/60 px-4 py-3">
              <p class="text-xs text-mist">Health</p>
              <p class={`mt-1 font-mono text-xl ${toneClass[healthTone(battery?.health_percent)]}`}>
                {fmtNumber(battery?.health_percent, 1, '%')}
              </p>
            </div>
            <div class="rounded-2xl border border-line bg-ink-soft/60 px-4 py-3">
              <p class="text-xs text-mist">Cycles</p>
              <p class="mt-1 font-mono text-xl">{fmtInt(battery?.cycle_count)}</p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="grid grid-cols-2 gap-3 sm:grid-cols-3">
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Voltage</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.voltage_now_v, 3, ' V')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Current</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.current_now_a, 3, ' A')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Power now</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.power_now_w, 2, ' W')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Energy full</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.energy_full_wh, 1, ' Wh')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Design full</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.energy_full_design_wh, 1, ' Wh')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Pack</p>
        <p class="mt-2 font-mono text-lg">{battery?.name ?? '—'}</p>
      </article>
    </section>

    <section class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h2 class="text-sm font-medium">Capabilities</h2>
        <p class="font-mono text-[11px] text-mist">
          {updatedAt ? `updated ${updatedAt.toLocaleTimeString()}` : 'waiting'}
        </p>
      </div>
      <p class="mt-2 text-sm text-mist">
        Milestone 1 is read-only telemetry. Shutdown, Docker, charge thresholds, notifications and
        authentication are not enabled.
      </p>
      <div class="mt-3 flex flex-wrap gap-2">
        {#each capabilities?.available_fields ?? [] as field}
          <span class="rounded-full bg-mint/10 px-3 py-1 font-mono text-[11px] text-mint">{field}</span>
        {/each}
        {#each telemetry?.missing_fields ?? [] as field}
          <span class="rounded-full bg-rose/10 px-3 py-1 font-mono text-[11px] text-rose">{field} missing</span>
        {/each}
      </div>
      {#if telemetry?.warnings?.length}
        <ul class="mt-3 space-y-1 text-xs text-amber">
          {#each telemetry.warnings as warning}
            <li>{warning}</li>
          {/each}
        </ul>
      {/if}
    </section>
  </div>
</div>
