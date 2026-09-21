/**
 * Tests for crowdsec-tab.js – badge text, dashboard rendering, collapsible
 * lists, activity feed, stats cards, and secret fields.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'crowdsec-tab.js');
});

/* ── crowdsecSyncBadgeText ─────────────────────────────── */
describe('crowdsecSyncBadgeText', () => {
  test('null status means not synced yet', () => {
    expect(crowdsecSyncBadgeText(null)).toBe('Not synced yet');
  });

  test('empty status means not synced yet', () => {
    expect(crowdsecSyncBadgeText({})).toBe('Not synced yet');
  });

  test('auth failure shows credential hint', () => {
    const text = crowdsecSyncBadgeText({ last_error: 'http 401: bad key', auth_failed: true });
    expect(text).toBe('Auth failed — check credentials');
  });

  test('sync error surfaces sanitized text', () => {
    const text = crowdsecSyncBadgeText({ last_error: 'LAPI unreachable', auth_failed: false });
    expect(text).toBe('Sync error: LAPI unreachable');
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
    expect(html).toContain('banned');
    expect(html).toContain('7×');
    // The display scenario strips the crowdsecurity/ prefix…
    const scenarioEl = document.querySelector('.crowdsec-alert-scenario');
    expect(scenarioEl.textContent).toBe('ssh-bf');
    // …but the tooltip keeps the full name for operator clarity.
    const row = document.querySelector('.crowdsec-alert-row');
    expect(row.getAttribute('title')).toContain('crowdsecurity/ssh-bf');
    expect(row.tabIndex).toBe(0);
    expect(row.getAttribute('aria-label')).toContain('banned');
  });

  test('alerts without coordinates are not exposed as map controls', () => {
    renderCrowdsecAlerts([{
      alert_id: 'no-geo', scenario: 'crowdsecurity/http-probing', source_value: '6.6.6.6',
      country: 'ru', has_decision: false, created_at: new Date().toISOString()
    }]);
    const row = document.querySelector('.crowdsec-alert-row');
    expect(row.classList.contains('is-mappable')).toBe(false);
    expect(row.hasAttribute('tabindex')).toBe(false);
    expect(row.hasAttribute('role')).toBe(false);
    expect(row.hasAttribute('aria-controls')).toBe(false);
  });

  test('scan without decision shows the no-ban outcome', () => {
    renderCrowdsecAlerts([{
      alert_id: '43', scenario: 'crowdsecurity/http-probing', source_value: '6.6.6.6',
      country: 'ru', has_decision: false, created_at: new Date().toISOString()
    }]);
    expect(document.getElementById('crowdsecAlerts').innerHTML).toContain('no ban');
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
      <div id="csStatTopCountry"></div>
      <div id="csStatTopCountryLabel"></div>
      <div id="csStatTopScenario"></div>
      <div id="crowdsecCountries"></div>
      <div id="crowdsecScenarios"></div>`;
  });

  test('stat cards fill from aggregates', () => {
    renderCrowdsecStats({
      active_decisions: 12, alerts_24h: 47,
      top_country: 'cn', top_country_count: 30,
      top_scenario: 'crowdsecurity/ssh-bf',
      countries: [{ country: 'cn', count: 30 }, { country: 'ru', count: 17 }],
      scenarios: [{ scenario: 'crowdsecurity/ssh-bf', count: 40 }]
    });
    expect(document.getElementById('csStatActive').textContent).toBe('12');
    expect(document.getElementById('csStatAlerts').textContent).toBe('47');
    expect(document.getElementById('csStatTopCountry').textContent).toBe('CN');
    expect(document.getElementById('csStatTopCountryLabel').textContent).toContain('30 detections');
    expect(document.getElementById('csStatTopScenario').textContent).toBe('ssh-bf');
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
    document.body.innerHTML = `
      <span id="crowdsecSyncBadge"></span>
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
    expect(document.getElementById('crowdsecDecisions').textContent).toContain('existing decisions');
    expect(document.getElementById('crowdsecSyncBadge').textContent).toContain('Partial data');
    expect(document.getElementById('crowdsecSyncBadge').title).toContain('decisions');
    expect(global.crowdsecMapApply).toHaveBeenCalledTimes(1);
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
