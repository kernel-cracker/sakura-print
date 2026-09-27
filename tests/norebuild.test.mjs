// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Tapping a choice updates it in place: no screen rebuilds, no scroll jumps (phone-speed WiFi).

export default async function (t) {
  // and record what visibly happens: scroll jumps, the screen being rebuilt, pictures going blank.
  const p = await t.launch();
  // a typical phone on home WiFi: 60 ms round trip, ~2 MB/s
  await p.send('Network.emulateNetworkConditions', { offline: false, latency: 60, downloadThroughput: 2e6, uploadThroughput: 1e6 });

  const look = () => p.eval(`({ y: Math.round(scrollY), same: document.querySelector('#app').firstElementChild === window.__first,
    blankImgs: [...document.querySelectorAll('#app img, .pvpage img')].filter(i => !i.complete || !i.naturalWidth).length })`);
  async function tapWatch(label, sel, nth = 0) {
    await p.eval(`(() => { const e = document.querySelectorAll(${JSON.stringify(sel)})[${nth}]; e.scrollIntoView({ block: 'center' }); window.__first = document.querySelector('#app').firstElementChild; })()`);
    const before = await look();
    await p.tap(sel, nth, false);
    const r = []; for (let i = 0; i < 4; i++) { r.push(await look()); await p.sleep(120); }
    t.ok(r[0].same && r.every(x => Math.abs(x.y - before.y) <= 2) && r.every(x => x.blankImgs === 0),
      `${label}: updates in place (rebuilt: ${!r[0].same}, scroll ${before.y} -> ${r.map(x => x.y).join(',')}, blank pictures ${r.map(x => x.blankImgs).join(',')})`);
  }

  await p.go(t.base + '/#/text'); await p.waitFor('#app .seg, #app textarea'); await p.idle();
  const segs = await p.eval(`[...document.querySelectorAll('#app [data-seg] button, #app [data-tog]')].length`);
  t.ok(segs >= 5, `the Text screen has its choices (${segs})`);
  await p.eval(`document.querySelector('#app textarea') && (document.querySelector('#app textarea').value = 'hello'); 0`);
  if (segs) { await tapWatch('text: 2nd option', '#app [data-seg] button', 1); await tapWatch('text: a toggle', '#app [data-tog]', 0); }
  await p.shot(t.shots + '/s3-text.png');

  await p.go(t.base + '/#/scan'); await p.idle(800);
  if (await p.eval(`!!document.querySelector('#app [data-seg] button')`)) {
    await tapWatch('scan: option', '#app [data-seg] button', 1);
    await tapWatch('scan: another option', '#app [data-seg] button', 3);
  }
  await p.shot(t.shots + '/s3-scan.png');

  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
