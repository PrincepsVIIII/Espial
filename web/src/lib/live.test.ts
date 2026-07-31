import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import {
  consumeSSE,
  liveConnection,
  startLiveInvalidations,
  type LiveInvalidation,
} from './live';

const encoder = new TextEncoder();

it('parses bounded SSE frames across chunk boundaries and ignores comments', async () => {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(
        encoder.encode(': heartbeat\n\nid: 7\nevent: resource_'),
      );
      controller.enqueue(
        encoder.encode(
          'state_changed\ndata: {"schema_version":1,"resource_id":"r1","changed_at":"2026-07-31T12:00:00Z"}\n\n',
        ),
      );
      controller.close();
    },
  });
  const events: LiveInvalidation[] = [];
  await consumeSSE(stream, (event) => events.push(event));
  expect(events).toHaveLength(1);
  expect(events[0]).toMatchObject({ id: '7', event: 'resource_state_changed' });
});

it('reconnects with Last-Event-ID and coalesces invalidations', async () => {
  let calls = 0;
  let secondController: ReadableStreamDefaultController<Uint8Array> | undefined;
  const fetcher = vi.fn(async (_path: string, init?: RequestInit) => {
    calls += 1;
    if (calls === 1) {
      return new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'id: 9\nevent: integration_changed\ndata: {"schema_version":1,"integration_id":"i1","changed_at":"2026-07-31T12:00:00Z"}\n\n',
              ),
            );
            controller.close();
          },
        }),
        { status: 200 },
      );
    }
    expect(new Headers(init?.headers).get('Last-Event-ID')).toBe('9');
    return new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          secondController = controller;
        },
      }),
      { status: 200 },
    );
  }) as unknown as typeof fetch;
  const refreshed = vi.fn();
  const stop = startLiveInvalidations({
    fetcher,
    onInvalidate: refreshed,
    baseDelayMS: 0,
    maximumDelayMS: 0,
    coalesceMS: 0,
    random: () => 0,
  });
  await vi.waitFor(() => expect(calls).toBe(2));
  await vi.waitFor(() => expect(get(liveConnection).status).toBe('live'));
  await vi.waitFor(() => expect(refreshed).toHaveBeenCalledTimes(1));
  stop();
  secondController?.close();
});
