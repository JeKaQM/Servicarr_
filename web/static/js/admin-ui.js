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
    alert_on_degraded_recovery: $('#alertOnDegradedRecovery').checked,
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
      $('#alertOnUp').checked = config.alert_on_up === true;
      $('#alertOnDegradedRecovery').checked = config.alert_on_degraded_recovery === true;
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

// ============ Service links ============
// The service editor's "Depends on" and "Connected to" pickers list the other
// services with their icon and status. A service that already needs this one,
// directly or through others, can't also be one of its dependencies: the loop
// would let the two silence each other's alerts, and the server rejects it.

function linkKeys(value) {
  return String(value || '').split(',').map(k => k.trim()).filter(Boolean);
}

// Admins link against the full list, hidden services included.
function linkCandidates() {
  if (Array.isArray(adminServicesData)) return adminServicesData;
  return Array.isArray(servicesData) ? servicesData : [];
}

// The chain of keys from "from" to "target" along depends_on links, or null
// when "from" doesn't need "target".
function linkDependencyPath(services, from, target) {
  const deps = new Map(services.map(s => [s.key, linkKeys(s.depends_on)]));
  const prev = new Map([[from, null]]);
  const queue = [from];
  while (queue.length) {
    const key = queue.shift();
    for (const next of deps.get(key) || []) {
      if (prev.has(next)) continue;
      prev.set(next, key);
      if (next === target) {
        const path = [next];
        for (let at = key; at !== null; at = prev.get(at)) path.unshift(at);
        return path;
      }
      queue.push(next);
    }
  }
  return null;
}

function linkStatus(svc) {
  if (svc.visible === false) return { cls: 'hidden-svc', text: 'Hidden' };
  const st = matrixStatusOf(svc);
  return st.statusClass === 'unknown' ? null : { cls: st.statusClass, text: st.statusLabel };
}

function buildLinkOption(svc, cls, opts) {
  const label = document.createElement('label');
  label.className = 'link-option' + (opts.disabled ? ' is-disabled' : '');
  label.dataset.search = ((svc.name || '') + ' ' + svc.key).toLowerCase();
  const cb = document.createElement('input');
  cb.type = 'checkbox';
  cb.value = svc.key;
  cb.className = cls;
  cb.checked = !!opts.checked;
  cb.disabled = !!opts.disabled;
  const icon = document.createElement('span');
  icon.className = 'link-option-icon';
  icon.innerHTML = serviceIconMarkup(svc);
  const name = document.createElement('span');
  name.className = 'link-option-name';
  name.textContent = svc.name || svc.key;
  const status = document.createElement('span');
  const st = linkStatus(svc);
  if (st) {
    status.className = 'link-option-status ' + st.cls;
    status.textContent = st.text;
  }
  label.append(cb, icon, name, status);
  if (opts.note) {
    const note = document.createElement('span');
    note.className = 'link-option-note' + (opts.warn ? ' is-warning' : '');
    note.textContent = opts.note;
    label.appendChild(note);
  }
  return label;
}

function fillLinkList(container, rows, searchId) {
  container.innerHTML = '';
  if (rows.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'link-picker-empty';
    empty.textContent = 'No other services yet';
    container.appendChild(empty);
  }
  rows.forEach(row => container.appendChild(row));
  // A filter only earns its place once the list scrolls.
  const search = document.getElementById(searchId);
  if (search) search.hidden = rows.length <= 6;
}

// Linked services first, then the rest, with unavailable ones last.
function orderLinkRows(entries) {
  const rank = e => (e.opts.checked ? 0 : e.opts.disabled ? 2 : 1);
  return entries
    .map((e, i) => ({ e, i }))
    .sort((a, b) => (rank(a.e) - rank(b.e)) || (a.i - b.i))
    .map(({ e }) => e.row);
}

function populateDependsOnDropdown(currentServiceKey, selectedKeys = []) {
  const container = $('#serviceDependsOnList');
  if (!container) return;
  const all = linkCandidates();
  const nameOf = key => {
    const svc = all.find(s => s.key === key);
    return svc ? (svc.name || svc.key) : key;
  };
  const selected = new Set(selectedKeys);

  const entries = all.filter(svc => svc.key !== currentServiceKey).map(svc => {
    const opts = { checked: selected.has(svc.key) };
    const path = currentServiceKey ? linkDependencyPath(all, svc.key, currentServiceKey) : null;
    if (path) {
      const via = path.slice(1, -1).map(nameOf);
      const reason = 'depends on ' + nameOf(currentServiceKey) + (via.length ? ' through ' + via.join(', ') : '');
      if (opts.checked) {
        // Older data can hold a loop; leave it editable so it can be fixed.
        opts.note = 'Loop: ' + reason + '. Uncheck to save.';
        opts.warn = true;
      } else {
        opts.disabled = true;
        opts.note = reason.charAt(0).toUpperCase() + reason.slice(1);
      }
    }
    return { opts, row: buildLinkOption(svc, 'depends-on-cb', opts) };
  });
  fillLinkList(container, orderLinkRows(entries), 'dependsOnSearch');

  const note = $('#neededByNote');
  if (note) {
    const neededBy = currentServiceKey
      ? all.filter(svc => linkKeys(svc.depends_on).includes(currentServiceKey)).map(svc => svc.name || svc.key)
      : [];
    note.hidden = neededBy.length === 0;
    note.textContent = neededBy.length
      ? 'Needed by ' + neededBy.join(', ') + ' (set on ' + (neededBy.length === 1 ? 'that service' : 'those services') + ').'
      : '';
  }
  updateLinkCounts();
}

function populateConnectedToList(currentServiceKey, selectedKeys = []) {
  const container = $('#serviceConnectedToList');
  if (!container) return;
  const all = linkCandidates();
  const selected = new Set(selectedKeys);

  const entries = all.filter(svc => svc.key !== currentServiceKey).map(svc => {
    // The map draws a connection set on either service, so show both here.
    const fromOther = !!currentServiceKey && linkKeys(svc.connected_to).includes(currentServiceKey);
    const name = svc.name || svc.key;
    const opts = { checked: selected.has(svc.key) || fromOther };
    if (fromOther && !selected.has(svc.key)) {
      opts.disabled = true;
      opts.note = 'Set on ' + name;
    } else if (fromOther) {
      opts.note = 'Also set on ' + name;
    }
    return { opts, row: buildLinkOption(svc, 'connected-to-cb', opts) };
  });
  fillLinkList(container, orderLinkRows(entries), 'connectedToSearch');
  updateLinkCounts();
}

function updateLinkCounts() {
  [['#serviceDependsOnList', '#dependsOnCount'], ['#serviceConnectedToList', '#connectedToCount']].forEach(([listSel, countSel]) => {
    const list = $(listSel), count = $(countSel);
    if (!list || !count) return;
    const n = list.querySelectorAll('input[type="checkbox"]:checked').length;
    count.textContent = n ? n + ' linked' : 'None';
  });
}

function filterLinkOptions(input) {
  const list = document.getElementById(input.dataset.linkFilter);
  if (!list) return;
  const query = input.value.trim().toLowerCase();
  let shown = 0;
  list.querySelectorAll('.link-option').forEach(opt => {
    opt.hidden = !!query && !opt.dataset.search.includes(query);
    if (!opt.hidden) shown++;
  });
  let empty = list.querySelector('.link-picker-empty[data-filter-empty]');
  if (!empty) {
    empty = document.createElement('div');
    empty.className = 'link-picker-empty';
    empty.dataset.filterEmpty = '';
    list.appendChild(empty);
  }
  empty.hidden = shown > 0 || !query;
  empty.textContent = empty.hidden ? '' : 'No services match "' + input.value.trim() + '"';
}

function resetLinkFilters() {
  $$('.link-picker-search').forEach(input => {
    input.value = '';
    filterLinkOptions(input);
  });
}

let linkPickersReady = false;
function initLinkPickers() {
  if (linkPickersReady) return;
  const section = $('#serviceLinks');
  if (!section) return;
  linkPickersReady = true;
  section.addEventListener('input', e => {
    if (e.target.matches('.link-picker-search')) filterLinkOptions(e.target);
  });
  section.addEventListener('change', e => {
    if (e.target.matches('.depends-on-cb, .connected-to-cb')) updateLinkCounts();
  });
  // Enter in a filter box must not submit the service form.
  section.addEventListener('keydown', e => {
    if (e.key === 'Enter' && e.target.matches('.link-picker-search')) e.preventDefault();
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
