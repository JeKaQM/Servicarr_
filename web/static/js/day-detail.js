
async function openDayDetail(serviceKey, dateStr) {
  const dialog = $('#dayDetailDialog');
  if (!dialog) return;

  // Find service name
  const svc = servicesData.find(s => s.key === serviceKey);
  const serviceName = svc ? (svc.name || svc.key) : serviceKey;

  const dateObj = new Date(dateStr);
  const formattedDate = dateObj.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });

  $('#dayDetailTitle').textContent = serviceName + ' — ' + formattedDate;

  const hoursContainer = $('#dayDetailHours');
  const eventsContainer = $('#dayDetailEvents');
  hoursContainer.innerHTML = '<div class="dd-loading">Loading hourly dataâ€¦</div>';
  eventsContainer.innerHTML = '';

  dialog.showModal();

  try {
    const data = await j(`/api/metrics/day-detail?key=${encodeURIComponent(serviceKey)}&date=${dateStr}`);
    renderDayDetailHours(data.hours, hoursContainer);
    renderDayDetailEvents(data.down_events, eventsContainer);
  } catch (e) {
    hoursContainer.innerHTML = '<div class="dd-loading" style="color:var(--down)">Failed to load data</div>';
    console.error('Day detail error', e);
  }
}

function renderDayDetailHours(hours, container) {
  container.innerHTML = '';

  // Summary row
  const totalChecks = hours.reduce((s, h) => s + h.checks, 0);
  const upHours = hours.filter(h => h.uptime >= 100).length;
  const dataHours = hours.filter(h => h.checks > 0).length;

  let dayUptime = 100;
  if (dataHours > 0) {
    const weighted = hours.filter(h => h.checks > 0);
    dayUptime = weighted.reduce((s, h) => s + h.uptime, 0) / weighted.length;
  }

  const summaryEl = document.createElement('div');
  summaryEl.className = 'dd-summary';
  summaryEl.innerHTML =
    '<div class="dd-stat"><span class="dd-stat-val">' + (dayUptime >= 100 ? '100%' : dayUptime.toFixed(2) + '%') + '</span><span class="dd-stat-lbl">Day Uptime</span></div>' +
    '<div class="dd-stat"><span class="dd-stat-val">' + upHours + '/' + dataHours + '</span><span class="dd-stat-lbl">Hours Clean</span></div>' +
    '<div class="dd-stat"><span class="dd-stat-val">' + totalChecks + '</span><span class="dd-stat-lbl">Total Checks</span></div>';
  container.appendChild(summaryEl);

  // Hour bar grid
  const grid = document.createElement('div');
  grid.className = 'dd-hour-grid';

  hours.forEach((h, i) => {
    const col = document.createElement('div');
    col.className = 'dd-hour-col';

    const bar = document.createElement('div');
    bar.className = 'dd-hour-bar';

    if (h.checks === 0 || h.uptime < 0) {
      bar.classList.add('dd-no-data');
    } else if (h.uptime >= 100) {
      bar.classList.add('dd-up');
    } else if (h.uptime >= 50) {
      bar.classList.add('dd-degraded');
    } else {
      bar.classList.add('dd-down');
    }

    // Tooltip
    const hourLabel = String(i).padStart(2, '0') + ':00';
    let tip = hourLabel;
    if (h.checks === 0) {
      tip += '\nNo data';
    } else {
      tip += '\n' + h.uptime.toFixed(1) + '% uptime';
      tip += '\n' + h.checks + ' checks';
      if (h.avg_ms != null) tip += '\n' + Math.round(h.avg_ms) + 'ms avg';
    }
    bar.title = tip;

    const lbl = document.createElement('span');
    lbl.className = 'dd-hour-lbl';
    // Show labels for every 3rd hour + last
    lbl.textContent = (i % 3 === 0 || i === 23) ? hourLabel : '';

    col.appendChild(bar);
    col.appendChild(lbl);
    grid.appendChild(col);
  });

  container.appendChild(grid);
}

function renderDayDetailEvents(events, container) {
  if (!events || events.length === 0) {
    container.innerHTML = '<div class="dd-no-events"><span class="dd-check-icon">âœ“</span> No downtime events recorded this day</div>';
    return;
  }

  const header = document.createElement('div');
  header.className = 'dd-events-header';
  header.textContent = 'Downtime Events (' + events.length + ')';
  container.appendChild(header);

  const list = document.createElement('div');
  list.className = 'dd-events-list';

  events.forEach(ev => {
    const row = document.createElement('div');
    row.className = 'dd-event-row';

    const ts = new Date(ev.time);
    const timeStr = ts.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

    let detail = '';
    if (ev.http_status) detail += 'HTTP ' + ev.http_status;
    if (ev.error) detail += (detail ? ' — ' : '') + ev.error;
    if (ev.latency_ms != null) detail += (detail ? ' â€¢ ' : '') + ev.latency_ms + 'ms';
    if (!detail) detail = 'Service unreachable';

    row.innerHTML =
      '<span class="dd-event-time">' + timeStr + '</span>' +
      '<span class="dd-event-dot"></span>' +
      '<span class="dd-event-detail">' + escapeHtml(detail) + '</span>';

    list.appendChild(row);
  });

  container.appendChild(list);
}

// Close day detail dialog
document.addEventListener('DOMContentLoaded', () => {
  const closeBtn = $('#closeDayDetail');
  if (closeBtn) {
    closeBtn.addEventListener('click', () => {
      const d = $('#dayDetailDialog');
      if (d) d.close();
    });
  }
  // Close on backdrop click
  const dialog = $('#dayDetailDialog');
  if (dialog) {
    dialog.addEventListener('click', (e) => {
      if (e.target === dialog) dialog.close();
    });
  }
});

async function refresh() {
  const suspendDashboard = shouldSuspendDashboardRefresh();

  try {
    const live = await j('/api/check');
    $('#updated').textContent = new Date(live.t).toLocaleString();

    // Update cards dynamically based on services returned from API
    if (live.status) {
      latestLiveStatus = live.status;  // cache for matrix view

      Object.keys(live.status).forEach(key => {
        const cardEl = document.getElementById(`card-${key}`);
        if (cardEl) {
          updCard(`card-${key}`, live.status[key] || {});
        }
      });

      // Update global health dot & summary bar
      updateHealthDot(live.status);
      updateStatusSummary(live.status);

      // Re-render matrix if active
      if (currentView === 'matrix') renderMatrix();
    }
  } catch (e) {
    console.error('live check failed', e);
  }

  // Resources (Glances)
  if (!suspendDashboard) {
    refreshResources();
  }

  try {
    if (suspendDashboard) {
      return;
    }

    const metrics = await j(`/api/metrics?days=${DAYS}`);
    $('#window').textContent = `Last ${DAYS} days`;

    renderIncidents(metrics.downs || []);
    renderUptimeBars(metrics, DAYS);

    // Fetch 24h stats for the service cards
    const stats24h = await j('/api/metrics?hours=24');
    updateServiceStats(stats24h);
  } catch (e) {
    // Rate limited — keep previous bars, don't wipe to empty
    if (e.status === 429) return;
    // Genuine error — render with no data
    if (!suspendDashboard) {
      renderUptimeBars(null, DAYS);
    }
  }

}

async function doLoginFlow() {
  const dlg = document.getElementById('loginModal');
  const err = $('#loginError', dlg);
  err.classList.add('hidden');
  err.textContent = '';

  // Clear any previous input
  $('#u', dlg).value = '';
  $('#p', dlg).value = '';

  dlg.showModal();
}

async function submitLogin() {
  const dlg = document.getElementById('loginModal');
  const u = $('#u', dlg).value.trim();
  const p = $('#p', dlg).value;

  if (!u || !p) {
    const el = $('#loginError', dlg);
    el.textContent = 'Username and password are required';
    el.classList.remove('hidden');
    return;
  }

  // Disable form while submitting to prevent double submission
  const submitBtn = $('#doLogin');
  submitBtn.disabled = true;
  submitBtn.textContent = 'Signing in...';

  try {
    const csrfToken = getCsrf();

    const result = await j('/api/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrfToken
      },
      body: JSON.stringify({ username: u, password: p })
    });

    dlg.close();
    // Reload page to get server-rendered admin elements
    window.location.reload();
  } catch (err) {
    const el = $('#loginError', dlg);

    if (err.status === 403) {
      el.textContent = 'Access denied - too many failed attempts. Try again later.';
    } else if (err.status === 401) {
      el.textContent = 'Invalid username or password';
    } else if (err.name === 'AbortError') {
      el.textContent = 'Request timeout - check your connection';
    } else {
      el.textContent = err.message || 'Login failed. Please try again.';
    }

    el.classList.remove('hidden');
    submitBtn.disabled = false;
    submitBtn.textContent = 'Sign In';
  }
}

async function logout() {
  try {
    await j('/api/logout', { method: 'POST' });
  } catch (_) { }
  // Reload page to remove server-rendered admin elements
  window.location.reload();
}