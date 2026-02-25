/**
 * Tests for dashboard.js – updCard DOM manipulation.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'resources.js', 'dashboard.js');
});

/* ── helper to build a card DOM ─────────────────────────── */
function buildCard(key) {
  const id = `card-${key}`;
  const html = `
    <section id="${id}" data-key="${key}">
      <div class="card-status-indicator"><span class="pill warn">\u2014</span></div>
      <div class="kpi">\u2014</div>
      <div class="kpi-status">\u2014</div>
      <input type="checkbox" class="monitorToggle" checked>
      <div id="last-check-${key}"></div>
    </section>
  `;
  document.body.insertAdjacentHTML('beforeend', html);
  return id;
}

beforeEach(() => {
  document.body.innerHTML = '';
});

/* ── updCard ────────────────────────────────────────────── */
describe('updCard', () => {
  test('shows UP pill for ok service', () => {
    const id = buildCard('plex');
    updCard(id, { ok: true, status: 200, ms: 42 });
    const pill = document.querySelector(`#${id} .pill`);
    expect(pill.textContent).toBe('UP');
    expect(pill.className).toBe('pill ok');
  });

  test('shows DOWN pill for failed service', () => {
    const id = buildCard('sonarr');
    updCard(id, { ok: false, status: 0, ms: null });
    const pill = document.querySelector(`#${id} .pill`);
    expect(pill.textContent).toBe('DOWN');
    expect(pill.className).toBe('pill down');
  });

  test('shows DEGRADED pill', () => {
    const id = buildCard('radarr');
    updCard(id, { ok: true, status: 200, degraded: true, ms: 5000 });
    const pill = document.querySelector(`#${id} .pill`);
    expect(pill.textContent).toBe('DEGRADED');
    expect(pill.className).toBe('pill warn');
  });

  test('shows DISABLED state', () => {
    const id = buildCard('test');
    updCard(id, { ok: false, disabled: true });
    const pill = document.querySelector(`#${id} .pill`);
    expect(pill.textContent).toBe('DISABLED');
    expect(pill.className).toBe('pill warn');
    const section = document.getElementById(id);
    expect(section.classList.contains('status-disabled')).toBe(true);
  });

  test('unchecks monitorToggle when disabled', () => {
    const id = buildCard('test2');
    updCard(id, { ok: false, disabled: true });
    const toggle = document.querySelector(`#${id} .monitorToggle`);
    expect(toggle.checked).toBe(false);
  });

  test('sets KPI to response time', () => {
    const id = buildCard('kpi');
    updCard(id, { ok: true, status: 200, ms: 150 });
    const kpi = document.querySelector(`#${id} .kpi`);
    expect(kpi.textContent).toBe('150 ms');
  });

  test('sets KPI dash for null ms', () => {
    const id = buildCard('kpi2');
    updCard(id, { ok: true, status: 200, ms: null });
    const kpi = document.querySelector(`#${id} .kpi`);
    expect(kpi.textContent).toBe('\u2014');
  });

  test('HTTP status shown for http check', () => {
    const id = buildCard('http');
    updCard(id, { ok: true, status: 200, ms: 100, check_type: 'http' });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('HTTP 200');
  });

  test('TCP check shows "Port open" when ok', () => {
    const id = buildCard('tcp');
    updCard(id, { ok: true, status: 0, ms: 10, check_type: 'tcp' });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('Port open');
  });

  test('TCP check shows "Connection refused" when down', () => {
    const id = buildCard('tcp2');
    updCard(id, { ok: false, status: 0, ms: null, check_type: 'tcp' });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('Connection refused');
  });

  test('DNS check shows "DNS resolved" when ok', () => {
    const id = buildCard('dns');
    updCard(id, { ok: true, ms: 5, check_type: 'dns' });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('DNS resolved');
  });

  test('always_up check shows "Always up"', () => {
    const id = buildCard('demo');
    updCard(id, { ok: true, ms: 0, check_type: 'always_up' });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('Always up');
  });

  test('status-up class added for ok service', () => {
    const id = buildCard('cls');
    updCard(id, { ok: true, status: 200, ms: 42 });
    expect(document.getElementById(id).classList.contains('status-up')).toBe(true);
  });

  test('status-down class added for down service', () => {
    const id = buildCard('cls2');
    updCard(id, { ok: false, status: 0, ms: null });
    expect(document.getElementById(id).classList.contains('status-down')).toBe(true);
  });

  test('status-degraded class added for degraded service', () => {
    const id = buildCard('cls3');
    updCard(id, { ok: true, degraded: true, status: 200, ms: 5000 });
    expect(document.getElementById(id).classList.contains('status-degraded')).toBe(true);
  });

  test('no-op for missing element (logs error but no throw)', () => {
    // Suppress console.error for this test
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => updCard('nonexistent', { ok: true })).not.toThrow();
    spy.mockRestore();
  });

  test('"No response" for HTTP status 0 when down', () => {
    const id = buildCard('noresp');
    updCard(id, { ok: false, status: 0, ms: null });
    const h = document.querySelector(`#${id} .kpi-status`);
    expect(h.textContent).toBe('No response');
  });
});
