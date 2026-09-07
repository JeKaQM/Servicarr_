async function saveAlertsConfig(e) {
  const btn = (e && e.currentTarget) ? e.currentTarget : $('#saveAlerts');
  const provider = btn && btn.closest('.notification-panel')?.getAttribute('data-provider');

  const config = {
    enabled: $('#alertsEnabled').checked,
    smtp_host: $('#smtpHost').value,
    smtp_port: parseInt($('#smtpPort').value) || 587,
    smtp_user: $('#smtpUser').value,
    alert_email: $('#alertEmail').value,
    from_email: $('#alertFromEmail').value,
    status_page_url: $('#statusPageUrl').value.trim(),
    smtp_skip_verify: $('#smtpSkipVerify').checked,
    alert_on_down: $('#alertOnDown').checked,
    alert_on_degraded: $('#alertOnDegraded').checked,
    alert_on_up: $('#alertOnUp').checked,
    // Multi-channel
    discord_enabled: $('#discordEnabled') ? $('#discordEnabled').checked : false,
    discord_username: $('#discordUsername') ? $('#discordUsername').value.trim() : '',
    discord_silent: $('#discordSilent') ? $('#discordSilent').checked : false,
    telegram_chat_id: $('#telegramChatId') ? $('#telegramChatId').value : '',
    telegram_enabled: $('#telegramEnabled') ? $('#telegramEnabled').checked : false,
    webhook_enabled: $('#webhookEnabled') ? $('#webhookEnabled').checked : false
  };

  // Only the active provider may contribute replacement secrets. Password
  // managers can populate hidden panels; those values must never overwrite or
  // invalidate an unrelated notification channel.
  if (provider === 'smtp') {
    config.smtp_password = $('#smtpPassword').value;
    config.clear_smtp_password = $('#clearSmtpPassword').checked;
  } else if (provider === 'discord') {
    config.discord_webhook_url = $('#discordWebhookUrl').value.trim();
    config.clear_discord_webhook_url = $('#clearDiscordWebhookUrl').checked;
  } else if (provider === 'telegram') {
    config.telegram_bot_token = $('#telegramBotToken').value;
    config.clear_telegram_bot_token = $('#clearTelegramBotToken').checked;
  } else if (provider === 'webhook') {
    config.webhook_url = $('#webhookUrl').value.trim();
    config.webhook_secret = $('#webhookSecret').value;
    config.clear_webhook_url = $('#clearWebhookUrl').checked;
    config.clear_webhook_secret = $('#clearWebhookSecret').checked;
  }

  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/alerts/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(config)
      });
      await loadAlertsConfig();

      setAlertStatus('Configuration saved successfully', 'success', 3000);
    },
    'Configuration saved',
    reportAlertActionError
  );
}

let alertStatusTimer = null;

function setAlertStatus(message, type, hideAfterMs = 0) {
  const statusEl = $('#alertStatus');
  if (!statusEl) return;
  if (alertStatusTimer) clearTimeout(alertStatusTimer);
  statusEl.textContent = message;
  statusEl.className = `status-message ${type}`;
  statusEl.classList.remove('hidden');
  alertStatusTimer = hideAfterMs > 0
    ? setTimeout(() => statusEl.classList.add('hidden'), hideAfterMs)
    : null;
}

function alertErrorMessage(err, fallback) {
  if (typeof err?.body === 'string' && err.body.trim()) return err.body.trim();
  if (err?.body && typeof err.body === 'object') {
    return err.body.message || err.body.error || err.message || fallback;
  }
  return err?.message || fallback;
}

function reportAlertActionError(err, message) {
  const rawDetail = message || alertErrorMessage(err, 'Notification action failed');
  const detail = typeof rawDetail === 'string' ? rawDetail.trim() : String(rawDetail);
  setAlertStatus(detail, 'error');
  if (typeof showToast === 'function') showToast(detail, 'error');
}

function setStoredCredentialField(inputSelector, clearSelector, configured, legacyValue, emptyPlaceholder) {
  const input = $(inputSelector);
  const clear = $(clearSelector);
  if (!input || !clear) return;

  // Older servers may still return the secret itself. Use only its presence as
  // a compatibility signal; never copy the value into the page.
  const hasStoredValue = configured === true || (typeof configured !== 'boolean' && Boolean(legacyValue));
  input.value = '';
  input.placeholder = hasStoredValue ? 'Saved — enter a replacement' : emptyPlaceholder;
  input.disabled = false;
  clear.checked = false;
  clear.disabled = !hasStoredValue;
  const control = clear.closest('.stored-secret-clear');
  if (control) control.classList.toggle('hidden', !hasStoredValue);
}

async function sendTestEmail() {
  const btn = $('#testEmail');

  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/alerts/test', {
        method: 'POST',
        headers: { 'X-CSRF-Token': getCsrf() }
      });

      setAlertStatus(result.message || 'Test email sent successfully', 'success', 5000);
    },
    'Test email sent',
    reportAlertActionError
  );
}

async function sendTestNotification(btn) {
  const channel = btn.getAttribute('data-channel');
  const status = channel === 'discord' ? ($('#discordTestStatus')?.value || 'test') : 'test';
  await handleButtonAction(btn, async () => {
    const result = await j('/api/admin/alerts/test-channel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ channel, status })
    });
    if (result.success !== true) throw new Error(result.message || 'Notification delivery failed');
    setAlertStatus(result.message || `Test ${channel} notification sent`, 'success');
  }, 'Test notification delivered', reportAlertActionError);
}

async function loadAlertsConfig() {
  try {
    const config = await j('/api/admin/alerts/config');
    if (config) {
      $('#alertsEnabled').checked = config.enabled || false;
      $('#smtpHost').value = config.smtp_host || '';
      $('#smtpPort').value = config.smtp_port || 587;
      $('#smtpUser').value = config.smtp_user || '';
      setStoredCredentialField('#smtpPassword', '#clearSmtpPassword', config.smtp_password_configured, config.smtp_password, '••••••••');
      $('#alertEmail').value = config.alert_email || '';
      $('#alertFromEmail').value = config.from_email || '';
      $('#statusPageUrl').value = config.status_page_url || '';
      $('#smtpSkipVerify').checked = config.smtp_skip_verify || false;
      $('#alertOnDown').checked = config.alert_on_down !== false;
      $('#alertOnDegraded').checked = config.alert_on_degraded !== false;
      $('#alertOnUp').checked = config.alert_on_up || false;
      // Multi-channel
      setStoredCredentialField('#discordWebhookUrl', '#clearDiscordWebhookUrl', config.discord_webhook_configured, config.discord_webhook_url, 'https://discord.com/api/webhooks/...');
      if ($('#discordEnabled')) $('#discordEnabled').checked = config.discord_enabled || false;
      if ($('#discordUsername')) $('#discordUsername').value = config.discord_username || '';
      if ($('#discordSilent')) $('#discordSilent').checked = config.discord_silent || false;
      setStoredCredentialField('#telegramBotToken', '#clearTelegramBotToken', config.telegram_bot_token_configured, config.telegram_bot_token, '123456:ABC-DEF...');
      if ($('#telegramChatId')) $('#telegramChatId').value = config.telegram_chat_id || '';
      if ($('#telegramEnabled')) $('#telegramEnabled').checked = config.telegram_enabled || false;
      setStoredCredentialField('#webhookUrl', '#clearWebhookUrl', config.webhook_url_configured, config.webhook_url, 'https://your-endpoint.com/webhook');
      setStoredCredentialField('#webhookSecret', '#clearWebhookSecret', config.webhook_secret_configured, config.webhook_secret, 'your-hmac-secret');
      if ($('#webhookEnabled')) $('#webhookEnabled').checked = config.webhook_enabled || false;
    }
  } catch (err) {
    reportAlertActionError(err, alertErrorMessage(err, 'Failed to load notification configuration'));
  }
}

function initStoredCredentialControls() {
  $$('.stored-secret-clear input[data-secret-input]').forEach(clear => {
    clear.addEventListener('change', () => {
      const input = document.getElementById(clear.getAttribute('data-secret-input'));
      if (!input) return;
      input.disabled = clear.checked;
      if (clear.checked) input.value = '';
    });
  });
}

// ============ Service Dependencies ============

function populateDependsOnDropdown(currentServiceKey) {
  const container = $('#serviceDependsOnList');
  if (!container) return;
  container.innerHTML = '';
  const available = servicesData.filter(svc => svc.key !== currentServiceKey);
  if (available.length === 0) {
    container.innerHTML = '<span class="muted" style="font-size:12px;">No other services available</span>';
    return;
  }
  available.forEach(svc => {
    const label = document.createElement('label');
    label.className = 'depends-on-option';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.value = svc.key;
    cb.className = 'depends-on-cb';
    label.appendChild(cb);
    label.appendChild(document.createTextNode(' ' + (svc.name || svc.key)));
    container.appendChild(label);
  });
}

function populateConnectedToList(currentServiceKey) {
  const container = $('#serviceConnectedToList');
  if (!container) return;
  container.innerHTML = '';
  const available = servicesData.filter(svc => svc.key !== currentServiceKey);
  if (available.length === 0) {
    container.innerHTML = '<span class="muted" style="font-size:12px;">No other services available</span>';
    return;
  }
  available.forEach(svc => {
    const label = document.createElement('label');
    label.className = 'depends-on-option';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.value = svc.key;
    cb.className = 'connected-to-cb';
    label.appendChild(cb);
    label.appendChild(document.createTextNode(' ' + (svc.name || svc.key)));
    container.appendChild(label);
  });
}

async function checkNowFor(card) {
  const btn = $('.checkNow', card);
  const key = card.getAttribute('data-key');
  const toggle = $('.monitorToggle', card);

  // Don't allow checks on disabled services
  if (toggle && !toggle.checked) {
    showToast('Cannot check disabled services', 'error');
    return;
  }

  await handleButtonAction(
    btn,
    async () => {
      const res = await j('/api/admin/check', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify({ service: key })
      });
      updCard('card-' + key, res);
    },
    `Check completed for ${key}`
  );
}

// ---- Admin-only initialization (admin bundle) ----
// This only runs for authenticated admin users. Core dashboard init is in app-init.js.
window.addEventListener('load', async () => {
  // Initialize services management (admin features)
  initServicesManagement();

  // Initialize settings tab (admin features)
  initSettingsTab();

  const ingestBtn = $('#ingestNow');
  if (ingestBtn) {
    ingestBtn.addEventListener('click', ingestAll);
  }

  const resetBtn = $('#resetRecent');
  if (resetBtn) {
    resetBtn.addEventListener('click', resetRecent);
  }

  // Tab functionality in admin panel
  const ingestBtnTab = $('#ingestNowTab');
  if (ingestBtnTab) {
    ingestBtnTab.addEventListener('click', ingestAll);
  }

  const resetBtnTab = $('#resetRecentTab');
  if (resetBtnTab) {
    resetBtnTab.addEventListener('click', resetRecent);
  }

  // Tab switching
  const tabBtns = $$('.tab-btn');
  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      const tabName = btn.getAttribute('data-tab');

      // Update active tab button
      tabBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      // Update active tab content
      $$('.tab-content').forEach(content => content.classList.remove('active'));
      const activeContent = $(`#tab-${tabName}`);
      if (activeContent) {
        activeContent.classList.add('active');
      }

      // Load data when tabs are clicked
      if (tabName === 'security') {
        loadSecurityData();
      } else if (tabName === 'banners') {
        loadAdminBanners();
        loadMaintenanceSchedules();
        populateBannerScopeDropdown();
      }
    });
  });

  // All channel and global save buttons share one handler.
  $$('.save-alerts-btn').forEach(btn => {
    btn.addEventListener('click', saveAlertsConfig);
  });
  initStoredCredentialControls();
  $$('.test-channel-btn').forEach(btn => {
    btn.addEventListener('click', () => sendTestNotification(btn));
  });

  const testEmailBtn = $('#testEmail');
  if (testEmailBtn) {
    testEmailBtn.addEventListener('click', sendTestEmail);
  }

  // Resources config handlers
  const saveResourcesBtn = $('#saveResources');
  if (saveResourcesBtn) {
    saveResourcesBtn.addEventListener('click', saveResourcesConfig);
  }

  const testGlancesBtn = $('#testGlances');
  if (testGlancesBtn) {
    testGlancesBtn.addEventListener('click', testGlancesConnection);
  }

  const testUPSBtn = $('#testUPS');
  if (testUPSBtn) {
    testUPSBtn.addEventListener('click', testUPSConnection);
  }

  // Security tab handlers
  const resetBlocksBtn = $('#resetBlocks');
  if (resetBlocksBtn) {
    resetBlocksBtn.addEventListener('click', clearAllBlocks);
  }

  const addWhitelistBtn = $('#addWhitelist');
  if (addWhitelistBtn) {
    addWhitelistBtn.addEventListener('click', addToWhitelist);
  }

  const addBlacklistBtn = $('#addBlacklist');
  if (addBlacklistBtn) {
    addBlacklistBtn.addEventListener('click', addToBlacklist);
  }

  $$('.checkNow').forEach(btn =>
    btn.addEventListener('click', () => checkNowFor(btn.closest('.card')))
  );

  $$('.monitorToggle').forEach(toggle =>
    toggle.addEventListener('change', (e) => toggleMonitoring(e.target.closest('.card'), e.target.checked))
  );

  // Banner management
  const createBannerBtn = $('#createBanner');
  if (createBannerBtn) {
    createBannerBtn.addEventListener('click', createBanner);
  }

  const cancelBannerEdit = $('#cancelBannerEdit');
  if (cancelBannerEdit) {
    cancelBannerEdit.addEventListener('click', resetBannerForm);
  }

  const maintenanceForm = $('#maintenanceScheduleForm');
  if (maintenanceForm) {
    maintenanceForm.addEventListener('submit', saveMaintenanceSchedule);
    maintenanceForm.addEventListener('change', updateMaintenanceScheduleForm);
    updateMaintenanceScheduleForm();
  }

  const cancelMaintenanceBtn = $('#cancelMaintenanceSchedule');
  if (cancelMaintenanceBtn) {
    cancelMaintenanceBtn.addEventListener('click', resetMaintenanceScheduleForm);
  }

  // Banner template selection
  const bannerTemplate = $('#bannerTemplate');
  if (bannerTemplate) {
    bannerTemplate.addEventListener('change', () => {
      const msgInput = $('#bannerMessage');
      if (msgInput && bannerTemplate.value) {
        msgInput.value = bannerTemplate.value;
        bannerTemplate.value = ''; // Reset dropdown
      }
    });
  }
});
