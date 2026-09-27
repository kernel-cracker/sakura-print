// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 12: The notifications centre. The bell counts what needs you; its panel says it in
// plain words, and a tap deals with it (see docs/design/notices.md).
'use strict';

let noticesList = [], noticesTimer = null, noticesBusy = false;

async function refreshNotices() {
  if (noticesBusy || !S.info) return;
  noticesBusy = true;
  try { noticesList = await api('/api/notices'); } catch (_) { noticesBusy = false; return; }
  noticesBusy = false;
  const n = noticesList.length, c = $('#bellcount');
  c.textContent = n ? String(n) : '';
  $('#bell').classList.toggle('urgent', noticesList.some(x => x.level === 'action' || x.level === 'problem'));
  $('#bell').setAttribute('aria-label', n ? `Notifications: ${n}` : 'Notifications');
  if (!$('#notices').hidden) drawNotices();
  if (!noticesTimer) noticesTimer = setInterval(refreshNotices, 15000);
}

const NOTICE_ICON = { action: 'printer', problem: 'x', warning: 'info', info: 'info' };

function drawNotices() {
  const p = $('#notices');
  p.innerHTML = `<div class="n-head"><h2>Notifications</h2><button class="icon-btn" data-n-close aria-label="Close">${ic('x')}</button></div>
    ${noticesList.length ? noticesList.map((n, i) => `<div class="notice ${n.level}" data-i="${i}" role="button" tabindex="0">
      <span class="n-ic">${icon(NOTICE_ICON[n.level] || 'info')}</span>
      <div class="n-body"><b class="n-title">${esc(n.title)}</b><span class="n-text">${esc(n.text)}</span>
        ${n.action ? `<button class="btn" data-n-action="${esc(n.action)}">${esc(n.actionLabel || 'Do it')}</button>` : ''}</div>
      ${n.level === 'action' ? '' : `<button class="icon-btn n-dismiss" data-n-dismiss="${esc(n.id)}" aria-label="Put away">${ic('x')}</button>`}
    </div>`).join('') : `<div class="n-empty">${bigIcon('check')}<p>Nothing needs you.</p></div>`}`;
  p.querySelector('[data-n-close]').onclick = closeNotices;
  p.querySelectorAll('.notice').forEach(el => el.onclick = e => {
    const n = noticesList[+el.dataset.i];
    const act = e.target.closest('[data-n-action]'), dis = e.target.closest('[data-n-dismiss]');
    if (act) return guard(async () => { try { await api('/api/notices', { id: n.id, action: act.dataset.nAction }); if (n.action === 'phones:1h') { S.info.phones = (await api('/api/settings')).phones; } } catch (x) { oops(x); } await refreshNotices(); if (location.hash === '#/' || location.hash === '#/settings' || location.hash === '') route(); });
    if (dis) return guard(async () => { try { await api('/api/notices', { id: n.id, action: 'dismiss' }); } catch (x) { oops(x); } await refreshNotices(); });
    openNotice(n);
  });
}

async function openNotice(n) {
  closeNotices();
  if (n.job) {   // a print waiting for the flip: its guide, where the other side is printed
    try { followJob(await api('/api/job/' + n.job)); } catch (e) { oops(e); }
    return;
  }
  if (n.go) location.hash = n.go;
}

function openNotices() { drawNotices(); $('#notices').hidden = false; document.body.classList.add('notices-open'); refreshNotices(); }
function closeNotices() { $('#notices').hidden = true; document.body.classList.remove('notices-open'); }
// coming back to the app (another window, the phone unlocked): what needs you may have changed
document.addEventListener('visibilitychange', () => { if (!document.hidden) refreshNotices(); });
window.addEventListener('focus', () => refreshNotices());

document.addEventListener('click', e => {
  if (e.target.closest('#bell')) { $('#notices').hidden ? openNotices() : closeNotices(); return; }
  if (!$('#notices').hidden && !e.target.closest('#notices') && !e.target.closest('.modal')) closeNotices();
});
