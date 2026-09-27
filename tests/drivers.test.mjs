// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Printers & drivers, on a pretend Debian computer (nothing is really installed: the pretend computer writes down
// what it would have run as the administrator). A Brother printer with no driver: Brother's own driver is offered
// first, its licence must be agreed to before anything happens, and one password window does all the steps.
import { writeFileSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import http from 'node:http';
export const restoreAfter = true;

const LPINFO = `Device: uri = dnssd://Brother%20DCP-T510W._ipp._tcp.local/?uuid=e3248000
        class = network
        info = Brother DCP-T510W
        make-and-model = Brother DCP-T510W
        device-id = MFG:Brother;MDL:DCP-T510W;CMD:HBP,PJL;
        location =
Device: uri = ipp://Brother%20DCP-T510W._ipp._tcp.local/
        class = network
        info = Brother DCP-T510W
        make-and-model = Brother DCP-T510W
        device-id = MFG:Brother;MDL:DCP-T510W;CMD:PWGRaster,URF;
        location =
`;
const INFS = '[DCP-T510W]\nPRN_DRV_RPM=dcpt510wpdrv-1.0.1-1.i386.rpm\nPRN_DRV_DEB=dcpt510wpdrv-1.0.1-0.i386.deb\nREQUIRE32LIB=yes\nSCANNER_DRV=brscan4\n';

export default async function (t) {
  const rootLog = join(t.work, 'root.log');
  const fake = join(t.work, 'fake-debian.json');
  writeFileSync(fake, JSON.stringify({
    have: ['apt-get', 'dpkg'], cups: 2, rootLog,
    out: {
      'lpinfo --timeout 10 -l -v': LPINFO, 'lpstat -v': '',
      'getent ahostsv4 BRW105BAD6F7645': '!', 'getent ahostsv4 BRW105BAD6F7645.local': '192.168.0.156 STREAM BRW105BAD6F7645.local\n',
      'avahi-browse -rtp --no-db-lookup _ipp._tcp': '=;wlo1;IPv4;Brother\\032DCP-T510W;_ipp._tcp;local;BRW105BAD6F7645.local;192.168.0.156;631;"ty=Brother DCP-T510W"\n',
    },
    afterRoot: {
      'dcpt510wpdrv': { 'lpstat -v': 'device for DCPT510W: dnssd://Brother%20DCP-T510W._ipp._tcp.local/\n' },
      'brscan4': { 'scanimage -L': "device `brother4:net1;dev0' is a Brother DCP-T510W USB scanner\n" },
    },
    fetch: {
      'https://download.brother.com/pub/com/linux/linux/infs/DCPT510W': INFS,
      'https://download.brother.com/pub/com/linux/linux/packages/dcpt510wpdrv-1.0.1-0.i386.deb': '@' + t.fixture('drivers/dcpt510wpdrv-1.0.1-0.i386.deb'),
      'https://download.brother.com/pub/com/linux/linux/infs/brscan4.lnk': 'DEB64=brscan4-0.4.11-1.amd64.deb\nRPM64=brscan4-0.4.11-2.x86_64.rpm\n',
      'https://download.brother.com/pub/com/linux/linux/packages/brscan4-0.4.11-1.amd64.deb': '@' + t.fixture('drivers/brscan4-0.4.11-1.amd64.deb'),
    },
  }));
  await t.resetSettings({}, { SAKURA_FAKE_SYSTEM: fake });
  const log = () => existsSync(rootLog) ? readFileSync(rootLog, 'utf8') : '';

  // phones can't set up printers
  const port = new URL(t.base).port;
  const code = await new Promise(res => http.get({ host: '127.0.0.1', port, path: '/api/drivers', headers: { Host: 'sakura-phone.test:' + port } }, r => { r.resume(); res(r.statusCode); }));
  t.ok(code === 401 || code === 403, `a phone can't reach printer setup (${code})`);

  const p = await t.launch();
  await p.go(t.base + '/#/addprinter');
  await p.waitFor('[data-dev]', 20000);
  t.ok(/Brother DCP-T510W/.test(await p.eval(`document.querySelector('.drvcard').textContent`)) &&
    /Not set up yet/.test(await p.eval(`document.querySelector('.drvcard').textContent`)), 'finds the Brother, not set up yet');
  await p.shot(t.shots + '/drivers-list.png');
  await p.tap('[data-dev]');
  await p.waitFor('[data-go-route]', 20000);
  const best = await p.eval(`document.querySelector('.drvroute.best h2').textContent`);
  t.ok(/Brother's driver, from Brother/.test(best), `Brother's own driver is the best way (${best})`);
  t.ok(await p.eval(`/only on Arch-based systems/.test(document.querySelector('.drvwhy').textContent)`), 'and "why not the others" explains the AUR is only for Arch');
  await p.shot(t.shots + '/drivers-plan.png');

  // not agreeing: nothing is installed
  await p.tap('.drvroute.best [data-go-route]');
  await p.waitFor('pre.licence', 20000);
  t.ok(/Brother License Agreement/.test(await p.eval(`document.querySelector('pre.licence').textContent`)), 'Brother\'s licence, from its own package, is shown');
  await p.shot(t.shots + '/drivers-licence.png');
  t.ok(log() === '', 'nothing installed while the licence is on the screen');
  await p.tap('[data-a=no]');
  await p.waitFor('#drvother', 10000);
  t.ok(/Nothing more was installed/.test(await p.eval(`document.querySelector('.modal').textContent`)) && log() === '', 'I don\'t agree: stopped, nothing installed');
  await p.tap('#drvother');

  // agreeing: one password window does everything, in the right order
  await p.waitFor('.drvroute.best [data-go-route]', 20000);
  await p.tap('.drvroute.best [data-go-route]');
  await p.waitFor('pre.licence', 20000);
  await p.tap('[data-a=yes]');
  await p.waitFor('#drvnext', 20000);
  t.ok(/is ready/.test(await p.eval(`document.querySelector('.modal').textContent`)), 'I agree: the printer is ready');
  await p.shot(t.shots + '/drivers-done.png');
  const steps = log().split('\n').filter(Boolean);
  const at = s => steps.findIndex(l => l.includes(s));
  t.ok(at('dpkg --add-architecture i386') >= 0 && at('apt-get install -y libc6:i386') > at('dpkg --add-architecture i386') &&
    at('dcpt510wpdrv-1.0.1-0.i386.deb') > at('libc6:i386') && at('sh -c') > at('dcpt510wpdrv'), 'as the administrator: 32-bit libraries, then the driver, then the print queue\n    ' + steps.filter(l => !l.startsWith(' ') && !/^(id|if|fi|else|echo|  )/.test(l)).map(l => l.slice(0, 90)).join('\n    '));
  const settings = JSON.parse(readFileSync(join(t.work, 'config', 'sakuraprint', 'settings.json'), 'utf8'));
  t.ok(settings.driverLicences?.length === 1 && settings.driverLicences[0].maker === 'Brother' && settings.driverHashes?.['dcpt510wpdrv-1.0.1-0.i386.deb'],
    'the licence agreed to and the download\'s fingerprint are written down');

  // the scanner: Brother's scanner driver, registered by the printer's network name
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  await p.go(t.base + '/#/'); await p.go(t.base + '/#/addprinter');
  await p.waitFor('[data-dev]', 20000);
  await p.tap('[data-dev]');
  await p.waitFor('[data-go-route=scan-maker]', 20000);
  t.ok(/Brother's scanner driver/.test(await p.eval(`document.querySelector('[data-route=scan-maker]').textContent`)), 'the scanner: Brother\'s scanner driver offered');
  await p.eval(`document.querySelector('[data-route=scan-maker]').scrollIntoView({ block: 'center' }); 0`); await p.shot(t.shots + '/drivers-scanner.png');
  await p.tap('[data-go-route=scan-maker]');
  await p.waitFor('pre.licence', 20000);
  await p.tap('[data-a=yes]');
  await p.waitFor('.modal [data-close]', 20000);
  t.ok(/Scanning works/.test(await p.eval(`document.querySelector('.modal').textContent`)), 'scanning works');
  const scanSteps = log();
  t.ok(scanSteps.includes('brscan4-0.4.11-1.amd64.deb') && scanSteps.includes('nodename=BRW105BAD6F7645.local'), 'installed, and registered by the printer\'s network name');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
