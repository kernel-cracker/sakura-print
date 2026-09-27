// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Building prints: from the job spec to print-ready PDFs, in the right page order for the printer and the flip.

// ---------- the main build ----------

func build(spec Spec, workRoot string) (*Built, error) {
	if len(spec.Items) == 0 {
		return nil, errors.New("pick at least one file first")
	}
	if spec.Copies < 1 {
		spec.Copies = 1
	}
	switch spec.Layout {
	case "2up", "4up":
	case "booklet":
		spec.Sides = "duplex" // a booklet is always printed on both sides
	default:
		spec.Layout = ""
	}
	if spec.Copies > 99 {
		spec.Copies = 99
	}
	if spec.Options == nil {
		spec.Options = map[string]string{}
	}
	id := newID()
	dir := filepath.Join(workRoot, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b := &Built{ID: id, Dir: dir, Printer: spec.Printer, Options: spec.Options, Warnings: []string{}, Created: time.Now(), Spec: spec}
	mono := isMono(spec.Options)
	paperName := spec.Options["PageSize"]
	var parts []string // qpdf --pages arguments: file range file range ...

	switch spec.Mode {
	case "photos":
		var paths []string
		for _, it := range spec.Items {
			f, ok := getFile(it.ID)
			if !ok || f.Kind != "image" {
				return nil, errors.New("one of the photos isn't there anymore, add it again")
			}
			paths = append(paths, f.Path)
		}
		if spec.Fit == "custom" && len(spec.Placed) == 0 {
			return nil, errors.New("put at least one photo on the page")
		}
		photos := filepath.Join(dir, "photos.pdf")
		if spec.Fit == "custom" {
			pages, err := customPhotoPDF(spec.Placed, paperName, dir, photos, spec.Preview)
			if err != nil {
				return nil, err
			}
			idx := map[string]int{}
			for i, it := range spec.Items {
				idx[it.ID] = i
			}
			b.Map = make([]PageRef, pages)
			for i := range b.Map {
				b.Map[i] = PageRef{K: "photos", Items: []int{}}
			}
			for _, pl := range spec.Placed {
				if i, ok := idx[pl.ID]; ok && pl.Page >= 0 && pl.Page < pages {
					b.Map[pl.Page].Items = append(b.Map[pl.Page].Items, i)
				}
			}
		} else {
			sheets, err := photoPDF(paths, paperName, spec.PerSheet, spec.Fit != "fit", dir, photos, spec.Preview)
			if err != nil {
				return nil, err
			}
			per := (len(paths) + sheets - 1) / sheets
			for s := 0; s < sheets; s++ {
				r := PageRef{K: "photos"}
				for i := s * per; i < (s+1)*per && i < len(paths); i++ {
					r.Items = append(r.Items, i)
				}
				b.Map = append(b.Map, r)
			}
		}
		parts = append(parts, photos, "1-z")
		spec.Sides = "single"
	case "copy":
		var paths []string
		for _, it := range spec.Items {
			if f, ok := getFile(it.ID); ok && f.Kind == "image" {
				paths = append(paths, f.Path)
			}
		}
		if len(paths) == 0 {
			return nil, errors.New("nothing scanned to copy")
		}
		cp := filepath.Join(dir, "copy.pdf")
		if _, err := photoPDF(paths, "A4", 1, false, dir, cp, spec.Preview); err != nil {
			return nil, err
		}
		parts = append(parts, cp, "1-z")
	default: // docs
		blank := ""
		for k, it := range spec.Items {
			f, ok := getFile(it.ID)
			if !ok {
				return nil, errors.New("one of the files isn't there anymore, add it again")
			}
			src := f.Path
			if f.Kind == "image" { // an image in a document job becomes one A4 page
				src = filepath.Join(dir, fmt.Sprintf("img-%d.pdf", k))
				if _, err := photoPDF([]string{f.Path}, "A4", 1, false, dir, src, spec.Preview); err != nil {
					return nil, err
				}
			}
			rng := strings.TrimSpace(it.Pages)
			if rng != "" && f.Kind == "pdf" {
				if !pageRangeRe.MatchString(rng) {
					return nil, fmt.Errorf("pages %q for %s look wrong, use something like 1-3,5", rng, f.Name)
				}
				for _, tok := range regexp.MustCompile(`[0-9]+`).FindAllString(rng, -1) {
					if v, _ := strconv.Atoi(tok); v < 1 || v > f.Pages {
						pl := "pages"
						if f.Pages == 1 {
							pl = "page"
						}
						return nil, fmt.Errorf("%s only has %d %s, so there's no page %d. Check the pages box in Settings", f.Name, f.Pages, pl, v)
					}
				}
				sel := filepath.Join(dir, fmt.Sprintf("sel-%d.pdf", k))
				if err := qpdf("--empty", "--pages", src, strings.ReplaceAll(rng, " ", ""), "--", sel); err != nil {
					return nil, fmt.Errorf("pages %q for %s: %v", rng, f.Name, err)
				}
				src = sel
			}
			n := npages(src)
			if n < 1 {
				return nil, fmt.Errorf("can't read %s (password protected?)", f.Name)
			}
			orig := make([]int, 0, n) // which page of the file each page is now (for "Edit this page")
			if rng != "" {
				orig = expandRange(rng, f.Pages)
			} else {
				for p := 1; p <= n; p++ {
					orig = append(orig, p)
				}
			}
			if spec.SkipBlank && f.Kind == "pdf" && n > 1 && (!spec.Preview || n <= 60) { // previews of huge files skip the check, for speed
				if blanks := blankPages(src, n, filepath.Join(dir, fmt.Sprintf("b%d", k))); len(blanks) > 0 && len(blanks) < n {
					var keep []string
					var kept []int
					for p := 1; p <= n; p++ {
						if !blanks[p] {
							keep = append(keep, strconv.Itoa(p))
							if p-1 < len(orig) {
								kept = append(kept, orig[p-1])
							}
						}
					}
					nb := filepath.Join(dir, fmt.Sprintf("noblank-%d.pdf", k))
					if err := qpdf("--empty", "--pages", src, strings.Join(keep, ","), "--", nb); err == nil {
						src, n, orig = nb, len(keep), kept
						b.Warnings = append(b.Warnings, fmt.Sprintf("%s: left out %d blank page(s)", f.Name, len(blanks)))
					}
				}
			}
			if spec.FitPage {
				if side := sidewaysPages(src, n); len(side) > 0 {
					up := filepath.Join(dir, fmt.Sprintf("upright-%d.pdf", k))
					if err := qpdf(src, "--rotate=+90:"+strings.Join(side, ","), up); err == nil {
						src = up
						b.Warnings = append(b.Warnings, fmt.Sprintf("%s: turned %d sideways page(s) upright", f.Name, len(side)))
					}
				}
			}
			used := 0
			if !spec.Join && (spec.Cover == 2 || spec.Cover == 3) {
				cov := filepath.Join(dir, fmt.Sprintf("cover-%d.pdf", k))
				if err := makeCover(cov, f.Name, n, spec, mono, paperName); err != nil {
					return nil, err
				}
				parts = append(parts, cov, "1")
				b.Map = append(b.Map, PageRef{K: "cover", Item: k})
				used = 1
			}
			parts = append(parts, src, "1-z")
			if f.Kind == "image" {
				b.Map = append(b.Map, PageRef{K: "image", Item: k})
			} else {
				for _, p := range orig {
					b.Map = append(b.Map, PageRef{K: "page", Item: k, Page: p})
				}
			}
			if !spec.Join && spec.Layout == "" && spec.Sides == "duplex" && (used+n)%2 == 1 {
				if blank == "" {
					var err error
					if blank, err = blankPDF(dir); err != nil {
						return nil, err
					}
				}
				parts = append(parts, blank, "1")
				b.Map = append(b.Map, PageRef{K: "blank"})
				used++
			}
		}
	}

	// double-sided needs an even page count, or the two passes don't line up sheet for sheet
	// (with a paper saver, that's counted on the finished sheets instead, further down)
	if spec.Sides == "duplex" && spec.Mode != "photos" && spec.Layout == "" {
		total := 0
		for i := 0; i+1 < len(parts); i += 2 {
			if parts[i+1] == "1" {
				total++
			} else {
				total += npages(parts[i])
			}
		}
		if total%2 == 1 {
			blank, err := blankPDF(dir)
			if err != nil {
				return nil, err
			}
			parts = append(parts, blank, "1")
			b.Map = append(b.Map, PageRef{K: "blank"})
		}
	}

	one := filepath.Join(dir, "one.pdf")
	if err := qpdf(append(append([]string{"--empty", "--pages"}, parts...), "--", one)...); err != nil {
		return nil, err
	}
	if spec.Layout != "" && (spec.Mode == "" || spec.Mode == "docs") { // paper savers: several pages on each sheet
		imp, err := impose(one, spec.Layout, spec.Sides == "duplex", spec.Preview, dir)
		if err != nil {
			return nil, err
		}
		if spec.Sides == "duplex" && npages(imp)%2 == 1 {
			blank, err := blankPDF(dir)
			if err != nil {
				return nil, err
			}
			padded := filepath.Join(dir, "imposed-even.pdf")
			if err := qpdf("--empty", "--pages", imp, "1-z", blank, "1", "--", padded); err != nil {
				return nil, err
			}
			imp = padded
		}
		one = imp
		b.Map = nil // pages share sheets now: "Edit this page" would be ambiguous
	}
	b.Combined = filepath.Join(dir, "combined.pdf")
	if spec.Copies > 1 {
		var cp []string
		for i := 0; i < spec.Copies; i++ {
			cp = append(cp, one, "1-z")
		}
		if err := qpdf(append(append([]string{"--empty", "--pages"}, cp...), "--", b.Combined)...); err != nil {
			return nil, err
		}
	} else if err := os.Rename(one, b.Combined); err != nil {
		return nil, err
	}
	if spec.FitPage && (spec.Mode == "" || spec.Mode == "docs") {
		opts := map[string]string{}
		for k, v := range spec.Options {
			opts[k] = v
		}
		opts["fit-to-page"] = "true" // shrink every page into the area the printer can actually reach
		b.Options = opts
	}
	b.Pages = npages(b.Combined)
	b.PerCopy = b.Pages / spec.Copies
	if len(b.Map) != b.PerCopy {
		b.Map = nil // something didn't line up (like a copy job): no per-page editing, but printing is unaffected
	}
	b.Duplex = spec.Sides == "duplex" && b.Pages > 1
	b.ShortEdge = spec.ShortEdge
	// a printer that prints both sides by itself gets the job in one go, with its own option switched on;
	// and switched off for one-sided jobs, since some drivers default to both sides
	if key, on, off, ok := autoDuplex(spec.Printer); ok {
		opts := map[string]string{}
		for k, v := range b.Options {
			opts[k] = v
		}
		if b.Duplex {
			opts[key], b.Duplex, b.Auto = on, false, true
			if short := duplexShortValue(ppdOptions(spec.Printer), key); spec.ShortEdge && short != "" {
				opts[key] = short
			}
		} else if off != "" {
			opts[key] = off
		}
		b.Options = opts
	}
	b.Sheets = b.Pages
	if b.Duplex || b.Auto {
		b.Sheets = (b.Pages + 1) / 2
	}
	return b, nil
}

// passes splits a built job into what actually gets sent to the printer
func (b *Built) passes() error {
	prof, _ := loadProfile(b.Printer)
	b.Rotate = prof.Rot
	b.Pass1 = filepath.Join(b.Dir, "pass1.pdf")
	if b.Auto { // the printer turns the sheets itself, so pages go in their normal order
		return qpdf("--empty", "--pages", b.Combined, "1-z", "--", b.Pass1)
	}
	if !b.Duplex {
		rng := "1-z"
		if prof.FaceUp {
			rng = "z-1" // face-up printers stack backwards, so send it backwards
		}
		return qpdf("--empty", "--pages", b.Combined, rng, "--", b.Pass1)
	}
	b.Pass2 = filepath.Join(b.Dir, "pass2.pdf")
	combined := b.Combined
	if b.ShortEdge { // bound on the short edge: every back is turned right round
		combined = filepath.Join(b.Dir, "short-edge.pdf")
		if err := qpdf(b.Combined, "--rotate=+180:1-z:even", combined); err != nil {
			return err
		}
	}
	second := "even"
	if prof.First == "even" {
		second = "odd"
	}
	mk := func(parity string, rev bool, out string) error {
		sel := filepath.Join(b.Dir, "sel-"+parity+".pdf")
		if err := qpdf("--empty", "--pages", combined, "1-z:"+parity, "--", sel); err != nil {
			return err
		}
		if rev {
			return qpdf("--empty", "--pages", sel, "z-1", "--", out)
		}
		return os.Rename(sel, out)
	}
	if err := mk(prof.First, prof.R1, b.Pass1); err != nil {
		return err
	}
	return mk(second, prof.R2, b.Pass2)
}

// pageSizeMM reads one page's size as it appears on screen
func pageSizeMM(path string, page int) (float64, float64) {
	out, err := exec.Command("pdfinfo", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), path).Output()
	if err != nil {
		return 210, 297
	}
	w, h, rot := 595.0, 842.0, 0
	if m := pageSizeRe.FindStringSubmatch(string(out)); m != nil {
		w, _ = strconv.ParseFloat(m[2], 64)
		h, _ = strconv.ParseFloat(m[3], 64)
	}
	if m := pageRotRe.FindStringSubmatch(string(out)); m != nil {
		rot, _ = strconv.Atoi(m[2])
	}
	if rot%180 == 90 {
		w, h = h, w
	}
	return w * 25.4 / 72, h * 25.4 / 72
}

// replacePage makes a copy of a PDF where one page is swapped for an (edited) picture of it
func replacePage(pdf string, page int, image string, dir, out string) error {
	n := npages(pdf)
	if page < 1 || page > n {
		return fmt.Errorf("that PDF has no page %d", page)
	}
	im, err := loadImage(image)
	if err != nil {
		return err
	}
	w, h := pageSizeMM(pdf, page)
	one := filepath.Join(dir, "edited-page.pdf")
	if err := writePicturePDF([]picPage{{w: w, h: h, items: []placement{{im: im, w: w, h: h, rot: im.Rot}}}}, one); err != nil {
		return err
	}
	args := []string{"--empty", "--pages"}
	if page > 1 {
		args = append(args, pdf, fmt.Sprintf("1-%d", page-1))
	}
	args = append(args, one, "1")
	if page < n {
		args = append(args, pdf, fmt.Sprintf("%d-z", page+1))
	}
	return qpdf(append(args, "--", out)...)
}

// testPDF makes the 4 big numbered pages used for calibration
func testPDF(dir string) (string, error) {
	var svgs []string
	for p := 1; p <= 4; p++ {
		svgs = append(svgs, fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="210mm" height="297mm" viewBox="0 0 300 424">`+
			`<rect width="300" height="424" fill="#ffffff"/>`+
			`<g transform="translate(150 40)" fill="#e0648f"><ellipse cy="-9" rx="7" ry="10"/><ellipse cy="-9" rx="7" ry="10" transform="rotate(72)"/><ellipse cy="-9" rx="7" ry="10" transform="rotate(144)"/><ellipse cy="-9" rx="7" ry="10" transform="rotate(216)"/><ellipse cy="-9" rx="7" ry="10" transform="rotate(288)"/><circle r="5" fill="#ffd166"/></g>`+
			`<text x="150" y="72" text-anchor="middle" font-family="sans-serif" font-size="10">▲ top of the page ▲</text>`+
			`<text x="150" y="270" text-anchor="middle" font-family="sans-serif" font-size="170" font-weight="bold">%d</text>`+
			`<text x="150" y="390" text-anchor="middle" font-family="sans-serif" font-size="11">duplex test · page %d of 4</text></svg>`, p, p))
	}
	out := filepath.Join(dir, "test.pdf")
	return out, svgsToPDF(svgs, dir, out)
}
