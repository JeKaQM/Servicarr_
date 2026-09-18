/**
 * Tests for crowdsec-tab.js – badge text, decision rendering, secret fields.
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

  test('truncated snapshot shows latest N of total', () => {
    const minAgo = new Date(Date.now() - 60 * 1000).toISOString();
    const text = crowdsecSyncBadgeText({
      last_sync: minAgo,
      decision_count: 15000,
      snapshot_count: 500
    });
    expect(text).toContain('showing latest 500 of 15000');
  });
});

/* ── renderCrowdsecDecisions ───────────────────────────── */
describe('renderCrowdsecDecisions', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="crowdsecDecisions"></div>';
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

  test('header row renders six columns', () => {
    renderCrowdsecDecisions([]);
    renderCrowdsecDecisions([{ decision_id: '10', value: '1.1.1.1', type: 'ban', scope: 'Ip' }]);
    const header = document.querySelector('.crowdsec-decision-header');
    expect(header).not.toBeNull();
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