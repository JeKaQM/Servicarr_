// ===== LOGS TAB FUNCTIONALITY =====
let logsOffset = 0;
const LOGS_LIMIT = 50;
let currentLogFilters = { level: '', category: '', service: '' };

async function loadLogStats() {
  try {
    const res = await j('/api/admin/logs/stats');
    if (res && res.success !== false) {
      setResText('logTotalCount', res.total_logs || 0);
      setResText('logErrorCount', res.error_count || 0);
      setResText('logWarnCount', res.warn_count || 0);
      setResText('logInfoCount', res.info_count || 0);
    }
  } catch (err) {
    console.error('[Logs] Failed to load stats:', err);
    throw err;
  }
}

async function loadLogs(append = false) {
  try {
    const params = new URLSearchParams();
    params.set('limit', LOGS_LIMIT);
    params.set('offset', logsOffset);
    if (currentLogFilters.level) params.set('level', currentLogFilters.level);
    if (currentLogFilters.category) params.set('category', currentLogFilters.category);
    if (currentLogFilters.service) params.set('service', currentLogFilters.service);

    const res = await j('/api/admin/logs?' + params.toString());
    if (!res || !res.logs) {
      if (!append) {
        renderLogs('#allLogsList', [], false);
        renderLogs('#errorLogsList', [], true);
      }
      return;
    }

    const logs = res.logs;
    if (!append) {
      renderLogs('#allLogsList', logs, false);
      // Also show errors/warnings in highlights section
      const errorWarnLogs = logs.filter(l => l.level === 'error' || l.level === 'warn');
      renderLogs('#errorLogsList', errorWarnLogs.slice(0, 10), true);
    } else {
      appendLogs('#allLogsList', logs);
    }

    // Show/hide load more button
    const loadMoreBtn = $('#loadMoreLogs');
    if (loadMoreBtn) {
      loadMoreBtn.style.display = logs.length >= LOGS_LIMIT ? 'block' : 'none';
    }
  } catch (err) {
    console.error('[Logs] Failed to load logs:', err);
    throw err;
  }
}

function renderLogs(selector, logs, isErrorList = false) {
  const container = $(selector);
  if (!container) return;

  if (!logs || logs.length === 0) {
    if (isErrorList) {
      // Success state for error list
      container.innerHTML = `
        <div class="logs-empty">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
          </svg>
          <p>No errors or warnings</p>
        </div>`;
    } else {
      container.innerHTML = `
        <div class="logs-empty">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
          </svg>
          <p>No logs found</p>
        </div>`;
    }
    return;
  }

  container.innerHTML = logs.map(log => renderLogEntry(log)).join('');
}

function appendLogs(selector, logs) {
  const container = $(selector);
  if (!container || !logs || logs.length === 0) return;

  // If there's an empty state, clear it first
  const emptyState = container.querySelector('.logs-empty');
  if (emptyState) {
    container.innerHTML = '';
  }

  const html = logs.map(log => renderLogEntry(log)).join('');
  container.insertAdjacentHTML('beforeend', html);
}

function renderLogEntry(log) {
  const time = new Date(log.timestamp).toLocaleString();
  const level = log.level || 'info';
  const category = log.category || '';
  const service = log.service || '';
  const message = escapeHtml(log.message || '');
  const details = log.details ? escapeHtml(log.details) : '';

  // Level icons
  const levelIcons = {
    error: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
    warn: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>',
    info: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
    debug: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>'
  };

  // Category labels
  const categoryLabels = {
    check: 'Check',
    email: 'Email',
    security: 'Security',
    system: 'System',
    schedule: 'Scheduler'
  };

  const levelIcon = levelIcons[level] || levelIcons.info;
  const categoryLabel = categoryLabels[category] || category;

  // Escape details for use in data attribute

  return `
    <div class="log-entry ${level}" data-action="show-log" data-log='${JSON.stringify({ time, level, category: categoryLabel, service, message: log.message || '', details: log.details || '' }).replace(/'/g, "&#39;").replace(/"/g, "&quot;")}'>
      <span class="log-time">${time}</span>
      <span class="log-badge level-${level}">${levelIcon}${level.toUpperCase()}</span>
      ${category ? `<span class="log-badge category">${categoryLabel}</span>` : ''}
      ${service ? `<span class="log-service-name">${escapeHtml(service)}</span>` : ''}
      <span class="log-message">${message}</span>
      ${details ? `<span class="log-details">${details}</span>` : ''}
    </div>`;
}

function showLogDetails(el) {
  try {
    const data = JSON.parse(el.dataset.log.replace(/&#39;/g, "'"));

    // Level icons for modal
    const levelIcons = {
      error: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
      warn: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>',
      info: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
      debug: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>'
    };

    const levelIcon = levelIcons[data.level] || levelIcons.info;

    const modal = document.createElement('div');
    modal.className = 'log-detail-modal';
    modal.innerHTML = `
      <div class="log-detail-content">
        <div class="log-detail-header">
          <div class="log-detail-level level-${data.level}">
            ${levelIcon}
            <span>${data.level.toUpperCase()}</span>
          </div>
          <button class="log-detail-close" data-action="close-modal">
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>
        <div class="log-detail-body">
          <div class="log-detail-row">
            <span class="log-detail-label">Time</span>
            <span class="log-detail-value">${escapeHtml(data.time)}</span>
          </div>
          ${data.category ? `<div class="log-detail-row">
            <span class="log-detail-label">Category</span>
            <span class="log-detail-value">${escapeHtml(data.category)}</span>
          </div>` : ''}
          ${data.service ? `<div class="log-detail-row">
            <span class="log-detail-label">Service</span>
            <span class="log-detail-value">${escapeHtml(data.service)}</span>
          </div>` : ''}
          <div class="log-detail-row">
            <span class="log-detail-label">Message</span>
            <span class="log-detail-value">${escapeHtml(data.message)}</span>
          </div>
          ${data.details ? `<div class="log-detail-row">
            <span class="log-detail-label">Details</span>
            <pre class="log-detail-details">${escapeHtml(data.details)}</pre>
          </div>` : ''}
        </div>
      </div>
    `;

    modal.querySelector('[data-action="close-modal"]').addEventListener('click', () => modal.remove());
    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);
  } catch (err) {
    console.error('Failed to show log details:', err);
  }
}

function renderLogsEmpty(selector) {
  const container = $(selector);
  if (!container) return;
  container.innerHTML = `
    <div class="logs-empty">
      <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
      </svg>
      <p>No logs found</p>
    </div>`;
}

async function refreshLogs(silent = false) {
  const btn = $('#refreshLogsBtn');
  if (btn) btn.classList.add('loading');

  try {
    logsOffset = 0;
    const [statsResult, logsResult] = await Promise.allSettled([loadLogStats(), loadLogs(false)]);
    const anyFailed = statsResult.status === 'rejected' || logsResult.status === 'rejected';
    if (anyFailed) {
      showToast('Failed to refresh logs', 'error');
    } else if (!silent) {
      showToast('Logs refreshed');
    }
  } catch (e) {
    showToast('Failed to refresh logs', 'error');
  } finally {
    if (btn) btn.classList.remove('loading');
  }
}

async function applyLogFilters() {
  const levelSelect = $('#logLevelFilter');
  const categorySelect = $('#logCategoryFilter');
  const serviceSelect = $('#logServiceFilter');

  currentLogFilters.level = levelSelect ? levelSelect.value : '';
  currentLogFilters.category = categorySelect ? categorySelect.value : '';
  currentLogFilters.service = serviceSelect ? serviceSelect.value : '';

  logsOffset = 0;
  await loadLogs(false);
}

async function loadMoreLogs() {
  logsOffset += LOGS_LIMIT;
  await loadLogs(true);
}

async function clearLogs() {
  if (!confirm('Are you sure you want to clear all logs? This cannot be undone.')) {
    return;
  }

  try {
    const res = await j('/api/admin/logs', {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ days: 0 })
    });
    if (res && res.success) {
      await refreshLogs(true);
      showToast('Logs cleared successfully');
    } else {
      showToast('Failed to clear logs', 'error');
    }
  } catch (err) {
    console.error('[Logs] Failed to clear logs:', err);
    showToast('Failed to clear logs', 'error');
  }
}

async function populateServiceFilter() {
  const serviceSelect = $('#logServiceFilter');
  if (!serviceSelect) return;

  try {
    const services = await j('/api/admin/services', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    if (Array.isArray(services)) {
      // Clear existing options except the first "All Services" option
      while (serviceSelect.options.length > 1) {
        serviceSelect.remove(1);
      }
      // Add service options
      services.forEach(svc => {
        const opt = document.createElement('option');
        opt.value = svc.key;
        opt.textContent = svc.name || svc.key;
        serviceSelect.appendChild(opt);
      });
    }
  } catch (err) {
    console.error('[Logs] Failed to populate service filter:', err);
  }
}

function initLogsTab() {
  // Filter change handlers
  const levelSelect = $('#logLevelFilter');
  const categorySelect = $('#logCategoryFilter');
  const serviceSelect = $('#logServiceFilter');

  if (levelSelect) {
    levelSelect.removeEventListener('change', applyLogFilters);
    levelSelect.addEventListener('change', applyLogFilters);
  }
  if (categorySelect) {
    categorySelect.removeEventListener('change', applyLogFilters);
    categorySelect.addEventListener('change', applyLogFilters);
  }
  if (serviceSelect) {
    serviceSelect.removeEventListener('change', applyLogFilters);
    serviceSelect.addEventListener('change', applyLogFilters);
  }

  // Refresh button
  const refreshBtn = $('#refreshLogsBtn');
  if (refreshBtn) {
    // Remove existing listener to prevent duplicates if init is called multiple times
    refreshBtn.removeEventListener('click', refreshLogs);
    refreshBtn.addEventListener('click', refreshLogs);
  }

  // Clear logs button
  const clearBtn = $('#clearLogsBtn');
  if (clearBtn) {
    clearBtn.removeEventListener('click', clearLogs);
    clearBtn.addEventListener('click', clearLogs);
  }

  // Load more button
  const loadMoreBtn = $('#loadMoreLogs');
  if (loadMoreBtn) {
    loadMoreBtn.removeEventListener('click', loadMoreLogs);
    loadMoreBtn.addEventListener('click', loadMoreLogs);
  }

  // Load data when Logs tab is clicked
  const logsTab = document.querySelector('[data-tab="logs"]');
  if (logsTab) {
    // Ensure we don't attach multiple listeners
    // Create a named handler or check if already attached?
    // Easiest is to just attach it, assuming initLogsTab is called once or on full reload.
    // But to be safe against double-init:
    if (!logsTab._logsInit) {
      logsTab.addEventListener('click', async () => {
        await populateServiceFilter();
        await refreshLogs();
      });
      logsTab._logsInit = true;
    }
  }
}

// Notification provider selector
function initNotificationSelector() {
  const selector = document.querySelector('.notification-selector');
  if (!selector) return;
  
  selector.addEventListener('click', (e) => {
    const btn = e.target.closest('.notification-option');
    if (!btn || btn.disabled) return;
    
    const provider = btn.dataset.provider;
    
    // Update button states
    $$('.notification-option').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    
    // Show corresponding panel
    $$('.notification-panel').forEach(p => p.classList.remove('active'));
    const panel = $(`.notification-panel[data-provider="${provider}"]`);
    if (panel) panel.classList.add('active');
  });
}

// Initialize logs tab on DOMContentLoaded