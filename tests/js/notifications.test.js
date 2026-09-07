const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

beforeAll(() => loadSource('core.js', 'admin-ui.js'));

function productionLikeButtonAction(btn, action, successMessage, onError) {
  return (async () => {
    try {
      await action();
      showToast(successMessage);
    } catch (err) {
      let message = err?.message || 'Action failed';
      if (typeof err?.body === 'string') message = err.body;
      if (err?.body && typeof err.body === 'object') {
        message = err.body.message || err.body.error || message;
      }
      if (typeof onError === 'function') await onError(err, message);
      else showToast(message, 'error');
    }
  })();
}

beforeEach(() => {
  jest.useFakeTimers();
  document.body.innerHTML = fs.readFileSync(path.resolve(__dirname, '../../web/templates/partials/_admin_alerts.html'), 'utf8');
  global.getCsrf = () => 'test-csrf';
  global.showToast = jest.fn();
  global.handleButtonAction = jest.fn(productionLikeButtonAction);
  global.j = jest.fn();
});

afterEach(() => {
  jest.runOnlyPendingTimers();
  jest.useRealTimers();
});

test('notification settings are accessible for every provider with unique field IDs', () => {
  expect(document.querySelector('#alertsEnabled').closest('.notification-panel')).toBeNull();
  expect(document.querySelector('#alertOnUp').closest('.notification-panel')).toBeNull();
  const status = document.querySelector('#alertStatus');
  expect(status.closest('.notification-panel')).toBeNull();
  expect(status.getAttribute('role')).toBe('status');
  expect(status.getAttribute('aria-live')).toBe('polite');
  const ids = [...document.querySelectorAll('[id]')].map(el => el.id);
  expect(new Set(ids).size).toBe(ids.length);
});

test('Discord identity and silent delivery survive load and save', async () => {
  global.j.mockResolvedValueOnce({
    enabled: true,
    discord_enabled: true,
    discord_username: 'Operations',
    discord_silent: true,
    alert_on_up: true
  });
  await loadAlertsConfig();
  expect(document.querySelector('#discordUsername').value).toBe('Operations');
  expect(document.querySelector('#discordSilent').checked).toBe(true);

  global.j.mockResolvedValueOnce({ success: true }).mockResolvedValueOnce({
    enabled: true,
    discord_enabled: true,
    discord_username: 'Operations',
    discord_silent: true,
    alert_on_up: true
  });
  await saveAlertsConfig({ currentTarget: document.querySelector('.save-alerts-btn') });

  const saved = JSON.parse(global.j.mock.calls[1][1].body);
  expect(saved).toMatchObject({
    enabled: true,
    discord_enabled: true,
    discord_username: 'Operations',
    discord_silent: true,
    alert_on_up: true
  });
  expect(global.j.mock.calls[2][0]).toBe('/api/admin/alerts/config');
  expect(document.querySelector('#alertStatus').textContent).toBe('Configuration saved successfully');
});

test('stored notification credentials never render into form fields', async () => {
  global.j.mockResolvedValueOnce({
    smtp_password: 'must-not-render-smtp',
    smtp_password_configured: true,
    discord_webhook_url: 'must-not-render-discord',
    discord_webhook_configured: true,
    telegram_bot_token: 'must-not-render-telegram',
    telegram_bot_token_configured: true,
    webhook_url: 'must-not-render-webhook-url',
    webhook_url_configured: true,
    webhook_secret: 'must-not-render-webhook-secret',
    webhook_secret_configured: true
  });

  await loadAlertsConfig();

  for (const id of ['smtpPassword', 'discordWebhookUrl', 'telegramBotToken', 'webhookUrl', 'webhookSecret']) {
    const input = document.getElementById(id);
    expect(input.value).toBe('');
    expect(input.placeholder).toContain('Saved');
  }
  for (const id of ['clearSmtpPassword', 'clearDiscordWebhookUrl', 'clearTelegramBotToken', 'clearWebhookUrl', 'clearWebhookSecret']) {
    expect(document.getElementById(id).closest('.stored-secret-clear').classList.contains('hidden')).toBe(false);
  }
});

test('webhook save trims its URL and excludes unrelated hidden autofill', async () => {
  document.querySelector('#webhookUrl').value = '  https://api-user:secret@hooks.example.com/events?sig=token  ';
  document.querySelector('#webhookSecret').value = 'signing secret';
  document.querySelector('#smtpPassword').value = 'password-manager-smtp';
  document.querySelector('#discordWebhookUrl').value = 'password-manager-discord';
  document.querySelector('#telegramBotToken').value = 'password-manager-telegram';
  global.j.mockResolvedValueOnce({ success: true }).mockResolvedValueOnce({
    webhook_url_configured: true,
    webhook_secret_configured: true
  });

  const button = document.querySelector('[data-provider="webhook"] .save-alerts-btn');
  await saveAlertsConfig({ currentTarget: button });

  const saved = JSON.parse(global.j.mock.calls[0][1].body);
  expect(saved.webhook_url).toBe('https://api-user:secret@hooks.example.com/events?sig=token');
  expect(saved.webhook_secret).toBe('signing secret');
  expect(saved.clear_webhook_url).toBe(false);
  expect(saved.clear_webhook_secret).toBe(false);
  expect(saved).not.toHaveProperty('smtp_password');
  expect(saved).not.toHaveProperty('discord_webhook_url');
  expect(saved).not.toHaveProperty('telegram_bot_token');
  expect(saved).not.toHaveProperty('clear_smtp_password');
  expect(saved).not.toHaveProperty('clear_discord_webhook_url');
  expect(saved).not.toHaveProperty('clear_telegram_bot_token');
  expect(document.querySelector('#webhookUrl').value).toBe('');
});

test('Discord save trims only the Discord webhook replacement', async () => {
  document.querySelector('#discordWebhookUrl').value = '  https://discord.com/api/webhooks/123/token  ';
  document.querySelector('#webhookUrl').value = 'hidden-webhook-autofill';
  global.j.mockResolvedValueOnce({ success: true }).mockResolvedValueOnce({ discord_webhook_configured: true });

  const button = document.querySelector('[data-provider="discord"] .save-alerts-btn');
  await saveAlertsConfig({ currentTarget: button });

  const saved = JSON.parse(global.j.mock.calls[0][1].body);
  expect(saved.discord_webhook_url).toBe('https://discord.com/api/webhooks/123/token');
  expect(saved.clear_discord_webhook_url).toBe(false);
  expect(saved).not.toHaveProperty('webhook_url');
  expect(saved).not.toHaveProperty('webhook_secret');
});

test('saved webhook credentials can be explicitly cleared', async () => {
  global.j.mockResolvedValueOnce({ webhook_url_configured: true, webhook_secret_configured: true });
  await loadAlertsConfig();
  initStoredCredentialControls();

  const urlInput = document.querySelector('#webhookUrl');
  urlInput.value = 'should-be-discarded';
  const clearURL = document.querySelector('#clearWebhookUrl');
  clearURL.checked = true;
  clearURL.dispatchEvent(new Event('change'));
  expect(urlInput.disabled).toBe(true);
  expect(urlInput.value).toBe('');

  global.j.mockResolvedValueOnce({ success: true }).mockResolvedValueOnce({ webhook_url_configured: false });
  const button = document.querySelector('[data-provider="webhook"] .save-alerts-btn');
  await saveAlertsConfig({ currentTarget: button });

  const saved = JSON.parse(global.j.mock.calls[1][1].body);
  expect(saved.webhook_url).toBe('');
  expect(saved.clear_webhook_url).toBe(true);
  expect(saved.clear_webhook_secret).toBe(false);
});

test('Discord test sends the selected scenario and reports confirmed delivery', async () => {
  document.querySelector('#discordTestStatus').value = 'degraded';
  global.j.mockResolvedValue({ success: true, message: 'Test discord notification sent' });

  await sendTestNotification(document.querySelector('[data-channel="discord"]'));

  expect(JSON.parse(global.j.mock.calls[0][1].body)).toEqual({ channel: 'discord', status: 'degraded' });
  expect(document.querySelector('#alertStatus').textContent).toBe('Test discord notification sent');
  expect(document.querySelector('#alertStatus').classList.contains('success')).toBe(true);
});

test('failed delivery is shown inline with production error handling', async () => {
  global.j.mockResolvedValue({ success: false, message: 'Destination returned HTTP 401' });

  await sendTestNotification(document.querySelector('[data-channel="discord"]'));

  const status = document.querySelector('#alertStatus');
  expect(status.textContent).toBe('Destination returned HTTP 401');
  expect(status.classList.contains('error')).toBe(true);
  expect(status.classList.contains('success')).toBe(false);
  expect(global.showToast).toHaveBeenCalledWith('Destination returned HTTP 401', 'error');
});

test('save and load HTTP errors remain visible in the inline status', async () => {
  const saveError = new Error('HTTP 400');
  saveError.body = 'Discord webhook URL is invalid\n';
  global.j.mockRejectedValueOnce(saveError);

  const button = document.querySelector('[data-provider="discord"] .save-alerts-btn');
  await saveAlertsConfig({ currentTarget: button });

  let status = document.querySelector('#alertStatus');
  expect(status.textContent).toBe('Discord webhook URL is invalid');
  expect(status.classList.contains('error')).toBe(true);
  expect(global.j).toHaveBeenCalledTimes(1);

  const loadError = new Error('HTTP 500');
  loadError.body = { message: 'Unable to read saved notification configuration' };
  global.j.mockRejectedValueOnce(loadError);
  await loadAlertsConfig();

  status = document.querySelector('#alertStatus');
  expect(status.textContent).toBe('Unable to read saved notification configuration');
  expect(status.classList.contains('error')).toBe(true);
});
