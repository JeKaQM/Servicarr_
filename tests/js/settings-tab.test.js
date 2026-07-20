const { loadSource } = require('./test-helpers');

beforeAll(() => {
  loadSource('core.js', 'utils.js', 'settings-tab.js');
});

beforeEach(() => {
  document.body.innerHTML = `
    <span id="softwareVersionBadge"></span>
    <span id="softwareVersion"></span>
    <span id="softwareCommit"></span>
    <span id="softwareBuildTime"></span>
    <span id="softwareStartedAt"></span>
    <span id="softwareDatabase"></span>
    <span id="softwareRuntime"></span>
    <span id="softwareInfoStatus"></span>
    <div id="deploymentHistory"></div>
  `;
  global.j = jest.fn();
});

describe('software version settings', () => {
  test('renders current build, database, and deployment history', () => {
    renderSystemInfo({
      version: '1.2.3',
      commit: 'abcdef1234567890',
      build_time: '2026-07-20T12:00:00Z',
      started_at: '2026-07-20T13:00:00Z',
      go_version: 'go1.25.3',
      database: {
        engine: 'SQLite',
        engine_version: '3.50.4',
        schema_version: 1
      },
      deployments: [{
        version: '1.2.3',
        commit: 'abcdef1234567890',
        last_started_at: '2026-07-20T13:00:00Z',
        startup_count: 2
      }]
    });

    expect(document.getElementById('softwareVersion').textContent).toBe('v1.2.3');
    expect(document.getElementById('softwareCommit').textContent).toBe('abcdef123456');
    expect(document.getElementById('softwareDatabase').textContent).toBe('SQLite 3.50.4 / schema 1');
    expect(document.getElementById('softwareRuntime').textContent).toBe('go1.25.3');
    expect(document.querySelectorAll('.deployment-history-row')).toHaveLength(1);
    expect(document.querySelector('.deployment-version').textContent).toBe('v1.2.3');
    expect(document.querySelector('.deployment-count').textContent).toBe('2 starts');
  });

  test('renders deployment values as text rather than HTML', () => {
    renderDeploymentHistory([{
      version: '<img src=x onerror=alert(1)>',
      commit: '<script>alert(1)</script>',
      last_started_at: '',
      startup_count: 1
    }]);

    expect(document.querySelector('#deploymentHistory img')).toBeNull();
    expect(document.querySelector('#deploymentHistory script')).toBeNull();
    expect(document.querySelector('.deployment-version').textContent).toContain('<img');
    expect(document.querySelector('.deployment-commit').textContent).toContain('<script>');
  });

  test('loads system information from the authenticated endpoint', async () => {
    global.j.mockResolvedValue({
      version: '1.0.0',
      commit: 'local',
      database: { engine: 'SQLite', schema_version: 1 },
      deployments: []
    });

    await loadSystemInfo();

    expect(global.j).toHaveBeenCalledWith('/api/admin/settings/system-info');
    expect(document.getElementById('softwareVersionBadge').textContent).toBe('v1.0.0');
    expect(document.querySelector('.deployment-history-empty').textContent).toBe('No deployment history recorded.');
  });
});
