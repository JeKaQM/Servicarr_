/**
 * Tests for the public CPU summary and admin processor detail view.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'resources.js', 'cpu-resources.js');
});

describe('updateCPUTile', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <section id="card-resources">
        <div class="resource-tile cpu-tile" data-kind="cpu">
          <div id="res-cpu"></div>
          <div id="meter-cpu" class="meter-fill"></div>
          <div id="res-cpu-detail"></div>
          <div id="res-cpu-avg-temp"></div>
          <button id="res-cpu-details-toggle" class="hidden" aria-expanded="false"></button>
          <div id="res-cpu-details" class="hidden">
            <div id="res-cpu-core-section">
              <div id="res-cpu-core-grid"></div>
              <div id="res-cpu-temp-note" class="hidden"></div>
            </div>
            <div id="res-cpu-sensors-section" class="hidden">
              <div id="res-cpu-sensor-grid"></div>
            </div>
          </div>
        </div>
      </section>
    `;
    globalThis.isAdminUser = false;
    setCPUDetailsExpanded(false);
  });

  test('keeps the public summary concise', () => {
    updateCPUTile({
      cpu_percent: 28,
      cpu_cores: 32,
      cpu_avg_temp_c: 61,
      cpu_core_metrics: [
        { index: 0, load_percent: 31, temp_c: 58 },
        { index: 1, load_percent: 9, temp_c: 59 }
      ]
    });

    expect(document.getElementById('res-cpu').textContent).toBe('28%');
    expect(document.getElementById('meter-cpu').style.width).toBe('28%');
    expect(document.getElementById('res-cpu-detail').textContent).toBe('32');
    expect(document.getElementById('res-cpu-avg-temp').textContent).toBe('61°C');
    expect(document.getElementById('res-cpu-details-toggle').classList.contains('hidden')).toBe(true);
    expect(document.getElementById('res-cpu-details').classList.contains('hidden')).toBe(true);
  });

  test('lets an admin expand individual processor readings', () => {
    globalThis.isAdminUser = true;
    updateCPUTile({
      cpu_percent: 22,
      cpu_cores: 2,
      cpu_avg_temp_c: 56.5,
      cpu_core_metrics: [
        { index: 0, load_percent: 31, temp_c: 55 },
        { index: 1, load_percent: 13, temp_c: 58 }
      ]
    });

    const toggle = document.getElementById('res-cpu-details-toggle');
    toggle.click();

    expect(toggle.classList.contains('hidden')).toBe(false);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(document.querySelector('[data-kind="cpu"]').classList.contains('cpu-expanded')).toBe(true);
    expect(document.querySelectorAll('.cpu-core-row')).toHaveLength(2);
    expect(document.querySelector('.cpu-core-row').textContent).toContain('CPU 0');
    expect(document.querySelector('.cpu-core-row').textContent).toContain('31%');
    expect(document.querySelector('.cpu-core-row').textContent).toContain('55°C');
  });

  test('shows real thermal sensors without assigning them to logical CPUs', () => {
    globalThis.isAdminUser = true;
    updateCPUTile({
      cpu_percent: 12,
      cpu_cores: 2,
      cpu_avg_temp_c: 61,
      cpu_core_metrics: [
        { index: 0, load_percent: 15 },
        { index: 1, load_percent: 9 }
      ],
      cpu_temperature_sensors: [
        { label: 'Tccd1', temp_c: 60 },
        { label: 'Tccd2', temp_c: 62 },
        { label: 'Tctl', temp_c: 69 }
      ]
    });

    document.getElementById('res-cpu-details-toggle').click();

    expect(document.getElementById('res-cpu-temp-note').classList.contains('hidden')).toBe(false);
    expect(document.querySelector('.cpu-core-temperature strong').textContent).toBe('—');
    expect(document.querySelectorAll('.cpu-sensor-row')).toHaveLength(3);
    expect(document.getElementById('res-cpu-sensor-grid').textContent).toContain('Tccd1');
    expect(document.getElementById('res-cpu-sensor-grid').textContent).toContain('60°C');
  });

  afterEach(() => {
    globalThis.isAdminUser = false;
    setCPUDetailsExpanded(false);
  });
});
