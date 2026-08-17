export function fmtNumber(value: number | undefined, digits: number, suffix = ''): string {
  if (value === undefined || Number.isNaN(value)) {
    return '—';
  }
  return `${value.toFixed(digits)}${suffix}`;
}

export function fmtInt(value: number | undefined): string {
  if (value === undefined || Number.isNaN(value)) {
    return '—';
  }
  return String(value);
}

export function abs(value: number | undefined): number | undefined {
  return value === undefined ? undefined : Math.abs(value);
}

export function statusLabel(status: string | undefined, present: boolean): string {
  if (!present) {
    return 'No battery';
  }
  if (!status) {
    return 'Unknown';
  }
  return status;
}

export type StatusTone = 'charge' | 'discharge' | 'full' | 'idle' | 'missing';

export function statusTone(status: string | undefined, present: boolean): StatusTone {
  if (!present) {
    return 'missing';
  }
  switch ((status ?? '').toLowerCase()) {
    case 'charging':
      return 'charge';
    case 'discharging':
      return 'discharge';
    case 'full':
      return 'full';
    default:
      return 'idle';
  }
}

export function healthTone(health: number | undefined): StatusTone {
  if (health === undefined) {
    return 'idle';
  }
  if (health >= 80) {
    return 'full';
  }
  if (health >= 60) {
    return 'charge';
  }
  return 'discharge';
}

/** Friendly remaining-time label from estimated_runtime_seconds. */
export function fmtEstimatedRuntime(seconds: number | null | undefined, available?: boolean): string {
  if (!available || seconds === undefined || seconds === null || !Number.isFinite(seconds) || seconds <= 0) {
    return '—';
  }
  const s = Math.round(seconds);
  if (s < 3600) {
    const min = Math.max(1, Math.round(s / 60));
    return `${min} min`;
  }
  if (s < 86400) {
    const h = Math.floor(s / 3600);
    const m = Math.round((s % 3600) / 60);
    if (m === 60) {
      return `${h + 1}h`;
    }
    if (m === 0) {
      return `${h}h`;
    }
    return `${h}h ${m}m`;
  }
  const d = Math.floor(s / 86400);
  const h = Math.round((s % 86400) / 3600);
  if (h === 24) {
    return `${d + 1}d`;
  }
  if (h === 0) {
    return `${d}d`;
  }
  return `${d}d ${h}h`;
}
