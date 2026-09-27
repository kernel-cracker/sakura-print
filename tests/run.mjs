// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
//
// Browser tests for Sakura Print. Builds the program, starts it on a spare port with its own throwaway
// settings and data folders, drives a headless Chromium like a phone (touch, taps, drags) and checks what
// comes out, often pixel by pixel in the saved pictures.
//
//   node tests/run.mjs            all tests
//   node tests/run.mjs editor     only tests whose file name contains "editor"
//   KEEP=1 node tests/run.mjs     keep the screenshots even when everything passes
//
// Needs: Go, Node 22+, Chromium. Tests that need a real printer skip themselves if there is none.
import { spawn, execFileSync } from 'node:child_process';
import { mkdtempSync, readdirSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:net';
import { launch } from './lib/cdp.mjs';

const here = dirname(fileURLToPath(import.meta.url)), root = join(here, '..');
const only = process.argv[2] || '';
const work = mkdtempSync(join(tmpdir(), 'sakura-test-'));
const bin = join(work, 'sakuraprint');

function which(cmd) { try { execFileSync('sh', ['-c', `command -v ${cmd}`]); return true; } catch { return false; } }
if (!which('chromium') && !which('google-chrome') && !which('chromium-browser')) {
  console.log('No Chromium found: skipping the browser tests.'); process.exit(0);
}
console.log('building…');
execFileSync('go', ['build', '-o', bin, '.'], { cwd: root, stdio: 'inherit' });

const freePort = () => new Promise(res => { const s = createServer().listen(0, () => { const p = s.address().port; s.close(() => res(p)); }); });
const port = await freePort(), base = `http://localhost:${port}`;
const cfg = join(work, 'config'), data = join(work, 'data'), printed = join(work, 'printed');
mkdirSync(printed, { recursive: true });
let hasPrinter = false;
try { hasPrinter = /\S/.test(execFileSync('lpstat', ['-a'], { encoding: 'utf8', timeout: 5000 })); } catch { }

function writeSettings(patch = {}) {
  // skip the first-time setup popup for whatever printer is there, so tests start on a clean screen
  mkdirSync(join(cfg, 'sakuraprint'), { recursive: true });
  let printers = [];
  try { printers = execFileSync('lpstat', ['-a'], { encoding: 'utf8', timeout: 5000 }).split('\n').map(l => l.split(' ')[0]).filter(Boolean); } catch { }
  const styles = Object.fromEntries(printers.map(p => [p, 'rear'])), setup = Object.fromEntries(printers.map(p => [p, 99]));
  writeFileSync(join(cfg, 'sakuraprint', 'settings.json'), JSON.stringify({ requirePin: true, styles, setup, welcome: true, ...patch }));
}
let server = null;
async function start(extraEnv = {}) {
  // nothing is ever really printed: "printed" PDFs land in a folder instead, and no Bonjour announcements go out.
  // Pretend phones say who they are with a header (they all really come from this computer).
  server = spawn(bin, ['serve', '-port', String(port)], {
    env: { ...process.env, XDG_CONFIG_HOME: cfg, XDG_DATA_HOME: data, SAKURA_DRY_PRINT: printed, SAKURA_NO_BONJOUR: '1', SAKURA_TEST_FAKE_MAC: '1', ...extraEnv }, stdio: 'ignore',
  });
  for (let i = 0; i < 100; i++) { try { await fetch(base + '/api/info'); return; } catch { await new Promise(r => setTimeout(r, 100)); } }
  throw new Error('the server did not start');
}
async function stop() { if (!server) return; const s = server; server = null; s.kill(); await new Promise(r => s.once('exit', r)); }
writeSettings();
await start();

const shots = join(work, 'screenshots'); mkdirSync(shots, { recursive: true });
let pass = 0, fail = 0, skipped = 0;
const results = [];
const files = readdirSync(here).filter(f => f.endsWith('.test.mjs') && f.includes(only)).sort();
for (const f of files) {
  const mod = await import(join(here, f));
  const name = f.replace('.test.mjs', '');
  if (mod.needsPrinter && !hasPrinter) { console.log(`\n── ${name}: skipped (needs a printer)`); skipped++; continue; }
  console.log(`\n── ${name}`);
  let fp = 0, ff = 0;
  const opened = [];   // browsers this test starts, closed afterwards even if it crashes
  const t = {
    base, hasPrinter, dataDir: join(data, 'sakuraprint'), shots, printed, work, which,
    fixture: n => join(here, 'fixtures', n),
    async launch(...a) { const p = await launch(...a); opened.push(p); return p; },
    ok(c, msg) { console.log((c ? '  PASS ' : '  FAIL ') + msg); c ? fp++ : ff++; },
    async restart() { await stop(); await start(); },
    async resetSettings(patch, env) { await stop(); writeSettings(patch); await start(env); },   // env: e.g. a pretend computer
    sleep: ms => new Promise(r => setTimeout(r, ms)),
  };
  const t0 = Date.now();
  try { await mod.default(t); } catch (e) { ff++; console.log('  FAIL crashed: ' + (e.stack || e).toString().split('\n').slice(0, 3).join(' | ')); }
  for (const p of opened) await p.close().catch(() => { });
  if (mod.restoreAfter) { await stop(); writeSettings(); await start(); }   // back to a normal server for the next test
  pass += fp; fail += ff;
  results.push(`${ff ? '✘' : '✔'} ${name.padEnd(14)} ${fp} passed${ff ? `, ${ff} failed` : ''}  (${((Date.now() - t0) / 1000).toFixed(0)} s)`);
}
await stop();
if (fail || process.env.KEEP) console.log(`\nScreenshots from this run are in ${shots}`);
else rmSync(work, { recursive: true, force: true });
console.log('\n' + results.join('\n'));
console.log(`\n${pass} passed, ${fail} failed${skipped ? `, ${skipped} test files skipped (no printer)` : ''}`);
process.exit(fail ? 1 : 0);
