/**
 * Tests for auth.js – getCsrf.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'resources.js', 'admin-ui.js', 'auth.js');
});

/* ── getCsrf ────────────────────────────────────────────── */
describe('getCsrf', () => {
  afterEach(() => {
    // Reset cookies
    document.cookie = 'csrf=; Max-Age=0';
  });

  test('returns csrf token from cookie', () => {
    document.cookie = 'csrf=abc123';
    expect(getCsrf()).toBe('abc123');
  });

  test('returns empty string when no csrf cookie', () => {
    // Clear all cookies
    document.cookie.split(';').forEach(c => {
      const name = c.split('=')[0].trim();
      document.cookie = `${name}=; Max-Age=0`;
    });
    expect(getCsrf()).toBe('');
  });

  test('finds csrf among multiple cookies', () => {
    document.cookie = 'session=xyz';
    document.cookie = 'csrf=mytoken';
    document.cookie = 'other=val';
    expect(getCsrf()).toBe('mytoken');
  });
});
