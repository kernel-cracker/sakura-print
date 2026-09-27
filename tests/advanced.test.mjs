// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Advanced: what's going on under the hood, for fixing problems. Only on the computer. The report that can be
// copied for a bug report never contains secrets.
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
export const needsPrinter = true;

export default async function (t) {
  // something secret to look for: a PIN (so its hash and salt are in the settings) and a logged-in session
  await t.resetSettings({ requirePin: true });
  const port = new URL(t.base).port;
  const req = (path, host, body) => new Promise(res => {
    const r = http.request({ host: '127.0.0.1', port, path, method: body ? 'POST' : 'GET', headers: { Host: host + ':' + port, 'Content-Type': 'application/json' } }, x => {
      let b = ''; x.on('data', d => b += d); x.on('end', () => res({ code: x.statusCode, body: b, cookie: (x.headers['set-cookie'] || [])[0] }));
    });
    if (body) r.write(JSON.stringify(body)); r.end();
  });
  await req('/api/settings', 'localhost', { addPin: { label: 'Mom', pin: '482915' } });
  await req('/api/login', 'sakura-phone.test', { pin: '482915' });
  const settings = JSON.parse(readFileSync(join(t.work, 'config', 'sakuraprint', 'settings.json'), 'utf8'));
  const secrets = [...settings.pins.flatMap(x => [x.hash, x.salt]), ...settings.sessions.map(x => x.hash), '482915'].filter(Boolean);
  t.ok(secrets.length >= 3, `there are secrets to protect (${secrets.length})`);

  const p = await t.launch();
  await p.go(t.base + '/#/advanced'); await p.waitFor('.adv-section', 20000);
  const text = await p.eval(`document.querySelector('#app').innerText`);
  const version = await p.eval(`S.info.version`);
  t.ok(text.includes(version) && /built/i.test(text), 'it says which version this is and how it was built');
  t.ok(/CUPS/.test(text) && text.includes(await p.eval(`S.info.printers[0]`)), 'the print queues, with their drivers');
  t.ok(/Scanners/.test(text) && /Print from any app/.test(text) && /Bonjour/.test(text), 'scanners, and print from any app with Bonjour');
  t.ok(await p.eval(`document.querySelectorAll('.adv-log li').length`) > 0, 'what the server has been doing (its log)');
  t.ok(/PIN added for Mom/.test(await p.eval(`document.querySelector('.adv-log').textContent`)), 'including changes to PINs (by name, never the PIN)');
  await p.shot(t.shots + '/advanced.png');

  const report = (await req('/api/debug/report', 'localhost')).body;
  t.ok(report.includes(version) && /CUPS/.test(report) && report.length > 300, `a report to copy for a bug report (${report.length} characters)`);
  const leaked = secrets.filter(s => report.includes(s));
  t.ok(leaked.length === 0, `the report has no secrets in it (no PIN, PIN hash, salt or session)${leaked.length ? ': LEAKED ' + leaked.length : ''}`);
  t.ok(!/sakura-phone\.test|192\.168\.\d+\.\d+/.test(report), 'nor the addresses of phones or of this computer on the WiFi');

  // phones: not there, and the API says no
  t.ok((await req('/api/debug', 'sakura-phone.test')).code >= 401 && (await req('/api/debug/report', 'sakura-phone.test')).code >= 401, 'phones can\'t get any of it');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
