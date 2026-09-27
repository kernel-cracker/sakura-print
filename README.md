# Sakura Print
**A friendly print, scan and copy app for your home printer. Use it on the computer, from any phone on your WiFi, or from any app's own Print button.**

Made for people who just want to print: big buttons, plain words, pictures instead of manuals. It works with any printer your Linux
computer can print to (HP, Epson, Canon, Brother, Samsung, Kyocera, office lasers…), on any major Linux, and gets the printer
maker's own driver for you when there is one.

> ### Honest disclaimer: this is VIBE CODED
> Sakura Print was built by me (PulseRoot) together with an AI (Claude). I planned the features, made the design calls and tested
> it; Claude wrote most of the code.
>
> What is tested, every time (`./test.sh`):
> - The Go server: PINs, sessions and login limits, phone access, printing and page order, paper savers, every file format, the
>   editor's autosave, Print again, print from any app (the IPP protocol, Apple and PWG raster, the printer's options, the flip),
>   the notifications, and printer driver setup on pretend Debian, Fedora, Arch, NixOS and CUPS 3 computers
> - The whole web app in a headless browser, tapped and dragged like a person would, on a phone and on a computer, often checking
>   the saved pictures pixel by pixel (over 200 checks)
> - The installer's build tuning, on pretend processors
>
> Tested on real hardware (a Brother DCP-T510W on Arch Linux, the printer this was built for): printing, the double-sided page
> order on real paper, scanning, removing and reinstalling its printer and scanner drivers through the app, and CUPS's own IPP
> conformance suites for print from any app.
>
> **Not** tested yet: print from any app with a real iPhone or Android phone, other printers on real hardware, and installing on
> distributions other than Arch and Ubuntu for real. Found a bug? Open an issue, be nice, I'm still learning.
>
> Not affiliated with any printer maker. Makers' names are only used to say what works.

> [!WARNING]
> **Not recommended on apartment, hostel, office or any other shared WiFi.** Phones talk to Sakura Print over plain HTTP (no
> encryption), so someone else on the same WiFi could in theory see a PIN when a phone logs in, and the files you print. Use it on
> your own home WiFi. If you must share the WiFi, keep phone access off and turn it on for an hour when someone needs it.

---

## What it can do

| | |
|---|---|
| **First-time setup** | Opens by itself the first time: finds your printer (and gets its driver), shows what it can do, asks how its paper goes in and out (with pictures), explains both sides, sets up phones and PINs and print from any app, then a tour of every function. Phones get their own short tour |
| **Print documents** | PDFs and pictures from the computer or a phone, in any order, only some pages, copies, colour or black & white, paper size, paper type and quality in the printer driver's own words |
| **Print photos** | 1, 2, 4 or 9 to a page, edge to edge or whole, borderless photo paper, or **arrange them yourself** on the page |
| **Print text** | Type or paste anything, in any language (Kannada, Hindi, emoji…), with sizes from small to huge |
| **Both sides** | Printers that print both sides by themselves just do it. The others: it prints one side, shows you with pictures how to turn the paper over, prints the other side, and everything comes out in order. A 2-sheet setup learns how your printer's paper goes |
| **Print from any app** | The printer shows up in every app's own Print button: **AirPrint** on iPhones, the built-in printing on **Android**, Chromebooks, Macs and Linux. The phone offers the printer's real paper sizes (borderless too), paper types, quality, colour and both sides. Double-sided from a phone: the computer shows a notification with a **Print the other side** button, and the phone's print queue says to turn the paper over. Phones that logged in with a PIN print straight away; prints from others wait until you allow them |
| **Notifications** | The bell: turning paper over, out of paper, paper jams, low ink, prints waiting to be allowed, newer drivers, phone access ending soon. A tap deals with it |
| **Printers & drivers** | Finds printers on the WiFi and USB and gets each one the best driver: the maker's own first (from your distribution, or from the maker), then community drivers, then driverless. On small inkjets the maker's driver is often much faster. Scanners too. Makers' licences are shown first; the computer's password is asked once. **Check for driver updates** |
| **Scan and copy** | Scan to PDF or pictures, save them, download them to a phone, print them. **Copy** scans and prints in one tap. No scanner? The **camera scanner** finds the page in a photo, straightens and cleans it |
| **Edit any page** | A photo editor for any photo, scan, or page inside a PDF: crop, straighten and perspective, adjust, filters, and markup (pen, highlighter, text, signature, speech bubbles, shapes, magnifier) in any colour. Every change is saved as you go, with undo |
| **Save paper** | 2 or 4 pages to a sheet, booklets, leaving out blank pages |
| **Print again** | Everything you print is kept for a month: one tap prints it the same way again |
| **Every kind of file** | iPhone and Android photos (HEIC, AVIF), WebP, GIF, BMP, TIFF, SVG, and with LibreOffice: Word, Excel, PowerPoint, OpenDocument, RTF, text and CSV |
| **Phones** | Open it on any phone on the same WiFi; everyone gets their own PIN. Phone access can be always on, off, or on for 1 or 3 hours (then off by itself) |
| **Printer care** | Status, stuck jobs, ink levels (if the printer says), print head cleaning, test page |
| **Advanced** | What's going on under the hood: the print queues and their drivers, scanners, print from any app, the tools found, the server's log, and a report to copy for a bug report (no secrets in it) |

---

## Install

You need Linux, and a printer that's switched on (Sakura Print can set it up; if the computer already prints to it, even
better).

1. Download `sakuraprint-v0.21.2.zip` into your **Downloads** folder
2. Open a terminal (`Ctrl + Alt + T`), paste this (`Ctrl + Shift + V`) and press Enter:

```bash
cd ~/Downloads && rm -rf sakuraprint && unzip -o sakuraprint-v0.21.2.zip && cd sakuraprint && ./install.sh && cd ~/Downloads && rm -rf sakuraprint sakuraprint-v*.zip
```

It asks for your password once, for installing the tools (nothing shows while you type it, that's normal). Then it **builds
Sakura Print on your computer**, tuned to your processor (about half a minute), and starts it. Open **Sakura Print** from your
app menu: setup starts by itself.

What the installer does:
- installs the tools it needs with your package manager (Debian/Ubuntu/Mint, Fedora, openSUSE, Arch/Manjaro), including Go to
  build with
- builds Sakura Print from the source, for your processor (see [docs/design/build.md](docs/design/build.md)); nothing ready-made
  is downloaded
- adds **Sakura Print** to your app menu, and starts it in the background when you log in, so phones can use it any time

To print **Word, Excel, PowerPoint and text files**, add `--office` (it installs LibreOffice, which is big).

<details>
<summary>What gets installed on my distro?</summary>

| Distro | Building | Printing | Scanning | Photo formats | Print from any app | Office (`--office`) |
|---|---|---|---|---|---|---|
| Debian, Ubuntu, Mint | `golang-go` | `qpdf librsvg2-bin cups-client poppler-utils` | `sane-utils sane-airscan` | `libheif-examples imagemagick` | `avahi-utils` | `libreoffice-writer -calc -impress` |
| Fedora | `golang` | `qpdf librsvg2-tools cups-client poppler-utils` | `sane-backends sane-airscan` | `libheif-tools ImageMagick` | `avahi-tools` | `libreoffice-writer -calc -impress` |
| openSUSE | `go` | `qpdf rsvg-convert cups-client poppler-tools` | `sane-backends sane-airscan` | `ImageMagick` (+ `heif-examples`)¹ | `avahi-utils` | `libreoffice-writer -calc -impress` |
| Arch, Manjaro, EndeavourOS | `go` | `qpdf librsvg cups poppler` | `sane sane-airscan` | `libheif imagemagick` | `avahi` | `libreoffice-fresh` |

If your distribution's Go is older than the code needs, Go fetches the right version from the Go project by itself, checked
against Go's public checksum database.

¹ openSUSE's own libheif can't decode iPhone HEIC photos (that codec came from Packman, [planned to shut down at the end of
2026](https://forums.opensuse.org/t/heif-libraries-related-to-the-packman-discontinued-from-jan-1st-2027-issue/195802)).
Photos sent from the iPhone's photo picker still work: the iPhone converts them itself.
</details>

<details>
<summary>Other install options</summary>

```bash
./install.sh --no-deps           # skip the package manager (Go and the tools must be there already)
./install.sh --print-build-env   # show how it would be built for this processor
./install.sh --uninstall         # remove it (settings and scans stay)
```

Runs great on a Raspberry Pi 4 or 5 as an always-on print server.
</details>

---

## Using it

### The first time
Open **Sakura Print** from the app menu. Setup walks you through everything:

1. **Your printer**: pick it, or let Sakura Print find it and get its maker's driver
2. **What it can do**: colour, both sides (by itself or with your help), a scanner
3. **How the paper goes in and out**, with pictures: so every picture in the app matches your printer
4. **Both sides**: how turning the paper over works, and the 2-sheet setup that learns your printer
5. **Phones**: the address to open, phone access, a PIN for each person
6. **Print from any app**, and how double-sided prints from phones are finished
7. **A tour** of every function

It's all in **Settings** afterwards, and **Settings → Run the setup and the tour again** goes through it again.

### On a phone
1. Make sure the phone is on the **same WiFi** as the computer
2. Open the address from the computer's home screen (like `http://192.168.0.100:8632`) in the phone's browser
3. Type your **PIN** (once)
4. Make it an app: **iPhone**: in Safari, **Share → Add to Home Screen**. **Android**: in Chrome, **⋮ → Add to Home screen**

### Printing from other apps (AirPrint, Android)
In any app: **Share → Print** (iPhone) or **⋮ → Print** (Android), and pick **Sakura Print**. The phone offers the printer's own
paper sizes, paper types, quality, colour and both sides. A phone that logged in to Sakura Print with its PIN prints straight
away; prints from other phones (a guest, a neighbour) wait on the computer's home screen, with a picture, until you allow them.

**Both sides from a phone**, when the printer can't do it by itself: side one prints, then the computer shows a notification
with a **Print the other side** button, the phone's print queue says to turn the paper over, and the bell has it too. Turn the
stack over as the pictures show, put it back, tap the button.

How safe is it? AirPrint can't ask for a PIN, so the computer knows each phone by its WiFi hardware address (learned when it
logs in with a PIN). That keeps random neighbours out, not a determined attacker, and all an attacker could do is print. With
PINs switched off, any phone on the WiFi can print. It can be switched off in **Settings → Phones**.

### The notifications (the bell)
The bell at the top counts what needs you: paper to turn over, the printer out of paper or jammed, low ink, prints waiting to be
allowed, a newer driver, phone access switching off soon. Tap one to deal with it; anything that isn't urgent can be put away.

### Both sides by hand
Pick **Both**. Side one prints; the screen shows, with numbered pictures drawn for your kind of printer, how to take the stack,
turn it over and put it back; then **Print the other side**. Not ready? **Later**: it waits in the bell.

### Editing a page
Tap **Edit this page** on the print screen, on any photo, scan, or page of a PDF. Everything is saved as you go: close it, lose
the WiFi or restart the computer, and it picks up where you left off.

### Scanning and copying
**Scan** scans one or many pages into one PDF or pictures, to save, download to a phone, or print. **Copy** scans and prints in
one go. No scanner: **Print → Documents → Scan with the camera**.

### Printers & drivers (on the computer)
**Settings → Printers & drivers** lists the printers found on the WiFi and USB and how each is set up, and gets the best driver:
one already installed; the maker's own from your distribution (HP's `hplip`, Epson's `escpr`…) or from the maker's server
(Brother); the maker's website (you download, Sakura Print installs); the AUR on Arch; open drivers (gutenprint…); or driverless.
The maker's licence is shown before anything is installed, and your password is asked in the system's own window. Details and
the rules that keep it legal: [docs/design/drivers.md](docs/design/drivers.md).

### Advanced
**Settings → Advanced** shows what's going on under the hood, for fixing problems, and **Copy the report** makes a text to paste
into a bug report. It never contains PINs, logins or phones, and WiFi addresses are blanked out.

---

## Troubleshooting

**The phone can't open the address**
- Is the phone on the same WiFi as the computer (not mobile data), and the computer on and awake?
- Is phone access on? (the home screen's **Phones** switch)
- A firewall: `sudo ufw allow 8632/tcp`
- Some routers keep devices apart ("AP/client isolation", common on guest networks): use your main WiFi

**A phone doesn't show "Sakura Print" when printing from another app**
- Is **Print from any app** on in **Settings → Phones**, and phone access on?
- Printed but nothing came out? It may be waiting to be allowed: look at the bell. Log in once on that phone with its PIN, and
  it won't have to wait again
- Bonjour must be running: `sudo systemctl enable --now avahi-daemon`, and with a firewall: `sudo ufw allow 5353/udp`
- **Settings → Advanced** shows whether it's announced on the WiFi

**"No scanner found"**
- Is the printer on and on the same WiFi? **Settings → Printers & drivers** offers the maker's scanner driver, or driverless
  scanning (sane-airscan). In a terminal, `scanimage -L` shows what Linux sees

**Double-sided pages come out mixed up, or one sheet off**
Almost always the printer grabbing **two sheets at once** on the second side. Before putting the stack back, **fan it** and
**count the sheets** (the flip screen says how many). Still mixed up? Run **Printer → Both-sides setup** again, and turn the stack
the same way every time.

**Printing is slow, or looks soft**
On small inkjets, driverless printing makes the printer do all the work. **Settings → Printers & drivers** gets the maker's
driver, which is often much faster and sharper.

**Something else**
**Settings → Advanced** shows what's going on; **Copy the report** and open an issue with it.

---

## Good to know

- **Privacy**: everything stays on your computer and your WiFi. Sakura Print only goes on the internet to download a printer
  driver from its maker when you ask (and to fetch the Go compiler when building, if yours is too old). Don't open port 8632 on
  your router
- **PINs** are stored scrambled (salted PBKDF2), never as they are. A phone that gets 5 wrong PINs waits 10 minutes; after 30
  wrong PINs in an hour from anyone, new logins pause for a while. Only the computer can change PINs or phone access
- **Files**: uploads are kept 2 days, scans in the app 7 days, editor autosaves 30 days after their last change, Print again the
  last 30 for a month. Saved scans in `~/Documents/Scans` are yours forever

| What | Where |
|---|---|
| Program | `~/.local/bin/sakuraprint` |
| Settings (PINs hashed, phones, driver licences agreed to) | `~/.config/sakuraprint/settings.json` |
| Driver hints of your own | `~/.config/sakuraprint/drivers.json` |
| Both-sides setup | `~/.config/pdftool/printers` |
| Saved scans | `~/Documents/Scans` |
| Everything else (uploads, autosaves, Print again, downloaded drivers) | `~/.local/share/sakuraprint` |

```bash
sakuraprint                  # open the app window
sakuraprint phone            # show the phone address
sakuraprint doctor drivers   # what each way of getting a printer driver finds on this computer
sakuraprint version          # the version, and how this copy was built
```

---

## For developers

One Go program (standard library only) with the web app embedded, and plain HTML, CSS and JavaScript with no build step. How it
all works, file by file: **[docs/design](docs/design/README.md)**.

```bash
go build -o sakuraprint . && ./sakuraprint serve   # then open http://localhost:8632
./test.sh                                         # every test (see docs/design/testing.md)
```

Tests come first: new work starts with a test that fails, bugs get a test that fails without the fix, and a failing test is
fixed in the code, not loosened.

## Credits

Made by PulseRoot, vibe coded with Claude, powered by [qpdf](https://github.com/qpdf/qpdf), [librsvg](https://gitlab.gnome.org/GNOME/librsvg), [Poppler](https://poppler.freedesktop.org/), [CUPS](https://openprinting.github.io/cups/), [SANE](http://www.sane-project.org/), [sane-airscan](https://github.com/alexpevzner/sane-airscan), [Avahi](https://avahi.org/), [Cropper.js](https://github.com/fengyuanchen/cropperjs) (MIT, © Chen Fengyuan) and [Fabric.js](https://github.com/fabricjs/fabric.js) (MIT, © Printio / Juriy Zaytsev, Maxim Chernyak). Test driver files: Brother's DCP-T510W driver file (redistributable under Brother's licence) and small made-up HP-like and Epson-like ones.

## License

Sakura Print is free software, licensed under the **GNU General Public License, version 3 or (at your option) any later version** (GPL-3.0-or-later), see [LICENSE](LICENSE). Copyright (C) 2026 PulseRoot.

In short: you can use it, change it and share it, but if you share it (changed or not), you have to share the source code too, under the same license. And if you put it on a device you sell, people must be able to install their own changed version on that device. Nobody gets to turn it into a closed box.

Cropper.js and Fabric.js in `web/vendor/` stay under their own MIT licenses ([cropper.LICENSE.txt](web/vendor/cropper.LICENSE.txt), [fabric.LICENSE.txt](web/vendor/fabric.LICENSE.txt)), which are compatible with the GPL. The app as a whole is shared under the GPL.
