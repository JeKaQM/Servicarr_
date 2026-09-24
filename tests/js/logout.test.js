/**
 * Regression tests for the logout flow.
 *
 * HandleLogout (app/internal/handlers/auth_handlers.go) rejects
 * POST /api/logout with 403 unless the X-CSRF-Token header matches the
 * csrf cookie. logout() in day-detail.js used to send no CSRF header, so
 * the server never cleared the session cookie and the post-logout page
 * reload logged the user straight back in. These tests pin the header.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'resources.js', 'day-detail.js');
});

beforeEach(() => {
  document.cookie = 'csrf=; Max-Age=0';
});

function jsonResponse() {
  return Promise.resolve({
    ok: true,
    status: 200,
    headers: { get: () => 'application/json' },
    json: () => Promise.resolve({ ok: true })
  });
}

/* ── j() CSRF auto-injection ─────────────────────────────── */
describe('j() CSRF header', () => {
  let fetchSpy;

  beforeEach(() => {
    fetchSpy = jest.fn(jsonResponse);
    global.fetch = fetchSpy;
  });

  afterEach(() => {
    delete global.fetch;
  });

  test('attaches X-CSRF-Token to POST requests', async () => {
    document.cookie = 'csrf=abc123';
    await j('/api/example', { method: 'POST' });
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.headers['X-CSRF-Token']).toBe('abc123');
  });

  test('does not attach X-CSRF-Token to GET requests', async () => {
    document.cookie = 'csrf=abc123';
    await j('/api/check');
    const [, opts] = fetchSpy.mock.calls[0];
    expect((opts.headers || {})['X-CSRF-Token']).toBeUndefined();
  });

  test('keeps an explicitly provided token', async () => {
    document.cookie = 'csrf=cookie-token';
    await j('/api/login', {
      method: 'POST',
      headers: { 'X-CSRF-Token': 'explicit-token' }
    });
    const [, opts] = fetchSpy.mock.calls[0];
    expect(opts.headers['X-CSRF-Token']).toBe('explicit-token');
  });
});

/* ── logout() ────────────────────────────────────────────── */
describe('logout()', () => {
  let fetchSpy;
  let consoleSpy;

  beforeEach(() => {
    fetchSpy = jest.fn(jsonResponse);
    global.fetch = fetchSpy;
    // jsdom logs "Not implemented: navigation" for location.reload()
    consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    delete global.fetch;
    consoleSpy.mockRestore();
  });

  test('sends POST /api/logout with the CSRF token header', async () => {
    document.cookie = 'csrf=tok-123';
    await logout();

    const [url, opts] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/logout');
    expect(opts.method).toBe('POST');
    expect(opts.headers['X-CSRF-Token']).toBe('tok-123');
  });

  test('still completes when the logout request fails (page reloads anyway)', async () => {
    global.fetch = jest.fn(() => Promise.reject(new Error('HTTP 403')));
    await expect(logout()).resolves.toBeUndefined();
  });
});