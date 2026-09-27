# From files to paper

## 1. Files in

Everything becomes a **file** (`files.go`) with an id the web app uses: uploads from a phone, files from the
computer's folders (`finder.go` keeps an index of PDFs and pictures in the home folder), scans, text turned into a
PDF, edited pictures, reprints from Print again.

`convert.go` recognises files by what's inside, not their name (`sniff`), and turns them into what printing needs:
PDFs stay PDFs; JPEG/PNG stay; HEIC, AVIF, WebP, GIF, BMP, SVG become JPEG/PNG; multi-page TIFFs become a PDF;
Word, Excel, PowerPoint, OpenDocument, RTF, text and CSV become a PDF through LibreOffice (with its own throwaway
profile). Results are kept by content hash for a week.

## 2. The job spec

The web app sends a `Spec` (`pdf.go`): which files and pages, documents or photos, single or both sides, copies,
cover page, fit to paper, photo layout (per sheet, fill or fit, or placed by hand), paper savers (2-up, 4-up,
booklet, skip blank pages) and the printer's own options (`Options`, straight from its driver: paper, quality,
colour, paper type…).

## 3. Building (`pdf_build.go: build`)

1. Each document is cut to its pages, turned upright and shrunk into the printable area when *fit to paper* is on
   (`sidewaysPages`, qpdf).
2. Photos are laid out on pages (`pdf_photos.go`) and written with the small picture-PDF writer (`pdfwrite.go`),
   which puts phone JPEGs in untouched: fast, no quality loss.
3. Covers (`pdf_covers.go`) are drawn from `assets/cover.svg` in the chosen palette.
4. Paper savers (`impose.go`): pages are drawn as pictures at print resolution and placed 2 or 4 to a sheet, or as a
   booklet; blank pages are found from a tiny grey drawing of each page.
5. Everything is joined into `combined.pdf`; `Map` says which original page each printed page is (for previews).

## 4. Page order and both sides (`Built.passes`)

The printer's **profile** (`profiles.go`, from the 2-sheet setup) says how its paper behaves: face-up or face-down
output, which pages go first, whether each pass is reversed, whether backs must be turned (`Rot`).

- **The printer does both sides itself** (`Auto`, `caps.go: autoDuplex` finds its duplex option): pages go in
  their normal order, with the driver's duplex option.
- **One side**: in order, or backwards for face-up printers so the stack comes out in order.
- **Both sides by hand**: pass 1 is one parity (say the even pages), reversed or not; the person turns the stack
  over as the picture guide shows; pass 2 is the other parity. On the Brother DCP-T510W: evens first, book-flip the
  stack, odds reversed.

## 5. Printing and the flip (`jobs.go`)

`printBuilt` sends pass 1 with `lp` (`cups.go: submit`) and follows it (`Job.refresh`: is the CUPS job still in
`lpstat -o`?). States: `printing1` → `flip` (both sides by hand) → `printing2` → `done`, or `canceled`/`error`.
In `flip` the app shows the guide; "Print the other side" sends pass 2 (`/api/job/<id>/side2`). Prints from other
apps (IPP) wait in `flip` the same way; see [notices.md](notices.md) for how the person is told.

A test mode (`SAKURA_DRY_PRINT=<folder>`) writes what would be printed into a folder instead: every test uses it.

## 6. Print again (`history.go`)

After a real print, one copy of exactly what was sent (covers and all), the settings and a small preview are kept:
the last 30, for 30 days. Reprinting rebuilds the same job; "Change settings" opens the print screen with them.
