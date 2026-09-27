# Sakura Print: how it's built

Sakura Print is one Go program (`package main`, standard library only) with the whole web app embedded in it. It
runs on the computer the printer is connected to, as the person (never as the administrator), and serves:

- the **web app** to the computer's own browser window and to phones on the WiFi (`http://<computer>:8632`);
- a **network printer** (IPP, for AirPrint and Android's built-in printing) at `/ipp/print`, announced with Bonjour;
- a **command line**: `sakuraprint` (open the window), `serve`, `phone`, `doctor drivers`, `version`.

It never talks to the internet except to download printer drivers from their makers when the person asks
([drivers.md](drivers.md)).

## The design documents

| Document | What it covers |
|---|---|
| [server.md](server.md) | Starting up, every HTTP route and who may call it, background work, caching |
| [printing.md](printing.md) | From files to paper: the job spec, building PDFs, page order, double-sided by hand, paper savers, Print again |
| [security.md](security.md) | PINs, sessions, phone access, the computer vs phones, cross-site and DNS-rebinding protection |
| [ipp.md](ipp.md) | Print from any app: the IPP printer, raster formats, who may print, Bonjour |
| [drivers.md](drivers.md) | Printers & drivers: finding printers and scanners, every way of getting a driver, legally |
| [scanning.md](scanning.md) | Scanners (SANE), the camera scanner, copying |
| [web-app.md](web-app.md) | The browser side: screens, the photo editor, the pixel engine, design rules |
| [notices.md](notices.md) | The notifications centre: flips, held prints, printer problems, updates |
| [build.md](build.md) | Building from source on each computer, tuned to its processor; the installer |
| [testing.md](testing.md) | The test suites, the pretend computer, and how to add a test first |

## The code, file by file

All Go files are in the top folder, one package. Names group them:

| Files | Area |
|---|---|
| `main.go` | `main()`, the command line, opening the app window |
| `server.go` | Starting the server, cleaning old files, the embedded web app, uploads |
| `api_*.go` | The HTTP routes, one file per area (`api_info`, `api_files`, `api_print`, `api_printer`, `api_scan`, `api_session`) |
| `settings.go` | Paths (`~/.config/sakuraprint`, `~/.local/share/sakuraprint`) and the settings file |
| `security.go`, `devices.go` | PINs, sessions, login limits, phone access; phones allowed to print from any app |
| `files.go`, `finder.go`, `convert.go`, `thumbs.go`, `images.go` | Files people add; the computer's files; every file format; thumbnails; fast image shrinking |
| `pdf.go`, `pdf_build.go`, `pdf_photos.go`, `pdf_covers.go`, `pdfwrite.go`, `impose.go` | Building print-ready PDFs |
| `jobs.go`, `history.go` | Print jobs and the flip; Print again |
| `cups.go`, `caps.go`, `profiles.go`, `options.go` | CUPS: printers, options, status, maintenance; what a printer can do; both-sides profiles; which driver option does what, for any maker |
| `scan.go` | Scanning with SANE |
| `edits.go` | The photo editor's autosave |
| `ipp.go`, `ipp_wire.go`, `ipp_printer.go`, `ipp_jobs.go`, `ipp_raster.go`, `ipp_options.go`, `bonjour.go` | Print from any app |
| `drivers.go`, `drivers_find.go`, `drivers_plan.go`, `drivers_run.go`, `drivers_api.go`, `drivers_pkg.go`, `drivers_scan.go`, `drivers_sys.go` | Printers & drivers |
| `notices.go` | The notifications centre |
| `debug.go` | Advanced: the diagnostics page and report |
| `assets/` | Cover design, app icon, driver hints (`drivers.json`) |
| `web/` | The web app (see [web-app.md](web-app.md)) |

## Where things are kept

| What | Where |
|---|---|
| Settings, PINs (hashed), sessions, phones, driver licences | `~/.config/sakuraprint/settings.json` (mode 600) |
| Driver hints of your own | `~/.config/sakuraprint/drivers.json` |
| Both-sides profiles | `~/.config/pdftool/printers` (shared with the old pdftool) |
| Uploads (2 days), scans (7 days), builds (6 hours) | `~/.local/share/sakuraprint/{uploads,scans,work}` |
| Editor autosaves and originals (30 days) | `~/.local/share/sakuraprint/edits` |
| Converted files (a week) | `~/.local/share/sakuraprint/converted` |
| Print again (last 30, 30 days) | `~/.local/share/sakuraprint/history` |
| Downloaded drivers | `~/.local/share/sakuraprint/drivers` |
| Saved scans | `~/Documents/Scans` (the person's, never cleaned) |

## Conventions

- **Standard library only** in Go; outside programs are called as commands (CUPS `lp`/`lpstat`/`lpadmin`, `qpdf`,
  `pdftoppm`, `scanimage`, `avahi-publish-service`, image converters) and every call has a timeout.
- **Plain words** in everything a person reads: comments too are written for someone new to the code.
- **Slow questions are cached** (`cached[T]` in `cups.go`): printers, their options, status, scanners. The first
  caller waits, later ones get the last answer at once while it refreshes.
- **The administrator only through polkit** (`pkexec`), one password window per task, never stored.
- **Tests before code** for new work: see [testing.md](testing.md).
