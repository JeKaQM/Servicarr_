/* Banner Functions */
let bannersLoading = false;

async function loadBanners() {
  if (bannersLoading) return;
  bannersLoading = true;
  try {
    const banners = await j('/api/status-alerts');
    renderSiteBanners(banners);
    renderServiceBanners(banners);
  } catch (e) {
    console.error('Failed to load banners', e);
  } finally {
    bannersLoading = false;
  }
}

function getAlertIcon(level) {
  const icons = {
    info: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10 9v4m0-6.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    warning: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><path d="M10 2L1 18h18L10 2z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M10 8v4m0 2v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    error: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M7 7l6 6m0-6l-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
  };
  return icons[level] || icons.info;
}

function getServiceAlertIcon(level) {
  const icons = {
    info: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10 9v4m0-6.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    warning: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><path d="M10 2L1 18h18L10 2z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M10 8v4m0 2v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    error: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M7 7l6 6m0-6l-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
  };
  return icons[level] || icons.info;
}

function formatBannerTime(isoString) {
  if (!isoString) return '';
  const date = new Date(isoString);
  const now = new Date();
  const diffMs = now - date;
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function formatScheduledBannerTime(endsAt) {
  if (!endsAt) return 'Scheduled maintenance';
  const end = new Date(endsAt);
  if (Number.isNaN(end.getTime())) return 'Scheduled maintenance';
  return `Ends ${end.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
}

function formatAutomaticBannerTime(banner) {
  if (banner?.kind === 'critical_outage') return 'Automatic outage alert';
  if (banner?.kind === 'services_restored' && banner.ends_at) {
    const end = new Date(banner.ends_at);
    if (!Number.isNaN(end.getTime())) {
      return `Monitoring until ${end.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
    }
  }
  return 'Automatic status update';
}

function normalizeAlertLevel(level) {
  const allowed = ['info', 'warning', 'error'];
  return allowed.includes(level) ? level : 'info';
}

function renderSiteBanners(banners) {
  const container = $('#siteAlerts');
  if (!container) return;

  // Manual banners refresh independently from automatic system alerts.
  Array.from(container.children).forEach(child => {
    if (!child.hasAttribute('data-auto-alert')) child.remove();
  });

  // Only show global banners (no service_key) at the top
  const globalBanners = banners.filter(b => !b.service_key);

  globalBanners.forEach(b => {
    const level = normalizeAlertLevel(b.level);
    const message = escapeHtml(b.message || '');
    const div = document.createElement('div');
    div.className = `site-alert ${level}`;
    div.dataset.id = b.id;
    const timeStr = b.scheduled
      ? formatScheduledBannerTime(b.ends_at)
      : b.automatic
        ? formatAutomaticBannerTime(b)
        : formatBannerTime(b.created_at);
    if (b.scheduled) div.dataset.scheduled = 'true';
    if (b.automatic) {
      div.classList.add('site-alert-automatic');
      div.dataset.automaticKind = b.kind || 'status_update';
      div.setAttribute('role', level === 'error' ? 'alert' : 'status');
      div.setAttribute('aria-live', level === 'error' ? 'assertive' : 'polite');
    }
    div.innerHTML = `
      ${getAlertIcon(level)}
      <div class="site-alert-content">
        <span class="site-alert-message">${message}</span>
        <span class="site-alert-time">${timeStr}</span>
      </div>
    `;
    container.appendChild(div);
  });
}

function updateUPSLineAlert(ups) {
  const container = $('#siteAlerts');
  if (!container) return;

  const existing = container.querySelector('[data-auto-alert="ups-line"]');
  if (!ups || typeof ups.power_present !== 'boolean') {
    // An unavailable reading is not proof that mains power recovered.
    return;
  }

  if (ups.power_present) {
    if (existing) existing.remove();
    return;
  }

  if (existing) return;

  const div = document.createElement('div');
  div.className = 'site-alert warning site-alert-automatic';
  div.dataset.autoAlert = 'ups-line';
  div.setAttribute('role', 'alert');
  div.setAttribute('aria-live', 'assertive');
  div.innerHTML = `
    ${getAlertIcon('warning')}
    <div class="site-alert-content">
      <span class="site-alert-message">Mains power lost. The monitored system is running on UPS battery.</span>
      <span class="site-alert-time">Automatic UPS warning</span>
    </div>
  `;
  container.prepend(div);
}

function clearUPSLineAlert() {
  const container = $('#siteAlerts');
  if (!container) return;
  const existing = container.querySelector('[data-auto-alert="ups-line"]');
  if (existing) existing.remove();
}

function renderServiceBanners(banners) {
  // Clear existing service alerts
  document.querySelectorAll('.service-alert').forEach(el => el.remove());

  // Filter to only service-specific banners
  const serviceBanners = banners.filter(b => b.service_key);

  serviceBanners.forEach(b => {
    const level = normalizeAlertLevel(b.level);
    const message = escapeHtml(b.message || '');
    const card = $(`#card-${b.service_key}`);
    if (!card) return;

    // Check if banner already exists
    const existing = card.querySelector(`.service-alert[data-id="${b.id}"]`);
    if (existing) return;

    const alertDiv = document.createElement('div');
    alertDiv.className = `service-alert ${level}`;
    alertDiv.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
    alertDiv.innerHTML = `
      ${getServiceAlertIcon(level)}
      <div class="service-alert-content">
        <span>${message}</span>
        <span class="service-alert-time">${timeStr}</span>
      </div>
    `;

    // Insert before adminRow if present, otherwise at end
    const adminRow = card.querySelector('.adminRow');
    if (adminRow) {
      card.insertBefore(alertDiv, adminRow);
    } else {
      card.appendChild(alertDiv);
    }
  });
}

async function loadAdminBanners() {
  try {
    const banners = await j('/api/admin/status-alerts', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    const list = $('#bannersList');
    if (!list) return;

    if (banners.length === 0) {
      list.innerHTML = '<div class="muted">No active banners</div>';
      return;
    }

    list.innerHTML = '';
    banners.forEach(b => {
      const level = normalizeAlertLevel(b.level);
      const message = escapeHtml(b.message || '');
      const div = document.createElement('div');
      div.className = 'banner-item';
      const scopeLabel = escapeHtml(b.service_key ? b.service_key.charAt(0).toUpperCase() + b.service_key.slice(1) : 'Global');
      div.innerHTML = `
        <span class="banner-item-level ${level}">${level.toUpperCase()}</span>
        <div class="banner-item-content">
          <span class="banner-item-msg">${message}</span>
          <span class="banner-item-service">${scopeLabel}</span>
        </div>
        <button class="banner-delete">Delete</button>
      `;
      div.querySelector('.banner-delete').addEventListener('click', () => deleteBanner(b.id));
      list.appendChild(div);
    });
  } catch (e) {
    console.error('Failed to load admin banners', e);
  }
}

const maintenanceWeekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
let editingMaintenanceScheduleID = '';

async function loadMaintenanceSchedules() {
  const list = $('#maintenanceSchedulesList');
  if (!list) return;

  try {
    const schedules = await j('/api/admin/maintenance-schedules', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    renderMaintenanceSchedules(schedules);
  } catch (e) {
    console.error('Failed to load maintenance schedules', e);
    list.innerHTML = '<div class="muted">Unable to load schedules</div>';
  }
}

function renderMaintenanceSchedules(schedules) {
  const list = $('#maintenanceSchedulesList');
  if (!list) return;
  if (!Array.isArray(schedules) || schedules.length === 0) {
    list.innerHTML = '<div class="muted">No recurring schedules</div>';
    return;
  }

  list.innerHTML = '';
  schedules.forEach(schedule => {
    const row = document.createElement('div');
    row.className = `maintenance-schedule-row${schedule.enabled ? '' : ' is-disabled'}`;
    const day = maintenanceWeekdays[Number(schedule.weekday)] || 'Unknown day';
    const name = escapeHtml(schedule.name || 'Scheduled maintenance');
    const message = escapeHtml(schedule.message || '');
    const timezone = escapeHtml(schedule.timezone || 'UTC');
    const level = normalizeAlertLevel(schedule.level);
    const monitorText = schedule.suppress_monitoring ? 'Monitoring paused' : 'Banner only';
    const enabledText = schedule.enabled ? 'Enabled' : 'Disabled';

    row.innerHTML = `
      <span class="banner-item-level ${level}">${level.toUpperCase()}</span>
      <div class="maintenance-schedule-content">
        <div class="maintenance-schedule-title">
          <span>${name}</span>
          <span class="maintenance-schedule-state">${enabledText}</span>
        </div>
        <span class="maintenance-schedule-message">${message}</span>
        <span class="maintenance-schedule-meta">${day} at ${escapeHtml(schedule.start_time || '')} for ${Number(schedule.duration_minutes) || 0} min | ${timezone} | ${monitorText}</span>
      </div>
      <div class="maintenance-schedule-actions">
        <button type="button" class="btn mini ghost maintenance-edit">Edit</button>
        <button type="button" class="btn mini danger maintenance-delete">Delete</button>
      </div>
    `;
    row.querySelector('.maintenance-edit').addEventListener('click', () => editMaintenanceSchedule(schedule));
    row.querySelector('.maintenance-delete').addEventListener('click', () => deleteMaintenanceSchedule(schedule.id));
    list.appendChild(row);
  });
}

function editMaintenanceSchedule(schedule) {
  editingMaintenanceScheduleID = schedule.id || '';
  $('#maintenanceName').value = schedule.name || '';
  $('#maintenanceMessage').value = schedule.message || '';
  $('#maintenanceWeekday').value = String(schedule.weekday ?? 1);
  $('#maintenanceStartTime').value = schedule.start_time || '02:55';
  $('#maintenanceDuration').value = String(schedule.duration_minutes || 30);
  $('#maintenanceTimezone').value = schedule.timezone || 'Europe/London';
  $('#maintenanceLevel').value = normalizeAlertLevel(schedule.level);
  $('#maintenanceEnabled').checked = Boolean(schedule.enabled);
  $('#maintenanceSuppressMonitoring').checked = Boolean(schedule.suppress_monitoring);
  $('#saveMaintenanceSchedule').textContent = 'Update Schedule';
  $('#cancelMaintenanceSchedule').classList.remove('hidden');
  $('#maintenanceName').focus();
}

function resetMaintenanceScheduleForm() {
  editingMaintenanceScheduleID = '';
  const form = $('#maintenanceScheduleForm');
  if (form) form.reset();
  $('#maintenanceWeekday').value = '1';
  $('#maintenanceStartTime').value = '02:55';
  $('#maintenanceDuration').value = '30';
  $('#maintenanceTimezone').value = 'Europe/London';
  $('#maintenanceLevel').value = 'warning';
  $('#maintenanceEnabled').checked = true;
  $('#maintenanceSuppressMonitoring').checked = true;
  $('#saveMaintenanceSchedule').textContent = 'Create Schedule';
  $('#cancelMaintenanceSchedule').classList.add('hidden');
}

async function saveMaintenanceSchedule(event) {
  if (event) event.preventDefault();
  const payload = {
    id: editingMaintenanceScheduleID,
    name: $('#maintenanceName').value.trim(),
    message: $('#maintenanceMessage').value.trim(),
    weekday: Number($('#maintenanceWeekday').value),
    start_time: $('#maintenanceStartTime').value,
    duration_minutes: Number($('#maintenanceDuration').value),
    timezone: $('#maintenanceTimezone').value.trim(),
    level: $('#maintenanceLevel').value,
    enabled: $('#maintenanceEnabled').checked,
    suppress_monitoring: $('#maintenanceSuppressMonitoring').checked
  };

  if (!payload.name || !payload.message || !payload.start_time || !payload.timezone || payload.duration_minutes < 1) {
    showToast('Complete all schedule fields', 'error');
    return;
  }

  try {
    await j('/api/admin/maintenance-schedules', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify(payload)
    });
    showToast(editingMaintenanceScheduleID ? 'Schedule updated' : 'Schedule created');
    resetMaintenanceScheduleForm();
    await Promise.all([loadMaintenanceSchedules(), loadBanners()]);
  } catch (e) {
    console.error('Failed to save maintenance schedule', e);
    showToast('Failed to save schedule', 'error');
  }
}

async function deleteMaintenanceSchedule(id) {
  if (!confirm('Delete this recurring schedule?')) return;
  try {
    await j(`/api/admin/maintenance-schedules?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    if (editingMaintenanceScheduleID === id) resetMaintenanceScheduleForm();
    showToast('Schedule deleted');
    await Promise.all([loadMaintenanceSchedules(), loadBanners()]);
  } catch (e) {
    console.error('Failed to delete maintenance schedule', e);
    showToast('Failed to delete schedule', 'error');
  }
}

async function createBanner() {
  const msgEl = $('#bannerMessage');
  const levelEl = $('#bannerLevel');
  const serviceEl = $('#bannerService');
  if (!msgEl || !levelEl) return;

  const message = msgEl.value.trim();
  const level = levelEl.value;
  const service_key = serviceEl ? serviceEl.value : '';

  if (!message) {
    alert('Please enter a message');
    return;
  }

  try {
    await j('/api/admin/status-alerts', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ message, level, service_key })
    });

    msgEl.value = '';
    showToast('Banner created');
    loadBanners();
    loadAdminBanners();
  } catch (e) {
    console.error('Failed to create banner', e);
    showToast('Failed to create banner', 'error');
  }
}

async function deleteBanner(id) {
  if (!confirm('Delete this banner?')) return;

  try {
    await j(`/api/admin/status-alerts?id=${id}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('Banner deleted');
    loadBanners();
    loadAdminBanners();
  } catch (e) {
    console.error('Failed to delete banner', e);
    showToast('Failed to delete banner', 'error');
  }
}

function populateBannerScopeDropdown() {
  const select = $('#bannerService');
  if (!select) return;

  // Keep the global option, remove service options
  const globalOption = select.querySelector('option[value=""]');
  select.innerHTML = '';
  if (globalOption) {
    select.appendChild(globalOption);
  } else {
    const opt = document.createElement('option');
    opt.value = '';
    opt.textContent = 'Global (top of page)';
    select.appendChild(opt);
  }

  // Add all services from servicesData
  if (servicesData && servicesData.length > 0) {
    const optgroup = document.createElement('optgroup');
    optgroup.label = 'Services';

    servicesData.forEach(svc => {
      const opt = document.createElement('option');
      opt.value = svc.key;
      opt.textContent = svc.name;
      optgroup.appendChild(opt);
    });

    select.appendChild(optgroup);
  }
}

/* ========================================
   Dynamic Services Management
   ======================================== */
