/**
 * Tests for core.js – constants, selectors, formatters, DOM helpers.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  // core.js references `document.addEventListener` at load time
  loadSource('core.js');
});

/* ── fmtMs ──────────────────────────────────────────────── */
describe('fmtMs', () => {
  test('null returns dash', () => {
    expect(fmtMs(null)).toBe('\u2014');
  });
  test('undefined returns dash', () => {
    expect(fmtMs(undefined)).toBe('\u2014');
  });
  test('0 returns "0 ms"', () => {
    expect(fmtMs(0)).toBe('0 ms');
  });
  test('150 returns "150 ms"', () => {
    expect(fmtMs(150)).toBe('150 ms');
  });
  test('string number still works', () => {
    expect(fmtMs('250')).toBe('250 ms');
  });
});

/* ── cls ────────────────────────────────────────────────── */
describe('cls', () => {
  test('not ok → pill down', () => {
    expect(cls(false, 500, false)).toBe('pill down');
  });
  test('ok + not degraded → pill ok', () => {
    expect(cls(true, 200, false)).toBe('pill ok');
  });
  test('ok + degraded → pill warn', () => {
    expect(cls(true, 200, true)).toBe('pill warn');
  });
  test('not ok takes priority over degraded', () => {
    expect(cls(false, 500, true)).toBe('pill down');
  });
});

/* ── fmtBytes ───────────────────────────────────────────── */
describe('fmtBytes', () => {
  test('null returns dash', () => {
    expect(fmtBytes(null)).toBe('\u2014');
  });
  test('undefined returns dash', () => {
    expect(fmtBytes(undefined)).toBe('\u2014');
  });
  test('NaN string returns dash', () => {
    expect(fmtBytes('abc')).toBe('\u2014');
  });
  test('0 → "0 B"', () => {
    expect(fmtBytes(0)).toBe('0 B');
  });
  test('512 → "512 B"', () => {
    expect(fmtBytes(512)).toBe('512 B');
  });
  test('1024 → "1.0 KB"', () => {
    expect(fmtBytes(1024)).toBe('1.0 KB');
  });
  test('1536 → "1.5 KB"', () => {
    expect(fmtBytes(1536)).toBe('1.5 KB');
  });
  test('1048576 → "1.0 MB"', () => {
    expect(fmtBytes(1048576)).toBe('1.0 MB');
  });
  test('1073741824 → "1.00 GB"', () => {
    expect(fmtBytes(1073741824)).toBe('1.00 GB');
  });
  test('1099511627776 → "1.00 TB"', () => {
    expect(fmtBytes(1099511627776)).toBe('1.00 TB');
  });
  test('large value caps at TB', () => {
    // 2.5 TB
    const val = 2.5 * 1099511627776;
    expect(fmtBytes(val)).toBe('2.50 TB');
  });
  test('string number "2048" → "2.0 KB"', () => {
    expect(fmtBytes('2048')).toBe('2.0 KB');
  });
});

/* ── fmtRateBps ─────────────────────────────────────────── */
describe('fmtRateBps', () => {
  test('null returns dash', () => {
    expect(fmtRateBps(null)).toBe('\u2014');
  });
  test('0 → "0 B/s"', () => {
    expect(fmtRateBps(0)).toBe('0 B/s');
  });
  test('1024 → "1.0 KB/s"', () => {
    expect(fmtRateBps(1024)).toBe('1.0 KB/s');
  });
  test('1073741824 → "1.00 GB/s"', () => {
    expect(fmtRateBps(1073741824)).toBe('1.00 GB/s');
  });
});

/* ── fmtPct ─────────────────────────────────────────────── */
describe('fmtPct', () => {
  test('null returns dash', () => {
    expect(fmtPct(null)).toBe('\u2014');
  });
  test('NaN returns dash', () => {
    expect(fmtPct(NaN)).toBe('\u2014');
  });
  test('0 → "0%"', () => {
    expect(fmtPct(0)).toBe('0%');
  });
  test('50.4 → "50%"', () => {
    expect(fmtPct(50.4)).toBe('50%');
  });
  test('99.7 → "100%"', () => {
    expect(fmtPct(99.7)).toBe('100%');
  });
  test('100 → "100%"', () => {
    expect(fmtPct(100)).toBe('100%');
  });
});

/* ── fmtFloat ───────────────────────────────────────────── */
describe('fmtFloat', () => {
  test('null returns dash', () => {
    expect(fmtFloat(null)).toBe('\u2014');
  });
  test('undefined returns dash', () => {
    expect(fmtFloat(undefined)).toBe('\u2014');
  });
  test('3.14159 default 2 digits → "3.14"', () => {
    expect(fmtFloat(3.14159)).toBe('3.14');
  });
  test('3.14159 with 4 digits → "3.1416"', () => {
    expect(fmtFloat(3.14159, 4)).toBe('3.1416');
  });
  test('0 → "0.00"', () => {
    expect(fmtFloat(0)).toBe('0.00');
  });
  test('integer 7 → "7.00"', () => {
    expect(fmtFloat(7)).toBe('7.00');
  });
});

/* ── fmtTempC ───────────────────────────────────────────── */
describe('fmtTempC', () => {
  test('null returns dash', () => {
    expect(fmtTempC(null)).toBe('\u2014');
  });
  test('45.7 → "46°C"', () => {
    expect(fmtTempC(45.7)).toBe('46\u00B0C');
  });
  test('0 → "0°C"', () => {
    expect(fmtTempC(0)).toBe('0\u00B0C');
  });
  test('99.4 → "99°C"', () => {
    expect(fmtTempC(99.4)).toBe('99\u00B0C');
  });
});

/* ── $ / $$ selectors ───────────────────────────────────── */
describe('$ and $$ selectors', () => {
  test('$ finds single element', () => {
    document.body.innerHTML = '<div id="test">Hello</div>';
    expect($('#test').textContent).toBe('Hello');
  });

  test('$ returns null for missing element', () => {
    document.body.innerHTML = '';
    expect($('#nope')).toBeNull();
  });

  test('$$ returns array of elements', () => {
    document.body.innerHTML = '<ul><li>1</li><li>2</li><li>3</li></ul>';
    const items = $$('li');
    expect(Array.isArray(items)).toBe(true);
    expect(items).toHaveLength(3);
  });

  test('$$ scoped to root element', () => {
    document.body.innerHTML = '<div id="a"><span>A</span></div><div id="b"><span>B1</span><span>B2</span></div>';
    const root = document.getElementById('b');
    expect($$('span', root)).toHaveLength(2);
  });
});

/* ── setResText ─────────────────────────────────────────── */
describe('setResText', () => {
  test('sets textContent of element by id', () => {
    document.body.innerHTML = '<span id="cpu-val"></span>';
    setResText('cpu-val', '42%');
    expect(document.getElementById('cpu-val').textContent).toBe('42%');
  });

  test('no-op when element does not exist', () => {
    document.body.innerHTML = '';
    expect(() => setResText('missing', 'x')).not.toThrow();
  });
});

/* ── setResClass ────────────────────────────────────────── */
describe('setResClass', () => {
  test('replaces className of element', () => {
    document.body.innerHTML = '<div id="bar" class="old"></div>';
    setResClass('bar', 'new-class');
    expect(document.getElementById('bar').className).toBe('new-class');
  });

  test('no-op when element does not exist', () => {
    document.body.innerHTML = '';
    expect(() => setResClass('missing', 'cls')).not.toThrow();
  });
});

/* ── REFRESH_MS / DAYS constants ────────────────────────── */
describe('constants', () => {
  test('DAYS is accessible and numeric', () => {
    // DAYS is used by dashboard logic; verify it's set
    expect(typeof DAYS).toBe('number');
  });
  test('isAdminUser defaults to false', () => {
    expect(isAdminUser).toBe(false);
  });
});
