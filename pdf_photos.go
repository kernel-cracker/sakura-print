// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Building prints, photos: page sizes, loading pictures, photo layouts.

// ---------- page sizes (mm) for photo layouts ----------

var paperMM = map[string][2]float64{
	"a4": {210, 297}, "letter": {215.9, 279.4}, "legal": {215.9, 355.6}, "executive": {184.2, 266.7},
	"a5": {148, 210}, "a6": {105, 148}, "b5": {182, 257}, "b6": {128, 182},
	"postc4x6": {101.6, 152.4}, "4x6": {101.6, 152.4}, "photol": {89, 127}, "3.5x5": {89, 127},
	"photo2l": {127, 178}, "5x7": {127, 178}, "indexc5x8": {127, 203.2}, "5x8": {127, 203.2},
	"postcard": {100, 148}, "hagaki": {100, 148}, "folio": {215.9, 330.2},
}

// paperSize turns a driver page size name like "BrA4_B" or "4x6.Borderless" into mm + borderless flag
func paperSize(name string) (w, h float64, borderless bool) {
	n := strings.ToLower(name)
	borderless = strings.HasSuffix(n, "_b") || strings.Contains(n, "borderless")
	n = strings.TrimSuffix(strings.TrimSuffix(n, "_b"), "_s")
	n = strings.TrimSuffix(n, ".borderless")
	n = strings.TrimPrefix(n, "br")
	if s, ok := paperMM[n]; ok {
		return s[0], s[1], borderless
	}
	return 210, 297, borderless
}

// ---------- images ----------

type imgInfo struct {
	W, H       int // as displayed, after EXIF rotation
	RawW, RawH int // as stored in the file
	Rot        int // EXIF rotation to apply: 0, 90, 180, 270 (clockwise)
	Mime       string
	Model      string // rgb | gray | cmyk | other
	Data       []byte
	Portr      bool
}

func loadImage(path string) (*imgInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s isn't a JPEG, PNG or GIF image", filepath.Base(path))
	}
	im := &imgInfo{W: cfg.Width, H: cfg.Height, RawW: cfg.Width, RawH: cfg.Height, Mime: "image/" + format, Data: data, Model: "other"}
	switch cfg.ColorModel {
	case color.YCbCrModel, color.RGBAModel, color.NRGBAModel:
		im.Model = "rgb"
	case color.GrayModel:
		im.Model = "gray"
	case color.CMYKModel:
		im.Model = "cmyk"
	}
	if format == "jpeg" {
		switch exifOrientation(data) {
		case 3:
			im.Rot = 180
		case 6:
			im.Rot = 90
		case 8:
			im.Rot = 270
		}
	}
	if im.Rot == 90 || im.Rot == 270 {
		im.W, im.H = im.H, im.W
	}
	im.Portr = im.H >= im.W
	return im, nil
}

// exifOrientation reads the rotation tag phones put in JPEGs (1 = normal, 3/6/8 = rotated)
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if marker == 0xE1 && i+10 <= len(b) && string(b[i+4:i+10]) == "Exif\x00\x00" {
			t := b[i+10:]
			if len(t) < 8 {
				return 1
			}
			var bo binary.ByteOrder = binary.BigEndian
			if string(t[:2]) == "II" {
				bo = binary.LittleEndian
			}
			off := int(bo.Uint32(t[4:8]))
			if off+2 > len(t) {
				return 1
			}
			n := int(bo.Uint16(t[off : off+2]))
			for e := 0; e < n; e++ {
				p := off + 2 + e*12
				if p+12 > len(t) {
					return 1
				}
				if bo.Uint16(t[p:p+2]) == 0x0112 {
					return int(bo.Uint16(t[p+8 : p+10]))
				}
			}
			return 1
		}
		if marker == 0xDA {
			return 1
		}
		i += 2 + size
	}
	return 1
}

func svgPage(wmm, hmm float64, body string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" `+
		`width="%.2fmm" height="%.2fmm" viewBox="0 0 %.2f %.2f"><rect width="%.2f" height="%.2f" fill="#ffffff"/>%s</svg>`,
		wmm, hmm, wmm, hmm, wmm, hmm, body)
}

func svgsToPDF(svgs []string, dir, out string) error {
	var files []string
	for i, s := range svgs {
		f := filepath.Join(dir, fmt.Sprintf("page-%04d.svg", i))
		if err := os.WriteFile(f, []byte(s), 0o644); err != nil {
			return err
		}
		files = append(files, f)
	}
	args := append([]string{"-f", "pdf", "-o", out}, files...)
	if o, err := exec.Command("rsvg-convert", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("rsvg-convert: %s", strings.TrimSpace(string(o)))
	}
	return nil
}

// photoPDF lays photos out on sheets: perSheet 1/2/4/9, fill (edge to edge, crops) or fit (whole photo)
func photoPDF(paths []string, paper string, perSheet int, fill bool, dir, out string, preview bool) (int, error) {
	pw, ph, borderless := paperSize(paper)
	cols, rows := 1, 1
	switch perSheet {
	case 2:
		cols, rows = 1, 2
	case 4:
		cols, rows = 2, 2
	case 9:
		cols, rows = 3, 3
	default:
		perSheet = 1
	}
	margin, gap := 6.0, 4.0
	if borderless && perSheet == 1 {
		margin = 0
	}
	if perSheet > 1 && borderless {
		margin = 2
	}
	cw := (pw - 2*margin - float64(cols-1)*gap) / float64(cols)
	ch := (ph - 2*margin - float64(rows-1)*gap) / float64(rows)
	var pages []picPage
	cur := picPage{w: pw, h: ph}
	n := 0
	for i, p := range paths {
		im, err := pictureFor(p, preview)
		if err != nil {
			return 0, err
		}
		c, r := n%cols, (n/cols)%rows
		x := margin + float64(c)*(cw+gap)
		y := margin + float64(r)*(ch+gap)
		rot := im.Rot
		if im.Portr != (ch >= cw) { // turn the photo to match the frame's shape
			rot += 90
		}
		cur.items = append(cur.items, placement{im: im, x: x, y: y, w: cw, h: ch, fill: fill, rot: rot})
		n++
		if n == perSheet || i == len(paths)-1 {
			pages = append(pages, cur)
			cur = picPage{w: pw, h: ph}
			n = 0
		}
	}
	return len(pages), writePicturePDF(pages, out)
}

// customPhotoPDF draws photos exactly where they were placed in the editor
func customPhotoPDF(placed []Placed, paper string, dir, out string, preview bool) (int, error) {
	pw, ph, _ := paperSize(paper)
	pages := 0
	for _, p := range placed {
		if p.Page < 0 || p.Page > 49 {
			return 0, errors.New("too many pages in the layout (50 max)")
		}
		if p.Page+1 > pages {
			pages = p.Page + 1
		}
	}
	pgs := make([]picPage, pages)
	for i := range pgs {
		pgs[i] = picPage{w: pw, h: ph}
	}
	cache := map[string]*imgInfo{}
	clamp := func(v, lo, hi float64) float64 {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	for _, p := range placed {
		f, ok := getFile(p.ID)
		if !ok || f.Kind != "image" {
			return 0, errors.New("one of the photos isn't there anymore, add it again")
		}
		im := cache[f.ID]
		if im == nil {
			var err error
			if im, err = pictureFor(f.Path, preview); err != nil {
				return 0, err
			}
			cache[f.ID] = im
		}
		w, h := clamp(p.W, 0.02, 2), clamp(p.H, 0.02, 2)
		x, y := clamp(p.X, -1, 1.5), clamp(p.Y, -1, 1.5)
		pgs[p.Page].items = append(pgs[p.Page].items, placement{im: im, x: x * pw, y: y * ph, w: w * pw, h: h * ph, fill: !p.Whole, rot: im.Rot + (p.Rot/90)*90})
	}
	if len(pgs) == 0 {
		return 0, errors.New("put at least one photo on the page")
	}
	return len(pgs), writePicturePDF(pgs, out)
}

var (
	pageSizeRe = regexp.MustCompile(`Page\s+(\d+) size:\s+([\d.]+) x ([\d.]+)`)
	pageRotRe  = regexp.MustCompile(`Page\s+(\d+) rot:\s+(\d+)`)
)

// sidewaysPages lists the pages that are wider than tall (as they'd appear on screen)
func sidewaysPages(path string, n int) []string {
	out, err := exec.Command("pdfinfo", "-f", "1", "-l", strconv.Itoa(n), path).Output()
	if err != nil {
		return nil
	}
	wide := map[string]bool{}
	for _, m := range pageSizeRe.FindAllStringSubmatch(string(out), -1) {
		w, _ := strconv.ParseFloat(m[2], 64)
		h, _ := strconv.ParseFloat(m[3], 64)
		wide[m[1]] = w > h*1.02
	}
	var pages []string
	for _, m := range pageRotRe.FindAllStringSubmatch(string(out), -1) {
		r, _ := strconv.Atoi(m[2])
		if wide[m[1]] != (r%180 == 90) {
			pages = append(pages, m[1])
		}
	}
	return pages
}
