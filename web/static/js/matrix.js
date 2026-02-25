let currentView = 'cards';       // 'cards' | 'matrix'
let latestLiveStatus = null;     // cache last /api/check result for matrix
let matrixAnimFrame = null;      // requestAnimationFrame id
let matrixTooltipEl = null;      // shared tooltip element

function initViewToggle() {
  const btnCards  = $('#viewCards');
  const btnMatrix = $('#viewMatrix');
  if (!btnCards || !btnMatrix) return;

  btnCards.addEventListener('click',  () => switchView('cards'));
  btnMatrix.addEventListener('click', () => switchView('matrix'));
}

/* ── Global Health Dot ─────────────────────────────────── */
function updateHealthDot(statusMap) {
  const dot = $('#healthDot');
  if (!dot) return;

  dot.classList.remove('all-up', 'some-down', 'some-degraded');

  let hasDown = false, hasDegraded = false, hasUp = false;
  Object.values(statusMap).forEach(s => {
    if (s.disabled) return;
    if (!s.ok) hasDown = true;
    else if (s.degraded) hasDegraded = true;
    else hasUp = true;
  });

  if (hasDown)           dot.classList.add('some-down');
  else if (hasDegraded)  dot.classList.add('some-degraded');
  else                   dot.classList.add('all-up');
}

/* ── Status Summary Bar ────────────────────────────────── */
function updateStatusSummary(statusMap) {
  const bar = $('#statusSummary');
  if (!bar) return;

  let up = 0, down = 0, degraded = 0, disabled = 0;
  Object.values(statusMap).forEach(s => {
    if (s.disabled)      disabled++;
    else if (!s.ok)      down++;
    else if (s.degraded) degraded++;
    else                 up++;
  });

  const parts = [];
  if (up > 0)       parts.push('<span class="status-summary-item"><span class="status-summary-dot up"></span><span class="status-summary-count">' + up + '</span> Operational</span>');
  if (down > 0)     parts.push('<span class="status-summary-item"><span class="status-summary-dot down"></span><span class="status-summary-count">' + down + '</span> Down</span>');
  if (degraded > 0) parts.push('<span class="status-summary-item"><span class="status-summary-dot degraded"></span><span class="status-summary-count">' + degraded + '</span> Degraded</span>');
  if (disabled > 0) parts.push('<span class="status-summary-item"><span class="status-summary-dot disabled"></span><span class="status-summary-count">' + disabled + '</span> Disabled</span>');

  bar.innerHTML = parts.join('');
}

function switchView(view) {
  currentView = view;
  const cards  = $('#services-container');
  const matrix = $('#matrix-container');
  const btnC   = $('#viewCards');
  const btnM   = $('#viewMatrix');
  const mainEl = document.querySelector('main');

  if (view === 'matrix') {
    cards  && cards.classList.add('hidden');
    matrix && matrix.classList.remove('hidden');
    btnC   && btnC.classList.remove('active');
    btnM   && btnM.classList.add('active');
    mainEl && mainEl.classList.add('matrix-active');
    renderMatrix();
  } else {
    matrix && matrix.classList.add('hidden');
    cards  && cards.classList.remove('hidden');
    btnM   && btnM.classList.remove('active');
    btnC   && btnC.classList.add('active');
    mainEl && mainEl.classList.remove('matrix-active');
    stopMatrixAnimation();
  }
}

/* ── Matrix status helpers ──────────────────────────────── */
function matrixStatusOf(svc) {
  let statusClass = 'unknown', statusLabel = 'Unknown', ms = null;
  if (latestLiveStatus && latestLiveStatus[svc.key]) {
    const s = latestLiveStatus[svc.key];
    if (s.disabled)       { statusClass = 'disabled'; statusLabel = 'Disabled'; }
    else if (!s.ok)       { statusClass = 'down';     statusLabel = 'Down';     }
    else if (s.degraded)  { statusClass = 'degraded'; statusLabel = 'Degraded'; }
    else                  { statusClass = 'up';       statusLabel = 'Operational'; }
    if (s.ms != null) ms = s.ms;
  }
  return { statusClass, statusLabel, ms };
}

const MATRIX_COLORS = {
  up:       { r: 34,  g: 197, b: 94  },
  down:     { r: 248, g: 113, b: 113 },
  degraded: { r: 251, g: 191, b: 36  },
  disabled: { r: 100, g: 116, b: 139 },
  unknown:  { r: 100, g: 116, b: 139 },
  hub:      { r: 99,  g: 102, b: 241 }
};

/* ── Canvas line animation engine ───────────────────────── */
function stopMatrixAnimation() {
  if (matrixAnimFrame) { cancelAnimationFrame(matrixAnimFrame); matrixAnimFrame = null; }
}

function animateMatrixLines(canvas, nodePositions) {
  stopMatrixAnimation();
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const dpr = window.devicePixelRatio || 1;

  function frame(t) {
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    canvas.width  = w * dpr;
    canvas.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const cx = w / 2;
    const cy = h / 2;

    // 1. Draw Hub to Node lines
    nodePositions.forEach(n => {
      const col = MATRIX_COLORS[n.status] || MATRIX_COLORS.unknown;
      const isDisabled = n.status === 'disabled';
      const isDown     = n.status === 'down';
      const isDegraded = n.status === 'degraded';

      const grad = ctx.createLinearGradient(n.x, n.y, cx, cy);

      if (isDisabled) {
        grad.addColorStop(0, `rgba(${col.r},${col.g},${col.b},0.1)`);
        grad.addColorStop(1, `rgba(${MATRIX_COLORS.hub.r},${MATRIX_COLORS.hub.g},${MATRIX_COLORS.hub.b},0.02)`);
        ctx.beginPath();
        ctx.moveTo(n.x, n.y);
        ctx.lineTo(cx, cy);
        ctx.strokeStyle = grad;
        ctx.lineWidth = 1;
        ctx.setLineDash([2, 4]);
        ctx.stroke();
        ctx.setLineDash([]);
        return;
      }

      if (isDown) {
        grad.addColorStop(0, `rgba(${col.r},${col.g},${col.b},0.4)`);
        grad.addColorStop(1, `rgba(${col.r},${col.g},${col.b},0.05)`);
        ctx.beginPath();
        ctx.moveTo(n.x, n.y);
        ctx.lineTo(cx, cy);
        ctx.strokeStyle = grad;
        ctx.lineWidth = 1.5;
        ctx.setLineDash([4, 8]);
        ctx.stroke();
        ctx.setLineDash([]);
        
        // Draw a small "X" or break in the middle of the line
        const mx = (n.x + cx) / 2;
        const my = (n.y + cy) / 2;
        ctx.save();
        const sz = 4;
        ctx.strokeStyle = `rgba(${col.r},${col.g},${col.b},0.8)`;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(mx - sz, my - sz);
        ctx.lineTo(mx + sz, my + sz);
        ctx.moveTo(mx + sz, my - sz);
        ctx.lineTo(mx - sz, my + sz);
        ctx.stroke();
        ctx.restore();
        return;
      }

      if (isDegraded) {
        grad.addColorStop(0, `rgba(${col.r},${col.g},${col.b},0.4)`);
        grad.addColorStop(1, `rgba(${MATRIX_COLORS.hub.r},${MATRIX_COLORS.hub.g},${MATRIX_COLORS.hub.b},0.05)`);
        ctx.beginPath();
        ctx.moveTo(n.x, n.y);
        ctx.lineTo(cx, cy);
        ctx.strokeStyle = grad;
        ctx.lineWidth = 1.5;
        ctx.setLineDash([6, 4]); // Dashed but less broken than down
        ctx.stroke();
        ctx.setLineDash([]);
        // No particles for degraded
        return;
      }

      grad.addColorStop(0, `rgba(${col.r},${col.g},${col.b},0.3)`);
      grad.addColorStop(1, `rgba(${MATRIX_COLORS.hub.r},${MATRIX_COLORS.hub.g},${MATRIX_COLORS.hub.b},0.05)`);

      ctx.beginPath();
      ctx.moveTo(n.x, n.y);
      ctx.lineTo(cx, cy);
      ctx.strokeStyle = grad;
      ctx.lineWidth = 1.5;
      ctx.stroke();

      // Particle
      const speed = 3000;
      const prog = ((t + n.phase) % speed) / speed;
      
      const px = n.x + (cx - n.x) * prog;
      const py = n.y + (cy - n.y) * prog;
      
      const alpha = Math.sin(prog * Math.PI); // fade in and out
      
      ctx.beginPath();
      ctx.arc(px, py, 1.5, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(${col.r},${col.g},${col.b},${alpha})`;
      ctx.fill();
      
      ctx.beginPath();
      ctx.arc(px, py, 4, 0, Math.PI * 2);
      const glow = ctx.createRadialGradient(px, py, 0, px, py, 4);
      glow.addColorStop(0, `rgba(${col.r},${col.g},${col.b},${alpha * 0.5})`);
      glow.addColorStop(1, `rgba(${col.r},${col.g},${col.b},0)`);
      ctx.fillStyle = glow;
      ctx.fill();
    });

    const nodeByKey = {};
    nodePositions.forEach(n => { nodeByKey[n.key] = n; });
    const drawnConnPairs = new Set();

    // Helper to draw curved links
    function drawLink(n, target, defaultColorStr, isDashed, phaseOffset, t) {
      const dx = target.x - n.x;
      const dy = target.y - n.y;
      const dist = Math.sqrt(dx*dx + dy*dy);
      const midX = (n.x + target.x) / 2;
      const midY = (n.y + target.y) / 2;
      
      // Normal vector
      const nx = -dy / dist;
      const ny = dx / dist;
      
      // Curve control point
      const curveAmount = dist * 0.25;
      const ctrlX = midX + nx * curveAmount;
      const ctrlY = midY + ny * curveAmount;

      // Determine link state based on node statuses
      let linkColorStr = defaultColorStr;
      let linkDashed = isDashed;
      let showParticles = true;
      let linkOpacity = 0.4;

      if (n.status === 'disabled' || target.status === 'disabled') {
        linkColorStr = `${MATRIX_COLORS.disabled.r},${MATRIX_COLORS.disabled.g},${MATRIX_COLORS.disabled.b}`;
        linkDashed = true;
        showParticles = false;
        linkOpacity = 0.15;
      } else if (n.status === 'down' || target.status === 'down') {
        linkColorStr = `${MATRIX_COLORS.down.r},${MATRIX_COLORS.down.g},${MATRIX_COLORS.down.b}`;
        linkDashed = true;
        showParticles = false;
        linkOpacity = 0.3;
      } else if (n.status === 'degraded' || target.status === 'degraded') {
        linkColorStr = `${MATRIX_COLORS.degraded.r},${MATRIX_COLORS.degraded.g},${MATRIX_COLORS.degraded.b}`;
        linkDashed = true;
        showParticles = false;
        linkOpacity = 0.3;
      }

      ctx.beginPath();
      ctx.moveTo(n.x, n.y);
      ctx.quadraticCurveTo(ctrlX, ctrlY, target.x, target.y);
      
      const grad = ctx.createLinearGradient(n.x, n.y, target.x, target.y);
      grad.addColorStop(0, `rgba(${linkColorStr}, 0.05)`);
      grad.addColorStop(0.5, `rgba(${linkColorStr}, ${linkOpacity})`);
      grad.addColorStop(1, `rgba(${linkColorStr}, 0.05)`);
      
      ctx.strokeStyle = grad;
      ctx.lineWidth = 1.5;
      
      if (linkDashed) {
        if (n.status === 'down' || target.status === 'down') {
          ctx.setLineDash([4, 8]); // More broken for down
        } else {
          ctx.setLineDash([4, 4]);
        }
      } else {
        ctx.setLineDash([]);
      }
      
      ctx.stroke();
      ctx.setLineDash([]);

      if (!showParticles) return;

      // Particle
      const speed = 4000;
      const prog = ((t + n.phase + phaseOffset) % speed) / speed;
      const bp = 1 - prog;
      const px = bp * bp * n.x + 2 * bp * prog * ctrlX + prog * prog * target.x;
      const py = bp * bp * n.y + 2 * bp * prog * ctrlY + prog * prog * target.y;
      
      const alpha = Math.sin(prog * Math.PI);
      
      ctx.beginPath();
      ctx.arc(px, py, 1.5, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(${linkColorStr}, ${alpha})`;
      ctx.fill();
      
      ctx.beginPath();
      ctx.arc(px, py, 5, 0, Math.PI * 2);
      const glow = ctx.createRadialGradient(px, py, 0, px, py, 5);
      glow.addColorStop(0, `rgba(${linkColorStr}, ${alpha * 0.4})`);
      glow.addColorStop(1, `rgba(${linkColorStr}, 0)`);
      ctx.fillStyle = glow;
      ctx.fill();
    }

    // 2. Dependencies
    nodePositions.forEach(n => {
      if (!n.depends_on) return;
      n.depends_on.forEach(depKey => {
        const dep = nodeByKey[depKey];
        if (!dep) return;
        drawLink(n, dep, "251,146,60", true, 300, t); // Orange
      });
    });

    // 3. Connections
    nodePositions.forEach(n => {
      if (!n.connected_to) return;
      n.connected_to.forEach(connKey => {
        const peer = nodeByKey[connKey];
        if (!peer) return;
        const pairKey = [n.key, connKey].sort().join('|');
        if (drawnConnPairs.has(pairKey)) return;
        drawnConnPairs.add(pairKey);
        
        drawLink(n, peer, "52,211,153", false, 150, t); // Emerald
      });
    });

    matrixAnimFrame = requestAnimationFrame(frame);
  }

  matrixAnimFrame = requestAnimationFrame(frame);
}

/* ── Render the full matrix view ────────────────────────── */
function renderMatrix() {
  const container = $('#matrix-container');
  if (!container) return;

  if (!servicesData || servicesData.length === 0) {
    container.innerHTML = '<div style="color:#9ca3af;text-align:center;padding:48px;">No services configured</div>';
    container.style.height = '';
    stopMatrixAnimation();
    return;
  }

  container.innerHTML = '';

  // ── Dynamic sizing based on service count ──
  const count    = servicesData.length;
  const RING_D   = 44;   // node ring diameter (px)
  const NODE_PAD = 60;   // extra clearance around each node for label
  const HUB_PAD  = 100;  // minimum space from hub to ring
  
  // Ideal orbital radius grows with count so nodes don't overlap
  const idealRadius = Math.max(160, HUB_PAD + (count * (RING_D + NODE_PAD)) / (2 * Math.PI));
  // Container must fit the full orbit + node overflow + padding
  const containerH = Math.max(400, Math.ceil((idealRadius + RING_D + NODE_PAD) * 2 + 60));
  container.style.height = containerH + 'px';

  // Canvas for animated lines
  const canvas = document.createElement('canvas');
  canvas.className = 'matrix-canvas';
  container.appendChild(canvas);

  // Centre hub
  const hub = document.createElement('div');
  hub.className = 'matrix-hub';
  hub.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>';
  container.appendChild(hub);

  // Nodes layer
  const nodesLayer = document.createElement('div');
  nodesLayer.className = 'matrix-nodes';
  container.appendChild(nodesLayer);

  // Legend / Key
  const legend = document.createElement('div');
  legend.className = 'matrix-legend';
  legend.innerHTML =
    '<div class="matrix-legend-item">' +
      '<span class="matrix-legend-line ml-status"></span>' +
      '<span>Status Link</span>' +
    '</div>' +
    '<div class="matrix-legend-item">' +
      '<span class="matrix-legend-line ml-depends"></span>' +
      '<span>Depends On</span>' +
    '</div>' +
    '<div class="matrix-legend-item">' +
      '<span class="matrix-legend-line ml-connected"></span>' +
      '<span>Connected To</span>' +
    '</div>';
  container.appendChild(legend);

  // Tooltip
  if (!matrixTooltipEl) {
    matrixTooltipEl = document.createElement('div');
    matrixTooltipEl.className = 'matrix-tooltip';
    document.body.appendChild(matrixTooltipEl);
  }

  // Lay nodes out after container is visible and sized
  requestAnimationFrame(() => {
    const rect = container.getBoundingClientRect();
    const W = rect.width;
    const H = rect.height;
    const cx = W / 2;
    const cy = H / 2;

    // Position hub
    hub.style.left = cx + 'px';
    hub.style.top  = cy + 'px';

    // Calculate orbital radii — elliptical, capped to available space
    // Alternate radius slightly to prevent overlap when crowded
    const baseRx = Math.min(cx - RING_D - 30, idealRadius);
    const baseRy = Math.min(cy - RING_D - 30, idealRadius);
    const ringHalf = RING_D / 2;
    const nodePositions = [];

    servicesData.forEach((svc, i) => {
      const angle = (2 * Math.PI * i / count) - Math.PI / 2;
      
      // Zig-zag radius if there are many nodes to prevent cramping
      const rOffset = (count > 8 && i % 2 !== 0) ? 30 : 0;
      const rx = Math.max(HUB_PAD, baseRx - rOffset);
      const ry = Math.max(HUB_PAD, baseRy - rOffset);

      // Ring centre coordinates — this is where lines will connect
      const ringCX = cx + rx * Math.cos(angle);
      const ringCY = cy + ry * Math.sin(angle);
      const { statusClass, statusLabel, ms } = matrixStatusOf(svc);

      // Build icon HTML
      let iconHtml = '';
      if (svc.icon_url && /^(https?:\/\/|data:image\/|\/static\/)/.test(svc.icon_url)) {
        iconHtml = '<img src="' + escapeHtml(svc.icon_url) + '" class="matrix-node-icon" alt="">';
      } else {
        const raw = getServiceIconHtml(svc);
        if (raw.includes('<img')) {
          iconHtml = raw.replace(/class="icon[^"]*"/g, 'class="matrix-node-icon"');
        } else {
          iconHtml = '<span class="matrix-node-icon-placeholder">' + raw.replace(/<\/?span[^>]*>/g, '') + '</span>';
        }
      }

      const name = escapeHtml(svc.display_name || svc.name || svc.key || '');
      const msText = ms != null ? ms + 'ms' : '';

      const node = document.createElement('div');
      node.className = 'matrix-node';
      // Position so the ring centre sits at (ringCX, ringCY).
      // CSS uses translateX(-50%) only, so left centres horizontally.
      // top = ringCY - ringHalf puts the top of the ring at the right
      // spot so its centre is exactly ringCY.
      node.style.left = ringCX + 'px';
      node.style.top  = (ringCY - ringHalf) + 'px';
      node.innerHTML =
        '<div class="matrix-node-ring ' + statusClass + '">' + iconHtml + '</div>' +
        '<span class="matrix-node-label">' + name + '</span>' +
        (msText ? '<span class="matrix-node-ms">' + msText + '</span>' : '');

      // Tooltip on hover
      const depNames = (svc.depends_on || '').split(',').map(d => d.trim()).filter(Boolean)
        .map(dk => { const s = servicesData.find(x => x.key === dk); return s ? (s.name || dk) : dk; });
      const connNames = (svc.connected_to || '').split(',').map(c => c.trim()).filter(Boolean)
        .map(ck => { const s = servicesData.find(x => x.key === ck); return s ? (s.name || ck) : ck; });
      let tipText = name + ' \u2014 ' + statusLabel + (msText ? ' (' + msText + ')' : '');
      if (depNames.length > 0) tipText += ' | Depends on: ' + depNames.join(', ');
      if (connNames.length > 0) tipText += ' | Connected to: ' + connNames.join(', ');
      node.addEventListener('mouseenter', function(e) {
        matrixTooltipEl.textContent = tipText;
        matrixTooltipEl.classList.add('visible');
      });
      node.addEventListener('mousemove', function(e) {
        matrixTooltipEl.style.left = e.clientX + 12 + 'px';
        matrixTooltipEl.style.top  = e.clientY - 30 + 'px';
      });
      node.addEventListener('mouseleave', function() {
        matrixTooltipEl.classList.remove('visible');
      });

      nodesLayer.appendChild(node);

      // Canvas lines target the ring centre, not the DOM node centre
      const depKeys = (svc.depends_on || '').split(',').map(d => d.trim()).filter(Boolean);
      const connKeys = (svc.connected_to || '').split(',').map(c => c.trim()).filter(Boolean);
      nodePositions.push({
        x: ringCX,
        y: ringCY,
        key: svc.key,
        depends_on: depKeys,
        connected_to: connKeys,
        status: statusClass,
        phase: i * 600  // stagger pulse per node
      });
    });

    // Start canvas animation
    animateMatrixLines(canvas, nodePositions);
  });
}

// Detect the actual protocol from URL and check_type