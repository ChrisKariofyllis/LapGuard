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
