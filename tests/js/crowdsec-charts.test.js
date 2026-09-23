/** Tests for dependency-free CrowdSec SVG charts. */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'crowdsec-charts.js');
});

function hourly(count = 24) {
  const start = Date.parse('2026-09-20T12:00:00Z');
  return Array.from({ length: count }, (_, index) => ({
    start: new Date(start + index * 3600000).toISOString(),
    detections: index,
    with_decision: Math.floor(index / 3),
    reported_events: index * 2
  }));
}

describe('renderCrowdsecTimeline', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="crowdsecTimeline"></div>';
    crowdsecTimelineHours = 24;
  });

  test('renders accessible stacked bars, event line, and exact data table', () => {
    renderCrowdsecTimeline({ hourly: hourly() }, 24);
    const svg = document.querySelector('#crowdsecTimeline svg');
    expect(svg).not.toBeNull();
    expect(svg.getAttribute('role')).toBe('img');
    expect(svg.querySelector('title').textContent).toContain('detection activity');
    expect(document.querySelectorAll('.crowdsec-chart-bucket')).toHaveLength(24);
    expect(document.querySelectorAll('.crowdsec-bar-actioned')).toHaveLength(24);
    expect(document.querySelector('.crowdsec-events-line').getAttribute('points')).not.toBe('');
    expect(document.querySelectorAll('.crowdsec-chart-data-row')).toHaveLength(25); // header + values
    expect(document.querySelectorAll('.crowdsec-chart-bucket[tabindex]')).toHaveLength(0);
    expect(svg.textContent).toContain('events');
  });

  test('uses compact chart geometry without shrinking phone labels from a desktop viewBox', () => {
    const container = document.getElementById('crowdsecTimeline');
    container.getBoundingClientRect = () => ({ width: 320, height: 0 });
    renderCrowdsecTimeline({ hourly: hourly() }, 24);
    const svg = container.querySelector('svg');
    expect(svg.getAttribute('viewBox')).toBe('0 0 320 250');
    expect(svg.getAttribute('width')).toBe('320');
    expect(svg.getAttribute('height')).toBe('250');
  });

  test('keeps the exact-values disclosure open across live refreshes', () => {
    const stats = { hourly: hourly() };
    renderCrowdsecTimeline(stats, 24);
    document.querySelector('.crowdsec-chart-data').open = true;
    const refreshed = { hourly: hourly() };
    refreshed.hourly[23].reported_events = 999;
    renderCrowdsecTimeline(refreshed, 24);
    expect(document.querySelector('.crowdsec-chart-data').open).toBe(true);
    expect(document.getElementById('crowdsecTimeline').textContent).toContain('999');
  });

  test('range selection slices the newest buckets and updates pressed state', () => {
    document.body.innerHTML = `
      <button data-crowdsec-range="6" aria-pressed="false"></button>
      <button data-crowdsec-range="24" aria-pressed="true"></button>
      <div id="crowdsecTimeline"></div>`;
    crowdsecLastStats = { hourly: hourly() };
    setCrowdsecTimelineRange(6);
    expect(document.querySelectorAll('.crowdsec-chart-bucket')).toHaveLength(6);
    expect(document.querySelector('[data-crowdsec-range="6"]').getAttribute('aria-pressed')).toBe('true');
    expect(document.querySelector('[data-crowdsec-range="24"]').getAttribute('aria-pressed')).toBe('false');
  });

  test('does not materialise hostile timestamps as HTML', () => {
    renderCrowdsecTimeline({ hourly: [{ start: '<img src=x onerror=alert(1)>', detections: 1, with_decision: 0, reported_events: 1 }] }, 24);
    expect(document.querySelector('#crowdsecTimeline img')).toBeNull();
  });

  test('shows an honest empty state without hourly data', () => {
    renderCrowdsecTimeline({ hourly: [] }, 24);
    expect(document.getElementById('crowdsecTimeline').textContent).toContain('No hourly statistics');
  });
});

describe('renderCrowdsecOutcomeChart', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="crowdsecOutcomeChart"></div>';
  });

  test('shows direct counts and an accessible rate', () => {
    renderCrowdsecOutcomeChart({ alerts_24h: 40, alerts_with_decision_24h: 10 });
    const chart = document.getElementById('crowdsecOutcomeChart');
    expect(chart.textContent).toContain('25.0%');
    expect(chart.textContent).toContain('Decision attached');
    expect(chart.textContent).toContain('Detection only');
    expect(chart.querySelector('svg').getAttribute('aria-label')).toContain('10 of 40');
  });

  test('is zero-safe', () => {
    renderCrowdsecOutcomeChart({ alerts_24h: 0, alerts_with_decision_24h: 0 });
    expect(document.getElementById('crowdsecOutcomeChart').textContent).toContain('0%');
    expect(document.querySelector('.crowdsec-ring-value').getAttribute('stroke-dasharray')).not.toContain('NaN');
    expect(document.querySelector('.crowdsec-ring-value').classList.contains('is-empty')).toBe(true);
  });
});
