/**
 * Tests for services.js – getServiceLabel, getServiceIconHtml, SERVICE_ICONS.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'resources.js', 'services.js');
});

/* ── getServiceLabel ────────────────────────────────────── */
describe('getServiceLabel', () => {
  test('plex → "Media Server"', () => {
    expect(getServiceLabel('plex')).toBe('Media Server');
  });
  test('sonarr → "TV Shows"', () => {
    expect(getServiceLabel('sonarr')).toBe('TV Shows');
  });
  test('radarr → "Movies"', () => {
    expect(getServiceLabel('radarr')).toBe('Movies');
  });
  test('jellyfin → "Media Server"', () => {
    expect(getServiceLabel('jellyfin')).toBe('Media Server');
  });
  test('prowlarr → "Indexer Manager"', () => {
    expect(getServiceLabel('prowlarr')).toBe('Indexer Manager');
  });
  test('pihole → "DNS Filter"', () => {
    expect(getServiceLabel('pihole')).toBe('DNS Filter');
  });
  test('portainer → "Container Manager"', () => {
    expect(getServiceLabel('portainer')).toBe('Container Manager');
  });
  test('website → "Website"', () => {
    expect(getServiceLabel('website')).toBe('Website');
  });
  test('custom → "Service"', () => {
    expect(getServiceLabel('custom')).toBe('Service');
  });
  test('unknown type → "Service"', () => {
    expect(getServiceLabel('zzzz')).toBe('Service');
  });
  test('undefined → "Service"', () => {
    expect(getServiceLabel(undefined)).toBe('Service');
  });
});

/* ── SERVICE_ICONS ──────────────────────────────────────── */
describe('SERVICE_ICONS', () => {
  test('has expected service types', () => {
    const expectedKeys = ['server', 'plex', 'sonarr', 'radarr', 'custom', 'website'];
    expectedKeys.forEach(key => {
      expect(SERVICE_ICONS).toHaveProperty(key);
    });
  });

  test('each icon is an SVG string', () => {
    Object.values(SERVICE_ICONS).forEach(icon => {
      expect(icon).toContain('<svg');
      expect(icon).toContain('</svg>');
    });
  });
});

/* ── getServiceIconHtml ─────────────────────────────────── */
describe('getServiceIconHtml', () => {
  test('known file-based type returns img tag', () => {
    const html = getServiceIconHtml('server');
    expect(html).toContain('<img');
    expect(html).toContain('/static/images/server.svg');
  });

  test('SVG-based type returns span with SVG', () => {
    const html = getServiceIconHtml('sonarr');
    expect(html).toContain('<span class="icon">');
    expect(html).toContain('<svg');
  });

  test('custom type returns custom SVG', () => {
    const html = getServiceIconHtml('custom');
    expect(html).toContain('<svg');
  });

  test('unknown type falls back to custom icon', () => {
    const html = getServiceIconHtml('zzz_unknown');
    expect(html).toContain(SERVICE_ICONS.custom);
  });

  test('service object with custom icon_url returns img', () => {
    const svc = { service_type: 'custom', icon_url: 'https://example.com/icon.png' };
    const html = getServiceIconHtml(svc);
    expect(html).toContain('<img');
    expect(html).toContain('https://example.com/icon.png');
    // Also has fallback span
    expect(html).toContain('icon-fallback');
  });

  test('service object with relative icon_url works', () => {
    const svc = { service_type: 'custom', icon_url: '/static/images/test.svg' };
    const html = getServiceIconHtml(svc);
    expect(html).toContain('/static/images/test.svg');
  });

  test('service object with data: URL works', () => {
    const svc = { service_type: 'custom', icon_url: 'data:image/png;base64,abc' };
    const html = getServiceIconHtml(svc);
    expect(html).toContain('data:image/png;base64,abc');
  });

  test('service object with unsafe URL ignores icon_url', () => {
    const svc = { service_type: 'plex', icon_url: 'javascript:alert(1)' };
    const html = getServiceIconHtml(svc);
    // Should NOT contain the javascript: URL
    expect(html).not.toContain('javascript:');
    // Falls back to file-based or SVG icon
    expect(html).toContain('<img');
  });

  test('service object without icon_url uses default', () => {
    const svc = { service_type: 'sonarr' };
    const html = getServiceIconHtml(svc);
    expect(html).toContain('<svg');
  });
});

/* ── renderServiceCards ─────────────────────────────────── */
describe('renderServiceCards', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="services-container"></div>';
    globalThis.isAdminUser = false;
  });

  test('renders cards for each service', () => {
    renderServiceCards([
      { key: 'plex', name: 'Plex', service_type: 'plex' },
      { key: 'sonarr', name: 'Sonarr', service_type: 'sonarr' },
    ]);
    const container = document.getElementById('services-container');
    expect(container.children).toHaveLength(2);
  });

  test('card has correct id', () => {
    renderServiceCards([{ key: 'test', name: 'Test', service_type: 'custom' }]);
    expect(document.getElementById('card-test')).not.toBeNull();
  });

  test('card shows service name', () => {
    renderServiceCards([{ key: 'test', name: 'My Service', service_type: 'custom' }]);
    const card = document.getElementById('card-test');
    expect(card.textContent).toContain('My Service');
  });

  test('card shows service label', () => {
    renderServiceCards([{ key: 'plex', name: 'Plex', service_type: 'plex' }]);
    const card = document.getElementById('card-plex');
    expect(card.textContent).toContain('Media Server');
  });

  test('empty array clears container', () => {
    renderServiceCards([{ key: 'plex', name: 'Plex', service_type: 'plex' }]);
    renderServiceCards([]);
    expect(document.getElementById('services-container').children).toHaveLength(0);
  });

  test('escapes service name HTML', () => {
    renderServiceCards([{ key: 'xss', name: '<script>alert(1)</script>', service_type: 'custom' }]);
    const card = document.getElementById('card-xss');
    expect(card.innerHTML).not.toContain('<script>');
  });

  test('no-op when container missing', () => {
    document.body.innerHTML = '';
    expect(() => renderServiceCards([{ key: 'a', name: 'a', service_type: 'custom' }])).not.toThrow();
  });
});
