<script lang="ts">
  import { onMount } from 'svelte';
  import ConfigPanel from './lib/ConfigPanel.svelte';
  import { fetchCapabilities, fetchDiscover, fetchTelemetry } from './lib/api';
  import {
    abs,
    fmtInt,
    fmtNumber,
    healthTone,
    statusLabel,
    statusTone,
    type StatusTone,
  } from './lib/format';
  import type { Capabilities, Telemetry, Tools } from './lib/types';

  const POLL_MS = 2000;
  const THEME_KEY = 'lapguard-theme';

  let dark = $state(true);
  let telemetry = $state<Telemetry | null>(null);
  let capabilities = $state<Capabilities | null>(null);
  let error = $state<string | null>(null);
  let updatedAt = $state<Date | null>(null);
  let scanning = $state(false);

  const battery = $derived(telemetry?.battery);
  const present = $derived(battery?.present ?? false);
  const capacity = $derived(battery?.capacity_percent);
  const tone = $derived(statusTone(battery?.status, present));
  const power = $derived(abs(battery?.power_w));
  const powerMode = $derived(battery?.power_calculation ?? capabilities?.power_calculation);
  const derivedPower = $derived(powerMode === 'current_voltage');
  const powerHeading = $derived(derivedPower ? 'Derived power' : 'Instantaneous power');

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

  function toolChips(tools: Tools | undefined): { label: string; on: boolean }[] {
    return [
      { label: tools?.tlp ? `TLP ${tools.tlp_version || ''}`.trim() : 'TLP', on: Boolean(tools?.tlp) },
      { label: 'UPower', on: Boolean(tools?.upower) },
      { label: 'ACPI', on: Boolean(tools?.acpi) },
      { label: 'tp-smapi', on: Boolean(tools?.tp_smapi) },
      { label: 'i8kutils', on: Boolean(tools?.i8kutils) },
    ];
  }
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
            <p class="font-mono text-[11px] uppercase tracking-[0.18em] text-mist">{powerHeading}</p>
            <p class="mt-1 font-mono text-4xl font-medium tracking-tight">
              {fmtNumber(power, 2)}
              <span class="text-lg text-mist">W</span>
            </p>
            <p class="mt-1 text-sm text-mist">
              {#if !present}
                No pack detected. Auto-fallback to mock is available with <span class="font-mono">--provider mock</span>.
              {:else if derivedPower}
                Calculated from <span class="font-mono">current_now × voltage_now</span>
                {#if tone === 'discharge'}
                  · drawing from the battery
                {:else if tone === 'charge'}
                  · charging the pack
                {/if}
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
        <p class="text-xs text-mist">{derivedPower ? 'Power estimate' : 'Raw power_now'}</p>
        <p class="mt-2 font-mono text-lg">
          {fmtNumber(derivedPower ? power : battery?.power_now_w, 2, ' W')}
        </p>
        {#if derivedPower}
          <p class="mt-1 text-[11px] text-mist">current_now × voltage_now</p>
        {/if}
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
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Temperature</p>
        <p class="mt-2 font-mono text-lg">{fmtNumber(battery?.temperature_c, 1, ' °C')}</p>
      </article>
      <article class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
        <p class="text-xs text-mist">Identity</p>
        <p class="mt-2 font-mono text-sm leading-snug">
          {battery?.manufacturer || capabilities?.battery?.manufacturer || '—'}
          {#if battery?.model_name || capabilities?.battery?.model}
            <span class="text-mist"> · {battery?.model_name || capabilities?.battery?.model}</span>
          {/if}
        </p>
      </article>
    </section>

    <section class="rounded-2xl border border-line bg-panel/70 px-4 py-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 class="text-sm font-medium">Capabilities</h2>
          <p class="mt-1 text-xs text-mist">
            Auto-discovered on this machine
            {#if capabilities?.hostname}
              · {capabilities.hostname}
            {/if}
            {#if capabilities?.os}
              · {capabilities.os}
            {/if}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <p class="font-mono text-[11px] text-mist">
            {updatedAt ? `updated ${updatedAt.toLocaleTimeString()}` : 'waiting'}
          </p>
          <button
            type="button"
            class="rounded-full border border-line px-3 py-1 text-xs text-mist transition hover:border-mist hover:text-snow disabled:opacity-50"
            onclick={() => void rescan()}
            disabled={scanning}
          >
            {scanning ? 'Scanning…' : 'Re-scan'}
          </button>
        </div>
      </div>

      <div class="mt-3 flex flex-wrap gap-2 text-[11px]">
        <span class="rounded-full border border-line px-3 py-1 font-mono text-mist">
          thresholds: {capabilities?.threshold_method ?? '—'}
        </span>
        {#if capabilities?.naming_convention}
          <span class="rounded-full border border-line px-3 py-1 font-mono text-mist">
            naming: {capabilities.naming_convention}
          </span>
        {/if}
        {#if capabilities?.power_calculation}
          <span class="rounded-full border border-line px-3 py-1 font-mono text-mist">
            power: {capabilities.power_calculation}
          </span>
        {/if}
        {#if capabilities?.kernel}
          <span class="rounded-full border border-line px-3 py-1 font-mono text-mist">
            {capabilities.kernel}
          </span>
        {/if}
      </div>

      <ul class="mt-4 space-y-3">
        {#each capabilities?.features ?? [] as feature}
          <li class="rounded-2xl border border-line bg-ink-soft/50 px-4 py-3">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <p class="text-sm font-medium">{feature.label}</p>
              {#if feature.enabled}
                <span class="rounded-full bg-mint/15 px-2.5 py-0.5 font-mono text-[11px] text-mint">Enabled ✓</span>
              {:else}
                <span class="rounded-full bg-rose/15 px-2.5 py-0.5 font-mono text-[11px] text-rose">Not supported</span>
              {/if}
            </div>
            {#if feature.method && feature.method !== 'none'}
              <p class="mt-1 font-mono text-[11px] text-sky">method: {feature.method} · {feature.detection_method}</p>
            {:else}
              <p class="mt-1 font-mono text-[11px] text-mist">{feature.detection_method}</p>
            {/if}
            <p class="mt-1 text-sm text-mist">{feature.recommendation}</p>
            {#if !feature.enabled && feature.why_not}
              <p class="mt-1 text-xs text-amber">{feature.why_not}</p>
            {/if}
          </li>
        {/each}
      </ul>

      <div class="mt-4">
        <h3 class="text-xs font-medium uppercase tracking-[0.16em] text-mist">Tools</h3>
        <div class="mt-2 flex flex-wrap gap-2">
          {#each toolChips(capabilities?.tools) as chip}
            <span
              class={`rounded-full px-3 py-1 font-mono text-[11px] ${chip.on ? 'bg-mint/10 text-mint' : 'bg-rose/10 text-rose'}`}
            >
              {chip.label}
            </span>
          {/each}
        </div>
      </div>

      <div class="mt-4">
        <h3 class="text-xs font-medium uppercase tracking-[0.16em] text-mist">Kernel modules</h3>
        <div class="mt-2 flex flex-wrap gap-2">
          {#if (capabilities?.kernel_modules ?? []).length === 0}
            <span class="text-xs text-mist">No vendor laptop modules detected</span>
          {:else}
            {#each capabilities?.kernel_modules ?? [] as mod}
              <span class="rounded-full bg-sky/10 px-3 py-1 font-mono text-[11px] text-sky">{mod}</span>
            {/each}
          {/if}
        </div>
      </div>

      <div class="mt-4">
        <h3 class="text-xs font-medium uppercase tracking-[0.16em] text-mist">Sysfs fields</h3>
        <div class="mt-2 flex flex-wrap gap-2">
          {#each capabilities?.available_fields ?? [] as field}
            <span class="rounded-full bg-mint/10 px-3 py-1 font-mono text-[11px] text-mint">{field}</span>
          {/each}
          {#each telemetry?.missing_fields ?? [] as field}
            <span class="rounded-full bg-rose/10 px-3 py-1 font-mono text-[11px] text-rose">{field} missing</span>
          {/each}
        </div>
      </div>
      {#if telemetry?.warnings?.length}
        <ul class="mt-3 space-y-1 text-xs text-amber">
          {#each telemetry.warnings as warning}
            <li>{warning}</li>
          {/each}
        </ul>
      {/if}
    </section>

    <ConfigPanel />
  </div>
</div>
