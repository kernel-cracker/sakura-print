# Print from any app (IPP)

Sakura Print pretends to be a network printer so the normal Print button in any app works: AirPrint on iPhones,
the built-in printing on Android, Chromebooks, Macs and Linux (IPP Everywhere). Prints still go through Sakura's
pipeline, with the real printer's driver, so they get the right page order and double-sided printing by hand, and
they're usually faster and sharper than printing to the printer directly (which uses slow driverless printing).

## The pieces

| File | Part |
|---|---|
| `ipp_wire.go` | The message format (RFC 8010): reading requests (collections flattened to `a.b.c` names), writing answers |
| `ipp_printer.go` | The printer's attributes, from the real printer and its driver; answering each operation |
| `ipp_jobs.go` | Taking a job: who may print, holding prints from unknown phones, into Sakura's pipeline |
| `ipp_raster.go` | Pages sent as pictures: Apple raster (URF, iPhones) and PWG raster (Android, Linux) to PDF |
| `bonjour.go` | Announcing it with `avahi-publish-service` (`_ipp._tcp` with the `_universal` subtype iPhones look for) |
| `ipp.go` | Wiring it in; the API for prints waiting to be allowed |

## Operations

Print-Job, Validate-Job, Create-Job + Send-Document (what iPhones use), Close-Job, Get-Job-Attributes, Get-Jobs
(with `which-jobs` and `job-ids`), Cancel-Job, Cancel-My-Jobs, Get-Printer-Attributes, Identify-Printer. Answers
use the version asked in. Passes CUPS's `ipptool` IPP 1.1, 2.0 and IPP Everywhere suites (all but the optional
per-page overrides).

## The printer's options, from its driver

The attributes aren't made up: they come from the real printer's driver (its PPD options, through `ppdOptions`),
so phones offer what the printer can really do:

| Phone shows | IPP attributes | From the driver |
|---|---|---|
| Paper size | `media-supported`, `media-col-database` (with borderless sizes: margins 0) | `PageSize` choices, their sizes (`*PaperDimension`) |
| Paper type | `media-type-supported` | `MediaType` choices (plain, inkjet, glossy, photo…) |
| Quality | `print-quality-supported` (draft, normal, high) | the driver's quality option |
| Colour / black & white | `print-color-mode-supported` | colour capability and the colour option |
| Double-sided | `sides-supported` (long and short edge) | by itself if the printer can; otherwise by hand, with the flip |

A job's attributes go back the same way (`jobOptions`): paper, paper type, quality and colour become the driver's
own choices; `sides` becomes Sakura's both-sides (by itself, or by hand).

## Double-sided from a phone

When the printer can't print both sides itself, side 1 prints, then the job waits in the flip state:

- the phone's print queue shows the job as waiting, with the message "Turn the paper over, then print the other
  side" (`job-state-message`, and the printer's `printer-state-message`);
- the computer shows a notification with a **Print the other side** button (libnotify actions), and the guide;
- every open Sakura Print (computer or phone) shows it in the notifications centre and on the home screen.

## Who may print

The computer; any phone when PINs are off; otherwise phones that logged in with a PIN (known by their WiFi
hardware address, `devices.go`). Other phones' prints are converted and held (job state "pending-held") until
someone allows them, with a picture; at most 10 wait, for 2 hours. See [security.md](security.md).

## Testing

`ipp_test.go` (message round trips, raster decoding against a test encoder), `tests/airprint.test.mjs` (a real
IPP client, `ipptool`, as the computer, and a tiny IPP client in the test as phones), and CUPS's own suites run by
hand: `ipptool -t ipp://localhost:8632/ipp/print ipp-everywhere.test`.
