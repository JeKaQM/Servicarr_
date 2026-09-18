// CrowdSec admin tab: configuration form, connection test, decisions dashboard.

function setCrowdsecStatus(message, type, hideAfterMs = 0) {
  const statusEl = $('#crowdsecStatus');
  if (!statusEl) return;
  if (typeof crowdsecStatusTimer !== 'undefined' && crowdsecStatusTimer) clearTimeout(crowdsecStatusTimer);
  statusEl.textContent = message;
  statusEl.className = `status-message ${type}`;
  statusEl.classList.remove('hidden');
  if (hideAfterMs > 0) {
    window.crowdsecStatusTimer = setTimeout(() => statusEl.classList.add('hidden'), hideAfterMs);
  } else {
    window.crowdsecStatusTimer = null;
  }
}

function crowdsecErrorMessage(err, fallback) {
  if (typeof err?.body === 'string' && err.body.trim()) return err.body.trim();
  if (err?.body && typeof err.body === 'object') {
    return err.body.message || err.body.error || err.message || fallback;
  }
  return err?.message || fallback;
}

function setCrowdsecSecretField(inputSelector, clearSelector, configured) {
  const input = $(inputSelector);
  const clear = $(clearSelector);
  if (!input || !clear) return;
  input.value = '';
  input.placeholder = configured ? 'Saved — enter a replacement' : input.getAttribute('data-empty-placeholder') || '';
  input.disabled = false;
  clear.checked = false;
  clear.disabled = !configured;
  const control = clear.closest('.stored-secret-clear');
  if (control) control.classList.toggle('hidden', !configured);
}

async function loadCrowdsecConfig() {
  if (!$('#crowdsecForm')) return;
  try {
    const config = await j('/api/admin/crowdsec/config');
    $('#crowdsecEnabled').checked = !!(config && config.enabled);
    $('#crowdsecURL').value = (config && config.lapi_url) || '';
    $('#crowdsecMachineID').value = (config && config.machine_id) || '';
    $('#crowdsecInterval').value = (config && config.poll_interval_seconds) || 30;
    $('#crowdsecSkipVerify').checked = !!(config && config.tls_skip_verify);
    setCrowdsecSecretField('#crowdsecBouncerKey', '#clearCrowdsecBouncerKey', !!(config && config.bouncer_api_key_configured));
    setCrowdsecSecretField('#crowdsecMachinePassword', '#clearCrowdsecMachinePassword', !!(config && config.machine_password_configured));
  } catch (err) {
    setCrowdsecStatus(crowdsecErrorMessage(err, 'Failed to load CrowdSec configuration'), 'error');
  }
}

async function saveCrowdsecConfig(e) {
  const btn = (e && e.currentTarget) ? e.currentTarget : $('#saveCrowdsec');
  const config = {
    enabled: $('#crowdsecEnabled').checked,
    lapi_url: $('#crowdsecURL').value.trim(),
    machine_id: $('#crowdsecMachineID').value.trim(),
    machine_password: $('#crowdsecMachinePassword').value,
    bouncer_api_key: $('#crowdsecBouncerKey').value,
    clear_machine_password: $('#clearCrowdsecMachinePassword').checked,
    clear_bouncer_api_key: $('#clearCrowdsecBouncerKey').checked,
    poll_interval_seconds: parseInt($('#crowdsecInterval').value, 10) || 30,
    tls_skip_verify: $('#crowdsecSkipVerify').checked
  };

  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/crowdsec/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(config)
      });
      await loadCrowdsecConfig();
      setCrowdsecStatus('Configuration saved successfully', 'success', 3000);
    },
    'Configuration saved',
    (err) => setCrowdsecStatus(crowdsecErrorMessage(err, 'Failed to save CrowdSec configuration'), 'error')
  );
}

async function testCrowdsecConnection(e) {
  const btn = (e && e.currentTarget) ? e.currentTarget : $('#testCrowdsec');
  const req = {
    lapi_url: $('#crowdsecURL').value.trim(),
    machine_id: $('#crowdsecMachineID').value.trim(),
    machine_password: $('#crowdsecMachinePassword').value,
    bouncer_key: $('#crowdsecBouncerKey').value,
    tls_skip_verify: $('#crowdsecSkipVerify').checked
  };

  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/crowdsec/test', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(req)
      });
      const parts = ['LAPI reachable'];
      if (result && result.decisions_ok) parts.push('decisions readable');
      if (result && result.alerts_ok) parts.push('alerts readable');
      setCrowdsecStatus('Connection OK — ' + parts.join(', '), 'success', 6000);
    },
    'Testing connection',
    (err) => setCrowdsecStatus(crowdsecErrorMessage(err, 'Connection test failed'), 'error')
  );
}

function crowdsecSyncBadgeText(status) {
  if (!status) return 'Not synced yet';
  if (status.last_error) {
    return status.auth_failed ? 'Auth failed — check credentials' : 'Sync error: ' + status.last_error;
  }
  if (!status.last_sync) return 'Not synced yet';
  const when = new Date(status.last_sync);
  if (isNaN(when.getTime())) return 'Not synced yet';
  const mins = Math.max(0, Math.round((Date.now() - when.getTime()) / 60000));
  const ago = mins < 1 ? 'just now' : mins + ' min ago';
  return `Synced ${ago}${status.decision_count > status.snapshot_count ? ' — showing latest ' + status.snapshot_count + ' of ' + status.decision_count : ''}`;
}

function renderCrowdsecDecisions(decisions) {
  const container = $('#crowdsecDecisions');
  if (!container) return;
  if (!decisions || !decisions.length) {
    container.innerHTML = '<div class="muted">No decisions synced yet.</div>';
    return;
  }

  const rows = decisions.map(d => {
    // escapeHtml escapes <, >, &, " and ' — safe for both text nodes and
    // attribute interpolation (attribute breakout requires a raw quote).
    const type = escapeHtml(d.type || 'ban');
    const value = escapeHtml(d.value || '');
    const scope = escapeHtml(d.scope || 'Ip');
    const origin = escapeHtml(d.origin || '');
    const scenario = escapeHtml(d.scenario || '');
    const duration = escapeHtml(d.duration || '');
    const created = escapeHtml((d.created_at || '').replace('T', ' ').slice(0, 19));
    const attrTitle = escapeHtml(`${d.scope || 'Ip'}: ${d.value || ''}`);
    const attrID = escapeHtml(d.decision_id || '');
    return `<div class="crowdsec-decision-row" data-decision-id="${attrID}">
      <span class="crowdsec-decision-type ${type === 'ban' ? 'crowdsec-type-ban' : 'crowdsec-type-other'}">${type}</span>
      <span class="crowdsec-decision-value" title="${attrTitle}">${value}</span>
      <span class="crowdsec-decision-origin">${origin}</span>
      <span class="crowdsec-decision-scenario">${scenario}</span>
      <span class="crowdsec-decision-duration">${duration}</span>
      <span class="crowdsec-decision-created">${created}</span>
    </div>`;
  }).join('');

  container.innerHTML = `<div class="crowdsec-decision-row crowdsec-decision-header">
      <span>Type</span><span>Value</span><span>Origin</span><span>Scenario</span><span>Remaining</span><span>Created</span>
    </div>${rows}`;
}

async function loadCrowdsecDecisions() {
  if (!$('#crowdsecDecisions')) return;
  try {
    const [decisions, status] = await Promise.all([
      j('/api/admin/crowdsec/decisions?active=true'),
      j('/api/admin/crowdsec/status')
    ]);
    renderCrowdsecDecisions(decisions && decisions.decisions ? decisions.decisions : []);
    const badge = $('#crowdsecSyncBadge');
    if (badge) {
      badge.textContent = crowdsecSyncBadgeText(status);
      badge.className = 'crowdsec-badge' + (status && status.last_error ? ' crowdsec-badge-error' : (status && status.last_sync ? '' : ' muted'));
    }
  } catch (err) {
    const badge = $('#crowdsecSyncBadge');
    if (badge) badge.textContent = 'Sync unavailable';
  }
}

async function crowdsecSyncNow(e) {
  const btn = (e && e.currentTarget) ? e.currentTarget : $('#crowdsecSyncNow');
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/crowdsec/sync-now', {
        method: 'POST',
        headers: { 'X-CSRF-Token': getCsrf() }
      });
      await loadCrowdsecDecisions();
      setCrowdsecStatus('Sync completed', 'success', 3000);
    },
    'Syncing',
    (err) => setCrowdsecStatus(crowdsecErrorMessage(err, 'Sync failed'), 'error')
  );
}

function initCrowdsecTab() {
  if (!$('#crowdsecForm')) return;
  loadCrowdsecConfig();
  loadCrowdsecDecisions();
  const save = $('#saveCrowdsec');
  if (save) save.addEventListener('click', saveCrowdsecConfig);
  const test = $('#testCrowdsec');
  if (test) test.addEventListener('click', testCrowdsecConnection);
  const syncNow = $('#crowdsecSyncNow');
  if (syncNow) syncNow.addEventListener('click', crowdsecSyncNow);
}