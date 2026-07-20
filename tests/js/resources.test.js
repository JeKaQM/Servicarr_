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
describe('batteryClassForPct', () => {
  test('null returns empty string', () => {
    expect(batteryClassForPct(null)).toBe('');
  });
  test('100 returns empty (healthy)', () => {
    expect(batteryClassForPct(100)).toBe('');
  });
  test('50 returns "warn"', () => {
    expect(batteryClassForPct(50)).toBe('warn');
  });
  test('20 returns "bad"', () => {
    expect(batteryClassForPct(20)).toBe('bad');
  });
});

describe('setBatteryMeter', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="ups-meter" class="" style="width: 0%"></div>';
  });

  test('sets width to battery percentage', () => {
    setBatteryMeter('ups-meter', 87);
    expect(document.getElementById('ups-meter').style.width).toBe('87%');
  });

  test('adds "bad" class for low battery', () => {
    setBatteryMeter('ups-meter', 12);
    expect(document.getElementById('ups-meter').classList.contains('bad')).toBe(true);
  });

  test('does not mark full battery as bad', () => {
    setBatteryMeter('ups-meter', 100);
    const el = document.getElementById('ups-meter');
    expect(el.classList.contains('bad')).toBe(false);
    expect(el.classList.contains('warn')).toBe(false);
  });
});

describe('fmtDurationSeconds', () => {
  test('formats seconds', () => {
    expect(fmtDurationSeconds(45)).toBe('45s');
  });
  test('formats minutes', () => {
    expect(fmtDurationSeconds(1446)).toBe('24m');
  });
  test('formats hours and minutes', () => {
    expect(fmtDurationSeconds(7320)).toBe('2h 2m');
  });
});

describe('fmtWatts', () => {
  test('formats watts', () => {
    expect(fmtWatts(86.1)).toBe('86 W');
  });
  test('formats kilowatts', () => {
    expect(fmtWatts(1250)).toBe('1.25 kW');
  });
  test('handles missing values', () => {
    expect(fmtWatts(null)).toBe('—');
  });
});

describe('setUPSState', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <section id="card-resources">
        <div class="resource-tile ups-tile" data-kind="ups"></div>
      </section>
      <div id="res-ups-state" class="ups-state ups-state-unknown">
        <span id="res-ups-state-label"></span>
        <strong id="res-ups-status"></strong>
      </div>
    `;
  });

  test('marks online UPS as line power present', () => {
    setUPSState({ power_present: true, status_text: 'Online' });

    expect(document.getElementById('res-ups-state-label').textContent).toBe('Line power present');
    expect(document.getElementById('res-ups-status').textContent).toBe('Online');
    expect(document.getElementById('res-ups-state').classList.contains('ups-state-ok')).toBe(true);
    expect(document.querySelector('[data-kind="ups"]').classList.contains('ups-warning')).toBe(false);
  });

  test('marks on-battery UPS as warning with readable text', () => {
    setUPSState({ power_present: false, status_text: 'On battery, Discharging' });

    expect(document.getElementById('res-ups-state-label').textContent).toBe('Mains power lost');
    expect(document.getElementById('res-ups-status').textContent).toBe('Running on UPS battery - Discharging');
    expect(document.getElementById('res-ups-state').classList.contains('ups-state-warning')).toBe(true);
    expect(document.querySelector('[data-kind="ups"]').classList.contains('ups-warning')).toBe(true);
  });

  test('marks low battery as critical even when line power is present', () => {
    setUPSState({
      power_present: true,
      status: 'OL LB',
      status_text: 'Online, Low battery',
      battery_charge_percent: 10
    });

    expect(document.getElementById('res-ups-state-label').textContent).toBe('Battery critically low');
    expect(document.getElementById('res-ups-state').classList.contains('ups-state-critical')).toBe(true);
    expect(document.querySelector('[data-kind="ups"]').classList.contains('ups-critical')).toBe(true);
    expect(document.querySelector('[data-kind="ups"]').classList.contains('ups-warning')).toBe(false);
  });
});

describe('getUPSAlertState', () => {
  test('does not render a discharging UPS as healthy', () => {
    expect(getUPSAlertState({ power_present: true, status: 'OL DISCHRG' })).toBe('warning');
  });

  test('treats overload as critical', () => {
    expect(getUPSAlertState({ power_present: true, status: 'OL OVER' })).toBe('critical');
  });
});

describe('applyResourcesVisibility', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <section id="card-resources" class="hidden">
        <div class="resource-tile hidden" data-kind="cpu"></div>
        <div class="resource-tile hidden" data-kind="ups"></div>
      </section>
    `;
  });

  test('shows UPS tile without Glances when NUT is configured', () => {
    applyResourcesVisibility({
      enabled: true,
      glances_url: '',
      nut_host: 'server.local:3493',
      ups_name: 'apc',
      cpu: true,
      ups: true
    });

    const section = document.getElementById('card-resources');
    const cpu = document.querySelector('[data-kind="cpu"]');
    const ups = document.querySelector('[data-kind="ups"]');
    expect(section.classList.contains('hidden')).toBe(false);
    expect(cpu.classList.contains('hidden')).toBe(true);
    expect(ups.classList.contains('hidden')).toBe(false);
  });

  test('uses redacted public source flags', () => {
    applyResourcesVisibility({
      enabled: true,
      glances_configured: false,
      ups_configured: true,
      cpu: true,
      ups: true
    });

    expect(document.getElementById('card-resources').classList.contains('hidden')).toBe(false);
    expect(document.querySelector('[data-kind="cpu"]').classList.contains('hidden')).toBe(true);
    expect(document.querySelector('[data-kind="ups"]').classList.contains('hidden')).toBe(false);
  });

  test('hides section when no source is configured', () => {
    applyResourcesVisibility({
      enabled: true,
      glances_url: '',
      nut_host: '',
      ups_name: '',
      cpu: true,
      ups: true
    });

    expect(document.getElementById('card-resources').classList.contains('hidden')).toBe(true);
  });

  test('hides section when configured sources have no visible tiles', () => {
    applyResourcesVisibility({
      enabled: true,
      glances_configured: true,
      ups_configured: true,
      cpu: false,
      ups: false
    });

    expect(document.getElementById('card-resources').classList.contains('hidden')).toBe(true);
  });
});

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
