/**
 * Tests for the uptime day-detail dialog rendering helpers.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'day-detail.js');
});

beforeEach(() => {
  document.body.innerHTML = '';
});

function buildHours() {
  return Array.from({ length: 24 }, (_, i) => ({
    hour: `2026-05-31T${String(i).padStart(2, '0')}:00`,
    uptime: i === 2 ? 0 : 100,
    checks: 4,
    avg_ms: 42
  }));
}

describe('day detail rendering', () => {
  test('wraps the hourly grid in a scroll container', () => {
    const container = document.createElement('div');

    renderDayDetailHours(buildHours(), container);

    const scroll = container.querySelector('.dd-hour-scroll');
    const grid = container.querySelector('.dd-hour-grid');
    expect(scroll).not.toBeNull();
    expect(grid).not.toBeNull();
    expect(scroll.contains(grid)).toBe(true);
    expect(grid.children).toHaveLength(24);
  });

  test('renders downtime events with fixed time and wrapping detail elements', () => {
    const container = document.createElement('div');

    renderDayDetailEvents([{
      time: '2026-05-31T08:09:10',
      error: 'connect: connection refused',
      latency_ms: 123
    }], container);

    const row = container.querySelector('.dd-event-row');
    const time = container.querySelector('.dd-event-time');
    const detail = container.querySelector('.dd-event-detail');

    expect(row).not.toBeNull();
    expect(time.tagName.toLowerCase()).toBe('time');
    expect(time.textContent).toMatch(/\d{2}:\d{2}:\d{2}/);
    expect(detail.textContent).toBe('connect: connection refused | 123ms');
  });
});
