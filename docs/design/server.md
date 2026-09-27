# The server

`sakuraprint serve` (`server.go: serve`) starts one HTTP server on port 8632 (all addresses, so phones on the WiFi
can reach it). The installer makes it start when the person logs in (`~/.config/autostart`).

## Starting up

1. Load the settings (migrating old formats: plain PINs become hashed ones, expired sessions and phones go).
2. Clear what's left of the last run (`work/`, thumbnails) and clean old uploads, scans, edits, converted files,
   Print again entries.
3. In the background: ask CUPS the slow questions (printers, options, status, scanners) so the first phone doesn't
   wait; index the home folder's PDFs and pictures (`finder.go`, again every 10 minutes); every 30 minutes drop
   builds and jobs older than 6 hours.
4. Register the routes (`api_*.go`, then `edits.go`, `history.go`, `drivers_api.go`, `ipp.go`, `notices.go`,
   `debug.go`) and wrap everything in `protect()` (security headers, cross-site protection, phone access).

## Who may call what

Three kinds of caller (see [security.md](security.md)):

- **the computer**: a request from this computer, addressed to `localhost` (`isLocal`);
- **a phone that logged in** with a PIN (session cookie), or any phone when PINs are off;
- **anyone** on the WiFi.

| Route | Methods | Who | What |
|---|---|---|---|
| `/` and the app's files | GET | anyone (phone access on) | The web app, embedded; `app.js` is `web/js/*.js` joined in order |
| `/api/login` | POST | anyone | PIN → session cookie. 5 wrong tries per phone per 10 minutes, 30 per hour in all |
| `/api/info` | GET | logged in | Version, printers, default printer, theme, scanning available, the computer's addresses; for the computer also PINs and phone access |
| `/api/settings` | GET, POST | logged in; security fields only the computer | Theme, favourite printer; PINs, phone access, print from any app, phones allowed |
| `/api/options` | GET | logged in | A printer's driver options (paper, quality, colour…) |
| `/api/printer` | GET | logged in | A printer's status, jobs, ink, maintenance commands, both-sides profile |
| `/api/maintenance` | POST | logged in | Cancel all, clean the print head, self-test page |
| `/api/test`, `/api/profile` | POST | logged in | The 2-sheet both-sides setup: print the test, save the answers |
| `/api/upload`, `/api/local`, `/api/local/add` | POST, GET | logged in | Add files: from the phone, or from the computer's folders |
| `/api/thumb/`, `/api/file/`, `/api/pageimage`, `/api/replacepage`, `/api/pdf/`, `/api/download/` | GET, POST | logged in | Thumbnails, file contents, a page as a picture, an edited page back into a PDF, downloads |
| `/api/edit/` | GET, POST, DELETE | logged in | The editor's autosave |
| `/api/build`, `/api/preview/` | POST, GET | logged in | Build the print-ready PDF from a job spec; preview pictures |
| `/api/print`, `/api/job/` | POST, GET | logged in | Send it; follow it; print the other side; cancel |
| `/api/jobs/waiting` | GET | logged in | Prints from other apps waiting for the flip |
| `/api/history`, `/api/history/` | GET, POST, DELETE | logged in | Print again |
| `/api/scanners`, `/api/scan`, `/api/scan/save` | GET, POST | logged in | Scan; save scans as PDF or pictures |
| `/api/airprint/waiting`, `/api/airprint/waiting/` | GET, POST | logged in | Prints from unknown phones: allow, allow always, refuse |
| `/api/drivers`, `/api/drivers/plan`, `/api/drivers/run`, `/api/drivers/job/`, `/api/drivers/updates` | GET, POST | the computer only | Printers & drivers |
| `/api/notices` | GET, POST | logged in (some notices only for the computer) | The notifications centre |
| `/api/debug`, `/api/debug/report` | GET | the computer only | Advanced: diagnostics |
| `/ipp/print` | POST | anyone with phone access, when print from any app is on | The IPP printer (see [ipp.md](ipp.md)) |

## Background work and state

- `builds` and `jobs` (`stateMu`): built PDFs and print jobs, in memory, dropped after 6 hours.
- `settings` (`settingsMu`): read at start, written on every change (to a temporary file, then renamed).
- The IPP server's jobs (`ippSrv.mu`), driver jobs (`drvJobsMu`, one install at a time: `drvBusy`).
- `cached[T]` values for everything slow CUPS or SANE answers.

Lock order, where two are held: a job's own lock, then `stateMu` or `settingsMu`; never the other way round.

## Adding a route

Put it in the `api_*.go` file for its area as `api("/api/…", handler)` (logged-in callers) or
`mux.HandleFunc` (anyone). Anything that changes the computer, or shows something only the computer should see,
checks `isLocal(r)`. Answer with `writeJSON` or `fail(w, code, "a sentence a person understands")`.
