import type { Capabilities, Telemetry } from './types';

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, {
    headers: { Accept: 'application/json' },
    signal,
  });
  if (!res.ok) {
    throw new Error(`${path} failed: ${res.status}`);
  }
  return res.json() as Promise<T>;
}

export function fetchTelemetry(signal?: AbortSignal): Promise<Telemetry> {
  return getJSON<Telemetry>('/api/v1/telemetry', signal);
}

export function fetchCapabilities(signal?: AbortSignal): Promise<Capabilities> {
  return getJSON<Capabilities>('/api/v1/capabilities', signal);
}
