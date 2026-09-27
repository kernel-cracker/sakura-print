// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Found by a real person: the phone access switch "didn't work". It redrew itself every second, and a press that
// lasts across a redraw (a normal press: finger down, then up) never became a tap. Here presses last a second and a
// half, with a mouse and with a finger, on the switch and the time buttons.
import http from 'node:http';
export default async function (t) {
  await t.resetSettings();
  const p = await t.launch();
  await p.go(t.base + '/#/'); await p.waitFor('.phoneaccess');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const state = () => p.eval(`document.querySelector('.phoneaccess [data-pa=toggle]').getAttribute('aria-checked')`);
  // scrolled into view first, as a person would (the phone card is below the fold): a press off screen hits nothing
  const centre = sel => p.eval(`(() => { const b = document.querySelector('${sel}'); b.scrollIntoView({ block: 'center' }); const r = b.getBoundingClientRect();
    return { x: r.x + r.width / 2, y: r.y + r.height / 2, hit: document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === b || b.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)) }; })()`);
  const holdMouse = async sel => {
    const c = await centre(sel);
    if (!c.hit) throw new Error(`${sel} is covered by something else`);
    await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: c.x, y: c.y });
    await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: c.x, y: c.y, button: 'left', clickCount: 1 });
    await p.sleep(1500);
    await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: c.x, y: c.y, button: 'left', clickCount: 1 });
  };
  const holdFinger = async sel => {
    const c = await centre(sel);
    if (!c.hit) throw new Error(`${sel} is covered by something else`);
    await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: c.x, y: c.y }] });
    await p.sleep(1500);
    await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  };
  const waitFor = async want => { for (let e = 0; e < 4000; e += 100) { if (await state() === want) return true; await p.sleep(100); } return false; };

  t.ok(await state() === 'true', 'phones on to start with');
  await holdMouse('.phoneaccess [data-pa=toggle]');
  t.ok(await waitFor('false'), 'a slow click on the switch turns phones off');
  await holdFinger('.phoneaccess [data-pa=toggle]');
  t.ok(await waitFor('true'), 'a slow tap with a finger turns them back on');
  await holdFinger('.phoneaccess [data-pa-chip="1h"]');
  let chip = '';
  for (let e = 0; e < 4000 && chip !== '1h'; e += 100) { await p.sleep(100); chip = await p.eval(`(document.querySelector('.phoneaccess [data-pa-chip].on') || {}).dataset?.paChip || ''`); }
  t.ok(chip === '1h', `a slow tap on "For 1 hour" works (${chip || 'nothing chosen'})`);
  // the real report: with "For 1 hour" on (a countdown ticking every second), the switch couldn't lock phones out again
  await p.sleep(1200);
  await holdFinger('.phoneaccess [data-pa=toggle]');
  t.ok(await waitFor('false'), 'during "For 1 hour", a slow tap on the switch locks phones out again');
  const port = new URL(t.base).port;
  const code = await new Promise(res => http.request({ host: '127.0.0.1', port, path: '/', headers: { Host: 'sakura-phone.test:' + port } }, x => { x.resume(); res(x.statusCode); }).end());
  t.ok(code === 403, `and a phone really is turned away (${code})`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
