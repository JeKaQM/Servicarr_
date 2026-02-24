/* Banner Functions */
async function loadBanners() {
  try {
    const banners = await j('/api/status-alerts');
    renderSiteBanners(banners);
    renderServiceBanners(banners);
  } catch (e) {
    console.error('Failed to load banners', e);
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

function normalizeAlertLevel(level) {
  const allowed = ['info', 'warning', 'error'];
  return allowed.includes(level) ? level : 'info';
}

function renderSiteBanners(banners) {
  const container = $('#siteAlerts');
  if (!container) return;
  container.innerHTML = '';

  // Only show global banners (no service_key) at the top
  const globalBanners = banners.filter(b => !b.service_key);

  globalBanners.forEach(b => {
    const level = normalizeAlertLevel(b.level);
    const message = escapeHtml(b.message || '');
    const div = document.createElement('div');
    div.className = `site-alert ${level}`;
    div.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
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
