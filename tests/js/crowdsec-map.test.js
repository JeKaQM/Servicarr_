/**
 * Tests for the clustered, finite-render CrowdSec geography view.
 */
const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'crowdsec-charts.js', 'crowdsec-tab.js', 'crowdsec-map.js');
});

describe('crowdsecProject', () => {
  test('converts lat/lng to normalized map space', () => {
    expect(crowdsecProject(0, 0)).toEqual({ x: 0.5, y: 0.5 });
    expect(crowdsecProject(90, -180)).toEqual({ x: 0, y: 0 });
    expect(crowdsecProject(-90, 180)).toEqual({ x: 1, y: 1 });
  });

  test('clamps invalid geographic bounds', () => {
    expect(crowdsecProject(120, -220)).toEqual({ x: 0, y: 0 });
    expect(crowdsecProject(-120, 220)).toEqual({ x: 1, y: 1 });
  });
});

describe('recognisable local basemap asset', () => {
  test('uses a Natural Earth SVG with real country paths', () => {
    const asset = fs.readFileSync(path.resolve(__dirname, '../../web/static/images/crowdsec-world-map.svg'), 'utf8');
    expect(asset).toContain('Natural Earth');
    expect(asset).toContain('viewBox="0 0 1200 600"');
    expect((asset.match(/<path /g) || []).length).toBeGreaterThan(100);
  });
});

describe('crowdsecMapAggregateAlerts', () => {
  test('clusters nearby points and retains complete counts', () => {
    const clusters = crowdsecMapAggregateAlerts([
      { alert_id: 'a', latitude: 51.501, longitude: -0.12, country: 'gb', scenario: 'ssh', source_value: '1.1.1.1', events_count: 5, has_decision: true },
      { alert_id: 'b', latitude: 51.504, longitude: -0.124, country: 'gb', scenario: 'ssh', source_value: '2.2.2.2', events_count: 3, has_decision: false },
      { alert_id: 'c', latitude: 40.7, longitude: -74, country: 'us', scenario: 'http', source_value: '3.3.3.3', events_count: 2, has_decision: false }
    ]);

    expect(clusters).toHaveLength(2);
    expect(clusters[0].count).toBe(2);
    expect(clusters[0].reportedEvents).toBe(8);
    expect(clusters[0].actioned).toBe(1);
    expect(clusters[0].observed).toBe(1);
    expect(clusters[0].sources.size).toBe(2);
    expect(clusters[0].country).toBe('GB');
  });

  test('ignores absent and out-of-range coordinates', () => {
    expect(crowdsecMapAggregateAlerts([
      { alert_id: 'missing' },
      { alert_id: 'bad', latitude: 95, longitude: 10 },
      { alert_id: 'valid', latitude: 0, longitude: 0 }
    ])).toHaveLength(1);
  });

  test('bubble radius grows with volume and remains bounded', () => {
    expect(crowdsecBubbleRadius(1, 100)).toBeGreaterThanOrEqual(4.5);
    expect(crowdsecBubbleRadius(100, 100)).toBeGreaterThan(crowdsecBubbleRadius(4, 100));
    expect(crowdsecBubbleRadius(100, 100)).toBeLessThanOrEqual(14);
  });
});

describe('map filters', () => {
  beforeEach(() => {
    crowdsecMapFilter = 'all';
    crowdsecAllAlerts = [
      { alert_id: 'actioned', latitude: 10, longitude: 20, has_decision: true },
      { alert_id: 'observed', latitude: 30, longitude: 40, has_decision: false },
      { alert_id: 'missing' }
    ];
    document.body.innerHTML = `
      <button data-crowdsec-map-filter="all" aria-pressed="true"></button>
      <button data-crowdsec-map-filter="actioned" aria-pressed="false"></button>
      <button data-crowdsec-map-filter="observed" aria-pressed="false"></button>
      <div id="crowdsecMapState"></div><div id="crowdsecMapInspector"></div>
      <button class="crowdsec-alert-row" data-alert-id="actioned" aria-pressed="false"></button>`;
  });

  test('selects decision-attached and detection-only subsets', () => {
    expect(crowdsecMapGeolocatedAlerts()).toHaveLength(2);
    crowdsecMapSetFilter('actioned');
    expect(crowdsecMapGeolocatedAlerts().map(item => item.alert_id)).toEqual(['actioned']);
    expect(document.querySelector('[data-crowdsec-map-filter="actioned"]').getAttribute('aria-pressed')).toBe('true');
    crowdsecMapSetFilter('observed');
    expect(crowdsecMapGeolocatedAlerts().map(item => item.alert_id)).toEqual(['observed']);
  });

  test('does not cap the map below the complete retained alert mirror', () => {
    crowdsecMapFilter = 'all';
    crowdsecAllAlerts = Array.from({ length: 150 }, (_, index) => ({
      alert_id: String(index), latitude: index % 80, longitude: index % 170
    }));
    expect(crowdsecMapGeolocatedAlerts()).toHaveLength(150);
  });

  test('clears an incompatible pinned selection when the filter changes', () => {
    crowdsecMapSelectCluster(crowdsecMapClusters().find(cluster => cluster.alertIDs.includes('actioned')));
    expect(document.querySelector('.crowdsec-alert-row').getAttribute('aria-pressed')).toBe('true');
    crowdsecMapSetFilter('observed');
    expect(crowdsecMapSelectedKey).toBe('');
    expect(document.querySelector('.crowdsec-alert-row').getAttribute('aria-pressed')).toBe('false');
  });

  test('feed click selects every row represented by the same map cluster', () => {
    crowdsecAllAlerts = [
      { alert_id: 'first', latitude: 10.01, longitude: 20.01 },
      { alert_id: 'second', latitude: 10.02, longitude: 20.02 }
    ];
    document.body.insertAdjacentHTML('beforeend',
      '<button class="crowdsec-alert-row" data-alert-id="first" aria-pressed="false"></button>' +
      '<button class="crowdsec-alert-row" data-alert-id="second" aria-pressed="false"></button>');

    selectCrowdsecAlertRow(document.querySelector('[data-alert-id="first"]'));

    expect(crowdsecMapSelectedKey).toBe('10.0,20.0');
    expect(document.querySelector('[data-alert-id="first"]').getAttribute('aria-pressed')).toBe('true');
    expect(document.querySelector('[data-alert-id="second"]').getAttribute('aria-pressed')).toBe('true');
  });
});

describe('crowdsecQueueStrike', () => {
  beforeEach(() => {
    crowdsecMapStrikes = [];
    crowdsecMapHome = { lat: 51.5, lng: -0.1 };
  });

  test('queues a finite strike only when a destination is configured', () => {
    const strike = crowdsecQueueStrike({ latitude: 55.75, longitude: 37.61, has_decision: true }, 0);
    expect(strike).not.toBeNull();
    expect(strike.from.x).toBeGreaterThan(0);
    expect(strike.duration).toBe(900);
    crowdsecMapHome = null;
    expect(crowdsecQueueStrike({ latitude: 1, longitude: 1 }, 0)).toBeNull();
  });

  test('uses distinct actioned and detection-only colors', () => {
    const actioned = crowdsecQueueStrike({ latitude: 1, longitude: 1, has_decision: true }, 0);
    const observed = crowdsecQueueStrike({ latitude: 2, longitude: 2, has_decision: false }, 0);
    expect(actioned.color).toBe('251, 113, 133');
    expect(observed.color).toBe('251, 191, 36');
  });

  test('honours reduced motion', () => {
    const original = window.matchMedia;
    window.matchMedia = jest.fn(() => ({ matches: true }));
    expect(crowdsecQueueStrike({ latitude: 1, longitude: 1 }, 0)).toBeNull();
    window.matchMedia = original;
  });
});

describe('crowdsecMapApply', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="crowdsecMapState"></div><div id="crowdsecMapInspector"></div>';
    crowdsecMapStrikes = [];
    crowdsecMapAlerts = [];
    crowdsecMapCanvas = null;
    crowdsecMapHome = null;
    crowdsecMapSelectedKey = '';
    crowdsecMapFilter = 'all';
    crowdsecMapLastStateText = '';
  });

  test('keeps destination unset instead of inventing a location', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20 }], 0, 0);
    expect(crowdsecMapHome).toBeNull();
    expect(document.getElementById('crowdsecMapState').textContent).toContain('destination not set');
  });

  test('applies explicit map destination coordinates, including zero latitude', () => {
    crowdsecMapApply([], 0, -3.7);
    expect(crowdsecMapHome).toEqual({ lat: 0, lng: -3.7 });
  });

  test('preserves a selected cluster across object replacement', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10.01, longitude: 20.01, country: 'GB' }]);
    crowdsecMapSelectCluster(crowdsecMapClusters()[0]);
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10.02, longitude: 20.02, country: 'GB' }]);
    expect(crowdsecMapSelectedKey).toBe('10.0,20.0');
    expect(document.getElementById('crowdsecMapInspector').dataset.clusterKey).toBe('10.0,20.0');
  });

  test('with no canvas mounted, apply stages alerts without firing arcs', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20 }], 40, -3);
    expect(crowdsecAllAlerts).toHaveLength(1);
    expect(crowdsecMapStrikes).toHaveLength(0);
  });

  test('reconciles a hover preview to fresh cluster data after polling', () => {
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20, country: 'GB', scenario: 'old' }]);
    crowdsecMapHover = crowdsecMapClusters()[0];
    const previousHover = crowdsecMapHover;
    crowdsecMapApply([{ alert_id: 'a1', latitude: 10, longitude: 20, country: 'GB', scenario: 'new' }]);
    expect(crowdsecMapHover).not.toBe(previousHover);
    expect(crowdsecMapHover.scenario).toBe('new');
    expect(document.getElementById('crowdsecMapInspector').textContent).toContain('new');
  });
});

describe('state, selection, and finite rendering', () => {
  let originalRAF;

  beforeEach(() => {
    originalRAF = global.requestAnimationFrame;
    global.requestAnimationFrame = jest.fn(() => 17);
    crowdsecAllAlerts = [];
    crowdsecMapFilter = 'all';
    crowdsecMapHover = null;
    crowdsecMapSelectedKey = '';
    crowdsecMapKeyboardIndex = -1;
    crowdsecMapCanvas = null;
    crowdsecMapAnim = null;
    crowdsecMapHome = null;
    crowdsecMapLastStateText = '';
    document.body.innerHTML = `
      <div id="tab-crowdsec" class="tab-content active"></div>
      <canvas id="crowdsecMap" aria-label="Observed source map"></canvas>
      <div id="crowdsecMapState"></div>
      <div id="crowdsecMapInspector"></div>
      <div id="crowdsecMapTip" class="hidden"></div>`;
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
  });

  afterEach(() => {
    global.requestAnimationFrame = originalRAF;
    crowdsecMapAnim = null;
  });

  test('reports mapped detections as aggregated locations', () => {
    crowdsecAllAlerts = [
      { alert_id: 'a', latitude: 10.01, longitude: 20.01 },
      { alert_id: 'b', latitude: 10.02, longitude: 20.02 },
      { alert_id: 'c' }
    ];
    crowdsecMapUpdateState();
    expect(document.getElementById('crowdsecMapState').textContent).toContain('2 mapped detections');
    expect(document.getElementById('crowdsecMapState').textContent).toContain('1 clustered location');
  });

  test('keyboard traversal previews clusters and Enter pins one', () => {
    crowdsecMapCanvas = document.getElementById('crowdsecMap');
    crowdsecAllAlerts = [
      { alert_id: 'a', latitude: 10, longitude: 20, country: 'GB', scenario: 'crowdsecurity/ssh-bf' },
      { alert_id: 'b', latitude: 30, longitude: 40, country: 'DE', scenario: 'crowdsecurity/http-probing' }
    ];
    const next = { key: 'ArrowRight', preventDefault: jest.fn() };
    crowdsecMapOnKeyDown(next);
    expect(crowdsecMapHover.country).toBe('GB');
    expect(next.preventDefault).toHaveBeenCalled();
    crowdsecMapOnKeyDown({ key: 'Enter', preventDefault: jest.fn() });
    expect(crowdsecMapSelectedKey).toBe(crowdsecMapHover.key);
    expect(document.getElementById('crowdsecMapInspector').textContent).toContain('United Kingdom');
    crowdsecMapOnKeyDown({ key: 'Escape', preventDefault: jest.fn() });
    expect(crowdsecMapSelectedKey).toBe('');
  });

  test('hit-tests click coordinates so a touch tap works without mousemove', () => {
    crowdsecMapCanvas = document.getElementById('crowdsecMap');
    crowdsecMapCanvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: 400, height: 200 });
    crowdsecAllAlerts = [{ alert_id: 'tap', latitude: 0, longitude: 0, country: 'GB' }];
    crowdsecMapHover = null;
    crowdsecMapOnClick({ clientX: 200, clientY: 100 });
    expect(crowdsecMapSelectedKey).toBe('0.0,0.0');
  });

  test('does not preview or select an alert excluded by the active filter', () => {
    crowdsecAllAlerts = [{ alert_id: 'observed', latitude: 10, longitude: 20, has_decision: false }];
    crowdsecMapFilter = 'actioned';
    expect(crowdsecMapPreviewAlert(crowdsecAllAlerts[0])).toBe(false);
    expect(crowdsecMapSelectAlert(crowdsecAllAlerts[0])).toBe(false);
    expect(crowdsecMapSelectedKey).toBe('');
  });

  test('describes both outcomes in a mixed cluster', () => {
    const cluster = crowdsecMapAggregateAlerts([
      { alert_id: 'a', latitude: 10, longitude: 20, has_decision: true },
      { alert_id: 'b', latitude: 10, longitude: 20, has_decision: false }
    ])[0];
    expect(crowdsecMapOutcomeLabel(cluster)).toBe('1 decision attached · 1 detection only');
  });

  test('schedules a draw only while the tab is visible', () => {
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
  test('keeps the 2:1 projection centred in different boxes', () => {
    const standard = crowdsecMapLayout(800, 400);
    expect(standard.w).toBeCloseTo(760);
    expect(standard.h).toBeCloseTo(380);
    const tall = crowdsecMapLayout(400, 500);
    expect(tall.w).toBeCloseTo(380);
    expect(tall.h).toBeCloseTo(190);
    expect(tall.y).toBeCloseTo(155);
  });

  test('handles tiny canvases without negative dimensions', () => {
    const layout = crowdsecMapLayout(10, 10);
    expect(layout.w).toBeGreaterThan(0);
    expect(layout.h).toBeGreaterThan(0);
    expect(layout.w).toBeCloseTo(layout.h * 2);
  });
});

describe('quad', () => {
  test('interpolates quadratic endpoints and midpoint', () => {
    expect(quad(0, 10, 20, 0)).toBe(0);
    expect(quad(0, 10, 20, 0.5)).toBeCloseTo(10);
    expect(quad(0, 10, 20, 1)).toBe(20);
  });
});
