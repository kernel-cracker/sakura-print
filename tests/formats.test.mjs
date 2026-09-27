// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Every kind of file: iPhone HEIC, WebP, Word, text... added from the phone, and the camera scanner with a
// HEIC photo (which Chrome can't open itself, so the computer converts it).
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
export const needsPrinter = true;

const haveTool = t => { try { execFileSync('sh', ['-c', `command -v ${t}`]); return true; } catch { return false; } };

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/docs'); await p.waitFor('[data-src=pc]');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  t.ok(await p.eval(`ACCEPT.docs === ''`), 'Documents accept any file from the phone');
  const office = haveTool('soffice') || haveTool('libreoffice'), heif = haveTool('heif-dec') || haveTool('magick');
  const names = ['photo.heic', 'photo.webp', ...(office ? ['letter.docx', 'notes.txt'] : [])];
  const files = names.map(n => ({ n, b64: readFileSync(t.fixture('formats/' + n)).toString('base64') }));
  const res = await p.eval(`(async () => {
    const fd = new FormData();
    for (const { n, b64 } of ${JSON.stringify(files)}) fd.append('files', new File([Uint8Array.from(atob(b64), c => c.charCodeAt(0))], n));
    return await (await fetch('/api/upload', { method: 'POST', body: fd })).json();
  })()`);
  const kinds = Object.fromEntries(res.files.map(f => [f.name, f.kind]));
  t.ok(!res.errors.length, 'nothing refused ' + res.errors.join(' | '));
  if (heif) t.ok(kinds['photo.heic'] === 'image', 'an iPhone HEIC photo becomes a picture');
  t.ok(kinds['photo.webp'] === 'image', 'a WebP picture (WhatsApp, the web) becomes a picture');
  if (office) t.ok(kinds['letter.docx'] === 'pdf' && kinds['notes.txt'] === 'pdf', 'Word and text files become PDFs');
  await p.eval(`S.docs = ${JSON.stringify(res.files)}; openWork('docs'); 0`);
  await p.waitFor('.pvpage img', 30000); await p.idle(800);
  const n = await p.eval(`document.querySelectorAll('.pvpage').length`);
  t.ok(n >= res.files.length, `they all print together, one after another (${n} pages for ${res.files.length} files)`);
  await p.eval(`document.querySelector('.pv [data-x=close]').click(); 0`); await p.sleep(300);

  // Photos: HEIC straight into the photo layout
  await p.eval(`S.photos = ${JSON.stringify(res.files.filter(f => f.kind === 'image'))}; openWork('photos'); 0`);
  await p.waitFor('.pvpage img', 20000);
  t.ok(await p.eval(`document.querySelector('.pvpage img').naturalWidth > 0 || new Promise(r => document.querySelector('.pvpage img').onload = () => r(true))`), 'iPhone and WebP photos print as photos');
  await p.eval(`document.querySelector('.pv [data-x=close]').click(); 0`); await p.sleep(300);

  // the camera scanner, given a HEIC photo: Chrome can't open it, the computer converts it
  if (heif) {
    await p.go(t.base + '/#/scan'); await p.waitFor('#camScan');
    await p.tap('#camScan', 0, false); await p.waitFor('.cammodal [data-x=choose]');
    await p.chooseFiles([t.fixture('formats/photo.heic')]);
    await p.tap('.cammodal [data-x=choose]', 0, false);
    await p.waitFor('.camh', 20000).catch(() => {});
    t.ok(await p.eval(`!!document.querySelector('.camh') && !document.querySelector('#camview').hidden`), 'the camera scanner opens an iPhone HEIC photo too');
    t.ok(!/Couldn't open/.test(await p.eval(`[...document.querySelectorAll('.toast')].map(x => x.textContent).join(' ')`)), 'with no "couldn\'t open" message');
  }
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
