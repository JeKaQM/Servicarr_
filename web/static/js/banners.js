/* Banner Functions */
let bannersLoading = false;
let editingStatusBanner = null;

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
  if (banner?.kind === 'ups_line_loss') return 'Automatic UPS warning';
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

  // Replace the immediate client-side fallback once the server-managed UPS
  // occurrence is available, so administrators can edit or hide it.
  if (globalBanners.some(b => b.kind === 'ups_line_loss')) {
    const localUPSAlert = container.querySelector('[data-auto-alert="ups-line"]');
    if (localUPSAlert) localUPSAlert.remove();
  }

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
  const managed = container.querySelector('[data-automatic-kind="ups_line_loss"]');
  if (!ups || typeof ups.power_present !== 'boolean') {
    // An unavailable reading is not proof that mains power recovered.
    return;
  }

  if (ups.power_present) {
    if (existing) existing.remove();
    if (managed) managed.remove();
    return;
  }

  // The server-managed alert is authoritative. Creating a separate local
  // banner here would bypass an administrator's edit or hide decision.
  if (existing) existing.remove();
}

function clearUPSLineAlert() {
  const container = $('#siteAlerts');
  if (!container) return;
  const existing = container.querySelector('[data-auto-alert="ups-line"]');
  if (existing) existing.remove();
  const managed = container.querySelector('[data-automatic-kind="ups_line_loss"]');
  if (managed) managed.remove();
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
      list.innerHTML = '<div class="muted">No manual or active automated banners</div>';
      return;
    }

    list.innerHTML = '';
    banners.forEach(b => {
      const level = normalizeAlertLevel(b.level);
      const message = escapeHtml(b.message || '');
      const div = document.createElement('div');
      const source = b.source || (b.automatic ? 'automatic' : b.scheduled ? 'scheduled' : 'manual');
      div.className = `banner-item${b.hidden ? ' is-hidden' : ''}`;
      const scopeLabel = escapeHtml(b.service_key ? b.service_key.charAt(0).toUpperCase() + b.service_key.slice(1) : 'Global');
      const sourceLabel = source === 'automatic' ? 'Automated' : source === 'scheduled' ? 'Scheduled' : 'Manual';
      const stateLabel = b.hidden ? 'Hidden from visitors' : 'Visible';
      div.innerHTML = `
        <span class="banner-item-level ${level}">${level.toUpperCase()}</span>
        <div class="banner-item-content">
          <span class="banner-item-msg">${message}</span>
          <span class="banner-item-service">${scopeLabel} | ${sourceLabel} | ${stateLabel}</span>
        </div>
        <div class="banner-actions">
          <button type="button" class="btn mini ghost banner-edit">Edit</button>
          ${b.hidden
            ? '<button type="button" class="btn mini banner-restore">Restore</button>'
            : `<button type="button" class="btn mini danger banner-delete">${source === 'manual' ? 'Delete' : 'Hide'}</button>`}
        </div>
      `;
      div.querySelector('.banner-edit').addEventListener('click', () => editBanner(b));
      const deleteButton = div.querySelector('.banner-delete');
      if (deleteButton) deleteButton.addEventListener('click', () => deleteBanner(b));
      const restoreButton = div.querySelector('.banner-restore');
      if (restoreButton) restoreButton.addEventListener('click', () => restoreBanner(b));
      list.appendChild(div);
    });
  } catch (e) {
    console.error('Failed to load admin banners', e);
  }
}

const maintenanceWeekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
let editingMaintenanceScheduleID = '';
let originalMaintenanceSchedule = null;

function updateMaintenanceScheduleForm() {
  const type = $('#maintenanceScheduleType')?.value || 'once';
  const once = type === 'once';
  $$('.maintenance-once-field').forEach(field => {
    field.classList.toggle('hidden', !once);
    $$('input', field).forEach(input => { input.disabled = !once; });
  });
  $$('.maintenance-recurring-field').forEach(field => {
    field.classList.toggle('hidden', once);
    $$('input, select', field).forEach(input => { input.disabled = once; });
  });
  const weekdays = $('#maintenanceWeekdaysField');
  if (weekdays) {
    weekdays.classList.toggle('hidden', type !== 'weekly');
    weekdays.disabled = type !== 'weekly';
  }
  const start = $('#maintenanceStartsAt');
  const end = $('#maintenanceEndsAt');
  if (start) start.required = once;
  if (end) {
    end.disabled = !once || Boolean($('#maintenanceNoEnd')?.checked);
    end.required = !end.disabled;
  }
  const time = $('#maintenanceStartTime');
  const duration = $('#maintenanceDuration');
  if (time) time.required = !once;
  if (duration) duration.required = !once;
}

// Format the stored instant in the schedule's timezone, never the browser's timezone.
function maintenanceLocalDate(value, timezone) {
  if (!value) return '';
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone: timezone || 'UTC', year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', hourCycle: 'h23'
    }).formatToParts(new Date(value));
    const fields = Object.fromEntries(parts.map(part => [part.type, part.value]));
    return `${fields.year.padStart(4, '0')}-${fields.month}-${fields.day}T${fields.hour}:${fields.minute}`;
  } catch (_) {
    return '';
  }
}

function maintenanceDurationParts(minutes) {
  const value = Number(minutes) || 30;
  const unit = [10080, 1440, 60, 1].find(candidate => value % candidate === 0);
  return { value: value / unit, unit };
}

function maintenanceTimingSummary(schedule) {
  if (schedule.schedule_type === 'once') {
    const start = maintenanceLocalDate(schedule.starts_at, schedule.timezone).replace('T', ' ');
    const end = maintenanceLocalDate(schedule.ends_at, schedule.timezone).replace('T', ' ');
    return `${start || 'Unknown start'} to ${end || 'no scheduled end'}`;
  }
  const days = schedule.schedule_type === 'daily' ? 'Every day'
    : (schedule.weekdays?.length ? schedule.weekdays : [schedule.weekday])
      .map(day => maintenanceWeekdays[Number(day)] || 'Unknown day').join(', ');
  const { value, unit } = maintenanceDurationParts(schedule.duration_minutes);
  const unitName = { 1: 'min', 60: 'hour', 1440: 'day', 10080: 'week' }[unit];
  return `${days} at ${schedule.start_time || ''} for ${value} ${unitName}${unit !== 1 && value !== 1 ? 's' : ''}`;
}

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
    list.innerHTML = '<div class="muted">No maintenance schedules</div>';
    return;
  }

  list.innerHTML = '';
  schedules.forEach(schedule => {
    const row = document.createElement('div');
    row.className = `maintenance-schedule-row${schedule.enabled ? '' : ' is-disabled'}`;
    const name = escapeHtml(schedule.name || 'Scheduled maintenance');
    const message = escapeHtml(schedule.message || '');
    const timezone = escapeHtml(schedule.timezone || 'UTC');
    const level = normalizeAlertLevel(schedule.level);
    const monitorText = schedule.suppress_monitoring ? 'Monitoring paused' : 'Banner only';
    const completed = schedule.schedule_type === 'once' && schedule.ends_at && new Date(schedule.ends_at) <= new Date();
    const enabledText = !schedule.enabled ? 'Disabled' : completed ? 'Completed' : 'Enabled';

    row.innerHTML = `
      <span class="banner-item-level ${level}">${level.toUpperCase()}</span>
      <div class="maintenance-schedule-content">
        <div class="maintenance-schedule-title">
          <span>${name}</span>
          <span class="maintenance-schedule-state">${enabledText}</span>
        </div>
        <span class="maintenance-schedule-message">${message}</span>
        <span class="maintenance-schedule-meta">${escapeHtml(maintenanceTimingSummary(schedule))} | ${timezone} | ${monitorText}</span>
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
  originalMaintenanceSchedule = schedule;
  $('#maintenanceName').value = schedule.name || '';
  $('#maintenanceMessage').value = schedule.message || '';
  $('#maintenanceScheduleType').value = schedule.schedule_type || 'weekly';
  const weekdays = schedule.weekdays?.length ? schedule.weekdays : [schedule.weekday ?? 1];
  $$('[name="maintenanceWeekdays"]').forEach(input => { input.checked = weekdays.includes(Number(input.value)); });
  $('#maintenanceStartsAt').value = maintenanceLocalDate(schedule.starts_at, schedule.timezone);
  $('#maintenanceEndsAt').value = maintenanceLocalDate(schedule.ends_at, schedule.timezone);
  $('#maintenanceNoEnd').checked = schedule.schedule_type === 'once' && !schedule.ends_at;
  $('#maintenanceStartTime').value = schedule.start_time || '02:55';
  const duration = maintenanceDurationParts(schedule.duration_minutes);
  $('#maintenanceDuration').value = String(duration.value);
  $('#maintenanceDurationUnit').value = String(duration.unit);
  $('#maintenanceTimezone').value = schedule.timezone || 'Europe/London';
  $('#maintenanceLevel').value = normalizeAlertLevel(schedule.level);
  $('#maintenanceEnabled').checked = Boolean(schedule.enabled);
  $('#maintenanceSuppressMonitoring').checked = Boolean(schedule.suppress_monitoring);
  $('#saveMaintenanceSchedule').textContent = 'Update Schedule';
  $('#cancelMaintenanceSchedule').classList.remove('hidden');
  updateMaintenanceScheduleForm();
  $('#maintenanceName').focus();
}

function resetMaintenanceScheduleForm() {
  editingMaintenanceScheduleID = '';
  originalMaintenanceSchedule = null;
  const form = $('#maintenanceScheduleForm');
  if (form) form.reset();
  $('#maintenanceScheduleType').value = 'once';
  $('#maintenanceStartsAt').value = '';
  $('#maintenanceEndsAt').value = '';
  $('#maintenanceNoEnd').checked = false;
  $$('[name="maintenanceWeekdays"]').forEach(input => { input.checked = input.value === '1'; });
  $('#maintenanceStartTime').value = '02:55';
  $('#maintenanceDuration').value = '30';
  $('#maintenanceDurationUnit').value = '1';
  $('#maintenanceTimezone').value = 'Europe/London';
  $('#maintenanceLevel').value = 'warning';
  $('#maintenanceEnabled').checked = true;
  $('#maintenanceSuppressMonitoring').checked = true;
  $('#saveMaintenanceSchedule').textContent = 'Create Schedule';
  $('#cancelMaintenanceSchedule').classList.add('hidden');
  updateMaintenanceScheduleForm();
}

async function saveMaintenanceSchedule(event) {
  if (event) event.preventDefault();
  const payload = {
    id: editingMaintenanceScheduleID,
    name: $('#maintenanceName').value.trim(),
    message: $('#maintenanceMessage').value.trim(),
    schedule_type: $('#maintenanceScheduleType').value,
    timezone: $('#maintenanceTimezone').value.trim(),
    level: $('#maintenanceLevel').value,
    enabled: $('#maintenanceEnabled').checked,
    suppress_monitoring: $('#maintenanceSuppressMonitoring').checked
  };

  const saveButton = $('#saveMaintenanceSchedule');
  if (saveButton.disabled) return;

  try {
    if (!payload.name || !payload.message || !payload.timezone) throw new Error('Complete all schedule fields');
    if (payload.schedule_type === 'once') {
      payload.starts_at = $('#maintenanceStartsAt').value;
      payload.ends_at = $('#maintenanceNoEnd').checked ? '' : $('#maintenanceEndsAt').value;
      if (!payload.starts_at || (!$('#maintenanceNoEnd').checked && !payload.ends_at)) {
        throw new Error('Choose a start and end date, or select no scheduled end');
      }
      // Preserve an API-supplied offset/seconds if its displayed wall time was not edited.
      // This also preserves the second occurrence of a repeated DST time.
      if (originalMaintenanceSchedule?.timezone === payload.timezone) {
        ['starts_at', 'ends_at'].forEach(key => {
          if (payload[key] && payload[key] === maintenanceLocalDate(originalMaintenanceSchedule[key], payload.timezone)) {
            payload[key] = originalMaintenanceSchedule[key];
          }
        });
      }
    } else {
      payload.start_time = $('#maintenanceStartTime').value;
      const duration = Number($('#maintenanceDuration').value) * Number($('#maintenanceDurationUnit').value);
      if (!Number.isFinite(duration) || duration < 1 || duration > 153722867 || Math.abs(duration - Math.round(duration)) > 0.000001) {
        throw new Error('Enter a duration of at least one whole minute (up to approximately 292 years)');
      }
      payload.duration_minutes = Math.round(duration);
      if (!payload.start_time) throw new Error('Choose a start time');
      if (payload.schedule_type === 'weekly') {
        payload.weekdays = $$('[name="maintenanceWeekdays"]:checked').map(input => Number(input.value));
        if (payload.weekdays.length === 0) throw new Error('Choose at least one weekday');
        payload.weekday = payload.weekdays[0];
      }
    }
    saveButton.disabled = true;
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
    showToast(typeof e.body === 'string' && e.body.trim() ? e.body.trim() : e.message || 'Failed to save schedule', 'error');
  } finally {
    saveButton.disabled = false;
  }
}

async function deleteMaintenanceSchedule(id) {
  if (!confirm('Delete this maintenance schedule?')) return;
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

function editBanner(banner) {
  editingStatusBanner = banner;
  const source = banner.source || (banner.automatic ? 'automatic' : banner.scheduled ? 'scheduled' : 'manual');
  const serviceEl = $('#bannerService');
  $('#bannerMessage').value = banner.message || '';
  $('#bannerLevel').value = normalizeAlertLevel(banner.level);
  if (serviceEl) {
    serviceEl.value = banner.service_key || '';
    serviceEl.disabled = source !== 'manual';
  }
  const templateEl = $('#bannerTemplate');
  if (templateEl) templateEl.disabled = true;
  const title = $('#bannerFormTitle');
  if (title) title.textContent = source === 'manual' ? 'Edit Manual Banner' : 'Adjust Generated Banner';
  $('#createBanner').textContent = 'Save Changes';
  $('#cancelBannerEdit').classList.remove('hidden');
  $('#bannerMessage').focus();
}

function resetBannerForm() {
  editingStatusBanner = null;
  const serviceEl = $('#bannerService');
  if (serviceEl) {
    serviceEl.value = '';
    serviceEl.disabled = false;
  }
  const templateEl = $('#bannerTemplate');
  if (templateEl) {
    templateEl.value = '';
    templateEl.disabled = false;
  }
  $('#bannerMessage').value = '';
  $('#bannerLevel').value = 'info';
  const title = $('#bannerFormTitle');
  if (title) title.textContent = 'Manual Banner';
  $('#createBanner').textContent = 'Create Banner';
  $('#cancelBannerEdit').classList.add('hidden');
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
    const editing = editingStatusBanner;
    const payload = editing
      ? {
          id: editing.id,
          occurrence_at: editing.created_at,
          message,
          level,
          service_key,
          hidden: Boolean(editing.hidden)
        }
      : { message, level, service_key };
    await j('/api/admin/status-alerts', {
      method: editing ? 'PUT' : 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify(payload)
    });

    resetBannerForm();
    showToast(editing ? 'Banner updated' : 'Banner created');
    await Promise.all([loadBanners(), loadAdminBanners()]);
  } catch (e) {
    console.error('Failed to create banner', e);
    showToast('Failed to create banner', 'error');
  }
}

async function deleteBanner(banner) {
  const source = banner.source || (banner.automatic ? 'automatic' : banner.scheduled ? 'scheduled' : 'manual');
  const generated = source !== 'manual';
  if (!confirm(generated ? 'Hide this generated banner for its current occurrence?' : 'Delete this banner?')) return;

  try {
    const params = new URLSearchParams({ id: banner.id });
    if (banner.created_at) params.set('occurrence_at', banner.created_at);
    await j(`/api/admin/status-alerts?${params.toString()}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast(generated ? 'Generated banner hidden' : 'Banner deleted');
    if (editingStatusBanner?.id === banner.id) resetBannerForm();
    await Promise.all([loadBanners(), loadAdminBanners()]);
  } catch (e) {
    console.error('Failed to delete banner', e);
    showToast('Failed to delete banner', 'error');
  }
}

async function restoreBanner(banner) {
  try {
    await j('/api/admin/status-alerts', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ id: banner.id, occurrence_at: banner.created_at, hidden: false })
    });
    showToast('Generated banner restored');
    await Promise.all([loadBanners(), loadAdminBanners()]);
  } catch (e) {
    console.error('Failed to restore banner', e);
    showToast('Failed to restore banner', 'error');
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
