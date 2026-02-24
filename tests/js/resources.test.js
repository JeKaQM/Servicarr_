/**
 * Tests for resources.js – meterClassForPct, setMeter, applyAdminUIState.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'resources.js');
});

/* ── meterClassForPct ───────────────────────────────────── */
describe('meterClassForPct', () => {
  test('null returns empty string', () => {
    expect(meterClassForPct(null)).toBe('');
  });
  test('undefined returns empty string', () => {
    expect(meterClassForPct(undefined)).toBe('');
  });
  test('NaN returns empty string', () => {
    expect(meterClassForPct(NaN)).toBe('');
  });
  test('0 returns empty (normal)', () => {
    expect(meterClassForPct(0)).toBe('');
  });
  test('74 returns empty (normal)', () => {
    expect(meterClassForPct(74)).toBe('');
  });
  test('75 returns "warn"', () => {
    expect(meterClassForPct(75)).toBe('warn');
  });
  test('89 returns "warn"', () => {
    expect(meterClassForPct(89)).toBe('warn');
  });
  test('90 returns "bad"', () => {
    expect(meterClassForPct(90)).toBe('bad');
  });
  test('100 returns "bad"', () => {
    expect(meterClassForPct(100)).toBe('bad');
  });
  test('string "85" returns "warn"', () => {
    expect(meterClassForPct('85')).toBe('warn');
  });
});

/* ── setMeter ───────────────────────────────────────────── */
describe('setMeter', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="cpu-meter" class="" style="width: 0%"></div>';
  });

  test('sets width to percentage', () => {
    setMeter('cpu-meter', 50);
    expect(document.getElementById('cpu-meter').style.width).toBe('50%');
  });

  test('clamps at 100%', () => {
    setMeter('cpu-meter', 150);
    expect(document.getElementById('cpu-meter').style.width).toBe('100%');
  });

  test('clamps at 0%', () => {
    setMeter('cpu-meter', -10);
    expect(document.getElementById('cpu-meter').style.width).toBe('0%');
  });

  test('null resets to 0% and removes classes', () => {
    const el = document.getElementById('cpu-meter');
    el.classList.add('bad');
    el.style.width = '90%';
    setMeter('cpu-meter', null);
    expect(el.style.width).toBe('0%');
    expect(el.classList.contains('bad')).toBe(false);
  });

  test('NaN resets to 0%', () => {
    setMeter('cpu-meter', NaN);
    expect(document.getElementById('cpu-meter').style.width).toBe('0%');
  });

  test('adds "warn" class for 80%', () => {
    setMeter('cpu-meter', 80);
    expect(document.getElementById('cpu-meter').classList.contains('warn')).toBe(true);
  });

  test('adds "bad" class for 95%', () => {
    setMeter('cpu-meter', 95);
    expect(document.getElementById('cpu-meter').classList.contains('bad')).toBe(true);
  });

  test('clears previous warn/bad when setting low value', () => {
    const el = document.getElementById('cpu-meter');
    el.classList.add('bad');
    setMeter('cpu-meter', 50);
    expect(el.classList.contains('bad')).toBe(false);
    expect(el.classList.contains('warn')).toBe(false);
  });

  test('no-op when element not found', () => {
    expect(() => setMeter('nonexistent', 50)).not.toThrow();
  });
});

/* ── applyAdminUIState ──────────────────────────────────── */
describe('applyAdminUIState', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="adminPanel" class="hidden"></div>
      <div class="adminRow hidden"></div>
      <div class="adminRow hidden"></div>
    `;
  });

  test('shows admin elements when isAdminUser is true', () => {
    globalThis.isAdminUser = true;
    applyAdminUIState();
    const panel = document.getElementById('adminPanel');
    expect(panel.classList.contains('hidden')).toBe(false);
    const rows = document.querySelectorAll('.adminRow');
    rows.forEach(r => expect(r.classList.contains('hidden')).toBe(false));
  });

  test('hides admin elements when isAdminUser is false', () => {
    globalThis.isAdminUser = false;
    applyAdminUIState();
    const panel = document.getElementById('adminPanel');
    expect(panel.classList.contains('hidden')).toBe(true);
    const rows = document.querySelectorAll('.adminRow');
    rows.forEach(r => expect(r.classList.contains('hidden')).toBe(true));
  });

  afterAll(() => {
    globalThis.isAdminUser = false;
  });
});
