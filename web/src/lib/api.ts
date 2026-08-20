import type { ActionPreflight, ActionResult, ActionStatus, AppConfig, AutoDrainConfig, AutoDrainStatus, AuthStatus, Capabilities, DiscoverReport, DockerConfig, EventsResponse, NotificationsConfig, PowerStatus, SafetyStatus, SafetyTestResult, ShutdownConfig, Telemetry } from './types';
import { authHeaders } from './auth';

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
      ...authHeaders(),
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

export function fetchAuthStatus(signal?: AbortSignal): Promise<AuthStatus> {
  return getJSON<AuthStatus>('/api/v1/auth/status', signal);
}

export function putConfig(body: {
  notifications: NotificationsConfig;
  shutdown: ShutdownConfig;
  docker: DockerConfig;
}): Promise<AppConfig> {
  return sendJSON<AppConfig>('PUT', '/api/v1/config', {
    notifications: notificationPayload(body.notifications),
    shutdown: body.shutdown,
    docker: body.docker,
  });
}

function notificationPayload(n: NotificationsConfig): Record<string, unknown> {
  const body: Record<string, unknown> = {
    provider: n.provider,
    enabled: n.enabled,
    dry_run: Boolean(n.dry_run),
  };
  if (n.webhook_url) {
    body.webhook_url = n.webhook_url;
  }
  if (n.chat_id) {
    body.chat_id = n.chat_id;
  }
  return body;
}

export type TestNotificationResult = {
  ok: boolean;
  provider?: string;
  dry_run?: boolean;
  error?: string;
};

export async function testNotification(): Promise<TestNotificationResult> {
  const res = await fetch('/api/v1/actions/test-notification', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: '{}',
  });
  const body = (await res.json().catch(() => ({}))) as TestNotificationResult & {
    error?: string;
    detail?: string;
  };
  if (!res.ok) {
    return {
      ok: false,
      provider: body.provider,
      dry_run: body.dry_run,
      error: body.error || body.detail || `test notification failed: ${res.status}`,
    };
  }
  return body;
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

export function fetchSafety(signal?: AbortSignal): Promise<SafetyStatus> {
  return getJSON<SafetyStatus>('/api/v1/safety', signal);
}

export function fetchAutoDrain(signal?: AbortSignal): Promise<AutoDrainStatus> {
  return getJSON<AutoDrainStatus>('/api/v1/auto-drain/status', signal);
}

export function putAutoDrainConfig(body: AutoDrainConfig): Promise<AutoDrainStatus> {
  return sendJSON<AutoDrainStatus>('PUT', '/api/v1/auto-drain/config', body);
}

export async function respondAutoDrain(action: 'yes' | 'no'): Promise<AutoDrainStatus> {
  const res = await fetch('/api/v1/auto-drain/respond', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: JSON.stringify({ action }),
  });
  const parsed = (await res.json().catch(() => ({}))) as AutoDrainStatus & { error?: string; detail?: string };
  if (!res.ok) {
    throw new Error(parsed.error || parsed.detail || `auto-drain respond failed: ${res.status}`);
  }
  return parsed;
}

export async function testSafety(scenario: 'warning' | 'critical'): Promise<SafetyTestResult> {
  const res = await fetch('/api/v1/safety/test', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: JSON.stringify({ scenario }),
  });
  const body = (await res.json().catch(() => ({}))) as SafetyTestResult & { detail?: string };
  if (!res.ok) {
    return {
      ok: false,
      dry_run: true,
      error: body.error || body.detail || `safety test failed: ${res.status}`,
      safety: body.safety,
    };
  }
  return body;
}

export function fetchActionStatus(signal?: AbortSignal): Promise<ActionStatus> {
  return getJSON<ActionStatus>('/api/v1/actions/status', signal);
}

export function fetchActionPreflight(signal?: AbortSignal): Promise<ActionPreflight> {
  return getJSON<ActionPreflight>('/api/v1/actions/preflight', signal);
}

export function previewActions(): Promise<ActionResult> {
  return sendJSON<ActionResult>('POST', '/api/v1/actions/preview', {});
}

export async function postPowerOff(): Promise<ActionResult> {
  return postManualAction('/api/v1/actions/poweroff', { confirm: 'POWER_OFF' });
}

export async function postDockerDrain(): Promise<ActionResult> {
  return postManualAction('/api/v1/actions/docker-drain', { confirm: 'STOP_DOCKER' });
}

async function postManualAction(path: string, body: unknown): Promise<ActionResult> {
  const res = await fetch(path, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: JSON.stringify(body),
  });
  const parsed = (await res.json().catch(() => ({}))) as ActionResult;
  if (!res.ok) {
    return {
      ok: false,
      dry_run: parsed.dry_run,
      real_enabled: parsed.real_enabled,
      commands_executed: false,
      plan: parsed.plan,
      gates: parsed.gates,
      error: parsed.error || parsed.detail || `${path} failed: ${res.status}`,
      detail: parsed.detail,
    };
  }
  return parsed;
}
