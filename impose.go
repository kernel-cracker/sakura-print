// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Paper savers: 2 or 4 pages on each sheet, booklets (print, fold in half, read like a little book), and
// leaving out blank pages. Each page is drawn as a picture at print resolution (300 dpi on the sheet) and the
// pictures are placed on A4 sheets by the tiny PDF writer, so no extra tools are needed.
//
// Sheets are portrait A4. Two pages per sheet sit top and bottom, turned a quarter (read with the sheet held
// sideways). Booklets and double-sided two-per-sheet need the paper flipped on its short edge, while printers
// (and the hand-flip) flip on the long edge: so the back of each such sheet is turned right round (180°).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	sheetW, sheetH = 210.0, 297.0 // A4, mm
	sheetMargin    = 6.0
	sheetGap       = 6.0
)

// cell is one page's spot on a sheet: which page (1-based, 0 = leave empty), where, turned how far
type cell struct {
	page       int
	x, y, w, h float64
	rot        int
}

// layoutSheets says which page goes where on each side of each sheet, for n pages
func layoutSheets(layout string, n int, duplex bool) [][]cell {
	m, g := sheetMargin, sheetGap
	halfH := (sheetH - 2*m - g) / 2
	top := func(p, rot int) cell { return cell{p, m, m, sheetW - 2*m, halfH, rot} }
	bottom := func(p, rot int) cell { return cell{p, m, m + halfH + g, sheetW - 2*m, halfH, rot} }
	var sides [][]cell
	switch layout {
	case "2up":
		for k := 0; 2*k < n; k++ {
			a, b := 2*k+1, 2*k+2
			if b > n {
				b = 0
			}
			if duplex && k%2 == 1 { // a back: the whole side turned right round
				sides = append(sides, []cell{top(b, 270), bottom(a, 270)})
			} else {
				sides = append(sides, []cell{top(a, 90), bottom(b, 90)})
			}
		}
	case "4up":
		cw := (sheetW - 2*m - g) / 2
		for k := 0; 4*k < n; k++ {
			var s []cell
			for i := 0; i < 4; i++ {
				p := 4*k + i + 1
				if p > n {
					p = 0
				}
				s = append(s, cell{p, m + float64(i%2)*(cw+g), m + float64(i/2)*(halfH+g), cw, halfH, 0})
			}
			sides = append(sides, s)
		}
	case "booklet":
		// pages padded to a multiple of 4; sheet i (from the outside in): front = last-2i | 2i+1, back = 2i+2 | last-2i-1
		N := (n + 3) / 4 * 4
		pg := func(p int) int {
			if p > n {
				return 0
			}
			return p
		}
		for i := 0; i < N/4; i++ {
			sides = append(sides,
				[]cell{top(pg(N-2*i), 90), bottom(pg(2*i+1), 90)},
				[]cell{top(pg(N-2*i-1), 270), bottom(pg(2*i+2), 270)})
		}
	}
	return sides
}

// impose makes the paper-saving version of a PDF. preview: small pictures, for the screen.
func impose(src, layout string, duplex, preview bool, dir string) (string, error) {
	n := npages(src)
	if n < 1 {
		return "", fmt.Errorf("can't read the pages")
	}
	sides := layoutSheets(layout, n, duplex)
	if len(sides) == 0 {
		return src, nil
	}
	dpi := 200 // a half-sheet page at 200 dpi prints at about 280 dpi
	if layout == "4up" {
		dpi = 150
	}
	if preview {
		dpi /= 4
	}
	rdir := filepath.Join(dir, "pages-"+layout)
	os.MkdirAll(rdir, 0o755)
	// all pages in one go (much faster than one program start per page)
	if o, err := exec.Command("pdftoppm", "-jpeg", "-jpegopt", "quality=90", "-r", strconv.Itoa(dpi), src, filepath.Join(rdir, "p")).CombinedOutput(); err != nil {
		return "", fmt.Errorf("drawing the pages: %s", bytes.TrimSpace(o))
	}
	// pdftoppm numbers files with as many digits as the page count needs: p-1.jpg or p-01.jpg ...
	digits := len(strconv.Itoa(n))
	pics := map[int]*imgInfo{}
	for p := 1; p <= n; p++ {
		im, err := loadImage(filepath.Join(rdir, fmt.Sprintf("p-%0*d.jpg", digits, p)))
		if err != nil {
			return "", fmt.Errorf("page %d: %v", p, err)
		}
		pics[p] = im
	}
	var pages []picPage
	for _, s := range sides {
		pg := picPage{w: sheetW, h: sheetH}
		for _, c := range s {
			if c.page == 0 {
				continue
			}
			pg.items = append(pg.items, placement{im: pics[c.page], x: c.x, y: c.y, w: c.w, h: c.h, fill: false, rot: c.rot})
		}
		pages = append(pages, pg)
	}
	out := filepath.Join(dir, "imposed-"+layout+".pdf")
	if err := writePicturePDF(pages, out); err != nil {
		return "", err
	}
	return out, nil
}

// blankPages finds pages that are (almost) entirely white, from a tiny grey drawing of each page
func blankPages(src string, n int, dir string) map[int]bool {
	bdir := filepath.Join(dir, "blankcheck")
	os.MkdirAll(bdir, 0o755)
	if err := exec.Command("pdftoppm", "-gray", "-r", "18", src, filepath.Join(bdir, "b")).Run(); err != nil {
		return nil
	}
	digits := len(strconv.Itoa(n))
	blank := map[int]bool{}
	for p := 1; p <= n; p++ {
		data, err := os.ReadFile(filepath.Join(bdir, fmt.Sprintf("b-%0*d.pgm", digits, p)))
		if err != nil {
			continue
		}
		px := pgmPixels(data)
		if len(px) == 0 {
			continue
		}
		ink := 0
		for _, v := range px {
			if v < 200 {
				ink++
			}
		}
		if float64(ink)/float64(len(px)) < 0.0015 { // a speck of dust or a scanner edge, not writing
			blank[p] = true
		}
	}
	return blank
}

// pgmPixels reads the grey values out of a binary PGM ("P5") file
func pgmPixels(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("P5")) {
		return nil
	}
	fields, i := 0, 2
	for fields < 3 && i < len(data) { // width, height, maxval, skipping spaces and comments
		for i < len(data) && (data[i] == ' ' || data[i] == '\n' || data[i] == '\r' || data[i] == '\t') {
			i++
		}
		if i < len(data) && data[i] == '#' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			i++
		}
		fields++
	}
	if i >= len(data) {
		return nil
	}
	return data[i+1:]
}
