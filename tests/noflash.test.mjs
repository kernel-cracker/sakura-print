// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Changing a print setting never blanks the page you are looking at (phone-speed WiFi).
export const needsPrinter = true;

export default async function (t) {
  const p = await t.launch();
  await p.send('Network.emulateNetworkConditions', { offline: false, latency: 60, downloadThroughput: 2e6, uploadThroughput: 1e6 });
  await p.go(t.base + '/#/text'); await p.waitFor('#txt');
  await p.eval(`document.querySelector('#txt').value = 'hello\\n'.repeat(200); document.querySelector('#txt').dispatchEvent(new Event('input')); 0`);
  await p.tap('#next'); await p.waitFor('.pvpage img'); await p.idle(600);
  await p.tap('.pvacts [data-x=set]'); await p.sleep(400);
  const visibleBlank = () => p.eval(`(() => { const t = document.querySelector('#pvtrack'), i = Math.round(t.scrollLeft / t.clientWidth);
    const img = t.children[i] && t.children[i].querySelector('img'); return !img || !img.complete || !img.naturalWidth; })()`);
  for (const [label, act] of [['text size L', () => p.tap('#pvtsize [data-v="16"]', 0, false)], ['sides Both', () => p.tap('#pvsides [data-v=duplex]', 0, false)], ['big title', () => p.tap('#pvthead', 0, false)]]) {
    const samples = [];
    await act();
    for (let i = 0; i < 25; i++) { samples.push(await visibleBlank() ? 'X' : '.'); await p.sleep(60); }
    t.ok(!samples.includes('X'), `${label}: the page on screen never goes blank while the new preview loads (${samples.join('')})`);
  }
  await p.close();
}
