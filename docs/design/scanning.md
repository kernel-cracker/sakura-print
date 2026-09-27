# Scanning and copying

## The printer's scanner (SANE)

`scan.go` asks `scanimage -L` which scanners there are (cached for 2 minutes), and tidies their names: the same
scanner often shows up several ways (the maker's driver, eSCL, airscan) and is shown once, by model; cameras
(`v4l`) are left out. `caps.go: scannerFor` matches a scanner to a printer by model.

A scan (`scanPage`) runs `scanimage` at the chosen quality (Quick 150 dpi, Normal 300, Detailed 600) in colour or
grey, one at a time (`scanBusy`), and becomes a file like any other. Scans can be saved as one PDF or as pictures in
`~/Documents/Scans`, downloaded to a phone, printed, or edited first.

Getting a scanner working (the maker's scanner driver, driverless eSCL through `sane-airscan`, the AUR) is part of
Printers & drivers: see [drivers.md](drivers.md).

## The camera scanner (in the browser)

`web/js/13-camscan.js` and `web/develop.js`: a photo of a page is searched for the page (`findPage`: Otsu
threshold and the largest bright region's corners), straightened (`warpQuad`, a homography), cleaned up
(`developFast`, the "document" filter) or turned crisp black and white (`blackWhite`, adaptive threshold). Pages
that the phone's browser can't open (HEIC on Android) are converted by the computer first. No scanner needed.

## Copying

Scan and print in one tap, or **Preview & fix**: scan, open the editor (crop, straighten, mark up), then print.
Hidden when no scanner is found.
