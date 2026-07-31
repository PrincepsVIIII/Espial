import { describe, expect, it, vi } from 'vitest';
import {
  ApiFailure,
  requestJSON,
  requestJSONWithMetadata,
  requestVoidWithMetadata,
} from './client';

describe('typed API client', () => {
  it('uses same-origin credentials and sends a request ID', async () => {
    const fetcher = vi.fn(async (_path: string, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(init?.credentials).toBe('same-origin');
      expect(headers.get('X-Request-ID')).toBeTruthy();
      return Response.json({ value: 4 });
    }) as unknown as typeof fetch;
    await expect(
      requestJSON<{ value: number }>(fetcher, '/api/test'),
    ).resolves.toEqual({ value: 4 });
  });

  it('maps a structured Core failure without exposing malformed text', async () => {
    const fetcher = vi.fn(async () =>
      Response.json(
        {
          error: {
            code: 'forbidden',
            message: 'Permission denied.',
            request_id: 'request-7',
          },
        },
        { status: 403 },
      ),
    ) as unknown as typeof fetch;
    await expect(requestJSON(fetcher, '/api/test')).rejects.toMatchObject({
      problem: {
        status: 403,
        code: 'forbidden',
        message: 'Permission denied.',
        request_id: 'request-7',
      },
    } satisfies Partial<ApiFailure>);
  });

  it('maps a network failure to Core unavailable', async () => {
    const fetcher = vi.fn(async () => {
      throw new Error('socket details must not escape');
    }) as unknown as typeof fetch;
    await expect(requestJSON(fetcher, '/api/test')).rejects.toMatchObject({
      problem: { code: 'core_unavailable', status: 0 },
    });
  });

  it('returns mutation receipt metadata without adding it to the payload', async () => {
    const fetcher = vi.fn(async () =>
      Response.json(
        { username: 'viewer' },
        {
          headers: {
            'X-Request-ID': 'user-change-7',
            ETag: '"2026-07-31T12:00:00Z"',
          },
        },
      ),
    ) as unknown as typeof fetch;
    await expect(
      requestJSONWithMetadata<{ username: string }>(fetcher, '/api/v1/users'),
    ).resolves.toEqual({
      data: { username: 'viewer' },
      request_id: 'user-change-7',
      etag: '"2026-07-31T12:00:00Z"',
    });
  });

  it('returns the request ID from an empty successful mutation', async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(null, {
          status: 204,
          headers: { 'X-Request-ID': 'password-change-8' },
        }),
    ) as unknown as typeof fetch;
    await expect(
      requestVoidWithMetadata(fetcher, '/api/v1/users/user/password'),
    ).resolves.toEqual({ request_id: 'password-change-8' });
  });
});
