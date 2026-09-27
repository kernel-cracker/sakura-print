// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Phone access: one control, the same on the home screen and in Settings. Off, always on, or on for 1 or 3 hours
// (then off by itself). What it says is always true: it follows the clock without reloading, and a phone really
// is let in or kept out.
import http from 'node:http';

export default async function (t) {
  const port = new URL(t.base).port;
  const phone = path => new Promise(res => {   // the server treats any request not addressed to this computer as a phone
    http.get({ host: '127.0.0.1', port, path, headers: { Host: 'sakura-phone.test:' + port } }, r => {
      let body = ''; r.on('data', d => body += d); r.on('end', () => res({ code: r.statusCode, body }));
    });
  });
  await t.resetSettings();   // from the start: phone access always on (another test may have changed it)
  const p = await t.launch();
  const read = () => p.eval(`(() => { const c = document.querySelector('.phoneaccess'); if (!c) return null;
    return { line: c.querySelector('.pa-line').textContent.replace(/\\s+/g, ' ').trim(), on: c.querySelector('[data-pa=toggle]').getAttribute('aria-checked'),
      chip: (c.querySelector('[data-pa-chip].on') || {}).dataset?.paChip || null }; })()`);
  const waitRead = async (want, ms = 5000) => { let s; for (let e = 0; e < ms; e += 200) { s = await read(); if (s && want(s)) return s; await p.sleep(200); } return s; };

  await p.go(t.base + '/#/'); await p.waitFor('.phoneaccess');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  let s = await read();
  t.ok(s.on === 'true' && s.chip === 'always' && /Phones can use Sakura Print/.test(s.line), `home: on, always (${s.line})`);
  t.ok((await phone('/')).code === 200, 'and a phone can open the app');

  await p.tap('.phoneaccess [data-pa=toggle]');
  s = await waitRead(x => x.on === 'false');
  t.ok(s.on === 'false' && /can't use Sakura Print/.test(s.line) && s.chip === null, `the switch turns it off (${s.line})`);
  const off = await phone('/');
  t.ok(off.code === 403 && /Phone access is off/.test(off.body), 'a phone now gets a friendly "phone access is off" page');
  t.ok((await phone('/api/info')).code === 403 && (await phone('/app.js')).code === 403, 'and nothing else');

  await p.tap('.phoneaccess [data-pa=toggle]');
  s = await waitRead(x => x.on === 'true');
  await p.tap('.phoneaccess [data-pa-chip="1h"]');
  s = await waitRead(x => x.chip === '1h');
  t.ok(s.on === 'true' && s.chip === '1h' && /until \d{1,2}:\d\d/.test(s.line) && /(59|60) min left/.test(s.line), `on for 1 hour: says until when, and how long is left (${s.line})`);
  t.ok((await phone('/')).code === 200, 'a phone can open the app');

  // Settings shows the very same state, with the same control
  await p.go(t.base + '/#/settings'); await p.waitFor('.phoneaccess');
  const s2 = await read();
  t.ok(JSON.stringify(s2) === JSON.stringify(s), `Settings says the same as the home screen (${s2.line})`);
  await p.tap('.phoneaccess [data-pa-chip="3h"]');
  s = await waitRead(x => x.chip === '3h');
  t.ok(/(179|180) min left/.test(s.line), `3 hours (${s.line})`);
  await p.tap('.phoneaccess [data-pa-chip="always"]');
  s = await waitRead(x => x.chip === 'always');
  t.ok(/Phones can use Sakura Print/.test(s.line) && !/until/.test(s.line), `back to always (${s.line})`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();

  // time runs out while the screen is open: it says so by itself, and phones are kept out
  await t.resetSettings({ phonesOff: true, phonesTill: new Date(Date.now() + 25000).toISOString() });
  const q = await t.launch();
  await q.go(t.base + '/#/'); await q.waitFor('.phoneaccess');
  await q.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const before = await q.eval(`document.querySelector('.phoneaccess .pa-line').textContent`);
  t.ok(/until/.test(before) && /1 min left|less than a minute/.test(before), `on for a few more seconds (${before.replace(/\s+/g, ' ').trim()})`);
  let after = before;
  for (let e = 0; e < 45000 && /until/.test(after); e += 1000) { await q.sleep(1000); after = await q.eval(`document.querySelector('.phoneaccess .pa-line').textContent`); }
  t.ok(/can't use Sakura Print/.test(after), `when the time is up, the screen says it's off without being reloaded (${after.replace(/\s+/g, ' ').trim()})`);
  t.ok((await phone('/')).code === 403, 'and phones are kept out');
  await q.close();
}
