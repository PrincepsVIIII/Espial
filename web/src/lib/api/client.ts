import type {
  APIErrorResponse,
  IntegrationAPIView,
  MonitoringOverview,
  ResourceAPIView,
} from './generated';

export type ResourceListResponse = {
  items: ResourceAPIView[];
  next_cursor?: string;
};

export type IntegrationListResponse = {
  items: IntegrationAPIView[];
  next_cursor?: string;
};

export type ClientProblem = {
  status: number;
  code: string;
  message: string;
  request_id?: string;
};

export class ApiFailure extends Error {
  readonly problem: ClientProblem;

  constructor(problem: ClientProblem) {
    super(problem.message);
    this.name = 'ApiFailure';
    this.problem = problem;
  }
}

export type MonitoringPayload = {
  overview: MonitoringOverview;
  resources: ResourceListResponse;
  integrations: IntegrationListResponse;
};

export async function requestJSON<T>(
  fetcher: typeof fetch,
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  headers.set('X-Request-ID', requestID());
  let response: Response;
  try {
    response = await fetcher(path, {
      ...init,
      headers,
      credentials: 'same-origin',
    });
  } catch {
    throw new ApiFailure({
      status: 0,
      code: 'core_unavailable',
      message: 'Espial Core could not be reached.',
    });
  }
  if (!response.ok) throw new ApiFailure(await responseProblem(response));
  try {
    return (await response.json()) as T;
  } catch {
    throw new ApiFailure({
      status: response.status,
      code: 'invalid_response',
      message: 'Espial Core returned an unreadable response.',
      request_id: response.headers.get('X-Request-ID') ?? undefined,
    });
  }
}

export function problemFrom(error: unknown): ClientProblem {
  if (error instanceof ApiFailure) return error.problem;
  return {
    status: 0,
    code: 'unexpected_error',
    message: 'Monitoring data could not be loaded.',
  };
}

async function responseProblem(response: Response): Promise<ClientProblem> {
  let body: APIErrorResponse | null = null;
  try {
    body = (await response.json()) as APIErrorResponse;
  } catch {
    // The proxy or an upstream may fail without a JSON API envelope.
  }
  const fallback = statusMessage(response.status);
  const code = safeText(body?.error?.code, 128) || fallback.code;
  const message = safeText(body?.error?.message, 512) || fallback.message;
  const requestID =
    safeText(body?.error?.request_id, 128) ||
    safeText(response.headers.get('X-Request-ID'), 128);
  return {
    status: response.status,
    code,
    message,
    ...(requestID ? { request_id: requestID } : {}),
  };
}

function statusMessage(
  status: number,
): Pick<ClientProblem, 'code' | 'message'> {
  if (status === 401)
    return { code: 'unauthenticated', message: 'Sign in is required.' };
  if (status === 403)
    return {
      code: 'forbidden',
      message: 'Your account does not have permission to view this data.',
    };
  if (status === 404)
    return {
      code: 'not_found',
      message: 'The requested record was not found.',
    };
  if (status >= 500)
    return {
      code: 'core_unavailable',
      message: 'Espial Core is temporarily unavailable.',
    };
  return {
    code: 'request_failed',
    message: 'The request could not be completed.',
  };
}

function safeText(value: string | null | undefined, maximum: number): string {
  if (!value || value.length > maximum || /[\r\n]/.test(value)) return '';
  return value;
}

let requestSequence = 0;

function requestID(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  requestSequence = (requestSequence + 1) % 1_000_000;
  return `web-${Date.now().toString(36)}-${requestSequence.toString(36)}`;
}
