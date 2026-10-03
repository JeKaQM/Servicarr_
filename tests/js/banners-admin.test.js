/**
 * Tests for the banner composer and banner list in banners-admin.js, run
 * against the real Banners tab markup.
 */
const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

const template = fs.readFileSync(path.resolve(__dirname, '../../web/templates/partials/_admin_services_banners.html'), 'utf8');

const SERVICES = [
  { key: 'plex', name: 'Plex', visible: true },
  { key: 'sonarr', name: 'Sonarr', visible: true },
  { key: 'backup', name: 'Backup', visible: false },
];

beforeAll(() => loadSource('core.js', 'utils.js', 'services.js', 'matrix.js', 'banners.js', 'admin-ui.js', 'banners-admin.js'));

beforeEach(() => {
  document.body.innerHTML = template;
  globalThis.adminServicesData = SERVICES;
  globalThis.j = jest.fn().mockResolvedValue([]);
  globalThis.getCsrf = jest.fn().mockReturnValue('csrf');
  globalThis.showToast = jest.fn();
  globalThis.loadBanners = jest.fn().mockResolvedValue();
  globalThis.confirm = jest.fn(() => true);
  globalThis.bannerSaving = false;
  globalThis.latestAdminBanners = [];
  globalThis.latestMaintenanceWindows = [];
  jest.spyOn(console, 'error').mockImplementation(() => {});
  initBannersTab();
});

afterEach(() => jest.restoreAllMocks());

const $id = id => document.getElementById(id);

function type(id, value) {
  $id(id).value = value;
  $id(id).dispatchEvent(new Event('input', { bubbles: true }));
}

function choose(name, value) {
  const input = document.querySelector(`input[name="${name}"][value="${value}"]`);
  input.checked = true;
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

function tick(key) {
  const box = document.querySelector(`#bannerServiceList input[value="${key}"]`);
  box.checked = true;
  box.dispatchEvent(new Event('change', { bubbles: true }));
}

function sent(method) {
  const call = j.mock.calls.find(([url, options]) => url === '/api/admin/status-alerts' && options?.method === method);
  expect(call).toBeDefined();
  return JSON.parse(call[1].body);
}

describe('composing a banner', () => {
  test('a template fills the message and picks its type', () => {
    document.querySelector('[data-template="down"]').click();
    expect($id('bannerMessage').value).toBe(BANNER_TEMPLATES.down.message);
    expect(radioValue('bannerLevel')).toBe('error');
    expect($id('bannerMessageCount').textContent).toBe(`${BANNER_TEMPLATES.down.message.length} / 500`);
  });

  test('the preview shows the banner as visitors will see it', () => {
    type('bannerMessage', 'Router firmware update tonight.');
    choose('bannerLevel', 'warning');
    expect($id('bannerPreview').querySelector('.site-alert.warning').textContent).toContain('Router firmware update tonight.');
    expect($id('bannerPreviewWhen').textContent).toBe('Shows at the top of the page from now until you end it.');

    choose('bannerWhere', 'services');
    expect($id('bannerServicePicker').hidden).toBe(false);
    tick('plex');
    tick('sonarr');
    const card = $id('bannerPreview').querySelector('.banner-preview-card');
    expect(card.querySelector('.banner-preview-card-title').textContent).toBe('Plex and Sonarr');
    expect(card.querySelector('.service-alert.warning')).not.toBeNull();
    expect($id('bannerPreviewWhen').textContent).toMatch(/^Shows on Plex and Sonarr from now/);
  });

  test('the picker lists hidden services and marks them', () => {
    choose('bannerWhere', 'services');
    const backup = document.querySelector('#bannerServiceList input[value="backup"]').closest('.link-option');
    expect(backup.querySelector('.link-option-status').textContent).toBe('Hidden');
  });

  test('publishing now with no end sends no times', async () => {
    type('bannerMessage', 'Plex is slow.');
    choose('bannerWhere', 'services');
    tick('plex');
    await saveBanner();
    expect(sent('POST')).toEqual({ message: 'Plex is slow.', level: 'info', service_keys: ['plex'], starts_at: '', ends_at: '' });
    expect(showToast).toHaveBeenCalledWith('Banner published');
    expect($id('bannerMessage').value).toBe('');
  });

  test('"After" ends it that long from now', async () => {
    type('bannerMessage', 'Back soon.');
    choose('bannerEnd', 'after');
    expect($id('bannerDuration').hidden).toBe(false);
    $id('bannerDuration').value = '120';
    const before = Date.now();
    await saveBanner();
    const end = new Date(sent('POST').ends_at).getTime();
    expect(end - before).toBeGreaterThanOrEqual(120 * 60000 - 1000);
    expect(end - before).toBeLessThan(120 * 60000 + 5000);
  });

  test('a later start and an end time are sent as instants', async () => {
    type('bannerMessage', 'Tonight.');
    choose('bannerStart', 'at');
    expect($id('bannerStartsAt').value).not.toBe(''); // a default start is offered
    $id('bannerStartsAt').value = '2099-10-04T21:00';
    choose('bannerEnd', 'at');
    $id('bannerEndsAt').value = '2099-10-04T23:00';
    await saveBanner();
    const body = sent('POST');
    expect(body.starts_at).toBe(new Date('2099-10-04T21:00').toISOString());
    expect(body.ends_at).toBe(new Date('2099-10-04T23:00').toISOString());
    expect(showToast).toHaveBeenCalledWith('Banner scheduled');
  });

  test.each([
    [() => {}, 'Write a message for the banner'],
    [() => { type('bannerMessage', 'x'); choose('bannerWhere', 'services'); }, 'Choose at least one service, or show it at the top of the page'],
    [() => { type('bannerMessage', 'x'); choose('bannerStart', 'at'); $id('bannerStartsAt').value = '2000-01-01T10:00'; }, 'Choose a start in the future, or pick Now'],
    [() => { type('bannerMessage', 'x'); choose('bannerEnd', 'at'); $id('bannerEndsAt').value = '2000-01-01T10:00'; }, 'Choose an end in the future'],
  ])('explains a problem before sending (%#)', async (setup, message) => {
    setup();
    await saveBanner();
    expect(j).not.toHaveBeenCalled();
    expect($id('bannerFormError').hidden).toBe(false);
    expect($id('bannerFormError').textContent).toBe(message);
  });

  test("the server's reason is shown when it refuses", async () => {
    type('bannerMessage', 'x');
    j.mockRejectedValueOnce(Object.assign(new Error('HTTP 400'), { body: 'Unknown service "ghost"\n' }));
    await saveBanner();
    expect($id('bannerFormError').textContent).toBe('Unknown service "ghost"');
    expect($id('bannerMessage').value).toBe('x');
    expect($id('saveBanner').disabled).toBe(false);
  });
});

describe('editing banners', () => {
  test('a manual banner loads with its services and times, and saves by ID', async () => {
    const start = new Date(Date.now() + 3600000);
    const end = new Date(Date.now() + 7200000);
    editBanner({
      id: 'alert_1', source: 'manual', state: 'scheduled', message: 'Later', level: 'warning',
      service_key: 'plex', service_keys: ['plex', 'sonarr'], starts_at: start.toISOString(), ends_at: end.toISOString()
    });
    expect($id('bannerFormTitle').textContent).toBe('Edit banner');
    expect($id('cancelBannerEdit').hidden).toBe(false);
    expect(radioValue('bannerWhere')).toBe('services');
    expect(pickedServiceKeys('bannerServiceList')).toEqual(['plex', 'sonarr']);
    expect(radioValue('bannerStart')).toBe('at');
    expect($id('bannerStartsAt').value).toBe(localInputValue(start));
    expect(radioValue('bannerEnd')).toBe('at');

    await saveBanner();
    expect(sent('PUT')).toMatchObject({ id: 'alert_1', message: 'Later', level: 'warning', service_keys: ['plex', 'sonarr'] });
  });

  test('an automatic banner only takes new wording and type', async () => {
    editBanner({ id: 'automatic:critical-outage', source: 'automatic', automatic: true, kind: 'critical_outage',
      message: 'Outage', level: 'error', created_at: '2026-10-03T10:00:00Z' });
    expect($id('bannerFormTitle').textContent).toBe('Adjust automatic banner');
    expect($id('bannerFormNote').hidden).toBe(false);
    expect(document.querySelector('input[name="bannerWhere"]').disabled).toBe(true);
    expect($id('bannerTemplates').hidden).toBe(true);

    type('bannerMessage', 'Outage being fixed');
    await saveBanner();
    expect(sent('PUT')).toEqual({ id: 'automatic:critical-outage', occurrence_at: '2026-10-03T10:00:00Z',
      message: 'Outage being fixed', level: 'error', hidden: false });

    resetBannerForm();
    expect(document.querySelector('input[name="bannerWhere"]').disabled).toBe(false);
    expect($id('bannerFormTitle').textContent).toBe('New banner');
  });

  test("a maintenance banner opens its window, where its wording lives", () => {
    latestMaintenanceWindows = [{ id: 'disk', name: 'NAS disk', message: 'New disk', level: 'warning', schedule_type: 'daily',
      start_time: '02:00', duration_minutes: 30, timezone: 'UTC', enabled: true }];
    editBanner({ id: 'scheduled:disk', source: 'scheduled', schedule_id: 'disk', message: 'New disk', level: 'warning' });
    expect($id('maintenanceName').value).toBe('NAS disk');
    expect($id('bannerFormTitle').textContent).toBe('New banner');
  });
});

describe('the banner list', () => {
  const iso = minutes => new Date(Date.now() + minutes * 60000).toISOString();
  const banners = [
    { id: 'live', source: 'manual', state: 'live', message: 'Showing', level: 'info', created_at: iso(-30), ends_at: iso(60) },
    { id: 'later', source: 'manual', state: 'scheduled', message: 'Later', level: 'warning', created_at: iso(-5), starts_at: iso(120), service_keys: ['plex'], service_key: 'plex' },
    { id: 'old', source: 'manual', state: 'ended', message: 'Done', level: 'info', created_at: iso(-300), ends_at: iso(-60) },
    { id: 'automatic:critical-outage', source: 'automatic', automatic: true, kind: 'critical_outage', state: 'live',
      message: 'Critical outage', level: 'error', created_at: iso(-10), hidden: true },
    { id: 'scheduled:disk', source: 'scheduled', scheduled: true, kind: 'maintenance', schedule_id: 'disk', state: 'live',
      message: 'New disk', level: 'warning', created_at: iso(-10), starts_at: iso(-10), ends_at: iso(80) },
  ];

  beforeEach(() => {
    latestMaintenanceWindows = [{ id: 'disk', name: 'NAS disk' }];
    latestAdminBanners = banners;
    renderBannerBoard(banners);
  });

  const groupTitles = () => [...document.querySelectorAll('.banner-group-title')].map(el => el.textContent);
  const row = id => document.querySelector(`.banner-row[data-id="${id}"]`);
  const actions = id => [...row(id).querySelectorAll('[data-banner-action]')].map(b => b.textContent);

  test('groups banners by what visitors see', () => {
    expect(groupTitles()).toEqual(['Showing now (2)', 'Scheduled (1)', 'Hidden from visitors (1)', 'Ended (1)']);
    expect($id('clearEndedBanners').hidden).toBe(false);
    expect(row('later').querySelector('.banner-row-meta').textContent).toMatch(/^On Plex · Starts /);
    expect(row('scheduled:disk').querySelector('.banner-row-meta').textContent).toMatch(/· From “NAS disk”$/);
    expect(row('live').querySelector('.level-chip').textContent).toBe('Info');
  });

  test('offers the actions that fit each banner', () => {
    expect(actions('live')).toEqual(['Edit', 'End now']);
    expect(actions('later')).toEqual(['Edit', 'Delete']);
    expect(actions('old')).toEqual(['Reuse', 'Delete']);
    expect(actions('automatic:critical-outage')).toEqual(['Edit', 'Show again']);
    expect(actions('scheduled:disk')).toEqual(['Edit window', 'Hide']);
  });

  test('End now ends a live banner', () => {
    row('live').querySelector('[data-banner-action="end"]').click();
    expect(sent('PUT')).toMatchObject({ id: 'live', end_now: true });
  });

  test('Reuse starts a new banner from an ended one', () => {
    row('old').querySelector('[data-banner-action="reuse"]').click();
    expect($id('bannerMessage').value).toBe('Done');
    expect($id('bannerFormTitle').textContent).toBe('New banner');
  });

  test('Show again restores a hidden occurrence', () => {
    row('automatic:critical-outage').querySelector('[data-banner-action="show"]').click();
    expect(sent('PUT')).toEqual({ id: 'automatic:critical-outage', occurrence_at: banners[3].created_at, hidden: false });
  });

  test('Delete ended clears every ended banner', () => {
    $id('clearEndedBanners').click();
    expect(j).toHaveBeenCalledWith('/api/admin/status-alerts?ended=1', expect.objectContaining({ method: 'DELETE' }));
  });

  test('says so when there are no banners', () => {
    renderBannerBoard([]);
    expect($id('bannersList').textContent).toContain('No banners');
    expect($id('clearEndedBanners').hidden).toBe(true);
  });
});

test('reloading services keeps what is ticked in the pickers', () => {
  choose('bannerWhere', 'services');
  tick('sonarr');
  adminServicesData = SERVICES.concat({ key: 'radarr', name: 'Radarr', visible: true });
  refreshBannerPickers();
  expect(pickedServiceKeys('bannerServiceList')).toEqual(['sonarr']);
  expect(document.querySelector('#bannerServiceList input[value="radarr"]')).not.toBeNull();
  expect(radioValue('bannerWhere')).toBe('services');
});
