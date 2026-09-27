# The web app

Plain HTML, CSS and JavaScript, no framework and no build step. The server embeds `web/` and serves `app.js` as
`web/js/*.js` joined in name order, so each part can use the ones before it.

| File | Part |
|---|---|
| `01-core.js` | Icons, illustrations, app state (`S`), talking to the server (`api`), theme, printer options, routing helpers |
| `02-home.js` | The home screen |
| `03-files.js` | Picking files: from the phone, the computer, the camera; the file list |
| `04-print.js` | The one print screen: pages big, settings sliding in, previews, printing and the flip guide |
| `05-text.js` | Printing text |
| `06-editor.js` | The photo editor (Crop, Adjust, Filters, Markup), autosave |
| `07-setup.js` | First-time setup, the tour, the printer questions |
| `08-scan.js` | Scanning and copying |
| `09-care.js` | Printer care |
| `10-settings.js` | Settings |
| `11-ui.js` | Shared pieces: modals, questions, toasts, the petal burst, the layout (sidebar or tab bar, header) |
| `12-notices.js` | The notifications centre |
| `13-camscan.js` | The camera scanner |
| `14-again.js` | Print again |
| `15-drivers.js` | Printers & drivers |
| `16-advanced.js` | Advanced: diagnostics |
| `99-start.js` | Routes and starting the app: must stay last |

Other files: `index.html`, `app.css` (the design tokens and every screen), `develop.js` (the editor's pixel
engine: adjustments, filters, blur, perspective, page finding; also used by the camera scanner; tested by
`develop_test.mjs`), `vendor/` (Cropper.js and Fabric.js, MIT, only loaded when the editor opens), icons, manifest.

## Layout and design rules

- **One layout everywhere**: a header (back, title, the printer, the notifications bell), the screen's content in
  a centred column (at most 720px wide), and navigation that's always there: a tab bar at the bottom on phones, a
  sidebar on wider screens. A screen's main action sits above the tab bar, never instead of it.
- **Design tokens** in `:root` (colours from the chosen palette, spacing, radii, type sizes); screens use tokens,
  not their own numbers.
- **Pictures over words**: drawn, filled icons (no emoji); numbered picture guides for anything physical (paper,
  flipping); one short line of text each.
- **Big and forgiving**: touch targets at least 44px; every destructive action asks first; nothing important only
  on hover.
- **A4 by default**, the person's language kept plain: no jargon without a picture or a sentence.

## Routing

`location.hash` picks the screen (`#/`, `#/docs`, `#/settings`…, see `routes` in `99-start.js`). Screens render
into `#app` with `render()`; modals stack above; the layout (header, navigation) is drawn once and only updated.
