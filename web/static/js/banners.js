/* Banner Functions */
let bannersLoading = false;
// Live banners attached to each service, for the topology map.
let serviceBannersByKey = {};

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

// A time with as much date as it needs: "23:30" today, "Sat 23:30" within
// the coming week, "4 Oct 23:30" further out.
function formatBannerClock(date, now = new Date()) {
  const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (date.toDateString() === now.toDateString()) return time;
  const days = (date - now) / 86400000;
  if (days > -6 && days < 6) {
    return `${date.toLocaleDateString([], { weekday: 'short' })} ${time}`;
  }
  return `${date.toLocaleDateString([], { day: 'numeric', month: 'short' })} ${time}`;
}

function formatScheduledBannerTime(endsAt) {
  if (!endsAt) return 'Scheduled maintenance';
  const end = new Date(endsAt);
  if (Number.isNaN(end.getTime())) return 'Scheduled maintenance';
  return `Ends ${formatBannerClock(end)}`;
}

// The window an upcoming-maintenance banner announces, in the visitor's time.
function formatUpcomingBannerTime(startsAt, endsAt) {
  const start = new Date(startsAt);
  if (!startsAt || Number.isNaN(start.getTime())) return 'Planned maintenance';
  const day = start.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' });
  const clock = d => d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const end = endsAt ? new Date(endsAt) : null;
  if (!end || Number.isNaN(end.getTime())) return `From ${day}, ${clock(start)}`;
  if (end.toDateString() === start.toDateString()) return `${day}, ${clock(start)}–${clock(end)}`;
  return `${day}, ${clock(start)} – ${formatBannerClock(end, start)}`;
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

// The short time note shown at the end of a banner.
function bannerTimeLabel(banner) {
  if (banner.kind === 'maintenance_upcoming') return formatUpcomingBannerTime(banner.starts_at, banner.ends_at);
  if (banner.scheduled) return formatScheduledBannerTime(banner.ends_at);
  if (banner.automatic) return formatAutomaticBannerTime(banner);
  const end = banner.ends_at ? new Date(banner.ends_at) : null;
  if (end && !Number.isNaN(end.getTime())) return `Until ${formatBannerClock(end)}`;
  return formatBannerTime(banner.starts_at || banner.created_at);
}

function normalizeAlertLevel(level) {
  const allowed = ['info', 'warning', 'error'];
  return allowed.includes(level) ? level : 'info';
}

// The services a banner shows on; none means the top of the page.
function bannerServiceKeys(banner) {
  if (Array.isArray(banner.service_keys) && banner.service_keys.length) return banner.service_keys;
  return banner.service_key ? [banner.service_key] : [];
}

function renderSiteBanners(banners) {
  const container = $('#siteAlerts');
  if (!container) return;

  // Manual banners refresh independently from automatic system alerts.
  Array.from(container.children).forEach(child => {
    if (!child.hasAttribute('data-auto-alert')) child.remove();
  });

  const globalBanners = banners.filter(b => bannerServiceKeys(b).length === 0);

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
    if (b.scheduled) div.dataset.scheduled = 'true';
    if (b.kind === 'maintenance_upcoming') div.dataset.upcoming = 'true';
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
        <span class="site-alert-time">${escapeHtml(bannerTimeLabel(b))}</span>
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

  const byKey = {};
  banners.forEach(b => {
    bannerServiceKeys(b).forEach(key => {
      (byKey[key] = byKey[key] || []).push(b);
      const card = $(`#card-${key}`);
      if (!card) return;

      // Check if banner already exists
      if (card.querySelector(`.service-alert[data-id="${b.id}"]`)) return;

      const level = normalizeAlertLevel(b.level);
      const alertDiv = document.createElement('div');
      alertDiv.className = `service-alert ${level}`;
      alertDiv.dataset.id = b.id;
      alertDiv.innerHTML = `
        ${getServiceAlertIcon(level)}
        <div class="service-alert-content">
          <span>${escapeHtml(b.message || '')}</span>
          <span class="service-alert-time">${escapeHtml(bannerTimeLabel(b))}</span>
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
  });

  // The map shows the same notices on its services.
  serviceBannersByKey = byKey;
  if (typeof updateTopologyNotices === 'function') updateTopologyNotices();
}
