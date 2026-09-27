// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// The layout, on every screen, on a phone and on a computer: navigation is always there (a tab bar on phones, a
// sidebar on wide screens) and never covered, the header's title is readable, content is centred, the
// notifications bell and the printer are always one tap away.
export const needsPrinter = true;

const SCREENS = ['', 'docs', 'photos', 'text', 'scan', 'again', 'printer', 'settings', 'addprinter', 'advanced'];

export default async function (t) {
  for (const [label, opts] of [['phone', {}], ['computer', { width: 1280, height: 800, mobile: false }]]) {
    const p = await t.launch(opts);
    await p.go(t.base + '/#/'); await p.waitFor('#top');
    await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
    const bad = [];
    for (const r of SCREENS) {
      await p.go(t.base + '/#/' + r);
      await p.eval(`new Promise(r => setTimeout(r, 700))`);
      await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
      const s = await p.eval(`(() => {
        const vis = e => { if (!e) return false; const cs = getComputedStyle(e), b = e.getBoundingClientRect(); return cs.display !== 'none' && cs.visibility !== 'hidden' && b.width > 0 && b.height > 0; };
        const tabs = document.querySelector('#tabs'), side = document.querySelector('#side'), title = document.querySelector('#ptitle');
        const tb = tabs.getBoundingClientRect(), acts = [...document.querySelectorAll('.actions')].filter(vis).map(a => a.getBoundingClientRect());
        const app = document.querySelector('#app').getBoundingClientRect(), main = document.querySelector('#main').getBoundingClientRect();
        return { tabs: vis(tabs), side: vis(side), tabsItems: [...tabs.querySelectorAll('[data-go]')].map(b => b.dataset.go),
          sideItems: [...side.querySelectorAll('[data-go]')].map(b => b.dataset.go),
          tabsOn: (tabs.querySelector('.on') || {}).dataset?.go ?? null, sideOn: (side.querySelector('.on') || {}).dataset?.go ?? null,
          titleCut: title.scrollWidth > title.clientWidth + 1, title: title.textContent,
          covered: acts.some(a => vis(tabs) && a.bottom > tb.top + 1),
          centred: Math.abs((app.left - main.left) - (main.right - app.right)) <= 2,
          bell: vis(document.querySelector('#bell')), chip: ${JSON.stringify(['settings', 'advanced', 'addprinter'].includes(r))} || vis(document.querySelector('#pchip')) };
      })()`);
      const name = r || 'home';
      if (label === 'phone' && (!s.tabs || s.side)) bad.push(`${name}: phones use the tab bar (tabs ${s.tabs}, sidebar ${s.side})`);
      if (label === 'computer' && (!s.side || s.tabs)) bad.push(`${name}: computers use the sidebar (sidebar ${s.side}, tabs ${s.tabs})`);
      if (s.covered) bad.push(`${name}: the action bar covers the tab bar`);
      if (s.titleCut) bad.push(`${name}: the title is cut off ("${s.title}")`);
      if (!s.centred) bad.push(`${name}: the content isn't centred`);
      if (!s.bell) bad.push(`${name}: no notifications bell`);
      if (!s.chip) bad.push(`${name}: no printer chip`);
      if (label === 'phone' && JSON.stringify(s.tabsItems) !== JSON.stringify(['', 'docs', 'scan', 'printer', 'settings'])) bad.push(`${name}: tab bar items ${s.tabsItems}`);
      if (label === 'computer') for (const want of ['', 'docs', 'photos', 'text', 'scan', 'again', 'printer', 'addprinter', 'settings', 'advanced'])
        if (!s.sideItems.includes(want)) bad.push(`${name}: the sidebar lacks ${want || 'home'}`);
      const on = label === 'phone' ? s.tabsOn : s.sideOn;
      const expect = label === 'phone' ? ({ photos: 'docs', text: 'docs', again: '', addprinter: 'settings', advanced: 'settings' }[r] ?? r) : r;
      if (on !== expect) bad.push(`${name}: the highlighted item is ${on}, want ${expect}`);
      await p.shot(`${t.shots}/layout-${label}-${name}.png`);
    }
    t.ok(bad.length === 0, `${label}: navigation, title, centring, bell and printer on every screen` + (bad.length ? '\n      ' + bad.join('\n      ') : ''));
    t.ok(!p.log.errors.length, `${label}: no JavaScript errors ` + p.log.errors.join(' | '));
    await p.close();
  }

  // the printer chip lists every printer the computer knows, and choosing one switches to it
  const p = await t.launch();
  await p.go(t.base + '/#/docs'); await p.waitFor('#pchip');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const printers = await p.eval(`S.info.printers`);
  await p.tap('#pchip');
  await p.waitFor('.pchoice');
  const choices = await p.eval(`[...document.querySelectorAll('.pchoice')].map(b => b.dataset.printer)`);
  t.ok(printers.length > 0 && JSON.stringify(choices) === JSON.stringify(printers), `the printer chip lists every printer (${choices.join(', ')})`);
  const last = printers[printers.length - 1];
  await p.eval(`document.querySelector('.pchoice[data-printer="${last.replace(/"/g, '\\"')}"]').click(); 0`);
  await p.sleep(400);
  t.ok(await p.eval(`S.printer`) === last && (await p.eval(`document.querySelector('#pchip').textContent`)).length > 0, 'choosing one switches to it');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
