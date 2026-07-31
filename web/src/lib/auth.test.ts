import { describe, expect, it } from 'vitest';
import { readCookie, safeReturnTo } from './auth';

describe('safeReturnTo', () => {
  it('keeps local paths', () =>
    expect(safeReturnTo('/datacenter?view=room')).toBe(
      '/datacenter?view=room',
    ));
  it.each(['https://evil.test', '//evil.test', null])(
    'rejects external or absent values',
    (value) => expect(safeReturnTo(value)).toBe('/dashboard'),
  );
});

it('reads an encoded cookie', () =>
  expect(readCookie('espial_csrf', 'other=1; espial_csrf=a%2Fb')).toBe('a/b'));
