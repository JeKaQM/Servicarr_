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
    // Clearing saved coordinates must also reset the in-memory map. Without
    // this reset the old location survives until the whole page is reloaded.
    if (typeof crowdsecMapHome !== 'undefined') {
      crowdsecMapHome = { lat: 51.5074, lng: -0.1278 };
    }
    if (config && typeof config.map_home_latitude === 'number') {
      const latInput = $('#crowdsecMapLat');
      if (latInput) latInput.value = config.map_home_latitude || '';
      const lngInput = $('#crowdsecMapLng');
      if (lngInput) lngInput.value = config.map_home_longitude || '';
      // 0,0 = unset → keep the London default on the canvas.
      if (config.map_home_latitude || config.map_home_longitude) {
        crowdsecMapHome = { lat: config.map_home_latitude, lng: config.map_home_longitude };
      }
    }
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
    tls_skip_verify: $('#crowdsecSkipVerify').checked,
    map_home_latitude: parseFloat($('#crowdsecMapLat').value) || 0,
    map_home_longitude: parseFloat($('#crowdsecMapLng').value) || 0
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
    clear_machine_password: $('#clearCrowdsecMachinePassword').checked,
    clear_bouncer_api_key: $('#clearCrowdsecBouncerKey').checked,
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
  return `Synced ${ago}`;
}

// Collapsed row counts for the expandable lists.
var crowdsecCollapsedRows = 5;
var crowdsecAlertsExpanded = false;
var crowdsecDecisionsExpanded = false;
var crowdsecAllAlerts = [];
var crowdsecAllDecisions = [];

function renderCrowdsecDecisions(decisions) {
  const container = $('#crowdsecDecisions');
  if (!container) return;
  crowdsecAllDecisions = decisions || [];

  const summary = $('#crowdsecDecisionsSummary');
  if (summary) {
    summary.textContent = crowdsecAllDecisions.length
      ? `${crowdsecAllDecisions.length} shown` : '';
  }

  if (!crowdsecAllDecisions.length) {
    container.innerHTML = '<div class="muted">No decisions synced yet.</div>';
    toggleCrowdsecExpand('#crowdsecDecisionsExpand', false);
    return;
  }

  const visible = crowdsecDecisionsExpanded
    ? crowdsecAllDecisions
    : crowdsecAllDecisions.slice(0, crowdsecCollapsedRows);

  const rows = visible.map(d => {
    // escapeHtml escapes <, >, &, " and ' — safe for both text nodes and
    // attribute interpolation (attribute breakout requires a raw quote).
    const type = escapeHtml(d.type || 'ban');
    const value = escapeHtml(d.value || '');
    const scope = escapeHtml(d.scope || 'Ip');
    const origin = escapeHtml(d.origin || '');
    const scenario = escapeHtml(d.scenario || '');
    const duration = escapeHtml(crowdsecRemainingTime(d.expires_at, d.duration));
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
      <span>Type</span><span>Value</span><span>Origin</span><span>Scenario</span><span>Remaining</span><span>First seen</span>
    </div>${rows}`;

  const hasMore = crowdsecAllDecisions.length > crowdsecCollapsedRows;
  toggleCrowdsecExpand('#crowdsecDecisionsExpand', hasMore);
  updateCrowdsecToggleLabel('decisions', crowdsecAllDecisions.length);
}

// toggleCrowdsecExpand shows/hides an expand bar.
function toggleCrowdsecExpand(selector, show) {
  const el = $(selector);
  if (el) el.classList.toggle('hidden', !show);
}

// updateCrowdsecToggleLabel keeps the button text in sync with state.
function updateCrowdsecToggleLabel(kind, total) {
  const bar = kind === 'decisions' ? $('#crowdsecDecisionsExpand') : $('#crowdsecAlertsExpand');
  if (!bar) return;
  const btn = bar.querySelector('button');
  if (!btn) return;
  const expanded = kind === 'decisions' ? crowdsecDecisionsExpanded : crowdsecAlertsExpanded;
  btn.textContent = expanded ? `Show fewer (top ${crowdsecCollapsedRows})` : `Show all ${total}`;
}

// renderCrowdsecAlerts draws the live activity feed. Each row shows the
// scenario, source IP, country flag-style code, and whether a ban followed.
function renderCrowdsecAlerts(alerts) {
  const container = $('#crowdsecAlerts');
  if (!container) return;
  crowdsecAllAlerts = alerts || [];

  if (!crowdsecAllAlerts.length) {
    container.innerHTML = '<div class="muted">No detections synced yet. This feed needs machine credentials: run <code>cscli machines add servicarr</code> on your CrowdSec host, then enter the generated password under <em>Machine Password</em> in Connection settings below. Decisions sync on the bouncer key alone.</div>';
    toggleCrowdsecExpand('#crowdsecAlertsExpand', false);
    return;
  }

  const visible = crowdsecAlertsExpanded
    ? crowdsecAllAlerts
    : crowdsecAllAlerts.slice(0, crowdsecCollapsedRows);

  const rows = visible.map(a => {
    const scenario = escapeHtml(shortScenario(a.scenario || 'unknown'));
    const ip = escapeHtml(a.source_value || '—');
    const countryName = (a.country || '??').toUpperCase();
    const country = escapeHtml(countryName);
    const when = escapeHtml(relativeTime(a.created_at));
    const count = a.events_count > 1 ? `<span class="crowdsec-alert-count" title="raw events">${a.events_count}×</span>` : '';
    const flag = `<span class="crowdsec-flag" title="${escapeHtml(country)}">${country}</span>`;
    const decision = a.has_decision
      ? '<span class="crowdsec-alert-outcome crowdsec-outcome-banned">banned</span>'
      : '<span class="crowdsec-alert-outcome crowdsec-outcome-scan">no ban</span>';
    const simulated = a.simulated ? '<span class="crowdsec-alert-simulated" title="simulated">⏻</span>' : '';
    const attrID = escapeHtml(a.alert_id || '');
    const attrTitle = escapeHtml(`${a.scenario || ''} from ${a.source_value || ''}${a.as_name ? ' — ' + a.as_name : ''}`);
    const ariaLabel = escapeHtml(`${a.scenario || 'Unknown scenario'}, ${a.source_value || 'unknown source'}, ${countryName}, ${a.has_decision ? 'banned' : 'no ban'}, ${relativeTime(a.created_at)}`);
    const mappable = a.latitude != null && a.longitude != null;
    const mapAttrs = mappable
      ? ` tabindex="0" role="button" aria-controls="crowdsecMap" aria-label="${ariaLabel}"`
      : '';
    return `<div class="crowdsec-alert-row${mappable ? ' is-mappable' : ''}" data-alert-id="${attrID}" title="${attrTitle}"${mapAttrs}>
      ${flag}
      <span class="crowdsec-alert-scenario">${scenario}</span>
      <span class="crowdsec-alert-ip">${ip}</span>
      ${count}${simulated}
      <span class="crowdsec-alert-outcome-wrap">${decision}</span>
      <span class="crowdsec-alert-when">${when}</span>
    </div>`;
  }).join('');

  container.innerHTML = rows;

  const hasMore = crowdsecAllAlerts.length > crowdsecCollapsedRows;
  toggleCrowdsecExpand('#crowdsecAlertsExpand', hasMore);
  updateCrowdsecToggleLabel('alerts', crowdsecAllAlerts.length);
}

// shortScenario trims the crowdsecurity/ namespace prefix for display.
function shortScenario(name) {
  return String(name || '').replace(/^crowdsecurity\//, '');
}

// relativeTime renders "3m ago"-style labels for the feed.
function relativeTime(rfc3339) {
  if (!rfc3339) return '';
  const t = new Date(rfc3339);
  if (isNaN(t.getTime())) return '';
  const secs = Math.max(0, Math.floor((Date.now() - t.getTime()) / 1000));
  if (secs < 60) return 'just now';
  if (secs < 3600) return Math.floor(secs / 60) + 'm ago';
  if (secs < 86400) return Math.floor(secs / 3600) + 'h ago';
  return Math.floor(secs / 86400) + 'd ago';
}

// LAPI's duration is remaining time at fetch time. The decision row is not
// rewritten when its stable expiry is unchanged, so derive the display from
// expires_at instead of freezing that fetched duration in the UI.
function crowdsecRemainingTime(expiresAt, fallback, nowMs = Date.now()) {
  const expiryMs = new Date(expiresAt || '').getTime();
  if (!Number.isFinite(expiryMs)) return fallback || '';
  let seconds = Math.max(0, Math.ceil((expiryMs - nowMs) / 1000));
  if (seconds === 0) return 'expired';
  const days = Math.floor(seconds / 86400);
  seconds %= 86400;
  const hours = Math.floor(seconds / 3600);
  seconds %= 3600;
  const minutes = Math.floor(seconds / 60);
  seconds %= 60;
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

// renderCrowdsecStats fills the overview cards and the country/scenario bars.
function renderCrowdsecStats(stats) {
  if (!stats) return;
  const active = $('#csStatActive');
  if (active) active.textContent = stats.active_decisions != null ? String(stats.active_decisions) : '—';
  const alerts = $('#csStatAlerts');
  if (alerts) alerts.textContent = stats.alerts_24h != null ? String(stats.alerts_24h) : '—';

  const topCountry = $('#csStatTopCountry');
  const topCountryLabel = $('#csStatTopCountryLabel');
  if (topCountry) {
    topCountry.textContent = stats.top_country ? String(stats.top_country).toUpperCase() : '—';
  }
  if (topCountryLabel) {
    topCountryLabel.textContent = stats.top_country
      ? `Top attack origin (${stats.top_country_count} detections)`
      : 'Top attack origin';
  }

  const topScenario = $('#csStatTopScenario');
  if (topScenario) {
    topScenario.textContent = stats.top_scenario ? shortScenario(stats.top_scenario) : '—';
    topScenario.title = stats.top_scenario || '';
  }

  renderCrowdsecBreakdown('#crowdsecCountries', stats.countries, c => String(c.country || '??').toUpperCase());
  renderCrowdsecBreakdown('#crowdsecScenarios', stats.scenarios, s => shortScenario(s.scenario));
}

// renderCrowdsecBreakdown draws horizontal volume bars for countries or
// scenarios, scaled to the largest count.
function renderCrowdsecBreakdown(selector, items, labelFn) {
  const container = $(selector);
  if (!container) return;
  if (!items || !items.length) {
    container.innerHTML = '<div class="muted">No data yet.</div>';
    return;
  }
  const max = Math.max(...items.map(i => i.count), 1);
  container.innerHTML = items.map(i => {
    const label = escapeHtml(labelFn(i));
    const pct = Math.max(4, Math.round((i.count / max) * 100));
    const attrTitle = escapeHtml(`${label}: ${i.count}`);
    return `<div class="crowdsec-breakdown-row" title="${attrTitle}">
      <span class="crowdsec-breakdown-label">${label}</span>
      <div class="crowdsec-breakdown-track"><div class="crowdsec-breakdown-bar" style="width:${pct}%"></div></div>
      <span class="crowdsec-breakdown-count">${escapeHtml(String(i.count))}</span>
    </div>`;
  }).join('');
}

var crowdsecDashboardPromise = null;
var crowdsecRefreshTimer = null;
var crowdsecRefreshIntervalMs = 15000;
var crowdsecTabObserver = null;
var crowdsecLiveRetrying = false;

// The dashboard only polls while the operator can actually see it. This keeps
// inactive admin tabs quiet and also gives map animations a stable pause point.
function crowdsecDashboardVisible() {
  const tab = $('#tab-crowdsec');
  return !!(tab && tab.classList.contains('active') && !document.hidden);
}

function setCrowdsecLiveState(active) {
  const indicator = $('#crowdsecLiveState');
  if (!indicator) return;
  const retrying = active && crowdsecLiveRetrying;
  indicator.classList.toggle('is-live', active && !retrying);
  indicator.classList.toggle('is-retrying', retrying);
  indicator.textContent = retrying ? 'Live view retrying' : (active ? 'Live view on' : 'Live view paused');
  indicator.setAttribute('aria-label', retrying
    ? 'Live view retrying. Cached data is shown while Servicarr retries the CrowdSec connection.'
    : (active
      ? 'Live view on. Dashboard refreshes every 15 seconds.'
      : 'Live view paused while this tab is not visible.'));
}

// Load each panel independently. A temporary failure in one endpoint must not
// blank otherwise healthy decisions, alerts, stats, or map data.
function loadCrowdsecDecisions() {
  if (!$('#crowdsecDecisions')) return Promise.resolve(null);
  if (crowdsecDashboardPromise) return crowdsecDashboardPromise;

  const requests = [
    j('/api/admin/crowdsec/decisions?active=true'),
    j('/api/admin/crowdsec/status'),
    j('/api/admin/crowdsec/alerts?limit=100'),
    j('/api/admin/crowdsec/stats')
  ];

  const pending = Promise.allSettled(requests).then(results => {
    const [decisionsResult, statusResult, alertsResult, statsResult] = results;
    const failures = [];

    if (decisionsResult.status === 'fulfilled') {
      const decisions = decisionsResult.value;
      renderCrowdsecDecisions(decisions && decisions.decisions ? decisions.decisions : []);
    } else {
      failures.push('decisions');
    }

    if (alertsResult.status === 'fulfilled') {
      const alerts = alertsResult.value;
      const items = alerts && alerts.alerts ? alerts.alerts : [];
      renderCrowdsecAlerts(items);
      if (typeof crowdsecMapApply === 'function') crowdsecMapApply(items);
    } else {
      failures.push('activity');
    }

    if (statsResult.status === 'fulfilled') {
      renderCrowdsecStats(statsResult.value);
    } else {
      failures.push('statistics');
    }

    const badge = $('#crowdsecSyncBadge');
    if (statusResult.status === 'fulfilled') {
      const status = statusResult.value;
      crowdsecLiveRetrying = !!(status && status.last_error);
      if (badge) {
        const badgeText = crowdsecSyncBadgeText(status);
        badge.textContent = failures.length ? `Partial data — ${badgeText}` : badgeText;
        badge.className = 'crowdsec-badge' + (status && status.last_error
          ? ' crowdsec-badge-error'
          : (failures.length ? ' crowdsec-badge-warning' : (status && status.last_sync ? '' : ' muted')));
      }
    } else {
      failures.push('sync status');
      crowdsecLiveRetrying = true;
      if (badge) {
        badge.textContent = 'Sync status unavailable';
        badge.className = 'crowdsec-badge crowdsec-badge-error';
      }
    }
    if (badge) {
      badge.title = failures.length ? `Could not refresh: ${failures.join(', ')}` : '';
    }
    setCrowdsecLiveState(crowdsecDashboardVisible());

    return results;
  }).finally(() => {
    if (crowdsecDashboardPromise === pending) crowdsecDashboardPromise = null;
  });

  crowdsecDashboardPromise = pending;
  return pending;
}

function stopCrowdsecAutoRefresh() {
  if (crowdsecRefreshTimer) clearInterval(crowdsecRefreshTimer);
  crowdsecRefreshTimer = null;
  setCrowdsecLiveState(false);
}

function updateCrowdsecAutoRefresh(refreshNow) {
  if (!crowdsecDashboardVisible()) {
    stopCrowdsecAutoRefresh();
    return;
  }
  setCrowdsecLiveState(true);
  if (!crowdsecRefreshTimer) {
    crowdsecRefreshTimer = setInterval(() => {
      if (crowdsecDashboardVisible()) loadCrowdsecDecisions();
    }, crowdsecRefreshIntervalMs);
  }
  if (refreshNow) loadCrowdsecDecisions();
  if (typeof crowdsecMapResize === 'function') crowdsecMapResize();
  if (typeof crowdsecMapResume === 'function') crowdsecMapResume();
}

function highlightCrowdsecAlertRow(row) {
  if (!row) return;
  const alert = crowdsecAllAlerts.find(item => String(item.alert_id || '') === row.dataset.alertId);
  if (!alert || alert.latitude == null || alert.longitude == null) {
    clearCrowdsecAlertHighlight();
    return;
  }
  if (alert && typeof crowdsecMapHighlightAlert === 'function') crowdsecMapHighlightAlert(alert);
  $$('.crowdsec-alert-row').forEach(item => item.classList.toggle('is-selected', item === row));
}

function clearCrowdsecAlertHighlight() {
  $$('.crowdsec-alert-row').forEach(item => item.classList.remove('is-selected'));
  if (typeof crowdsecMapHighlightAlert === 'function') crowdsecMapHighlightAlert(null);
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
      // If an automatic refresh was already in flight, let it finish before
      // starting the post-sync read so the button always reveals fresh data.
      if (crowdsecDashboardPromise) await crowdsecDashboardPromise;
      await loadCrowdsecDecisions();
      setCrowdsecStatus('Sync completed', 'success', 3000);
    },
    'Syncing',
    (err) => setCrowdsecStatus(crowdsecErrorMessage(err, 'Sync failed'), 'error')
  );
}

// toggleCrowdsecSection expands/collapses a list or the settings panel.
function toggleCrowdsecSection(kind) {
  if (kind === 'alerts') {
    crowdsecAlertsExpanded = !crowdsecAlertsExpanded;
    renderCrowdsecAlerts(crowdsecAllAlerts);
  } else if (kind === 'decisions') {
    crowdsecDecisionsExpanded = !crowdsecDecisionsExpanded;
    renderCrowdsecDecisions(crowdsecAllDecisions);
  } else if (kind === 'settings') {
    const body = $('#crowdsecSettingsBody');
    const header = document.querySelector('.crowdsec-collapsible-header[data-crowdsec-toggle="settings"]');
    if (!body) return;
    const collapsed = body.classList.toggle('crowdsec-collapsed');
    if (header) {
      header.setAttribute('aria-expanded', String(!collapsed));
      const chevron = header.querySelector('.crowdsec-chevron');
      if (chevron) chevron.textContent = collapsed ? '▸' : '▾';
    }
  }
}

function initCrowdsecTab() {
  const form = $('#crowdsecForm');
  if (!form || form.dataset.crowdsecInitialized === 'true') return;
  form.dataset.crowdsecInitialized = 'true';
  loadCrowdsecConfig();
  if (typeof crowdsecMapInit === 'function') crowdsecMapInit();
  const mapCanvas = $('#crowdsecMap');
  if (mapCanvas && typeof crowdsecMapOnMouseMove === 'function') {
    mapCanvas.addEventListener('mousemove', crowdsecMapOnMouseMove);
    if (typeof crowdsecMapOnMouseLeave === 'function') {
      mapCanvas.addEventListener('mouseleave', crowdsecMapOnMouseLeave);
      mapCanvas.addEventListener('blur', crowdsecMapOnMouseLeave);
    }
    if (typeof crowdsecMapOnKeyDown === 'function') mapCanvas.addEventListener('keydown', crowdsecMapOnKeyDown);
  }
  const save = $('#saveCrowdsec');
  if (save) save.addEventListener('click', saveCrowdsecConfig);
  const test = $('#testCrowdsec');
  if (test) test.addEventListener('click', testCrowdsecConnection);
  const syncNow = $('#crowdsecSyncNow');
  if (syncNow) syncNow.addEventListener('click', crowdsecSyncNow);

  const alerts = $('#crowdsecAlerts');
  if (alerts) {
    alerts.addEventListener('mouseover', e => highlightCrowdsecAlertRow(e.target.closest('.crowdsec-alert-row')));
    alerts.addEventListener('focusin', e => highlightCrowdsecAlertRow(e.target.closest('.crowdsec-alert-row')));
    alerts.addEventListener('click', e => highlightCrowdsecAlertRow(e.target.closest('.crowdsec-alert-row')));
    alerts.addEventListener('keydown', e => {
      const row = e.target.closest('.crowdsec-alert-row');
      if (!row || (e.key !== 'Enter' && e.key !== ' ')) return;
      e.preventDefault();
      highlightCrowdsecAlertRow(row);
    });
    alerts.addEventListener('mouseleave', clearCrowdsecAlertHighlight);
    alerts.addEventListener('focusout', e => {
      if (!alerts.contains(e.relatedTarget)) clearCrowdsecAlertHighlight();
    });
  }

  const crowdsecTab = $('#tab-crowdsec');
  if (crowdsecTab && typeof MutationObserver === 'function') {
    crowdsecTabObserver = new MutationObserver(() => updateCrowdsecAutoRefresh(true));
    crowdsecTabObserver.observe(crowdsecTab, { attributes: true, attributeFilter: ['class'] });
  }
  document.addEventListener('visibilitychange', () => updateCrowdsecAutoRefresh(true));
  updateCrowdsecAutoRefresh(true);

  // Delegated toggles for expand/collapse controls.
  document.addEventListener('click', (e) => {
    const toggle = e.target.closest('[data-crowdsec-toggle]');
    if (!toggle) return;
    toggleCrowdsecSection(toggle.dataset.crowdsecToggle);
  });
}
