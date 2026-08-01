import { readCookie } from '$lib/auth';
import { requestJSON } from '$lib/api/client';
import type { AdministrativeMutationReceipt } from '$lib/api/generated';

export function versionETag(version: number): string {
  return `"v${version.toString(16)}"`;
}

export function newIdempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;
}

export function administrativeMutation<T>(
  path: string,
  method: 'POST' | 'PUT',
  body: T | undefined,
  version?: number,
): Promise<AdministrativeMutationReceipt> {
  const headers = new Headers({
    'X-CSRF-Token': readCookie('espial_csrf'),
    'Idempotency-Key': newIdempotencyKey(),
  });
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  if (version !== undefined) headers.set('If-Match', versionETag(version));
  return requestJSON<AdministrativeMutationReceipt>(fetch, path, {
    method,
    headers,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}
