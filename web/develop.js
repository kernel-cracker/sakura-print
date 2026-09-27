// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
//
// Sakura Print photo engine: the Adjust sliders, the filters and perspective, done on raw pixels so the
// screen and the printed picture always match. Plain functions, no DOM except the perspective warp,
// so they can be tested on their own (see develop_test.mjs).
'use strict';
(function (root) {
  // every Adjust slider goes from -100 to 100 (Auto, Sharpness, Definition, Noise reduction: 0 to 100)
  const ADJUST = [
    ['auto', 'Auto', 0, 100], ['exposure', 'Exposure'], ['brilliance', 'Brilliance'], ['highlights', 'Highlights'],
    ['shadows', 'Shadows'], ['contrast', 'Contrast'], ['brightness', 'Brightness'], ['black', 'Black point'],
    ['saturation', 'Saturation'], ['vibrance', 'Vibrance'], ['warmth', 'Warmth'], ['tint', 'Tint'],
    ['sharpness', 'Sharpness', 0, 100], ['definition', 'Definition', 0, 100], ['noise', 'Noise reduction', 0, 100],
    ['vignette', 'Vignette'],
  ].map(([key, label, min = -100, max = 100]) => ({ key, label, min, max }));

  const FILTERS = [
    ['none', 'Original'], ['document', 'Document'], ['vivid', 'Vivid'], ['vividWarm', 'Vivid Warm'], ['vividCool', 'Vivid Cool'],
    ['dramatic', 'Dramatic'], ['dramaticWarm', 'Dramatic Warm'], ['dramaticCool', 'Dramatic Cool'],
    ['mono', 'Mono'], ['silvertone', 'Silvertone'], ['noir', 'Noir'],
  ].map(([key, label]) => ({ key, label }));

  const blank = () => Object.fromEntries(ADJUST.map(a => [a.key, 0]));
  const clamp = (v, a, b) => v < a ? a : v > b ? b : v;
  const luma = (r, g, b) => 0.299 * r + 0.587 * g + 0.114 * b;

  // separable box blur of a single channel, with running sums so the cost doesn't grow with the radius.
  // Both passes read memory in order (row after row); going down columns instead made it ~10x slower on
  // big photos, because every step jumped a whole row ahead in memory.
  function boxBlur(src, w, h, r) {
    if (r < 1) return src.slice();
    const n = 2 * r + 1, tmp = new Float32Array(w * h), out = new Float32Array(w * h);
    for (let y = 0; y < h; y++) {   // across: each row, edges repeat the outermost pixel
      const row = y * w;
      let s = src[row] * (r + 1);
      for (let x = 1; x <= r; x++) s += src[row + (x < w ? x : w - 1)];
      for (let x = 0; x < w; x++) {
        tmp[row + x] = s / n;
        const ad = x + r + 1, sb = x - r;
        s += src[row + (ad < w ? ad : w - 1)] - src[row + (sb > 0 ? sb : 0)];
      }
    }
    const acc = new Float32Array(w);   // down: a running total per column, updated a whole row at a time
    for (let x = 0; x < w; x++) acc[x] = tmp[x] * (r + 1);
    for (let y = 1; y <= r; y++) { const row = (y < h ? y : h - 1) * w; for (let x = 0; x < w; x++) acc[x] += tmp[row + x]; }
    for (let y = 0; y < h; y++) {
      const o = y * w, ad = (y + r + 1 < h ? y + r + 1 : h - 1) * w, sb = (y - r > 0 ? y - r : 0) * w;
      for (let x = 0; x < w; x++) { out[o + x] = acc[x] / n; acc[x] += tmp[ad + x] - tmp[sb + x]; }
    }
    return out;
  }

  // Auto: what the picture needs, from its brightness spread (a gentle "levels" fix), before strength
  function autoFix(px) {
    const hist = new Uint32Array(256); let n = 0, sum = 0;
    for (let i = 0; i < px.length; i += 16) { const l = luma(px[i], px[i + 1], px[i + 2]) | 0; hist[l]++; n++; sum += l; }
    if (!n) return blank();
    const pct = q => { let c = 0; for (let v = 0; v < 256; v++) { c += hist[v]; if (c >= n * q) return v; } return 255; };
    const lo = pct(0.02), hi = pct(0.98), mean = sum / n;
    const a = blank();
    a.exposure = clamp(Math.log2(118 / Math.max(mean, 8)) * 55, -45, 45);
    // stretching the tones only makes sense if there's a real spread; a flat picture just needs light
    const spread = hi - lo;
    a.black = spread > 60 ? clamp((lo - 6) * 1.2, -10, 35) : 0;
    a.contrast = spread > 60 ? clamp((200 / spread - 1) * 45, -15, 35) : 0;
    a.highlights = hi > 245 ? -20 : 0;
    a.shadows = lo < 20 ? 15 : 0;
    a.vibrance = 18;
    return a;
  }

  const defRadius = (w, h) => Math.max(2, Math.round(Math.min(w, h) / 60));
  // resolve folds Auto into the other sliders, using the whole picture's brightness
  function resolve(px, adj) {
    const A = { ...blank(), ...adj };
    if (A.auto) { const f = autoFix(px), k = A.auto / 100; for (const key in f) if (key !== 'auto') A[key] += f[key] * k; A.auto = 0; }
    return A;
  }

  // developFast: the same as develop, but split into strips done at the same time by background workers
  // (one per processor core, up to 6). Strips overlap by the blur size, so the joins are exact.
  let pool = null;
  function workers() {
    if (pool) return pool;
    const cores = Math.max(1, Math.min(6, (typeof navigator !== 'undefined' && navigator.hardwareConcurrency) || 2));
    try {
      const src = (typeof document !== 'undefined' && document.currentScript && document.currentScript.src) || 'develop.js';
      const code = `importScripts(${JSON.stringify(new URL(src, location.href).href)});
        onmessage = e => { const { id, buf, w, h, adj, filter, amount, whole } = e.data; const img = { data: new Uint8ClampedArray(buf), width: w, height: h };
          SakuraDevelop.develop(img, adj, filter, amount, whole); postMessage({ id, buf }, [buf]); };`;
      const url = URL.createObjectURL(new Blob([code], { type: 'text/javascript' }));
      pool = Array.from({ length: cores }, () => new Worker(url));
    } catch (_) { pool = []; }
    return pool;
  }
  let jobId = 0;
  async function developFast(img, adj, filter = 'none', amount = 100) {
    const w = img.width, h = img.height, px = img.data;
    const ws = typeof Worker !== 'undefined' && w * h > 400000 ? workers() : [];
    if (ws.length < 2) return develop(img, adj, filter, amount);
    const A = resolve(px, adj), radius = defRadius(w, h), pad = (A.definition ? radius : 0) + 2;
    const rows = Math.ceil(h / ws.length);
    await Promise.all(ws.map((wk, k) => new Promise(done => {
      const y0 = k * rows, y1 = Math.min(h, y0 + rows);
      if (y0 >= y1) return done();
      const s0 = Math.max(0, y0 - pad), s1 = Math.min(h, y1 + pad);
      const buf = px.slice(s0 * w * 4, s1 * w * 4).buffer, id = ++jobId;
      const got = e => {
        if (e.data.id !== id) return;
        wk.removeEventListener('message', got);
        const out = new Uint8ClampedArray(e.data.buf);
        px.set(out.subarray((y0 - s0) * w * 4, (y1 - s0) * w * 4), y0 * w * 4);   // keep only this strip's own rows
        done();
      };
      wk.addEventListener('message', got);
      // Definition's blur size and the vignette centre come from the whole picture, not the strip
      wk.postMessage({ id, buf, w, h: s1 - s0, adj: A, filter, amount, whole: { w, h, top: s0, radius } }, [buf]);
    })));
    return img;
  }

  // each filter as a few numbers instead of a function, so the pixel loop never makes a new array:
  // c/b: contrast and brightness around mid-grey, s: saturation, mono: brightness only, warm: warmer (+) or
  // cooler (-), off: a tint added to each channel after going grey (Silvertone's cool shadows)
  const LOOKS = {
    document: { c: 1.36, b: 36, s: 0.9 }, vivid: { c: 1.08, s: 1.32 }, dramatic: { c: 1.32, b: -10, s: 0.78 },
    vividWarm: { c: 1.08, s: 1.32, warm: 14 }, vividCool: { c: 1.08, s: 1.32, warm: -12 },
    dramaticWarm: { c: 1.32, b: -10, s: 0.78, warm: 14 }, dramaticCool: { c: 1.32, b: -10, s: 0.78, warm: -12 },
    mono: { mono: true }, silvertone: { mono: true, c: 1.12, b: 6, off: [-3, 0, 6] }, noir: { mono: true, c: 1.6, b: -14 },
  };

  // develop changes the pixels of an ImageData (or anything with .data, .width, .height) in place.
  // adj: the Adjust values; filter / amount: the filter and how strong (0..100)
  // Everything that only depends on a brightness level is worked out once into small tables, so the loop
  // over millions of pixels does as little as possible (this is what makes saving fast).
  // whole: when this is one strip of a bigger picture, {w, h, top, radius} of the whole thing, so the vignette
  // centre and the Definition blur size match the whole picture (and Auto should already be folded in: resolve())
  function develop(img, adj, filter = 'none', amount = 100, whole = null) {
    const px = img.data, w = img.width, h = img.height, n = w * h;
    const A = whole ? { ...blank(), ...adj } : resolve(px, adj);
    const look = LOOKS[filter], fa = look ? amount / 100 : 0;
    if (!Object.keys(A).some(k => A[k]) && !fa) return img;

    const ex = Math.pow(2, A.exposure / 100 * 1.4), br = A.brightness * 0.6, ct = 1 + A.contrast / 100 * 0.7;
    const hi = A.highlights / 100, sh = A.shadows / 100, bri = A.brilliance / 100, bp = A.black * 0.35;
    const sa = 1 + A.saturation / 100, vib = A.vibrance / 100, wa = A.warmth * 0.3, ti = A.tint * 0.3;
    // brilliance / highlights / shadows / brightness: an amount added, by brightness level
    const tone = new Float32Array(256);
    for (let v = 0; v < 256; v++) { const t = v / 255, u = 1 - t; tone[v] = br + sh * u * u * 90 + hi * t * t * 90 + bri * (u * u * 55 - t * t * 35); }
    // contrast then black point, folded into one straight line: out = x * mul + add
    const kb = bp ? 255 / (255 - bp) : 1, mul = ct * kb, add = (128 * (1 - ct) - bp) * kb;
    const addR = wa + ti * 0.5, addG = -ti, addB = -wa + ti * 0.5;
    const colour = sa !== 1 || vib !== 0;

    // local detail (sharpness, definition, noise reduction) works on brightness only, so colours don't fringe
    let L = null, fine = null, broad = null;
    if (A.sharpness || A.noise || A.definition) {
      L = new Float32Array(n);
      for (let i = 0, j = 0; j < n; i += 4, j++) L[j] = 0.299 * px[i] + 0.587 * px[i + 1] + 0.114 * px[i + 2];
      if (A.sharpness || A.noise) fine = boxBlur(L, w, h, 1);
      if (A.definition) broad = boxBlur(L, w, h, whole ? whole.radius : defRadius(w, h));
    }
    const shp = A.sharpness / 100 * 1.6 - A.noise / 100, def = A.definition / 100 * 0.9 * 4;

    // vignette: darkening by squared distance from the middle, from a table (no square root per pixel)
    const W0 = whole ? whole.w : w, H0 = whole ? whole.h : h, top = whole ? whole.top : 0;
    const vg = A.vignette / 100, cx = (W0 - 1) / 2, cy = (H0 - 1) / 2, rad2 = (cx * cx + cy * cy) || 1, VN = 2048;
    let vtab = null;
    if (vg) {
      vtab = new Float32Array(VN + 1);
      for (let q = 0; q <= VN; q++) { const d = Math.sqrt(q / VN), e = clamp((d - 0.35) / 0.75, 0, 1); vtab[q] = 1 - vg * e * e * 0.85; }
    }
    const fc = look ? (look.c || 1) : 1, fb = look ? (look.b || 0) : 0, fs = look ? (look.s || 1) : 1, fm = look && look.mono, fwarm = look ? (look.warm || 0) : 0;
    const fo = look && look.off ? look.off : [0, 0, 0], fadd = 128 * (1 - fc) + fb;

    for (let y = 0, j = 0, i = 0; y < h; y++) {
      const dy2 = (y + top - cy) * (y + top - cy);
      for (let x = 0; x < w; x++, j++, i += 4) {
        let r = px[i], g = px[i + 1], b = px[i + 2];
        if (L) {   // detail: push each pixel's brightness away from (sharpen) or toward (smooth) its neighbours
          let d = 0;
          if (fine) d += (L[j] - fine[j]) * shp;
          if (broad) { const m = L[j] / 255; d += (L[j] - broad[j]) * def * m * (1 - m); }
          r += d; g += d; b += d;
        }
        r *= ex; g *= ex; b *= ex;
        let lv = 0.299 * r + 0.587 * g + 0.114 * b;
        const t = tone[lv < 0 ? 0 : lv > 255 ? 255 : lv | 0];
        r = (r + t) * mul + add + addR; g = (g + t) * mul + add + addG; b = (b + t) * mul + add + addB;
        if (colour) {
          lv = 0.299 * r + 0.587 * g + 0.114 * b;
          const mx = r > g ? (r > b ? r : b) : (g > b ? g : b), mn = r < g ? (r < b ? r : b) : (g < b ? g : b);
          const s = (mx - mn) / 255, k = sa * (1 + vib * (1 - (s > 1 ? 1 : s)));
          r = lv + (r - lv) * k; g = lv + (g - lv) * k; b = lv + (b - lv) * k;
        }
        if (vtab) { const dx = x - cx, q = (dx * dx + dy2) / rad2 * VN, k = vtab[q > VN ? VN : q | 0]; r *= k; g *= k; b *= k; }
        if (fa) {
          let fr, fg, fb2;
          if (fm) { const l = (0.299 * r + 0.587 * g + 0.114 * b) * fc + fadd; fr = l + fo[0]; fg = l + fo[1]; fb2 = l + fo[2]; }
          else {
            fr = r * fc + fadd; fg = g * fc + fadd; fb2 = b * fc + fadd;
            if (fs !== 1) { const l = 0.299 * fr + 0.587 * fg + 0.114 * fb2; fr = l + (fr - l) * fs; fg = l + (fg - l) * fs; fb2 = l + (fb2 - l) * fs; }
            if (fwarm) { fr += fwarm; fg += fwarm * 0.25; fb2 -= fwarm; }
          }
          r += (fr - r) * fa; g += (fg - g) * fa; b += (fb2 - b) * fa;
        }
        px[i] = r; px[i + 1] = g; px[i + 2] = b;   // Uint8ClampedArray clamps to 0..255 by itself
      }
    }
    return img;
  }

  // perspective: a 3x3 homography mapping the unit square to a quadrilateral (for the warp below)
  function squareToQuad(q) {
    const [[x0, y0], [x1, y1], [x2, y2], [x3, y3]] = q;   // top-left, top-right, bottom-right, bottom-left
    const dx1 = x1 - x2, dx2 = x3 - x2, dy1 = y1 - y2, dy2 = y3 - y2, sx = x0 - x1 + x2 - x3, sy = y0 - y1 + y2 - y3;
    const den = dx1 * dy2 - dx2 * dy1;
    const g = (sx * dy2 - dx2 * sy) / den, hh = (dx1 * sy - sx * dy1) / den;
    return [x1 - x0 + g * x1, x3 - x0 + hh * x3, x0, y1 - y0 + g * y1, y3 - y0 + hh * y3, y0, g, hh, 1];
  }
  const applyH = (H, u, v) => { const z = H[6] * u + H[7] * v + H[8]; return [(H[0] * u + H[1] * v + H[2]) / z, (H[3] * u + H[4] * v + H[5]) / z]; };

  // where each corner of the output comes from in the picture, for the Vertical / Horizontal sliders.
  // Leaning the sample area inward on one side stretches that side back out: converging lines straighten.
  // The area always stays inside the picture, so there are never empty corners.
  function perspectiveQuad(w, h, vert, horiz) {
    const a = Math.abs(vert) / 100 * 0.22 * w, b = Math.abs(horiz) / 100 * 0.22 * h;
    const q = [[0, 0], [w, 0], [w, h], [0, h]];
    if (vert > 0) { q[0][0] += a; q[1][0] -= a; } else if (vert < 0) { q[3][0] += a; q[2][0] -= a; }
    if (horiz > 0) { q[0][1] += b; q[3][1] -= b; } else if (horiz < 0) { q[1][1] += b; q[2][1] -= b; }
    return q;
  }

  // warp: draws `src` into `dst` through the Vertical / Horizontal perspective sliders
  function warp(src, dst, vert, horiz, cells = 16) { return warpQuad(src, dst, perspectiveQuad(src.width, src.height, vert, horiz), cells); }

  // warpQuad: the four-sided area `quad` of `src` (corners top-left, top-right, bottom-right, bottom-left)
  // stretched flat to fill `dst`, drawn as many small triangles (a 2D canvas can only stretch evenly, so the
  // picture is cut into pieces that are each nearly flat)
  function warpQuad(src, dst, quad, cells = 16) {
    const w = dst.width, h = dst.height, g = dst.getContext('2d');
    const H = squareToQuad(quad);
    g.fillStyle = '#fff'; g.fillRect(0, 0, w, h);
    const P = [];   // grid of [output point, source point]
    for (let j = 0; j <= cells; j++) for (let i = 0; i <= cells; i++) P.push([[i / cells * w, j / cells * h], applyH(H, i / cells, j / cells)]);
    const at = (i, j) => P[j * (cells + 1) + i];
    const tri = (a, b, c) => {
      const [[X0, Y0], [u0, v0]] = a, [[X1, Y1], [u1, v1]] = b, [[X2, Y2], [u2, v2]] = c;
      const den = u0 * (v2 - v1) - u1 * v2 + u2 * v1 + (u1 - u2) * v0;
      if (!den) return;
      const m11 = -(v0 * (X2 - X1) - v1 * X2 + v2 * X1 + (v1 - v2) * X0) / den, m12 = (v1 * Y2 + v0 * (Y1 - Y2) - v2 * Y1 + (v2 - v1) * Y0) / den;
      const m21 = (u0 * (X2 - X1) - u1 * X2 + u2 * X1 + (u1 - u2) * X0) / den, m22 = -(u1 * Y2 + u0 * (Y1 - Y2) - u2 * Y1 + (u2 - u1) * Y0) / den;
      const dx = (u0 * (v2 * X1 - v1 * X2) + v0 * (u1 * X2 - u2 * X1) + (u2 * v1 - u1 * v2) * X0) / den;
      const dy = (u0 * (v2 * Y1 - v1 * Y2) + v0 * (u1 * Y2 - u2 * Y1) + (u2 * v1 - u1 * v2) * Y0) / den;
      g.save();
      // grow each triangle a hair past its edges so no hairline gaps show between pieces
      const mx = (X0 + X1 + X2) / 3, my = (Y0 + Y1 + Y2) / 3, grow = p => [p[0] + Math.sign(p[0] - mx) * 0.6, p[1] + Math.sign(p[1] - my) * 0.6];
      const [p0, p1, p2] = [grow([X0, Y0]), grow([X1, Y1]), grow([X2, Y2])];
      g.beginPath(); g.moveTo(...p0); g.lineTo(...p1); g.lineTo(...p2); g.closePath(); g.clip();
      g.setTransform(m11, m12, m21, m22, dx, dy);
      const us = [u0, u1, u2], vs = [v0, v1, v2], sx = Math.max(0, Math.floor(Math.min(...us)) - 2), sy = Math.max(0, Math.floor(Math.min(...vs)) - 2);
      const sw = Math.min(src.width, Math.ceil(Math.max(...us)) + 2) - sx, sh = Math.min(src.height, Math.ceil(Math.max(...vs)) + 2) - sy;
      if (sw > 0 && sh > 0) g.drawImage(src, sx, sy, sw, sh, sx, sy, sw, sh);   // only the bit this triangle needs
      g.restore();
    };
    for (let j = 0; j < cells; j++) for (let i = 0; i < cells; i++) {
      tri(at(i, j), at(i + 1, j), at(i + 1, j + 1));
      tri(at(i, j), at(i + 1, j + 1), at(i, j + 1));
    }
    return dst;
  }

  // ---------- document scanning with the phone's camera ----------

  // findPage: the four corners of a sheet of paper in a photo, or null. Paper is almost always brighter than what
  // it lies on, so: split bright from dark (Otsu's method picks the split for this photo), take the bright patch
  // in the middle (or the biggest one), and its four extremes are the corners. img should be small (~400 px).
  function findPage(img) {
    const { data, width: w, height: h } = img, n = w * h;
    let L = new Float32Array(n);
    for (let i = 0, j = 0; j < n; i += 4, j++) L[j] = luma(data[i], data[i + 1], data[i + 2]);
    L = boxBlur(L, w, h, 2);
    const hist = new Float64Array(256);
    for (let j = 0; j < n; j++) hist[L[j] | 0]++;
    let sum = 0; for (let v = 0; v < 256; v++) sum += v * hist[v];
    let wb = 0, sb = 0, best = -1, t = 128;
    for (let v = 0; v < 256; v++) {   // Otsu: the split that separates the two groups best
      wb += hist[v]; if (!wb) continue;
      const wf = n - wb; if (!wf) break;
      sb += v * hist[v];
      const d = sb / wb - (sum - sb) / wf, between = wb * wf * d * d;
      if (between > best) { best = between; t = v; }
    }
    // try a bright page on a darker surface first, then a dark page on a bright one (a white table)
    return pageFrom(L, w, h, v => v > t) || pageFrom(L, w, h, v => v <= t);
  }

  // pageFrom: the patch of pixels passing `on` that sits in the middle (or the biggest one), as four corners;
  // null if it's really the background (it runs along most of the photo's edges) or too small to be a page
  function pageFrom(L, w, h, on) {
    const n = w * h, label = new Int32Array(n).fill(-1), sizes = [], stack = new Int32Array(n);
    for (let start = 0; start < n; start++) {   // group touching pixels into patches
      if (label[start] >= 0 || !on(L[start])) continue;
      const id = sizes.length; let top = 0, size = 0;
      stack[top++] = start; label[start] = id;
      while (top) {
        const j = stack[--top], x = j % w; size++;
        const nb = [x > 0 ? j - 1 : -1, x < w - 1 ? j + 1 : -1, j - w, j + w];
        for (const k of nb) if (k >= 0 && k < n && label[k] < 0 && on(L[k])) { label[k] = id; stack[top++] = k; }
      }
      sizes.push(size);
    }
    if (!sizes.length) return null;
    const mid = label[(h >> 1) * w + (w >> 1)];
    const pick = mid >= 0 && sizes[mid] > n * 0.1 ? mid : sizes.indexOf(Math.max(...sizes));
    let edge = 0;
    for (let x = 0; x < w; x++) edge += (label[x] === pick) + (label[(h - 1) * w + x] === pick);
    for (let y = 0; y < h; y++) edge += (label[y * w] === pick) + (label[y * w + w - 1] === pick);
    if (edge > (w + h) * 2 * 0.5) return null;   // runs along most of the edges: that's the table, not the paper
    let tl = [0, 0, Infinity], tr = [0, 0, -Infinity], br = [0, 0, -Infinity], bl = [0, 0, Infinity];
    for (let j = 0; j < n; j++) {
      if (label[j] !== pick) continue;
      const x = j % w, y = (j / w) | 0;
      if (x + y < tl[2]) tl = [x, y, x + y];
      if (x + y > br[2]) br = [x, y, x + y];
      if (x - y > tr[2]) tr = [x, y, x - y];
      if (x - y < bl[2]) bl = [x, y, x - y];
    }
    const q = [tl, tr, br, bl].map(([x, y]) => [x, y]);
    const area = Math.abs(q.reduce((a, [x, y], i) => { const [x2, y2] = q[(i + 1) % 4]; return a + x * y2 - x2 * y; }, 0)) / 2;
    return area > n * 0.15 && area < n * 0.995 ? q : null;
  }

  // pageSize: how big the flattened page should be, from its corners (snapped to A4's shape when it's close)
  function pageSize(quad, longest = 2339) {
    const d = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);
    const w = (d(quad[0], quad[1]) + d(quad[3], quad[2])) / 2, h = (d(quad[0], quad[3]) + d(quad[1], quad[2])) / 2;
    let r = w / h;
    const a4 = 210 / 297;
    if (Math.abs(r / a4 - 1) < 0.14) r = a4; else if (Math.abs(r * a4 - 1) < 0.14) r = 1 / a4;
    const long = Math.round(Math.min(longest, Math.max(w, h) * 1.25));
    return r < 1 ? { w: Math.round(long * r), h: long } : { w: long, h: Math.round(long / r) };
  }

  // blackWhite: crisp black writing on white paper, even with a shadow across the page: each pixel is compared
  // with its own neighbourhood rather than with the whole page
  function blackWhite(img) {
    const { data, width: w, height: h } = img, n = w * h, L = new Float32Array(n);
    for (let i = 0, j = 0; j < n; i += 4, j++) L[j] = luma(data[i], data[i + 1], data[i + 2]);
    const local = boxBlur(L, w, h, Math.max(8, Math.round(Math.min(w, h) / 28)));
    for (let i = 0, j = 0; j < n; i += 4, j++) { const v = L[j] < local[j] - 10 ? 0 : 255; data[i] = data[i + 1] = data[i + 2] = v; }
    return img;
  }

  root.SakuraDevelop = { ADJUST, FILTERS, LOOKS, blank, develop, developFast, resolve, autoFix, boxBlur, squareToQuad, applyH, perspectiveQuad, warp, warpQuad, findPage, pageSize, blackWhite };
})(typeof window !== 'undefined' ? window : globalThis);
