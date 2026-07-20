/**
 * Tests for auth.js – getCsrf.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'resources.js', 'auth.js');
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

describe('handleButtonAction', () => {
  let consoleSpy;

  beforeEach(() => {
    document.body.innerHTML = '<button id="btn"></button>';
    consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleSpy.mockRestore();
  });

  test('passes parsed response message to custom error handler', async () => {
    const btn = document.getElementById('btn');
    const err = new Error('HTTP 502');
    err.body = { error: 'ups_unavailable', message: 'NUT does not know UPS "ups"' };
    const onError = jest.fn();

    await handleButtonAction(
      btn,
      async () => {
        throw err;
      },
      'ok',
      onError
    );

    expect(onError).toHaveBeenCalledWith(err, 'NUT does not know UPS "ups"');
    expect(btn.disabled).toBe(false);
    expect(btn.classList.contains('loading')).toBe(false);
  });
});
