/**
 * Tests for the maintenance window form and list in banners-admin.js, run
 * against the real Banners tab markup.
 */
const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

const template = fs.readFileSync(path.resolve(__dirname, '../../web/templates/partials/_admin_services_banners.html'), 'utf8');

const SERVICES = [
  { key: 'router', name: 'Router', visible: true },
  { key: 'nas', name: 'NAS', depends_on: 'router', visible: true },
  { key: 'plex', name: 'Plex', depends_on: 'nas', visible: true },
  { key: 'overseerr', name: 'Overseerr', depends_on: 'plex', visible: true },
];

beforeAll(() => loadSource('core.js', 'utils.js', 'services.js', 'matrix.js', 'banners.js', 'admin-ui.js', 'banners-admin.js'));

beforeEach(() => {
  document.body.innerHTML = template;
  globalThis.adminServicesData = SERVICES;
  globalThis.j = jest.fn().mockResolvedValue({});
  globalThis.getCsrf = jest.fn().mockReturnValue('csrf');
  globalThis.showToast = jest.fn();
  globalThis.loadBanners = jest.fn().mockResolvedValue();
  jest.spyOn(console, 'error').mockImplementation(() => {});
  globalThis.latestMaintenanceWindows = [];
  globalThis.maintenanceSaving = false;
  globalThis.bannerSaving = false;
  initBannersTab();
  document.getElementById('maintenanceName').value = 'Server upgrade';
  document.getElementById('maintenanceMessage').value = 'Maintenance is in progress.';
  document.getElementById('maintenanceTimezone').value = 'Europe/London';
});

afterEach(() => jest.restoreAllMocks());

function choose(name, value) {
  const input = document.querySelector(`input[name="${name}"][value="${value}"]`);
  input.checked = true;
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

function postedSchedule() {
  const call = j.mock.calls.find(([url, options]) => url === '/api/admin/maintenance-schedules' && options?.method === 'POST');
  expect(call).toBeDefined();
  expect(call[1].headers['X-CSRF-Token']).toBe('csrf');
  return JSON.parse(call[1].body);
}

const formError = () => document.getElementById('maintenanceFormError');

test('a new window starts as a one-time window that ends an hour after it starts', () => {
  resetMaintenanceScheduleForm();
  const start = document.getElementById('maintenanceStartsAt').value;
  expect(start).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:(00|30)$/);
  expect(document.getElementById('maintenanceEndsAt').value).toBe(addMinutesToWallTime(start, 60));
  expect(document.getElementById('maintenanceRecurringFields').hidden).toBe(true);
  expect(document.getElementById('maintenanceTimezone').value).toBe(defaultTimezone());
});

test('submits multi-month dates in the selected timezone without browser timezone conversion', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  document.getElementById('maintenanceEndsAt').value = '2027-12-20T19:00';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({
    schedule_type: 'once', starts_at: '2026-09-15T10:00', ends_at: '2027-12-20T19:00', timezone: 'Europe/London', service_keys: []
  });
  expect(postedSchedule()).not.toHaveProperty('duration_minutes');
});

test('"After" ends the window by wall-clock time', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-10-04T23:30';
  choose('maintenanceEnd', 'after');
  document.getElementById('maintenanceOnceDuration').value = '120';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ starts_at: '2026-10-04T23:30', ends_at: '2026-10-05T01:30' });
});

test('"When I end it" submits an empty end', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  choose('maintenanceEnd', 'never');
  expect(document.getElementById('maintenanceEndsAt').hidden).toBe(true);
  await saveMaintenanceSchedule();
  expect(postedSchedule().ends_at).toBe('');
});

test('an end before the start is explained before making a request', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  document.getElementById('maintenanceEndsAt').value = '2026-09-15T09:00';
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(formError().textContent).toBe('The end must be after the start');
});

test('selected weekdays and multiweek durations submit without the old cap', async () => {
  choose('maintenanceRepeat', 'weekly');
  expect(document.getElementById('maintenanceWeekdaysField').hidden).toBe(false);
  document.querySelector('[name="maintenanceWeekdays"][value="5"]').checked = true;
  document.querySelector('[name="maintenanceWeekdays"][value="6"]').checked = true;
  document.getElementById('maintenanceDuration').value = '3';
  document.getElementById('maintenanceDurationUnit').value = '10080';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ schedule_type: 'weekly', weekdays: [1, 5, 6], weekday: 1, duration_minutes: 30240 });
});

test('daily repeats need no weekday selection and accept fractional hours', async () => {
  choose('maintenanceRepeat', 'daily');
  expect(document.getElementById('maintenanceWeekdaysField').hidden).toBe(true);
  document.getElementById('maintenanceDuration').value = '1.5';
  document.getElementById('maintenanceDurationUnit').value = '60';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ schedule_type: 'daily', duration_minutes: 90 });
  expect(postedSchedule()).not.toHaveProperty('weekdays');
});

test('empty weekday selections are explained before making a request', async () => {
  choose('maintenanceRepeat', 'weekly');
  document.querySelectorAll('[name="maintenanceWeekdays"]').forEach(input => { input.checked = false; });
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(formError().textContent).toBe('Choose at least one weekday');
});

test.each(['0', '-1', '0.5', '153722868'])('rejects invalid minute duration %s', async value => {
  choose('maintenanceRepeat', 'daily');
  document.getElementById('maintenanceDuration').value = value;
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(formError().textContent).toContain('whole minute');
});

test('an unknown timezone is explained before making a request', async () => {
  document.getElementById('maintenanceTimezone').value = 'Mars/Olympus';
  document.getElementById('maintenanceTimezone').dispatchEvent(new Event('input', { bubbles: true }));
  expect(document.getElementById('maintenanceTimezoneNow').textContent).toContain('Unknown timezone');
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(formError().textContent).toContain('timezone');
});

test('covering chosen services offers the services that depend on them', async () => {
  choose('maintenanceCovers', 'services');
  expect(document.getElementById('maintenanceServicePicker').hidden).toBe(false);
  const nas = document.querySelector('#maintenanceServiceList input[value="nas"]');
  nas.checked = true;
  nas.dispatchEvent(new Event('change', { bubbles: true }));
  expect(document.getElementById('maintenanceDependents').hidden).toBe(false);
  expect(document.getElementById('maintenanceDependentsText').textContent).toBe('Plex and Overseerr depend on it and may report problems too.');

  document.getElementById('addMaintenanceDependents').click();
  expect(pickedServiceKeys('maintenanceServiceList').sort()).toEqual(['nas', 'overseerr', 'plex']);
  expect(document.getElementById('maintenanceDependents').hidden).toBe(true);

  document.getElementById('maintenanceNotice').value = '1440';
  document.getElementById('maintenanceStartsAt').value = '2026-10-04T22:00';
  document.getElementById('maintenanceEndsAt').value = '2026-10-04T23:30';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ service_keys: ['nas', 'plex', 'overseerr'], notice_minutes: 1440 });
});

test('chosen services must include at least one', async () => {
  choose('maintenanceCovers', 'services');
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(formError().textContent).toBe('Choose at least one service, or cover all services');
});

test('the summary says what the window will do', () => {
  choose('maintenanceRepeat', 'weekly');
  document.getElementById('maintenanceStartTime').value = '02:55';
  document.getElementById('maintenanceNotice').value = '60';
  document.getElementById('maintenanceNotice').dispatchEvent(new Event('change', { bubbles: true }));
  expect(document.getElementById('maintenanceSummary').textContent)
    .toBe('Mondays at 02:55 for 30 min · Europe/London · covers all services · pauses monitoring · announced 1 hour before');
});

test('editing a legacy schedule keeps its weekday and sensible duration units', () => {
  editMaintenanceSchedule({
    id: 'legacy', name: 'Old schedule', message: 'Maintenance', level: 'warning', weekday: 6,
    start_time: '23:30', duration_minutes: 2880, timezone: 'Asia/Kathmandu', enabled: true, suppress_monitoring: true
  });
  expect(radioValue('maintenanceRepeat')).toBe('weekly');
  expect(document.querySelector('[name="maintenanceWeekdays"][value="6"]').checked).toBe(true);
  expect(document.querySelector('[name="maintenanceWeekdays"][value="1"]').checked).toBe(false);
  expect(document.getElementById('maintenanceDuration').value).toBe('2');
  expect(document.getElementById('maintenanceDurationUnit').value).toBe('1440');
  expect(document.getElementById('maintenanceTimezone').value).toBe('Asia/Kathmandu');
  expect(document.getElementById('maintenanceFormTitle').textContent).toBe('Edit maintenance window');
  expect(document.getElementById('cancelMaintenanceSchedule').hidden).toBe(false);
});

test('editing restores chosen services and an unusual notice period', () => {
  editMaintenanceSchedule({
    id: 'disk', name: 'Disk', message: 'm', level: 'info', schedule_type: 'once', starts_at: '2026-10-04T21:00:00Z',
    ends_at: '', timezone: 'UTC', enabled: true, suppress_monitoring: false, service_keys: ['plex'], notice_minutes: 90
  });
  expect(radioValue('maintenanceCovers')).toBe('services');
  expect(pickedServiceKeys('maintenanceServiceList')).toEqual(['plex']);
  expect(radioValue('maintenanceEnd')).toBe('never');
  expect(document.getElementById('maintenanceNotice').value).toBe('90');
});

test('editing a one-time date displays its timezone and preserves an explicit DST offset', async () => {
  editMaintenanceSchedule({
    id: 'fold', name: 'Clock change', message: 'Maintenance', level: 'warning', schedule_type: 'once',
    starts_at: '2026-10-25T01:30:42Z', ends_at: '2026-11-01T12:00:00Z', timezone: 'Europe/London', enabled: true
  });
  expect(document.getElementById('maintenanceStartsAt').value).toBe('2026-10-25T01:30');
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ id: 'fold', starts_at: '2026-10-25T01:30:42Z' });
});

test('a changed timezone submits new wall time rather than retaining the old instant', async () => {
  editMaintenanceSchedule({
    id: 'once', name: 'Move', message: 'Maintenance', level: 'warning', schedule_type: 'once',
    starts_at: '2026-09-15T09:00:00Z', ends_at: '', timezone: 'Europe/London', enabled: true
  });
  expect(document.getElementById('maintenanceStartsAt').value).toBe('2026-09-15T10:00');
  document.getElementById('maintenanceTimezone').value = 'UTC';
  await saveMaintenanceSchedule();
  expect(postedSchedule().starts_at).toBe('2026-09-15T10:00');
});

test('API validation errors retain the form and explain the problem', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-03-29T01:30';
  choose('maintenanceEnd', 'never');
  j.mockRejectedValue(Object.assign(new Error('HTTP 400'), { body: 'starts_at: this local time does not exist because the clocks change\n' }));
  await saveMaintenanceSchedule();
  expect(formError().hidden).toBe(false);
  expect(formError().textContent).toBe('starts_at: this local time does not exist because the clocks change');
  expect(document.getElementById('maintenanceStartsAt').value).toBe('2026-03-29T01:30');
  expect(document.getElementById('saveMaintenanceSchedule').disabled).toBe(false);
});

test('does not submit duplicate windows while a save is pending', async () => {
  choose('maintenanceRepeat', 'daily');
  let finish;
  j.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  const firstSave = saveMaintenanceSchedule();
  await saveMaintenanceSchedule();
  expect(j).toHaveBeenCalledTimes(1);
  finish({});
  await firstSave;
  expect(document.getElementById('saveMaintenanceSchedule').disabled).toBe(false);
});

test('a successful save resets the form and refreshes both lists', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  await saveMaintenanceSchedule();
  const urls = j.mock.calls.map(([url]) => url);
  expect(urls).toContain('/api/admin/status-alerts');
  expect(urls.filter(url => url === '/api/admin/maintenance-schedules')).toHaveLength(2);
  expect(loadBanners).toHaveBeenCalled();
  expect(document.getElementById('maintenanceName').value).toBe('');
});

test('one-time summaries show both local dates or an explicit open end', () => {
  expect(maintenanceTimingSummary({ schedule_type: 'once', starts_at: '2026-09-15T09:00:00Z', ends_at: '2026-12-01T12:00:00Z', timezone: 'Europe/London' }))
    .toBe('15 Sep 2026, 10:00 to 1 Dec 2026, 12:00');
  expect(maintenanceTimingSummary({ schedule_type: 'once', starts_at: '2026-09-15T09:00:00Z', timezone: 'UTC' }))
    .toBe('15 Sep 2026, 09:00 to when you end it');
  expect(maintenanceTimingSummary({ schedule_type: 'weekly', weekdays: [1, 3, 5], start_time: '03:00', duration_minutes: 60 }))
    .toBe('Mon, Wed and Fri at 03:00 for 1 hour');
  expect(maintenanceTimingSummary({ schedule_type: 'once', starts_at: '2026-10-04T21:00:00Z', ends_at: '2026-10-04T22:30:00Z', timezone: 'Europe/London' }))
    .toBe('4 Oct 2026, 22:00 to 23:30');
});

describe('the window list', () => {
  const now = Date.now();
  const iso = minutes => new Date(now + minutes * 60000).toISOString();
  const windows = [
    { id: 'off', name: 'Old', message: 'm', level: 'info', schedule_type: 'daily', start_time: '01:00', duration_minutes: 10, timezone: 'UTC', enabled: false },
    { id: 'next', name: 'Monday maintenance', message: 'Scheduled maintenance', level: 'warning', weekday: 1, weekdays: [1],
      schedule_type: 'weekly', start_time: '02:55', duration_minutes: 30, timezone: 'Europe/London', enabled: true,
      suppress_monitoring: true, next_starts_at: iso(600) },
    { id: 'running', name: 'NAS disk', message: '<b>New disk</b>', level: 'warning', schedule_type: 'once',
      starts_at: iso(-10), timezone: 'UTC', enabled: true, active: true, active_ends_at: iso(50),
      service_keys: ['nas', 'plex'], notice_minutes: 60 },
    { id: 'broken', name: 'Broken', message: 'm', level: 'info', schedule_type: 'daily', start_time: '01:00',
      duration_minutes: 10, timezone: 'Bad/Zone', enabled: true, problem: 'invalid timezone' },
  ];

  beforeEach(() => {
    latestMaintenanceWindows = windows;
    renderMaintenanceSchedules(windows);
  });

  const rows = () => [...document.querySelectorAll('#maintenanceSchedulesList .banner-row')];

  test('puts problems and running windows first, with their state in words', () => {
    expect(rows().map(row => row.dataset.id)).toEqual(['broken', 'running', 'next', 'off']);
    expect(rows().map(row => row.querySelector('.state-chip').textContent))
      .toEqual(['Needs attention', 'Running', expect.stringMatching(/^Next /), 'Off']);
    expect(rows()[0].textContent).toContain('invalid timezone');
  });

  test('describes what each window covers, escaping its text', () => {
    const running = rows()[1];
    expect(running.querySelector('.banner-row-meta').textContent).toMatch(/^Until .* · UTC · Covers NAS and Plex · Pauses monitoring|^Until .* · UTC · Covers NAS and Plex · Banner only/);
    expect(running.innerHTML).not.toContain('<b>New disk</b>');
    expect(running.textContent).toContain('<b>New disk</b>');
    expect(rows()[2].querySelector('.banner-row-meta').textContent).toContain('Mondays at 02:55 for 30 min · Europe/London · Covers all services');
  });

  test('the switch turns a window off without opening it', () => {
    const toggle = rows()[2].querySelector('[data-window-action="toggle"]');
    toggle.checked = false;
    toggle.dispatchEvent(new Event('change', { bubbles: true }));
    const body = postedSchedule();
    expect(body).toMatchObject({ id: 'next', enabled: false, start_time: '02:55' });
    expect(body).not.toHaveProperty('next_starts_at');
  });

  test('edit and delete act on the right window', () => {
    rows()[1].querySelector('[data-window-action="edit"]').click();
    expect(document.getElementById('maintenanceName').value).toBe('NAS disk');

    globalThis.confirm = jest.fn(() => true);
    rows()[3].querySelector('[data-window-action="delete"]').click();
    expect(j).toHaveBeenCalledWith('/api/admin/maintenance-schedules?id=off', expect.objectContaining({ method: 'DELETE' }));
  });
});
