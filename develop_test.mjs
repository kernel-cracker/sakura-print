// Tests for web/develop.js. Run: node develop_test.mjs
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
const ctx = {}; vm.createContext(ctx);
vm.runInContext(readFileSync(fileURLToPath(new URL('./web/develop.js', import.meta.url)), 'utf8'), ctx);
const D = ctx.SakuraDevelop;
let fails = 0;
const ok = (c, m) => { if (!c) { fails++; console.log('FAIL ' + m); } else console.log('PASS ' + m); };

// a test picture: left half a mid-grey gradient, right half coloured noise, with a sharp edge in the middle
function pic(w = 64, h = 48, fill) {
  const data = new Uint8ClampedArray(w * h * 4);
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const i = (y * w + x) * 4, v = fill ? fill(x, y) : x < w / 2 ? [60 + x * 3, 90 + y, 120] : [180 + ((x * 7 + y * 13) % 30), 70, 50 + ((x * 3) % 20)];
    data[i] = v[0]; data[i + 1] = v[1]; data[i + 2] = v[2]; data[i + 3] = 255;
  }
  return { data, width: w, height: h };
}
const mean = (img, ch) => { let s = 0, n = 0; for (let i = ch; i < img.data.length; i += 4) { s += img.data[i]; n++; } return s / n; };
const lum = img => 0.299 * mean(img, 0) + 0.587 * mean(img, 1) + 0.114 * mean(img, 2);
const run = (adj, filter, amount) => D.develop(pic(), adj, filter, amount);
const base = pic();

ok(D.develop(pic(), {}).data.every((v, i) => v === base.data[i]), 'no changes leaves every pixel as it was');
ok(lum(run({ exposure: 50 })) > lum(base) + 15 && lum(run({ exposure: -50 })) < lum(base) - 15, 'Exposure brightens / darkens');
ok(lum(run({ brightness: 50 })) > lum(base) + 10, 'Brightness brightens');
const spread = img => { let lo = 255, hi = 0; for (let i = 0; i < img.data.length; i += 4) { const l = img.data[i + 1]; lo = Math.min(lo, l); hi = Math.max(hi, l); } return hi - lo; };
ok(spread(run({ contrast: 60 })) > spread(base) && spread(run({ contrast: -60 })) < spread(base), 'Contrast spreads / squeezes tones');
const dark = pic(64, 48, () => [30, 30, 30]), light = pic(64, 48, () => [225, 225, 225]);
ok(mean(D.develop(pic(64, 48, () => [30, 30, 30]), { shadows: 80 }), 1) > mean(dark, 1) + 20, 'Shadows lifts dark areas');
ok(Math.abs(mean(D.develop(pic(64, 48, () => [225, 225, 225]), { shadows: 80 }), 1) - mean(light, 1)) < 8, '...and barely touches bright ones');
ok(mean(D.develop(pic(64, 48, () => [225, 225, 225]), { highlights: -80 }), 1) < mean(light, 1) - 20, 'Highlights (minus) pulls down bright areas');
ok(mean(D.develop(pic(64, 48, () => [40, 40, 40]), { black: 60 }), 1) < 40, 'Black point deepens blacks');
const satOf = img => { let s = 0, n = 0; for (let i = 0; i < img.data.length; i += 4) { const d = img.data; s += Math.max(d[i], d[i + 1], d[i + 2]) - Math.min(d[i], d[i + 1], d[i + 2]); n++; } return s / n; };
ok(satOf(run({ saturation: 60 })) > satOf(base) + 5 && satOf(run({ saturation: -100 })) < 2, 'Saturation boosts colour; -100 is grey');
ok(satOf(run({ vibrance: 60 })) > satOf(base), 'Vibrance boosts colour');
const w = run({ warmth: 60 });
ok(mean(w, 0) > mean(base, 0) + 5 && mean(w, 2) < mean(base, 2) - 5, 'Warmth + is warmer (more red, less blue)');
ok(mean(run({ tint: 60 }), 1) < mean(base, 1) - 5, 'Tint + is more magenta (less green)');
const gray = () => pic(64, 48, () => [150, 150, 150]);
const vg = D.develop(gray(), { vignette: 80 }), px = (img, x, y) => img.data[(y * img.width + x) * 4];
ok(px(vg, 0, 0) < 110 && Math.abs(px(vg, 32, 24) - 150) < 3, 'Vignette darkens corners, not the middle');
ok(px(D.develop(gray(), { vignette: -80 }), 0, 0) > 170, 'negative Vignette lightens corners');
const edge = () => pic(64, 48, x => x < 32 ? [100, 100, 100] : [160, 160, 160]);
const sh = D.develop(edge(), { sharpness: 100 });
ok(px(sh, 31, 20) < 100 && px(sh, 32, 20) > 160, 'Sharpness makes an edge crisper');
const noisy = () => pic(64, 48, (x, y) => { const v = 128 + (((x * 31 + y * 17) % 7) - 3) * 12; return [v, v, v]; });
const varOf = img => { const m = mean(img, 0); let s = 0, n = 0; for (let i = 0; i < img.data.length; i += 4) { s += (img.data[i] - m) ** 2; n++; } return s / n; };
ok(varOf(D.develop(noisy(), { noise: 100 })) < varOf(noisy()) * 0.7, 'Noise reduction smooths grain');
ok(varOf(D.develop(pic(), { definition: 100 })) > varOf(pic()), 'Definition adds mid-tone contrast');
ok(lum(D.develop(pic(64, 48, () => [40, 50, 45]), { auto: 100 })) > 60, 'Auto brightens a dark picture');
ok(Math.abs(lum(D.develop(pic(64, 48, () => [40, 50, 45]), { auto: 0 })) - 45.7) < 1, 'Auto at 0% does nothing');
const mono = run({}, 'mono', 100);
ok(satOf(mono) < 1, 'Mono is grey');
ok(D.develop(pic(), {}, 'vivid', 0).data.every((v, i) => v === base.data[i]), 'a filter at 0% changes nothing');
ok(satOf(run({}, 'mono', 50)) > 5 && satOf(run({}, 'mono', 50)) < satOf(base) - 5, 'a filter at 50% is halfway');
for (const f of D.FILTERS.filter(f => f.key !== 'none')) ok(run({}, f.key, 100).data.some((v, i) => v !== base.data[i]), `filter ${f.label} changes the picture`);
ok(lum(run({}, 'document', 100)) > lum(base) + 10, 'Document makes the page lighter');
const bb = D.boxBlur(new Float32Array(100).fill(7), 10, 10, 3);
ok(bb.every(v => Math.abs(v - 7) < 1e-4), 'blur of a flat image stays flat');
const H = D.squareToQuad([[10, 5], [90, 0], [100, 80], [0, 70]]);
const near = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]) < 1e-6;
ok(near(D.applyH(H, 0, 0), [10, 5]) && near(D.applyH(H, 1, 0), [90, 0]) && near(D.applyH(H, 1, 1), [100, 80]) && near(D.applyH(H, 0, 1), [0, 70]), 'perspective maps the four corners exactly');
for (const [v, h] of [[60, 0], [-60, 0], [0, 60], [0, -60], [100, -100]]) {
  const q = D.perspectiveQuad(200, 100, v, h);
  ok(q.every(([x, y]) => x >= 0 && x <= 200 && y >= 0 && y <= 100), `perspective ${v}/${h} only samples inside the picture (no empty corners)`);
}
ok(D.ADJUST.length === 16 && D.FILTERS.length === 11, 'all 16 adjustments and 11 filters are there');
// ---- document scanning ----
// a photo of a page: background colour, a four-sided white page with dark text lines, optional shadow
function photo(w, h, quad, { bg = [70, 60, 55], page = [238, 236, 230], shadow = 0 } = {}) {
  const img = pic(w, h, () => bg);
  const inside = (x, y) => { let c = false; for (let i = 0, j = 3; i < 4; j = i++) { const [xi, yi] = quad[i], [xj, yj] = quad[j]; if ((yi > y) !== (yj > y) && x < (xj - xi) * (y - yi) / (yj - yi) + xi) c = !c; } return c; };
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const i = (y * w + x) * 4, k = 1 - shadow * x / w;
    let c = inside(x, y) ? page : bg;
    if (inside(x, y) && (y % 20 < 2) && x % 60 > 8) c = [40, 40, 50];   // lines of writing
    img.data[i] = c[0] * k; img.data[i + 1] = c[1] * k; img.data[i + 2] = c[2] * k;
  }
  return img;
}
const cornersNear = (got, want, tol) => got && got.every((p, i) => Math.hypot(p[0] - want[i][0], p[1] - want[i][1]) <= tol);
for (const [label, quad, opts] of [
  ['straight page', [[60, 40], [260, 40], [260, 320], [60, 320]]],
  ['tilted page', [[90, 30], [280, 70], [240, 330], [50, 290]]],
  ['page shot at an angle', [[100, 50], [220, 50], [290, 330], [30, 330]]],
  ['page off to one side', [[10, 20], [180, 25], [175, 260], [8, 250]]],
  ['page in bad light', [[60, 40], [260, 40], [260, 320], [60, 320]], { shadow: 0.45 }],
]) {
  const got = D.findPage(photo(320, 360, quad, opts));
  ok(cornersNear(got, quad, 9), `finds the corners: ${label} (${JSON.stringify(got)})`);
}
{ const q = [[60, 40], [260, 40], [260, 320], [60, 320]], got = D.findPage(photo(320, 360, q, { bg: [235, 235, 235], page: [70, 70, 70] }));
  ok(cornersNear(got, q, 9), `a dark page on a white table: finds the page, not the table (${JSON.stringify(got)})`); }
ok(D.findPage(pic(100, 100, () => [128, 128, 128])) === null, 'a photo with no page in it: gives up');
const sz = D.pageSize([[0, 0], [210, 0], [212, 300], [0, 296]]);
ok(Math.abs(sz.w / sz.h - 210 / 297) < 4e-3 && sz.h === 373, `a nearly-A4 page is made exactly A4-shaped (${sz.w}x${sz.h})`);
ok(D.pageSize([[0, 0], [300, 0], [300, 100], [0, 100]]).w > D.pageSize([[0, 0], [300, 0], [300, 100], [0, 100]]).h, 'a wide receipt stays wide');
ok(D.pageSize([[0, 0], [5000, 0], [5000, 7000], [0, 7000]]).h === 2339, 'never bigger than A4 at 200 dpi');
const bw = D.blackWhite(photo(300, 300, [[0, 0], [300, 0], [300, 300], [0, 300]], { shadow: 0.55 }));
const at = (x, y) => bw.data[(y * 300 + x) * 4];
ok(at(30, 0) === 0 && at(270, 0) === 0, 'Black & white: writing stays black, in the light and in the shadow');
ok(at(30, 10) === 255 && at(270, 10) === 255, 'Black & white: paper is white, in the light and in the shadow');

console.log(fails ? `${fails} failed` : 'all passed');
process.exit(fails ? 1 : 0);
