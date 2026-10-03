/**
 * Tests for banners.js – alert icon builders, time formatting, level normalization.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  // banners.js depends on core.js ($, $$) and utils.js (escapeHtml)
  loadSource('core.js', 'utils.js', 'banners.js');
});

/* ── getAlertIcon ───────────────────────────────────────── */
describe('getAlertIcon', () => {
  test('info returns SVG with circle', () => {
    const svg = getAlertIcon('info');
    expect(svg).toContain('<svg');
    expect(svg).toContain('site-alert-icon');
    expect(svg).toContain('<circle');
  });

  test('warning returns SVG with triangle path', () => {
    const svg = getAlertIcon('warning');
    expect(svg).toContain('<svg');
    expect(svg).toContain('M10 2L1 18h18L10 2z');
  });

  test('error returns SVG with X paths', () => {
    const svg = getAlertIcon('error');
    expect(svg).toContain('<svg');
    expect(svg).toContain('M7 7l6 6');
  });

  test('unknown level falls back to info icon', () => {
    expect(getAlertIcon('critical')).toBe(getAlertIcon('info'));
  });

  test('undefined falls back to info icon', () => {
    expect(getAlertIcon(undefined)).toBe(getAlertIcon('info'));
  });
});

/* ── getServiceAlertIcon ────────────────────────────────── */
describe('getServiceAlertIcon', () => {
  test('info returns SVG with service-alert-icon class', () => {
    const svg = getServiceAlertIcon('info');
    expect(svg).toContain('service-alert-icon');
  });

  test('warning returns triangle', () => {
    const svg = getServiceAlertIcon('warning');
    expect(svg).toContain('M10 2L1 18h18L10 2z');
  });

  test('error returns X', () => {
    const svg = getServiceAlertIcon('error');
    expect(svg).toContain('M7 7l6 6');
  });

  test('unknown falls back to info', () => {
    expect(getServiceAlertIcon('nope')).toBe(getServiceAlertIcon('info'));
  });
});

/* ── formatBannerTime ───────────────────────────────────── */
describe('formatBannerTime', () => {
  test('falsy input returns empty string', () => {
    expect(formatBannerTime(null)).toBe('');
    expect(formatBannerTime('')).toBe('');
    expect(formatBannerTime(undefined)).toBe('');
  });

  test('"Just now" for less than 1 minute ago', () => {
    const now = new Date();
    expect(formatBannerTime(now.toISOString())).toBe('Just now');
  });

  test('minutes ago', () => {
    const d = new Date(Date.now() - 5 * 60000);
    expect(formatBannerTime(d.toISOString())).toBe('5m ago');
  });

  test('hours ago', () => {
    const d = new Date(Date.now() - 3 * 3600000);
    expect(formatBannerTime(d.toISOString())).toBe('3h ago');
  });

  test('days ago', () => {
    const d = new Date(Date.now() - 2 * 86400000);
    expect(formatBannerTime(d.toISOString())).toBe('2d ago');
  });

  test('more than 7 days returns locale date', () => {
    const d = new Date(Date.now() - 10 * 86400000);
    const result = formatBannerTime(d.toISOString());
    // Should NOT be "Xd ago" format
    expect(result).not.toMatch(/^\d+d ago$/);
    // Should contain some date-like content
    expect(result.length).toBeGreaterThan(0);
  });

  test('exactly 59 minutes ago → minutes format', () => {
    const d = new Date(Date.now() - 59 * 60000);
    expect(formatBannerTime(d.toISOString())).toBe('59m ago');
  });

  test('exactly 23 hours ago → hours format', () => {
    const d = new Date(Date.now() - 23 * 3600000);
    expect(formatBannerTime(d.toISOString())).toBe('23h ago');
  });
});

/* ── normalizeAlertLevel ────────────────────────────────── */
describe('formatScheduledBannerTime', () => {
  test('shows an end time for a valid active window', () => {
    expect(formatScheduledBannerTime('2026-07-20T02:25:00Z')).toMatch(/^Ends /);
  });

  test('falls back safely for a missing or invalid end time', () => {
    expect(formatScheduledBannerTime('')).toBe('Scheduled maintenance');
    expect(formatScheduledBannerTime('not-a-date')).toBe('Scheduled maintenance');
  });
});

describe('formatAutomaticBannerTime', () => {
  test('labels a critical outage clearly', () => {
    expect(formatAutomaticBannerTime({ kind: 'critical_outage' })).toBe('Automatic outage alert');
  });

  test('shows how long restoration monitoring remains active', () => {
    expect(formatAutomaticBannerTime({
      kind: 'services_restored',
      ends_at: '2026-07-22T10:00:00Z'
    })).toMatch(/^Monitoring until /);
  });

  test('falls back for unknown automatic updates', () => {
    expect(formatAutomaticBannerTime({ kind: 'unknown' })).toBe('Automatic status update');
  });

  test('labels a UPS power warning clearly', () => {
    expect(formatAutomaticBannerTime({ kind: 'ups_line_loss' })).toBe('Automatic UPS warning');
  });
});

describe('normalizeAlertLevel', () => {
  test('info → info', () => {
    expect(normalizeAlertLevel('info')).toBe('info');
  });
  test('warning → warning', () => {
    expect(normalizeAlertLevel('warning')).toBe('warning');
  });
  test('error → error', () => {
    expect(normalizeAlertLevel('error')).toBe('error');
  });
  test('unknown level falls back to info', () => {
    expect(normalizeAlertLevel('critical')).toBe('info');
  });
  test('empty string falls back to info', () => {
    expect(normalizeAlertLevel('')).toBe('info');
  });
  test('undefined falls back to info', () => {
    expect(normalizeAlertLevel(undefined)).toBe('info');
  });
});

/* ── renderSiteBanners ──────────────────────────────────── */
describe('renderSiteBanners', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="siteAlerts"></div>';
  });

  test('renders global banners (no service_key)', () => {
    const banners = [
      { id: 1, level: 'info', message: 'Maintenance tonight', created_at: new Date().toISOString() },
      { id: 2, level: 'warning', message: 'Slow network', created_at: new Date().toISOString() },
    ];
    renderSiteBanners(banners);
    const container = document.getElementById('siteAlerts');
    expect(container.children).toHaveLength(2);
    expect(container.innerHTML).toContain('Maintenance tonight');
    expect(container.innerHTML).toContain('Slow network');
  });

  test('filters out service-specific banners', () => {
    const banners = [
      { id: 1, level: 'info', message: 'Global', created_at: new Date().toISOString() },
      { id: 2, level: 'error', message: 'Service down', service_key: 'plex', created_at: new Date().toISOString() },
    ];
    renderSiteBanners(banners);
    const container = document.getElementById('siteAlerts');
    expect(container.children).toHaveLength(1);
    expect(container.innerHTML).toContain('Global');
    expect(container.innerHTML).not.toContain('Service down');
  });

  test('empty banners array clears container', () => {
    document.getElementById('siteAlerts').innerHTML = '<div>old</div>';
    renderSiteBanners([]);
    expect(document.getElementById('siteAlerts').children).toHaveLength(0);
  });

  test('banner has correct CSS class for level', () => {
    renderSiteBanners([{ id: 1, level: 'error', message: 'fail', created_at: new Date().toISOString() }]);
    const alert = document.querySelector('.site-alert');
    expect(alert.classList.contains('error')).toBe(true);
  });

  test('escapes HTML in message', () => {
    renderSiteBanners([{ id: 1, level: 'info', message: '<script>alert(1)</script>', created_at: new Date().toISOString() }]);
    const container = document.getElementById('siteAlerts');
    expect(container.innerHTML).not.toContain('<script>');
    expect(container.textContent).toContain('<script>');
  });

  test('no-op when container missing', () => {
    document.body.innerHTML = '';
    expect(() => renderSiteBanners([{ id: 1, level: 'info', message: 'x' }])).not.toThrow();
  });

  test('renders a critical automatic outage as an assertive error', () => {
    renderSiteBanners([{
      id: 'automatic:critical-outage',
      level: 'error',
      message: 'Critical outage: Core Server is unavailable.',
      automatic: true,
      kind: 'critical_outage'
    }]);

    const alert = document.querySelector('[data-automatic-kind="critical_outage"]');
    expect(alert).not.toBeNull();
    expect(alert.classList.contains('error')).toBe(true);
    expect(alert.classList.contains('site-alert-automatic')).toBe(true);
    expect(alert.getAttribute('role')).toBe('alert');
    expect(alert.getAttribute('aria-live')).toBe('assertive');
    expect(alert.textContent).toContain('Automatic outage alert');
  });

  test('renders restored services as a polite monitoring update', () => {
    renderSiteBanners([{
      id: 'automatic:services-restored',
      level: 'info',
      message: 'Services have been restored.',
      automatic: true,
      kind: 'services_restored',
      ends_at: '2026-07-22T10:00:00Z'
    }]);

    const alert = document.querySelector('[data-automatic-kind="services_restored"]');
    expect(alert).not.toBeNull();
    expect(alert.classList.contains('info')).toBe(true);
    expect(alert.getAttribute('role')).toBe('status');
    expect(alert.getAttribute('aria-live')).toBe('polite');
    expect(alert.textContent).toContain('Monitoring until');
  });

  test('replaces the local UPS fallback with the managed UPS banner', () => {
    updateUPSLineAlert({ power_present: false });
    renderSiteBanners([{
      id: 'automatic:ups-line-loss',
      level: 'warning',
      message: 'Managed UPS warning',
      automatic: true,
      kind: 'ups_line_loss'
    }]);

    expect(document.querySelector('[data-auto-alert="ups-line"]')).toBeNull();
    expect(document.querySelector('[data-automatic-kind="ups_line_loss"]')).not.toBeNull();
  });
});

/* ── automatic UPS line alert ───────────────────────────── */
describe('scheduled maintenance banner', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="siteAlerts"></div>';
  });

  test('marks scheduled maintenance and shows its end time', () => {
    renderSiteBanners([{
      id: 'scheduled:weekly',
      level: 'warning',
      message: 'Maintenance in progress',
      scheduled: true,
      ends_at: '2026-07-20T02:25:00Z'
    }]);
    const alert = document.querySelector('[data-scheduled="true"]');
    expect(alert).not.toBeNull();
    expect(alert.textContent).toContain('Maintenance in progress');
    expect(alert.textContent).toContain('Ends');
  });
});

describe('automatic UPS line alert', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="siteAlerts"></div>';
  });

  test('does not create a client-only warning that bypasses admin controls', () => {
    updateUPSLineAlert({ power_present: false });

    expect(document.querySelector('[data-auto-alert="ups-line"]')).toBeNull();
  });

  test('keeps a managed warning when UPS state is temporarily unavailable', () => {
    renderSiteBanners([{
      id: 'automatic:ups-line-loss', level: 'warning', message: 'Mains power lost',
      automatic: true, kind: 'ups_line_loss'
    }]);
    updateUPSLineAlert(null);

    expect(document.querySelector('[data-automatic-kind="ups_line_loss"]')).not.toBeNull();
  });

  test('removes the warning after confirmed line-power recovery', () => {
    renderSiteBanners([{
      id: 'automatic:ups-line-loss', level: 'warning', message: 'Mains power lost',
      automatic: true, kind: 'ups_line_loss'
    }]);
    updateUPSLineAlert({ power_present: true });

    expect(document.querySelector('[data-automatic-kind="ups_line_loss"]')).toBeNull();
  });

  test('clears the warning when UPS monitoring is disabled', () => {
    renderSiteBanners([{
      id: 'automatic:ups-line-loss', level: 'warning', message: 'Mains power lost',
      automatic: true, kind: 'ups_line_loss'
    }]);
    clearUPSLineAlert();

    expect(document.querySelector('[data-automatic-kind="ups_line_loss"]')).toBeNull();
  });
});

/* ── renderServiceBanners ───────────────────────────────── */
describe('renderServiceBanners', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="card-plex"><div class="adminRow"></div></div>
      <div id="card-sonarr"></div>
    `;
  });

  test('adds banner to correct service card', () => {
    const banners = [
      { id: 10, level: 'warning', message: 'High load', service_key: 'plex', created_at: new Date().toISOString() },
    ];
    renderServiceBanners(banners);
    const card = document.getElementById('card-plex');
    expect(card.querySelector('.service-alert')).not.toBeNull();
    expect(card.innerHTML).toContain('High load');
  });

  test('inserts before adminRow when present', () => {
    renderServiceBanners([{ id: 10, level: 'info', message: 'Test', service_key: 'plex', created_at: new Date().toISOString() }]);
    const card = document.getElementById('card-plex');
    const children = Array.from(card.children);
    const alertIdx = children.findIndex(c => c.classList.contains('service-alert'));
    const adminIdx = children.findIndex(c => c.classList.contains('adminRow'));
    expect(alertIdx).toBeLessThan(adminIdx);
  });

  test('appends to end when no adminRow', () => {
    renderServiceBanners([{ id: 10, level: 'info', message: 'Test', service_key: 'sonarr', created_at: new Date().toISOString() }]);
    const card = document.getElementById('card-sonarr');
    const last = card.lastElementChild;
    expect(last.classList.contains('service-alert')).toBe(true);
  });

  test('skips banner for missing card', () => {
    expect(() => {
      renderServiceBanners([{ id: 10, level: 'info', message: 'x', service_key: 'missing' }]);
    }).not.toThrow();
  });

  test('does not duplicate banners with same id', () => {
    const banner = { id: 10, level: 'info', message: 'Test', service_key: 'plex', created_at: new Date().toISOString() };
    renderServiceBanners([banner]);
    renderServiceBanners([banner]);
    const alerts = document.querySelectorAll('#card-plex .service-alert');
    expect(alerts).toHaveLength(1);
  });

  test('filters out global banners', () => {
    renderServiceBanners([{ id: 10, level: 'info', message: 'Global', created_at: new Date().toISOString() }]);
    expect(document.querySelectorAll('.service-alert')).toHaveLength(0);
  });
});

/* ── Banners on several services ────────────────────────── */
describe('banners on several services', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="siteAlerts"></div>
      <div id="card-plex"></div>
      <div id="card-sonarr"></div>
    `;
    globalThis.updateTopologyNotices = jest.fn();
  });

  afterEach(() => {
    delete globalThis.updateTopologyNotices;
  });

  const shared = { id: 'alert_1', level: 'warning', message: 'Media is slow', service_key: 'plex', service_keys: ['plex', 'sonarr'] };

  test('shows on every card it names, and not at the top', () => {
    renderSiteBanners([shared]);
    renderServiceBanners([shared]);
    expect(document.querySelectorAll('#card-plex .service-alert')).toHaveLength(1);
    expect(document.querySelectorAll('#card-sonarr .service-alert')).toHaveLength(1);
    expect(document.querySelectorAll('#siteAlerts .site-alert')).toHaveLength(0);
  });

  test('older payloads with one service still work', () => {
    renderServiceBanners([{ id: 'old', level: 'info', message: 'Old style', service_key: 'sonarr' }]);
    expect(document.querySelector('#card-sonarr .service-alert').textContent).toContain('Old style');
  });

  test('hands the notices to the map', () => {
    renderServiceBanners([shared, { id: 'top', level: 'info', message: 'Everyone' }]);
    expect(serviceBannersByKey.plex.map(b => b.id)).toEqual(['alert_1']);
    expect(serviceBannersByKey.sonarr.map(b => b.id)).toEqual(['alert_1']);
    expect(serviceBannersByKey.top).toBeUndefined();
    expect(updateTopologyNotices).toHaveBeenCalled();
  });
});

/* ── Time labels ────────────────────────────────────────── */
describe('banner time labels', () => {
  const at = (minutes) => new Date(Date.now() + minutes * 60000).toISOString();

  test('a manual banner with an end says until when', () => {
    expect(bannerTimeLabel({ source: 'manual', created_at: at(-5), ends_at: at(60) })).toMatch(/^Until /);
  });

  test('a manual banner without an end says how long it has shown', () => {
    expect(bannerTimeLabel({ source: 'manual', created_at: at(-5) })).toBe('5m ago');
    expect(bannerTimeLabel({ source: 'manual', created_at: at(-600), starts_at: at(-2) })).toBe('2m ago');
  });

  // Times use UK formats in the viewer's timezone, whatever the browser's
  // language; these dates are local, so the expectations hold anywhere.
  test('upcoming maintenance gives its window', () => {
    const start = new Date(2026, 9, 4, 22, 0);
    const end = new Date(2026, 9, 4, 23, 30);
    const label = bannerTimeLabel({ kind: 'maintenance_upcoming', scheduled: true, starts_at: start.toISOString(), ends_at: end.toISOString() });
    expect(label).toBe('Sun 4 Oct, 22:00–23:30');
    expect(formatUpcomingBannerTime(start.toISOString(), '')).toBe('From Sun 4 Oct, 22:00');
    expect(formatUpcomingBannerTime('', '')).toBe('Planned maintenance');
  });

  test('running maintenance still says when it ends', () => {
    expect(bannerTimeLabel({ kind: 'maintenance', scheduled: true, ends_at: at(30) })).toMatch(/^Ends \d{2}:\d{2}$/);
  });

  test('times say which day once they are not today', () => {
    const now = new Date(2026, 9, 3, 12, 0);
    expect(formatBannerClock(new Date(2026, 9, 3, 23, 30), now)).toBe('23:30');
    expect(formatBannerClock(new Date(2026, 9, 5, 9, 0), now)).toBe('Mon 09:00');
    expect(formatBannerClock(new Date(2026, 11, 25, 9, 0), now)).toBe('25 Dec 09:00');
  });

  test('upcoming banners are marked for styling', () => {
    document.body.innerHTML = '<div id="siteAlerts"></div>';
    renderSiteBanners([{ id: 'upcoming:disk', kind: 'maintenance_upcoming', scheduled: true, level: 'info', message: 'Planned maintenance: Disk.', starts_at: at(30), ends_at: at(90) }]);
    expect(document.querySelector('[data-upcoming="true"]')).not.toBeNull();
  });
});
