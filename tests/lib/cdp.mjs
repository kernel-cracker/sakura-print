// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Minimal Chrome DevTools Protocol driver: launches headless Chromium as a phone, lets a scenario
// tap things like a finger would, and records navigations, requests and timings.
import { spawn } from 'node:child_process';
import { writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const sleep = ms => new Promise(r => setTimeout(r, ms));

// args: extra Chromium switches, like --host-resolver-rules to reach the server under a phone's name
export async function launch({ width = 390, height = 844, mobile = true, args = [] } = {}) {
  const port = 9300 + Math.floor(Math.random() * 500);
  const prof = mkdtempSync(join(tmpdir(), 'chr-'));
  const proc = spawn('chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', `--remote-debugging-port=${port}`,
    `--user-data-dir=${prof}`, '--no-first-run', ...args, 'about:blank'], { stdio: 'ignore' });
  let targets;
  for (let i = 0; i < 50; i++) {
    try { targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json(); if (targets.length) break; } catch { }
    await sleep(100);
  }
  const page = targets.find(t => t.type === 'page');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise(r => ws.onopen = r);
  let id = 0; const pending = new Map(), listeners = [];
  ws.onmessage = ev => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) { const { res, rej } = pending.get(m.id); pending.delete(m.id); m.error ? rej(new Error(m.error.message)) : res(m.result); }
    else listeners.forEach(l => l(m));
  };
  const send = (method, params = {}) => new Promise((res, rej) => { const i = ++id; pending.set(i, { res, rej }); ws.send(JSON.stringify({ id: i, method, params })); });

  const log = { navs: [], loads: 0, requests: [], errors: [], console: [] };
  const reqs = new Map();
  listeners.push(m => {
    if (m.method === 'Page.frameNavigated' && !m.params.frame.parentId) log.navs.push({ t: Date.now(), url: m.params.frame.url });
    if (m.method === 'Page.loadEventFired') log.loads++;
    if (m.method === 'Network.requestWillBeSent') reqs.set(m.params.requestId, { url: m.params.request.url, method: m.params.request.method, start: m.params.timestamp, type: m.params.type });
    if (m.method === 'Network.loadingFinished') { const r = reqs.get(m.params.requestId); if (r) { r.ms = Math.round((m.params.timestamp - r.start) * 1000); r.bytes = m.params.encodedDataLength; log.requests.push(r); } }
    if (m.method === 'Network.responseReceived') { const r = reqs.get(m.params.requestId); if (r) { r.status = m.params.response.status; r.cached = m.params.response.fromDiskCache; } }
    if (m.method === 'Runtime.exceptionThrown') log.errors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text);
    if (m.method === 'Runtime.consoleAPICalled') log.console.push(m.params.args.map(a => a.value ?? a.description).join(' '));
  });
  await send('Page.enable'); await send('Network.enable'); await send('Runtime.enable');
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 3, mobile });
  if (mobile) await send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 });

  const P = {
    send, log, sleep,
    async go(url) { await send('Page.navigate', { url }); await this.idle(); },
    async eval(expr) {
      const r = await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true });
      if (r.exceptionDetails) throw new Error('eval: ' + (r.exceptionDetails.exception?.description || r.exceptionDetails.text));
      return r.result.value;
    },
    async idle(quietMs = 400, maxMs = 20000) {   // wait until no requests have been in flight for quietMs
      const end = Date.now() + maxMs; let last = log.requests.length, since = Date.now();
      while (Date.now() < end) {
        await sleep(100);
        const inflight = [...reqs.values()].filter(r => r.ms === undefined && !r.dead).length;
        if (log.requests.length !== last || inflight) { last = log.requests.length; since = Date.now(); }
        else if (Date.now() - since >= quietMs) return;
      }
    },
    async waitFor(sel, ms = 15000) {
      const end = Date.now() + ms;
      while (Date.now() < end) { if (await this.eval(`!!document.querySelector(${JSON.stringify(sel)})`)) return true; await sleep(100); }
      throw new Error('timed out waiting for ' + sel);
    },
    // a real tap: touch events at the element's centre, the way a finger does it
    async tap(sel, nth = 0, scroll = true) {
      const box = await this.eval(`(() => { const e = document.querySelectorAll(${JSON.stringify(sel)})[${nth}]; if (!e) return null;
        if (${scroll}) e.scrollIntoView({ block: 'center' }); const r = e.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
      if (!box) throw new Error('no element ' + sel);
      await send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [box] });
      await send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await sleep(60);
    },
    async select(sel, value) {   // choosing from a dropdown fires "change" like a phone's picker does
      await this.eval(`(() => { const s = document.querySelector(${JSON.stringify(sel)}); s.value = ${JSON.stringify(value)}; s.dispatchEvent(new Event('change', { bubbles: true })); })()`);
    },
    on(method, fn) { listeners.push(m => { if (m.method === method) fn(m.params); }); },
    // the next file picker the page opens gets these files (headless Chromium has no real picker to tap through)
    async chooseFiles(paths) {
      if (!this._chooser) {
        await send('Page.setInterceptFileChooserDialog', { enabled: true });
        this._queue = [];
        this.on('Page.fileChooserOpened', ev => { const files = this._queue.shift() || []; send('DOM.setFileInputFiles', { files, backendNodeId: ev.backendNodeId }).catch(() => {}); });
        this._chooser = true;
      }
      this._queue.push(paths);
    },
    async shot(path) { const { data } = await send('Page.captureScreenshot', { format: 'png' }); writeFileSync(path, Buffer.from(data, 'base64')); },
    mark() { return { navs: log.navs.length, loads: log.loads, reqs: log.requests.length }; },
    since(m) { return { navs: log.navs.slice(m.navs), loads: log.loads - m.loads, reqs: log.requests.slice(m.reqs) }; },
    async close() {   // and throw away its profile (about 150 MB each)
      try { ws.close(); } catch { }
      if (proc.exitCode === null) { proc.kill(); await new Promise(r => { proc.once('exit', r); setTimeout(r, 3000); }); }
      rmSync(prof, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });   // Chromium may still be writing to it
    },
  };
  return P;
}
