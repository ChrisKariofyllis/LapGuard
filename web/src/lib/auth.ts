const TOKEN_KEY = 'lapguard.apiToken';

export function getAPIToken(): string {
  try {
    return sessionStorage.getItem(TOKEN_KEY) ?? '';
  } catch {
    return '';
  }
}

export function setAPIToken(token: string): void {
  try {
    const trimmed = token.trim();
    if (!trimmed) {
      sessionStorage.removeItem(TOKEN_KEY);
      return;
    }
    sessionStorage.setItem(TOKEN_KEY, trimmed);
  } catch {
    /* private mode */
  }
}

export function authHeaders(): Record<string, string> {
  const token = getAPIToken();
  if (!token) {
    return {};
  }
  return { Authorization: `Bearer ${token}` };
}
