// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Building prints, covers: the pretty first page, from assets/cover.svg.

// ---------- covers ----------

func makeCover(out, name string, pages int, spec Spec, mono bool, paperName string) error {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	title = strings.NewReplacer("_", " ", "-", " ").Replace(title)
	title = strings.Join(strings.Fields(title), " ")
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:38]) + "…"
	}
	words := strings.Fields(title)
	t1, t2 := title, ""
	tl := len([]rune(title))
	if tl > 16 && len(words) > 1 {
		t1 = ""
		half := tl / 2
		for _, w := range words {
			if t2 == "" && (t1 == "" || len([]rune(t1))+len([]rune(w))+1 <= half+3) {
				if t1 != "" {
					t1 += " "
				}
				t1 += w
			} else {
				if t2 != "" {
					t2 += " "
				}
				t2 += w
			}
		}
	}
	longest := len([]rune(t1))
	if l := len([]rune(t2)); l > longest {
		longest = l
	}
	ts := 22
	if longest > 16 {
		ts = 364 / longest
	}
	if ts < 11 {
		ts = 11
	}
	y1, y2 := 186, 186
	if t2 != "" {
		y1, y2 = 172, 198
	}
	fname := name
	if r := []rune(fname); len(r) > 32 {
		fname = string(r[:20]) + "…" + string(r[len(r)-9:])
	}
	pl := fmt.Sprintf("%d pages", pages)
	if pages == 1 {
		pl = "1 page"
	}
	meta := pl
	if paperName != "" {
		meta += " · " + paperName
	}
	if mono {
		meta += " · mono"
	} else {
		meta += " · colour"
	}

	fl, ge, dt, nm := 1, 1, 1, 1
	style := spec.Style
	if style == 5 {
		style = rand.Intn(4) + 1
	}
	switch style {
	case 2:
		ge = 0
	case 3:
		fl = 0
	case 4:
		fl, ge, dt = 0, 0, 0
	}
	p := spec.Palette
	if p < 0 || p >= len(palettes) {
		p = rand.Intn(len(palettes) - 1) // random never picks Graphite
	}
	if mono {
		p = len(palettes) - 1
	}
	pal := palettes[p]
	bg, card := pal.Tint, mix(pal.Tint, "#ffffff", 70)
	if spec.WhiteBg {
		bg, card, dt = "#ffffff", "#ffffff", 0
	}
	if spec.Cover == 3 {
		nm = 0
	}
	svg := strings.NewReplacer(
		"@BG@", bg, "@CARD@", card, "@AC@", pal.Acc, "@DEEP@", pal.Deep,
		"@L1@", mix(pal.Acc, "#ffffff", 45), "@L2@", mix(pal.Acc, "#ffffff", 25),
		"@MID@", mix(pal.Acc, pal.Deep, 30), "@MUTED@", mix(pal.Deep, "#ffffff", 20),
		"@T1@", xmlEsc(t1), "@T2@", xmlEsc(t2),
		"@Y1@", strconv.Itoa(y1), "@Y2@", strconv.Itoa(y2), "@TS@", strconv.Itoa(ts),
		"@FILE@", xmlEsc(fname), "@META@", xmlEsc(meta),
		"@FL@", strconv.Itoa(fl), "@GE@", strconv.Itoa(ge), "@DT@", strconv.Itoa(dt), "@NM@", strconv.Itoa(nm),
	).Replace(coverTemplate)
	svgPath := strings.TrimSuffix(out, ".pdf") + ".svg"
	if err := os.WriteFile(svgPath, []byte(svg), 0o644); err != nil {
		return err
	}
	if o, err := exec.Command("rsvg-convert", "-f", "pdf", "-o", out, svgPath).CombinedOutput(); err != nil {
		return fmt.Errorf("cover: %s", strings.TrimSpace(string(o)))
	}
	return nil
}

func blankPDF(dir string) (string, error) {
	out := filepath.Join(dir, "blank.pdf")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	return out, svgsToPDF([]string{svgPage(210, 297, "")}, filepath.Join(dir), out)
}
