import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const stylesheet = readFileSync(
  new URL('../styles.css', import.meta.url),
  'utf8',
);
const loginPage = readFileSync(
  new URL('../routes/login/+page.svelte', import.meta.url),
  'utf8',
);
const appShell = readFileSync(
  new URL('../routes/(app)/+layout.svelte', import.meta.url),
  'utf8',
);

function token(name: string): string {
  const match = stylesheet.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`));
  if (!match) throw new Error(`Missing color token --${name}`);
  return match[1];
}

function luminance(hex: string): number {
  const channels = hex
    .slice(1)
    .match(/.{2}/g)!
    .map((value) => Number.parseInt(value, 16) / 255)
    .map((value) =>
      value <= 0.04045 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4),
    );
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrast(foreground: string, background: string): number {
  const light = Math.max(luminance(foreground), luminance(background));
  const dark = Math.min(luminance(foreground), luminance(background));
  return (light + 0.05) / (dark + 0.05);
}

describe('UBNetDef design tokens', () => {
  it('keeps the official UB brand anchors', () => {
    expect(token('ub-blue')).toBe('#005bbb');
    expect(token('harriman-blue')).toBe('#002f56');
    expect(token('hayes-white')).toBe('#ffffff');
  });

  it('keeps UBNetDef attribution in both entry points', () => {
    expect(loginPage).toContain('UBNetDef Infrastructure Operations');
    expect(appShell).toContain('UBNetDef Operations');
  });

  it('does not regress to the rejected template effects', () => {
    expect(stylesheet).toContain('color-scheme: dark');
    expect(stylesheet).not.toMatch(/gradient\s*\(/i);
    expect(stylesheet).not.toMatch(/box-shadow\s*:/i);
    expect(stylesheet).not.toMatch(/backdrop-filter\s*:/i);
  });

  it.each([
    ['text on canvas', 'text', 'canvas'],
    ['text on surface', 'text', 'surface-1'],
    ['muted text on canvas', 'text-muted', 'canvas'],
    ['muted text on surface', 'text-muted', 'surface-1'],
    ['white action text on UB Blue', 'hayes-white', 'ub-blue'],
    ['critical text on critical surface', 'critical', 'critical-surface'],
    ['healthy text on healthy surface', 'healthy', 'healthy-surface'],
  ])('%s meets WCAG AA for ordinary text', (_, foreground, background) => {
    expect(
      contrast(token(foreground), token(background)),
    ).toBeGreaterThanOrEqual(4.5);
  });
});
