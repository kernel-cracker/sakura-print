# Printer drivers: how Sakura Print gets them

The plan for installing printer drivers on any Linux, for any printer maker, built to keep working for years without
updates to Sakura Print. Not legal advice: it does what the makers' own tools and the distributions already do.

**Why drivers at all:** on the Brother DCP-T510W, Brother's driver prints a page in 30 to 60 seconds, looking better.
Driverless printing (IPP Everywhere) takes about 3 minutes, looking worse, because the printer's small processor has
to do all the work itself. That's typical of cheap inkjets; office lasers usually print driverless just as well.

## How it stays working for years

Anything written into Sakura Print goes stale: distribution names, package names, download links, which printer
needs which driver. So Sakura Print writes almost none of it in. It asks, every time:

| Question | Asked of | Not |
|---|---|---|
| Which printer is this? | The printer itself: its make and model over the network (`ty=` in its Bonjour announcement, `printer-make-and-model` over IPP) or USB (IEEE 1284 device ID) | A list of printers |
| Is a driver for it already here? | CUPS: `lpinfo --device-id 'MFG:Brother;MDL:DCP-T510W;' -m`. CUPS does the matching, for every driver installed, now and later | A list of which driver fits which printer |
| Which package has a driver for it? | The package manager, where packages say which printers they drive (Fedora and RHEL: `postscriptdriver(brother;dcp-t510w;)`, 1252 of them in `hplip` alone). Any distribution that starts doing this later works without changes | A list of package names |
| How does this computer install things? | Which tools exist (below) | A list of distributions |
| Which CUPS is this? | `cups-config --version` / the CUPS server | An assumption |

What little *is* written in is **hints**, and every hint is checked before use, so a stale hint is skipped, never
followed into a mistake:

- A few package names per maker (like `hplip`, `printer-driver-escpr`, `gutenprint-cups`), in a data file. Before
  offering one, Sakura Print asks the package manager whether it exists. After installing, CUPS (`lpinfo`) decides
  whether it really drives this printer; if not, Sakura Print says so and goes on to the next route.
- Makers' support sites, as bare domain names (`support.brother.com`, `epson.com`, `canon.com`…), which outlive
  any deep link.
- One maker lookup: Brother's per-model driver list (below). If Brother ever moves it, that route is skipped.

**The data file can be changed without a new Sakura Print:** `~/.config/sakuraprint/drivers.json` adds or replaces
hints (a package name, a maker domain, a lookup address). Anyone can fix a stale hint with a text editor.

**`sakuraprint doctor drivers`** shows, for this computer and printer, what each route finds and why a route is
skipped. When something changes in the world, this says what, without reading code.

**Printing never depends on it.** If every route fails, the printer is set up driverless, which works with nearly
every printer sold in the last ten years (AirPrint and Mopria require it). And Sakura Print only ever sends to a
CUPS print queue, whatever is behind it.

## The rules

1. **Sakura Print never hands out drivers itself.** No bundling, mirroring or re-uploading. Every driver comes from
   its maker or the distribution, onto this computer, when the person asks.
2. **Official first.** Community drivers only when the maker has nothing for this printer.
3. **The person sees the licence and agrees**, wherever the maker's own installer asks. Nothing is installed before
   "I agree". What was agreed to, and when, is kept in the settings.
4. **Only official ways in.** The package manager; a lookup the maker's own installer uses; otherwise the maker's
   site, opened in the browser for the person to click through themselves. Never scraping, never around logins,
   captchas or licence pages. Every request says who it is (`User-Agent: SakuraPrint/<version>`).
5. **Only on the computer, with the computer's password.** Installing runs code as the administrator, so phones can
   never start it. Every install asks in the system's own password window (polkit: `pkexec`, or PackageKit's own).
6. **Brand names only to say what works.** No logos; the README says Sakura Print isn't made or endorsed by any
   printer maker.

## The routes, in order

Each route is tried only if the one before found nothing. After each install, CUPS is asked again whether a driver
now fits the printer.

| # | Route | How it works |
|---|---|---|
| 0 | **Already installed** | `lpinfo --device-id … -m`. Nothing to download |
| 1 | **Official, in the distribution** | Packages that say they drive this printer (Fedora, RHEL); otherwise the maker's hint packages that exist here: HP's `hplip`, Epson's `escpr`, makers' PostScript files published through OpenPrinting (`openprinting-ppds`, `foomatic-db`) |
| 2 | **Official, from the maker's server** | Brother today: `download.brother.com/pub/com/linux/linux/infs/<MODEL>` lists the model's `.deb`/`.rpm` (and whether it needs 32-bit libraries, `REQUIRE32LIB`, and the scanner driver); the file comes from `…/linux/packages/`. Licence screens exactly like Brother's installer (Brother licence, then GPL). Another maker can be added through the data file, if it offers a lookup |
| 3 | **Official, guided download** | For makers without a lookup (Canon, Epson's other drivers, Kyocera, Lexmark, Pantum…): the maker's support site opens in the browser with the model name to search for. The person picks their system, accepts the licence there, downloads. Sakura Print spots the new `.deb`/`.rpm`/`.ppd` in Downloads, checks the package names the maker as its vendor, and installs it |
| 4 | **Community** | The AUR, on Arch-based systems with `yay` or `paru`: found through the AUR's public search, shown with votes, maintainer, last update and licence; installed in a visible terminal showing the recipe. Written by volunteers, not vetted: the screen says so |
| 5 | **Open drivers** | `gutenprint`, `brlaser`, `splix`, `foo2zjs`: community projects covering hundreds of printers, signed by the distribution |
| 6 | **Printer Applications** | OpenPrinting's drivers packaged as small apps that look like driverless printers to CUPS: `hplip-printer-app`, `gutenprint-printer-app`, `ps-printer-app`, `ghostscript-printer-app` (published by OpenPrinting as snaps; checked 2026-09-26). Used where snaps work and routes 1 to 5 found nothing, and always on CUPS 3 (below) |
| 7 | **Driverless** | CUPS's built-in IPP Everywhere support |

If a driver and driverless both work, setup can print a timed test page with each and keep the faster one.

## How this computer installs things

Decided by what exists, in this order, not by the distribution's name:

| Found | Installs packages with | Installs a maker's file with | Notes |
|---|---|---|---|
| `/run/ostree-booted` (Silverblue, Kinoite, Bazzite…) | `rpm-ostree install` | `rpm-ostree install ./file.rpm` | Active after a restart; the screen says so |
| `/etc/NIXOS` | — | — | NixOS installs from its configuration: Sakura Print shows the line to add (`services.printing.drivers = [ pkgs.hplip ];`) |
| `apt-get` | `apt-get install` | `apt-get install ./file.deb` (fetches what it needs, unlike `dpkg -i`) | |
| `dnf` | `dnf install` | `dnf install ./file.rpm` | Package lookup by printer: `dnf repoquery --whatprovides 'postscriptdriver(…)'` |
| `zypper` | `zypper install` | `zypper install ./file.rpm` | |
| `pacman` | `pacman -S --needed` | Turned into a pacman package (below) | AUR through `yay`/`paru` if installed |
| `pkcon` (PackageKit) | `pkcon install` | `pkcon install-local` | For any other system: PackageKit speaks to many package managers |
| none of these | — | — | Sakura Print says which driver to install by hand, and sets the printer up driverless meanwhile |

**32-bit libraries** (Brother's lookup says when they're needed): the package that provides the 32-bit loader,
`ld-linux.so.2`, found by asking the package manager for that file where it can (`dnf provides`, `zypper
what-provides`); otherwise the long-standing names (`libc6:i386` after `dpkg --add-architecture i386`,
`glibc.i686`, `glibc-32bit`, `lib32-glibc` from Arch's multilib).

## A maker's package on Arch

Arch doesn't install `.deb`/`.rpm` files, so Sakura Print turns them into a pacman package, the way AUR recipes do
by hand; pacman then knows every file, so removing and updating is clean.

```
bsdtar -xf dcpt510wpdrv-1.0.1-0.i386.deb   # the files, and the install scripts
# a PKGBUILD: sakura-brother-dcpt510w 1.0.1-0, the file's sha256, the files, depends on the 32-bit libraries
makepkg
pkexec pacman -U sakura-brother-dcpt510w-1.0.1-0-x86_64.pkg.tar.zst
```

Packages also run install scripts, which the PKGBUILD has to redo. Brother's all do the same thing (set up the CUPS
files), so Brother's are converted. Any other maker's scripts could do anything, so they're not converted: Arch goes
on to the AUR for those.

## CUPS 3

CUPS 3 drops the classic driver files (PPDs) that routes 1 to 5 install; drivers come as Printer Applications
instead, which CUPS sees as driverless printers. When Sakura Print finds CUPS 3 or later, it skips routes 1 to 5 and
uses 6, then 7. Sakura Print already sends everything as ordinary print jobs to a CUPS queue, so printing itself
doesn't change.

## Check for updates

A button, never automatic:

- **Maker's packages** (routes 2 and 3): the lookup again (Brother), or the maker's site (others). Newer than
  installed (`dpkg -s`, `rpm -q`, `pacman -Q`) → "Update available" → the licence again only if its text changed.
- **Distribution packages and Printer Applications** (routes 1, 5, 6): "comes with your normal system update".
  Sakura Print never updates one package on its own: on Arch that's a partial upgrade, which can break the system.
- **AUR:** `yay -S <package>` in a visible terminal.

## Makers' download addresses

Only addresses are kept, never drivers. An address is a fact, like a phone number.

- HTTPS only, to the maker domains in the data file. Redirects anywhere else are refused.
- Brother publishes no checksums, so the first download's sha256 is recorded per file name. If the same file later
  comes back different, it's refused with a warning.
- A guided download is checked before installing: the package's own vendor field must name the maker.
- If a maker moves its files, that route is skipped and the next one tried; nothing breaks.

## Checked on 2026-09-26

Brother's lookup and downloads, and the licence steps in Brother's installer. Fedora `hplip` declaring its printers.
`lpinfo --device-id` matching the DCP-T510W. OpenPrinting's Printer Applications on the Snap Store. Package names on
Debian 13, Ubuntu 24.04, Fedora (rawhide), openSUSE Tumbleweed and Arch:

| Driver | Debian / Ubuntu | Fedora | openSUSE | Arch |
|---|---|---|---|---|
| HP | `hplip` (+ `printer-driver-hpcups`) | `hplip` | `hplip` | `hplip` |
| Epson escpr | `printer-driver-escpr` | not packaged | `epson-inkjet-printer-escpr` | AUR |
| gutenprint | `printer-driver-gutenprint` | `gutenprint-cups` | `gutenprint` | `gutenprint` |
| brlaser | `printer-driver-brlaser` | not packaged | `printer-driver-brlaser` | AUR |
| splix | `printer-driver-splix` | `splix` | `splix` | AUR |
| foo2zjs | `printer-driver-foo2zjs` | `foo2zjs` | not packaged | AUR |
| Makers' PostScript files | `openprinting-ppds`, `foomatic-db` | `foomatic-db` | `OpenPrintingPPDs` | `foomatic-db`, `foomatic-db-nonfree` |
| P-touch labels | `printer-driver-ptouch` | `ptouch-driver` | not packaged | AUR |

These are the starting hints in the data file, each checked on the computer before use.

## Scanners, and HP's plugin

A printer that scans gets the same treatment: if scanning doesn't work yet, **the maker's scanner driver** (Brother's
list names it, `SCANNER_DRV=brscan4`, and `infs/brscan4.lnk` names the 64-bit `.deb`/`.rpm`; licence first; then it's
registered with the maker's own tool by the printer's network name, `brsaneconfig4 -a … nodename=BRW…`, which survives
a new address), else **driverless scanning** (`sane-airscan`, when the printer announces eSCL), else the AUR on Arch.
On Arch, files a maker puts under `/usr/lib64`, `/lib`, `/sbin` are moved to where Arch keeps them.

Some HP models need a closed part from HP on top of `hplip`. `hplip`'s own model list says which (`plugin=1` needed,
`plugin=2` optional); after `hplip`, Sakura Print offers HP's own `hp-plugin` tool, which shows HP's licence and
fetches it, in a terminal window. Whether it's installed is read from hplip's `/var/lib/hp/hplip.state`.

## Status

Built in 0.20.0: all eight printer routes, the scanner routes, HP's plugin step, the Arch packaging of makers'
`.deb`s, licence screens, fingerprints, updates for Brother's printer and scanner drivers, `sakuraprint doctor
drivers`, the hints file and its override. Tested on pretend Debian, Fedora, Arch, NixOS and CUPS 3 computers; the
real Brother printer and scanner packages packaged for Arch (built, not installed); `doctor` on a real Arch computer.
Tested for real (0.20.1), on Arch with a Brother DCP-T510W: its printer and scanner drivers (from the AUR) were
removed, then installed again through Sakura Print from Brother's server: licence, packaging for Arch, one password
window, the print queue kept (Brother's extra one removed), Brother's 32-bit filter printing through `cupsfilter`,
and a real scan through Brother's scanner driver. That found five problems the pretend computers couldn't, all fixed
and now tested: the queue's name was read from the script's own echoed text; the scanner was registered by a bare
name that home networks don't resolve (now the first of name, `name.local`, address that does); the scanner check
counted the driverless scanner that was already there; a queue whose driver was removed still looked set up; and a
new driver reset the queue's defaults (the paper went to Letter). Paper size for new queues comes from an explicit
setting, `/etc/papersize`, then the time zone's country (not the language: `en_US` is common outside the US).

## What it will not do

- Download from mirrors, driver collection websites, or anything but the maker, the distribution, OpenPrinting or the AUR
- Install anything silently, in the background, or because a phone asked
- Keep a copy of a driver to share it with anyone
- Get around a maker's login, download wall or licence page
