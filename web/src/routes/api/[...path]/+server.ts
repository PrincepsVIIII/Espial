import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const proxy: RequestHandler = async ({ request, params, url, fetch }) => {
  const coreURL = env.ESPIAL_CORE_URL ?? 'http://127.0.0.1:8080';
  const target = `${coreURL}/api/${params.path}${url.search}`;
  const headers = new Headers(request.headers);
  headers.delete('host');
  const response = await fetch(target, {
    method: request.method,
    headers,
    body:
      request.method === 'GET' || request.method === 'HEAD'
        ? undefined
        : request.body,
    duplex: 'half',
  } as RequestInit);
  return new Response(response.body, {
    status: response.status,
    headers: response.headers,
  });
};

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
