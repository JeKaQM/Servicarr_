let currentView = 'cards';       // 'cards' | 'matrix'
let latestLiveStatus = null;     // cache last /api/check result for matrix

function initViewToggle() {
  const btnCards  = $('#viewCards');
  const btnMatrix = $('#viewMatrix');
  if (!btnCards || !btnMatrix) return;

  btnCards.addEventListener('click',  () => switchView('cards'));
  btnMatrix.addEventListener('click', () => switchView('matrix'));
}

/* ── Global Health Dot ─────────────────────────────────── */
function updateHealthDot(statusMap) {
  const dot = $('#healthDot');
  if (!dot) return;

  dot.classList.remove('all-up', 'some-down', 'some-degraded');

  let hasDown = false, hasDegraded = false, hasUp = false;
  Object.values(statusMap).forEach(s => {
    if (s.disabled) return;
    if (s.maintenance) hasDegraded = true;
    else if (!s.ok) hasDown = true;
    else if (s.degraded) hasDegraded = true;
    else hasUp = true;
  });

  if (hasDown)           dot.classList.add('some-down');
  else if (hasDegraded)  dot.classList.add('some-degraded');
  else                   dot.classList.add('all-up');
}

/* ── Status Summary Bar ────────────────────────────────── */
function updateStatusSummary(statusMap) {
  const bar = $('#statusSummary');
  if (!bar) return;

  let up = 0, down = 0, degraded = 0, disabled = 0, maintenance = 0;
  Object.values(statusMap).forEach(s => {
    if (s.disabled)      disabled++;
    else if (s.maintenance) maintenance++;
    else if (!s.ok)      down++;
    else if (s.degraded) degraded++;
    else                 up++;
  });

  const parts = [];
  if (up > 0)       parts.push('<span class="status-summary-item"><span class="status-summary-dot up"></span><span class="status-summary-count">' + up + '</span> Operational</span>');
  if (down > 0)     parts.push('<span class="status-summary-item"><span class="status-summary-dot down"></span><span class="status-summary-count">' + down + '</span> Down</span>');
  if (degraded > 0) parts.push('<span class="status-summary-item"><span class="status-summary-dot degraded"></span><span class="status-summary-count">' + degraded + '</span> Degraded</span>');
  if (disabled > 0) parts.push('<span class="status-summary-item"><span class="status-summary-dot disabled"></span><span class="status-summary-count">' + disabled + '</span> Disabled</span>');
  if (maintenance > 0) parts.push('<span class="status-summary-item"><span class="status-summary-dot maintenance"></span><span class="status-summary-count">' + maintenance + '</span> Maintenance</span>');

  bar.innerHTML = parts.join('');
}

function switchView(view) {
  currentView = view;
  const cards  = $('#services-container');
  const matrix = $('#matrix-container');
  const btnC   = $('#viewCards');
  const btnM   = $('#viewMatrix');
  const mainEl = document.querySelector('main');

  if (view === 'matrix') {
    cards  && cards.classList.add('hidden');
    matrix && matrix.classList.remove('hidden');
    btnC   && btnC.classList.remove('active');
    btnM   && btnM.classList.add('active');
    mainEl && mainEl.classList.add('matrix-active');
    renderMatrix();
  } else {
    matrix && matrix.classList.add('hidden');
    cards  && cards.classList.remove('hidden');
    btnM   && btnM.classList.remove('active');
    btnC   && btnC.classList.add('active');
    mainEl && mainEl.classList.remove('matrix-active');
    stopMatrixAnimation();
  }
}

/* ── Matrix status helpers ──────────────────────────────── */
function matrixStatusOf(svc) {
  let statusClass = 'unknown', statusLabel = 'Unknown', ms = null;
  if (latestLiveStatus && latestLiveStatus[svc.key]) {
    const s = latestLiveStatus[svc.key];
    if (s.disabled)       { statusClass = 'disabled'; statusLabel = 'Disabled'; }
    else if (s.maintenance) { statusClass = 'maintenance'; statusLabel = 'Maintenance'; }
    else if (!s.ok)       { statusClass = 'down';     statusLabel = 'Down';     }
    else if (s.degraded)  { statusClass = 'degraded'; statusLabel = 'Degraded'; }
    else                  { statusClass = 'up';       statusLabel = 'Operational'; }
    if (s.ms != null) ms = s.ms;
  }
  return { statusClass, statusLabel, ms };
}

const MATRIX_COLORS = {
  up:       { r: 34,  g: 197, b: 94  },
  down:     { r: 248, g: 113, b: 113 },
  degraded: { r: 251, g: 191, b: 36  },
  maintenance: { r: 245, g: 158, b: 11 },
  disabled: { r: 100, g: 116, b: 139 },
  unknown:  { r: 100, g: 116, b: 139 },
  hub:      { r: 99,  g: 102, b: 241 }
};

// Icon markup for a service: an allowlisted image URL, or the built-in icon.
function serviceIconMarkup(svc) {
  if (svc.icon_url && /^(https?:\/\/|data:image\/|\/static\/)/.test(svc.icon_url)) {
    return '<img src="' + escapeHtml(svc.icon_url) + '" class="matrix-node-icon" alt="">';
  }
  const raw = getServiceIconHtml(svc);
  if (raw.includes('<img')) {
    return raw.replace(/class="icon[^"]*"/g, 'class="matrix-node-icon"');
  }
  return '<span class="matrix-node-icon-placeholder">' + raw.replace(/<\/?span[^>]*>/g, '') + '</span>';
}

/* ── Topology map ───────────────────────────────────────── */
// Services sit in columns by dependency depth: the services others rely on
// (router, NAS) on the left and the services that need them to the right.
// Arrows point at the service that needs the other one, so an outage can be
// followed left to right along the red links. Narrow screens use rows.

const TOPO_SIZES = {
  wide:   { ring: 48, nodeSlot: 104, dummySlot: 26, padMain: 96, padCross: 28, minGap: 190, maxGap: 420, rowGap: 0, label: 36 },
  narrow: { ring: 40, nodeSlot: 96,  dummySlot: 22, padMain: 28, padCross: 24, minGap: 0,   maxGap: 0,   rowGap: 136, label: 34 }
};
const TOPO_NARROW_BELOW = 640;   // container width (px) under which layers become rows
const TOPO_SVG_NS = 'http://www.w3.org/2000/svg';
const TOPO_XLINK_NS = 'http://www.w3.org/1999/xlink';
let topoState = null;

function topologyKeys(value) {
  return String(value || '').split(',').map(k => k.trim()).filter(Boolean);
}

// The link graph: dependency edges run from the upstream service to the one
// that needs it; peer ("connected to") links are undirected and deduplicated.
function buildTopologyModel(services) {
  const byKey = new Map();
  services.forEach(svc => { if (svc && svc.key && !byKey.has(svc.key)) byKey.set(svc.key, svc); });
  const deps = new Map(), dependents = new Map(), peers = new Map();
  byKey.forEach((_, key) => { deps.set(key, []); dependents.set(key, []); peers.set(key, new Set()); });
  const depEdges = [], peerEdges = [], seenPairs = new Set();

  byKey.forEach((svc, key) => {
    topologyKeys(svc.depends_on).forEach(up => {
      if (up === key || !byKey.has(up) || deps.get(key).includes(up)) return;
      deps.get(key).push(up);
      dependents.get(up).push(key);
      depEdges.push({ type: 'dep', from: up, to: key });
    });
    topologyKeys(svc.connected_to).forEach(peer => {
      if (peer === key || !byKey.has(peer)) return;
      const pair = [key, peer].sort().join('|');
      if (seenPairs.has(pair)) return;
      seenPairs.add(pair);
      peers.get(key).add(peer);
      peers.get(peer).add(key);
      peerEdges.push({ type: 'peer', from: key, to: peer });
    });
  });

  const linked = [], standalone = [];
  byKey.forEach((_, key) => {
    const hasLinks = deps.get(key).length || dependents.get(key).length || peers.get(key).size;
    (hasLinks ? linked : standalone).push(key);
  });
  return { byKey, order: [...byKey.keys()], deps, dependents, peers, depEdges, peerEdges, linked, standalone };
}

// Layered layout (longest-path layers, a lane per column for links that skip
// columns, barycentre ordering to cut crossings). Returns ring-centre
// positions and an SVG path per link, in stage pixels.
function layoutTopology(model, containerWidth) {
  const narrow = containerWidth < TOPO_NARROW_BELOW;
  const S = narrow ? TOPO_SIZES.narrow : TOPO_SIZES.wide;
  const R = S.ring / 2;

  // 1. Layers. Links that close a loop are left out of the layering.
  const layer = new Map();
  const visiting = new Set();
  const loopLinks = new Set();
  const visit = key => {
    if (layer.has(key)) return layer.get(key);
    if (visiting.has(key)) return -1;
    visiting.add(key);
    let depth = 0;
    model.deps.get(key).forEach(up => {
      const upLayer = visit(up);
      if (upLayer < 0) loopLinks.add(up + '>' + key);
      else depth = Math.max(depth, upLayer + 1);
    });
    visiting.delete(key);
    layer.set(key, depth);
    return depth;
  };
  model.linked.forEach(visit);
  // Services with only peer links sit in the column of a peer that has dependencies.
  model.linked.forEach(key => {
    if (model.deps.get(key).length || model.dependents.get(key).length) return;
    const anchor = [...model.peers.get(key)].find(p => model.deps.get(p).length || model.dependents.get(p).length);
    layer.set(key, anchor ? layer.get(anchor) : 0);
  });

  const layerCount = model.linked.length ? Math.max(...model.linked.map(k => layer.get(k))) + 1 : 0;
  const columns = Array.from({ length: layerCount }, () => []);
  const seed = new Map(model.order.map((k, i) => [k, i]));
  const items = new Map();
  const addItem = item => { items.set(item.id, item); columns[item.layer].push(item); };
  model.linked.forEach(key => addItem({ id: key, key, layer: layer.get(key), dummy: false, seed: seed.get(key) }));

  // 2. Dependency chains, with a lane in every column a link passes through.
  const upNbrs = new Map(), downNbrs = new Map();
  const connect = (a, b) => {
    if (!downNbrs.has(a)) downNbrs.set(a, []);
    if (!upNbrs.has(b)) upNbrs.set(b, []);
    downNbrs.get(a).push(b);
    upNbrs.get(b).push(a);
  };
  const chains = model.depEdges.map(edge => {
    const from = layer.get(edge.from), to = layer.get(edge.to);
    if (loopLinks.has(edge.from + '>' + edge.to) || to <= from) return { edge, ids: [edge.from, edge.to], loop: true };
    const ids = [edge.from];
    for (let l = from + 1; l < to; l++) {
      const id = '~' + edge.from + '>' + edge.to + '@' + l;
      addItem({ id, key: null, layer: l, dummy: true, seed: (seed.get(edge.from) + seed.get(edge.to)) / 2 });
      ids.push(id);
    }
    ids.push(edge.to);
    for (let i = 1; i < ids.length; i++) connect(ids[i - 1], ids[i]);
    return { edge, ids, loop: false };
  });
  columns.forEach(col => col.sort((a, b) => a.seed - b.seed));

  // 3. Barycentre sweeps, alternating direction.
  const relative = col => {
    const m = new Map();
    col.forEach((it, i) => m.set(it.id, col.length > 1 ? i / (col.length - 1) : 0.5));
    return m;
  };
  for (let pass = 0; pass < 8; pass++) {
    const downward = pass % 2 === 0;
    for (let step = 1; step < layerCount; step++) {
      const l = downward ? step : layerCount - 1 - step;
      const ref = relative(columns[downward ? l - 1 : l + 1]);
      const current = relative(columns[l]);
      const bary = new Map();
      columns[l].forEach(it => {
        const known = ((downward ? upNbrs : downNbrs).get(it.id) || []).filter(n => ref.has(n));
        bary.set(it.id, known.length ? known.reduce((s, n) => s + ref.get(n), 0) / known.length : current.get(it.id));
      });
      columns[l].sort((a, b) => (bary.get(a.id) - bary.get(b.id)) || (current.get(a.id) - current.get(b.id)));
    }
  }

  // 4. Coordinates. "Main" runs across layers, "cross" along a layer.
  const slotOf = it => (it.dummy ? S.dummySlot : S.nodeSlot);
  const extents = columns.map(col => col.reduce((sum, it) => sum + slotOf(it), 0));
  const crossContent = Math.max(0, ...extents);
  let width, height, mainAt;
  if (narrow) {
    width = Math.max(containerWidth, crossContent + 2 * S.padCross);
    height = 2 * S.padCross + (layerCount - 1) * S.rowGap + S.ring + S.label + 16;
    mainAt = l => S.padCross + 8 + R + l * S.rowGap;
  } else {
    const span = layerCount > 1 ? (containerWidth - 2 * S.padMain) / (layerCount - 1) : 0;
    const gap = Math.min(S.maxGap, Math.max(S.minGap, span));
    const contentW = 2 * S.padMain + (layerCount - 1) * gap;
    width = Math.max(containerWidth, contentW);
    height = crossContent + 2 * S.padCross;
    const start = (width - contentW) / 2 + S.padMain;
    mainAt = l => start + l * gap;
  }
  const crossSpace = narrow ? width : height;
  const point = new Map();
  columns.forEach((col, l) => {
    let at = (crossSpace - extents[l]) / 2;
    col.forEach(it => {
      const cross = it.dummy ? at + S.dummySlot / 2 : at + (narrow ? S.nodeSlot / 2 : 8 + R);
      at += slotOf(it);
      const main = mainAt(l);
      point.set(it.id, narrow ? { x: cross, y: main } : { x: main, y: cross });
    });
  });

  // 5. Link paths. Links leave a ring on its outgoing side (below the label on
  // narrow screens) and stop short of the next ring to leave room for the arrow.
  const leave = p => (narrow ? { x: p.x, y: p.y + R + S.label } : { x: p.x + R + 3, y: p.y });
  const arrive = p => (narrow ? { x: p.x, y: p.y - R - 7 } : { x: p.x - R - 7, y: p.y });
  const segment = (a, b) => (narrow
    ? ' C ' + a.x + ' ' + (a.y + b.y) / 2 + ' ' + b.x + ' ' + (a.y + b.y) / 2 + ' ' + b.x + ' ' + b.y
    : ' C ' + (a.x + b.x) / 2 + ' ' + a.y + ' ' + (a.x + b.x) / 2 + ' ' + b.y + ' ' + b.x + ' ' + b.y);
  // Two rings in the same layer (or a loop): a bow beside the column, or above
  // the row on narrow screens where labels sit under the rings.
  const bow = (a, b, gapEnd) => {
    const bend = 40 + Math.abs(narrow ? b.x - a.x : b.y - a.y) * 0.18;
    if (narrow) {
      const s = { x: a.x, y: a.y - R - 3 }, e = { x: b.x, y: b.y - R - gapEnd };
      return { d: 'M ' + s.x + ' ' + s.y + ' C ' + s.x + ' ' + (s.y - bend) + ' ' + e.x + ' ' + (e.y - bend) + ' ' + e.x + ' ' + e.y,
        mid: { x: (s.x + e.x) / 2, y: (s.y + e.y) / 2 - bend * 0.75 } };
    }
    const s = { x: a.x + R + 3, y: a.y }, e = { x: b.x + R + gapEnd, y: b.y };
    return { d: 'M ' + s.x + ' ' + s.y + ' C ' + (s.x + bend) + ' ' + s.y + ' ' + (e.x + bend) + ' ' + e.y + ' ' + e.x + ' ' + e.y,
      mid: { x: (s.x + e.x) / 2 + bend * 0.75, y: (s.y + e.y) / 2 } };
  };
  const along = pts => {
    let d = 'M ' + pts[0].x + ' ' + pts[0].y;
    for (let i = 1; i < pts.length; i++) d += segment(pts[i - 1], pts[i]);
    const m = Math.floor((pts.length - 1) / 2);
    return { d, mid: { x: (pts[m].x + pts[m + 1].x) / 2, y: (pts[m].y + pts[m + 1].y) / 2 } };
  };

  const routes = [];
  chains.forEach(chain => {
    const { from, to } = chain.edge;
    const geo = chain.loop
      ? bow(point.get(from), point.get(to), 7)
      : along(chain.ids.map((id, i) => (i === 0 ? leave(point.get(id)) : i === chain.ids.length - 1 ? arrive(point.get(id)) : point.get(id))));
    routes.push({ type: 'dep', from, to, loop: chain.loop, d: geo.d, mid: geo.mid, length: geo.d.length });
  });
  model.peerEdges.forEach(edge => {
    let a = point.get(edge.from), b = point.get(edge.to);
    const sameLayer = layer.get(edge.from) === layer.get(edge.to);
    if (!sameLayer && layer.get(edge.from) > layer.get(edge.to)) [a, b] = [b, a];
    const geo = sameLayer ? bow(a, b, 3) : along([leave(a), { x: arrive(b).x + (narrow ? 0 : 4), y: arrive(b).y + (narrow ? 4 : 0) }]);
    routes.push({ type: 'peer', from: edge.from, to: edge.to, loop: false, d: geo.d, mid: geo.mid });
  });

  const pos = new Map();
  model.linked.forEach(key => pos.set(key, point.get(key)));
  return { narrow, ring: S.ring, width: Math.round(width), height: Math.round(height), pos, routes, layerCount };
}

/* ── Rendering ──────────────────────────────────────────── */
function topologyMotionAllowed() {
  return !(typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
}

function topologySvg(tag, attrs) {
  const el = document.createElementNS(TOPO_SVG_NS, tag);
  Object.keys(attrs || {}).forEach(name => el.setAttribute(name, attrs[name]));
  return el;
}

function topologyName(key) {
  const svc = topoState && topoState.model.byKey.get(key);
  return svc ? (svc.name || svc.key) : key;
}

function topologyStatusText(st) {
  const ms = st.ms != null ? st.ms + ' ms' : '';
  switch (st.statusClass) {
    case 'up': return ms || 'Operational';
    case 'degraded': return ms ? 'Degraded · ' + ms : 'Degraded';
    case 'unknown': return 'No data yet';
    default: return st.statusLabel;
  }
}

function topologyNode(svc) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'topo-node';
  btn.dataset.key = svc.key;
  const ring = document.createElement('span');
  ring.className = 'matrix-node-ring unknown';
  ring.innerHTML = serviceIconMarkup(svc);
  const name = document.createElement('span');
  name.className = 'topo-node-name';
  name.textContent = svc.name || svc.key;
  const sub = document.createElement('span');
  sub.className = 'topo-node-sub';
  btn.append(ring, name, sub);
  return btn;
}

function topologyMarkers() {
  const defs = topologySvg('defs');
  ['ok', 'warn', 'down', 'off', 'idle'].forEach(state => {
    // 9px arrowheads whose tip lands just short of the ring (links stop 7px out).
    const marker = topologySvg('marker', {
      id: 'topo-arrow-' + state, class: 'topo-arrow topo-arrow-' + state,
      viewBox: '0 0 10 10', refX: '3', refY: '5', markerWidth: '9', markerHeight: '9',
      markerUnits: 'userSpaceOnUse', orient: 'auto'
    });
    marker.appendChild(topologySvg('path', { d: 'M 0 0 L 10 5 L 0 10 z' }));
    defs.appendChild(marker);
  });
  return defs;
}

function topologyLegend(adminHint) {
  const legend = document.createElement('div');
  legend.className = 'topo-legend';
  legend.innerHTML =
    '<span class="topo-legend-item"><svg class="topo-legend-swatch" viewBox="0 0 34 10" aria-hidden="true">' +
      '<line x1="1" y1="5" x2="25" y2="5" class="topo-legend-dep"/><path d="M 25 1 L 33 5 L 25 9 z" class="topo-legend-arrow"/></svg>Needed by</span>' +
    '<span class="topo-legend-item"><svg class="topo-legend-swatch" viewBox="0 0 34 10" aria-hidden="true">' +
      '<line x1="1" y1="5" x2="33" y2="5" class="topo-legend-peer"/></svg>Connected to</span>' +
    '<span class="topo-legend-item"><svg class="topo-legend-swatch" viewBox="0 0 34 10" aria-hidden="true">' +
      '<line x1="1" y1="5" x2="33" y2="5" class="topo-legend-down"/><path d="M 13 1 L 21 9 M 21 1 L 13 9" class="topo-legend-x"/></svg>Link down</span>' +
    '<span class="topo-legend-hint">Hover or tab to a service to trace its links.' + (adminHint ? ' Click a service to edit them.' : '') + '</span>';
  return legend;
}

function renderMatrix() {
  const container = $('#matrix-container');
  if (!container) return;
  const services = Array.isArray(servicesData) ? servicesData : [];
  if (services.length === 0) {
    container.innerHTML = '<div class="topo-empty">No services configured</div>';
    topoState = null;
    return;
  }
  const width = Math.round(container.clientWidth || container.getBoundingClientRect().width || 960);
  const signature = JSON.stringify([
    width < TOPO_NARROW_BELOW, Math.round(width / 40),
    !!(typeof isAdminUser !== 'undefined' && isAdminUser),
    services.map(s => [s.key, s.name, s.service_type, s.icon_url, s.depends_on, s.connected_to])
  ]);
  if (topoState && topoState.signature === signature && container.contains(topoState.root)) {
    updateTopologyStatus();
    return;
  }
  buildTopology(container, services, width, signature);
}

function buildTopology(container, services, width, signature) {
  const model = buildTopologyModel(services);
  const layout = layoutTopology(model, width);
  const admin = typeof isAdminUser !== 'undefined' && isAdminUser && typeof openServiceModal === 'function';
  const root = document.createElement('div');
  root.className = 'topo' + (layout.narrow ? ' is-narrow' : '');
  root.setAttribute('role', 'group');
  root.setAttribute('aria-label', 'Service topology');
  const nodes = new Map();
  const remember = (key, btn) => { if (!nodes.has(key)) nodes.set(key, []); nodes.get(key).push(btn); };
  const edges = [];

  if (model.linked.length) {
    const scroller = document.createElement('div');
    scroller.className = 'topo-scroll';
    const stage = document.createElement('div');
    stage.className = 'topo-stage';
    stage.style.width = layout.width + 'px';
    stage.style.height = layout.height + 'px';
    const svg = topologySvg('svg', {
      class: 'topo-edges', width: layout.width, height: layout.height,
      viewBox: '0 0 ' + layout.width + ' ' + layout.height, 'aria-hidden': 'true', focusable: 'false'
    });
    svg.appendChild(topologyMarkers());
    layout.routes.forEach((route, i) => {
      const g = topologySvg('g', { class: 'topo-edge topo-edge--' + route.type + (route.loop ? ' is-loop' : '') });
      const path = topologySvg('path', { id: 'topo-edge-' + i, class: 'topo-edge-line', d: route.d });
      g.appendChild(path);
      svg.appendChild(g);
      edges.push(Object.assign({ g, path, state: null, particle: null, cross: null }, route));
    });
    stage.appendChild(svg);

    // Tab order follows the columns, top to bottom.
    [...layout.pos.entries()]
      .sort((a, b) => (layout.narrow ? (a[1].y - b[1].y) || (a[1].x - b[1].x) : (a[1].x - b[1].x) || (a[1].y - b[1].y)))
      .forEach(([key, p]) => {
        const btn = topologyNode(model.byKey.get(key));
        btn.style.left = p.x + 'px';
        btn.style.top = (p.y - layout.ring / 2) + 'px';
        stage.appendChild(btn);
        remember(key, btn);
      });

    const card = document.createElement('div');
    card.className = 'topo-card';
    card.hidden = true;
    stage.appendChild(card);
    scroller.appendChild(stage);
    root.appendChild(scroller);
  }

  if (model.standalone.length) {
    const section = document.createElement('div');
    section.className = 'topo-standalone';
    if (model.linked.length) {
      const title = document.createElement('div');
      title.className = 'topo-standalone-title';
      title.textContent = 'Not linked';
      section.appendChild(title);
    }
    const row = document.createElement('div');
    row.className = 'topo-standalone-nodes';
    model.standalone.forEach(key => {
      const btn = topologyNode(model.byKey.get(key));
      row.appendChild(btn);
      remember(key, btn);
    });
    section.appendChild(row);
    if (!model.linked.length && admin) {
      const hint = document.createElement('p');
      hint.className = 'topo-empty-hint';
      hint.textContent = 'No links yet. Click a service, then use Links in its settings to say what it depends on.';
      section.appendChild(hint);
    }
    root.appendChild(section);
  }

  if (model.linked.length) root.appendChild(topologyLegend(admin));

  container.innerHTML = '';
  container.appendChild(root);
  topoState = { signature, model, layout, root, nodes, edges, admin, card: root.querySelector('.topo-card'), focusKey: null, pinned: null, status: new Map() };
  bindTopologyEvents(root);
  observeTopologyResize(container);
  updateTopologyStatus();
}

/* ── Live status ────────────────────────────────────────── */
function topologyEdgeState(edge, status) {
  const cls = key => (status.get(key) || {}).statusClass || 'unknown';
  const states = edge.type === 'dep' ? [cls(edge.from)] : [cls(edge.from), cls(edge.to)];
  if (states.includes('down')) return 'down';
  if (states.includes('degraded') || states.includes('maintenance')) return 'warn';
  if (states.includes('disabled')) return 'off';
  if (states.includes('unknown')) return 'idle';
  return 'ok';
}

function setTopologyEdgeState(edge, state) {
  if (edge.state === state) return;
  if (edge.state) edge.g.classList.remove('is-' + edge.state);
  edge.g.classList.add('is-' + state);
  edge.state = state;
  if (edge.type === 'dep') edge.path.setAttribute('marker-end', 'url(#topo-arrow-' + state + ')');

  // A light travels along healthy dependency links, from the service relied on.
  const wantsParticle = state === 'ok' && edge.type === 'dep' && !edge.loop && topologyMotionAllowed();
  if (wantsParticle && !edge.particle) {
    const dot = topologySvg('circle', { r: '2.6', class: 'topo-particle' });
    const motion = topologySvg('animateMotion', {
      dur: Math.min(6, Math.max(2.2, edge.length / 140)).toFixed(2) + 's',
      repeatCount: 'indefinite', begin: '-' + (Math.random() * 3).toFixed(2) + 's'
    });
    const mpath = topologySvg('mpath');
    mpath.setAttribute('href', '#' + edge.path.id);
    mpath.setAttributeNS(TOPO_XLINK_NS, 'xlink:href', '#' + edge.path.id);
    motion.appendChild(mpath);
    dot.appendChild(motion);
    edge.g.appendChild(dot);
    edge.particle = dot;
  } else if (!wantsParticle && edge.particle) {
    edge.particle.remove();
    edge.particle = null;
  }

  // A cross on a broken link, so "down" doesn't rely on colour alone.
  if (state === 'down' && !edge.cross) {
    const s = 4.5;
    edge.cross = topologySvg('path', {
      class: 'topo-edge-cross',
      d: 'M ' + (edge.mid.x - s) + ' ' + (edge.mid.y - s) + ' L ' + (edge.mid.x + s) + ' ' + (edge.mid.y + s) +
        ' M ' + (edge.mid.x + s) + ' ' + (edge.mid.y - s) + ' L ' + (edge.mid.x - s) + ' ' + (edge.mid.y + s)
    });
    edge.g.appendChild(edge.cross);
  } else if (state !== 'down' && edge.cross) {
    edge.cross.remove();
    edge.cross = null;
  }
}

function topologyDescription(key) {
  const { model } = topoState;
  const st = topoState.status.get(key) || matrixStatusOf(model.byKey.get(key));
  const list = keys => keys.map(topologyName).join(', ');
  const parts = [topologyName(key) + ', ' + st.statusLabel + (st.ms != null ? ', ' + st.ms + ' ms' : '') + '.'];
  if (model.deps.get(key).length) parts.push('Depends on ' + list(model.deps.get(key)) + '.');
  if (model.dependents.get(key).length) parts.push('Needed by ' + list(model.dependents.get(key)) + '.');
  if (model.peers.get(key).size) parts.push('Connected to ' + list([...model.peers.get(key)]) + '.');
  return parts.join(' ');
}

function updateTopologyStatus() {
  if (!topoState) return;
  const { model } = topoState;
  model.byKey.forEach((svc, key) => topoState.status.set(key, matrixStatusOf(svc)));
  topoState.nodes.forEach((buttons, key) => {
    const st = topoState.status.get(key);
    buttons.forEach(btn => {
      btn.querySelector('.matrix-node-ring').className = 'matrix-node-ring ' + st.statusClass;
      btn.dataset.status = st.statusClass;
      btn.querySelector('.topo-node-sub').textContent = topologyStatusText(st);
      btn.setAttribute('aria-label', topologyDescription(key));
    });
  });
  topoState.edges.forEach(edge => setTopologyEdgeState(edge, topologyEdgeState(edge, topoState.status)));
  if (topoState.focusKey && topoState.card && !topoState.card.hidden) fillTopologyCard(topoState.focusKey);
}

/* ── Tracing a service's links ──────────────────────────── */
function topologyRelations(key) {
  const { model } = topoState;
  const walk = map => {
    const seen = new Set();
    const stack = [...map.get(key)];
    while (stack.length) {
      const next = stack.pop();
      if (next === key || seen.has(next)) continue;
      seen.add(next);
      stack.push(...map.get(next));
    }
    return seen;
  };
  return { upstream: walk(model.deps), downstream: walk(model.dependents), peers: new Set(model.peers.get(key)) };
}

function traceTopology(key) {
  if (!topoState) return;
  const { root, nodes, edges } = topoState;
  topoState.focusKey = key;
  root.classList.toggle('is-tracing', !!key);
  root.querySelectorAll('.is-active, .is-focus').forEach(el => el.classList.remove('is-active', 'is-focus'));
  if (!key) {
    if (topoState.card) topoState.card.hidden = true;
    return;
  }
  const rel = topologyRelations(key);
  const active = new Set([key, ...rel.upstream, ...rel.downstream, ...rel.peers]);
  nodes.forEach((buttons, k) => buttons.forEach(btn => {
    if (active.has(k)) btn.classList.add('is-active');
    if (k === key) btn.classList.add('is-focus');
  }));
  edges.forEach(edge => {
    const on = edge.type === 'dep'
      ? (rel.upstream.has(edge.from) && (edge.to === key || rel.upstream.has(edge.to))) ||
        (rel.downstream.has(edge.to) && (edge.from === key || rel.downstream.has(edge.from)))
      : (edge.from === key && rel.peers.has(edge.to)) || (edge.to === key && rel.peers.has(edge.from));
    if (on) edge.g.classList.add('is-active');
  });
  showTopologyCard(key);
}

function fillTopologyCard(key) {
  const { card, model } = topoState;
  if (!card) return;
  const st = topoState.status.get(key) || matrixStatusOf(model.byKey.get(key));
  card.textContent = '';
  const title = document.createElement('div');
  title.className = 'topo-card-title';
  title.textContent = topologyName(key);
  const status = document.createElement('div');
  status.className = 'topo-card-status is-' + st.statusClass;
  status.textContent = st.statusLabel + (st.ms != null ? ' · ' + st.ms + ' ms' : '');
  card.append(title, status);
  const rows = [
    ['Depends on', model.deps.get(key)],
    ['Needed by', model.dependents.get(key)],
    ['Connected to', [...model.peers.get(key)]]
  ].filter(([, keys]) => keys.length);
  if (rows.length) {
    const dl = document.createElement('dl');
    rows.forEach(([label, keys]) => {
      const dt = document.createElement('dt');
      dt.textContent = label;
      const dd = document.createElement('dd');
      dd.textContent = keys.map(topologyName).join(', ');
      dl.append(dt, dd);
    });
    card.appendChild(dl);
  }
  if (topoState.admin) {
    const hint = document.createElement('div');
    hint.className = 'topo-card-hint';
    hint.textContent = 'Click to edit links';
    card.appendChild(hint);
  }
}

function showTopologyCard(key) {
  const { card, nodes } = topoState;
  const btn = card && (nodes.get(key) || []).find(b => card.parentNode.contains(b));
  if (!card || !btn) return;
  fillTopologyCard(key);
  card.hidden = false;
  const stage = card.parentNode;
  const ring = topoState.layout.ring;
  const x = btn.offsetLeft, y = btn.offsetTop;
  const w = card.offsetWidth || 230, h = card.offsetHeight || 120;
  const right = x + ring / 2 + 14;
  const left = right + w > stage.offsetWidth - 8 ? x - ring / 2 - 14 - w : right;
  card.style.left = Math.max(8, left) + 'px';
  card.style.top = Math.max(8, Math.min(y - 6, stage.offsetHeight - h - 8)) + 'px';
}

function bindTopologyEvents(root) {
  const nodeOf = el => el && el.closest ? el.closest('.topo-node') : null;
  // Only services on the map have links to trace.
  const traceable = btn => btn && topoState.layout.pos.has(btn.dataset.key);
  root.addEventListener('mouseover', e => {
    const btn = nodeOf(e.target);
    if (traceable(btn) && !topoState.pinned) traceTopology(btn.dataset.key);
  });
  root.addEventListener('mouseout', e => {
    const btn = nodeOf(e.target);
    if (!btn || btn.contains(e.relatedTarget) || topoState.pinned) return;
    const focused = root.contains(document.activeElement) ? nodeOf(document.activeElement) : null;
    traceTopology(traceable(focused) ? focused.dataset.key : null);
  });
  root.addEventListener('focusin', e => {
    const btn = nodeOf(e.target);
    if (!topoState.pinned) traceTopology(traceable(btn) ? btn.dataset.key : null);
  });
  root.addEventListener('focusout', e => {
    if (!root.contains(e.relatedTarget) && !topoState.pinned) traceTopology(null);
  });
  root.addEventListener('click', e => {
    const btn = nodeOf(e.target);
    if (!btn) {
      topoState.pinned = null;
      traceTopology(null);
      return;
    }
    const key = btn.dataset.key;
    if (topoState.admin) {
      openTopologyEditor(key);
      return;
    }
    if (!traceable(btn)) return;
    topoState.pinned = topoState.pinned === key ? null : key;
    traceTopology(topoState.pinned);
  });
  root.addEventListener('keydown', e => {
    if (e.key !== 'Escape') return;
    topoState.pinned = null;
    traceTopology(null);
  });
}

// Admins jump from the map straight to the service's links. The editor needs
// the admin record: the public one has no URL and omits links to hidden services.
async function openTopologyEditor(key) {
  if (typeof openServiceModal !== 'function') return;
  const list = Array.isArray(adminServicesData) ? adminServicesData : await loadAllServices();
  const svc = (list || []).find(s => s.key === key);
  if (!svc) return;
  openServiceModal(svc);
  const links = document.getElementById('serviceLinks');
  if (!links) return;
  if (typeof links.scrollIntoView === 'function') links.scrollIntoView({ block: 'start' });
  const first = [...links.querySelectorAll('.link-picker-search, .depends-on-cb')].find(el => !el.hidden && !el.disabled);
  if (first) first.focus({ preventScroll: true });
}

let topoResizeObserver = null;
function observeTopologyResize(container) {
  if (topoResizeObserver || typeof ResizeObserver !== 'function') return;
  let pending = null;
  topoResizeObserver = new ResizeObserver(() => {
    if (currentView !== 'matrix') return;
    clearTimeout(pending);
    pending = setTimeout(renderMatrix, 150);
  });
  topoResizeObserver.observe(container);
}

// Kept for the view switch: clears any traced service and its card.
function stopMatrixAnimation() {
  if (!topoState) return;
  topoState.pinned = null;
  traceTopology(null);
}

// Detect the actual protocol from URL and check_type
