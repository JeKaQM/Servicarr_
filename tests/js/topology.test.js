/**
 * Tests for the topology map in matrix.js: the link model, the layered
 * layout, rendering, live status and tracing a service's links.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'services.js', 'matrix.js');
});

// router → nas/proxmox/pihole → plex/prowlarr → sonarr/overseerr, plus one
// service without links. Sonarr's link to the NAS skips a column.
const SAMPLE = [
  { key: 'router', name: 'Router' },
  { key: 'nas', name: 'NAS', depends_on: 'router' },
  { key: 'proxmox', name: 'Proxmox', depends_on: 'router' },
  { key: 'pihole', name: 'Pi-hole', depends_on: 'router' },
  { key: 'plex', name: 'Plex', depends_on: 'nas,proxmox' },
  { key: 'prowlarr', name: 'Prowlarr', depends_on: 'proxmox' },
  { key: 'sonarr', name: 'Sonarr', depends_on: 'nas,prowlarr', connected_to: 'plex' },
  { key: 'overseerr', name: 'Overseerr', depends_on: 'plex', connected_to: 'sonarr' },
  { key: 'demo', name: 'Demo Service' },
];

const LIVE = {
  router: { ok: true, ms: 3 },
  nas: { ok: false },
  proxmox: { ok: true, ms: 12 },
  pihole: { ok: true, ms: 4 },
  plex: { ok: true, ms: 40 },
  prowlarr: { ok: true, ms: 20 },
  sonarr: { ok: true, degraded: true, ms: 900 },
  overseerr: { ok: true, ms: 30 },
  demo: { ok: true, ms: 1 },
};

function render(services, live, opts = {}) {
  document.body.innerHTML = '<div id="matrix-container"></div>';
  const container = document.getElementById('matrix-container');
  if (opts.width) Object.defineProperty(container, 'clientWidth', { value: opts.width });
  globalThis.servicesData = services;
  globalThis.latestLiveStatus = live || null;
  globalThis.isAdminUser = !!opts.admin;
  globalThis.topoState = null;
  renderMatrix();
  return container;
}

const node = (container, key) => container.querySelector(`.topo-node[data-key="${key}"]`);
const edge = (from, to) => topoState.edges.find(e => e.from === from && e.to === to);

/* ── buildTopologyModel ─────────────────────────────────── */
describe('buildTopologyModel', () => {
  test('dependency links run from the service relied on to the one that needs it', () => {
    const model = buildTopologyModel(SAMPLE);
    expect(model.depEdges).toHaveLength(9);
    expect(model.depEdges).toContainEqual({ type: 'dep', from: 'router', to: 'nas' });
    expect(model.deps.get('plex')).toEqual(['nas', 'proxmox']);
    expect(model.dependents.get('nas')).toEqual(['plex', 'sonarr']);
  });

  test('peer links are undirected and counted once', () => {
    const model = buildTopologyModel([
      { key: 'a', connected_to: 'b' },
      { key: 'b', connected_to: 'a' },
    ]);
    expect(model.peerEdges).toHaveLength(1);
    expect([...model.peers.get('a')]).toEqual(['b']);
    expect([...model.peers.get('b')]).toEqual(['a']);
  });

  test('self links, unknown services and repeats are ignored', () => {
    const model = buildTopologyModel([
      { key: 'router' },
      { key: 'x', depends_on: 'x, ghost, router, router', connected_to: 'x,ghost' },
    ]);
    expect(model.deps.get('x')).toEqual(['router']);
    expect(model.depEdges).toHaveLength(1);
    expect(model.peerEdges).toHaveLength(0);
  });

  test('services without links are kept apart', () => {
    const model = buildTopologyModel(SAMPLE);
    expect(model.standalone).toEqual(['demo']);
    expect(model.linked).toHaveLength(8);
  });
});

/* ── layoutTopology ─────────────────────────────────────── */
describe('layoutTopology', () => {
  let model;
  beforeAll(() => {
    model = buildTopologyModel(SAMPLE);
  });

  test('wide screens put every service to the right of what it depends on', () => {
    const layout = layoutTopology(model, 1200);
    expect(layout.narrow).toBe(false);
    expect(layout.layerCount).toBe(4);
    model.depEdges.forEach(({ from, to }) => {
      expect(layout.pos.get(to).x).toBeGreaterThan(layout.pos.get(from).x);
    });
  });

  test('services never overlap and stay inside the stage', () => {
    const layout = layoutTopology(model, 1200);
    const points = [...layout.pos.values()];
    points.forEach((a, i) => points.slice(i + 1).forEach(b => {
      if (Math.abs(a.x - b.x) < 1) expect(Math.abs(a.y - b.y)).toBeGreaterThanOrEqual(TOPO_SIZES.wide.nodeSlot - 0.5);
    }));
    points.forEach(p => {
      expect(p.x - 56).toBeGreaterThanOrEqual(0);
      expect(p.x + 56).toBeLessThanOrEqual(layout.width);
      expect(p.y - layout.ring / 2).toBeGreaterThanOrEqual(0);
      expect(p.y + layout.ring / 2 + TOPO_SIZES.wide.label).toBeLessThanOrEqual(layout.height);
    });
  });

  test('a link that skips a column bends through a lane in it', () => {
    const layout = layoutTopology(model, 1200);
    const skip = layout.routes.find(r => r.from === 'nas' && r.to === 'sonarr');
    const direct = layout.routes.find(r => r.from === 'router' && r.to === 'nas');
    expect(skip.d.match(/ C /g)).toHaveLength(2);
    expect(direct.d.match(/ C /g)).toHaveLength(1);
  });

  test('every link gets a route, peers included', () => {
    const layout = layoutTopology(model, 1200);
    expect(layout.routes.filter(r => r.type === 'dep')).toHaveLength(9);
    expect(layout.routes.filter(r => r.type === 'peer')).toHaveLength(2);
    layout.routes.forEach(r => expect(r.d).toMatch(/^M [\d.-]+ [\d.-]+/));
  });

  test('narrow screens stack the columns as rows, top to bottom', () => {
    const layout = layoutTopology(model, 390);
    expect(layout.narrow).toBe(true);
    model.depEdges.forEach(({ from, to }) => {
      expect(layout.pos.get(to).y).toBeGreaterThan(layout.pos.get(from).y);
    });
    expect(layout.pos.get('nas').y).toBe(layout.pos.get('pihole').y);
  });

  test('a dependency loop in stored data still lays out', () => {
    const loop = buildTopologyModel([
      { key: 'a', depends_on: 'b' },
      { key: 'b', depends_on: 'a' },
    ]);
    const layout = layoutTopology(loop, 900);
    expect(layout.pos.size).toBe(2);
    expect(layout.routes.some(r => r.loop)).toBe(true);
  });
});

/* ── renderMatrix ───────────────────────────────────────── */
describe('renderMatrix', () => {
  test('draws linked services on the map and the rest in a row below', () => {
    const container = render(SAMPLE, LIVE);
    expect(container.querySelectorAll('.topo-stage .topo-node')).toHaveLength(8);
    expect(container.querySelectorAll('.topo-standalone-nodes .topo-node')).toHaveLength(1);
    expect(container.querySelector('.topo-standalone-title').textContent).toBe('Not linked');
    expect(container.querySelectorAll('.topo-edge--dep')).toHaveLength(9);
    expect(container.querySelectorAll('.topo-edge--peer')).toHaveLength(2);
    expect(container.querySelectorAll('marker')).toHaveLength(5);
  });

  test('the legend explains line styles and the public hint has no edit prompt', () => {
    const container = render(SAMPLE, LIVE);
    const legend = container.querySelector('.topo-legend').textContent;
    expect(legend).toContain('Needed by');
    expect(legend).toContain('Connected to');
    expect(legend).toContain('Link down');
    expect(legend).not.toContain('Click a service');
  });

  test('a down service shows its status in words and breaks its outgoing links', () => {
    const container = render(SAMPLE, LIVE);
    const nas = node(container, 'nas');
    expect(nas.querySelector('.matrix-node-ring').classList.contains('down')).toBe(true);
    expect(nas.querySelector('.topo-node-sub').textContent).toBe('Down');
    expect(nas.getAttribute('aria-label')).toBe('NAS, Down. Depends on Router. Needed by Plex, Sonarr.');
    const broken = edge('nas', 'plex');
    expect(broken.g.classList.contains('is-down')).toBe(true);
    expect(broken.g.querySelector('.topo-edge-cross')).not.toBeNull();
    expect(broken.path.getAttribute('marker-end')).toBe('url(#topo-arrow-down)');
  });

  test('healthy links carry a moving light; peer links take the worse state', () => {
    const container = render(SAMPLE, LIVE);
    const ok = edge('router', 'nas');
    expect(ok.g.classList.contains('is-ok')).toBe(true);
    const mpath = ok.g.querySelector('.topo-particle animateMotion mpath');
    expect(mpath.getAttribute('href')).toBe('#' + ok.path.id);
    expect(edge('sonarr', 'plex').g.classList.contains('is-warn')).toBe(true);
    expect(node(container, 'plex').querySelector('.topo-node-sub').textContent).toBe('40 ms');
    expect(node(container, 'sonarr').querySelector('.topo-node-sub').textContent).toBe('Degraded · 900 ms');
  });

  test('services without live data say so', () => {
    const container = render(SAMPLE, null);
    expect(node(container, 'router').querySelector('.topo-node-sub').textContent).toBe('No data yet');
    expect(edge('router', 'nas').g.classList.contains('is-idle')).toBe(true);
  });

  test('a status refresh updates the map in place', () => {
    const container = render(SAMPLE, LIVE);
    const root = container.querySelector('.topo');
    globalThis.latestLiveStatus = { ...LIVE, nas: { ok: true, ms: 5 } };
    renderMatrix();
    expect(container.querySelector('.topo')).toBe(root);
    expect(node(container, 'nas').querySelector('.matrix-node-ring').classList.contains('up')).toBe(true);
    expect(edge('nas', 'plex').g.classList.contains('is-ok')).toBe(true);
    expect(edge('nas', 'plex').g.querySelector('.topo-edge-cross')).toBeNull();
  });

  test('changed links rebuild the map', () => {
    const container = render(SAMPLE, LIVE);
    const root = container.querySelector('.topo');
    globalThis.servicesData = SAMPLE.map(s => (s.key === 'demo' ? { ...s, depends_on: 'router' } : s));
    renderMatrix();
    expect(container.querySelector('.topo')).not.toBe(root);
    expect(container.querySelector('.topo-standalone')).toBeNull();
    expect(container.querySelectorAll('.topo-edge--dep')).toHaveLength(10);
  });

  test('narrow containers get the stacked layout', () => {
    const container = render(SAMPLE, LIVE, { width: 390 });
    expect(container.querySelector('.topo').classList.contains('is-narrow')).toBe(true);
  });

  test('no services, and services without any links', () => {
    expect(render([], null).textContent).toBe('No services configured');
    globalThis.openServiceModal = jest.fn();
    try {
      const container = render([{ key: 'a', name: 'A' }, { key: 'b', name: 'B' }], null, { admin: true });
      expect(container.querySelector('.topo-stage')).toBeNull();
      expect(container.querySelector('.topo-legend')).toBeNull();
      expect(container.querySelector('.topo-standalone-title')).toBeNull();
      expect(container.querySelectorAll('.topo-node')).toHaveLength(2);
      expect(container.querySelector('.topo-empty-hint').textContent).toContain('use Links in its settings');
    } finally {
      delete globalThis.openServiceModal;
    }
  });
});

/* ── Tracing ────────────────────────────────────────────── */
describe('tracing a service', () => {
  const hover = el => el.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }));
  const active = container => [...container.querySelectorAll('.topo-node.is-active')].map(n => n.dataset.key).sort();

  test('hovering lights up what it depends on, what needs it and its peers', () => {
    const container = render(SAMPLE, LIVE);
    hover(node(container, 'plex'));
    expect(container.querySelector('.topo').classList.contains('is-tracing')).toBe(true);
    expect(node(container, 'plex').classList.contains('is-focus')).toBe(true);
    expect(active(container)).toEqual(['nas', 'overseerr', 'plex', 'proxmox', 'router', 'sonarr']);
    expect(edge('router', 'nas').g.classList.contains('is-active')).toBe(true);
    expect(edge('plex', 'overseerr').g.classList.contains('is-active')).toBe(true);
    expect(edge('sonarr', 'plex').g.classList.contains('is-active')).toBe(true);
    expect(edge('router', 'pihole').g.classList.contains('is-active')).toBe(false);
    expect(edge('nas', 'sonarr').g.classList.contains('is-active')).toBe(false);
  });

  test('the card lists the links in words', () => {
    const container = render(SAMPLE, LIVE);
    hover(node(container, 'plex'));
    const card = container.querySelector('.topo-card');
    expect(card.hidden).toBe(false);
    const rows = [...card.querySelectorAll('dt')].map(dt => dt.textContent + ': ' + dt.nextElementSibling.textContent);
    expect(rows).toEqual(['Depends on: NAS, Proxmox', 'Needed by: Overseerr', 'Connected to: Sonarr']);
    expect(card.querySelector('.topo-card-status').textContent).toBe('Operational · 40 ms');
  });

  test('leaving the service or pressing Escape clears the trace', () => {
    const container = render(SAMPLE, LIVE);
    const root = container.querySelector('.topo');
    hover(node(container, 'plex'));
    node(container, 'plex').dispatchEvent(new MouseEvent('mouseout', { bubbles: true, relatedTarget: root }));
    expect(root.classList.contains('is-tracing')).toBe(false);
    expect(container.querySelector('.topo-card').hidden).toBe(true);
    hover(node(container, 'nas'));
    root.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(root.classList.contains('is-tracing')).toBe(false);
  });

  test('keyboard focus traces too', () => {
    const container = render(SAMPLE, LIVE);
    node(container, 'pihole').dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    expect(active(container)).toEqual(['pihole', 'router']);
  });

  test('services without links have nothing to trace', () => {
    const container = render(SAMPLE, LIVE);
    hover(node(container, 'demo'));
    expect(container.querySelector('.topo').classList.contains('is-tracing')).toBe(false);
  });

  test('a click pins the trace for visitors, a second click releases it', () => {
    const container = render(SAMPLE, LIVE);
    const root = container.querySelector('.topo');
    const plex = node(container, 'plex');
    plex.click();
    plex.dispatchEvent(new MouseEvent('mouseout', { bubbles: true, relatedTarget: root }));
    hover(node(container, 'pihole'));
    expect(node(container, 'plex').classList.contains('is-focus')).toBe(true);
    plex.click();
    expect(root.classList.contains('is-tracing')).toBe(false);
  });

  test('switching views clears a pinned trace', () => {
    const container = render(SAMPLE, LIVE);
    node(container, 'plex').click();
    stopMatrixAnimation();
    expect(container.querySelector('.topo').classList.contains('is-tracing')).toBe(false);
    expect(topoState.pinned).toBeNull();
  });
});

/* ── Admin: click to edit links ─────────────────────────── */
describe('editing links from the map', () => {
  const ADMIN = SAMPLE.map(s => ({ ...s, url: 'http://' + s.key + '.lan' }));

  afterEach(() => {
    globalThis.adminServicesData = null;
    delete globalThis.openServiceModal;
  });

  test('admins see the edit hint, and a click opens the full admin record', async () => {
    globalThis.openServiceModal = jest.fn();
    globalThis.adminServicesData = ADMIN;
    const container = render(SAMPLE, LIVE, { admin: true });
    expect(container.querySelector('.topo-legend-hint').textContent).toContain('Click a service to edit them.');
    node(container, 'plex').click();
    await Promise.resolve();
    expect(openServiceModal).toHaveBeenCalledWith(expect.objectContaining({ key: 'plex', url: 'http://plex.lan' }));
  });

  test('the admin list is fetched when it has not loaded yet', async () => {
    globalThis.openServiceModal = jest.fn();
    const realLoad = globalThis.loadAllServices;
    globalThis.loadAllServices = jest.fn().mockResolvedValue(ADMIN);
    try {
      const container = render(SAMPLE, LIVE, { admin: true });
      node(container, 'demo').click();
      await new Promise(r => setTimeout(r, 0));
      expect(loadAllServices).toHaveBeenCalled();
      expect(openServiceModal).toHaveBeenCalledWith(expect.objectContaining({ key: 'demo', url: 'http://demo.lan' }));
    } finally {
      globalThis.loadAllServices = realLoad;
    }
  });
});
