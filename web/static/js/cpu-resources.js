let cpuDetailsExpanded = false;

function hasMetricNumber(value) {
  return value != null && Number.isFinite(Number(value));
}

function normalizeCPUCoreMetrics(snapshot) {
  const snap = snapshot || {};
  const metrics = [];

  if (Array.isArray(snap.cpu_core_metrics) && snap.cpu_core_metrics.length) {
    snap.cpu_core_metrics.forEach(metric => {
      const index = Number(metric && metric.index);
      if (!Number.isInteger(index) || index < 0) return;
      metrics.push({
        index,
        loadPercent: hasMetricNumber(metric.load_percent) ? Number(metric.load_percent) : null,
        tempC: hasMetricNumber(metric.temp_c) ? Number(metric.temp_c) : null
      });
    });
  } else if (Array.isArray(snap.cpu_per_core_percent)) {
    snap.cpu_per_core_percent.forEach((load, index) => {
      metrics.push({
        index,
        loadPercent: hasMetricNumber(load) ? Number(load) : null,
        tempC: null
      });
    });
  }

  return metrics.sort((a, b) => a.index - b.index);
}

function normalizeCPUTemperatureSensors(snapshot) {
  if (!snapshot || !Array.isArray(snapshot.cpu_temperature_sensors)) return [];
  return snapshot.cpu_temperature_sensors
    .map(sensor => ({
      label: String(sensor && sensor.label ? sensor.label : '').trim(),
      tempC: sensor && hasMetricNumber(sensor.temp_c) ? Number(sensor.temp_c) : null
    }))
    .filter(sensor => sensor.label && sensor.tempC != null);
}

function renderCPUCoreMetrics(metrics) {
  const section = document.getElementById('res-cpu-core-section');
  const grid = document.getElementById('res-cpu-core-grid');
  const note = document.getElementById('res-cpu-temp-note');
  if (!grid) return;

  grid.replaceChildren();
  metrics.forEach(metric => {
    const row = document.createElement('div');
    row.className = 'cpu-core-row';
    row.setAttribute(
      'aria-label',
      `CPU ${metric.index}, load ${fmtPct(metric.loadPercent)}, temperature ${fmtTempC(metric.tempC)}`
    );

    const heading = document.createElement('div');
    heading.className = 'cpu-core-row-head';

    const name = document.createElement('strong');
    name.className = 'cpu-core-name';
    name.textContent = `CPU ${metric.index}`;

    const temperature = document.createElement('span');
    temperature.className = 'cpu-core-temperature';
    const temperatureLabel = document.createElement('span');
    temperatureLabel.textContent = 'Temp';
    const temperatureValue = document.createElement('strong');
    temperatureValue.textContent = fmtTempC(metric.tempC);
    temperature.append(temperatureLabel, temperatureValue);
    heading.append(name, temperature);

    const loadLine = document.createElement('div');
    loadLine.className = 'cpu-core-load-line';
    const loadLabel = document.createElement('span');
    loadLabel.textContent = 'Load';
    const loadValue = document.createElement('strong');
    loadValue.textContent = fmtPct(metric.loadPercent);
    loadLine.append(loadLabel, loadValue);

    const meter = document.createElement('div');
    meter.className = 'cpu-core-meter';
    meter.setAttribute('aria-hidden', 'true');
    const fill = document.createElement('div');
    fill.className = 'cpu-core-meter-fill';
    if (metric.loadPercent != null) {
      const percent = Math.max(0, Math.min(100, metric.loadPercent));
      fill.style.width = `${percent}%`;
      const stateClass = meterClassForPct(percent);
      if (stateClass) fill.classList.add(stateClass);
    }
    meter.append(fill);
    row.append(heading, loadLine, meter);
    grid.append(row);
  });

  section?.classList.toggle('hidden', metrics.length === 0);
  const hasUnavailableTemperatures = metrics.some(metric => metric.tempC == null);
  note?.classList.toggle('hidden', metrics.length === 0 || !hasUnavailableTemperatures);
}

function renderCPUTemperatureSensors(sensors) {
  const section = document.getElementById('res-cpu-sensors-section');
  const grid = document.getElementById('res-cpu-sensor-grid');
  if (!grid) return;

  grid.replaceChildren();
  sensors.forEach(sensor => {
    const row = document.createElement('div');
    row.className = 'cpu-sensor-row';
    const label = document.createElement('span');
    label.textContent = sensor.label;
    const value = document.createElement('strong');
    value.textContent = fmtTempC(sensor.tempC);
    row.append(label, value);
    grid.append(row);
  });
  section?.classList.toggle('hidden', sensors.length === 0);
}

function setCPUDetailsExpanded(expanded) {
  const tile = document.querySelector('#card-resources .resource-tile[data-kind="cpu"]')
    || document.querySelector('.resource-tile[data-kind="cpu"]');
  const toggle = document.getElementById('res-cpu-details-toggle');
  const panel = document.getElementById('res-cpu-details');
  const hasDetails = Number(toggle?.dataset.detailCount || 0) > 0;
  cpuDetailsExpanded = Boolean(expanded && isAdminUser && hasDetails);

  tile?.classList.toggle('cpu-expanded', cpuDetailsExpanded);
  toggle?.setAttribute('aria-expanded', cpuDetailsExpanded ? 'true' : 'false');
  panel?.classList.toggle('hidden', !cpuDetailsExpanded);
}

function syncCPUAdminVisibility() {
  const toggle = document.getElementById('res-cpu-details-toggle');
  if (!toggle) return;

  const hasDetails = Number(toggle.dataset.detailCount || 0) > 0;
  const showToggle = Boolean(isAdminUser && hasDetails);
  toggle.classList.toggle('hidden', !showToggle);
  if (!showToggle) {
    setCPUDetailsExpanded(false);
  } else {
    setCPUDetailsExpanded(cpuDetailsExpanded);
  }
}

function bindCPUDetailsToggle() {
  const toggle = document.getElementById('res-cpu-details-toggle');
  if (!toggle || toggle.dataset.bound === 'true') return;
  toggle.dataset.bound = 'true';
  toggle.addEventListener('click', () => setCPUDetailsExpanded(!cpuDetailsExpanded));
}

function updateCPUTile(snapshot) {
  const snap = snapshot || {};
  const metrics = normalizeCPUCoreMetrics(snap);
  const sensors = normalizeCPUTemperatureSensors(snap);
  const reportedCPUCount = hasMetricNumber(snap.cpu_cores)
    ? Math.max(0, Math.round(Number(snap.cpu_cores)))
    : 0;
  const logicalCPUCount = reportedCPUCount > 0 ? reportedCPUCount : metrics.length;

  setResText('res-cpu', fmtPct(snap.cpu_percent));
  setMeter('meter-cpu', snap.cpu_percent);
  setResText('res-cpu-detail', logicalCPUCount > 0 ? logicalCPUCount.toString() : '—');
  setResText('res-cpu-avg-temp', fmtTempC(snap.cpu_avg_temp_c));

  renderCPUCoreMetrics(metrics);
  renderCPUTemperatureSensors(sensors);

  const toggle = document.getElementById('res-cpu-details-toggle');
  if (toggle) toggle.dataset.detailCount = String(metrics.length + sensors.length);
  bindCPUDetailsToggle();
  syncCPUAdminVisibility();
}
