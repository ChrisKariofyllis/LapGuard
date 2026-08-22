<script lang="ts">
  import PowerPanel from '../PowerPanel.svelte';
  import {
    abs,
    fmtEstimatedRuntime,
    fmtInt,
    fmtNumber,
    healthTone,
    powerHeading as batteryPowerHeading,
    statusLabel,
    statusTone,
    type StatusTone,
  } from '../format';
  import type { Capabilities, Telemetry, Tools } from '../types';

  let {
    telemetry,
    capabilities,
    updatedAt,
    scanning,
    onrescan,
  }: {
    telemetry: Telemetry | null;
    capabilities: Capabilities | null;
    updatedAt: Date | null;
    scanning: boolean;
    onrescan: () => void;
  } = $props();

  const battery = $derived(telemetry?.battery);
  const present = $derived(battery?.present ?? false);
  const capacity = $derived(battery?.capacity_percent);
  const tone = $derived(statusTone(battery?.status, present));
  const power = $derived(battery?.battery_power_w ?? abs(battery?.power_w));
  const powerMode = $derived(battery?.power_calculation ?? capabilities?.power_calculation);
  const derivedPower = $derived(powerMode === 'current_voltage');
  const powerHeading = $derived(batteryPowerHeading(battery));
  const powerDirection = $derived(battery?.power_direction);

  const ring = $derived.by(() => {
    const pct = Math.max(0, Math.min(100, capacity ?? 0));
    const radius = 88;
    const circ = 2 * Math.PI * radius;
    return { pct, radius, circ, dash: circ * (pct / 100) };
  });

  const toneClass: Record<StatusTone, string> = {
    charge: 'text-mint',
    discharge: 'text-amber',
    full: 'text-mint',
    idle: 'text-sky',
    missing: 'text-rose',
  };

  const ringClass: Record<StatusTone, string> = {
    charge: 'stroke-mint text-mint lg-ring-glow',
    discharge: 'stroke-amber text-amber lg-ring-glow',
    full: 'stroke-mint text-mint lg-ring-glow',
    idle: 'stroke-sky text-sky lg-ring-glow',
    missing: 'stroke-rose text-rose lg-ring-glow',
  };

  const statusBadge: Record<StatusTone, string> = {
    charge: 'lg-badge--ok',
    discharge: 'lg-badge--warn',
    full: 'lg-badge--ok',
    idle: 'lg-badge--info',
    missing: 'lg-badge--err',
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

<div class="flex flex-col gap-6">
  <section class="lg-card">
    <div class="flex flex-col items-center gap-8 sm:flex-row sm:items-center sm:justify-between">
      <div class={`lg-gauge lg-gauge--${tone} relative grid place-items-center`}>
        <svg width="240" height="240" viewBox="0 0 220 220" aria-hidden="true">
          <circle cx="110" cy="110" r={ring.radius} class="lg-ring-track fill-none" stroke-width="12" />
          <circle
            cx="110"
            cy="110"
            r={ring.radius}
            class={`fill-none ${ringClass[tone]}`}
            stroke-width="12"
            stroke-linecap="round"
            stroke-dasharray={`${ring.dash} ${ring.circ}`}
            transform="rotate(-90 110 110)"
          />
        </svg>
        <div class="absolute inset-0 grid place-items-center text-center">
          <div>
            <p class="lg-hero-num font-mono text-6xl font-medium">
              {capacity === undefined ? '—' : capacity}<span class="text-2xl font-normal text-mist">%</span>
            </p>
            <p class={`lg-badge mt-3 ${statusBadge[tone]}`}>
              {#if tone === 'charge' || tone === 'full'}
                <svg class="h-3 w-3" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M9.2 1.2 4 8.4h3.2L6.8 14.8 12 7.6H8.8z"/></svg>
              {:else if tone === 'discharge'}
                <svg class="h-3 w-3" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="2.5" y="4.5" width="9.5" height="7" rx="1.4"/><path d="M13.5 7v2M5 8h4"/></svg>
              {:else}
                <span class={`h-1.5 w-1.5 rounded-full ${toneClass[tone].replace('text-', 'bg-')}`}></span>
              {/if}
              {statusLabel(battery?.status, present)}
            </p>
          </div>
        </div>
      </div>

      <div class="w-full flex-1 space-y-4">
        <div>
          <p class="lg-stat-label">{powerHeading}</p>
          <p class="lg-hero-num mt-1 font-mono text-4xl font-medium tracking-tight">
            {fmtNumber(power, 2)}
            <span class="text-lg font-normal text-mist">W</span>
          </p>
          <p class="mt-2 text-sm leading-relaxed text-mist">
            {#if !present}
              No pack detected. Auto-fallback to mock is available with <span class="font-mono">--provider mock</span>.
            {:else}
              {#if derivedPower && powerDirection !== 'unknown'}
                Calculated from <span class="font-mono">current_now × voltage_now</span>.
              {/if}
              {#if powerDirection === 'charge'}
                Charging power into the battery, not total system consumption.
              {:else if powerDirection === 'discharge'}
                Power drawn from the battery.
              {:else if powerDirection === 'idle'}
                Pack at rest (Full / Not charging).
              {:else}
                Battery-side power is unavailable.
              {/if}
            {/if}
          </p>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="lg-card-nested">
            <p class="lg-stat-label">Health</p>
            <p class={`lg-stat-value mt-2 ${toneClass[healthTone(battery?.health_percent)]}`}>
              {fmtNumber(battery?.health_percent, 1, '%')}
            </p>
          </div>
          <div class="lg-card-nested">
            <p class="lg-stat-label">Cycles</p>
            <p class="lg-stat-value mt-2">{fmtInt(battery?.cycle_count)}</p>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section class="grid grid-cols-2 gap-4 sm:grid-cols-3">
    <article class="lg-card-nested">
      <p class="lg-stat-label">Voltage</p>
      <p class="lg-stat-value mt-2">{fmtNumber(battery?.voltage_now_v, 3, ' V')}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Current</p>
      <p class="lg-stat-value mt-2">{fmtNumber(battery?.current_now_a, 3, ' A')}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">{powerHeading}</p>
      <p class="lg-stat-value mt-2">{fmtNumber(power, 2, ' W')}</p>
      {#if derivedPower}
        <p class="mt-1 text-[11px] leading-relaxed text-mist">current_now × voltage_now</p>
      {/if}
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Energy full</p>
      <p class="lg-stat-value mt-2">{fmtNumber(battery?.energy_full_wh, 1, ' Wh')}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Design full</p>
      <p class="lg-stat-value mt-2">{fmtNumber(battery?.energy_full_design_wh, 1, ' Wh')}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Pack</p>
      <p class="lg-stat-value mt-2">{battery?.name ?? '—'}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Temperature</p>
      <p class="lg-stat-value mt-2">{fmtNumber(battery?.temperature_c, 1, ' °C')}</p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Identity</p>
      <p class="mt-2 font-mono text-sm leading-snug">
        {battery?.manufacturer || capabilities?.battery?.manufacturer || '—'}
        {#if battery?.model_name || capabilities?.battery?.model}
          <span class="text-mist"> · {battery?.model_name || capabilities?.battery?.model}</span>
        {/if}
      </p>
    </article>
    <article class="lg-card-nested">
      <p class="lg-stat-label">Estimated time left</p>
      <p class="lg-stat-value mt-2">
        {fmtEstimatedRuntime(battery?.estimated_runtime_seconds, battery?.estimated_runtime_available)}
      </p>
      {#if battery?.estimated_runtime_available}
        <p class="mt-1 text-[11px] leading-relaxed text-mist">Based on current battery usage</p>
      {:else}
        <p class="mt-1 text-[11px] leading-relaxed text-mist">
          {battery?.estimated_runtime_reason ?? 'Available while discharging'}
        </p>
      {/if}
    </article>
  </section>

  <PowerPanel />

  <section class="lg-card">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <h2 class="text-[22px] font-medium tracking-tight">Capabilities</h2>
        <p class="mt-1 text-sm text-mist">
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
        <button type="button" class="lg-btn lg-btn-secondary" onclick={() => onrescan()} disabled={scanning}>
          {scanning ? 'Scanning…' : 'Re-scan'}
        </button>
      </div>
    </div>

    <div class="mt-3 flex flex-wrap gap-2 text-[11px]">
      <span class="lg-badge font-mono">thresholds: {capabilities?.threshold_method ?? '—'}</span>
      {#if capabilities?.naming_convention}
        <span class="lg-badge font-mono">naming: {capabilities.naming_convention}</span>
      {/if}
      {#if capabilities?.power_calculation}
        <span class="lg-badge font-mono">power: {capabilities.power_calculation}</span>
      {/if}
      {#if capabilities?.kernel}
        <span class="lg-badge font-mono">{capabilities.kernel}</span>
      {/if}
    </div>

    <ul class="mt-4 space-y-3">
      {#each capabilities?.features ?? [] as feature}
        <li class="lg-card-nested">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <p class="text-sm font-medium">{feature.label}</p>
            {#if feature.enabled}
              <span class="lg-badge lg-badge--ok font-mono text-[11px]">Enabled</span>
            {:else}
              <span class="lg-badge lg-badge--err font-mono text-[11px]">Not supported</span>
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
      <h3 class="text-[13px] font-medium tracking-[0.4px] text-mist uppercase">Tools</h3>
      <div class="mt-2 flex flex-wrap gap-2">
        {#each toolChips(capabilities?.tools) as chip}
          <span class={`lg-badge font-mono text-[11px] ${chip.on ? 'lg-badge--ok' : 'lg-badge--err'}`}>
            {chip.label}
          </span>
        {/each}
      </div>
    </div>

    <div class="mt-4">
      <h3 class="text-[13px] font-medium tracking-[0.4px] text-mist uppercase">Kernel modules</h3>
      <div class="mt-2 flex flex-wrap gap-2">
        {#if (capabilities?.kernel_modules ?? []).length === 0}
          <span class="text-xs text-mist">No vendor laptop modules detected</span>
        {:else}
          {#each capabilities?.kernel_modules ?? [] as mod}
            <span class="lg-badge lg-badge--info font-mono text-[11px]">{mod}</span>
          {/each}
        {/if}
      </div>
    </div>

    <div class="mt-4">
      <h3 class="text-[13px] font-medium tracking-[0.4px] text-mist uppercase">Sysfs fields</h3>
      <div class="mt-2 flex flex-wrap gap-2">
        {#each capabilities?.available_fields ?? [] as field}
          <span class="lg-badge lg-badge--ok font-mono text-[11px]">{field}</span>
        {/each}
        {#each telemetry?.missing_fields ?? [] as field}
          <span class="lg-badge lg-badge--err font-mono text-[11px]">{field} missing</span>
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
</div>
