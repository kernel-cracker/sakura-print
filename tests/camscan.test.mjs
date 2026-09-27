// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Scanning a page with the phone's camera: the page is found, straightened, cleaned up, and added.
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/scan'); await p.waitFor('#camScan');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  // a photo of a tilted white page with lines of writing, lying on a dark table (1200 x 1600)
  const quad = [[330, 250], [960, 380], [850, 1400], [200, 1260]];
  const jpg = await p.eval(`(async () => {
    const c = new OffscreenCanvas(1200, 1600), g = c.getContext('2d'), q = ${JSON.stringify(quad)};
    g.fillStyle = 'rgb(88,70,60)'; g.fillRect(0, 0, 1200, 1600);
    g.beginPath(); q.forEach(([x, y], i) => i ? g.lineTo(x, y) : g.moveTo(x, y)); g.closePath(); g.fillStyle = 'rgb(236,234,228)'; g.fill();
    g.save(); g.clip();
    const H = SakuraDevelop.squareToQuad(q);   // writing, drawn in the page's own (tilted) space
    g.strokeStyle = 'rgb(30,30,40)'; g.lineWidth = 7;
    for (let k = 1; k <= 8; k++) { const v = k / 10; g.beginPath(); for (let u = 0.12; u <= 0.88; u += 0.02) { const [x, y] = SakuraDevelop.applyH(H, u, v); u === 0.12 ? g.moveTo(x, y) : g.lineTo(x, y); } g.stroke(); }
    g.restore();
    const b = await c.convertToBlob({ type: 'image/jpeg', quality: 0.9 }), a = new Uint8Array(await b.arrayBuffer());
    let s = ''; for (const x of a) s += String.fromCharCode(x); return btoa(s);
  })()`.replace('(async () => {', `(async () => { await loadScript('develop.js', 'SakuraDevelop');`));
  const file = join(t.shots, 'page-photo.jpg');
  writeFileSync(file, Buffer.from(jpg, 'base64'));

  await p.tap('#camScan', 0, false);
  await p.waitFor('.cammodal [data-x=choose]');
  t.ok(await p.eval(`document.querySelector('.cammodal [data-x=done]').disabled`), 'Done is off until there is a page');
  await p.chooseFiles([file]);
  await p.tap('.cammodal [data-x=choose]', 0, false);
  await p.waitFor('.camh', 10000); await p.sleep(300);
  const found = await p.eval(`(() => { const s = document.querySelector('#camsvg'), c = document.querySelector('#camc'), k = c.clientWidth / 1200;
    return [...s.querySelectorAll('.camh')].map(h => [+h.getAttribute('cx') / k, +h.getAttribute('cy') / k]); })()`);
  const off = found.map((f, i) => Math.hypot(f[0] - quad[i][0], f[1] - quad[i][1]));
  t.ok(off.every(d => d < 40), `finds the page's corners by itself (off by ${off.map(d => Math.round(d)).join(', ')} px of a 1600 px photo)`);
  t.ok(await p.eval(`document.querySelector('#camhint').textContent.startsWith('Found the page')`), 'says it found the page');
  // drag a corner a little and back: the handles follow the finger
  const h0 = await p.eval(`(() => { const r = document.querySelectorAll('.camh')[0].getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
  await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [h0] });
  for (let i = 1; i <= 6; i++) { await p.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: h0.x + i * 5, y: h0.y + i * 5 }] }); await p.sleep(16); }
  await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(200);
  const moved = await p.eval(`(() => { const r = document.querySelectorAll('.camh')[0].getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
  t.ok(Math.abs(moved.x - h0.x - 30) < 4 && Math.abs(moved.y - h0.y - 30) < 4, 'a corner can be dragged with a finger');
  await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [moved] });
  for (let i = 1; i <= 6; i++) { await p.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: moved.x - i * 5, y: moved.y - i * 5 }] }); await p.sleep(16); }
  await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(200);
  await p.shot(t.shots + '/camscan-corners.png');

  await p.tap('.cammodal [data-x=use]', 0, false);
  await p.waitFor('.campage img', 15000); await p.sleep(300);
  t.ok(await p.eval(`document.querySelector('.cammodal [data-x=done]').textContent`) === 'Done (1)', 'the page is added, and Done shows how many');
  // second page, in black & white
  await p.tap('.cammodal [data-mode=bw]', 0, false).catch(() => {});
  await p.chooseFiles([file]);
  await p.tap('.cammodal [data-x=choose]', 0, false);
  await p.waitFor('.camh', 10000); await p.sleep(300);
  await p.tap('.cammodal [data-mode=bw]', 0, false);
  await p.tap('.cammodal [data-x=use]', 0, false);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (document.querySelectorAll('.campage').length === 2) { clearInterval(t); r(); } }, 50); })`);
  await p.tap('.cammodal [data-x=done]', 0, false);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('.cammodal') && S.scans.length) { clearInterval(t); r(); } }, 50); })`);
  t.ok(await p.eval(`S.scans.length`) === 2, 'both pages are on the Scan screen, ready to save or print');

  // check the straightened pages
  const check = await p.eval(`(async () => {
    const out = [];
    for (const f of S.scans) {
      const bmp = await createImageBitmap(await (await fetch('/api/file/' + f.id)).blob());
      const c = new OffscreenCanvas(bmp.width, bmp.height), g = c.getContext('2d'); g.drawImage(bmp, 0, 0);
      const d = g.getImageData(0, 0, bmp.width, bmp.height).data, L = (x, y) => { const i = (Math.floor(y) * bmp.width + Math.floor(x)) * 4; return (d[i] + d[i + 1] + d[i + 2]) / 3; };
      let dark = 0, grey = 0, n = 0;   // along each line of writing, and between them
      // each line of writing should be found within 2% of where it belongs, all the way across: straight, level lines
      for (let k = 1; k <= 8; k++) { const y0 = bmp.height * k / 10; for (let x = bmp.width * 0.2; x < bmp.width * 0.8; x += 7) { n++; let lo = 255; for (let y = y0 - bmp.height * 0.02; y <= y0 + bmp.height * 0.02; y++) lo = Math.min(lo, L(x, y)); if (lo < 110) dark++; } }
      let paper = 0, m = 0; for (let k = 1; k <= 8; k++) { const y = bmp.height * (k / 10 + 0.05); for (let x = bmp.width * 0.2; x < bmp.width * 0.8; x += 7) { m++; if (L(x, y) > 200) paper++; } }
      for (let i = 0; i < d.length; i += 4 * 97) if (d[i] > 20 && d[i] < 235) grey++;
      out.push({ w: bmp.width, h: bmp.height, onLines: dark / n, paperWhite: paper / m, midTones: grey });
    }
    return out;
  })()`);
  const [a, b] = check;
  t.ok(Math.abs(a.w / a.h - 210 / 297) < 0.01, `the page comes out A4-shaped (${a.w} x ${a.h})`);
  t.ok(a.onLines > 0.8 && a.paperWhite > 0.9, `straightened: the lines of writing run straight across the page (${Math.round(a.onLines * 100)}% on the lines, ${Math.round(a.paperWhite * 100)}% clean paper between)`);
  t.ok(b.midTones < 40, `Black & white is really black and white (${b.midTones} grey samples)`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
