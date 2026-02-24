/**
 * Tests for matrix.js – matrixStatusOf, updateHealthDot, updateStatusSummary, switchView.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'admin-ui.js', 'services.js', 'matrix.js');
});

/* ── matrixStatusOf ─────────────────────────────────────── */
describe('matrixStatusOf', () => {
  afterEach(() => {
    globalThis.latestLiveStatus = null;
  });

  test('returns unknown when no live status', () => {
    globalThis.latestLiveStatus = null;
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('unknown');
    expect(result.statusLabel).toBe('Unknown');
    expect(result.ms).toBeNull();
  });

  test('returns up for operational service', () => {
    globalThis.latestLiveStatus = { plex: { ok: true, ms: 42 } };
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('up');
    expect(result.statusLabel).toBe('Operational');
    expect(result.ms).toBe(42);
  });

  test('returns down for failed service', () => {
    globalThis.latestLiveStatus = { plex: { ok: false, ms: null } };
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('down');
    expect(result.statusLabel).toBe('Down');
  });

  test('returns degraded for slow service', () => {
    globalThis.latestLiveStatus = { plex: { ok: true, degraded: true, ms: 5000 } };
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('degraded');
    expect(result.statusLabel).toBe('Degraded');
  });

  test('returns disabled for disabled service', () => {
    globalThis.latestLiveStatus = { plex: { ok: false, disabled: true } };
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('disabled');
    expect(result.statusLabel).toBe('Disabled');
  });

  test('returns unknown for missing key in status map', () => {
    globalThis.latestLiveStatus = { sonarr: { ok: true } };
    const result = matrixStatusOf({ key: 'plex' });
    expect(result.statusClass).toBe('unknown');
  });
});

/* ── MATRIX_COLORS ──────────────────────────────────────── */
describe('MATRIX_COLORS', () => {
  test('has rgb values for all statuses', () => {
    ['up', 'down', 'degraded', 'disabled'].forEach(status => {
      expect(MATRIX_COLORS).toHaveProperty(status);
      const c = MATRIX_COLORS[status];
      expect(c).toHaveProperty('r');
      expect(c).toHaveProperty('g');
      expect(c).toHaveProperty('b');
    });
  });

  test('up is green-ish', () => {
    expect(MATRIX_COLORS.up.g).toBeGreaterThan(MATRIX_COLORS.up.r);
  });

  test('down is red-ish', () => {
    expect(MATRIX_COLORS.down.r).toBeGreaterThan(MATRIX_COLORS.down.g);
  });
});

/* ── updateHealthDot ────────────────────────────────────── */
describe('updateHealthDot', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="healthDot"></div>';
  });

  test('all-up when every service is ok', () => {
    updateHealthDot({ a: { ok: true }, b: { ok: true } });
    const dot = document.getElementById('healthDot');
    expect(dot.classList.contains('all-up')).toBe(true);
    expect(dot.classList.contains('some-down')).toBe(false);
  });

  test('some-down when any service is not ok', () => {
    updateHealthDot({ a: { ok: true }, b: { ok: false } });
    const dot = document.getElementById('healthDot');
    expect(dot.classList.contains('some-down')).toBe(true);
  });

  test('some-degraded when degraded but nothing down', () => {
    updateHealthDot({ a: { ok: true }, b: { ok: true, degraded: true } });
    const dot = document.getElementById('healthDot');
    expect(dot.classList.contains('some-degraded')).toBe(true);
  });

  test('down takes priority over degraded', () => {
    updateHealthDot({ a: { ok: false }, b: { ok: true, degraded: true } });
    const dot = document.getElementById('healthDot');
    expect(dot.classList.contains('some-down')).toBe(true);
    expect(dot.classList.contains('some-degraded')).toBe(false);
  });

  test('disabled services are ignored', () => {
    updateHealthDot({ a: { ok: false, disabled: true }, b: { ok: true } });
    const dot = document.getElementById('healthDot');
    expect(dot.classList.contains('all-up')).toBe(true);
  });

  test('previous classes are cleared', () => {
    const dot = document.getElementById('healthDot');
    dot.classList.add('some-down');
    updateHealthDot({ a: { ok: true } });
    expect(dot.classList.contains('some-down')).toBe(false);
    expect(dot.classList.contains('all-up')).toBe(true);
  });

  test('no-op when dot element missing', () => {
    document.body.innerHTML = '';
    expect(() => updateHealthDot({ a: { ok: true } })).not.toThrow();
  });
});

/* ── updateStatusSummary ────────────────────────────────── */
describe('updateStatusSummary', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="statusSummary"></div>';
  });

  test('shows operational count', () => {
    updateStatusSummary({ a: { ok: true }, b: { ok: true } });
    const bar = document.getElementById('statusSummary');
    expect(bar.textContent).toContain('2');
    expect(bar.textContent).toContain('Operational');
  });

  test('shows down count', () => {
    updateStatusSummary({ a: { ok: false } });
    const bar = document.getElementById('statusSummary');
    expect(bar.textContent).toContain('1');
    expect(bar.textContent).toContain('Down');
  });

  test('shows degraded count', () => {
    updateStatusSummary({ a: { ok: true, degraded: true } });
    const bar = document.getElementById('statusSummary');
    expect(bar.textContent).toContain('Degraded');
  });

  test('shows disabled count', () => {
    updateStatusSummary({ a: { disabled: true } });
    const bar = document.getElementById('statusSummary');
    expect(bar.textContent).toContain('Disabled');
  });

  test('shows mixed status counts', () => {
    updateStatusSummary({
      a: { ok: true },
      b: { ok: false },
      c: { ok: true, degraded: true },
      d: { disabled: true },
    });
    const bar = document.getElementById('statusSummary');
    expect(bar.textContent).toContain('Operational');
    expect(bar.textContent).toContain('Down');
    expect(bar.textContent).toContain('Degraded');
    expect(bar.textContent).toContain('Disabled');
  });

  test('no-op when bar element missing', () => {
    document.body.innerHTML = '';
    expect(() => updateStatusSummary({ a: { ok: true } })).not.toThrow();
  });
});

/* ── switchView ─────────────────────────────────────────── */
describe('switchView', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <main>
        <div id="services-container"></div>
        <div id="matrix-container" class="hidden"></div>
        <button id="viewCards" class="active"></button>
        <button id="viewMatrix"></button>
      </main>
    `;
    globalThis.currentView = 'cards';
    // stub renderMatrix to avoid needing canvas
    globalThis.renderMatrix = jest.fn();
    globalThis.stopMatrixAnimation = jest.fn();
  });

  test('switching to matrix hides cards, shows matrix', () => {
    switchView('matrix');
    expect(document.getElementById('services-container').classList.contains('hidden')).toBe(true);
    expect(document.getElementById('matrix-container').classList.contains('hidden')).toBe(false);
  });

  test('switching to matrix activates matrix button', () => {
    switchView('matrix');
    expect(document.getElementById('viewMatrix').classList.contains('active')).toBe(true);
    expect(document.getElementById('viewCards').classList.contains('active')).toBe(false);
  });

  test('switching to cards hides matrix, shows cards', () => {
    switchView('matrix');
    switchView('cards');
    expect(document.getElementById('services-container').classList.contains('hidden')).toBe(false);
    expect(document.getElementById('matrix-container').classList.contains('hidden')).toBe(true);
  });

  test('matrix view adds matrix-active to main', () => {
    switchView('matrix');
    expect(document.querySelector('main').classList.contains('matrix-active')).toBe(true);
  });

  test('cards view removes matrix-active from main', () => {
    switchView('matrix');
    switchView('cards');
    expect(document.querySelector('main').classList.contains('matrix-active')).toBe(false);
  });

  test('switching between views is idempotent', () => {
    switchView('matrix');
    switchView('matrix'); // no-op
    expect(document.getElementById('matrix-container').classList.contains('hidden')).toBe(false);
    switchView('cards');
    switchView('cards'); // no-op
    expect(document.getElementById('services-container').classList.contains('hidden')).toBe(false);
  });
});
