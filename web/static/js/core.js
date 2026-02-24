const REFRESH_MS = 15000;
let DAYS = 30;
let resourcesConfig = null; // Cache the config
let isAdminUser = false;
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const fmtMs = ms => ms == null ? '—' : ms + ' ms';
const cls = (ok, status, degraded) => {
  if (!ok) return 'pill down'; // Down = red
  if (degraded) return 'pill warn'; // Degraded = amber
  return 'pill ok'; // Up = green
};

// Delegated image error handler — replaces inline onerror attributes (CSP compliance)
document.addEventListener('error', function(e) {
  if (e.target.tagName !== 'IMG') return;
  const img = e.target;
  if (img.classList.contains('service-icon-img')) {
    img.style.display = 'none';
    if (img.nextElementSibling) img.nextElementSibling.style.display = 'flex';
  } else if (img.classList.contains('matrix-node-icon')) {
    img.style.display = 'none';
  } else if (img.classList.contains('icon-preview-img')) {
    img.style.display = 'none';
    if (img.nextElementSibling) img.nextElementSibling.style.display = 'block';
  }
}, true);

function fmtBytes(n) {
  if (n == null || isNaN(n)) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = Number(n);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = i === 0 ? 0 : (i >= 3 ? 2 : 1);
  return `${v.toFixed(digits)} ${units[i]}`;
}

function fmtRateBps(n) {
  if (n == null || isNaN(n)) return '—';
  return `${fmtBytes(n)}/s`;
}

function fmtPct(n) {
  if (n == null || isNaN(n)) return '—';
  return `${Number(n).toFixed(0)}%`;
}

function fmtFloat(n, digits = 2) {
  if (n == null || isNaN(n)) return '—';
  return Number(n).toFixed(digits);
}

function fmtTempC(n) {
  if (n == null || isNaN(n)) return '—';
  return `${Number(n).toFixed(0)}°C`;
}

function setResText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

function setResClass(id, clsName) {
  const el = document.getElementById(id);
  if (el) el.className = clsName;
}