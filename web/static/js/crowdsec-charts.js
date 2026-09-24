// Dependency-free visualisations for the CrowdSec dashboard. The endpoint
// supplies bounded, zero-filled hourly or daily buckets for the selected range.

var crowdsecLastStats = null;
var crowdsecTimelineResizeObserver = null;
var crowdsecTimelineObservedElement = null;
var crowdsecTimelineObservedWidth = 0;

function crowdsecChartNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
}

function crowdsecFormatNumber(value) {
  return Math.round(crowdsecChartNumber(value)).toLocaleString();
}

function crowdsecChartEscape(value) {
  if (typeof escapeHtml === 'function') return escapeHtml(String(value));
  return String(value).replace(/[&<>"']/g, char => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'
  })[char]);
}

function crowdsecHourLabel(value, includeDate) {
  const date = new Date(value || '');
  if (Number.isNaN(date.getTime())) return 'Unknown hour';
  const options = includeDate
    ? { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
    : { hour: '2-digit', minute: '2-digit' };
  return new Intl.DateTimeFormat(undefined, options).format(date);
}

function crowdsecNormaliseBuckets(stats) {
  const all = Array.isArray(stats && stats.hourly) ? stats.hourly.slice() : [];
  all.sort((left, right) => new Date(left.start).getTime() - new Date(right.start).getTime());
  return all.map(bucket => ({
    start: bucket.start,
    detections: crowdsecChartNumber(bucket.detections),
    withDecision: Math.min(crowdsecChartNumber(bucket.with_decision), crowdsecChartNumber(bucket.detections)),
    events: crowdsecChartNumber(bucket.reported_events)
  }));
}

function crowdsecBucketLabel(value, stats, full = false) {
  if (stats && stats.bucket_unit === 'day') {
    const date = new Date(value || '');
    if (Number.isNaN(date.getTime())) return 'Unknown period';
    return new Intl.DateTimeFormat(undefined, full
      ? { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
      : { month: 'short', day: 'numeric' }).format(date);
  }
  return crowdsecHourLabel(value, full);
}

function crowdsecBucketDescription(bucket, stats) {
  const start = crowdsecBucketLabel(bucket.start, stats, true);
  const seconds = Number(stats && stats.bucket_seconds) || 3600;
  if (seconds <= 3600 || start === 'Unknown period' || start === 'Unknown hour') return start;
  const end = new Date(new Date(bucket.start).getTime() + seconds * 1000);
  return `${start} to ${crowdsecBucketLabel(end.toISOString(), stats, true)}`;
}

function crowdsecTimelineDimensions(container) {
  const rect = container && typeof container.getBoundingClientRect === 'function'
    ? container.getBoundingClientRect()
    : null;
  const measured = Math.floor((rect && rect.width) || (container && container.clientWidth) || 900);
  const width = Math.max(300, Math.min(900, measured));
  const compact = width < 520;
  return {
    width,
    height: compact ? 250 : 270,
    margin: compact
      ? { top: 20, right: 45, bottom: 38, left: 32 }
      : { top: 18, right: 48, bottom: 38, left: 40 }
  };
}

function crowdsecObserveTimeline(container) {
  if (!container || typeof ResizeObserver !== 'function') return;
  if (crowdsecTimelineObservedElement === container && crowdsecTimelineResizeObserver) return;
  if (crowdsecTimelineResizeObserver) crowdsecTimelineResizeObserver.disconnect();
  crowdsecTimelineObservedElement = container;
  crowdsecTimelineObservedWidth = 0;
  crowdsecTimelineResizeObserver = new ResizeObserver(entries => {
    const entry = entries && entries[0];
    const width = Math.floor(entry && entry.contentRect ? entry.contentRect.width : 0);
    if (!width || Math.abs(width - crowdsecTimelineObservedWidth) < 2) return;
    crowdsecTimelineObservedWidth = width;
    if (crowdsecLastStats) renderCrowdsecTimeline(crowdsecLastStats);
  });
  crowdsecTimelineResizeObserver.observe(container);
}

function renderCrowdsecTimeline(stats) {
  const container = $('#crowdsecTimeline');
  if (!container) return;
  crowdsecObserveTimeline(container);
  const buckets = crowdsecNormaliseBuckets(stats);
  if (!buckets.length) {
    container.innerHTML = '<div class="crowdsec-chart-empty">No activity buckets available for this period.</div>';
    delete container.dataset.renderSignature;
    return;
  }

  const dimensions = crowdsecTimelineDimensions(container);
  const width = dimensions.width;
  const height = dimensions.height;
  const margin = dimensions.margin;
  const plotW = width - margin.left - margin.right;
  const plotH = height - margin.top - margin.bottom;
  const maxDetectionValue = Math.max(0, ...buckets.map(bucket => bucket.detections));
  const maxEventValue = Math.max(0, ...buckets.map(bucket => bucket.events));
  const detectionScale = Math.max(1, maxDetectionValue);
  const eventScale = Math.max(1, maxEventValue);
  const slot = plotW / buckets.length;
  const barWidth = Math.min(26, slot * 0.58);

  const grid = [0, 0.5, 1].map(ratio => {
    const y = margin.top + plotH - ratio * plotH;
    const value = Math.round(maxDetectionValue * ratio);
    const label = maxDetectionValue === 0 && ratio > 0
      ? ''
      : `<text x="${margin.left - 9}" y="${y + 4}" text-anchor="end" class="crowdsec-chart-axis">${value}</text>`;
    return `<line x1="${margin.left}" y1="${y}" x2="${width - margin.right}" y2="${y}" class="crowdsec-chart-grid" />` +
      label;
  }).join('');

  const bars = buckets.map((bucket, index) => {
    const x = margin.left + index * slot + (slot - barWidth) / 2;
    const actionedHeight = (bucket.withDecision / detectionScale) * plotH;
    const observedHeight = ((bucket.detections - bucket.withDecision) / detectionScale) * plotH;
    const baseY = margin.top + plotH;
    const title = `${crowdsecBucketDescription(bucket, stats)}: ${crowdsecFormatNumber(bucket.detections)} detections, ${crowdsecFormatNumber(bucket.withDecision)} with a decision, ${crowdsecFormatNumber(bucket.events)} reported events`;
    const showLabel = index === 0 || index === buckets.length - 1 || index % Math.max(1, Math.ceil(buckets.length / 6)) === 0;
    return `<g class="crowdsec-chart-bucket">` +
      `<title>${crowdsecChartEscape(title)}</title>` +
      `<rect x="${x}" y="${baseY - observedHeight}" width="${barWidth}" height="${Math.max(0, observedHeight)}" rx="3" class="crowdsec-bar-observed" />` +
      `<rect x="${x}" y="${baseY - observedHeight - actionedHeight}" width="${barWidth}" height="${Math.max(0, actionedHeight)}" rx="3" class="crowdsec-bar-actioned" />` +
      (showLabel ? `<text x="${x + barWidth / 2}" y="${height - 13}" text-anchor="middle" class="crowdsec-chart-axis">${crowdsecChartEscape(crowdsecBucketLabel(bucket.start, stats))}</text>` : '') +
      '</g>';
  }).join('');

  const eventPoints = buckets.map((bucket, index) => {
    const x = margin.left + index * slot + slot / 2;
    const y = margin.top + plotH - (bucket.events / eventScale) * plotH;
    return `${x},${y}`;
  }).join(' ');
  const eventUnit = maxEventValue === 1 ? 'event' : 'events';
  const eventAxis = `<text x="${width - 4}" y="${margin.top + 4}" text-anchor="end" class="crowdsec-chart-axis crowdsec-chart-axis-events">${crowdsecFormatNumber(maxEventValue)} ${eventUnit}</text>` +
    (maxEventValue > 0 ? `<text x="${width - 4}" y="${margin.top + plotH + 4}" text-anchor="end" class="crowdsec-chart-axis crowdsec-chart-axis-events">0</text>` : '');
  const totalDetections = buckets.reduce((sum, bucket) => sum + bucket.detections, 0);
  const totalEvents = buckets.reduce((sum, bucket) => sum + bucket.events, 0);
  const period = typeof crowdsecHistoryLabel === 'function' ? crowdsecHistoryLabel() : 'selected period';
  const description = `Observed activity in ${period}: ${crowdsecFormatNumber(totalDetections)} detections and ${crowdsecFormatNumber(totalEvents)} reported events across ${buckets.length} ${stats && stats.bucket_unit === 'day' ? 'daily' : 'hourly'} buckets.`;

  const dataRows = buckets.map(bucket => `<div class="crowdsec-chart-data-row" role="row"><span role="cell">${crowdsecChartEscape(crowdsecBucketDescription(bucket, stats))}</span><span role="cell">${crowdsecFormatNumber(bucket.detections)}</span><span role="cell">${crowdsecFormatNumber(bucket.withDecision)}</span><span role="cell">${crowdsecFormatNumber(bucket.events)}</span></div>`).join('');

  const signature = JSON.stringify([width, height, stats && stats.bucket_seconds, period, buckets]);
  if (container.dataset.renderSignature === signature) return;
  const currentDetails = container.querySelector('.crowdsec-chart-data');
  const detailsOpen = !!(currentDetails && currentDetails.open);
  const summaryFocused = !!(currentDetails && currentDetails.querySelector('summary') === document.activeElement);

  container.innerHTML = `<svg width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="crowdsecTimelineSvgTitle crowdsecTimelineSvgDesc" preserveAspectRatio="xMidYMid meet">` +
    `<title id="crowdsecTimelineSvgTitle">CrowdSec detection activity</title><desc id="crowdsecTimelineSvgDesc">${crowdsecChartEscape(description)}</desc>` +
    grid + bars + `<polyline points="${eventPoints}" class="crowdsec-events-line" vector-effect="non-scaling-stroke" />${eventAxis}</svg>` +
    `<details class="crowdsec-chart-data"><summary>View exact values</summary><div role="table" aria-label="CrowdSec activity by time bucket"><div class="crowdsec-chart-data-row crowdsec-chart-data-head" role="row"><span role="columnheader">Period</span><span role="columnheader">Detections</span><span role="columnheader">Decision attached</span><span role="columnheader">Events</span></div>${dataRows}</div></details>`;
  container.dataset.renderSignature = signature;
  const nextDetails = container.querySelector('.crowdsec-chart-data');
  if (nextDetails) {
    nextDetails.open = detailsOpen;
    if (summaryFocused) nextDetails.querySelector('summary').focus({ preventScroll: true });
  }
}

function renderCrowdsecOutcomeChart(stats) {
  const container = $('#crowdsecOutcomeChart');
  if (!container) return;
  const total = crowdsecChartNumber(stats && stats.alerts_24h);
  const actioned = Math.min(total, crowdsecChartNumber(stats && stats.alerts_with_decision_24h));
  const observed = Math.max(0, total - actioned);
  const rate = total > 0 ? (actioned / total) * 100 : 0;
  const circumference = 2 * Math.PI * 46;
  const actionLength = (rate / 100) * circumference;
  const accessible = `${crowdsecFormatNumber(actioned)} of ${crowdsecFormatNumber(total)} observed detections had a decision attached, ${rate.toFixed(1)} percent.`;

  container.innerHTML = `<div class="crowdsec-ring-wrap"><svg class="crowdsec-ring" viewBox="0 0 120 120" role="img" aria-label="${crowdsecChartEscape(accessible)}">` +
    '<circle cx="60" cy="60" r="46" class="crowdsec-ring-track" />' +
    `<circle cx="60" cy="60" r="46" class="crowdsec-ring-value${actioned ? '' : ' is-empty'}" stroke-dasharray="${actionLength} ${Math.max(0, circumference - actionLength)}" />` +
    `<text x="60" y="57" text-anchor="middle" class="crowdsec-ring-number">${rate.toFixed(total ? 1 : 0)}%</text><text x="60" y="74" text-anchor="middle" class="crowdsec-ring-label">attached</text></svg></div>` +
    `<div class="crowdsec-outcome-list"><div><span class="crowdsec-outcome-swatch is-actioned" aria-hidden="true"></span><span>Decision attached</span><strong>${crowdsecFormatNumber(actioned)}</strong></div><div><span class="crowdsec-outcome-swatch is-observed" aria-hidden="true"></span><span>Detection only</span><strong>${crowdsecFormatNumber(observed)}</strong></div><div class="crowdsec-outcome-total"><span>Total observed</span><strong>${crowdsecFormatNumber(total)}</strong></div></div>`;
}
