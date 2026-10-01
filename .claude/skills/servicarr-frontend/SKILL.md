---
name: servicarr-frontend
description: Conventions for Servicarr's no-build vanilla JS/CSS frontend - bundle order, CSP-safe event wiring, escaping, polling, accessibility, styling and how to verify UI changes. Use before editing anything under web/ (static JS, CSS, templates).
---

# Servicarr frontend conventions

## How code reaches the browser

- There is no bundler, transpiler or module system. `app/internal/handlers/bundle.go` concatenates the files it lists, **in that order**, into `public-bundle.{js,css}` and `admin-bundle.{js,css}` when the server starts. A new file must be added to the right list (a listed file that is missing aborts startup). Restart the server, or rebuild the container, to see changes.
- Admin bundles are only served to authenticated sessions. Public code must not depend on admin functions; guard optional calls with `typeof fn === 'function'`.
- Every file shares one global scope, so check for name collisions before adding a top-level function. Shared helpers: `$`/`$$` and constants (`core.js`), `escapeHtml`/`showToast`/`getCsrf` (`utils.js`), `j()` fetch wrapper (bottom of `resources.js`), main `refresh()` loop (`day-detail.js`), `getServiceIconHtml` (`services.js`).
- `setup.html` and `blocked.html` are standalone pages with their own scripts (`setup.js`, `blocked.js`).

## Security constraints

- The CSP forbids inline scripts, inline event handlers and script CDNs. Wire behaviour with delegated listeners on `data-action` attributes, never `onclick=`.
- Prefer `textContent` and `createElement`. When building HTML strings, pass every interpolated value through `escapeHtml`, which also escapes quotes and so works in attribute values. Treat CrowdSec fields, log and audit entries, service names/URLs and Glances/NUT values as attacker-influenced.
- URLs placed in `src`/`href` must match the allowlist used for icons, `/^(https?:\/\/|data:image\/|\/static\/)/`, before escaping.
- Mutations go through `j()` with the `X-CSRF-Token` header from `getCsrf()`. The server rejects cross-origin mutations and requires the token on authenticated non-GET requests.

## Data and polling

- Dashboards poll every `REFRESH_MS` (15s). Public responses (`/api/check`, `/api/metrics`) are cached and shared server-side for 10-15s, so faster polling gains nothing.
- Treat 429 as "keep the last good render", not as "no data" (see `refresh()`).
- Highlights and summary panels must query what they summarise. Do not derive them from one page of a paginated list; `loadErrorHighlights` in `logs-tab.js` replaced exactly that mistake.

## Accessibility baseline

- Anything clickable is a `<button>`, or has `role="button"`, `tabindex="0"` and Enter/Space handling.
- Never convey status by colour alone: pair it with text or an icon (UP/DOWN pills, tooltips).
- Keep visible `:focus-visible` outlines, label every form control, and use `aria-live` for toasts and status changes.
- Honour `prefers-reduced-motion` for animations (matrix view, map, entrance effects).

## Charts, meters and KPI tiles

Load the `dataviz` skill before building or restyling any chart, sparkline, meter or stat tile, including CrowdSec charts, resource meters and uptime bars. Keep series colours consistent between charts and meet contrast in the dark theme.

## Design tokens (`web/static/css/base.css`)

Every page loads `base.css` first, through the bundles or `main.css`. Use tokens, not new hex values:

| Group | Tokens |
|---|---|
| Surfaces | `--bg`, `--surface-1` (cards, dialogs), `--surface-2`, `--surface-inset`, `--border`, `--border-subtle` |
| Ink | `--text`, `--text-secondary`, `--muted`, `--faint` |
| Status marks | `--status-up`, `--status-partial`, `--status-down`, `--status-none`, `--status-maintenance`, `--status-disabled` |
| Status text | `--status-up-ink`, `--status-partial-ink`, `--status-down-ink` (at least 4.5:1 on `--surface-1`) |
| Type, shape, motion | `--font-sans`, `--font-mono`, `--text-xs/sm/base`, `--radius-sm/md/lg`, `--ease`, `--dur-fast`, `--dur`, `--focus` |

- **Status colours are a validated, fixed scale.** The previous green/red pair was indistinguishable for deuteranopes (dE 1.1). The current marks keep dE 10.0 or more for every pair, which is why the red is the darker `#e11d48`. Don't swap a status value without re-running the dataviz skill's `validate_palette.js --mode dark --surface "#111827" --pairs all`.
- Marks (bars, dots, swatches) use `--status-*`. Text uses `--status-*-ink`, or plain ink beside a coloured mark. Never render "unavailable" or "N/A" in a healthy colour.
- Legacy names (`--ok`, `--down`, `--warn`, `--card`) keep their original values for older rules. Aliases cover names some rules used without defining (`--card-bg`, `--fg`, `--up`, `--text-dim`). An undefined custom property silently drops the declaration (the day-detail dialog was transparent because of one), so add a fallback or define the token.
- Charts follow the dataviz rules: no dual axes (use stacked panels on a shared x-axis, as in `crowdsec-charts.js`), solid hairline grids, 2px lines, 2px surface gaps between stacked segments, no glows or outlines around marks, axis text in ink, a legend for 2 or more series, and a data-table view.
- Repeated interactive marks (uptime days) use a roving tabindex: one tab stop per group, arrow keys inside it, Enter/Space to activate. Preserve focus across the 15s re-render (see `renderUptimeBars`).
- Leak to know about: `day-detail.css` styles every bare `li` (flex layout, padding, dashed bottom border). Reset those properties on new lists, as `.uptime-legend li` does.

## Styling

- `mobile.css` holds the responsive overrides and comes after component CSS in the bundle, so a component's mobile rules belong there.
- `base.css` already handles `prefers-reduced-motion` and the very-wide max-width, so don't re-implement them per component.
- Check both 1440px and 390px widths.

## Verify

1. `npx jest --ci`. Tests in `tests/js` evaluate the real source files in jsdom; add one for new logic.
2. Rebuild the Docker stack and take screenshots with the `run-servicarr` skill, then look at them. Pages must load with no console errors and no 4xx/5xx responses.
