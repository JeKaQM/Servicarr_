/**
 * Tests for crowdsec-map.js – projection math, strike queuing, map-home
 * defaults, and the fresh-alert diffing that powers the animated arcs.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'crowdsec-tab.js', 'crowdsec-map.js');
});

/* ── crowdsecProject ───────────────────────────────────── */
describe('crowdsecProject', () => {
  test('converts lat/lng to normalized map space', () => {
    const p = crowdsecProject(0, 0);
    expect(p.x).toBeCloseTo(0.5);
    expect(p.y).toBeCloseTo(0.5);
  });

  test('top-left corner of the map', () => {
    const p = crowdsecProject(90, -180);
    expect(p.x).toBeCloseTo(0);
    expect(p.y).toBeCloseTo(0);
  });

  test('bottom-right corner of the map', () => {
    const p = crowdsecProject(-90, 180);
    expect(p.x).toBeCloseTo(1);
    expect(p.y).toBeCloseTo(1);
  });

  test('London lands in the north-east quadrant', () => {
    const p = crowdsecProject(51.5074, -0.1278);
    expect(p.x).toBeGreaterThan(0.45);
    expect(p.y).toBeLessThan(0.3);
  });
});

/* ── land tiles ────────────────────────────────────────── */
describe('CROWDSEC_LAND_TILES', () => {
  test('covers continents with a reasonable dot count', () => {
    expect(CROWDSEC_LAND_TILES.length).toBeGreaterThan(150);
    expect(CROWDSEC_LAND_TILES.length).toBeLessThan(2000);
  });

  test('every tile is within valid geo bounds', () => {
    for (const [lat, lng] of CROWDSEC_LAND_TILES) {
      expect(lat).toBeGreaterThanOrEqual(-90);
      expect(lat).toBeLessThan(90);
      expect(lng).toBeGreaterThanOrEqual(-180);
      expect(lng).toBeLessThan(180);
    }
  });

  test('has no duplicate tiles', () => {
    const seen = new Set();
    for (const [lat, lng] of CROWDSEC_LAND_TILES) {
      const key = lat + ',' + lng;
      expect(seen.has(key)).toBe(false);
      seen.add(key);
    }
  });
});

/* ── crowdsecQueueStrike ───────────────────────────────── */
describe('crowdsecQueueStrike', () => {
  beforeEach(() => {
    crowdsecMapStrikes = [];
  });

  test('queues a strike for a geolocated alert', () => {
    const strike = crowdsecQueueStrike({ latitude: 55.75, longitude: 37.61, has_decision: true }, 0);
    expect(strike).not.toBeNull();
    expect(crowdsecMapStrikes).toHaveLength(1);
    expect(strike.from.x).toBeGreaterThan(0);
    expect(strike.progress).toBe(0);
  });

  test('returns null when the alert has no coordinates', () => {
    expect(crowdsecQueueStrike({ scenario: 'x' }, 0)).toBeNull();
    expect(crowdsecQueueStrike({ latitude: 55.75 }, 0)).toBeNull();
    expect(crowdsecMapStrikes).toHaveLength(0);
  });

  test('banned alerts are red, unbanned are amber', () => {
    const banned = crowdsecQueueStrike({ latitude: 1, longitude: 1, has_decision: true }, 0);
    const scan = crowdsecQueueStrike({ latitude: 2, longitude: 2, has_decision: false }, 0);
    expect(banned.color).toContain('220, 38, 38');
    expect(scan.color).toContain('217, 119, 6');
  });
});

/* ── crowdsecMapApply ──────────────────────────────────── */
describe('crowdsecMapApply', () => {
  beforeEach(() => {
    crowdsecMapStrikes = [];
    crowdsecMapAlerts = [];
    crowdsecMapCanvas = null;
    crowdsecMapHome = { lat: 51.5074, lng: -0.1278 };
  });

  test('stores alerts and keeps London default home when unset', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20 }]);
    expect(crowdsecAllAlerts).toHaveLength(1);
    expect(crowdsecMapHome.lat).toBe(51.5074);
  });

  test('applies explicit map home coordinates', () => {
    crowdsecMapApply([], 40.4, -3.7);
    expect(crowdsecMapHome).toEqual({ lat: 40.4, lng: -3.7 });
  });

  test('ignores zero-zero map home (unset sentinel)', () => {
    crowdsecMapApply([], 40.4, -3.7);
    crowdsecMapApply([], 0, 0);
    expect(crowdsecMapHome.lat).toBe(51.5074);
    expect(crowdsecMapHome.lng).toBe(-0.1278);
  });

  test('skips alerts without geo data', () => {
    crowdsecMapApply([{ alert_id: 'a1' }, { alert_id: 'a2', latitude: null, longitude: null }]);
    expect(crowdsecAllAlerts).toHaveLength(2);
    expect(crowdsecMapStrikes).toHaveLength(0);
  });

  test('with no canvas mounted, apply stages alerts without firing strikes', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20 }]);
    expect(crowdsecMapStrikes).toHaveLength(0);
  });

  test('fires strikes only for alerts newer than the previous batch head', () => {
    crowdsecMapCanvas = { fake: true };
    const batch1 = [
      { alert_id: 'a1', latitude: 10, longitude: 20 },
      { alert_id: 'a2', latitude: 30, longitude: 40 }
    ];
    crowdsecMapApply(batch1);
    expect(crowdsecMapStrikes).toHaveLength(2);

    // Next refresh contains one new alert at the head; only it fires.
    crowdsecMapStrikes = [];
    const batch2 = [
      { alert_id: 'a3', latitude: -5, longitude: 15 },
      { alert_id: 'a1', latitude: 10, longitude: 20 },
      { alert_id: 'a2', latitude: 30, longitude: 40 }
    ];
    crowdsecMapApply(batch2);
    expect(crowdsecMapStrikes).toHaveLength(1);
    expect(crowdsecAllAlerts[0].alert_id).toBe('a3');
  });

  test('completely fresh batch fires strikes for all geolocated alerts', () => {
    crowdsecMapCanvas = { fake: true };
    crowdsecMapAlerts = [{ alert_id: 'old', latitude: 1, longitude: 1 }];
    const fresh = [
      { alert_id: 'n1', latitude: 10, longitude: 20 },
      { alert_id: 'n2', latitude: 30, longitude: 40 }
    ];
    crowdsecMapApply(fresh);
    expect(crowdsecMapStrikes).toHaveLength(2);
  });
});

/* ── crowdsecMapLayout ─────────────────────────────────── */
describe('persistent map markers', () => {
  let originalRAF;

  beforeEach(() => {
    originalRAF = global.requestAnimationFrame;
    crowdsecAllAlerts = [];
    crowdsecMapHover = null;
    crowdsecMapKeyboardIndex = -1;
    crowdsecMapCanvas = null;
    document.body.innerHTML = `
      <div id="tab-crowdsec" class="tab-content active"></div>
      <canvas id="crowdsecMap"></canvas>
      <div id="crowdsecMapState"></div>`;
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
  });

  afterEach(() => {
    global.requestAnimationFrame = originalRAF;
    crowdsecMapAnim = null;
  });

  test('filters unmapped alerts and caps persistent markers', () => {
    crowdsecAllAlerts = [
      { alert_id: 'missing' },
      ...Array.from({ length: 105 }, (_, i) => ({ alert_id: String(i), latitude: i % 80, longitude: i % 170 }))
    ];
    expect(crowdsecMapGeolocatedAlerts()).toHaveLength(100);
    expect(crowdsecMapGeolocatedAlerts()[0].alert_id).toBe('0');
  });

  test('draws both banned and detection-only origins after arcs finish', () => {
    const banned = { alert_id: 'b', latitude: 10, longitude: 20, has_decision: true };
    const scan = { alert_id: 's', latitude: 30, longitude: 40, has_decision: false };
    crowdsecAllAlerts = [banned, scan];
    crowdsecMapHover = banned;
    const ctx = {
      beginPath: jest.fn(), arc: jest.fn(), fill: jest.fn(), stroke: jest.fn(),
      fillStyle: '', strokeStyle: '', lineWidth: 0, shadowColor: '', shadowBlur: 0
    };

    crowdsecMapDrawMarkers(ctx, { x: 0, y: 0, w: 360, h: 180 });

    expect(ctx.fill).toHaveBeenCalledTimes(2);
    expect(ctx.stroke).toHaveBeenCalledTimes(1);
    expect(ctx.arc).toHaveBeenCalledTimes(3);
  });

  test('reports an understandable empty and populated state', () => {
    crowdsecMapUpdateState();
    const state = document.getElementById('crowdsecMapState');
    expect(state.textContent).toContain('Waiting');
    expect(state.classList.contains('is-empty')).toBe(true);

    crowdsecAllAlerts = [{ alert_id: 'a', latitude: 10, longitude: 20 }, { alert_id: 'b' }];
    crowdsecMapUpdateState();
    expect(state.textContent).toContain('1 mapped origin');
    expect(state.textContent).toContain('2 recent detections');
    expect(state.classList.contains('is-empty')).toBe(false);
  });

  test('arrow keys cycle through geolocated origins for keyboard users', () => {
    const canvas = document.getElementById('crowdsecMap');
    crowdsecMapCanvas = canvas;
    crowdsecAllAlerts = [
      { alert_id: 'a', latitude: 10, longitude: 20, country: 'GB', scenario: 'crowdsecurity/ssh-bf' },
      { alert_id: 'b', latitude: 30, longitude: 40, country: 'DE', scenario: 'crowdsecurity/http-probing' }
    ];
    const firstEvent = { key: 'ArrowRight', preventDefault: jest.fn() };
    crowdsecMapOnKeyDown(firstEvent);
    expect(crowdsecMapHover.alert_id).toBe('a');
    expect(canvas.getAttribute('aria-label')).toContain('GB');
    expect(firstEvent.preventDefault).toHaveBeenCalled();

    crowdsecMapOnKeyDown({ key: 'ArrowRight', preventDefault: jest.fn() });
    expect(crowdsecMapHover.alert_id).toBe('b');

    crowdsecMapOnKeyDown({ key: 'Escape', preventDefault: jest.fn() });
    expect(crowdsecMapHover).toBeNull();
  });

  test('map animation resumes only while the CrowdSec tab is visible', () => {
    global.requestAnimationFrame = jest.fn(() => 17);
    crowdsecMapAnim = null;

    document.getElementById('tab-crowdsec').classList.remove('active');
    crowdsecMapResume();
    expect(global.requestAnimationFrame).not.toHaveBeenCalled();

    document.getElementById('tab-crowdsec').classList.add('active');
    crowdsecMapResume();
    expect(global.requestAnimationFrame).toHaveBeenCalledTimes(1);
    expect(crowdsecMapAnim).toBe(17);

  });
});

describe('crowdsecMapLayout', () => {
  test('keeps the 2:1 projection aspect in a wide box', () => {
    const l = crowdsecMapLayout(800, 400); // nearly 2:1 container
    expect(l.w).toBeCloseTo(760);
    expect(l.h).toBeCloseTo(l.w / 2);
  });

  test('letterboxes vertically when the box is too short', () => {
    const l = crowdsecMapLayout(400, 500); // narrow-tall container: 2:1 fits on width
    expect(l.w).toBeCloseTo(380);
    expect(l.h).toBeCloseTo(190);
    expect(l.y).toBeCloseTo((500 - 190) / 2); // centered
  });

  test('letterboxes vertically when the box is too wide (max-height clamp)', () => {
    const l = crowdsecMapLayout(1000, 300); // ultra-wide container
    expect(l.h).toBeCloseTo(280);
    expect(l.w).toBeCloseTo(560);
  });

  test('centers the map area inside the canvas', () => {
    const l = crowdsecMapLayout(400, 500);
    expect(l.x).toBeCloseTo((400 - l.w) / 2);
    expect(l.y).toBeCloseTo((500 - l.h) / 2);
  });

  test('dot size scales with map width and stays readable', () => {
    const tiny = crowdsecMapLayout(300, 150);
    const wide = crowdsecMapLayout(1000, 300);
    expect(tiny.dot).toBeGreaterThanOrEqual(1.3);
    expect(wide.dot).toBeLessThanOrEqual(3.2);
    expect(wide.dot).toBeGreaterThanOrEqual(tiny.dot);
  });

  test('degenerate tiny canvas still yields a usable map', () => {
    const l = crowdsecMapLayout(100, 40);
    expect(l.w).toBeGreaterThan(0);
    expect(l.h).toBeGreaterThan(0);
    expect(l.w).toBeCloseTo(l.h * 2);
  });
});

/* ── quad (bezier helper) ───────────────────────────────── */
describe('quad', () => {
  test('interpolates endpoints correctly', () => {
    expect(quad(0, 10, 20, 0)).toBe(0);
    expect(quad(0, 10, 20, 1)).toBe(20);
  });

  test('midpoint passes through the curve', () => {
    const mid = quad(0, 10, 20, 0.5);
    expect(mid).toBeCloseTo(10);
  });

  test('monotonic progression between endpoints', () => {
    let prev = quad(0, 40, 100, 0);
    for (let t = 0.1; t <= 1; t += 0.1) {
      const v = quad(0, 40, 100, t);
      expect(v).toBeGreaterThan(prev);
      prev = v;
    }
  });
});
