/**
 * Tests for the service editor's Links section (admin-ui.js pickers and the
 * parts of service-mgmt.js that open and save them), run against the real
 * service modal markup.
 */
const fs = require('fs');
const path = require('path');
const { loadSource } = require('./test-helpers');

const MODALS = fs.readFileSync(path.resolve(__dirname, '..', '..', 'web', 'templates', 'partials', '_modals.html'), 'utf-8');
const SERVICE_MODAL = MODALS.slice(MODALS.indexOf('<dialog id="serviceModal">'), MODALS.indexOf('</dialog>', MODALS.indexOf('<dialog id="serviceModal">')) + '</dialog>'.length);

const ADMIN = [
  { id: 1, key: 'router', name: 'Router', url: 'http://router.lan', visible: true },
  { id: 2, key: 'nas', name: 'NAS', url: 'http://nas.lan', depends_on: 'router', visible: true },
  { id: 3, key: 'proxmox', name: 'Proxmox', url: 'http://proxmox.lan', depends_on: 'router', visible: true },
  { id: 4, key: 'plex', name: 'Plex', url: 'http://plex.lan', depends_on: 'nas,proxmox', visible: true },
  { id: 5, key: 'sonarr', name: 'Sonarr', url: 'http://sonarr.lan', depends_on: 'nas', connected_to: 'plex', visible: true },
  { id: 6, key: 'overseerr', name: 'Overseerr', url: 'http://overseerr.lan', depends_on: 'plex', connected_to: 'sonarr', visible: true },
  { id: 7, key: 'backup', name: 'Backup', url: 'http://backup.lan', depends_on: 'nas', visible: false },
];

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'services.js', 'matrix.js', 'admin-ui.js', 'service-mgmt.js');
  // jsdom has no modal dialogs.
  HTMLDialogElement.prototype.showModal = function () { this.open = true; };
  HTMLDialogElement.prototype.close = function () { this.open = false; };
});

beforeEach(() => {
  document.body.innerHTML = SERVICE_MODAL;
  globalThis.adminServicesData = ADMIN;
  globalThis.servicesData = ADMIN;
  globalThis.latestLiveStatus = { router: { ok: true, ms: 2 }, nas: { ok: false } };
  globalThis.linkPickersReady = false;
  initLinkPickers();
});

const rows = listId => [...document.querySelectorAll(`#${listId} .link-option`)].map(row => ({
  key: row.querySelector('input').value,
  checked: row.querySelector('input').checked,
  disabled: row.querySelector('input').disabled,
  note: row.querySelector('.link-option-note')?.textContent || '',
  warn: !!row.querySelector('.link-option-note.is-warning'),
  status: row.querySelector('.link-option-status')?.textContent || '',
}));
const row = (listId, key) => rows(listId).find(r => r.key === key);

/* ── Depends on ─────────────────────────────────────────── */
describe('Depends on picker', () => {
  test('lists linked services first and blocks ones that would close a loop', () => {
    populateDependsOnDropdown('nas', ['router']);
    expect(rows('serviceDependsOnList').map(r => r.key)).toEqual(['router', 'proxmox', 'plex', 'sonarr', 'overseerr', 'backup']);
    expect(row('serviceDependsOnList', 'router')).toMatchObject({ checked: true, disabled: false });
    expect(row('serviceDependsOnList', 'proxmox')).toMatchObject({ checked: false, disabled: false, note: '' });
    expect(row('serviceDependsOnList', 'plex')).toMatchObject({ disabled: true, note: 'Depends on NAS' });
    expect(row('serviceDependsOnList', 'overseerr')).toMatchObject({ disabled: true, note: 'Depends on NAS through Plex' });
  });

  test('says which services need this one', () => {
    populateDependsOnDropdown('nas', ['router']);
    const note = document.getElementById('neededByNote');
    expect(note.hidden).toBe(false);
    expect(note.textContent).toBe('Needed by Plex, Sonarr, Backup (set on those services).');
    expect(document.getElementById('dependsOnCount').textContent).toBe('1 linked');
  });

  test('shows status with a word, and marks hidden services', () => {
    populateDependsOnDropdown('plex', ['nas', 'proxmox']);
    expect(row('serviceDependsOnList', 'router').status).toBe('Operational');
    expect(row('serviceDependsOnList', 'nas').status).toBe('Down');
    expect(row('serviceDependsOnList', 'backup').status).toBe('Hidden');
    expect(row('serviceDependsOnList', 'proxmox').status).toBe('');
  });

  test('a loop already in stored data stays editable with a warning', () => {
    globalThis.adminServicesData = [
      { key: 'a', name: 'A', depends_on: 'b' },
      { key: 'b', name: 'B', depends_on: 'a' },
    ];
    populateDependsOnDropdown('a', ['b']);
    expect(row('serviceDependsOnList', 'b')).toMatchObject({
      checked: true, disabled: false, warn: true, note: 'Loop: depends on A. Uncheck to save.',
    });
  });

  test('a new service can depend on anything', () => {
    populateDependsOnDropdown(undefined, []);
    expect(rows('serviceDependsOnList')).toHaveLength(7);
    expect(rows('serviceDependsOnList').some(r => r.disabled)).toBe(false);
    expect(document.getElementById('neededByNote').hidden).toBe(true);
    expect(document.getElementById('dependsOnCount').textContent).toBe('None');
  });

  test('falls back to the dashboard list before the admin list loads', () => {
    globalThis.adminServicesData = null;
    globalThis.servicesData = [{ key: 'a', name: 'A' }, { key: 'b', name: 'B' }];
    populateDependsOnDropdown('a', []);
    expect(rows('serviceDependsOnList').map(r => r.key)).toEqual(['b']);
  });
});

/* ── Connected to ───────────────────────────────────────── */
describe('Connected to picker', () => {
  test('shows connections set on the other service, read-only', () => {
    populateConnectedToList('sonarr', ['plex']);
    expect(row('serviceConnectedToList', 'plex')).toMatchObject({ checked: true, disabled: false, note: '' });
    expect(row('serviceConnectedToList', 'overseerr')).toMatchObject({ checked: true, disabled: true, note: 'Set on Overseerr' });
    expect(document.getElementById('connectedToCount').textContent).toBe('2 linked');
  });

  test('a connection set on both sides stays editable here', () => {
    globalThis.adminServicesData = ADMIN.map(s => (s.key === 'plex' ? { ...s, connected_to: 'sonarr' } : s));
    populateConnectedToList('sonarr', ['plex']);
    expect(row('serviceConnectedToList', 'plex')).toMatchObject({ checked: true, disabled: false, note: 'Also set on Plex' });
  });
});

/* ── Filtering and counts ───────────────────────────────── */
describe('filtering', () => {
  const type = (input, value) => {
    input.value = value;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  };
  const visible = listId => [...document.querySelectorAll(`#${listId} .link-option`)].filter(r => !r.hidden).map(r => r.querySelector('input').value);

  test('the filter box appears only once the list is long', () => {
    populateDependsOnDropdown('nas', []);
    expect(document.getElementById('dependsOnSearch').hidden).toBe(true);
    populateDependsOnDropdown(undefined, []);
    expect(document.getElementById('dependsOnSearch').hidden).toBe(false);
  });

  test('typing narrows the list and says when nothing matches', () => {
    populateDependsOnDropdown(undefined, []);
    const search = document.getElementById('dependsOnSearch');
    type(search, 'PL');
    expect(visible('serviceDependsOnList')).toEqual(['plex']);
    type(search, 'zzz');
    expect(visible('serviceDependsOnList')).toEqual([]);
    expect(document.querySelector('#serviceDependsOnList .link-picker-empty').textContent).toBe('No services match "zzz"');
    resetLinkFilters();
    expect(search.value).toBe('');
    expect(visible('serviceDependsOnList')).toHaveLength(7);
    expect(document.querySelector('#serviceDependsOnList .link-picker-empty').hidden).toBe(true);
  });

  test('ticking a box updates the count', () => {
    populateDependsOnDropdown('plex', ['nas']);
    const proxmox = document.querySelector('#serviceDependsOnList input[value="proxmox"]');
    proxmox.checked = true;
    proxmox.dispatchEvent(new Event('change', { bubbles: true }));
    expect(document.getElementById('dependsOnCount').textContent).toBe('2 linked');
  });
});

/* ── Opening and saving ─────────────────────────────────── */
describe('editing a service', () => {
  let saved;
  beforeEach(() => {
    saved = null;
    globalThis.j = jest.fn(async (url, opts) => {
      saved = { url, body: JSON.parse(opts.body) };
      return {};
    });
    globalThis.showToast = jest.fn();
    globalThis.loadAllServices = jest.fn();
    globalThis.loadServices = jest.fn().mockResolvedValue([]);
    globalThis.renderDynamicUptimeBars = jest.fn();
    globalThis.refresh = jest.fn();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  test('opens with the stored links ticked', () => {
    openServiceModal(ADMIN.find(s => s.key === 'plex'));
    expect(rows('serviceDependsOnList').filter(r => r.checked).map(r => r.key)).toEqual(['nas', 'proxmox']);
    expect(rows('serviceConnectedToList').filter(r => r.checked).map(r => r.key)).toEqual(['sonarr']);
  });

  test('saves only the links that belong to this service', async () => {
    openServiceModal(ADMIN.find(s => s.key === 'sonarr'));
    await saveService();
    expect(saved.url).toBe('/api/admin/services/5');
    expect(saved.body.depends_on).toBe('nas');
    expect(saved.body.connected_to).toBe('plex');
  });

  test('shows the server\'s plain-text reason when a save is refused', async () => {
    globalThis.j = jest.fn().mockRejectedValue(Object.assign(new Error('HTTP 400'), {
      body: "Plex can't depend on Overseerr: Overseerr depends on Plex\n",
    }));
    openServiceModal(ADMIN.find(s => s.key === 'plex'));
    await saveService();
    const err = document.getElementById('serviceError');
    expect(err.textContent).toBe("Plex can't depend on Overseerr: Overseerr depends on Plex");
    expect(err.classList.contains('hidden')).toBe(false);
  });

  test('still shows JSON error messages', async () => {
    globalThis.j = jest.fn().mockRejectedValue(Object.assign(new Error('HTTP 409'), { body: { error: 'Taken' } }));
    openServiceModal(ADMIN.find(s => s.key === 'plex'));
    await saveService();
    expect(document.getElementById('serviceError').textContent).toBe('Taken');
  });
});
