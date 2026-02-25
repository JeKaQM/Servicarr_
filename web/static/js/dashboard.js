function updCard(id, data) {
  const el = document.getElementById(id);
  if (!el) {
    console.error('Card element not found:', id);
    return;
  }

  const pill = $('.pill', el);
  const k = $('.kpi', el);
  const h = $('.kpi-status', el); // More specific selector for status label
  const toggle = $('.monitorToggle', el);

  if (!pill || !k || !h) {
    console.error('Required elements not found in card:', id);
    return;
  }

  // Set checkbox state based on disabled flag from server
  if (toggle) {
    toggle.checked = !data.disabled;
  }

  if (data.disabled) {
    pill.textContent = 'DISABLED';
    pill.className = 'pill warn';
    el.classList.remove('status-up', 'status-down', 'status-degraded');
    el.classList.add('status-disabled');
    k.textContent = '—';
    h.textContent = 'Monitoring disabled';
    return;
  }

  if (data.degraded) {
    pill.textContent = 'DEGRADED';
  } else {
    pill.textContent = data.ok ? 'UP' : 'DOWN';
  }
  pill.className = cls(data.ok, data.status, data.degraded);

  // Update the left accent bar
  el.classList.remove('status-up', 'status-down', 'status-degraded', 'status-disabled');
  if (data.degraded)  el.classList.add('status-degraded');
  else if (data.ok)   el.classList.add('status-up');
  else                el.classList.add('status-down');
  k.textContent = fmtMs(data.ms);
  
  // Show appropriate status message based on check type
  const checkType = (data.check_type || 'http').toLowerCase();
  if (checkType === 'always_up') {
    h.textContent = data.ok ? 'Always up' : 'Down';
  } else if (checkType === 'tcp') {
    h.textContent = data.ok ? 'Port open' : 'Connection refused';
  } else if (checkType === 'dns') {
    h.textContent = data.ok ? 'DNS resolved' : 'Lookup failed';
  } else {
    // HTTP/HTTPS
    if (typeof data.status === 'number' && data.status > 0) {
      h.textContent = 'HTTP ' + data.status;
    } else if (data.status === 0 && !data.ok) {
      h.textContent = 'No response';
    } else {
      h.textContent = '—';
    }
  }

  // Update last check time
  const lastCheckEl = $(`#last-check-${id.split('-').pop()}`);
  if (lastCheckEl) {
    const now = new Date();
    lastCheckEl.textContent = now.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  }
}

async function toggleMonitoring(card, enabled) {
  const key = card.getAttribute('data-key');
  try {
    await j('/api/admin/toggle-monitoring', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ service: key, enable: enabled })
    });
    showToast(`Monitoring ${enabled ? 'enabled' : 'disabled'} for ${key}`);
    await refresh();
  } catch (err) {
    console.error('toggle failed', err);
    showToast('Failed to toggle monitoring', 'error');
  }
}



function renderIncidents(items) {
  const list = $('#incidents');
  const VISIBLE_LIMIT = 5;

  if (!items?.length) {
    list.innerHTML = '<li class="no-incidents"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="#22c55e" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg> No incidents in last 24h</li>';
    return;
  }

  const rendered = items.map((i, idx) => {
    const rawTs = i.taken_at || i.time || '';
    let ts = '';
    if (rawTs) {
      const d = new Date(rawTs);
      ts = Number.isNaN(d.getTime()) ? String(rawTs) : d.toLocaleString();
    }

    const svcRaw = i.service_name || i.service_key || '';
    const svc = escapeHtml(svcRaw);
    const statusCode = Number(i.http_status) || 0;
    const latency = i.latency_ms ?? i.ping;
    let err = '';
    if (typeof i.error === 'string') err = i.error;
    else if (typeof i.msg === 'string') err = i.msg;
    if (err) {
      err = err.trim();
    }

    const parts = [];
    const checkType = String(i.check_type || '').trim();
    if (checkType && checkType.toLowerCase() !== 'http') {
      parts.push(checkType.toUpperCase());
    }
    if (statusCode > 0) parts.push(`HTTP ${statusCode}`);
    if (latency && Number(latency) > 0) parts.push(`${Number(latency)}ms`);
    if (err) parts.push(err);

    const detail = parts.length ? parts.join(' | ') : 'down';
    const summary = detail.length > 90 ? detail.slice(0, 87) + '...' : detail;

    const payload = JSON.stringify({
      time: ts || rawTs,
      service: svcRaw,
      check_type: checkType || 'http',
      status: statusCode > 0 ? `HTTP ${statusCode}` : 'No response',
      latency: latency && Number(latency) > 0 ? `${Number(latency)}ms` : '',
      detail,
      error: err || ''
    }).replace(/'/g, '&#39;').replace(/"/g, '&quot;');

    const hiddenClass = idx >= VISIBLE_LIMIT ? ' incident-hidden' : '';
    const hiddenAttr = idx >= VISIBLE_LIMIT ? ' data-hidden="true"' : '';

    return `
      <li class="incident-item${hiddenClass}"${hiddenAttr} data-incident="${payload}">
        <span class="dot"></span>
        <div class="incident-content">
          <span class="incident-time">${escapeHtml(ts)}</span>
          <span class="incident-detail">${svc} (${escapeHtml(summary)})</span>
        </div>
        <span class="incident-action">Details <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg></span>
      </li>
    `;
  });

  // Add "View more" button if there are hidden items
  const hiddenCount = items.length - VISIBLE_LIMIT;
  if (hiddenCount > 0) {
    rendered.push(`
      <li class="incidents-toggle" id="incidentsToggle">
        <button class="btn ghost incidents-toggle-btn" type="button">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          View ${hiddenCount} more incident${hiddenCount > 1 ? 's' : ''}
        </button>
      </li>
    `);
  }

  list.innerHTML = rendered.join('');

  // Wire up toggle button
  const toggleBtn = $('#incidentsToggle button');
  if (toggleBtn) {
    toggleBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const hidden = $$('#incidents .incident-item[data-hidden]');
      const isExpanded = toggleBtn.dataset.expanded === 'true';
      hidden.forEach(el => {
        if (isExpanded) {
          el.classList.add('incident-hidden');
        } else {
          el.classList.remove('incident-hidden');
        }
      });
      toggleBtn.dataset.expanded = isExpanded ? 'false' : 'true';
      toggleBtn.innerHTML = isExpanded
        ? `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg> View ${hiddenCount} more incident${hiddenCount > 1 ? 's' : ''}`
        : `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"/></svg> Show less`;
    });
  }

  $$('#incidents .incident-item').forEach(item => {
    item.addEventListener('click', () => showIncidentDetails(item));
  });
}

function updateServiceStats(metrics) {
  // Get service keys from the servicesData array (dynamic)
  const services = servicesData.map(s => s.key);

  services.forEach(key => {
    const uptimeEl = $(`#uptime-24h-${key}`);
    const avgResponseEl = $(`#avg-response-${key}`);

    if (uptimeEl && metrics.overall) {
      const uptime = metrics.overall[key] || 0;
      uptimeEl.textContent = `${uptime.toFixed(1)}%`;
      // Green only for 100%, orange for <100%, red for <50%
      uptimeEl.className = 'stat-value ' + (uptime >= 100 ? 'good' : uptime >= 50 ? 'warning' : 'bad');
    }

    if (avgResponseEl && metrics.series && metrics.series[key]) {
      const data = metrics.series[key];
      let totalMs = 0;
      let count = 0;

      data.forEach(point => {
        if (point.avg_ms && point.avg_ms > 0) {
          totalMs += point.avg_ms;
          count++;
        }
      });

      if (count > 0) {
        const avgMs = totalMs / count;
        avgResponseEl.textContent = `${Math.round(avgMs)}ms`;
        avgResponseEl.className = 'stat-value ' + (avgMs < 100 ? 'good' : avgMs < 500 ? 'warning' : 'bad');
      } else {
        avgResponseEl.textContent = '—';
        avgResponseEl.className = 'stat-value';
      }
    }
  });
}

function renderUptimeBars(metrics, days) {
  const daysToShow = days || DAYS;
  // Get service keys from the servicesData array (dynamic)
  const services = servicesData.map(s => s.key);
  const now = new Date();
  const daysAgo = now.getTime() - (daysToShow * 24 * 60 * 60 * 1000);

  // Find the earliest date with actual data across all services
  let earliestDate = null;
  if (metrics && metrics.series) {
    services.forEach(key => {
      const data = metrics.series[key] || [];
      data.forEach(point => {
        const dayStr = point.day || point.hour?.substr(0, 10);
        if (dayStr && point.uptime !== null && point.uptime !== undefined) {
          const d = new Date(dayStr);
          if (!earliestDate || d < earliestDate) {
            earliestDate = d;
          }
        }
      });
    });
  }

  // Update global timestamp with actual tracking start date
  const globalTimestamp = $('#timestamp-global');
  if (globalTimestamp) {
    if (earliestDate) {
      const startDate = earliestDate.toLocaleDateString();
      globalTimestamp.textContent = `Tracking since ${startDate} • Hover over blocks for details`;
    } else {
      globalTimestamp.textContent = `No data yet • Hover over blocks for details`;
    }
  }

  services.forEach(key => {
    const bar = $(`#uptime-bar-${key}`);
    const uptimePercent = $(`#uptime-${key}`);

    if (!bar) return;

    // Add data attribute for CSS styling based on day count
    bar.setAttribute('data-days', daysToShow);

    const data = (metrics && metrics.series) ? metrics.series[key] || [] : [];

    // Calculate average uptime from the daily/hourly data points (not the overall which dilutes short outages)
    let avgUptime = 0;
    if (data.length > 0) {
      const validPoints = data.filter(p => p.uptime !== null && p.uptime !== undefined);
      if (validPoints.length > 0) {
        avgUptime = validPoints.reduce((sum, p) => sum + p.uptime, 0) / validPoints.length;
      }
    }

    // Update uptime percentage
    if (uptimePercent) {
      if (data.length === 0) {
        uptimePercent.textContent = 'N/A';
        uptimePercent.style.color = 'var(--text-dim)';
      } else {
        // Use 2 decimal places to avoid rounding 99.99% to 100%
        // But show "100%" without decimals if truly 100%
        if (avgUptime >= 100) {
          uptimePercent.textContent = '100%';
        } else {
          uptimePercent.textContent = `${avgUptime.toFixed(2)}%`;
        }
        // Green only for 100%, orange for <100%, red for <50%
        uptimePercent.style.color = avgUptime >= 100 ? 'var(--ok)' : avgUptime >= 50 ? 'var(--warn)' : 'var(--down)';
      }
    }

    // Clear existing blocks
    bar.innerHTML = '';

    // Create blocks for each day - always show DAYS blocks
    // If we have data, use it; otherwise show gray "no data" blocks
    const blocks = [];

    if (data.length > 0) {
      // Fill in missing days with null data
      const dataMap = {};
      data.forEach(point => {
        // API returns 'day' field for daily aggregation
        const dayKey = point.day || point.hour?.substr(0, 10);
        if (dayKey) {
          dataMap[dayKey] = point;
        }
      });

      // Create all days
      for (let i = daysToShow - 1; i >= 0; i--) {
        const dayTime = new Date(now.getTime() - (i * 24 * 60 * 60 * 1000));
        const dayBin = dayTime.toISOString().substr(0, 10);
        blocks.push(dataMap[dayBin] || { day: dayBin, uptime: null });
      }
    } else {
      // No data yet - create empty blocks
      for (let i = daysToShow - 1; i >= 0; i--) {
        const dayTime = new Date(now.getTime() - (i * 24 * 60 * 60 * 1000));
        const dayBin = dayTime.toISOString().substr(0, 10);
        blocks.push({ day: dayBin, uptime: null });
      }
    }

    blocks.forEach((point) => {
      const block = document.createElement('div');
      block.className = 'uptime-block';

      const uptime = point.uptime;
      const dayDate = new Date(point.day);
      const formattedDate = dayDate.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: daysToShow > 90 ? 'numeric' : undefined
      });

      let tooltipText = '';
      if (uptime === null || uptime === undefined) {
        block.classList.add('unknown');
        tooltipText = `${formattedDate}\nNo data available`;
      } else if (uptime >= 100) {
        // 100% uptime = green
        block.classList.add('up');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n✓ Fully operational`;
      } else if (uptime >= 50) {
        // 50-99% uptime = orange (partial outage)
        block.classList.add('degraded');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n⚠ Partial outage`;
      } else {
        // Below 50% = red (major outage)
        block.classList.add('down');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n✗ Major outage`;
      }

      block.title = tooltipText;
      block.setAttribute('data-tooltip', tooltipText);
      block.setAttribute('data-day', point.day);
      block.setAttribute('data-service-key', key);
      block.style.cursor = 'pointer';

      // Click to open hourly detail
      block.addEventListener('click', (e) => {
        e.stopPropagation();
        openDayDetail(key, point.day);
      });

      // Add mobile-friendly touch feedback
      block.addEventListener('touchstart', (e) => {
        // Show a quick visual feedback on touch
        block.style.transition = 'transform 0.1s';

        // Create temporary tooltip for mobile
        const isMobile = window.innerWidth <= 768;
        if (isMobile && tooltipText) {
          showMobileTooltip(block, tooltipText, e.touches[0]);
        }
      }, { passive: true });

      block.addEventListener('touchend', () => {
        block.style.transition = '';
      }, { passive: true });

      bar.appendChild(block);
    });
  });
}

// Mobile tooltip function for uptime blocks
let tooltipTimeout;
function showMobileTooltip(element, text, touch) {
  // Remove any existing tooltip
  const existingTooltip = document.querySelector('.mobile-tooltip');
  if (existingTooltip) {
    existingTooltip.remove();
  }

  clearTimeout(tooltipTimeout);

  const tooltip = document.createElement('div');
  tooltip.className = 'mobile-tooltip';
  tooltip.textContent = text.replace(/\n/g, ' • ');
  tooltip.style.cssText = `
    position: fixed;
    background: rgba(0, 0, 0, 0.9);
    color: white;
    padding: 8px 12px;
    border-radius: 6px;
    font-size: 12px;
    z-index: 10000;
    pointer-events: none;
    max-width: 80vw;
    text-align: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    left: 50%;
    top: ${touch ? touch.clientY - 60 : 100}px;
    transform: translateX(-50%);
    animation: fadeIn 0.2s ease-in;
  `;

  document.body.appendChild(tooltip);

  tooltipTimeout = setTimeout(() => {
    tooltip.style.animation = 'fadeOut 0.2s ease-out';
    setTimeout(() => tooltip.remove(), 200);
  }, 2000);
}

// ============ Incident Details Modal ============
function showIncidentDetails(el) {
  try {
    const data = JSON.parse(el.dataset.incident.replace(/&#39;/g, "'"));
    const levelIcon = '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>';

    const modal = document.createElement('div');
    modal.className = 'log-detail-modal';
    modal.innerHTML = `
      <div class="log-detail-content">
        <div class="log-detail-header">
          <div class="log-detail-level level-error">
            ${levelIcon}
            <span>INCIDENT</span>
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
            <span class="log-detail-value">${escapeHtml(data.time || '')}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Service</span>
            <span class="log-detail-value">${escapeHtml(data.service || '')}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Check</span>
            <span class="log-detail-value">${escapeHtml((data.check_type || 'http').toUpperCase())}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Status</span>
            <span class="log-detail-value">${escapeHtml(data.status || 'Down')}</span>
          </div>
          ${data.latency ? `<div class="log-detail-row">
            <span class="log-detail-label">Latency</span>
            <span class="log-detail-value">${escapeHtml(data.latency)}</span>
          </div>` : ''}
          ${data.error ? `<div class="log-detail-row">
            <span class="log-detail-label">Error</span>
            <span class="log-detail-value">${escapeHtml(data.error)}</span>
          </div>` : ''}
          <div class="log-detail-row">
            <span class="log-detail-label">Details</span>
            <span class="log-detail-value">${escapeHtml(data.detail || '')}</span>
          </div>
        </div>
      </div>
    `;

    modal.querySelector('[data-action="close-modal"]').addEventListener('click', () => modal.remove());
    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);
  } catch (err) {
    console.error('Failed to show incident details:', err);
  }
}

// ============ Day Detail Popup ============