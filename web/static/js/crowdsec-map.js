// CrowdSec attack map: animated dot-matrix world with strike arcs from
// attacker geolocations to your home position. Canvas-based, no external
// assets (CSP-safe), DPR-aware, pauses when the tab is not visible.

// Equirectangular projection: lat/lng → normalized [0..1] map space.
function crowdsecProject(lat, lng) {
  const x = (lng + 180) / 360;
  const y = (90 - lat) / 180;
  return { x, y };
}

// Minimal world land-mass representation for the dot-matrix map: a coarse
// grid of cells covering major continents (rounded to 10-degree tiles).
// Each entry: [lat, lng] of the tile's south-west corner. This renders the
// classic "cyber threat map" dotted-continents look without any asset files.
var CROWDSEC_LAND_TILES = (function () {
  const tiles = [];
  const add = (lat, lng) => tiles.push([lat, lng]);

  // North America (rough): Alaska, Canada, USA, Mexico, Central America.
  for (let lng = -168; lng <= -128; lng += 8) add(52, lng); // Alaska
  for (let lng = -140; lng <= -56; lng += 8) for (let lat = 42; lat <= 68; lat += 8) add(lat, lng); // Canada
  for (let lng = -124; lng <= -68; lng += 8) for (let lat = 30; lat <= 42; lat += 8) add(lat, lng); // CONUS
  add(24, -104); add(20, -104); add(18, -96); add(16, -96); add(20, -88); add(16, -88); // Mexico
  add(14, -88); add(12, -84); add(9, -84); // Central America
  add(19, -72); add(18, -72); add(20, -76); add(24, -80); // Caribbean
  add(64, -52); add(60, -44); // Greenland (sparse)

  // South America.
  for (let lng = -80; lng <= -36; lng += 8) add(0, lng);
  for (let lat = -40; lat <= -8; lat += 8) for (let lng = -72; lng <= -48; lng += 8) add(lat, lng);
  for (let lat = 8; lat >= -8; lat -= 8) for (let lng = -78; lng <= -50; lng += 8) add(lat, lng);
  for (let lat = 0; lat <= 8; lat += 8) for (let lng = -76; lng <= -56; lng += 8) add(lat, lng);
  add(-8, -76); add(-16, -72); add(-48, -72); add(-40, -68); add(-48, -68); add(-44, -72); add(-36, -64); add(-32, -64); add(-28, -60); add(-24, -60);

  // Europe.
  for (let lng = -8; lng <= 32; lng += 8) add(48, lng);
  for (let lng = -8; lng <= 40; lng += 8) for (let lat = 40; lat <= 48; lat += 8) add(lat, lng);
  for (let lat = 36; lat <= 44; lat += 8) for (let lng = 0; lng <= 40; lng += 8) add(lat, lng);
  add(56, 8); add(60, 8); add(62, 8); add(56, 16); add(60, 16); add(52, 24); add(56, 24); // Nordics/Baltics
  add(36, -6); add(38, -6); add(40, -4); add(43, -2); // Iberia
  add(52, -2); add(52, 0); add(50, -2); // UK
  add(38, 24); add(36, 24); add(40, 22); // Greece/Balkans
  add(70, 24); add(68, 32); add(66, 40); // Arctic Russia fringe

  // Africa.
  for (let lat = 8; lat <= 32; lat += 8) for (let lng = -16; lng <= 48; lng += 8) add(lat, lng); // North Africa
  for (let lat = -8; lat <= 8; lat += 8) for (let lng = 8; lng <= 40; lng += 8) add(lat, lng); // Central
  for (let lat = -32; lat <= -8; lat += 8) for (let lng = 16; lng <= 40; lng += 8) add(lat, lng); // South
  add(4, -16); add(8, -12); add(12, -16); add(16, -16); add(-4, 12); add(-12, 12); add(20, -16); add(24, -16); add(4, 44); add(12, 44); add(-16, 44); add(-20, 44);

  // Middle East.
  for (let lat = 24; lat <= 40; lat += 8) for (let lng = 40; lng <= 60; lng += 8) add(lat, lng);
  add(28, 48); add(24, 52); add(32, 56); add(36, 60);

  // Russia / Central Asia.
  for (let lng = 48; lng <= 136; lng += 8) for (let lat = 48; lat <= 64; lat += 8) add(lat, lng);
  for (let lng = 56; lng <= 120; lng += 8) for (let lat = 40; lat <= 48; lat += 8) add(lat, lng);
  add(36, 64); add(36, 72); add(40, 72); add(36, 80); add(40, 80); add(36, 88); add(40, 88);

  // South Asia.
  add(20, 72); add(24, 72); add(20, 76); add(16, 76); add(12, 76); add(8, 76); add(20, 80); add(24, 80); add(16, 80); add(24, 84); add(20, 84); add(28, 76); add(28, 80); add(32, 76);

  // East / South-East Asia.
  for (let lat = 24; lat <= 44; lat += 8) for (let lng = 104; lng <= 128; lng += 8) add(lat, lng); // China
  add(20, 104); add(16, 104); add(12, 104); add(8, 104); // SE Asia
  add(16, 108); add(12, 108); add(8, 108); add(16, 112); add(12, 112); add(8, 112);
  add(10, 120); add(14, 120); add(10, 124); add(6, 120); // Philippines
  add(-2, 112); add(2, 112); add(-2, 116); add(2, 116); add(-6, 112); add(-6, 116); add(-8, 120); add(-4, 120); // Indonesia
  add(52, 128); add(56, 128); add(52, 136); add(56, 136); add(60, 136); add(60, 144); // Russia Far East

  // Japan / Korea.
  add(36, 136); add(40, 136); add(44, 136); add(36, 140); add(40, 140); add(44, 140); add(34, 132); add(38, 128);

  // Australia / NZ.
  for (let lat = -40; lat <= -24; lat += 8) for (let lng = 112; lng <= 152; lng += 8) add(lat, lng);
  add(-40, 168); add(-44, 168); // NZ

  // Deduplicate overlapping regions (tiles added by more than one loop).
  const unique = [];
  const seen = new Set();
  for (const tile of tiles) {
    const key = tile[0] + ',' + tile[1];
    if (!seen.has(key)) {
      seen.add(key);
      unique.push(tile);
    }
  }
  return unique;
})();

// Map runtime state.
var crowdsecMapCanvas = null;
var crowdsecMapObserver = null;
var crowdsecMapAnim = null;
var crowdsecMapAlerts = [];
var crowdsecMapHome = { lat: 51.5074, lng: -0.1278 }; // London default
var crowdsecMapStrikes = [];

// crowdsecMapInit boots the map on first tab open.
function crowdsecMapInit() {
  const canvas = $('#crowdsecMap');
  if (!canvas || crowdsecMapCanvas === canvas) return;
  crowdsecMapCanvas = canvas;

  // The canvas starts hidden (tab not active), so a plain resize listener
  // would measure 0x0; ResizeObserver re-fires when the tab becomes visible.
  if (typeof ResizeObserver === 'function') {
    crowdsecMapObserver = new ResizeObserver(crowdsecMapResize);
    crowdsecMapObserver.observe(canvas);
  } else {
    window.addEventListener('resize', crowdsecMapResize);
  }
  crowdsecMapResize();

  // Strike arcs fire when the feed reloads; replay recent history on boot.
  crowdsecMapStrikes = [];
  const recent = crowdsecAllAlerts.filter(a => a.latitude != null && a.longitude != null).slice(0, 15);
  recent.forEach((a, i) => crowdsecQueueStrike(a, i * 220));
  // Remember which batch we replayed so the first live refresh doesn't
  // re-fire strikes for alerts we just animated.
  crowdsecMapAlerts = crowdsecAllAlerts;

  if (typeof crowdsecMapLoop === 'function') crowdsecMapLoop();
}

function crowdsecMapResize() {
  const canvas = crowdsecMapCanvas || $('#crowdsecMap');
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return; // hidden tab
  const dpr = window.devicePixelRatio || 1;
  if (canvas.width === Math.round(rect.width * dpr) && canvas.height === Math.round(rect.height * dpr)) return;
  canvas.width = Math.round(rect.width * dpr);
  canvas.height = Math.round(rect.height * dpr);
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

// crowdsecQueueStrike schedules a strike from an alert's geo to home.
function crowdsecQueueStrike(alert, delayMs) {
  const lat = alert.latitude;
  const lng = alert.longitude;
  if (lat == null || lng == null) return null;
  const strike = {
    from: crowdsecProject(lat, lng),
    color: alert.has_decision ? 'rgba(220, 38, 38, ' : 'rgba(217, 119, 6, ',
    progress: 0,
    delay: delayMs || 0,
    trail: [],
    alert
  };
  crowdsecMapStrikes.push(strike);
  return strike;
}

// crowdsecMapLayout computes the letterboxed 2:1 map area inside the canvas
// so the equirectangular projection never distorts, whatever the container
// size is. Returns {x, y, w, h, dot} — map origin, size, and adaptive dot size.
function crowdsecMapLayout(w, h) {
  const pad = 10;
  const availW = w - pad * 2;
  const availH = h - pad * 2;
  // Fit the largest 2:1 box inside the available area (letterbox either way).
  let mapW = availW;
  let mapH = mapW / 2;
  if (mapH > availH) {
    mapH = availH;
    mapW = mapH * 2;
  }
  return {
    x: (w - mapW) / 2,
    y: (h - mapH) / 2,
    w: mapW,
    h: mapH,
    // Dot size scales with the map width, clamped for readability.
    dot: Math.min(3.2, Math.max(1.3, mapW / 460))
  };
}

// The animation loop: draw dots, home beacon, strike arcs, pins.
function crowdsecMapLoop() {
  const canvas = $('#crowdsecMap');
  if (!canvas) return;
  // Pause rendering while the browser tab is hidden to save CPU.
  if (document.hidden) {
    requestAnimationFrame(crowdsecMapLoop);
    return;
  }
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.width / dpr;
  const h = canvas.height / dpr;

  ctx.clearRect(0, 0, w, h);

  // Background: subtle radial glow.
  const bg = ctx.createRadialGradient(w / 2, h / 2, 10, w / 2, h / 2, Math.max(w, h) * 0.7);
  bg.addColorStop(0, 'rgba(20, 32, 52, 1)');
  bg.addColorStop(1, 'rgba(8, 14, 24, 1)');
  ctx.fillStyle = bg;
  ctx.fillRect(0, 0, w, h);

  // Dot-matrix land inside the letterboxed 2:1 area.
  const layout = crowdsecMapLayout(w, h);
  const mapW = layout.w;
  const mapH = layout.h;
  const ox = layout.x;
  const oy = layout.y;
  ctx.fillStyle = 'rgba(90, 130, 180, 0.35)';
  for (const [lat, lng] of CROWDSEC_LAND_TILES) {
    const p = crowdsecProject(lat + 4, lng + 4); // tile center
    ctx.beginPath();
    ctx.arc(ox + p.x * mapW, oy + p.y * mapH, layout.dot, 0, Math.PI * 2);
    ctx.fill();
  }

  const home = crowdsecProject(crowdsecMapHome.lat, crowdsecMapHome.lng);
  const hx = ox + home.x * mapW;
  const hy = oy + home.y * mapH;

  // Home beacon: pulsing rings.
  const t = Date.now() / 1000;
  for (let ring = 0; ring < 3; ring++) {
    const phase = ((t * 0.5 + ring / 3) % 1);
    ctx.beginPath();
    ctx.arc(hx, hy, 4 + phase * 22, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(59, 200, 130, ' + (0.55 * (1 - phase)) + ')';
    ctx.lineWidth = 1.5;
    ctx.stroke();
  }
  ctx.beginPath();
  ctx.arc(hx, hy, 4, 0, Math.PI * 2);
  ctx.fillStyle = '#3bc882';
  ctx.shadowColor = '#3bc882';
  ctx.shadowBlur = 10;
  ctx.fill();
  ctx.shadowBlur = 0;

  // Strike arcs + attacker pins.
  const active = crowdsecMapStrikes.filter(s => s.progress < 1.35);
  for (const s of active) {
    if (s.delay > 0) { s.delay -= 16; continue; }
    s.progress = Math.min(1.35, s.progress + 0.016);

    const from = s.from;
    const fx = ox + from.x * mapW;
    const fy = oy + from.y * mapH;

    // Attacker pin: small red dot that glows while its strike is live.
    if (s.progress < 1.1) {
      ctx.beginPath();
      ctx.arc(fx, fy, 2.6, 0, Math.PI * 2);
      ctx.fillStyle = s.color + '0.9)';
      ctx.shadowColor = s.color + '0.8)';
      ctx.shadowBlur = 8;
      ctx.fill();
      ctx.shadowBlur = 0;
    }

    // Quadratic arc from attacker to home; the comet head travels along it.
    // The apex lifts proportionally to the arc span but stays inside the box.
    const mx = (fx + hx) / 2;
    const apex = Math.min(fy, hy) - Math.max(14, Math.min(fy, hy) - oy);
    const my = Math.max(oy, apex - Math.abs(hx - fx) * 0.12);
    const head = Math.min(1, s.progress);

    ctx.beginPath();
    ctx.moveTo(fx, fy);
    ctx.quadraticCurveTo(mx, my, hx, hy);
    ctx.strokeStyle = s.color + (0.12 * Math.max(0, 1 - s.progress)) + ')';
    ctx.lineWidth = 1;
    ctx.stroke();

    // Comet head with short trail.
    const tt = head;
    const cx = quad(fx, mx, hx, tt);
    const cy = quad(fy, my, hy, tt);
    for (let k = 1; k <= 5; k++) {
      const bt = Math.max(0, tt - k * 0.03);
      const bx = quad(fx, mx, hx, bt);
      const by = quad(fy, my, hy, bt);
      ctx.beginPath();
      ctx.arc(bx, by, 2.2 - k * 0.35, 0, Math.PI * 2);
      ctx.fillStyle = s.color + (0.5 - k * 0.09) + ')';
      ctx.fill();
    }
    ctx.beginPath();
    ctx.arc(cx, cy, 2.6, 0, Math.PI * 2);
    ctx.fillStyle = s.color + '0.95)';
    ctx.shadowColor = s.color + '0.9)';
    ctx.shadowBlur = 9;
    ctx.fill();
    ctx.shadowBlur = 0;

    // Impact ripple at home when the comet lands.
    if (s.progress > 1) {
      const ripplePhase = (s.progress - 1) / 0.35;
      ctx.beginPath();
      ctx.arc(hx, hy, ripplePhase * 14, 0, Math.PI * 2);
      ctx.strokeStyle = s.color + (0.6 * (1 - ripplePhase)) + ')';
      ctx.lineWidth = 1.5;
      ctx.stroke();
    }
  }
  // Retire finished strikes.
  crowdsecMapStrikes = crowdsecMapStrikes.filter(s => s.progress < 1.35);

  // Hover tooltip handling (cheap hit test on attacker pins).
  crowdsecMapDrawHover(ctx, layout);

  requestAnimationFrame(crowdsecMapLoop);
}

// quad computes a point on a quadratic bezier curve at t.
function quad(p0, p1, p2, t) {
  const a = (1 - t) * (1 - t);
  const b = 2 * (1 - t) * t;
  const c = t * t;
  return a * p0 + b * p1 + c * p2;
}

var crowdsecMapHover = null;

// crowdsecMapDrawHover renders the tooltip for the nearest attacker pin.
function crowdsecMapDrawHover(ctx, layout) {
  const tip = $('#crowdsecMapTip');
  if (!tip) return;
  if (!crowdsecMapHover) {
    tip.classList.add('hidden');
    return;
  }
  const a = crowdsecMapHover;
  const country = String(a.country || '??').toUpperCase();
  tip.innerHTML = '<strong>' + escapeHtml(country) + '</strong> · ' +
    escapeHtml(shortScenario(a.scenario || '')) +
    (a.source_value ? '<br>' + escapeHtml(a.source_value) : '') +
    (a.has_decision ? ' · <span class="crowdsec-tip-banned">banned</span>' : ' · <span class="crowdsec-tip-scan">no ban</span>');
  tip.classList.remove('hidden');
  // Position near the pin, clamped to the canvas.
  const p = crowdsecProject(a.latitude, a.longitude);
  const x = Math.min(Math.max(layout.x + p.x * layout.w, 60), layout.w + layout.x - 20);
  const y = Math.max(10, layout.y + p.y * layout.h - 40);
  tip.style.left = x + 'px';
  tip.style.top = y + 'px';
}

// crowdsecMapOnMouseMove hit-tests recent alert pins for the tooltip.
function crowdsecMapOnMouseMove(e) {
  const canvas = crowdsecMapCanvas || $('#crowdsecMap');
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const mx = e.clientX - rect.left;
  const my = e.clientY - rect.top;
  const layout = crowdsecMapLayout(rect.width, rect.height);
  let best = null;
  let bestDist = 12;
  for (const a of crowdsecAllAlerts) {
    if (a.latitude == null || a.longitude == null) continue;
    const p = crowdsecProject(a.latitude, a.longitude);
    const ax = layout.x + p.x * layout.w;
    const ay = layout.y + p.y * layout.h;
    const d = Math.hypot(mx - ax, my - ay);
    if (d < bestDist) {
      bestDist = d;
      best = a;
    }
  }
  crowdsecMapHover = best;
}

// crowdsecMapApply feeds new alert data to the map and fires fresh strikes.
function crowdsecMapApply(alerts, homeLat, homeLng) {
  crowdsecAllAlerts = alerts || [];
  if (typeof homeLat === 'number' && typeof homeLng === 'number' && (homeLat !== 0 || homeLng !== 0)) {
    crowdsecMapHome = { lat: homeLat, lng: homeLng };
  }
  if (!crowdsecMapCanvas) return; // map not mounted yet; init will replay
  // Fire strikes for alerts newer than the previous batch head.
  const prevHead = crowdsecMapAlerts.length ? crowdsecMapAlerts[0].alert_id : null;
  const fresh = [];
  for (const a of crowdsecAllAlerts) {
    if (a.alert_id === prevHead) break;
    if (a.latitude != null && a.longitude != null) fresh.push(a);
  }
  fresh.forEach((a, i) => crowdsecQueueStrike(a, i * 350));
  crowdsecMapAlerts = crowdsecAllAlerts;
}