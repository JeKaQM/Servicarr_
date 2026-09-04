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
