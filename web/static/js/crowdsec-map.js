// Interactive CrowdSec source geography. A real Natural Earth basemap sits
// below this transparent canvas; the canvas only draws aggregated data marks
// and optional destination arcs. Rendering stops when nothing is animating.

var crowdsecMapCanvas = null;
var crowdsecMapObserver = null;
var crowdsecMapAnim = null;
var crowdsecMapAlerts = [];
var crowdsecMapHome = null;
var crowdsecMapStrikes = [];
var crowdsecMapKeyboardIndex = -1;
var crowdsecMapHover = null;
var crowdsecMapSelectedKey = '';
var crowdsecMapFilter = 'all';
var crowdsecMapLastStateText = '';
var crowdsecMapClusterCacheAlerts = null;
var crowdsecMapClusterCache = Object.create(null);

function crowdsecProject(lat, lng) {
  const safeLat = Math.max(-90, Math.min(90, Number(lat) || 0));
  const safeLng = Math.max(-180, Math.min(180, Number(lng) || 0));
  return { x: (safeLng + 180) / 360, y: (90 - safeLat) / 180 };
}

function crowdsecMapValidCoordinates(alert) {
  if (!alert || alert.latitude == null || alert.longitude == null) return false;
  const lat = Number(alert.latitude);
  const lng = Number(alert.longitude);
  return Number.isFinite(lat) && Number.isFinite(lng) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180;
}

function crowdsecMapIsVisible() {
  const tab = $('#tab-crowdsec');
  return !!(tab && tab.classList.contains('active') && !document.hidden);
}

function crowdsecMapGeolocatedAlerts() {
  const alerts = typeof crowdsecAllAlerts === 'undefined' ? [] : crowdsecAllAlerts;
  return (alerts || []).filter(alert => {
    if (!crowdsecMapValidCoordinates(alert)) return false;
    if (crowdsecMapFilter === 'actioned') return !!alert.has_decision;
    if (crowdsecMapFilter === 'observed') return !alert.has_decision;
    return true;
  });
}

function crowdsecMapClusterKey(lat, lng) {
  return `${Number(lat).toFixed(1)},${Number(lng).toFixed(1)}`;
}

function crowdsecMostFrequent(values, fallback) {
  let best = fallback || '';
  let bestCount = -1;
  Object.keys(values).sort().forEach(label => {
    if (values[label] > bestCount) {
      best = label;
      bestCount = values[label];
    }
  });
  return best;
}

function crowdsecMapAggregateAlerts(alerts) {
  const clusters = new Map();
  (alerts || []).forEach(alert => {
    if (!crowdsecMapValidCoordinates(alert)) return;
    const lat = Number(alert.latitude);
    const lng = Number(alert.longitude);
    const key = crowdsecMapClusterKey(lat, lng);
    let cluster = clusters.get(key);
    if (!cluster) {
      cluster = {
        key,
        latitude: lat,
        longitude: lng,
        count: 0,
        reportedEvents: 0,
        actioned: 0,
        observed: 0,
        alertIDs: [],
        sources: new Set(),
        countries: {},
        scenarios: {},
        networks: {},
        latestAt: '',
        latestAlert: null
      };
      clusters.set(key, cluster);
    }
    cluster.count++;
    cluster.reportedEvents += Math.max(0, Number(alert.events_count) || 0);
    if (alert.has_decision) cluster.actioned++; else cluster.observed++;
    if (alert.alert_id != null) cluster.alertIDs.push(String(alert.alert_id));
    if (alert.source_value) cluster.sources.add(String(alert.source_value));
    const country = String(alert.country || '??').toUpperCase();
    const scenario = String(alert.scenario || 'unknown');
    const network = String(alert.as_name || alert.as_number || 'unknown');
    cluster.countries[country] = (cluster.countries[country] || 0) + 1;
    cluster.scenarios[scenario] = (cluster.scenarios[scenario] || 0) + 1;
    cluster.networks[network] = (cluster.networks[network] || 0) + 1;
    const createdAt = String(alert.created_at || '');
    if (!cluster.latestAlert || createdAt > cluster.latestAt) {
      cluster.latestAt = createdAt;
      cluster.latestAlert = alert;
    }
  });

  return Array.from(clusters.values()).map(cluster => {
    cluster.country = crowdsecMostFrequent(cluster.countries, '??');
    cluster.scenario = crowdsecMostFrequent(cluster.scenarios, 'unknown');
    cluster.network = crowdsecMostFrequent(cluster.networks, 'unknown');
    return cluster;
  }).sort((left, right) => right.count - left.count || left.key.localeCompare(right.key));
}

function crowdsecMapClusters() {
  const alerts = typeof crowdsecAllAlerts === 'undefined' ? [] : crowdsecAllAlerts;
  if (crowdsecMapClusterCacheAlerts !== alerts) {
    crowdsecMapClusterCacheAlerts = alerts;
    crowdsecMapClusterCache = Object.create(null);
  }
  if (!Object.prototype.hasOwnProperty.call(crowdsecMapClusterCache, crowdsecMapFilter)) {
    crowdsecMapClusterCache[crowdsecMapFilter] = crowdsecMapAggregateAlerts(crowdsecMapGeolocatedAlerts());
  }
  return crowdsecMapClusterCache[crowdsecMapFilter];
}

function crowdsecBubbleRadius(count, maxCount) {
  if (maxCount <= 1) return 5;
  return 4.5 + Math.sqrt(Math.max(1, count) / maxCount) * 9.5;
}

function crowdsecMapUpdateState() {
  const state = $('#crowdsecMapState');
  if (!state) return;
  const allAlerts = typeof crowdsecAllAlerts === 'undefined' ? [] : (crowdsecAllAlerts || []);
  const mapped = crowdsecMapGeolocatedAlerts();
  const clusters = crowdsecMapClusters();
  let text;
  if (!allAlerts.length) {
    text = 'Waiting for recent CrowdSec detections';
  } else if (!mapped.length) {
    text = crowdsecMapFilter === 'all'
      ? `${allAlerts.length} recent detection${allAlerts.length === 1 ? '' : 's'} · no location data`
      : `No mapped detections match this outcome filter`;
  } else {
    const destination = crowdsecMapHome ? '' : ' · destination not set';
    text = `${mapped.length} mapped detection${mapped.length === 1 ? '' : 's'} · ${clusters.length} clustered location${clusters.length === 1 ? '' : 's'}${destination}`;
  }
  state.classList.toggle('is-empty', mapped.length === 0);
  if (text !== crowdsecMapLastStateText) {
    state.textContent = text;
    crowdsecMapLastStateText = text;
  }
}

function crowdsecMapInit() {
  const canvas = $('#crowdsecMap');
  if (!canvas || crowdsecMapCanvas === canvas) return;
  crowdsecMapCanvas = canvas;

  if (typeof ResizeObserver === 'function') {
    crowdsecMapObserver = new ResizeObserver(crowdsecMapResize);
    crowdsecMapObserver.observe(canvas);
  } else {
    window.addEventListener('resize', crowdsecMapResize);
  }

  canvas.addEventListener('mousemove', crowdsecMapOnMouseMove);
  canvas.addEventListener('mouseleave', crowdsecMapOnMouseLeave);
  canvas.addEventListener('click', crowdsecMapOnClick);
  canvas.addEventListener('keydown', crowdsecMapOnKeyDown);
  canvas.addEventListener('blur', crowdsecMapOnMouseLeave);
  $$('[data-crowdsec-map-filter]').forEach(button => {
    button.addEventListener('click', () => crowdsecMapSetFilter(button.dataset.crowdsecMapFilter));
  });

  crowdsecMapResize();
  crowdsecMapUpdateState();
  crowdsecMapUpdateInspector(null);
  crowdsecMapAlerts = (typeof crowdsecAllAlerts === 'undefined' ? [] : crowdsecAllAlerts || []).slice();
  if (crowdsecMapHome) {
    crowdsecMapGeolocatedAlerts().slice(0, 5).forEach((alert, index) => crowdsecQueueStrike(alert, index * 130));
  }
  crowdsecMapResume();
}

function crowdsecMapResume() {
  if (!crowdsecMapIsVisible()) return;
  crowdsecMapRequestDraw();
}

function crowdsecMapResize() {
  const canvas = crowdsecMapCanvas || $('#crowdsecMap');
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  if (!rect.width || !rect.height) return;
  const dpr = window.devicePixelRatio || 1;
  const width = Math.round(rect.width * dpr);
  const height = Math.round(rect.height * dpr);
  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (ctx && typeof ctx.setTransform === 'function') ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  crowdsecMapRequestDraw();
}

function crowdsecMapReducedMotion() {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function crowdsecQueueStrike(alert, delayMs) {
  if (!crowdsecMapHome || !crowdsecMapValidCoordinates(alert) || crowdsecMapReducedMotion()) return null;
  const now = typeof performance !== 'undefined' && performance.now ? performance.now() : Date.now();
  const strike = {
    from: crowdsecProject(alert.latitude, alert.longitude),
    color: alert.has_decision ? '251, 113, 133' : '251, 191, 36',
    progress: 0,
    startAt: now + Math.max(0, Number(delayMs) || 0),
    duration: 900,
    alert
  };
  crowdsecMapStrikes.push(strike);
  crowdsecMapRequestDraw();
  return strike;
}

function crowdsecMapLayout(width, height) {
  const pad = 10;
  const availableW = Math.max(1, width - pad * 2);
  const availableH = Math.max(1, height - pad * 2);
  let mapW = availableW;
  let mapH = mapW / 2;
  if (mapH > availableH) {
    mapH = availableH;
    mapW = mapH * 2;
  }
  return {
    x: (width - mapW) / 2,
    y: (height - mapH) / 2,
    w: mapW,
    h: mapH,
    dot: Math.min(3.2, Math.max(1.3, mapW / 460))
  };
}

function crowdsecMapRequestDraw() {
  if (crowdsecMapAnim != null || !crowdsecMapIsVisible()) return;
  if (typeof requestAnimationFrame !== 'function') return;
  crowdsecMapAnim = requestAnimationFrame(crowdsecMapFrame);
}

function crowdsecMapFrame(timestamp) {
  crowdsecMapAnim = null;
  if (!crowdsecMapIsVisible()) return;
  const hasActiveAnimation = crowdsecMapDraw(Number(timestamp) || 0);
  if (hasActiveAnimation) crowdsecMapRequestDraw();
}

function crowdsecMapDraw(timestamp) {
  const canvas = crowdsecMapCanvas || $('#crowdsecMap');
  if (!canvas || !canvas.getContext) return false;
  const ctx = canvas.getContext('2d');
  if (!ctx) return false;
  const dpr = window.devicePixelRatio || 1;
  const width = canvas.width / dpr;
  const height = canvas.height / dpr;
  if (!width || !height) return false;
  const layout = crowdsecMapLayout(width, height);
  ctx.clearRect(0, 0, width, height);
  crowdsecMapDrawMarkers(ctx, layout);
  crowdsecMapDrawHome(ctx, layout);
  const animating = crowdsecMapDrawStrikes(ctx, layout, timestamp);
  crowdsecMapPositionTip(layout);
  return animating;
}

function crowdsecMapDrawMarkers(ctx, layout) {
  const clusters = crowdsecMapClusters();
  const maxCount = Math.max(1, ...clusters.map(cluster => cluster.count));
  clusters.slice().reverse().forEach(cluster => {
    const point = crowdsecProject(cluster.latitude, cluster.longitude);
    const x = layout.x + point.x * layout.w;
    const y = layout.y + point.y * layout.h;
    const radius = crowdsecBubbleRadius(cluster.count, maxCount);
    const selected = cluster.key === crowdsecMapSelectedKey || (crowdsecMapHover && cluster.key === crowdsecMapHover.key);
    const actionRatio = cluster.count ? cluster.actioned / cluster.count : 0;

    if (selected) {
      ctx.beginPath();
      ctx.arc(x, y, radius + 4, 0, Math.PI * 2);
      ctx.strokeStyle = 'rgba(147, 197, 253, 0.9)';
      ctx.lineWidth = 1.5;
      ctx.stroke();
    }

    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    if (cluster.actioned > 0) {
      ctx.fillStyle = `rgba(251, 113, 133, ${0.42 + actionRatio * 0.38})`;
      ctx.fill();
    } else {
      ctx.fillStyle = 'rgba(251, 191, 36, 0.08)';
      ctx.fill();
    }
    ctx.strokeStyle = cluster.observed > 0 ? 'rgba(251, 191, 36, 0.95)' : 'rgba(251, 113, 133, 0.95)';
    ctx.lineWidth = cluster.observed > 0 && cluster.actioned === 0 ? 2 : 1.4;
    ctx.stroke();

    if (cluster.count > 1 && radius >= 8) {
      ctx.fillStyle = '#f8fafc';
      ctx.font = `700 ${Math.max(8, Math.min(11, radius * 0.78))}px system-ui, sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(String(cluster.count), x, y + 0.5);
    }
  });
}

function crowdsecMapDrawHome(ctx, layout) {
  if (!crowdsecMapHome) return;
  const home = crowdsecProject(crowdsecMapHome.lat, crowdsecMapHome.lng);
  const x = layout.x + home.x * layout.w;
  const y = layout.y + home.y * layout.h;
  ctx.beginPath();
  ctx.arc(x, y, 7, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(52, 211, 153, 0.16)';
  ctx.fill();
  ctx.beginPath();
  ctx.arc(x, y, 3.4, 0, Math.PI * 2);
  ctx.fillStyle = '#34d399';
  ctx.fill();
  ctx.strokeStyle = '#d1fae5';
  ctx.lineWidth = 1;
  ctx.stroke();
}

function crowdsecMapDrawStrikes(ctx, layout, timestamp) {
  if (!crowdsecMapHome || !crowdsecMapStrikes.length || crowdsecMapReducedMotion()) {
    crowdsecMapStrikes = [];
    return false;
  }
  const home = crowdsecProject(crowdsecMapHome.lat, crowdsecMapHome.lng);
  const hx = layout.x + home.x * layout.w;
  const hy = layout.y + home.y * layout.h;
  const keep = [];
  let active = false;
  const now = timestamp || (typeof performance !== 'undefined' && performance.now ? performance.now() : Date.now());

  crowdsecMapStrikes.forEach(strike => {
    if (now < strike.startAt) {
      keep.push(strike);
      active = true;
      return;
    }
    strike.progress = Math.min(1, (now - strike.startAt) / strike.duration);
    const point = strike.from;
    const x = layout.x + point.x * layout.w;
    const y = layout.y + point.y * layout.h;
    const mx = (x + hx) / 2;
    const my = Math.max(layout.y + 4, Math.min(y, hy) - Math.max(14, Math.abs(hx - x) * 0.11));
    const progress = 1 - Math.pow(1 - strike.progress, 3);
    const headX = quad(x, mx, hx, progress);
    const headY = quad(y, my, hy, progress);
    const alpha = Math.max(0, 0.56 * (1 - strike.progress * 0.45));

    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.quadraticCurveTo(mx, my, headX, headY);
    ctx.strokeStyle = `rgba(${strike.color}, ${alpha})`;
    ctx.lineWidth = 1.2;
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(headX, headY, 2.4, 0, Math.PI * 2);
    ctx.fillStyle = `rgba(${strike.color}, 0.95)`;
    ctx.fill();

    if (strike.progress < 1) {
      keep.push(strike);
      active = true;
    }
  });
  crowdsecMapStrikes = keep;
  return active;
}

function quad(p0, p1, p2, t) {
  const inverse = 1 - t;
  return inverse * inverse * p0 + 2 * inverse * t * p1 + t * t * p2;
}

function crowdsecCountryName(code) {
  const normalized = String(code || '??').toUpperCase();
  if (normalized === '??') return 'Unknown country';
  try {
    if (typeof Intl.DisplayNames === 'function') {
      return new Intl.DisplayNames(undefined, { type: 'region' }).of(normalized) || normalized;
    }
  } catch (_) { /* use code fallback */ }
  return normalized;
}

function crowdsecMapEscape(value) {
  if (typeof crowdsecChartEscape === 'function') return crowdsecChartEscape(value);
  if (typeof escapeHtml === 'function') return escapeHtml(String(value));
  return String(value).replace(/[&<>"']/g, char => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'
  })[char]);
}

function crowdsecMapUpdateInspector(cluster) {
  const inspector = $('#crowdsecMapInspector');
  if (!inspector) return;
  if (!cluster) {
    const destination = crowdsecMapHome
      ? 'Destination arcs use the configured server coordinates.'
      : 'No destination is configured, so attack arcs are hidden.';
    const html = `<p>Select a bubble or use the arrow keys on the map to inspect detections at that location.</p><p class="crowdsec-inspector-scenario">${destination}</p>`;
    if (inspector.innerHTML !== html) inspector.innerHTML = html;
    inspector.dataset.clusterKey = '';
    return;
  }

  const country = crowdsecMapEscape(crowdsecCountryName(cluster.country));
  const code = crowdsecMapEscape(cluster.country || '??');
  const sourceCount = cluster.sources.size;
  const actionLabel = crowdsecMapOutcomeLabel(cluster);
  const html = `<div class="crowdsec-inspector-country"><strong>${country}</strong><span>${code}</span></div>` +
    `<div class="crowdsec-inspector-grid"><div><small>Detections</small><strong>${cluster.count}</strong></div><div><small>Reported events</small><strong>${cluster.reportedEvents}</strong></div><div><small>Sources</small><strong>${sourceCount}</strong></div><div><small>Outcome</small><strong title="${crowdsecMapEscape(actionLabel)}">${crowdsecMapEscape(actionLabel)}</strong></div><div><small>Network</small><strong title="${crowdsecMapEscape(cluster.network)}">${crowdsecMapEscape(cluster.network)}</strong></div><div><small>Latest</small><strong>${crowdsecMapEscape(typeof relativeTime === 'function' ? relativeTime(cluster.latestAt) : cluster.latestAt)}</strong></div></div>` +
    `<p class="crowdsec-inspector-scenario"><strong>Top scenario:</strong> ${crowdsecMapEscape(typeof shortScenario === 'function' ? shortScenario(cluster.scenario) : cluster.scenario)}</p>`;
  if (inspector.dataset.clusterKey !== cluster.key || inspector.innerHTML !== html) {
    inspector.innerHTML = html;
    inspector.dataset.clusterKey = cluster.key;
  }
}

function crowdsecMapOutcomeLabel(cluster) {
  if (!cluster) return 'No detections';
  const parts = [];
  if (cluster.actioned) parts.push(`${cluster.actioned} decision attached`);
  if (cluster.observed) parts.push(`${cluster.observed} detection${cluster.observed === 1 ? '' : 's'} only`);
  return parts.join(' · ') || 'No detections';
}

function crowdsecMapPositionTip(layout) {
  const tip = $('#crowdsecMapTip');
  if (!tip) return;
  const cluster = crowdsecMapHover;
  if (!cluster) {
    tip.classList.add('hidden');
    return;
  }
  const point = crowdsecProject(cluster.latitude, cluster.longitude);
  const x = Math.max(25, Math.min(layout.x + point.x * layout.w, layout.x + layout.w - 25));
  const y = Math.max(22, layout.y + point.y * layout.h);
  const outcome = [
    cluster.actioned ? `<span class="crowdsec-tip-actioned">${cluster.actioned} decision attached</span>` : '',
    cluster.observed ? `<span class="crowdsec-tip-observed">${cluster.observed} detection${cluster.observed === 1 ? '' : 's'} only</span>` : ''
  ].filter(Boolean).join(' · ');
  const html = `<strong>${crowdsecMapEscape(crowdsecCountryName(cluster.country))}</strong> · ${cluster.count} detection${cluster.count === 1 ? '' : 's'}<br>${crowdsecMapEscape(typeof shortScenario === 'function' ? shortScenario(cluster.scenario) : cluster.scenario)} · ${outcome}`;
  if (tip.innerHTML !== html) tip.innerHTML = html;
  tip.style.left = `${x}px`;
  tip.style.top = `${y}px`;
  tip.classList.remove('hidden');
}

function crowdsecMapHitTest(clientX, clientY) {
  const canvas = crowdsecMapCanvas || $('#crowdsecMap');
  if (!canvas) return null;
  const rect = canvas.getBoundingClientRect();
  const layout = crowdsecMapLayout(rect.width, rect.height);
  const clusters = crowdsecMapClusters();
  const maxCount = Math.max(1, ...clusters.map(cluster => cluster.count));
  const coarsePointer = typeof window.matchMedia === 'function' && window.matchMedia('(pointer: coarse)').matches;
  let best = null;
  let bestDistance = Infinity;
  clusters.forEach(cluster => {
    const point = crowdsecProject(cluster.latitude, cluster.longitude);
    const x = layout.x + point.x * layout.w;
    const y = layout.y + point.y * layout.h;
    const distance = Math.hypot(clientX - rect.left - x, clientY - rect.top - y);
    const hitRadius = crowdsecBubbleRadius(cluster.count, maxCount) + (coarsePointer ? 16 : 7);
    if (distance <= hitRadius && distance < bestDistance) {
      best = cluster;
      bestDistance = distance;
    }
  });
  return best;
}

function crowdsecMapOnMouseMove(event) {
  const next = crowdsecMapHitTest(event.clientX, event.clientY);
  if ((next && crowdsecMapHover && next.key === crowdsecMapHover.key) || (!next && !crowdsecMapHover)) return;
  crowdsecMapHover = next;
  if (!crowdsecMapSelectedKey) crowdsecMapUpdateInspector(next);
  crowdsecMapRequestDraw();
}

function crowdsecMapOnMouseLeave() {
  crowdsecMapHover = null;
  const tip = $('#crowdsecMapTip');
  if (tip) tip.classList.add('hidden');
  if (!crowdsecMapSelectedKey) crowdsecMapUpdateInspector(null);
  crowdsecMapRequestDraw();
}

function crowdsecMapOnClick(event) {
  const hasCoordinates = event && Number.isFinite(event.clientX) && Number.isFinite(event.clientY);
  const cluster = hasCoordinates ? crowdsecMapHitTest(event.clientX, event.clientY) : crowdsecMapHover;
  if (!cluster) return;
  crowdsecMapHover = cluster;
  crowdsecMapSelectCluster(cluster);
}

function crowdsecMapSelectCluster(cluster) {
  crowdsecMapSelectedKey = cluster ? cluster.key : '';
  crowdsecMapUpdateInspector(cluster || null);
  $$('.crowdsec-alert-row').forEach(row => {
    row.classList.toggle('is-selected', !!cluster && cluster.alertIDs.includes(String(row.dataset.alertId || '')));
    row.setAttribute('aria-pressed', String(!!cluster && cluster.alertIDs.includes(String(row.dataset.alertId || ''))));
  });
  crowdsecMapRequestDraw();
}

function crowdsecMapClusterForAlert(alert) {
  if (!crowdsecMapValidCoordinates(alert)) return null;
  const key = crowdsecMapClusterKey(alert.latitude, alert.longitude);
  return crowdsecMapClusters().find(cluster => cluster.key === key) || null;
}

function crowdsecMapPreviewAlert(alert) {
  crowdsecMapHover = alert ? crowdsecMapClusterForAlert(alert) : null;
  if (!crowdsecMapSelectedKey) crowdsecMapUpdateInspector(crowdsecMapHover);
  crowdsecMapRequestDraw();
  return !!crowdsecMapHover;
}

function crowdsecMapSelectAlert(alert) {
  const cluster = alert ? crowdsecMapClusterForAlert(alert) : null;
  crowdsecMapSelectCluster(cluster);
  return !!cluster;
}

function crowdsecMapClearPreview() {
  crowdsecMapOnMouseLeave();
}

// Backward-compatible entry point used by existing feed integrations.
function crowdsecMapHighlightAlert(alert) {
  return crowdsecMapPreviewAlert(alert);
}

function crowdsecMapOnKeyDown(event) {
  const clusters = crowdsecMapClusters();
  if (!clusters.length) return;
  let next = crowdsecMapKeyboardIndex;
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    next = (next + 1 + clusters.length) % clusters.length;
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    next = (next - 1 + clusters.length) % clusters.length;
  } else if (event.key === 'Home') {
    next = 0;
  } else if (event.key === 'End') {
    next = clusters.length - 1;
  } else if ((event.key === 'Enter' || event.key === ' ') && crowdsecMapHover) {
    event.preventDefault();
    crowdsecMapSelectCluster(crowdsecMapHover);
    return;
  } else if (event.key === 'Escape') {
    event.preventDefault();
    crowdsecMapKeyboardIndex = -1;
    crowdsecMapHover = null;
    crowdsecMapSelectCluster(null);
    return;
  } else {
    return;
  }
  event.preventDefault();
  crowdsecMapKeyboardIndex = next;
  crowdsecMapHover = clusters[next];
  crowdsecMapUpdateInspector(crowdsecMapHover);
  crowdsecMapRequestDraw();
}

function crowdsecMapSetFilter(filter) {
  if (!['all', 'actioned', 'observed'].includes(filter)) return;
  crowdsecMapFilter = filter;
  $$('[data-crowdsec-map-filter]').forEach(button => {
    button.setAttribute('aria-pressed', String(button.dataset.crowdsecMapFilter === filter));
  });
  crowdsecMapHover = null;
  crowdsecMapKeyboardIndex = -1;
  if (crowdsecMapSelectedKey && !crowdsecMapClusters().some(cluster => cluster.key === crowdsecMapSelectedKey)) {
    crowdsecMapSelectCluster(null);
  } else {
    const selected = crowdsecMapClusters().find(cluster => cluster.key === crowdsecMapSelectedKey) || null;
    crowdsecMapUpdateInspector(selected);
  }
  crowdsecMapUpdateState();
  crowdsecMapRequestDraw();
}

function crowdsecMapApply(alerts, homeLat, homeLng) {
  const previous = crowdsecMapAlerts.slice();
  crowdsecAllAlerts = Array.isArray(alerts) ? alerts : [];
  if (typeof homeLat === 'number' && typeof homeLng === 'number') {
    const unset = homeLat === 0 && homeLng === 0;
    crowdsecMapHome = unset ? null : { lat: homeLat, lng: homeLng };
  }

  crowdsecMapUpdateState();
  const clusters = crowdsecMapClusters();
  if (crowdsecMapSelectedKey) {
    const selected = clusters.find(cluster => cluster.key === crowdsecMapSelectedKey) || null;
    if (!selected) crowdsecMapSelectedKey = '';
    crowdsecMapSelectCluster(selected);
  } else {
    const hoverKey = crowdsecMapHover && crowdsecMapHover.key;
    crowdsecMapHover = hoverKey ? (clusters.find(cluster => cluster.key === hoverKey) || null) : null;
    crowdsecMapUpdateInspector(crowdsecMapHover);
  }

  if (crowdsecMapCanvas && crowdsecMapHome) {
    const previousIDs = new Set(previous.map(alert => String(alert.alert_id || '')));
    const fresh = crowdsecAllAlerts.filter(alert => crowdsecMapValidCoordinates(alert) && !previousIDs.has(String(alert.alert_id || ''))).slice(0, 6);
    fresh.forEach((alert, index) => crowdsecQueueStrike(alert, index * 150));
  }
  crowdsecMapAlerts = crowdsecAllAlerts.slice();
  crowdsecMapRequestDraw();
}
