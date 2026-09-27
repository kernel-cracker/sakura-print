// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// The notifications bell: a count of what needs you, a list that says it in plain words, and a tap does it.
// A double-sided print sent from another app waits for its flip (made with CUPS's ipptool, as a phone would), and
// phone access about to switch itself off offers to stay on.
import { execFileSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
export const needsPrinter = true;

export default async function (t) {
  if (!t.which('ipptool')) { t.ok(true, 'no ipptool here: skipping'); return; }
  await t.resetSettings({ phonesOff: true, phonesTill: new Date(Date.now() + 10 * 60000).toISOString() });
  const port = new URL(t.base).port;
  const f = join(t.work, 'notice-job.test');
  writeFileSync(f, `{
  OPERATION Print-Job
  GROUP operation-attributes-tag
  ATTR charset attributes-charset utf-8
  ATTR naturalLanguage attributes-natural-language en
  ATTR uri printer-uri $uri
  ATTR name job-name "Homework"
  ATTR mimeMediaType document-format application/pdf
  GROUP job-attributes-tag
  ATTR keyword sides two-sided-long-edge
  FILE ${t.fixture('twopage.pdf')}
  STATUS successful-ok
}
`);
  execFileSync('ipptool', ['-t', `ipp://localhost:${port}/ipp/print`, f], { encoding: 'utf8', timeout: 30000 });

  const p = await t.launch();
  await p.go(t.base + '/#/'); await p.waitFor('#bell');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  let count = '';
  for (let e = 0; e < 15000 && !/^[1-9]/.test(count); e += 300) { await p.sleep(300); count = await p.eval(`(document.querySelector('#bellcount') || {}).textContent || ''`); }
  t.ok(/^[1-9]/.test(count), `the bell shows how many things need you (${count})`);
  await p.tap('#bell');
  await p.waitFor('#notices .notice');
  const list = await p.eval(`[...document.querySelectorAll('#notices .notice')].map(n => n.querySelector('.n-title').textContent.trim())`);
  t.ok(list[0] && /Turn the paper over: “Homework”/.test(list[0]), `the flip comes first (${list.join(' | ')})`);
  t.ok(list.some(x => /Phone access turns off soon/.test(x)), 'and phone access ending soon is there too');
  await p.shot(t.shots + '/notices-panel.png');

  // "Keep on for an hour" does it, and that notice goes
  await p.eval(`[...document.querySelectorAll('#notices .notice')].find(n => /turns off soon/.test(n.textContent)).querySelector('[data-n-action]').click(); 0`);
  let gone = false;
  for (let e = 0; e < 5000 && !gone; e += 200) { await p.sleep(200); gone = await p.eval(`![...document.querySelectorAll('#notices .notice')].some(n => /turns off soon/.test(n.textContent))`); }
  t.ok(gone, '"Keep on for an hour": done, and the notice goes');
  const line = await p.eval(`(document.querySelector('.phoneaccess .pa-line') || {}).textContent || ''`);
  t.ok(/(59|60) min left/.test(line), `phone access now has an hour (${line.replace(/\s+/g, ' ').trim()})`);

  // tapping the flip opens the guide for that print
  await p.eval(`document.querySelector('#notices .notice').click(); 0`);
  await p.waitFor('#side2', 10000);
  t.ok(await p.eval(`/Time to flip the paper/.test(document.querySelector('.modal').textContent)`), 'tapping the flip opens its guide');
  await p.tap('#side2');
  let done = false;
  for (let e = 0; e < 10000 && !done; e += 300) { await p.sleep(300); done = await p.eval(`/All done/.test((document.querySelector('.modal') || {}).textContent || '')`); }
  t.ok(done, 'and the other side prints');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  let after = 'x';
  for (let e = 0; e < 8000 && after !== ''; e += 300) { await p.sleep(300); after = await p.eval(`(document.querySelector('#bellcount') || {}).textContent || ''`); }
  t.ok(after === '', `nothing left: the bell's count goes (${after || 'none'})`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
