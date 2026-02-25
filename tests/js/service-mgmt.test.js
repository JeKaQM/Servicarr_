/**
 * Tests for service-mgmt.js – getProtocolBadge, maskUrl, generateServiceKey.
 */
const { loadSource } = require('./test-helpers');

beforeAll(() => {
  // service-mgmt.js depends on core.js ($, $$), utils.js (escapeHtml),
  // and services.js (getServiceIconHtml, etc.)
  loadSource('core.js', 'utils.js', 'services.js', 'service-mgmt.js');
});

/* ── getProtocolBadge ───────────────────────────────────── */
describe('getProtocolBadge', () => {
  test('always_up check_type → "DEMO"', () => {
    expect(getProtocolBadge({ check_type: 'always_up', url: '' })).toBe('DEMO');
  });

  test('tcp check_type → "TCP"', () => {
    expect(getProtocolBadge({ check_type: 'tcp', url: '' })).toBe('TCP');
  });

  test('tcp:// url → "TCP"', () => {
    expect(getProtocolBadge({ url: 'tcp://192.168.1.1:8080' })).toBe('TCP');
  });

  test('dns check_type → "DNS"', () => {
    expect(getProtocolBadge({ check_type: 'dns', url: '' })).toBe('DNS');
  });

  test('dns:// url → "DNS"', () => {
    expect(getProtocolBadge({ url: 'dns://example.com' })).toBe('DNS');
  });

  test('https:// url → "HTTPS"', () => {
    expect(getProtocolBadge({ url: 'https://example.com' })).toBe('HTTPS');
  });

  test('http:// url → "HTTP"', () => {
    expect(getProtocolBadge({ url: 'http://example.com' })).toBe('HTTP');
  });

  test('missing url and check_type defaults to "HTTP" uppercased', () => {
    expect(getProtocolBadge({})).toBe('HTTP');
  });

  test('custom check_type uppercased', () => {
    expect(getProtocolBadge({ check_type: 'icmp', url: '' })).toBe('ICMP');
  });

  test('case insensitive url matching', () => {
    expect(getProtocolBadge({ url: 'HTTPS://Example.Com' })).toBe('HTTPS');
  });

  test('case insensitive check_type', () => {
    expect(getProtocolBadge({ check_type: 'Always_Up' })).toBe('DEMO');
  });
});

/* ── maskUrl ────────────────────────────────────────────── */
describe('maskUrl', () => {
  test('strips path and port from https URL', () => {
    expect(maskUrl('https://plex.example.com:32400/web/index.html')).toBe('https://plex.example.com');
  });

  test('strips path from http URL', () => {
    expect(maskUrl('http://192.168.1.10:8080/api')).toBe('http://192.168.1.10');
  });

  test('preserves protocol', () => {
    const result = maskUrl('https://myserver.com/path');
    expect(result.startsWith('https://')).toBe(true);
  });

  test('invalid URL returns "***"', () => {
    expect(maskUrl('not-a-url')).toBe('***');
  });

  test('empty string returns "***"', () => {
    expect(maskUrl('')).toBe('***');
  });

  test('plain domain without protocol returns "***"', () => {
    expect(maskUrl('example.com')).toBe('***');
  });
});

/* ── generateServiceKey ─────────────────────────────────── */
describe('generateServiceKey', () => {
  test('lowercases name', () => {
    expect(generateServiceKey('Plex')).toBe('plex');
  });

  test('replaces spaces with hyphens', () => {
    expect(generateServiceKey('My Service')).toBe('my-service');
  });

  test('removes special characters', () => {
    expect(generateServiceKey('Sonarr (TV)')).toBe('sonarr-tv');
  });

  test('strips leading/trailing hyphens', () => {
    expect(generateServiceKey('--test--')).toBe('test');
  });

  test('truncates to 32 characters', () => {
    const longName = 'a'.repeat(50);
    expect(generateServiceKey(longName).length).toBe(32);
  });

  test('collapses consecutive special chars to single hyphen', () => {
    expect(generateServiceKey('a!!!b')).toBe('a-b');
  });

  test('empty string returns empty', () => {
    expect(generateServiceKey('')).toBe('');
  });

  test('already kebab-case passes through', () => {
    expect(generateServiceKey('my-service-name')).toBe('my-service-name');
  });
});
