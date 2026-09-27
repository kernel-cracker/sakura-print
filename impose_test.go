// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

// upOf: which way the top of a page's writing points, seen from the front of the sheet (x right, y down).
// Printers put the back so that turning the sheet over on its LONG edge shows it upright: seen from the front,
// the back is mirrored left-right.
func upOf(rot int, back bool) [2]int {
	a := float64(rot) * math.Pi / 180
	x, y := int(math.Round(math.Sin(a))), int(math.Round(-math.Cos(a))) // (0,-1) turned clockwise
	if back {
		x = -x
	}
	return [2]int{x, y}
}

// Fold the printed stack in half and read it cover to cover: every page must come in order, the right way up.
func TestBookletReadsInOrder(t *testing.T) {
	for _, n := range []int{1, 4, 5, 8, 11, 16} {
		sides := layoutSheets("booklet", n, true)
		S := len(sides) / 2
		if S != (n+3)/4 {
			t.Fatalf("%d pages: want %d sheets, got %d", n, (n+3)/4, S)
		}
		at := func(sheet int, back bool, top bool) cell {
			s := sides[2*sheet]
			if back {
				s = sides[2*sheet+1]
			}
			for _, c := range s {
				if (c.y < sheetH/2) == top {
					return c
				}
			}
			t.Fatalf("no cell")
			return cell{}
		}
		// folded along the middle, top half behind: the cover is the outer sheet's front bottom half; then each
		// sheet's back bottom and the next sheet's front bottom; the middle spread is the inner sheet's back;
		// then back out through the top halves to the back cover (the outer sheet's front top half)
		type spot struct {
			sheet     int
			back, top bool
		}
		var order []spot
		for i := 0; i < S; i++ {
			order = append(order, spot{i, false, false}, spot{i, true, false})
		}
		for i := S - 1; i >= 0; i-- {
			order = append(order, spot{i, true, true}, spot{i, false, true})
		}
		var read []int
		for _, sp := range order {
			c := at(sp.sheet, sp.back, sp.top)
			read = append(read, c.page)
			// the booklet's top edge is the sheet's right side (+x): every page's writing must point that way
			if c.page != 0 && upOf(c.rot, sp.back) != [2]int{1, 0} {
				t.Errorf("%d pages: page %d is not the right way up (rot %d, back %v)", n, c.page, c.rot, sp.back)
			}
		}
		for i, p := range read {
			want := i + 1
			if want > n {
				want = 0 // blank pages only at the very end
			}
			if p != want {
				t.Fatalf("%d pages: reading order %v", n, read)
			}
		}
	}
}

// Two per sheet, both sides: turn the sheet over (like a page of a book held sideways) and the back reads on
func TestTwoUpDuplexBackReadsOn(t *testing.T) {
	sides := layoutSheets("2up", 4, true)
	if len(sides) != 2 {
		t.Fatalf("4 pages two per sheet: want 2 sides, got %d", len(sides))
	}
	for k, s := range sides {
		back := k%2 == 1
		for _, c := range s {
			up := upOf(c.rot, back)
			if back { // turned over on the short edge (a horizontal line, seen from the front): up and down swap
				up = [2]int{up[0], -up[1]}
			}
			if up != [2]int{1, 0} {
				t.Errorf("side %d page %d is not the right way up", k, c.page)
			}
		}
		// held sideways, the sheet's top half is the left: the lower page number must be on the left as read
		var left, right int
		for _, c := range s {
			isTop := c.y < sheetH/2
			if back { // seen from the back after turning it over on the short edge, top and bottom swap
				isTop = !isTop
			}
			if isTop {
				left = c.page
			} else {
				right = c.page
			}
		}
		if left != 2*k+1 || right != 2*k+2 {
			t.Errorf("side %d reads %d then %d", k, left, right)
		}
	}
}

func TestFourUpOrder(t *testing.T) {
	s := layoutSheets("4up", 6, false)
	if len(s) != 2 || s[0][0].page != 1 || s[0][1].page != 2 || s[0][2].page != 3 || s[0][3].page != 4 || s[1][1].page != 6 || s[1][2].page != 0 {
		t.Fatalf("4 per sheet, left to right, top to bottom: %+v", s)
	}
	if !(s[0][0].x < s[0][1].x && s[0][0].y < s[0][2].y) {
		t.Fatal("cells are in the wrong places")
	}
}

func TestImposeAndSkipBlank(t *testing.T) {
	for _, tool := range []string{"qpdf", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("needs " + tool)
		}
	}
	dir := t.TempDir()
	two := filepath.Join("tests", "fixtures", "twopage.pdf")
	for _, c := range []struct {
		layout string
		duplex bool
		sheets int
	}{{"2up", false, 1}, {"4up", false, 1}, {"booklet", true, 2}} {
		out, err := impose(two, c.layout, c.duplex, false, dir)
		if err != nil {
			t.Fatal(c.layout, err)
		}
		if got := npages(out); got != c.sheets {
			t.Errorf("%s of 2 pages: want %d sheet sides, got %d", c.layout, c.sheets, got)
		}
		if w, h := pageSizeMM(out, 1); math.Abs(w-210) > 1 || math.Abs(h-297) > 1 {
			t.Errorf("%s: sheets should be A4, got %.0fx%.0f mm", c.layout, w, h)
		}
	}
	// a PDF with a blank page in the middle
	blank, err := blankPDF(dir)
	if err != nil {
		t.Fatal(err)
	}
	mixed := filepath.Join(dir, "mixed.pdf")
	if err := qpdf("--empty", "--pages", two, "1", blank, "1", two, "2", "--", mixed); err != nil {
		t.Fatal(err)
	}
	got := blankPages(mixed, 3, filepath.Join(dir, "bc"))
	if len(got) != 1 || !got[2] {
		t.Fatalf("want only page 2 found blank, got %v", got)
	}
}
