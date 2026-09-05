const { loadSource } = require('./test-helpers');

beforeAll(() => loadSource('utils.js', 'core.js', 'services.js', 'logs-tab.js'));
afterEach(() => { document.body.innerHTML = ''; });

test('custom icon URLs and service types cannot create HTML attributes', () => {
  const url = 'https://example.com/icon.png" onerror="alert(1)';
  const type = 'custom" onload="alert(2)';
  document.body.innerHTML = getServiceIconHtml({ icon_url: url, service_type: type });
  const img = document.querySelector('img');
  expect(img.getAttribute('src')).toBe(url);
  expect(img.getAttribute('alt')).toBe(type);
  expect(img.hasAttribute('onerror')).toBe(false);
  expect(img.hasAttribute('onload')).toBe(false);
});

test('uptime keys and protocol labels cannot inject markup', () => {
  document.body.innerHTML = '<div id="uptime-bars-container"></div>';
  const key = 'svc" onmouseover="alert(1)';
  renderDynamicUptimeBars([{ key, name: 'Example', check_type: '<img src=x onerror=alert(1)>' }]);
  expect(document.querySelector('[onmouseover]')).toBeNull();
  expect(document.querySelector('img')).toBeNull();
  expect(document.getElementById('uptime-' + key)).not.toBeNull();
});

test('log markup is escaped and detail JSON preserves literal entities', () => {
  const message = 'Literal &quot; and &#39; plus "quoted" text';
  document.body.innerHTML = renderLogEntry({
    timestamp: '2026-09-05T12:00:00Z', level: 'info" onclick="alert(1)',
    category: '<img src=x onerror=alert(1)>', message
  });
  expect(document.querySelector('[onclick], img')).toBeNull();
  const entry = document.querySelector('.log-entry');
  expect(JSON.parse(entry.dataset.log).message).toBe(message);
  expect(JSON.parse(entry.dataset.log).level).toBe('info');
});
