// Screenshot driver for the run-servicarr skill.
// Usage: SVC_USER=... SVC_PASS=... node shoot.js <outDir> [--admin]
// Env: BASE_URL (default http://127.0.0.1:4555), CHROME (path to Chrome/Edge).
const fs = require('fs');
const path = require('path');
const puppeteer = require('puppeteer-core');

const BASE = process.env.BASE_URL || 'http://127.0.0.1:4555';
const CHROME = process.env.CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const outDir = process.argv[2];
const wantAdmin = process.argv.includes('--admin');
if (!outDir) {
  console.error('usage: node shoot.js <outDir> [--admin]');
  process.exit(2);
}
fs.mkdirSync(outDir, { recursive: true });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function watch(page, label, problems) {
  page.on('console', (m) => { if (m.type() === 'error') problems.push(`[${label}] console: ${m.text()}`); });
  page.on('pageerror', (e) => problems.push(`[${label}] pageerror: ${e.message}`));
  page.on('response', (r) => { if (r.status() >= 400) problems.push(`[${label}] HTTP ${r.status()} ${r.request().method()} ${r.url()}`); });
  page.on('requestfailed', (r) => problems.push(`[${label}] failed: ${r.url()} ${r.failure() && r.failure().errorText}`));
}

(async () => {
  const browser = await puppeteer.launch({ executablePath: CHROME, headless: true, args: ['--hide-scrollbars'] });
  const report = { shots: [], problems: [] };
  try {
    const viewports = [
      { name: 'desktop', width: 1440, height: 900, deviceScaleFactor: 1 },
      { name: 'mobile', width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true },
    ];
    for (const vp of viewports) {
      const page = await browser.newPage();
      watch(page, vp.name, report.problems);
      await page.setViewport(vp);
      await page.goto(BASE + '/', { waitUntil: 'networkidle2', timeout: 60000 });
      await sleep(2500);
      const file = path.join(outDir, `public-${vp.name}.png`);
      await page.screenshot({ path: file, fullPage: true });
      report.shots.push(file);
      await page.close();
    }

    if (wantAdmin) {
      if (!process.env.SVC_USER || !process.env.SVC_PASS) {
        throw new Error('--admin needs SVC_USER and SVC_PASS (ask the user for the test account)');
      }
      const page = await browser.newPage();
      watch(page, 'admin', report.problems);
      await page.setViewport({ width: 1440, height: 900 });
      await page.goto(BASE + '/', { waitUntil: 'networkidle2', timeout: 60000 });
      // One login through the real API; the page load sets the CSRF cookie.
      report.login_status = await page.evaluate(async (u, p) => {
        const csrf = (document.cookie.match(/(?:^|; )csrf=([^;]+)/) || [])[1] || '';
        const res = await fetch('/api/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': decodeURIComponent(csrf) },
          body: JSON.stringify({ username: u, password: p }),
        });
        return res.status;
      }, process.env.SVC_USER, process.env.SVC_PASS);
      if (report.login_status !== 200) {
        throw new Error(`login failed with HTTP ${report.login_status}; stop to avoid an IP block`);
      }
      await page.goto(BASE + '/', { waitUntil: 'networkidle2', timeout: 60000 });
      await sleep(2000);
      const tabs = await page.$$eval('[data-tab]', (els) => [...new Set(els.map((e) => e.getAttribute('data-tab')).filter(Boolean))]);
      report.tabs = tabs;
      for (const tab of tabs) {
        await page.evaluate((t) => document.querySelector(`[data-tab="${t}"]`).click(), tab);
        await sleep(2500);
        const file = path.join(outDir, `admin-${tab}.png`);
        await page.screenshot({ path: file, fullPage: true });
        report.shots.push(file);
      }
      await page.close();
    }
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify(report, null, 2));
  process.exit(report.problems.length ? 1 : 0);
})().catch((e) => { console.error(e.message || e); process.exit(1); });
