/**
 * Tests for crowdsec-tab.js – badge text, dashboard rendering, collapsible
 * lists, activity feed, stats cards, and secret fields.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'crowdsec-charts.js', 'crowdsec-tab.js');
});

/* ── crowdsecSyncBadgeText ─────────────────────────────── */
describe('crowdsecSyncBadgeText', () => {
  test('null status means not synced yet', () => {
    expect(crowdsecSyncBadgeText(null)).toBe('Not synced yet');
  });

  test('empty status means not synced yet', () => {
    expect(crowdsecSyncBadgeText({})).toBe('Not synced yet');
  });

  test('disabled integration distinguishes retained data from a new installation', () => {
    expect(crowdsecSyncBadgeText({ enabled: false })).toBe('Integration disabled');
    expect(crowdsecSyncBadgeText({ enabled: false, last_sync: new Date().toISOString(), last_error: 'stale failure' }))
      .toBe('Integration disabled · cached data');
  });

  test('auth failure shows credential hint', () => {
    const text = crowdsecSyncBadgeText({ last_error: 'http 401: bad key', auth_failed: true });
    expect(text).toBe('Authentication failed');
  });

  test('sync error surfaces sanitized text', () => {
    const text = crowdsecSyncBadgeText({ last_error: 'LAPI unreachable', auth_failed: false });
    expect(text).toBe('LAPI unreachable · cached data');
  });

  test('recent sync shows minutes ago', () => {
    const twoMinAgo = new Date(Date.now() - 2 * 60 * 1000).toISOString();
    const text = crowdsecSyncBadgeText({ last_sync: twoMinAgo });
    expect(text).toContain('Synced 2 min ago');
  });

  test('very recent sync shows just now', () => {
    const now = new Date().toISOString();
    expect(crowdsecSyncBadgeText({ last_sync: now })).toContain('Synced just now');
  });

  test('unparseable sync time falls back', () => {
    expect(crowdsecSyncBadgeText({ last_sync: 'garbage' })).toBe('Not synced yet');
  });

  test('does not imply that the fetched decision count is a true LAPI total', () => {
    const minAgo = new Date(Date.now() - 60 * 1000).toISOString();
    const text = crowdsecSyncBadgeText({
      last_sync: minAgo,
      decision_count: 15000,
      snapshot_count: 500
    });
    expect(text).toBe('Synced 1 min ago');
    expect(text).not.toContain('of 15000');
  });
});

/* ── renderCrowdsecDecisions ───────────────────────────── */
describe('renderCrowdsecDecisions', () => {
  beforeEach(() => {
    crowdsecDecisionsExpanded = false;
    document.body.innerHTML = `
      <div id="crowdsecDecisions"></div>
      <span id="crowdsecDecisionsSummary"></span>
      <div class="crowdsec-expand hidden" id="crowdsecDecisionsExpand">
        <button type="button" data-crowdsec-toggle="decisions"></button>
      </div>`;
  });

  test('empty decisions render placeholder', () => {
    renderCrowdsecDecisions([]);
    expect(document.getElementById('crowdsecDecisions').textContent).toContain('No decisions synced yet');
  });

  test('null decisions render placeholder', () => {
    renderCrowdsecDecisions(null);
    expect(document.getElementById('crowdsecDecisions').textContent).toContain('No decisions synced yet');
  });

  test('decisions render values and types', () => {
    renderCrowdsecDecisions([{
      decision_id: '7', value: '1.2.3.4', type: 'ban', scope: 'Ip',
      origin: 'crowdsec', scenario: 'crowdsecurity/ssh-bf', duration: '3h59m55s',
      created_at: '2026-09-18T10:00:00Z'
    }]);
    const html = document.getElementById('crowdsecDecisions').innerHTML;
    expect(html).toContain('1.2.3.4');
    expect(html).toContain('crowdsec-type-ban');
    expect(html).toContain('crowdsecurity/ssh-bf');
    expect(html).toContain('3h59m55s');
    expect(html).toContain('2026-09-18 10:00:00');
  });

  test('non-ban types use the other style', () => {
    renderCrowdsecDecisions([{ decision_id: '8', value: '9.9.9.9', type: 'captcha', scope: 'Ip' }]);
    const html = document.getElementById('crowdsecDecisions').innerHTML;
    expect(html).toContain('crowdsec-type-other');
  });

  test('decision values are HTML-escaped in text AND attributes', () => {
    renderCrowdsecDecisions([{
      decision_id: '9', value: '<script>alert(1)</script>', type: 'ban', scope: 'Ip'
    }]);
    // Text node is entity-escaped in the serialized HTML.
    const html = document.getElementById('crowdsecDecisions').innerHTML;
    expect(html).toContain('&lt;script&gt;');
    // DOM-level: no script element was actually created (inert attribute value).
    const scriptEl = document.getElementById('crowdsecDecisions').querySelector('script');
    expect(scriptEl).toBeNull();
    // The value stays a plain attribute string.
    const valueEl = document.querySelector('.crowdsec-decision-value');
    expect(valueEl.getAttribute('title')).toBe('Ip: <script>alert(1)</script>');
  });

  test('quote characters cannot break out of attributes', () => {
    renderCrowdsecDecisions([{
      decision_id: 'q', value: '1.2.3.4"><img src=x onerror=alert(1)>', type: 'ban', scope: 'Ip'
    }]);
    // DOM-level: the injected <img> must NOT materialize as an element.
    const injectedImg = document.getElementById('crowdsecDecisions').querySelector('img');
    expect(injectedImg).toBeNull();
    // The row element still parses with its original attribute intact.
    const row = document.querySelector('.crowdsec-decision-row[data-decision-id="q"]');
    expect(row).not.toBeNull();
    const title = row.querySelector('.crowdsec-decision-value').getAttribute('title');
    expect(title).toBe('Ip: 1.2.3.4"><img src=x onerror=alert(1)>');
  });

  test('collapsed by default: only the first 5 rows render', () => {
    const many = Array.from({ length: 12 }, (_, i) => ({
      decision_id: String(i), value: `10.0.0.${i}`, type: 'ban', scope: 'Ip'
    }));
    renderCrowdsecDecisions(many);
    const rows = document.querySelectorAll('.crowdsec-decision-row:not(.crowdsec-decision-header)');
    expect(rows.length).toBe(5);
    // Expand bar is visible with the full count.
    const expand = document.getElementById('crowdsecDecisionsExpand');
    expect(expand.classList.contains('hidden')).toBe(false);
    expect(expand.querySelector('button').textContent).toContain('Show all 12');
  });

  test('expand toggle renders all rows and flips the label', () => {
    const many = Array.from({ length: 8 }, (_, i) => ({
      decision_id: String(i), value: `10.0.0.${i}`, type: 'ban', scope: 'Ip'
    }));
    renderCrowdsecDecisions(many);
    crowdsecDecisionsExpanded = true;
    renderCrowdsecDecisions(many);
    const rows = document.querySelectorAll('.crowdsec-decision-row:not(.crowdsec-decision-header)');
    expect(rows.length).toBe(8);
    expect(document.getElementById('crowdsecDecisionsExpand').querySelector('button').textContent).toContain('Show fewer');
  });

  test('fewer than collapse threshold hides the expand bar', () => {
    renderCrowdsecDecisions([{ decision_id: '1', value: '1.1.1.1', type: 'ban', scope: 'Ip' }]);
    expect(document.getElementById('crowdsecDecisionsExpand').classList.contains('hidden')).toBe(true);
  });

  test('header row renders six columns', () => {
    renderCrowdsecDecisions([]);
    renderCrowdsecDecisions([{ decision_id: '10', value: '1.1.1.1', type: 'ban', scope: 'Ip' }]);
    const header = document.querySelector('.crowdsec-decision-header');
    expect(header).not.toBeNull();
    expect(header.textContent).toContain('First seen');
    expect(header.textContent).not.toContain('Created');
    expect(document.getElementById('crowdsecDecisions').getAttribute('role')).toBe('table');
    expect(header.getAttribute('role')).toBe('row');
    expect(header.querySelectorAll('[role="columnheader"]')).toHaveLength(6);
    expect(document.querySelectorAll('.crowdsec-decision-row:not(.crowdsec-decision-header) [role="cell"]')).toHaveLength(6);
  });
});

/* ── renderCrowdsecAlerts (live activity feed) ─────────── */
describe('renderCrowdsecAlerts', () => {
  beforeEach(() => {
    crowdsecAlertsExpanded = false;
    document.body.innerHTML = `
      <div id="crowdsecAlerts"></div>
      <div class="crowdsec-expand hidden" id="crowdsecAlertsExpand">
        <button type="button" data-crowdsec-toggle="alerts"></button>
      </div>`;
  });

  test('empty alerts render the credentials hint', () => {
    renderCrowdsecAlerts([]);
    const text = document.getElementById('crowdsecAlerts').textContent;
    expect(text).toContain('No detections synced yet');
    expect(text).toContain('cscli machines add servicarr');
  });

  test('alert row shows scenario, IP, country, and outcome', () => {
    renderCrowdsecAlerts([{
      alert_id: '42', scenario: 'crowdsecurity/ssh-bf', source_value: '5.5.5.5',
      country: 'cn', events_count: 7, has_decision: true, created_at: new Date().toISOString(),
      latitude: 39.9, longitude: 116.4
    }]);
    const html = document.getElementById('crowdsecAlerts').innerHTML;
    expect(html).toContain('ssh-bf');
    expect(html).toContain('5.5.5.5');
    expect(html).toContain('CN'); // country code uppercased
    expect(html).toContain('decision attached');
    expect(html).toContain('7×');
    // The display scenario strips the crowdsecurity/ prefix…
    const scenarioEl = document.querySelector('.crowdsec-alert-scenario');
    expect(scenarioEl.textContent).toBe('ssh-bf');
    // …but the tooltip keeps the full name for operator clarity.
    const row = document.querySelector('.crowdsec-alert-row');
    expect(row.getAttribute('title')).toContain('crowdsecurity/ssh-bf');
    expect(row.tabIndex).toBe(0);
    expect(row.getAttribute('aria-label')).toContain('decision attached');
    expect(row.getAttribute('aria-label')).toContain('7 raw events');
    expect(row.getAttribute('aria-pressed')).toBe('false');
    expect(row.querySelector('.crowdsec-alert-meta .crowdsec-alert-count')).not.toBeNull();
  });

  test('groups raw-event and simulated badges in one responsive metadata cell', () => {
    renderCrowdsecAlerts([{
      alert_id: 'meta', scenario: 'crowdsecurity/test', source_value: '5.5.5.5',
      country: 'gb', events_count: 3, simulated: true, created_at: new Date().toISOString()
    }]);
    const meta = document.querySelector('.crowdsec-alert-meta');
    expect(meta.querySelector('.crowdsec-alert-count')).not.toBeNull();
    expect(meta.querySelector('.crowdsec-alert-simulated')).not.toBeNull();
    expect(document.querySelector('.crowdsec-alert-row').getAttribute('aria-label')).toContain('simulated detection');
  });

  test('keeps the active row DOM stable when a live poll returns unchanged data', () => {
    const alerts = [{
      alert_id: 'stable', scenario: 'crowdsecurity/test', source_value: '5.5.5.5',
      country: 'gb', events_count: 1, created_at: new Date().toISOString(), latitude: 1, longitude: 2
    }];
    renderCrowdsecAlerts(alerts);
    const row = document.querySelector('.crowdsec-alert-row');
    row.classList.add('is-selected');
    row.setAttribute('aria-pressed', 'true');
    renderCrowdsecAlerts(alerts);
    expect(document.querySelector('.crowdsec-alert-row')).toBe(row);
    expect(row.classList.contains('is-selected')).toBe(true);
    expect(row.getAttribute('aria-pressed')).toBe('true');
  });

  test('restores keyboard focus when relative time changes during a poll', () => {
    const createdAt = new Date(Date.now() - 90_000).toISOString();
    const alerts = [{ alert_id: 'focused', scenario: 'test', source_value: '1.2.3.4',
      created_at: createdAt, latitude: 1, longitude: 2 }];
    const originalNow = Date.now;
    const firstNow = originalNow();
    try {
      jest.spyOn(Date, 'now').mockReturnValue(firstNow);
      renderCrowdsecAlerts(alerts);
      const before = document.querySelector('.crowdsec-alert-row');
      before.focus();
      expect(document.activeElement).toBe(before);

      Date.now.mockReturnValue(firstNow + 60_000);
      renderCrowdsecAlerts(alerts);
      const after = document.querySelector('.crowdsec-alert-row');
      expect(after).not.toBe(before);
      expect(document.activeElement).toBe(after);
      expect(after.textContent).toContain('2m ago');
    } finally {
      Date.now.mockRestore();
    }
  });

  test('alerts without coordinates are not exposed as map controls', () => {
    renderCrowdsecAlerts([{
      alert_id: 'no-geo', scenario: 'crowdsecurity/http-probing', source_value: '6.6.6.6',
      country: 'ru', has_decision: false, created_at: new Date().toISOString()
    }]);
    const row = document.querySelector('.crowdsec-alert-row');
    expect(row.classList.contains('is-mappable')).toBe(false);
    expect(row.hasAttribute('tabindex')).toBe(false);
    expect(row.getAttribute('role')).toBe('group');
    expect(row.hasAttribute('aria-controls')).toBe(false);
    expect(row.getAttribute('aria-label')).toContain('0 raw events');
  });

  test('alert without a decision is labelled detection-only', () => {
    renderCrowdsecAlerts([{
      alert_id: '43', scenario: 'crowdsecurity/http-probing', source_value: '6.6.6.6',
      country: 'ru', has_decision: false, created_at: new Date().toISOString()
    }]);
    expect(document.getElementById('crowdsecAlerts').innerHTML).toContain('detection only');
  });

  test('feed collapses to 5 rows with expand bar', () => {
    const many = Array.from({ length: 9 }, (_, i) => ({
      alert_id: String(i), scenario: 'crowdsecurity/ssh-bf', source_value: `1.2.3.${i}`,
      country: 'us', created_at: new Date().toISOString()
    }));
    renderCrowdsecAlerts(many);
    expect(document.querySelectorAll('.crowdsec-alert-row').length).toBe(5);
    const expand = document.getElementById('crowdsecAlertsExpand');
    expect(expand.classList.contains('hidden')).toBe(false);
  });

  test('alert fields are HTML-escaped', () => {
    renderCrowdsecAlerts([{
      alert_id: 'x', scenario: '<img src=x onerror=alert(2)>', source_value: '7.7.7.7',
      country: 'de', created_at: new Date().toISOString()
    }]);
    const container = document.getElementById('crowdsecAlerts');
    expect(container.querySelector('img')).toBeNull();
  });
});

/* ── shortScenario / relativeTime ───────────────────────── */
describe('shortScenario', () => {
  test('strips the crowdsecurity namespace', () => {
    expect(shortScenario('crowdsecurity/ssh-bf')).toBe('ssh-bf');
  });
  test('leaves non-namespaced names alone', () => {
    expect(shortScenario('custom-scenario')).toBe('custom-scenario');
  });
  test('empty falls back to empty', () => {
    expect(shortScenario('')).toBe('');
  });
});

describe('relativeTime', () => {
  test('just now under a minute', () => {
    expect(relativeTime(new Date().toISOString())).toBe('just now');
  });
  test('minutes ago', () => {
    expect(relativeTime(new Date(Date.now() - 5 * 60 * 1000).toISOString())).toBe('5m ago');
  });
  test('hours ago', () => {
    expect(relativeTime(new Date(Date.now() - 3 * 3600 * 1000).toISOString())).toBe('3h ago');
  });
  test('days ago', () => {
    expect(relativeTime(new Date(Date.now() - 26 * 3600 * 1000).toISOString())).toBe('1d ago');
  });
  test('invalid input gives empty string', () => {
    expect(relativeTime('garbage')).toBe('');
    expect(relativeTime('')).toBe('');
  });
});

describe('crowdsecRemainingTime', () => {
  const now = Date.parse('2026-09-21T12:00:00Z');

  test('derives a live countdown from the stable expiry', () => {
    expect(crowdsecRemainingTime('2026-09-21T15:05:30Z', 'stale', now)).toBe('3h 5m');
    expect(crowdsecRemainingTime('2026-09-21T12:02:10Z', 'stale', now)).toBe('2m 10s');
  });

  test('marks elapsed decisions expired and falls back for invalid expiry', () => {
    expect(crowdsecRemainingTime('2026-09-21T11:59:59Z', 'stale', now)).toBe('expired');
    expect(crowdsecRemainingTime('', '1h')).toBe('1h');
  });
});

/* ── renderCrowdsecStats (dashboard cards + breakdowns) ── */
describe('renderCrowdsecStats', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="csStatActive"></div>
      <div id="csStatAlerts"></div>
      <div id="csStatEvents"></div>
      <div id="csStatSources"></div>
      <div id="csStatActionRate"></div>
      <div id="csStatActionRateLabel"></div>
      <div id="csStatGeoRate"></div>
      <div id="csStatGeoRateLabel"></div>
      <div id="csStatSimulated"></div>
      <div id="csStatCountries"></div>
      <div id="csStatTopCountry"></div>
      <div id="csStatTopCountryLabel"></div>
      <div id="csStatTopScenario"></div>
      <div id="crowdsecCountries"></div>
      <div id="crowdsecScenarios"></div>
      <div id="crowdsecNetworks"></div>
      <div id="crowdsecSources"></div>
      <div id="crowdsecDecisionTypes"></div>
      <div id="crowdsecDecisionOrigins"></div>
      <div id="crowdsecTimeline"></div>
      <div id="crowdsecOutcomeChart"></div>`;
  });

  test('stat cards fill from aggregates', () => {
    renderCrowdsecStats({
      active_decisions: 12, alerts_24h: 47, alerts_with_decision_24h: 12,
      decision_action_rate_percent: 25.5, reported_events_24h: 301,
      unique_sources_24h: 39, simulated_alerts_24h: 2, geolocated_alerts_24h: 40,
      top_country: 'cn', top_country_count: 30,
      top_scenario: 'crowdsecurity/ssh-bf',
      countries: [{ country: 'cn', count: 30 }, { country: 'ru', count: 17 }],
      scenarios: [{ scenario: 'crowdsecurity/ssh-bf', count: 40 }],
      networks: [{ as_number: 'AS64500', as_name: 'Example Net', count: 8 }],
      sources: [{ source: '1.2.3.4', count: 5 }],
      active_decision_types: [{ type: 'ban', count: 10 }],
      active_decision_origins: [{ origin: 'crowdsec', count: 12 }],
      hourly: [{ start: '2026-09-21T10:00:00Z', detections: 47, with_decision: 12, reported_events: 301 }]
    });
    expect(document.getElementById('csStatActive').textContent).toBe('12');
    expect(document.getElementById('csStatAlerts').textContent).toBe('47');
    expect(document.getElementById('csStatEvents').textContent).toBe('301');
    expect(document.getElementById('csStatSources').textContent).toBe('39');
    expect(document.getElementById('csStatActionRate').textContent).toBe('25.5%');
    expect(document.getElementById('csStatActionRateLabel').textContent).toContain('12 of 47');
    expect(document.getElementById('csStatGeoRate').textContent).toBe('85.1%');
    expect(document.getElementById('csStatSimulated').textContent).toBe('2');
    expect(document.getElementById('csStatTopCountry').textContent).toBe('CN');
    expect(document.getElementById('csStatTopCountryLabel').textContent).toContain('30 detections');
    expect(document.getElementById('csStatTopScenario').textContent).toBe('ssh-bf');
    expect(document.getElementById('crowdsecNetworks').textContent).toContain('Example Net');
    expect(document.getElementById('crowdsecSources').textContent).toContain('1.2.3.4');
    expect(document.getElementById('crowdsecDecisionTypes').textContent).toContain('ban');
    expect(document.getElementById('crowdsecTimeline').querySelector('svg')).not.toBeNull();
    expect(document.getElementById('crowdsecOutcomeChart').textContent).toContain('Decision attached');
  });

  test('country breakdown renders scaled bars', () => {
    renderCrowdsecStats({
      active_decisions: 0, alerts_24h: 0,
      countries: [{ country: 'cn', count: 100 }, { country: 'ru', count: 50 }],
      scenarios: []
    });
    const rows = document.querySelectorAll('#crowdsecCountries .crowdsec-breakdown-row');
    expect(rows.length).toBe(2);
    const bars = document.querySelectorAll('#crowdsecCountries .crowdsec-breakdown-bar');
    expect(bars[0].style.width).toBe('100%');
    expect(bars[1].style.width).toBe('50%');
  });

  test('breakdowns expose top-ten overflow as an explicit Other row', () => {
    renderCrowdsecStats({
      active_decisions: 0, alerts_24h: 12,
      countries: [{ country: 'us', count: 8 }], countries_other_count: 4,
      scenarios: [], networks: [], sources: [], active_decision_types: [], active_decision_origins: [], hourly: []
    });
    const rows = document.querySelectorAll('#crowdsecCountries .crowdsec-breakdown-row');
    expect(rows).toHaveLength(2);
    expect(rows[1].textContent).toContain('Other');
    expect(rows[1].classList.contains('is-other')).toBe(true);
  });

  test('null countries/scenarios show the empty placeholder', () => {
    renderCrowdsecStats({ active_decisions: 0, alerts_24h: 0, countries: null, scenarios: null });
    expect(document.getElementById('crowdsecCountries').textContent).toContain('No data yet');
    expect(document.getElementById('crowdsecScenarios').textContent).toContain('No data yet');
  });

  test('stats values are HTML-escaped', () => {
    renderCrowdsecStats({
      active_decisions: 0, alerts_24h: 0,
      countries: [{ country: '<script>x</script>', count: 5 }],
      scenarios: [{ scenario: '<b>bold</b>', count: 1 }]
    });
    // No script element materializes from the country value.
    expect(document.querySelector('#crowdsecCountries script')).toBeNull();
    // The <b> in the scenario label is entity-escaped: it renders as text
    // (the DOM element is a .crowdsec-breakdown-label, not a <b> child).
    expect(document.querySelector('#crowdsecScenarios b')).toBeNull();
    // The label shows the literal source text (jsdom serializes tag names
    // uppercased — compare case-insensitively).
    const label = document.querySelector('#crowdsecCountries .crowdsec-breakdown-label');
    expect(label.textContent.toLowerCase()).toBe('<script>x</script>');
  });
});

/* ── live dashboard loading ───────────────────────────────────── */
describe('loadCrowdsecDecisions', () => {
  let originalJ;

  beforeEach(() => {
    originalJ = global.j;
    crowdsecDashboardPromise = null;
    crowdsecIntegrationEnabled = null;
    document.body.innerHTML = `
      <span id="crowdsecSyncBadge"></span>
      <details id="crowdsecSyncDetails"><summary>Issue</summary><div id="crowdsecSyncDetailsMessage"></div></details>
      <div id="crowdsecDecisions"><div>existing decisions</div></div>
      <span id="crowdsecDecisionsSummary"></span>
      <div id="crowdsecDecisionsExpand" class="hidden"><button></button></div>
      <div id="crowdsecAlerts"><div>existing activity</div></div>
      <div id="crowdsecAlertsExpand" class="hidden"><button></button></div>
      <div id="csStatActive"></div><div id="csStatAlerts"></div>
      <div id="csStatTopCountry"></div><div id="csStatTopCountryLabel"></div>
      <div id="csStatTopScenario"></div>
      <div id="crowdsecCountries"></div><div id="crowdsecScenarios"></div>`;
  });

  afterEach(() => {
    global.j = originalJ;
    crowdsecDashboardPromise = null;
    delete global.crowdsecMapApply;
  });

  test('renders successful panels when one endpoint fails', async () => {
    const now = new Date().toISOString();
    global.j = jest.fn(url => {
      if (url.includes('/decisions')) return Promise.reject(new Error('decision endpoint down'));
      if (url.includes('/status')) return Promise.resolve({ last_sync: now });
      if (url.includes('/alerts')) return Promise.resolve({ alerts: [{
        alert_id: 'a1', scenario: 'crowdsecurity/ssh-bf', source_value: '1.2.3.4',
        country: 'gb', created_at: now
      }] });
      return Promise.resolve({
        active_decisions: 9, alerts_24h: 1, top_country: 'gb', top_country_count: 1,
        top_scenario: 'crowdsecurity/ssh-bf', countries: [], scenarios: []
      });
    });
    global.crowdsecMapApply = jest.fn();

    await loadCrowdsecDecisions();

    expect(document.querySelector('.crowdsec-alert-row').textContent).toContain('ssh-bf');
    expect(document.getElementById('csStatActive').textContent).toBe('9');
    expect(document.getElementById('csStatTopCountryLabel').textContent).toContain('(1 detection)');
    expect(document.getElementById('crowdsecDecisions').textContent).toContain('existing decisions');
    expect(document.getElementById('crowdsecSyncBadge').textContent).toContain('Partial data');
    expect(document.getElementById('crowdsecSyncBadge').title).toContain('decisions');
    expect(document.getElementById('crowdsecSyncDetailsMessage').textContent).toContain('Could not refresh: decisions');
    expect(global.j).toHaveBeenCalledWith('/api/admin/crowdsec/alerts?limit=2000&compact=true');
    expect(global.crowdsecMapApply).toHaveBeenCalledTimes(1);
  });

  test('shows disabled cached data without carrying forward an old retry error', async () => {
    const now = new Date().toISOString();
    document.body.insertAdjacentHTML('afterbegin', '<div id="tab-crowdsec" class="active"></div><span id="crowdsecLiveState"></span>');
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    global.j = jest.fn(url => {
      if (url.includes('/decisions')) return Promise.resolve({ decisions: [] });
      if (url.includes('/status')) return Promise.resolve({ enabled: false, last_sync: now, last_error: 'old failure' });
      if (url.includes('/alerts')) return Promise.resolve({ alerts: [] });
      return Promise.resolve({ active_decisions: 0, alerts_24h: 0, countries: [], scenarios: [] });
    });

    await loadCrowdsecDecisions();

    expect(document.getElementById('crowdsecLiveState').textContent).toBe('Integration disabled');
    expect(document.getElementById('crowdsecSyncBadge').textContent).toBe('Integration disabled · cached data');
    expect(document.getElementById('crowdsecSyncDetails').classList.contains('hidden')).toBe(true);
    expect(crowdsecLiveRetrying).toBe(false);
  });

  test('coalesces overlapping refreshes into one request batch', async () => {
    const resolvers = [];
    global.j = jest.fn(() => new Promise(resolve => resolvers.push(resolve)));

    const first = loadCrowdsecDecisions();
    const second = loadCrowdsecDecisions();
    expect(second).toBe(first);
    expect(global.j).toHaveBeenCalledTimes(4);

    resolvers[0]({ decisions: [] });
    resolvers[1]({ last_sync: new Date().toISOString() });
    resolvers[2]({ alerts: [] });
    resolvers[3]({ active_decisions: 0, alerts_24h: 0, countries: [], scenarios: [] });
    await first;
    expect(crowdsecDashboardPromise).toBeNull();
  });
});

describe('CrowdSec live-view visibility', () => {
  let originalJ;

  beforeEach(() => {
    originalJ = global.j;
  });

  afterEach(() => {
    stopCrowdsecAutoRefresh();
    crowdsecDashboardPromise = null;
    crowdsecLiveRetrying = false;
    crowdsecIntegrationEnabled = null;
    global.j = originalJ;
    jest.useRealTimers();
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
  });

  test('is active only when both the CrowdSec tab and page are visible', () => {
    document.body.innerHTML = '<div id="tab-crowdsec" class="tab-content active"></div>';
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    expect(crowdsecDashboardVisible()).toBe(true);

    Object.defineProperty(document, 'hidden', { configurable: true, value: true });
    expect(crowdsecDashboardVisible()).toBe(false);

    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    document.getElementById('tab-crowdsec').classList.remove('active');
    expect(crowdsecDashboardVisible()).toBe(false);
  });

  test('starts and stops the refresh interval with visibility', () => {
    jest.useFakeTimers();
    document.body.innerHTML = `
      <div id="tab-crowdsec" class="tab-content active"></div>
      <span id="crowdsecLiveState"></span>
      <div id="crowdsecDecisions"></div>`;
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    global.j = jest.fn(() => new Promise(() => {}));

    updateCrowdsecAutoRefresh(false);
    expect(crowdsecRefreshTimer).not.toBeNull();
    expect(document.getElementById('crowdsecLiveState').textContent).toBe('Live view on');
    jest.advanceTimersByTime(crowdsecRefreshIntervalMs);
    expect(global.j).toHaveBeenCalledTimes(4);

    Object.defineProperty(document, 'hidden', { configurable: true, value: true });
    updateCrowdsecAutoRefresh(false);
    expect(crowdsecRefreshTimer).toBeNull();
    expect(document.getElementById('crowdsecLiveState').textContent).toBe('Live view paused');

  });

  test('distinguishes a retrying connection from a healthy live view', () => {
    document.body.innerHTML = '<span id="crowdsecLiveState"></span>';
    crowdsecLiveRetrying = true;

    setCrowdsecLiveState(true);

    const indicator = document.getElementById('crowdsecLiveState');
    expect(indicator.textContent).toBe('Live view retrying');
    expect(indicator.classList.contains('is-live')).toBe(false);
    expect(indicator.classList.contains('is-retrying')).toBe(true);
    expect(indicator.getAttribute('aria-label')).toContain('Cached data is shown');
  });

  test('disabled integration takes precedence over tab visibility and retry state', () => {
    document.body.innerHTML = '<span id="crowdsecLiveState"></span>';
    crowdsecIntegrationEnabled = false;
    crowdsecLiveRetrying = true;

    setCrowdsecLiveState(true);
    const indicator = document.getElementById('crowdsecLiveState');
    expect(indicator.textContent).toBe('Integration disabled');
    expect(indicator.classList.contains('is-retrying')).toBe(false);
    expect(indicator.getAttribute('aria-label')).toContain('Previously synced data remains visible');
    setCrowdsecLiveState(false);
    expect(indicator.textContent).toBe('Integration disabled');
  });
});

describe('CrowdSec header sync feedback', () => {
  let originalAction;
  let originalJ;

  beforeEach(() => {
    originalAction = global.handleButtonAction;
    originalJ = global.j;
    document.body.innerHTML = `
      <button id="crowdsecSyncNow"></button>
      <span id="crowdsecSyncFeedback" class="crowdsec-sync-feedback hidden"></span>
      <div id="crowdsecStatus" class="status-message hidden"></div>`;
    global.handleButtonAction = jest.fn(async (button, action, message, onError) => {
      try { await action(); } catch (error) { await onError(error); }
    });
  });

  afterEach(() => {
    global.handleButtonAction = originalAction;
    global.j = originalJ;
  });

  test('shows the manual sync success beside its button', async () => {
    global.j = jest.fn().mockResolvedValue({ success: true });
    await crowdsecSyncNow({ currentTarget: document.getElementById('crowdsecSyncNow') });
    const feedback = document.getElementById('crowdsecSyncFeedback');
    expect(feedback.textContent).toBe('Sync completed');
    expect(feedback.classList.contains('is-success')).toBe(true);
    expect(global.handleButtonAction.mock.calls[0][2]).toBe('Sync completed');
  });

  test('shows the manual sync failure beside its button', async () => {
    global.j = jest.fn().mockRejectedValue({ body: 'LAPI unavailable' });
    await crowdsecSyncNow({ currentTarget: document.getElementById('crowdsecSyncNow') });
    const feedback = document.getElementById('crowdsecSyncFeedback');
    expect(feedback.textContent).toBe('LAPI unavailable');
    expect(feedback.classList.contains('is-error')).toBe(true);
    expect(feedback.getAttribute('aria-live')).toBe('assertive');
  });
});

/* ── toggleCrowdsecSection ─────────────────────────────── */
describe('toggleCrowdsecSection', () => {
  test('settings toggle collapses and expands the body', () => {
    document.body.innerHTML = `
      <button class="crowdsec-collapsible-header" data-crowdsec-toggle="settings" aria-expanded="true">
        <h3>Connection settings</h3><span class="crowdsec-chevron">▾</span>
      </button>
      <div id="crowdsecSettingsBody" class="crowdsec-collapsible-body"></div>`;
    toggleCrowdsecSection('settings');
    let body = document.getElementById('crowdsecSettingsBody');
    let header = document.querySelector('.crowdsec-collapsible-header');
    expect(body.classList.contains('crowdsec-collapsed')).toBe(true);
    expect(header.getAttribute('aria-expanded')).toBe('false');
    expect(header.querySelector('.crowdsec-chevron').textContent).toBe('▸');
    toggleCrowdsecSection('settings');
    expect(body.classList.contains('crowdsec-collapsed')).toBe(false);
    expect(header.getAttribute('aria-expanded')).toBe('true');
    expect(header.querySelector('.crowdsec-chevron').textContent).toBe('▾');
  });

  test('alerts toggle flips expansion and re-renders', () => {
    document.body.innerHTML = `
      <div id="crowdsecAlerts"></div>
      <div class="crowdsec-expand hidden" id="crowdsecAlertsExpand">
        <button type="button" data-crowdsec-toggle="alerts"></button>
      </div>`;
    const many = Array.from({ length: 7 }, (_, i) => ({
      alert_id: String(i), scenario: 'crowdsecurity/ssh-bf', created_at: new Date().toISOString()
    }));
    renderCrowdsecAlerts(many);
    expect(document.querySelectorAll('.crowdsec-alert-row').length).toBe(5);
    toggleCrowdsecSection('alerts');
    expect(crowdsecAlertsExpanded).toBe(true);
    expect(document.querySelectorAll('.crowdsec-alert-row').length).toBe(7);
  });
});

/* ── setCrowdsecSecretField ─────────────────────────────── */
describe('setCrowdsecSecretField', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <input type="password" id="crowdsecBouncerKey" data-empty-placeholder="Bouncer API key">
      <div class="stored-secret-clear"><input type="checkbox" id="clearCrowdsecBouncerKey"></div>
    `;
  });

  test('configured secret shows saved placeholder and enabled clear', () => {
    setCrowdsecSecretField('#crowdsecBouncerKey', '#clearCrowdsecBouncerKey', true);
    const input = document.getElementById('crowdsecBouncerKey');
    const clear = document.getElementById('clearCrowdsecBouncerKey');
    expect(input.placeholder).toBe('Saved — enter a replacement');
    expect(clear.disabled).toBe(false);
    expect(clear.closest('.stored-secret-clear').classList.contains('hidden')).toBe(false);
  });

  test('unset secret shows empty placeholder and hidden clear', () => {
    setCrowdsecSecretField('#crowdsecBouncerKey', '#clearCrowdsecBouncerKey', false);
    const input = document.getElementById('crowdsecBouncerKey');
    const clear = document.getElementById('clearCrowdsecBouncerKey');
    expect(input.placeholder).toBe('Bouncer API key');
    expect(clear.disabled).toBe(true);
    expect(clear.closest('.stored-secret-clear').classList.contains('hidden')).toBe(true);
  });

  test('input value is always cleared on load (never echo secrets)', () => {
    const input = document.getElementById('crowdsecBouncerKey');
    input.value = 'should-be-cleared';
    setCrowdsecSecretField('#crowdsecBouncerKey', '#clearCrowdsecBouncerKey', true);
    expect(input.value).toBe('');
  });
});

/* ── setCrowdsecStatus ─────────────────────────────────── */
describe('setCrowdsecStatus', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="crowdsecStatus" class="status-message hidden"></div>';
  });

  test('sets message and type class', () => {
    setCrowdsecStatus('Saved', 'success');
    const el = document.getElementById('crowdsecStatus');
    expect(el.textContent).toBe('Saved');
    expect(el.classList.contains('status-message')).toBe(true);
    expect(el.classList.contains('success')).toBe(true);
    expect(el.classList.contains('hidden')).toBe(false);
  });

  test('missing element is a no-op', () => {
    expect(() => setCrowdsecStatus('x', 'error')).not.toThrow();
  });
});

/* ── crowdsecErrorMessage ───────────────────────────────── */
describe('crowdsecErrorMessage', () => {
  test('string body wins', () => {
    expect(crowdsecErrorMessage({ body: 'Bad URL' }, 'fallback')).toBe('Bad URL');
  });

  test('object body message wins', () => {
    expect(crowdsecErrorMessage({ body: { message: 'nope' } }, 'fallback')).toBe('nope');
  });

  test('object body error field', () => {
    expect(crowdsecErrorMessage({ body: { error: 'code-x' } }, 'fallback')).toBe('code-x');
  });

  test('falls back to message', () => {
    expect(crowdsecErrorMessage({ message: 'boom' }, 'fallback')).toBe('boom');
  });

  test('ultimate fallback', () => {
    expect(crowdsecErrorMessage({}, 'fallback')).toBe('fallback');
  });
});
