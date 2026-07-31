import { writable } from 'svelte/store';
import type { LiveInvalidationData } from '$lib/api/generated';

export type LiveStatus = 'live' | 'reconnecting' | 'disconnected';

export type LiveConnectionState = {
  status: LiveStatus;
  last_refresh: string | null;
};

export type LiveInvalidation = {
  id: string;
  event: string;
  data: LiveInvalidationData;
};

export type LiveOptions = {
  fetcher?: typeof fetch;
  onInvalidate: (fullResync: boolean) => void | Promise<void>;
  onUnauthorized?: () => void | Promise<void>;
  random?: () => number;
  baseDelayMS?: number;
  maximumDelayMS?: number;
  coalesceMS?: number;
};

export const liveConnection = writable<LiveConnectionState>({
  status: 'disconnected',
  last_refresh: null,
});

export function markMonitoringRefresh(value = new Date().toISOString()): void {
  liveConnection.update((current) => ({ ...current, last_refresh: value }));
}

export function startLiveInvalidations(options: LiveOptions): () => void {
  const fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
  const random = options.random ?? Math.random;
  const baseDelay = options.baseDelayMS ?? 1_000;
  const maximumDelay = options.maximumDelayMS ?? 30_000;
  const coalesceDelay = options.coalesceMS ?? 250;
  let stopped = false;
  let controller: AbortController | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let coalesceTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectAttempt = 0;
  let lastEventID = '';
  let fullResyncPending = false;

  const queueRefresh = (fullResync: boolean) => {
    fullResyncPending ||= fullResync;
    if (coalesceTimer) return;
    coalesceTimer = setTimeout(async () => {
      coalesceTimer = null;
      const refreshAll = fullResyncPending;
      fullResyncPending = false;
      await options.onInvalidate(refreshAll);
    }, coalesceDelay);
  };

  const connect = async () => {
    if (stopped) return;
    controller = new AbortController();
    const headers = new Headers({ Accept: 'text/event-stream' });
    if (lastEventID) headers.set('Last-Event-ID', lastEventID);
    try {
      const response = await fetcher('/api/v1/events/stream', {
        headers,
        credentials: 'same-origin',
        cache: 'no-store',
        signal: controller.signal,
      });
      if (response.status === 401 || response.status === 403) {
        stopped = true;
        liveConnection.update((current) => ({
          ...current,
          status: 'disconnected',
        }));
        await options.onUnauthorized?.();
        return;
      }
      if (!response.ok || !response.body) {
        throw new Error(`event stream returned ${response.status}`);
      }
      reconnectAttempt = 0;
      liveConnection.update((current) => ({ ...current, status: 'live' }));
      await consumeSSE(response.body, (event) => {
        if (event.id) lastEventID = event.id;
        if (event.event === 'resync_required') {
          lastEventID = '';
          queueRefresh(true);
          return;
        }
        queueRefresh(false);
      });
    } catch (error) {
      if (stopped || controller.signal.aborted) return;
    }
    if (stopped) return;
    reconnectAttempt += 1;
    liveConnection.update((current) => ({
      ...current,
      status: reconnectAttempt >= 5 ? 'disconnected' : 'reconnecting',
    }));
    const exponential = Math.min(
      maximumDelay,
      baseDelay * 2 ** Math.min(reconnectAttempt - 1, 10),
    );
    const delay = Math.max(0, Math.round(exponential * (0.8 + random() * 0.4)));
    reconnectTimer = setTimeout(connect, delay);
  };

  liveConnection.update((current) => ({
    ...current,
    status: 'reconnecting',
  }));
  void connect();

  return () => {
    stopped = true;
    controller?.abort();
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (coalesceTimer) clearTimeout(coalesceTimer);
    liveConnection.update((current) => ({
      ...current,
      status: 'disconnected',
    }));
  };
}

export async function consumeSSE(
  stream: ReadableStream<Uint8Array>,
  onEvent: (event: LiveInvalidation) => void,
): Promise<void> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let boundary = eventBoundary(buffer);
      while (boundary) {
        const block = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary.length);
        const event = parseEventBlock(block);
        if (event) onEvent(event);
        boundary = eventBoundary(buffer);
      }
      if (done) break;
    }
    const finalEvent = parseEventBlock(buffer);
    if (finalEvent) onEvent(finalEvent);
  } finally {
    reader.releaseLock();
  }
}

function eventBoundary(
  value: string,
): { index: number; length: number } | null {
  const match = /\r?\n\r?\n/.exec(value);
  return match ? { index: match.index, length: match[0].length } : null;
}

function parseEventBlock(block: string): LiveInvalidation | null {
  let id = '';
  let event = 'message';
  const data: string[] = [];
  for (const rawLine of block.split(/\r?\n/)) {
    if (!rawLine || rawLine.startsWith(':')) continue;
    const separator = rawLine.indexOf(':');
    const field = separator < 0 ? rawLine : rawLine.slice(0, separator);
    let value = separator < 0 ? '' : rawLine.slice(separator + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'id' && /^\d{1,20}$/.test(value)) id = value;
    if (field === 'event' && /^[a-z][a-z0-9_]{0,126}$/.test(value))
      event = value;
    if (field === 'data') data.push(value);
  }
  if (data.length === 0) return null;
  try {
    const parsed = JSON.parse(data.join('\n')) as LiveInvalidationData;
    if (parsed.schema_version !== 1 || typeof parsed.changed_at !== 'string')
      return null;
    return { id, event, data: parsed };
  } catch {
    return null;
  }
}
