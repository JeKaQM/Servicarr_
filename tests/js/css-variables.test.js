/**
 * Every CSS custom property the frontend reads must be defined somewhere.
 * An undefined var() without a fallback silently drops the declaration: that
 * left the day-detail dialog see-through and the blocked page's "Try Again"
 * button without a background.
 */
const fs = require('fs');
const path = require('path');

const WEB = path.join(__dirname, '..', '..', 'web');

function filesUnder(dir, exts) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return filesUnder(full, exts);
    return exts.includes(path.extname(entry.name)) ? [full] : [];
  });
}

const sources = filesUnder(WEB, ['.css', '.js', '.html']).map((file) => ({
  file: path.relative(WEB, file).replace(/\\/g, '/'),
  text: fs.readFileSync(file, 'utf8'),
}));

function definedProperties() {
  const defined = new Set();
  for (const { text } of sources) {
    for (const m of text.matchAll(/(--[\w-]+)\s*:/g)) defined.add(m[1]);
    for (const m of text.matchAll(/setProperty\(\s*['"](--[\w-]+)/g)) defined.add(m[1]);
  }
  return defined;
}

test('every var() without a fallback refers to a defined custom property', () => {
  const defined = definedProperties();
  const missing = [];
  for (const { file, text } of sources) {
    for (const m of text.matchAll(/var\(\s*(--[\w-]+)\s*([,)])/g)) {
      if (m[2] === ')' && !defined.has(m[1])) missing.push(`${file}: ${m[1]}`);
    }
  }
  expect(missing).toEqual([]);
});
