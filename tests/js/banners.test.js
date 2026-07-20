/**
 * Tests for banners.js – alert icon builders, time formatting, level normalization.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  // banners.js depends on core.js ($, $$) and admin-ui.js (escapeHtml)
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

  test('shows a persistent warning when line power is lost', () => {
    updateUPSLineAlert({ power_present: false });

    const alert = document.querySelector('[data-auto-alert="ups-line"]');
    expect(alert).not.toBeNull();
    expect(alert.classList.contains('warning')).toBe(true);
    expect(alert.textContent).toContain('Mains power lost');
    expect(alert.getAttribute('role')).toBe('alert');
  });

  test('does not duplicate the warning on subsequent UPS polls', () => {
    updateUPSLineAlert({ power_present: false });
    updateUPSLineAlert({ power_present: false });

    expect(document.querySelectorAll('[data-auto-alert="ups-line"]')).toHaveLength(1);
  });

  test('keeps the warning when UPS state is temporarily unavailable', () => {
    updateUPSLineAlert({ power_present: false });
    updateUPSLineAlert(null);

    expect(document.querySelector('[data-auto-alert="ups-line"]')).not.toBeNull();
  });

  test('removes the warning after confirmed line-power recovery', () => {
    updateUPSLineAlert({ power_present: false });
    updateUPSLineAlert({ power_present: true });

    expect(document.querySelector('[data-auto-alert="ups-line"]')).toBeNull();
  });

  test('manual banner refresh preserves the automatic warning', () => {
    updateUPSLineAlert({ power_present: false });
    renderSiteBanners([{ id: 1, level: 'info', message: 'Maintenance' }]);

    expect(document.querySelector('[data-auto-alert="ups-line"]')).not.toBeNull();
    expect(document.getElementById('siteAlerts').textContent).toContain('Maintenance');
  });

  test('clears the warning when UPS monitoring is disabled', () => {
    updateUPSLineAlert({ power_present: false });
    clearUPSLineAlert();

    expect(document.querySelector('[data-auto-alert="ups-line"]')).toBeNull();
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

/* ── populateBannerScopeDropdown ────────────────────────── */
describe('populateBannerScopeDropdown', () => {
  beforeEach(() => {
    document.body.innerHTML = '<select id="bannerService"><option value="">Global (top of page)</option></select>';
    // Set up global servicesData
    globalThis.servicesData = [
      { key: 'plex', name: 'Plex' },
      { key: 'sonarr', name: 'Sonarr' },
    ];
  });

  test('populates dropdown with services', () => {
    populateBannerScopeDropdown();
    const select = document.getElementById('bannerService');
    expect(select.querySelectorAll('option')).toHaveLength(3); // Global + 2 services
  });

  test('keeps Global option first', () => {
    populateBannerScopeDropdown();
    const first = document.getElementById('bannerService').querySelector('option');
    expect(first.value).toBe('');
    expect(first.textContent).toContain('Global');
  });

  test('no-op when select not found', () => {
    document.body.innerHTML = '';
    expect(() => populateBannerScopeDropdown()).not.toThrow();
  });

  test('creates Global option if missing', () => {
    document.body.innerHTML = '<select id="bannerService"></select>';
    populateBannerScopeDropdown();
    const first = document.getElementById('bannerService').querySelector('option');
    expect(first.value).toBe('');
    expect(first.textContent).toContain('Global');
  });
});

describe('renderMaintenanceSchedules', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="maintenanceSchedulesList"></div>';
  });

  test('renders schedule timing and suppression state', () => {
    renderMaintenanceSchedules([{
      id: 'weekly',
      name: 'Monday maintenance',
      message: 'Maintenance in progress',
      level: 'warning',
      weekday: 1,
      start_time: '02:55',
      duration_minutes: 30,
      timezone: 'Europe/London',
      suppress_monitoring: true,
      enabled: true
    }]);
    const text = document.getElementById('maintenanceSchedulesList').textContent;
    expect(text).toContain('Monday at 02:55 for 30 min');
    expect(text).toContain('Monitoring paused');
    expect(text).toContain('Enabled');
  });

  test('escapes schedule content', () => {
    renderMaintenanceSchedules([{
      id: 'weekly', name: '<script>x</script>', message: '<b>bad</b>', level: 'info',
      weekday: 1, start_time: '03:00', duration_minutes: 10, timezone: 'UTC', enabled: false
    }]);
    const list = document.getElementById('maintenanceSchedulesList');
    expect(list.innerHTML).not.toContain('<script>');
    expect(list.innerHTML).not.toContain('<b>bad</b>');
    expect(list.textContent).toContain('<script>x</script>');
  });
});
