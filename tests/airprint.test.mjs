// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Print from any app. The computer prints with CUPS's ipptool (a real IPP client): a double-sided job waits on
// the home screen for the flip. Phones are pretend (a tiny IPP client below, saying which phone it is with a test
// header): a phone that logged in with a PIN prints straight away, a stranger's print waits for someone to allow
// it, and can't touch other people's prints.
import { execFileSync } from 'node:child_process';
import { writeFileSync, readdirSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import http from 'node:http';
export const needsPrinter = true;

const MOM = 'aa:bb:cc:00:00:01', STRANGER = 'aa:bb:cc:00:00:66';

// ---- a tiny IPP client ----
function ippRequest(op, id, attrs, doc) {
  const parts = [Buffer.from([2, 0, op >> 8, op & 255, 0, 0, 0, id])];
  const attr = (tag, name, value) => {
    const v = typeof value === 'number' ? Buffer.from([value >>> 24 & 255, value >>> 16 & 255, value >>> 8 & 255, value & 255]) : Buffer.from(value);
    const n = Buffer.from(name);
    parts.push(Buffer.from([tag, n.length >> 8, n.length & 255]), n, Buffer.from([v.length >> 8, v.length & 255]), v);
  };
  parts.push(Buffer.from([1]));   // operation attributes
  attr(0x47, 'attributes-charset', 'utf-8');
  attr(0x48, 'attributes-natural-language', 'en');
  attr(0x45, 'printer-uri', 'ipp://sakura-phone.test/ipp/print');
  for (const [tag, name, value] of attrs) attr(tag, name, value);
  parts.push(Buffer.from([3]));
  if (doc) parts.push(doc);
  return Buffer.concat(parts);
}
function readReply(buf) {   // status, and the first value of each attribute
  const out = { status: buf.readUInt16BE(2) };
  let i = 8;
  while (i < buf.length) {
    const tag = buf[i++]; if (tag === 3) break; if (tag < 0x10) continue;
    const nl = buf.readUInt16BE(i); const name = buf.subarray(i + 2, i + 2 + nl).toString(); i += 2 + nl;
    const vl = buf.readUInt16BE(i); const v = buf.subarray(i + 2, i + 2 + vl); i += 2 + vl;
    if (name && !(name in out)) out[name] = (tag >= 0x21 && tag <= 0x23 && vl === 4) ? v.readInt32BE(0) : v.toString();
  }
  return out;
}

export default async function (t) {
  if (!t.which('ipptool')) { t.ok(true, 'no ipptool here (CUPS client tools): skipping'); return; }
  const port = new URL(t.base).port;
  const phone = (mac, path, body, headers = {}) => new Promise((res, rej) => {
    const req = http.request({ host: '127.0.0.1', port, path, method: body ? 'POST' : 'GET',
      headers: { Host: 'sakura-phone.test:' + port, 'X-Test-Mac': mac, ...headers, ...(body ? { 'Content-Length': body.length } : {}) } }, r => {
      const chunks = []; r.on('data', d => chunks.push(d)); r.on('end', () => res({ code: r.statusCode, headers: r.headers, body: Buffer.concat(chunks) }));
    });
    req.on('error', rej); if (body) req.write(body); req.end();
  });
  let rid = 1;
  const phonePrint = async (mac, title) => readReply((await phone(mac, '/ipp/print', ippRequest(0x0002, rid++,
    [[0x42, 'job-name', title], [0x42, 'requesting-user-name', mac === STRANGER ? 'Neighbour' : 'Mom'], [0x49, 'document-format', 'application/pdf']],
    readFileSync(t.fixture('twopage.pdf'))), { 'Content-Type': 'application/ipp' })).body);
  const phoneJob = async (mac, op, id) => readReply((await phone(mac, '/ipp/print', ippRequest(op, rid++, id ? [[0x21, 'job-id', id]] : []),
    { 'Content-Type': 'application/ipp' })).body);
  const printed = () => readdirSync(t.printed).length;
  const waitFor = async (f, ms = 5000) => { for (let e = 0; e < ms; e += 100) { if (await f()) return true; await t.sleep(100); } return false; };

  // the computer: ipptool, a two-sided print
  const job = status => `{
  NAME "Print-Job, both sides"
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
  STATUS ${status}
}
`;
  const send = status => {
    const f = join(t.work, 'job.test'); writeFileSync(f, job(status));
    try { return /\[PASS\]/.test(execFileSync('ipptool', ['-t', `ipp://localhost:${port}/ipp/print`, f], { encoding: 'utf8', timeout: 30000 })); }
    catch (e) { console.log('    ' + (e.stdout || e.message).split('\n').filter(l => /FAIL|EXPECTED|GOT/.test(l)).join(' | ')); return false; }
  };

  await t.resetSettings();
  rmSync(join(t.dataDir, 'history'), { recursive: true, force: true });   // this test counts its own prints
  const p = await t.launch();
  await p.go(t.base + '/#/settings'); await p.waitFor('[data-tog=airprint]');
  t.ok(await p.eval(`document.querySelector('[data-tog=airprint]').classList.contains('on')`), 'Settings: "Print from any app" is on by default');
  await p.tap('[data-tog=airprint]'); await p.sleep(500);
  t.ok(send('server-error-not-accepting-jobs') && printed() === 0, 'switched off: prints are turned away');
  await p.tap('[data-tog=airprint]'); await p.sleep(500);
  t.ok(/logged in here with a PIN print straight away/.test(await p.eval(`document.querySelector('[data-tog=airprint]').parentElement.nextElementSibling.textContent`)),
    'and back on: it says only phones with a PIN print straight away');

  t.ok(send('successful-ok'), 'the computer sends a double-sided job: taken');
  t.ok(await waitFor(() => printed() === 1), `side 1 went to the printer (${printed()} "printed")`);
  await p.go(t.base + '/#/'); await p.waitFor('.ftile');
  t.ok(await waitFor(() => p.eval(`/Homework/.test(document.querySelector('.flipwait')?.textContent || '')`)), 'the home screen says "Homework" is waiting for the flip');
  await p.shot(t.shots + '/airprint-flip.png');
  await p.tap('.flipwait [data-flip]'); await p.waitFor('#side2');
  await p.tap('#side2');
  t.ok(await waitFor(() => p.eval(`/All done/.test(document.querySelector('.modal')?.textContent || '')`), 9000), 'Flip now → Print the other side: all done');
  t.ok(printed() === 2, 'side 2 went to the printer too');
  const hist = join(t.dataDir, 'history');
  t.ok(existsSync(hist) && readdirSync(hist).length === 1, 'and it\'s in Print again');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);

  // a stranger's phone: its print waits, with a picture, until someone decides
  let r = await phonePrint(STRANGER, 'Mystery');
  t.ok(r.status === 0 && r['job-state'] === 4, `a stranger's print is taken but held (job-state ${r['job-state']})`);
  const held = r['job-id'];
  await t.sleep(300);
  t.ok(printed() === 2, 'nothing printed yet');
  await p.go(t.base + '/#/settings'); await p.go(t.base + '/#/');
  t.ok(await waitFor(() => p.eval(`/Mystery/.test(document.querySelector('.heldwait')?.textContent || '')`)), 'the home screen shows "Mystery" waiting');
  t.ok(await waitFor(() => p.eval(`(() => { const i = document.querySelector('.heldwait img'); return !!i && i.complete && i.naturalWidth > 0; })()`)), 'with a picture of it');
  await p.shot(t.shots + '/airprint-held.png');

  // it can't cancel someone else's print
  t.ok((await phoneJob(STRANGER, 0x0008, 1)).status === 0x0403, 'the stranger can\'t cancel the computer\'s print');
  t.ok((await phoneJob(STRANGER, 0x0009, held))['job-state'] === 4, 'and sees its own print as held');

  await p.tap('.heldwait [data-act=refuse]'); await p.sleep(500);
  t.ok(printed() === 2 && (await phoneJob(STRANGER, 0x0009, held))['job-state'] === 7, 'Don\'t print: not printed, and the phone is told it was cancelled');
  t.ok(!await p.eval(`!!document.querySelector('.heldwait')`), 'and the card is gone');

  r = await phonePrint(STRANGER, 'Mystery 2');
  await p.go(t.base + '/#/settings'); await p.go(t.base + '/#/');
  await waitFor(() => p.eval(`!!document.querySelector('.heldwait [data-act=trust]')`));
  await p.tap('.heldwait [data-act=trust]');
  t.ok(await waitFor(() => printed() === 3), 'Print, and always allow this phone: printed');
  r = await phonePrint(STRANGER, 'Mystery 3');
  t.ok(r['job-state'] !== 4 && await waitFor(() => printed() === 4), 'and that phone\'s next print goes straight through');

  // Mom's phone: logs in with her PIN once, then prints from any app straight away
  await p.go(t.base + '/#/settings'); await p.waitFor('#addPin');
  await p.eval(`document.querySelector('#pinLabel').value = 'Mom'; document.querySelector('#pinNew').value = '482915'; 0`);
  await p.tap('#addPin'); await p.sleep(500);
  r = await phonePrint(MOM, 'Before login');
  t.ok(r['job-state'] === 4, 'Mom\'s phone before logging in: held like anyone');
  const login = await phone(MOM, '/api/login', Buffer.from(JSON.stringify({ pin: '482915' })), { 'Content-Type': 'application/json' });
  const cookie = (login.headers['set-cookie'] || []).map(c => c.split(';')[0]).join('; ');
  t.ok(login.code === 200 && cookie, 'Mom logs in to Sakura Print with her PIN');
  await phone(MOM, '/api/info', null, { Cookie: cookie }); await t.sleep(400);
  r = await phonePrint(MOM, 'After login');
  t.ok(r['job-state'] !== 4 && await waitFor(() => printed() === 5), 'now her prints from any app go straight through');

  await p.go(t.base + '/#/'); await p.go(t.base + '/#/settings'); await p.waitFor('[data-rmphone]');
  const phones = await p.eval(`[...document.querySelectorAll('[data-rmphone]')].map(b => b.dataset.label).join(', ')`);
  t.ok(/Mom/.test(phones) && /Neighbour/.test(phones), `Settings lists the phones that can print (${phones})`);
  await p.eval(`document.querySelector('[data-rmpin]').scrollIntoView({ block: 'center' }); 0`); await p.shot(t.shots + '/airprint-phones.png');
  await p.tap('[data-rmpin]'); await p.waitFor('.modal [data-v=yes], .modal button.main');
  await p.eval(`(document.querySelector('.modal [data-v=yes]') || document.querySelector('.modal button.main')).click(); 0`); await p.sleep(600);
  r = await phonePrint(MOM, 'After PIN removed');
  t.ok(r['job-state'] === 4, 'removing Mom\'s PIN: her phone\'s prints wait again');
  await p.go(t.base + '/#/'); await p.go(t.base + '/#/settings'); await p.waitFor('[data-tog=airprint]');
  t.ok(!/Mom/.test(await p.eval(`[...document.querySelectorAll('[data-rmphone]')].map(b => b.dataset.label).join()`)), 'and she\'s gone from the list');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
