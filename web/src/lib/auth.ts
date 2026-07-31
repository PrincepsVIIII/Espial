export type User = {
  id: string;
  username: string;
  display_name: string;
  roles: string[];
  permissions: string[];
};

export type SessionResponse = {
  user: User;
  expires_at: string;
  capabilities: { local: boolean; sso: boolean };
};

export function safeReturnTo(value: string | null): string {
  if (!value || !value.startsWith('/') || value.startsWith('//'))
    return '/dashboard';
  try {
    const parsed = new URL(value, 'http://espial.local');
    return parsed.origin === 'http://espial.local'
      ? parsed.pathname + parsed.search + parsed.hash
      : '/dashboard';
  } catch {
    return '/dashboard';
  }
}

export function readCookie(
  name: string,
  cookieHeader = document.cookie,
): string {
  const prefix = `${encodeURIComponent(name)}=`;
  const match = cookieHeader
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  return match ? decodeURIComponent(match.slice(prefix.length)) : '';
}
