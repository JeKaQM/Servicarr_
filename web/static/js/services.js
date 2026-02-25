let servicesData = [];
let serviceTemplates = [];
let editingServiceId = null;

// Service Icons SVG - inline for simplicity
const SERVICE_ICONS = {
  server: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="13" width="20" height="6" rx="2"/><circle cx="6" cy="6" r="1" fill="currentColor"/><circle cx="6" cy="16" r="1" fill="currentColor"/></svg>`,
  plex: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L4 7v10l8 5 8-5V7l-8-5zm0 2.5L17 8v8l-5 3-5-3V8l5-3.5z"/></svg>`,
  overseerr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M8 12l3 3 5-5" stroke="white" stroke-width="2" fill="none"/></svg>`,
  jellyfin: `<svg viewBox="0 0 24 24" fill="currentColor"><ellipse cx="12" cy="6" rx="4" ry="4"/><ellipse cx="12" cy="18" rx="4" ry="4"/><ellipse cx="12" cy="12" rx="8" ry="4" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`,
  emby: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/></svg>`,
  sonarr: `<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 15l3-3 2 2 5-5" stroke="white" stroke-width="2" fill="none"/></svg>`,
  radarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3" fill="white"/></svg>`,
  prowlarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M8 12h8M12 8v8" stroke="white" stroke-width="2"/></svg>`,
  lidarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="4" fill="white"/><circle cx="12" cy="12" r="1.5" fill="currentColor"/></svg>`,
  readarr: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M4 4h6v16H4V4zm10 0h6v16h-6V4z"/></svg>`,
  bazarr: `<svg viewBox="0 0 24 24" fill="currentColor"><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h12M6 14h8" stroke="white" stroke-width="1.5"/></svg>`,
  tautulli: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 3v18h18V3H3zm16 16H5V5h14v14z"/><path d="M7 17V9l4 4-4 4zm6-8h4v2h-4V9zm0 4h4v2h-4v-2z"/></svg>`,
  sabnzbd: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`,
  qbittorrent: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M12 6v12M6 12h12" stroke="white" stroke-width="2"/></svg>`,
  transmission: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2" stroke="white" stroke-width="2" fill="none"/></svg>`,
  homeassistant: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 3L2 12h3v8h14v-8h3L12 3zm0 12a2 2 0 100-4 2 2 0 000 4z"/></svg>`,
  pihole: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6" fill="white"/><circle cx="12" cy="12" r="3" fill="currentColor"/></svg>`,
  portainer: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M4 4h7v7H4V4zm9 0h7v7h-7V4zM4 13h7v7H4v-7zm9 0h7v7h-7v-7z"/></svg>`,
  website: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z"/></svg>`,
  custom: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 1v4m0 14v4M4.22 4.22l2.83 2.83m9.9 9.9l2.83 2.83M1 12h4m14 0h4M4.22 19.78l2.83-2.83m9.9-9.9l2.83-2.83"/></svg>`
};

// Service Icons - use image files for known services, inline SVG for others
const SERVICE_ICON_FILES = ['server', 'plex', 'overseerr'];

// Get icon HTML for a service - supports custom icon URLs
function getServiceIconHtml(serviceTypeOrObj, iconUrl = null) {
  let serviceType = serviceTypeOrObj;
  let customIconUrl = iconUrl;

  // Handle service object
  if (typeof serviceTypeOrObj === 'object' && serviceTypeOrObj !== null) {
    serviceType = serviceTypeOrObj.service_type || 'custom';
    customIconUrl = serviceTypeOrObj.icon_url || null;
  }

  // Use custom icon URL if provided
  if (customIconUrl) {
    // Sanitize URL - only allow http(s), data, and relative paths
    const safeUrl = /^(https?:\/\/|data:image\/|\/static\/)/.test(customIconUrl) ? escapeHtml(customIconUrl) : '';
    if (safeUrl) {
      return `<img src="${safeUrl}" class="icon service-icon-img" alt="${escapeHtml(serviceType)}" data-fallback="${escapeHtml(serviceType)}"/><span class="icon icon-fallback" style="display:none;">${SERVICE_ICONS[serviceType] || SERVICE_ICONS.custom}</span>`;
    }
  }

  // Use local image file if available
  if (SERVICE_ICON_FILES.includes(serviceType)) {
    return `<img src="/static/images/${serviceType}.svg" class="icon" alt="${serviceType}"/>`;
  }

  // Fallback to inline SVG for other services
  return `<span class="icon">${SERVICE_ICONS[serviceType] || SERVICE_ICONS.custom}</span>`;
}

async function loadServiceTemplates() {
  try {
    const templates = await j('/api/services/templates');
    serviceTemplates = templates;
    populateTemplateDropdown();
  } catch (e) {
    console.error('Failed to load service templates', e);
  }
}

function populateTemplateDropdown() {
  const select = $('#serviceTemplate');
  if (!select || !serviceTemplates.length) return;

  select.innerHTML = '<option value="">Select a template...</option>';
  serviceTemplates.forEach(t => {
    const opt = document.createElement('option');
    opt.value = t.type; // Templates use 'type' field
    opt.textContent = t.name;
    select.appendChild(opt);
  });
}

function getProtocolBadge(svc) {
  const url = (svc.url || '').toLowerCase();
  const checkType = (svc.check_type || 'http').toLowerCase();

  if (checkType === 'always_up') {
    return 'DEMO';
  }
  if (checkType === 'tcp' || url.startsWith('tcp://')) {
    return 'TCP';
  }
  if (checkType === 'dns' || url.startsWith('dns://')) {
    return 'DNS';
  }
  if (url.startsWith('https://')) {
    return 'HTTPS';
  }
  if (url.startsWith('http://')) {
    return 'HTTP';
  }
  return checkType.toUpperCase();
}

function renderDynamicUptimeBars(services) {
  const container = $('#uptime-bars-container');
  if (!container) return;

  container.innerHTML = '';

  services.forEach(svc => {
    const protocolBadge = getProtocolBadge(svc);
    const svcName = escapeHtml(svc.name || '');
    const row = document.createElement('div');
    row.className = 'service-uptime';
    row.innerHTML = `
      <div class="service-uptime-header">
        <span class="service-name">${svcName}</span>
        <span class="protocol-badge">${protocolBadge}</span>
        <span class="uptime-percent" id="uptime-${svc.key}">—%</span>
      </div>
      <div class="uptime-bar-container">
        <div class="uptime-bar" id="uptime-bar-${svc.key}"></div>
      </div>
    `;
    container.appendChild(row);
  });
}

async function loadServices() {
  try {
    // Load visible services for public view
    const services = await j('/api/services');
    servicesData = services;
    renderServiceCards(services);
    return services;
  } catch (e) {
    console.error('Failed to load services', e);
    return [];
  }
}

async function loadAllServices() {
  try {
    // Load all services for admin view (includes hidden)
    const services = await j('/api/admin/services', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    servicesData = services;
    renderAdminServicesList(services);
    populateBannerScopeDropdown(); // Update banner scope dropdown
    return services;
  } catch (e) {
    console.error('Failed to load all services', e);
    return [];
  }
}

function renderServiceCards(services) {
  const container = $('#services-container');
  if (!container) return;

  container.innerHTML = '';

  services.forEach(svc => {
    const card = document.createElement('section');
    card.className = 'card';
    card.id = `card-${svc.key}`;
    card.setAttribute('data-key', svc.key);

    const iconHtml = getServiceIconHtml(svc);
    const svcName = escapeHtml(svc.name || '');
    const svcLabel = escapeHtml(getServiceLabel(svc.service_type));

    // Match original structure - no clickable link exposing URL
    card.innerHTML = `
      <div class="card-header">
        <div class="card-icon-wrapper">
          ${iconHtml}
        </div>
        <div class="card-title-wrapper">
          <strong class="card-title">${svcName}</strong>
          <div class="card-subtitle">${svcLabel}</div>
        </div>
        <div class="card-status-indicator">
          <span class="pill warn">—</span>
        </div>
      </div>
      
      <div class="card-body">
        <div class="kpi-container">
          <div class="kpi">—</div>
          <div class="label kpi-status">—</div>
        </div>
        
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-label">Uptime</div>
            <div class="stat-value" id="uptime-24h-${svc.key}">—</div>
          </div>
          <div class="stat-item">
            <div class="stat-label">Response</div>
            <div class="stat-value" id="avg-response-${svc.key}">—</div>
          </div>
          <div class="stat-item">
            <div class="stat-label">Checked</div>
            <div class="stat-value" id="last-check-${svc.key}">—</div>
          </div>
        </div>
      </div>
      
      <div class="row adminRow hidden">
        <div class="label">Admin</div>
        <div class="ops">
          <button class="btn mini checkNow">Check now</button>
          <label class="toggle">
            <input type="checkbox" class="monitorToggle" checked>
            <span class="slider"></span>
            <span class="toggleLabel">Monitor</span>
          </label>
        </div>
      </div>
    `;

    container.appendChild(card);
  });

  // Rebind event handlers for new cards
  $$('.checkNow').forEach(btn => {
    btn.removeEventListener('click', checkNowHandler);
    btn.addEventListener('click', checkNowHandler);
  });

  $$('.monitorToggle').forEach(toggle => {
    toggle.removeEventListener('change', toggleMonitoringHandler);
    toggle.addEventListener('change', toggleMonitoringHandler);
  });

  applyAdminUIState();
}

// Get service label based on type
function getServiceLabel(serviceType) {
  const labels = {
    server: 'Health Check',
    plex: 'Media Server',
    overseerr: 'Request Management',
    jellyfin: 'Media Server',
    emby: 'Media Server',
    sonarr: 'TV Shows',
    radarr: 'Movies',
    prowlarr: 'Indexer Manager',
    lidarr: 'Music',
    readarr: 'Books',
    bazarr: 'Subtitles',
    tautulli: 'Plex Stats',
    sabnzbd: 'Usenet Downloader',
    qbittorrent: 'Torrent Client',
    transmission: 'Torrent Client',
    homeassistant: 'Home Automation',
    pihole: 'DNS Filter',
    portainer: 'Container Manager',
    website: 'Website',
    custom: 'Service'
  };
  return labels[serviceType] || 'Service';
}

function checkNowHandler(e) {
  checkNowFor(e.target.closest('.card'));
}

function toggleMonitoringHandler(e) {
  toggleMonitoring(e.target.closest('.card'), e.target.checked);
}

/* ── View Toggle (Cards ↔ Matrix) ──────────────────────── */