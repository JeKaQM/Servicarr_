const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

beforeAll(() => loadSource('core.js', 'admin-ui.js'));

beforeEach(() => {
  document.body.innerHTML = fs.readFileSync(path.resolve(__dirname, '../../web/templates/partials/_admin_alerts.html'), 'utf8');
  global.getCsrf = () => 'test-csrf';
  global.handleButtonAction = jest.fn(async (btn, action) => action());
  global.j = jest.fn();
});

test('notification settings are accessible for every provider with unique field IDs', () => {
  expect(document.querySelector('#alertsEnabled').closest('.notification-panel')).toBeNull();
  expect(document.querySelector('#alertOnUp').closest('.notification-panel')).toBeNull();
  expect(document.querySelector('#alertStatus').closest('.notification-panel')).toBeNull();
  const ids = [...document.querySelectorAll('[id]')].map(el => el.id);
  expect(new Set(ids).size).toBe(ids.length);
});

test('Discord identity and silent delivery survive load and save', async () => {
  jest.useFakeTimers();
  global.j.mockResolvedValueOnce({ enabled: true, discord_enabled: true, discord_username: 'Operations', discord_silent: true, alert_on_up: true });
  await loadAlertsConfig();
  expect(document.querySelector('#discordUsername').value).toBe('Operations');
  expect(document.querySelector('#discordSilent').checked).toBe(true);
  global.j.mockResolvedValueOnce({ success: true });
  await saveAlertsConfig({ currentTarget: document.querySelector('.save-alerts-btn') });
  const saved = JSON.parse(global.j.mock.calls[1][1].body);
  expect(saved).toMatchObject({ enabled: true, discord_enabled: true, discord_username: 'Operations', discord_silent: true, alert_on_up: true });
  jest.runOnlyPendingTimers();
  jest.useRealTimers();
});

test('Discord test sends the selected scenario and reports confirmed delivery', async () => {
  document.querySelector('#discordTestStatus').value = 'degraded';
  global.j.mockResolvedValue({ success: true, message: 'Test discord notification sent' });
  await sendTestNotification(document.querySelector('[data-channel="discord"]'));
  expect(JSON.parse(global.j.mock.calls[0][1].body)).toEqual({ channel: 'discord', status: 'degraded' });
  expect(document.querySelector('#alertStatus').textContent).toBe('Test discord notification sent');
});

test('failed delivery is never presented as successful', async () => {
  global.j.mockResolvedValue({ success: false, message: 'Destination returned HTTP 401' });
  await expect(sendTestNotification(document.querySelector('[data-channel="discord"]'))).rejects.toThrow('HTTP 401');
  expect(document.querySelector('#alertStatus').classList.contains('success')).toBe(false);
});
