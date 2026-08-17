import type { AppConfig, Capabilities, DiscoverReport, DockerConfig, EventsResponse, NotificationsConfig, PowerStatus, ShutdownConfig, Telemetry } from './types';

async function readError(path: string, res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string; detail?: string };
    if (body.detail) {
      return body.detail;
    }
    if (body.error) {
      return body.error;
    }
  } catch {
    /* ignore non-JSON errors */
  }
  return `${path} failed: ${res.status}`;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, {
    headers: { Accept: 'application/json' },
    signal,
  });
  if (!res.ok) {
    throw new Error(await readError(path, res));
  }
  return res.json() as Promise<T>;
}

async function sendJSON<T>(method: string, path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(await readError(path, res));
  }
  return res.json() as Promise<T>;
}

export function fetchTelemetry(signal?: AbortSignal): Promise<Telemetry> {
  return getJSON<Telemetry>('/api/v1/telemetry', signal);
}

export function fetchCapabilities(signal?: AbortSignal): Promise<Capabilities> {
  return getJSON<Capabilities>('/api/v1/capabilities', signal);
}

export function fetchDiscover(signal?: AbortSignal): Promise<DiscoverReport> {
  return getJSON<DiscoverReport>('/api/v1/discover', signal);
}

export function fetchConfig(signal?: AbortSignal): Promise<AppConfig> {
  return getJSON<AppConfig>('/api/v1/config', signal);
}

export function putConfig(body: {
  notifications: NotificationsConfig;
  shutdown: ShutdownConfig;
  docker: DockerConfig;
}): Promise<AppConfig> {
  return sendJSON<AppConfig>('PUT', '/api/v1/config', body);
}

export function postNotifications(body: NotificationsConfig): Promise<AppConfig> {
  return sendJSON<AppConfig>('POST', '/api/v1/config/notifications', body);
}

export function postShutdown(body: ShutdownConfig): Promise<AppConfig> {
  return sendJSON<AppConfig>('POST', '/api/v1/config/shutdown', body);
}

export function fetchPower(signal?: AbortSignal): Promise<PowerStatus> {
  return getJSON<PowerStatus>('/api/v1/power', signal);
}

export function fetchEvents(limit = 50, type?: string, signal?: AbortSignal): Promise<EventsResponse> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  if (type) {
    params.set('type', type);
  }
  return getJSON<EventsResponse>(`/api/v1/events?${params.toString()}`, signal);
}
