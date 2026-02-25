async function saveAlertsConfig(e) {
  const statusEl = $('#alertStatus');
  const btn = (e && e.target) ? e.target : $('#saveAlerts');

  const config = {
    enabled: $('#alertsEnabled').checked,
    smtp_host: $('#smtpHost').value,
    smtp_port: parseInt($('#smtpPort').value) || 587,
    smtp_user: $('#smtpUser').value,
    smtp_password: $('#smtpPassword').value,
    alert_email: $('#alertEmail').value,
    from_email: $('#alertFromEmail').value,
    status_page_url: $('#statusPageUrl').value.trim(),
    smtp_skip_verify: $('#smtpSkipVerify').checked,
    alert_on_down: $('#alertOnDown').checked,
    alert_on_degraded: $('#alertOnDegraded').checked,
    alert_on_up: $('#alertOnUp').checked,
    // Multi-channel
    discord_webhook_url: $('#discordWebhookUrl') ? $('#discordWebhookUrl').value : '',
    discord_enabled: $('#discordEnabled') ? $('#discordEnabled').checked : false,
    telegram_bot_token: $('#telegramBotToken') ? $('#telegramBotToken').value : '',
    telegram_chat_id: $('#telegramChatId') ? $('#telegramChatId').value : '',
    telegram_enabled: $('#telegramEnabled') ? $('#telegramEnabled').checked : false,
    webhook_url: $('#webhookUrl') ? $('#webhookUrl').value : '',
    webhook_secret: $('#webhookSecret') ? $('#webhookSecret').value : '',
    webhook_enabled: $('#webhookEnabled') ? $('#webhookEnabled').checked : false
  };

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

      statusEl.textContent = 'Configuration saved successfully';
      statusEl.className = 'status-message success';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    },
    'Configuration saved'
  );
}

async function sendTestEmail() {
  const statusEl = $('#alertStatus');
  const btn = $('#testEmail');

  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/alerts/test', {
        method: 'POST',
        headers: { 'X-CSRF-Token': getCsrf() }
      });

      statusEl.textContent = result.message || 'Test email sent successfully';
      statusEl.className = 'status-message success';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 5000);
    },
    'Test email sent'
  );
}

async function loadAlertsConfig() {
  try {
    const config = await j('/api/admin/alerts/config');
    if (config) {
      $('#alertsEnabled').checked = config.enabled || false;
      $('#smtpHost').value = config.smtp_host || '';
      $('#smtpPort').value = config.smtp_port || 587;
      $('#smtpUser').value = config.smtp_user || '';
      $('#smtpPassword').value = config.smtp_password || '';
      $('#alertEmail').value = config.alert_email || '';
      $('#alertFromEmail').value = config.from_email || '';
      $('#statusPageUrl').value = config.status_page_url || '';
      $('#smtpSkipVerify').checked = config.smtp_skip_verify || false;
      $('#alertOnDown').checked = config.alert_on_down !== false;
      $('#alertOnDegraded').checked = config.alert_on_degraded !== false;
      $('#alertOnUp').checked = config.alert_on_up || false;
      // Multi-channel
      if ($('#discordWebhookUrl')) $('#discordWebhookUrl').value = config.discord_webhook_url || '';
      if ($('#discordEnabled')) $('#discordEnabled').checked = config.discord_enabled || false;
      if ($('#telegramBotToken')) $('#telegramBotToken').value = config.telegram_bot_token || '';
      if ($('#telegramChatId')) $('#telegramChatId').value = config.telegram_chat_id || '';
      if ($('#telegramEnabled')) $('#telegramEnabled').checked = config.telegram_enabled || false;
      if ($('#webhookUrl')) $('#webhookUrl').value = config.webhook_url || '';
      if ($('#webhookSecret')) $('#webhookSecret').value = config.webhook_secret || '';
      if ($('#webhookEnabled')) $('#webhookEnabled').checked = config.webhook_enabled || false;
    }
  } catch (err) {
    // No alerts config available
  }
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
        populateBannerScopeDropdown();
      }
    });
  });

  // Alerts form handlers
  const saveAlertsBtn = $('#saveAlerts');
  if (saveAlertsBtn) {
    saveAlertsBtn.addEventListener('click', saveAlertsConfig);
  }
  // Also wire up all save-alerts-btn buttons in channel panels
  $$('.save-alerts-btn').forEach(btn => {
    btn.addEventListener('click', saveAlertsConfig);
  });
  // Test channel buttons
  $$('.test-channel-btn').forEach(btn => {
    btn.addEventListener('click', async function() {
      const channel = this.getAttribute('data-channel');
      try {
        const result = await j('/api/admin/alerts/test-channel', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
          body: JSON.stringify({ channel })
        });
        alert(result.message || `Test ${channel} notification sent`);
      } catch (err) {
        alert(`Failed to send test: ${err.message || err}`);
      }
    });
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