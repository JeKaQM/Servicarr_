const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

const template = fs.readFileSync(path.resolve(__dirname, '../../web/templates/partials/_admin_services_banners.html'), 'utf8');

beforeAll(() => loadSource('core.js', 'utils.js', 'banners.js'));

beforeEach(() => {
  document.body.innerHTML = template;
  globalThis.j = jest.fn().mockResolvedValue({});
  globalThis.getCsrf = jest.fn().mockReturnValue('csrf');
  globalThis.showToast = jest.fn();
  globalThis.loadMaintenanceSchedules = jest.fn().mockResolvedValue();
  globalThis.loadBanners = jest.fn().mockResolvedValue();
  jest.spyOn(console, 'error').mockImplementation(() => {});
  resetMaintenanceScheduleForm();
  document.getElementById('maintenanceName').value = 'Server upgrade';
  document.getElementById('maintenanceMessage').value = 'Maintenance is in progress.';
});

afterEach(() => jest.restoreAllMocks());

function selectType(value) {
  document.getElementById('maintenanceScheduleType').value = value;
  updateMaintenanceScheduleForm();
}

function postedSchedule() {
  const [url, options] = j.mock.calls[0];
  expect(url).toBe('/api/admin/maintenance-schedules');
  expect(options.method).toBe('POST');
  expect(options.headers['X-CSRF-Token']).toBe('csrf');
  return JSON.parse(options.body);
}

test('one-time calendar fields are required and hidden recurrence fields do not block submission', () => {
  expect(document.getElementById('maintenanceStartsAt').required).toBe(true);
  expect(document.getElementById('maintenanceEndsAt').required).toBe(true);
  expect(document.getElementById('maintenanceStartTime').disabled).toBe(true);
  expect(document.getElementById('maintenanceWeekdaysField').disabled).toBe(true);
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  document.getElementById('maintenanceEndsAt').value = '2027-12-20T19:00';
  expect(document.getElementById('maintenanceScheduleForm').checkValidity()).toBe(true);
});

test('submits multi-month dates in the selected timezone without browser timezone conversion', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  document.getElementById('maintenanceEndsAt').value = '2027-12-20T19:00';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({
    schedule_type: 'once', starts_at: '2026-09-15T10:00', ends_at: '2027-12-20T19:00', timezone: 'Europe/London'
  });
  expect(postedSchedule()).not.toHaveProperty('duration_minutes');
  expect(loadMaintenanceSchedules).toHaveBeenCalledTimes(1);
});

test('open-ended dates disable the end field and submit an empty end', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-09-15T10:00';
  document.getElementById('maintenanceNoEnd').checked = true;
  updateMaintenanceScheduleForm();
  expect(document.getElementById('maintenanceEndsAt').disabled).toBe(true);
  expect(document.getElementById('maintenanceEndsAt').required).toBe(false);
  expect(document.getElementById('maintenanceScheduleForm').checkValidity()).toBe(true);
  await saveMaintenanceSchedule();
  expect(postedSchedule().ends_at).toBe('');
});

test('selected weekdays and multiweek durations submit without the old cap', async () => {
  selectType('weekly');
  document.querySelector('[name="maintenanceWeekdays"][value="5"]').checked = true;
  document.querySelector('[name="maintenanceWeekdays"][value="6"]').checked = true;
  document.getElementById('maintenanceDuration').value = '3';
  document.getElementById('maintenanceDurationUnit').value = '10080';
  expect(document.getElementById('maintenanceStartsAt').disabled).toBe(true);
  expect(document.getElementById('maintenanceStartTime').required).toBe(true);
  expect(document.getElementById('maintenanceScheduleForm').checkValidity()).toBe(true);
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ schedule_type: 'weekly', weekdays: [1, 5, 6], duration_minutes: 30240 });
});

test('daily repeats need no weekday selection and accept fractional hours', async () => {
  selectType('daily');
  document.getElementById('maintenanceDuration').value = '1.5';
  document.getElementById('maintenanceDurationUnit').value = '60';
  await saveMaintenanceSchedule();
  expect(postedSchedule()).toMatchObject({ schedule_type: 'daily', duration_minutes: 90 });
  expect(postedSchedule()).not.toHaveProperty('weekdays');
});

test('empty weekday selections are explained before making a request', async () => {
  selectType('weekly');
  document.querySelectorAll('[name="maintenanceWeekdays"]').forEach(input => { input.checked = false; });
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(showToast).toHaveBeenCalledWith('Choose at least one weekday', 'error');
});

test.each(['0', '-1', '0.5', '153722868'])('rejects invalid minute duration %s', async value => {
  selectType('daily');
  document.getElementById('maintenanceDuration').value = value;
  await saveMaintenanceSchedule();
  expect(j).not.toHaveBeenCalled();
  expect(showToast).toHaveBeenCalledWith(expect.stringContaining('whole minute'), 'error');
});

test('editing a legacy schedule keeps its weekday and sensible duration units', () => {
  editMaintenanceSchedule({
    id: 'legacy', name: 'Old schedule', message: 'Maintenance', level: 'warning', weekday: 6,
    start_time: '23:30', duration_minutes: 2880, timezone: 'Asia/Kathmandu', enabled: true, suppress_monitoring: true
  });
  expect(document.getElementById('maintenanceScheduleType').value).toBe('weekly');
  expect(document.querySelector('[name="maintenanceWeekdays"][value="6"]').checked).toBe(true);
  expect(document.querySelector('[name="maintenanceWeekdays"][value="1"]').checked).toBe(false);
  expect(document.getElementById('maintenanceDuration').value).toBe('2');
  expect(document.getElementById('maintenanceDurationUnit').value).toBe('1440');
  expect(document.getElementById('maintenanceTimezone').value).toBe('Asia/Kathmandu');
});

test('editing a one-time date displays its timezone and preserves an explicit DST offset', async () => {
  editMaintenanceSchedule({
    id: 'fold', name: 'Clock change', message: 'Maintenance', level: 'warning', schedule_type: 'once',
    starts_at: '2026-10-25T01:30:42Z', ends_at: '2026-11-01T12:00:00Z', timezone: 'Europe/London', enabled: true
  });
  expect(document.getElementById('maintenanceStartsAt').value).toBe('2026-10-25T01:30');
  await saveMaintenanceSchedule();
  expect(postedSchedule().starts_at).toBe('2026-10-25T01:30:42Z');
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

test('API validation errors retain the form and explain the invalid date', async () => {
  document.getElementById('maintenanceStartsAt').value = '2026-03-29T01:30';
  document.getElementById('maintenanceNoEnd').checked = true;
  j.mockRejectedValue(Object.assign(new Error('HTTP 400'), { body: 'starts_at: this local time does not exist because the clocks change\n' }));
  await saveMaintenanceSchedule();
  expect(showToast).toHaveBeenCalledWith('starts_at: this local time does not exist because the clocks change', 'error');
  expect(document.getElementById('maintenanceStartsAt').value).toBe('2026-03-29T01:30');
  expect(document.getElementById('saveMaintenanceSchedule').disabled).toBe(false);
});

test('does not submit duplicate schedules while a save is pending', async () => {
  selectType('daily');
  let finish;
  j.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  const firstSave = saveMaintenanceSchedule();
  await saveMaintenanceSchedule();
  expect(j).toHaveBeenCalledTimes(1);
  finish({});
  await firstSave;
  expect(document.getElementById('saveMaintenanceSchedule').disabled).toBe(false);
});

test('one-time summaries show both local dates or an explicit open end', () => {
  expect(maintenanceTimingSummary({ schedule_type: 'once', starts_at: '2026-09-15T09:00:00Z', ends_at: '2026-12-01T12:00:00Z', timezone: 'Europe/London' }))
    .toBe('2026-09-15 10:00 to 2026-12-01 12:00');
  expect(maintenanceTimingSummary({ schedule_type: 'once', starts_at: '2026-09-15T09:00:00Z', timezone: 'UTC' }))
    .toContain('no scheduled end');
});
