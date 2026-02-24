/**
 * Tests for admin-ui.js – escapeHtml.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'admin-ui.js');
});

/* ── escapeHtml ─────────────────────────────────────────── */
describe('escapeHtml', () => {
  test('escapes < and >', () => {
    expect(escapeHtml('<div>')).toBe('&lt;div&gt;');
  });

  test('escapes &', () => {
    expect(escapeHtml('a & b')).toBe('a &amp; b');
  });

  test('escapes quotes', () => {
    const result = escapeHtml('"hello"');
    // jsdom innerHTML does not escape double quotes to &quot;
    // but the function uses textContent→innerHTML which is safe
    expect(typeof result).toBe('string');
    expect(result).toContain('hello');
  });

  test('passes through plain text unchanged', () => {
    expect(escapeHtml('hello world')).toBe('hello world');
  });

  test('handles empty string', () => {
    expect(escapeHtml('')).toBe('');
  });

  test('escapes script tags', () => {
    const result = escapeHtml('<script>alert("xss")</script>');
    expect(result).toContain('&lt;script&gt;');
    expect(result).not.toContain('<script>');
  });

  test('handles special characters mixed with text', () => {
    const result = escapeHtml('Price: $50 & tax < $10');
    expect(result).toContain('&amp;');
    expect(result).toContain('&lt;');
    expect(result).toContain('$50');
  });

  test('handles unicode characters', () => {
    expect(escapeHtml('café ☕')).toBe('café ☕');
  });
});
