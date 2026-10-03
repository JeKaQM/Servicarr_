/* ============ Banners tab ============
   Compose banners and maintenance windows, and manage both lists. Visitor
   rendering lives in banners.js; this file is in the admin bundle. */

let bannerEditing = null;        // banner being edited; null composes a new one
let bannerSaving = false;
let maintenanceEditing = null;   // window being edited; null composes a new one
let maintenanceSaving = false;
let latestAdminBanners = [];
let latestMaintenanceWindows = [];
let bannersRefreshTimer = null;

const BANNER_TEMPLATES = {
  investigating: { level: 'warning', message: 'We are aware of a problem and are investigating.' },
  slow: { level: 'warning', message: 'Some services are running slowly. We are working on a fix.' },
  down: { level: 'error', message: 'A service is down. We are working to restore it.' },
  maintenance: { level: 'info', message: 'Scheduled maintenance tonight from 22:00 to 23:00. Services may be briefly unavailable.' },
  resolved: { level: 'info', message: 'The issue has been resolved. Thank you for your patience.' }
};
const BANNER_LEVEL_WORDS = { info: 'Info', warning: 'Warning', error: 'Critical' };
const maintenanceWeekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const MAINTENANCE_MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const BANNERS_REFRESH_MS = 30000;

/* ── Small helpers ──────────────────────────────────────── */
function radioValue(name) {
  const input = document.querySelector(`input[name="${name}"]:checked`);
  return input ? input.value : '';
}

function setRadio(name, value) {
  document.querySelectorAll(`input[name="${name}"]`).forEach(input => { input.checked = input.value === value; });
}

function bannerServiceList() {
  if (typeof adminServicesData !== 'undefined' && Array.isArray(adminServicesData)) return adminServicesData;
  return Array.isArray(servicesData) ? servicesData : [];
}

function bannerServiceName(key) {
  const svc = bannerServiceList().find(s => s.key === key);
  return svc ? (svc.name || svc.key) : key;
}

// "Plex", "Plex and Sonarr", "Plex, Sonarr and Radarr", "Plex, Sonarr and 3 more".
function namesList(names) {
  if (names.length <= 1) return names.join('');
  if (names.length <= 3) return names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1];
  return names.slice(0, 2).join(', ') + ' and ' + (names.length - 2) + ' more';
}

function bannerSourceOf(banner) {
  return banner.source || (banner.automatic ? 'automatic' : banner.scheduled ? 'scheduled' : 'manual');
}

function requestErrorText(e, fallback) {
  const body = e && e.body;
  if (typeof body === 'string' && body.trim()) return body.trim();
  if (body && typeof body === 'object' && body.error) return body.error;
  return fallback;
}

function showFormError(id, message) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = message;
  el.hidden = false;
}

function clearFormError(id) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = '';
  el.hidden = true;
}

// Browser-local value for a datetime-local input.
function localInputValue(date) {
  const pad = n => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

// The next :00 or :30 at least ten minutes away, a sensible default start.
function nextHalfHour(now = new Date()) {
  const date = new Date(now.getTime() + 10 * 60000);
  date.setSeconds(0, 0);
  const minutes = date.getMinutes();
  date.setMinutes(minutes === 0 || minutes === 30 ? minutes : minutes < 30 ? 30 : 60);
  return date;
}

// Adds minutes to a "YYYY-MM-DDTHH:MM" wall time without any timezone.
function addMinutesToWallTime(value, minutes) {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value || '');
  if (!match) return '';
  const [, y, mo, d, h, mi] = match.map(Number);
  const at = new Date(Date.UTC(y, mo - 1, d, h, mi) + minutes * 60000);
  const pad = n => String(n).padStart(2, '0');
  return `${String(at.getUTCFullYear()).padStart(4, '0')}-${pad(at.getUTCMonth() + 1)}-${pad(at.getUTCDate())}T${pad(at.getUTCHours())}:${pad(at.getUTCMinutes())}`;
}

function updateMessageCount(textareaId, countId) {
  const area = document.getElementById(textareaId);
  const count = document.getElementById(countId);
  if (area && count) count.textContent = `${area.value.length} / ${area.maxLength > 0 ? area.maxLength : 500}`;
}

/* ── Service pickers ────────────────────────────────────── */
function pickedServiceKeys(listId) {
  const list = document.getElementById(listId);
  if (!list) return [];
  return Array.from(list.querySelectorAll('input[type="checkbox"]:checked')).map(cb => cb.value);
}

// Fills a picker with every service, ticked ones first, keeping the filter.
function fillServicePicker(listId, searchId, selected, cls) {
  const list = document.getElementById(listId);
  if (!list) return;
  const chosen = new Set(selected || []);
  const services = bannerServiceList();
  list.innerHTML = '';
  if (services.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'link-picker-empty';
    empty.textContent = 'No services yet. Add one in the Services tab.';
    list.appendChild(empty);
  }
  services
    .map((svc, index) => ({ svc, index, checked: chosen.has(svc.key) }))
    .sort((a, b) => (Number(b.checked) - Number(a.checked)) || (a.index - b.index))
    .forEach(({ svc, checked }) => list.appendChild(buildLinkOption(svc, cls, { checked })));
  const search = document.getElementById(searchId);
  if (search) {
    search.hidden = services.length <= 6;
    filterLinkOptions(search);
  }
}

// Called when the service list reloads; keeps what is ticked.
function refreshBannerPickers() {
  fillServicePicker('bannerServiceList', 'bannerServiceSearch', pickedServiceKeys('bannerServiceList'), 'banner-service-cb');
  fillServicePicker('maintenanceServiceList', 'maintenanceServiceSearch', pickedServiceKeys('maintenanceServiceList'), 'maintenance-service-cb');
  updateBannerForm();
  updateMaintenanceScheduleForm();
}

/* ── Banner composer ────────────────────────────────────── */
function bannerFormState() {
  return {
    message: ($('#bannerMessage')?.value || '').trim(),
    level: radioValue('bannerLevel') || 'info',
    where: radioValue('bannerWhere') || 'page',
    keys: pickedServiceKeys('bannerServiceList'),
    start: radioValue('bannerStart') || 'now',
    startsAt: $('#bannerStartsAt')?.value || '',
    end: radioValue('bannerEnd') || 'never',
    duration: Number($('#bannerDuration')?.value) || 60,
    endsAt: $('#bannerEndsAt')?.value || ''
  };
}

// The banner's start and end as dates (null for "now" and "when I end it"),
// or the reason they don't work.
function bannerTimes(state, now = new Date()) {
  let start = null;
  if (state.start === 'at') {
    start = state.startsAt ? new Date(state.startsAt) : null;
    if (!start || Number.isNaN(start.getTime())) return { error: 'Choose when the banner starts' };
    if (start <= now) return { error: 'Choose a start in the future, or pick Now' };
  }
  let end = null;
  if (state.end === 'after') end = new Date((start || now).getTime() + state.duration * 60000);
  if (state.end === 'at') {
    end = state.endsAt ? new Date(state.endsAt) : null;
    if (!end || Number.isNaN(end.getTime())) return { error: 'Choose when the banner ends' };
    if (end <= (start || now)) return { error: start ? 'The end must be after the start' : 'Choose an end in the future' };
  }
  return { start, end };
}

function bannerFormProblem(state) {
  if (!state.message) return 'Write a message for the banner';
  if (state.where === 'services' && state.keys.length === 0) return 'Choose at least one service, or show it at the top of the page';
  return '';
}

// One sentence on where and when the banner shows.
function bannerWhenSentence(state, times) {
  const where = state.where === 'services'
    ? (state.keys.length ? 'on ' + namesList(state.keys.map(bannerServiceName)) : 'on the services you choose')
    : 'at the top of the page';
  if (times.error) return `Shows ${where}.`;
  const from = times.start ? `from ${formatBannerClock(times.start)}` : 'from now';
  const until = times.end ? `until ${formatBannerClock(times.end, times.start || new Date())}` : 'until you end it';
  return `Shows ${where} ${from} ${until}.`;
}

function updateBannerPreview() {
  const preview = $('#bannerPreview');
  if (!preview) return;
  const state = bannerFormState();
  const times = bannerTimes(state);
  const level = normalizeAlertLevel(state.level);
  const message = escapeHtml(state.message || 'Your message appears here.');
  const generated = bannerEditing && bannerSourceOf(bannerEditing) !== 'manual';
  const time = escapeHtml(generated ? bannerTimeLabel(bannerEditing)
    : times.end && !times.error ? `Until ${formatBannerClock(times.end)}` : 'Just now');
  if (state.where === 'services' && !generated) {
    const names = state.keys.length ? namesList(state.keys.map(bannerServiceName)) : 'Chosen services';
    preview.innerHTML = `
      <div class="banner-preview-card">
        <div class="banner-preview-card-title">${escapeHtml(names)}</div>
        <div class="service-alert ${level}">
          ${getServiceAlertIcon(level)}
          <div class="service-alert-content"><span>${message}</span><span class="service-alert-time">${time}</span></div>
        </div>
      </div>`;
  } else {
    preview.innerHTML = `
      <div class="site-alert ${level}">
        ${getAlertIcon(level)}
        <div class="site-alert-content"><span class="site-alert-message">${message}</span><span class="site-alert-time">${time}</span></div>
      </div>`;
  }
  const when = $('#bannerPreviewWhen');
  if (when) when.textContent = generated ? 'Shows where the automatic banner shows.' : bannerWhenSentence(state, times);
}

// Shows the inputs the chosen options need, filling sensible defaults.
function updateBannerForm() {
  const state = bannerFormState();
  const picker = $('#bannerServicePicker');
  if (picker) picker.hidden = state.where !== 'services';
  const startsAt = $('#bannerStartsAt');
  if (startsAt) {
    startsAt.hidden = state.start !== 'at';
    if (state.start === 'at' && !startsAt.value) startsAt.value = localInputValue(nextHalfHour());
  }
  const duration = $('#bannerDuration');
  if (duration) duration.hidden = state.end !== 'after';
  const endsAt = $('#bannerEndsAt');
  if (endsAt) {
    endsAt.hidden = state.end !== 'at';
    if (state.end === 'at' && !endsAt.value) {
      const from = state.start === 'at' && startsAt?.value ? new Date(startsAt.value) : nextHalfHour();
      endsAt.value = localInputValue(new Date(from.getTime() + 60 * 60000));
    }
  }
  updateMessageCount('bannerMessage', 'bannerMessageCount');
  updateBannerPreview();
}

function applyBannerTemplate(name) {
  const template = BANNER_TEMPLATES[name];
  const message = $('#bannerMessage');
  if (!template || !message) return;
  message.value = template.message;
  setRadio('bannerLevel', template.level);
  updateBannerForm();
  message.focus();
}

function setBannerScheduleEditable(editable) {
  ['#bannerWhereField', '#bannerWhenFields'].forEach(sel => {
    const el = $(sel);
    if (!el) return;
    el.querySelectorAll('input, select').forEach(input => { input.disabled = !editable; });
    el.classList.toggle('is-locked', !editable);
  });
  const templates = $('#bannerTemplates');
  if (templates) templates.hidden = !editable;
}

function resetBannerForm() {
  bannerEditing = null;
  const form = $('#bannerForm');
  if (!form) return;
  $('#bannerMessage').value = '';
  setRadio('bannerLevel', 'info');
  setRadio('bannerWhere', 'page');
  setRadio('bannerStart', 'now');
  setRadio('bannerEnd', 'never');
  $('#bannerStartsAt').value = '';
  $('#bannerEndsAt').value = '';
  $('#bannerDuration').value = '60';
  const search = $('#bannerServiceSearch');
  if (search) search.value = '';
  fillServicePicker('bannerServiceList', 'bannerServiceSearch', [], 'banner-service-cb');
  setBannerScheduleEditable(true);
  $('#bannerFormTitle').textContent = 'New banner';
  $('#saveBanner').textContent = 'Publish banner';
  $('#cancelBannerEdit').hidden = true;
  const note = $('#bannerFormNote');
  note.hidden = true;
  note.textContent = '';
  clearFormError('bannerFormError');
  updateBannerForm();
}

// Loads a banner into the composer. A maintenance banner opens its window
// instead, which is where its wording lives.
function editBanner(banner) {
  if (banner.schedule_id) {
    const schedule = latestMaintenanceWindows.find(s => s.id === banner.schedule_id);
    if (schedule) {
      editMaintenanceSchedule(schedule);
      return;
    }
  }
  resetBannerForm();
  bannerEditing = banner;
  const manual = bannerSourceOf(banner) === 'manual';
  const keys = bannerServiceKeys(banner);
  $('#bannerMessage').value = banner.message || '';
  setRadio('bannerLevel', normalizeAlertLevel(banner.level));
  setRadio('bannerWhere', keys.length ? 'services' : 'page');
  fillServicePicker('bannerServiceList', 'bannerServiceSearch', keys, 'banner-service-cb');
  if (manual) {
    const start = banner.starts_at ? new Date(banner.starts_at) : null;
    if (start && start > new Date()) {
      setRadio('bannerStart', 'at');
      $('#bannerStartsAt').value = localInputValue(start);
    }
    if (banner.ends_at) {
      setRadio('bannerEnd', 'at');
      $('#bannerEndsAt').value = localInputValue(new Date(banner.ends_at));
    }
  }
  setBannerScheduleEditable(manual);
  $('#bannerFormTitle').textContent = manual ? 'Edit banner' : 'Adjust automatic banner';
  $('#saveBanner').textContent = 'Save changes';
  $('#cancelBannerEdit').hidden = false;
  if (!manual) {
    const note = $('#bannerFormNote');
    note.textContent = 'Changes apply to this occurrence only; the next one uses the automatic wording.';
    note.hidden = false;
  }
  updateBannerForm();
  $('#bannerForm').scrollIntoView?.({ block: 'start', behavior: 'smooth' });
  $('#bannerMessage').focus({ preventScroll: true });
}

// Starts a new banner from an ended one.
function reuseBanner(banner) {
  resetBannerForm();
  const keys = bannerServiceKeys(banner);
  $('#bannerMessage').value = banner.message || '';
  setRadio('bannerLevel', normalizeAlertLevel(banner.level));
  setRadio('bannerWhere', keys.length ? 'services' : 'page');
  fillServicePicker('bannerServiceList', 'bannerServiceSearch', keys, 'banner-service-cb');
  updateBannerForm();
  $('#bannerForm').scrollIntoView?.({ block: 'start', behavior: 'smooth' });
  $('#bannerMessage').focus({ preventScroll: true });
}

async function saveBanner(event) {
  if (event) event.preventDefault();
  if (bannerSaving) return;
  clearFormError('bannerFormError');
  const editing = bannerEditing;
  const state = bannerFormState();
  let payload;
  let scheduled = false;
  if (editing && bannerSourceOf(editing) !== 'manual') {
    if (!state.message) {
      showFormError('bannerFormError', 'Write a message for the banner');
      return;
    }
    payload = { id: editing.id, occurrence_at: editing.created_at, message: state.message, level: state.level, hidden: Boolean(editing.hidden) };
  } else {
    const problem = bannerFormProblem(state);
    const times = bannerTimes(state);
    if (problem || times.error) {
      showFormError('bannerFormError', problem || times.error);
      return;
    }
    scheduled = Boolean(times.start);
    payload = {
      message: state.message,
      level: state.level,
      service_keys: state.where === 'services' ? state.keys : [],
      starts_at: times.start ? times.start.toISOString() : '',
      ends_at: times.end ? times.end.toISOString() : ''
    };
    if (editing) payload.id = editing.id;
  }

  const button = $('#saveBanner');
  bannerSaving = true;
  button.disabled = true;
  try {
    await j('/api/admin/status-alerts', {
      method: editing ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify(payload)
    });
    showToast(editing ? 'Banner updated' : scheduled ? 'Banner scheduled' : 'Banner published');
    resetBannerForm();
    await refreshBannersTab();
  } catch (e) {
    console.error('Failed to save banner', e);
    showFormError('bannerFormError', requestErrorText(e, editing ? 'Could not save the banner' : 'Could not publish the banner'));
  } finally {
    bannerSaving = false;
    button.disabled = false;
  }
}

/* ── Banner list ────────────────────────────────────────── */
async function loadAdminBanners() {
  const list = $('#bannersList');
  if (!list) return;
  try {
    const banners = await j('/api/admin/status-alerts', { headers: { 'X-CSRF-Token': getCsrf() } });
    latestAdminBanners = Array.isArray(banners) ? banners : [];
    renderBannerBoard(latestAdminBanners);
  } catch (e) {
    console.error('Failed to load admin banners', e);
    list.innerHTML = '<p class="banner-empty">Couldn’t load banners. The list refreshes every 30 seconds.</p>';
  }
}

function bannerRowWhen(banner, now = new Date()) {
  const clock = value => formatBannerClock(new Date(value), now);
  if (banner.kind === 'maintenance_upcoming') return `Announcing ${formatUpcomingBannerTime(banner.starts_at, banner.ends_at)}`;
  if (banner.kind === 'maintenance' || (banner.scheduled && !banner.automatic)) {
    return banner.ends_at ? `Until ${clock(banner.ends_at)}` : 'Until the window is ended';
  }
  if (banner.automatic) return formatAutomaticBannerTime(banner);
  if (banner.state === 'scheduled') {
    return `Starts ${clock(banner.starts_at)}` + (banner.ends_at ? `, ends ${clock(banner.ends_at)}` : '');
  }
  if (banner.state === 'ended') return banner.ends_at ? `Ended ${clock(banner.ends_at)}` : 'Ended';
  const since = banner.starts_at || banner.created_at;
  return (since ? `Since ${clock(since)}` : 'Showing') + (banner.ends_at ? `, until ${clock(banner.ends_at)}` : ', until you end it');
}

function bannerRowSource(banner) {
  if (banner.schedule_id) {
    const schedule = latestMaintenanceWindows.find(s => s.id === banner.schedule_id);
    return schedule ? `From “${schedule.name}”` : 'Maintenance window';
  }
  return bannerSourceOf(banner) === 'manual' ? '' : 'Automatic';
}

function bannerRowActions(banner) {
  const manual = bannerSourceOf(banner) === 'manual';
  const button = (action, label, extra = '') =>
    `<button type="button" class="btn mini ${extra}" data-banner-action="${action}">${label}</button>`;
  if (!manual) {
    return button('edit', banner.schedule_id ? 'Edit window' : 'Edit', 'ghost') +
      (banner.hidden ? button('show', 'Show again') : button('hide', 'Hide', 'ghost'));
  }
  if (banner.state === 'ended') return button('reuse', 'Reuse', 'ghost') + button('delete', 'Delete', 'danger');
  if (banner.state === 'scheduled') return button('edit', 'Edit', 'ghost') + button('delete', 'Delete', 'danger');
  return button('edit', 'Edit', 'ghost') + button('end', 'End now', 'danger');
}

function bannerRow(banner) {
  const level = normalizeAlertLevel(banner.level);
  const keys = bannerServiceKeys(banner);
  const where = keys.length ? 'On ' + namesList(keys.map(bannerServiceName)) : 'Top of the page';
  const meta = [where, bannerRowWhen(banner), bannerRowSource(banner)].filter(Boolean).join(' · ');
  const row = document.createElement('div');
  row.className = `banner-row is-${level}${banner.hidden ? ' is-hidden' : ''}`;
  row.dataset.id = banner.id;
  row.dataset.occurrence = banner.created_at || '';
  row.innerHTML = `
    <span class="level-chip is-${level}">${BANNER_LEVEL_WORDS[level]}</span>
    <div class="banner-row-body">
      <p class="banner-row-message">${escapeHtml(banner.message || '')}</p>
      <p class="banner-row-meta">${escapeHtml(meta)}</p>
    </div>
    <div class="banner-row-actions">${bannerRowActions(banner)}</div>
  `;
  return row;
}

function renderBannerBoard(banners) {
  const list = $('#bannersList');
  if (!list) return;
  const groups = { live: [], scheduled: [], hidden: [], ended: [] };
  banners.forEach(b => {
    const manual = bannerSourceOf(b) === 'manual';
    if (!manual && b.hidden) groups.hidden.push(b);
    else if (manual && b.state === 'scheduled') groups.scheduled.push(b);
    else if (manual && b.state === 'ended') groups.ended.push(b);
    else groups.live.push(b);
  });
  groups.scheduled.sort((a, b) => String(a.starts_at).localeCompare(String(b.starts_at)));

  list.innerHTML = '';
  const addGroup = (title, items, note, collapsed) => {
    if (!items.length) return;
    const group = document.createElement(collapsed ? 'details' : 'section');
    group.className = 'banner-group';
    const heading = document.createElement(collapsed ? 'summary' : 'h4');
    heading.className = 'banner-group-title';
    heading.textContent = `${title} (${items.length})`;
    group.appendChild(heading);
    if (note) {
      const p = document.createElement('p');
      p.className = 'banner-group-note';
      p.textContent = note;
      group.appendChild(p);
    }
    items.forEach(b => group.appendChild(bannerRow(b)));
    list.appendChild(group);
  };
  addGroup('Showing now', groups.live);
  addGroup('Scheduled', groups.scheduled);
  addGroup('Hidden from visitors', groups.hidden, 'Hidden for this occurrence; the next one shows again.');
  addGroup('Ended', groups.ended, 'Kept for 30 days so you can reuse them.', true);
  if (!list.children.length) {
    list.innerHTML = '<p class="banner-empty">No banners. Visitors see the page without any notice.</p>';
  }
  const clear = $('#clearEndedBanners');
  if (clear) clear.hidden = groups.ended.length === 0;
}

function findBannerFromRow(row) {
  return latestAdminBanners.find(b => b.id === row.dataset.id && (b.created_at || '') === row.dataset.occurrence) ||
    latestAdminBanners.find(b => b.id === row.dataset.id);
}

async function bannerRequest(method, url, body, successText, failureText) {
  try {
    await j(url, {
      method,
      headers: Object.assign({ 'X-CSRF-Token': getCsrf() }, body ? { 'Content-Type': 'application/json' } : {}),
      body: body ? JSON.stringify(body) : undefined
    });
    showToast(successText);
    await refreshBannersTab();
  } catch (e) {
    console.error(failureText, e);
    showToast(requestErrorText(e, failureText), 'error');
  }
}

function bannerDeleteURL(banner) {
  const params = new URLSearchParams({ id: banner.id });
  if (banner.created_at) params.set('occurrence_at', banner.created_at);
  return `/api/admin/status-alerts?${params.toString()}`;
}

async function onBannerBoardClick(event) {
  const button = event.target.closest('[data-banner-action]');
  if (!button) return;
  const banner = findBannerFromRow(button.closest('.banner-row'));
  if (!banner) return;
  switch (button.dataset.bannerAction) {
    case 'edit':
      editBanner(banner);
      break;
    case 'reuse':
      reuseBanner(banner);
      break;
    case 'end':
      await bannerRequest('PUT', '/api/admin/status-alerts',
        { id: banner.id, message: banner.message, level: banner.level, end_now: true },
        'Banner ended', 'Could not end the banner');
      break;
    case 'delete':
      if (!confirm('Delete this banner?')) return;
      if (bannerEditing?.id === banner.id) resetBannerForm();
      await bannerRequest('DELETE', bannerDeleteURL(banner), null, 'Banner deleted', 'Could not delete the banner');
      break;
    case 'hide':
      if (!confirm('Hide this banner from visitors? It shows again the next time it happens.')) return;
      await bannerRequest('DELETE', bannerDeleteURL(banner), null, 'Banner hidden', 'Could not hide the banner');
      break;
    case 'show':
      await bannerRequest('PUT', '/api/admin/status-alerts',
        { id: banner.id, occurrence_at: banner.created_at, hidden: false },
        'Banner shown again', 'Could not show the banner');
      break;
  }
}

async function clearEndedBanners() {
  if (!confirm('Delete every banner that has ended?')) return;
  await bannerRequest('DELETE', '/api/admin/status-alerts?ended=1', null, 'Ended banners deleted', 'Could not delete ended banners');
}

/* ── Maintenance windows ────────────────────────────────── */
function defaultTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch (_) {
    return 'UTC';
  }
}

function isValidTimezone(zone) {
  if (!zone) return false;
  try {
    new Intl.DateTimeFormat('en-GB', { timeZone: zone });
    return true;
  } catch (_) {
    return false;
  }
}

function populateTimezones() {
  const list = $('#maintenanceTimezones');
  if (!list || list.children.length) return;
  let zones = [];
  try {
    zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];
  } catch (_) {
    zones = [];
  }
  ['UTC', 'Europe/London', defaultTimezone()].forEach(zone => { if (!zones.includes(zone)) zones.push(zone); });
  zones.forEach(zone => {
    const option = document.createElement('option');
    option.value = zone;
    list.appendChild(option);
  });
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

// "4 Oct 2026, 22:00" from a "YYYY-MM-DDTHH:MM" wall time.
function maintenanceWallText(wall) {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}:\d{2})$/.exec(wall || '');
  if (!match) return '';
  return `${Number(match[3])} ${MAINTENANCE_MONTHS[Number(match[2]) - 1]} ${match[1]}, ${match[4]}`;
}

// "4 Oct 2026, 22:00 to 23:30", repeating the date only when it changes.
function maintenanceRangeText(startWall, endWall) {
  const start = maintenanceWallText(startWall);
  if (!start) return '';
  if (!endWall) return `${start} to when you end it`;
  const sameDay = startWall.slice(0, 10) === endWall.slice(0, 10);
  return `${start} to ${sameDay ? endWall.slice(11, 16) : maintenanceWallText(endWall)}`;
}

function maintenanceDurationParts(minutes) {
  const value = Number(minutes) || 30;
  const unit = [10080, 1440, 60, 1].find(candidate => value % candidate === 0);
  return { value: value / unit, unit };
}

function maintenanceDurationText(minutes) {
  const { value, unit } = maintenanceDurationParts(minutes);
  const name = { 1: 'min', 60: 'hour', 1440: 'day', 10080: 'week' }[unit];
  return `${value} ${name}${unit !== 1 && value !== 1 ? 's' : ''}`;
}

// The rule in words, in the window's own timezone.
function maintenanceTimingSummary(schedule) {
  if (schedule.schedule_type === 'once') {
    const range = maintenanceRangeText(maintenanceLocalDate(schedule.starts_at, schedule.timezone),
      maintenanceLocalDate(schedule.ends_at, schedule.timezone));
    return range || 'Unknown start';
  }
  const days = schedule.schedule_type === 'daily' ? 'Every day'
    : (schedule.weekdays?.length ? schedule.weekdays : [schedule.weekday])
      .map(day => maintenanceWeekdays[Number(day)] || 'Unknown day');
  const daysText = Array.isArray(days)
    ? (days.length === 1 ? days[0] + 's' : namesList(days.map(day => day.slice(0, 3))))
    : days;
  return `${daysText} at ${schedule.start_time || ''} for ${maintenanceDurationText(schedule.duration_minutes)}`;
}

function maintenanceNoticeText(minutes) {
  const option = $(`#maintenanceNotice option[value="${Number(minutes)}"]`);
  if (option && Number(minutes) > 0) return 'announced ' + option.textContent.toLowerCase();
  if (Number(minutes) > 0) return `announced ${maintenanceDurationText(minutes)} before`;
  return '';
}

function maintenanceFormState() {
  return {
    name: ($('#maintenanceName')?.value || '').trim(),
    message: ($('#maintenanceMessage')?.value || '').trim(),
    covers: radioValue('maintenanceCovers') || 'all',
    keys: pickedServiceKeys('maintenanceServiceList'),
    repeat: radioValue('maintenanceRepeat') || 'once',
    startsAt: $('#maintenanceStartsAt')?.value || '',
    end: radioValue('maintenanceEnd') || 'at',
    endsAt: $('#maintenanceEndsAt')?.value || '',
    onceDuration: Number($('#maintenanceOnceDuration')?.value) || 60,
    weekdays: $$('[name="maintenanceWeekdays"]:checked').map(input => Number(input.value)),
    startTime: $('#maintenanceStartTime')?.value || '',
    duration: Number($('#maintenanceDuration')?.value) * Number($('#maintenanceDurationUnit')?.value),
    timezone: ($('#maintenanceTimezone')?.value || '').trim(),
    notice: Number($('#maintenanceNotice')?.value) || 0,
    level: radioValue('maintenanceLevel') || 'warning',
    enabled: Boolean($('#maintenanceEnabled')?.checked),
    suppress: Boolean($('#maintenanceSuppressMonitoring')?.checked)
  };
}

// The end of a one-time window as a wall time, from the chosen end option.
function maintenanceOnceEnd(state) {
  if (state.end === 'never') return '';
  if (state.end === 'after') return addMinutesToWallTime(state.startsAt, state.onceDuration);
  return state.endsAt;
}

// Services that need the chosen ones, directly or through others.
function maintenanceDependents(keys) {
  const services = bannerServiceList();
  const covered = new Set(keys);
  const added = [];
  let grew = true;
  while (grew) {
    grew = false;
    services.forEach(svc => {
      if (covered.has(svc.key)) return;
      if (linkKeys(svc.depends_on).some(key => covered.has(key))) {
        covered.add(svc.key);
        added.push(svc.key);
        grew = true;
      }
    });
  }
  return added;
}

function maintenanceSummarySentence(state) {
  const parts = [];
  if (state.repeat === 'once') {
    const range = maintenanceRangeText(state.startsAt, maintenanceOnceEnd(state));
    if (range) parts.push(range);
  } else {
    parts.push(maintenanceTimingSummary({
      schedule_type: state.repeat, weekdays: state.weekdays, start_time: state.startTime,
      duration_minutes: Number.isFinite(state.duration) && state.duration > 0 ? Math.round(state.duration) : 30
    }));
  }
  if (state.timezone) parts.push(state.timezone);
  parts.push(state.covers === 'services'
    ? (state.keys.length ? 'covers ' + namesList(state.keys.map(bannerServiceName)) : 'choose the services it covers')
    : 'covers all services');
  parts.push(state.suppress ? 'pauses monitoring' : 'banner only');
  const notice = maintenanceNoticeText(state.notice);
  if (notice) parts.push(notice);
  return parts.join(' · ');
}

function updateTimezoneHint() {
  const hint = $('#maintenanceTimezoneNow');
  const zone = ($('#maintenanceTimezone')?.value || '').trim();
  if (!hint) return;
  if (!isValidTimezone(zone)) {
    hint.textContent = zone ? 'Unknown timezone. Try a name such as Europe/London.' : '';
    return;
  }
  const now = new Intl.DateTimeFormat('en-GB', {
    timeZone: zone, weekday: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23'
  }).format(new Date());
  hint.textContent = `It's ${now} there now.`;
}

function updateMaintenanceScheduleForm() {
  const form = $('#maintenanceScheduleForm');
  if (!form) return;
  const state = maintenanceFormState();
  const once = state.repeat === 'once';
  $('#maintenanceOnceFields').hidden = !once;
  $('#maintenanceRecurringFields').hidden = once;
  $('#maintenanceWeekdaysField').hidden = state.repeat !== 'weekly';
  $('#maintenanceEndsAt').hidden = state.end !== 'at';
  $('#maintenanceOnceDuration').hidden = state.end !== 'after';
  if (once && state.end === 'at' && state.startsAt && !state.endsAt) {
    $('#maintenanceEndsAt').value = addMinutesToWallTime(state.startsAt, 60);
  }
  $('#maintenanceServicePicker').hidden = state.covers !== 'services';

  const dependents = state.covers === 'services' ? maintenanceDependents(state.keys) : [];
  const hint = $('#maintenanceDependents');
  hint.hidden = dependents.length === 0;
  if (dependents.length) {
    const names = namesList(dependents.map(bannerServiceName));
    $('#maintenanceDependentsText').textContent =
      `${names} ${dependents.length === 1 ? 'depends' : 'depend'} on ${state.keys.length === 1 ? 'it' : 'these'} and may report problems too.`;
  }
  updateTimezoneHint();
  updateMessageCount('maintenanceMessage', 'maintenanceMessageCount');
  $('#maintenanceSummary').textContent = maintenanceSummarySentence(maintenanceFormState());
}

function addMaintenanceDependents() {
  const state = maintenanceFormState();
  const keys = state.keys.concat(maintenanceDependents(state.keys));
  fillServicePicker('maintenanceServiceList', 'maintenanceServiceSearch', keys, 'maintenance-service-cb');
  updateMaintenanceScheduleForm();
}

function setNoticeValue(minutes) {
  const select = $('#maintenanceNotice');
  const value = String(Number(minutes) || 0);
  if (!select.querySelector(`option[value="${value}"]`)) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = `${maintenanceDurationText(Number(value))} before`;
    select.appendChild(option);
  }
  select.value = value;
}

function resetMaintenanceScheduleForm() {
  maintenanceEditing = null;
  const form = $('#maintenanceScheduleForm');
  if (!form) return;
  const start = localInputValue(nextHalfHour());
  $('#maintenanceName').value = '';
  $('#maintenanceMessage').value = '';
  setRadio('maintenanceCovers', 'all');
  setRadio('maintenanceRepeat', 'once');
  $('#maintenanceStartsAt').value = start;
  setRadio('maintenanceEnd', 'at');
  $('#maintenanceEndsAt').value = addMinutesToWallTime(start, 60);
  $('#maintenanceOnceDuration').value = '60';
  $$('[name="maintenanceWeekdays"]').forEach(input => { input.checked = input.value === '1'; });
  $('#maintenanceStartTime').value = '02:00';
  $('#maintenanceDuration').value = '30';
  $('#maintenanceDurationUnit').value = '1';
  $('#maintenanceTimezone').value = defaultTimezone();
  setNoticeValue(0);
  setRadio('maintenanceLevel', 'warning');
  $('#maintenanceEnabled').checked = true;
  $('#maintenanceSuppressMonitoring').checked = true;
  const search = $('#maintenanceServiceSearch');
  if (search) search.value = '';
  fillServicePicker('maintenanceServiceList', 'maintenanceServiceSearch', [], 'maintenance-service-cb');
  $('#maintenanceFormTitle').textContent = 'New maintenance window';
  $('#saveMaintenanceSchedule').textContent = 'Save window';
  $('#cancelMaintenanceSchedule').hidden = true;
  clearFormError('maintenanceFormError');
  updateMaintenanceScheduleForm();
}

function editMaintenanceSchedule(schedule) {
  resetMaintenanceScheduleForm();
  maintenanceEditing = schedule;
  const keys = Array.isArray(schedule.service_keys) ? schedule.service_keys : [];
  $('#maintenanceName').value = schedule.name || '';
  $('#maintenanceMessage').value = schedule.message || '';
  setRadio('maintenanceCovers', keys.length ? 'services' : 'all');
  fillServicePicker('maintenanceServiceList', 'maintenanceServiceSearch', keys, 'maintenance-service-cb');
  setRadio('maintenanceRepeat', schedule.schedule_type || 'weekly');
  const weekdays = schedule.weekdays?.length ? schedule.weekdays : [schedule.weekday ?? 1];
  $$('[name="maintenanceWeekdays"]').forEach(input => { input.checked = weekdays.includes(Number(input.value)); });
  $('#maintenanceStartsAt').value = maintenanceLocalDate(schedule.starts_at, schedule.timezone);
  setRadio('maintenanceEnd', schedule.schedule_type === 'once' && !schedule.ends_at ? 'never' : 'at');
  $('#maintenanceEndsAt').value = maintenanceLocalDate(schedule.ends_at, schedule.timezone);
  $('#maintenanceStartTime').value = schedule.start_time || '02:00';
  const duration = maintenanceDurationParts(schedule.duration_minutes);
  $('#maintenanceDuration').value = String(duration.value);
  $('#maintenanceDurationUnit').value = String(duration.unit);
  $('#maintenanceTimezone').value = schedule.timezone || defaultTimezone();
  setNoticeValue(schedule.notice_minutes);
  setRadio('maintenanceLevel', normalizeAlertLevel(schedule.level));
  $('#maintenanceEnabled').checked = Boolean(schedule.enabled);
  $('#maintenanceSuppressMonitoring').checked = Boolean(schedule.suppress_monitoring);
  $('#maintenanceFormTitle').textContent = 'Edit maintenance window';
  $('#saveMaintenanceSchedule').textContent = 'Save changes';
  $('#cancelMaintenanceSchedule').hidden = false;
  updateMaintenanceScheduleForm();
  $('#maintenanceScheduleForm').scrollIntoView?.({ block: 'start', behavior: 'smooth' });
  $('#maintenanceName').focus({ preventScroll: true });
}

// The request body for the form, or the reason it can't be saved yet.
function maintenancePayload(state) {
  if (!state.name || !state.message) return { error: 'Give the window a name and a banner message' };
  if (!isValidTimezone(state.timezone)) return { error: 'Choose a timezone such as Europe/London or UTC' };
  if (state.covers === 'services' && state.keys.length === 0) return { error: 'Choose at least one service, or cover all services' };
  const payload = {
    id: maintenanceEditing?.id || '',
    name: state.name,
    message: state.message,
    schedule_type: state.repeat,
    timezone: state.timezone,
    level: state.level,
    enabled: state.enabled,
    suppress_monitoring: state.suppress,
    service_keys: state.covers === 'services' ? state.keys : [],
    notice_minutes: state.notice
  };
  if (state.repeat === 'once') {
    if (!state.startsAt) return { error: 'Choose when the window starts' };
    payload.starts_at = state.startsAt;
    payload.ends_at = maintenanceOnceEnd(state);
    if (state.end === 'at' && !payload.ends_at) return { error: 'Choose when the window ends' };
    if (payload.ends_at && payload.ends_at <= payload.starts_at) return { error: 'The end must be after the start' };
    // Keep an exact stored instant (including the second of two repeated
    // clock times at a clock change) when its shown wall time wasn't edited.
    if (maintenanceEditing?.timezone === payload.timezone) {
      ['starts_at', 'ends_at'].forEach(key => {
        if (payload[key] && payload[key] === maintenanceLocalDate(maintenanceEditing[key], payload.timezone)) {
          payload[key] = maintenanceEditing[key];
        }
      });
    }
    return { payload };
  }
  if (!state.startTime) return { error: 'Choose the time it starts' };
  const duration = state.duration;
  if (!Number.isFinite(duration) || duration < 1 || duration > 153722867 || Math.abs(duration - Math.round(duration)) > 0.000001) {
    return { error: 'Enter a duration of at least one whole minute (up to approximately 292 years)' };
  }
  payload.start_time = state.startTime;
  payload.duration_minutes = Math.round(duration);
  if (state.repeat === 'weekly') {
    if (state.weekdays.length === 0) return { error: 'Choose at least one weekday' };
    payload.weekdays = state.weekdays;
    payload.weekday = state.weekdays[0];
  }
  return { payload };
}

async function saveMaintenanceSchedule(event) {
  if (event) event.preventDefault();
  if (maintenanceSaving) return;
  clearFormError('maintenanceFormError');
  const { payload, error } = maintenancePayload(maintenanceFormState());
  if (error) {
    showFormError('maintenanceFormError', error);
    return;
  }
  const button = $('#saveMaintenanceSchedule');
  maintenanceSaving = true;
  button.disabled = true;
  try {
    await j('/api/admin/maintenance-schedules', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify(payload)
    });
    showToast(payload.id ? 'Window updated' : 'Window saved');
    resetMaintenanceScheduleForm();
    await refreshBannersTab();
  } catch (e) {
    console.error('Failed to save maintenance window', e);
    showFormError('maintenanceFormError', requestErrorText(e, 'Could not save the window'));
  } finally {
    maintenanceSaving = false;
    button.disabled = false;
  }
}

async function loadMaintenanceSchedules() {
  const list = $('#maintenanceSchedulesList');
  if (!list) return;
  try {
    const schedules = await j('/api/admin/maintenance-schedules', { headers: { 'X-CSRF-Token': getCsrf() } });
    latestMaintenanceWindows = Array.isArray(schedules) ? schedules : [];
    renderMaintenanceSchedules(latestMaintenanceWindows);
  } catch (e) {
    console.error('Failed to load maintenance schedules', e);
    list.innerHTML = '<p class="banner-empty">Couldn’t load maintenance windows. The list refreshes every 30 seconds.</p>';
  }
}

// State chip text and class for a window, most urgent first.
function maintenanceWindowState(schedule, now = new Date()) {
  if (schedule.problem) return { cls: 'is-problem', text: 'Needs attention' };
  if (!schedule.enabled) return { cls: 'is-off', text: 'Off' };
  if (schedule.active) return { cls: 'is-running', text: 'Running' };
  if (schedule.next_starts_at) return { cls: 'is-next', text: 'Next ' + formatBannerClock(new Date(schedule.next_starts_at), now) };
  return { cls: 'is-done', text: 'Finished' };
}

function renderMaintenanceSchedules(schedules) {
  const list = $('#maintenanceSchedulesList');
  if (!list) return;
  if (!Array.isArray(schedules) || schedules.length === 0) {
    list.innerHTML = '<p class="banner-empty">No maintenance windows.</p>';
    return;
  }
  const rank = s => (s.problem ? 0 : !s.enabled ? 3 : s.active ? 1 : s.next_starts_at ? 2 : 4);
  const ordered = schedules.slice().sort((a, b) => (rank(a) - rank(b)) ||
    String(a.next_starts_at || '').localeCompare(String(b.next_starts_at || '')));

  list.innerHTML = '';
  ordered.forEach(schedule => {
    const level = normalizeAlertLevel(schedule.level);
    const state = maintenanceWindowState(schedule);
    const keys = Array.isArray(schedule.service_keys) ? schedule.service_keys : [];
    const covers = keys.length ? 'Covers ' + namesList(keys.map(bannerServiceName)) : 'Covers all services';
    const timing = schedule.active && schedule.enabled
      ? (schedule.active_ends_at ? `Until ${formatBannerClock(new Date(schedule.active_ends_at))}` : 'Until you end it')
      : maintenanceTimingSummary(schedule);
    const meta = [
      timing, schedule.timezone || 'UTC', covers,
      schedule.suppress_monitoring ? 'Pauses monitoring' : 'Banner only',
      maintenanceNoticeText(schedule.notice_minutes)
    ].filter(Boolean).join(' · ');

    const row = document.createElement('div');
    row.className = `banner-row window-row is-${level}${schedule.enabled ? '' : ' is-off'}`;
    row.dataset.id = schedule.id;
    row.innerHTML = `
      <span class="state-chip ${state.cls}">${escapeHtml(state.text)}</span>
      <div class="banner-row-body">
        <p class="banner-row-title">${escapeHtml(schedule.name || 'Maintenance')}</p>
        <p class="banner-row-message">${escapeHtml(schedule.message || '')}</p>
        <p class="banner-row-meta">${escapeHtml(meta)}</p>
        ${schedule.problem ? `<p class="banner-row-problem">${escapeHtml(schedule.problem)}</p>` : ''}
      </div>
      <div class="banner-row-actions">
        <label class="toggle window-toggle" title="Window enabled">
          <input type="checkbox" data-window-action="toggle" ${schedule.enabled ? 'checked' : ''} aria-label="${escapeHtml((schedule.name || 'Window') + ' enabled')}">
          <span class="slider"></span>
        </label>
        <button type="button" class="btn mini ghost" data-window-action="edit">Edit</button>
        <button type="button" class="btn mini danger" data-window-action="delete">Delete</button>
      </div>
    `;
    list.appendChild(row);
  });
}

async function onMaintenanceListClick(event) {
  const control = event.target.closest('[data-window-action]');
  if (!control) return;
  const row = control.closest('.banner-row');
  const schedule = latestMaintenanceWindows.find(s => s.id === row?.dataset.id);
  if (!schedule) return;
  const action = control.dataset.windowAction;
  if (action === 'edit' && event.type === 'click') editMaintenanceSchedule(schedule);
  if (action === 'delete' && event.type === 'click') await deleteMaintenanceSchedule(schedule.id);
  if (action === 'toggle' && event.type === 'change') await setMaintenanceEnabled(schedule, control.checked);
}

async function setMaintenanceEnabled(schedule, enabled) {
  const body = Object.assign({}, schedule, { enabled });
  ['active', 'active_starts_at', 'active_ends_at', 'next_starts_at', 'next_ends_at', 'problem'].forEach(key => { delete body[key]; });
  await bannerRequest('POST', '/api/admin/maintenance-schedules', body,
    enabled ? 'Window turned on' : 'Window turned off', 'Could not change the window');
}

async function deleteMaintenanceSchedule(id) {
  if (!confirm('Delete this maintenance window?')) return;
  if (maintenanceEditing?.id === id) resetMaintenanceScheduleForm();
  await bannerRequest('DELETE', `/api/admin/maintenance-schedules?id=${encodeURIComponent(id)}`, null,
    'Window deleted', 'Could not delete the window');
}

/* ── Wiring ─────────────────────────────────────────────── */
async function refreshBannersTab() {
  await Promise.all([
    loadAdminBanners(),
    loadMaintenanceSchedules(),
    typeof loadBanners === 'function' ? loadBanners() : null
  ]);
}

function onPickerInput(event) {
  if (event.target.matches('.link-picker-search')) filterLinkOptions(event.target);
}

function initBannersTab() {
  const bannerForm = $('#bannerForm');
  const maintenanceForm = $('#maintenanceScheduleForm');
  if (!bannerForm || !maintenanceForm || bannerForm.dataset.ready) return;
  bannerForm.dataset.ready = 'true';
  populateTimezones();

  bannerForm.addEventListener('submit', saveBanner);
  bannerForm.addEventListener('input', event => { onPickerInput(event); updateBannerForm(); });
  bannerForm.addEventListener('change', updateBannerForm);
  bannerForm.addEventListener('keydown', event => {
    if (event.key === 'Enter' && event.target.matches('.link-picker-search')) event.preventDefault();
  });
  $('#bannerTemplates').addEventListener('click', event => {
    const button = event.target.closest('[data-template]');
    if (button) applyBannerTemplate(button.dataset.template);
  });
  $('#cancelBannerEdit').addEventListener('click', resetBannerForm);
  $('#clearEndedBanners').addEventListener('click', clearEndedBanners);
  $('#bannersList').addEventListener('click', onBannerBoardClick);

  maintenanceForm.addEventListener('submit', saveMaintenanceSchedule);
  maintenanceForm.addEventListener('input', event => { onPickerInput(event); updateMaintenanceScheduleForm(); });
  maintenanceForm.addEventListener('change', updateMaintenanceScheduleForm);
  maintenanceForm.addEventListener('keydown', event => {
    if (event.key === 'Enter' && event.target.matches('.link-picker-search')) event.preventDefault();
  });
  $('#addMaintenanceDependents').addEventListener('click', addMaintenanceDependents);
  $('#cancelMaintenanceSchedule').addEventListener('click', resetMaintenanceScheduleForm);
  const windows = $('#maintenanceSchedulesList');
  windows.addEventListener('click', onMaintenanceListClick);
  windows.addEventListener('change', onMaintenanceListClick);

  resetBannerForm();
  resetMaintenanceScheduleForm();

  // Banners start and end on their own; keep the lists current while open.
  if (!bannersRefreshTimer) {
    bannersRefreshTimer = setInterval(() => {
      const tab = document.getElementById('tab-banners');
      if (!tab || !tab.classList.contains('active') || document.hidden) return;
      loadAdminBanners();
      loadMaintenanceSchedules();
    }, BANNERS_REFRESH_MS);
  }
}

// Called by the tab switcher in admin-ui.js.
function showBannersTab() {
  refreshBannerPickers();
  loadMaintenanceSchedules().then(loadAdminBanners);
}

window.addEventListener('load', initBannersTab);
