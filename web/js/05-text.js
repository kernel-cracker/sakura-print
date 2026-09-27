// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 05: Print text: paste or type anything.
'use strict';

// ---------- print text ----------
// The browser lays the text out into A4 pages (so every language and emoji works),
// then they're uploaded as pictures and printed like any other document.
function wrapLines(text, measure, maxW) {
  const out = [];
  for (const para of text.replace(/\r/g, '').split('\n')) {
    if (!para.trim()) { out.push(''); continue; }
    const tokens = para.match(/\S+|\s+/g) || [];
    let line = '';
    for (let tok of tokens) {
      if (/^\s+$/.test(tok)) { if (line) line += tok.replace(/\t/g, '    '); continue; }
      if (measure(line + tok) <= maxW) { line += tok; continue; }
      if (line.trim()) { out.push(line.trimEnd()); line = ''; }
      while (measure(tok) > maxW) {             // one word longer than the whole line: break it
        const ch = [...tok]; let n = 1;
        while (n < ch.length && measure(ch.slice(0, n + 1).join('')) <= maxW) n++;
        out.push(ch.slice(0, n).join('')); tok = ch.slice(n).join('');
      }
      line = tok;
    }
    out.push(line.trimEnd());
  }
  return out;
}

async function textPages(t) {
  const W = 1654, H = 2339, dpi = 200, margin = Math.round(20 * dpi / 25.4);   // A4 at 200 dpi, 2 cm margins
  const cv = document.createElement('canvas'); cv.width = W; cv.height = H;
  const g = cv.getContext('2d');
  const face = '"Noto Sans", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';
  const body = { font: `${t.size * dpi / 72}px ${face}`, lh: t.size * dpi / 72 * 1.45 };
  const head = { font: `bold ${t.size * 1.6 * dpi / 72}px ${face}`, lh: t.size * 1.6 * dpi / 72 * 1.35 };
  const lines = [];
  let rest = t.text;
  if (t.heading) {
    const i = rest.search(/\S/), nl = rest.indexOf('\n', i);
    const first = nl < 0 ? rest.slice(i) : rest.slice(i, nl);
    rest = nl < 0 ? '' : rest.slice(nl + 1).replace(/^([ \t]*\n)+/, '');   // no big gap under the title
    g.font = head.font;
    wrapLines(first, s => g.measureText(s).width, W - 2 * margin).forEach(l => lines.push({ l, ...head }));
    lines.push({ l: '', font: body.font, lh: body.lh * 0.6 });
  }
  g.font = body.font;
  wrapLines(rest, s => g.measureText(s).width, W - 2 * margin).forEach(l => lines.push({ l, ...body }));
  while (lines.length && !lines[lines.length - 1].l) lines.pop();   // no trailing empty lines

  const pages = [];
  let y = margin;
  const newPage = () => { g.fillStyle = '#fff'; g.fillRect(0, 0, W, H); g.fillStyle = '#111'; g.textBaseline = 'top'; y = margin; };
  const flush = async () => { const b = await new Promise(r => cv.toBlob(r, 'image/png')); pages.push(new File([b], `text page ${pages.length + 1}.png`, { type: 'image/png' })); };
  newPage();
  for (const x of lines) {
    if (y + x.lh > H - margin) { await flush(); newPage(); if (!x.l) continue; }
    g.font = x.font; g.fillText(x.l, margin, y); y += x.lh;
  }
  await flush();
  return pages;
}

async function textSpec() {
  const t = S.txt;
  if (!t.text.trim()) { toast('Type or paste some text first'); return null; }
  const key = [t.text, t.size, t.heading].join('\u0000');
  if (key !== t.key || !t.files.length) {
    const pages = await textPages(t);
    const files = await new Promise(res => upload(pages, res));
    if (files.length !== pages.length) return null;
    t.key = key; t.files = files;
  }
  return { printer: S.printer, mode: 'docs', join: true, fitPage: true, items: t.files.map(f => ({ id: f.id })), sides: t.sides, copies: t.copies, cover: 1,
    options: printOptions({ paper: S.txt.paper, quality: S.txt.quality, colour: S.txt.colour, media: S.txt.media }) };
}

function textView() {
  const t = S.txt;
  const canPaste = window.isSecureContext && navigator.clipboard && navigator.clipboard.readText;
  render(`${pageHead('text', 'A recipe, a letter, a list. Any language works.', 'Print text')}
    <div class="card"><textarea class="big" id="txt" rows="11" placeholder="Paste or type here…">${esc(t.text)}</textarea>
      <div class="row" style="margin-top:10px">${canPaste ? `<button class="btn" id="paste">${ic('clip')} Paste</button>` : ''}
      <button class="btn" id="clear">Clear</button></div>
      ${canPaste ? '' : '<p class="hint">To paste: press and hold inside the box, then tap Paste.</p>'}</div>
    <div class="card"><h2>How it looks</h2>
      <div class="field"><span class="lbl">Text size</span>${seg('size', [[11, 'Small'], [13, 'Normal'], [16, 'Big'], [20, 'Huge']], t.size)}</div>
      <div class="field">${toggle('heading', t.heading, 'Make the first line a big title')}</div>
      <div class="field"><span class="lbl">Sides</span>${seg('sides', [['single', 'One side'], ['duplex', 'Both sides']], t.sides)}</div>
      <div class="field"><span class="lbl">Copies</span>${stepper('copies', t.copies)}</div></div>`);
  wire(t, textView);
  const ta = $('#txt');
  ta.oninput = () => { t.text = ta.value; };
  $('#clear').onclick = () => { t.text = ''; textView(); };
  const ps = $('#paste');
  if (ps) ps.onclick = async () => { try { t.text += (t.text && !t.text.endsWith('\n') ? '\n' : '') + await navigator.clipboard.readText(); textView(); } catch (_) { toast('Couldn\'t read the clipboard, press and hold in the box instead'); } };
  const bar = actions(`<button class="btn main" id="next">Next ›</button>`);
  bar.querySelector('#next').onclick = () => { if (S.txt.text.trim()) openWork('text'); else toast('Type or paste some text first'); };
}
