/**
 * Test helper – loads vanilla JS source files into the jsdom global scope.
 *
 * Usage:
 *   const { loadSource } = require('./test-helpers');
 *   beforeAll(() => loadSource('core.js'));
 */
const fs = require('fs');
const path = require('path');

const JS_DIR = path.resolve(__dirname);

/**
 * Evaluate a source file in the current (global / jsdom) scope.
 * Successive calls accumulate – just like multiple <script> tags.
 *
 * Top-level `const` and `let` are rewritten to `var` so that
 * indirect eval makes them properties of the global object,
 * matching browser <script> behaviour for function-scoped vars.
 */
function loadSource(...files) {
  for (const file of files) {
    let code = fs.readFileSync(path.join(JS_DIR, file), 'utf-8');
    // Replace top-level (unindented) const/let with var so they attach to global
    code = code.replace(/^(const|let) /gm, 'var ');
    // Indirect eval → runs in global scope; function decls + var become global
    (0, eval)(code);
  }
}

module.exports = { loadSource };
