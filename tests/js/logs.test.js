const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'logs-tab.js');
});

beforeEach(() => {
  document.body.innerHTML = '';
});

describe('audit log presentation', () => {
  test('summarizes structured audit context for scanning', () => {
    const summary = summarizeLogDetails(JSON.stringify({
      actor: 'admin', outcome: 'success', status: 200, ip: '127.0.0.1', duration_ms: 12
    }));

    expect(summary).toBe('User: admin | Success | HTTP 200 | 127.0.0.1');
  });

  test('renders audit entries in a distinct category', () => {
    const html = renderLogEntry({
      timestamp: '2026-09-04T12:00:00Z',
      level: 'info',
      category: 'audit',
      message: 'Service card refreshed',
      details: JSON.stringify({ actor: 'admin', outcome: 'success', status: 200 })
    });

    expect(html).toContain('category-audit');
    expect(html).toContain('User Action');
    expect(html).toContain('User: admin | Success | HTTP 200');
  });

  test('leaves legacy plain-text details readable', () => {
    expect(summarizeLogDetails('status=200, latency=20ms')).toBe('status=200, latency=20ms');
  });
});

describe('errors and warnings highlights', () => {
  afterEach(() => {
    delete global.j;
  });

  test('queries both levels directly and shows the newest first', async () => {
    document.body.innerHTML = '<div id="errorLogsList"></div>';
    global.j = jest.fn(async (url) => {
      if (url.includes('level=error')) {
        return { logs: [{ id: 3, timestamp: '2026-10-01 09:00:00', level: 'error', category: 'check', message: 'Service check failed' }] };
      }
      return { logs: [{ id: 7, timestamp: '2026-10-01 10:00:00', level: 'warn', category: 'audit', message: 'Login failed' }] };
    });

    await loadErrorHighlights();

    expect(global.j).toHaveBeenCalledWith('/api/admin/logs?limit=10&level=error');
    expect(global.j).toHaveBeenCalledWith('/api/admin/logs?limit=10&level=warn');
    const text = document.querySelector('#errorLogsList').textContent;
    expect(text).toContain('Login failed');
    expect(text).toContain('Service check failed');
    expect(text.indexOf('Login failed')).toBeLessThan(text.indexOf('Service check failed'));
  });
});
