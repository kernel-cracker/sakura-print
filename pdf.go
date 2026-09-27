// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Turning uploaded files into print-ready PDFs: covers, photo layouts, page order.

import (
	"errors"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- palettes (same as pdftool) ----------

type Palette struct {
	Name string `json:"name"`
	Tint string `json:"tint"`
	Acc  string `json:"acc"`
	Deep string `json:"deep"`
}

var palettes = []Palette{
	{"Sakura", "#fbe9ee", "#f2b5c4", "#9c4a61"},
	{"Matcha", "#eef3e2", "#b5c99a", "#5e7248"},
	{"Blueberry milk", "#e8eef8", "#a9bfe3", "#4a5f86"},
	{"Strawberry", "#fbe8e6", "#efb1aa", "#9a4b45"},
	{"Lavender", "#efeaf7", "#c7b8e6", "#65568e"},
	{"Peach", "#fdeee3", "#f5c6a5", "#9a5f3c"},
	{"Mint", "#e6f4ef", "#a8d8c6", "#4a7d6c"},
	{"Latte", "#f3ece4", "#cfb89f", "#6e5642"},
	{"Butter", "#fcf5dc", "#ecd98f", "#857435"},
	{"Graphite", "#f2f2f2", "#bdbdbd", "#4a4a4a"},
}

func mix(a, b string, pct int) string {
	pa, pb := strings.TrimPrefix(a, "#"), strings.TrimPrefix(b, "#")
	out := "#"
	for i := 0; i < 6; i += 2 {
		ca, _ := strconv.ParseInt(pa[i:i+2], 16, 0)
		cb, _ := strconv.ParseInt(pb[i:i+2], 16, 0)
		out += fmt.Sprintf("%02x", ca+(cb-ca)*int64(pct)/100)
	}
	return out
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// ---------- the job spec the web page sends ----------

type Item struct {
	ID    string `json:"id"`
	Pages string `json:"pages"` // "" = all, or like "1-3,5"
}

type Spec struct {
	Printer   string            `json:"printer"`
	Mode      string            `json:"mode"` // docs | photos | copy
	Items     []Item            `json:"items"`
	Sides     string            `json:"sides"` // single | duplex
	Copies    int               `json:"copies"`
	Cover     int               `json:"cover"`   // 1 none, 2 with name, 3 no name
	Style     int               `json:"style"`   // 1 full, 2 flowers, 3 geometry, 4 minimal, 5 random
	Palette   int               `json:"palette"` // 0-9, 10 = random
	WhiteBg   bool              `json:"whiteBg"`
	PerSheet  int               `json:"perSheet"` // photos: 1, 2, 4, 9
	Fit       string            `json:"fit"`      // photos: fill | fit
	Options   map[string]string `json:"options"`
	Join      bool              `json:"join"`      // docs: treat all items as one document (no covers or padding between them)
	FitPage   bool              `json:"fitPage"`   // docs: turn sideways pages upright and shrink every page into the printable area
	Placed    []Placed          `json:"placed"`    // photos: arranged by hand in the editor
	Preview   bool              `json:"preview"`   // for the screen only: use small copies of the pictures (much faster)
	Again     bool              `json:"again"`     // a reprint from "Print again": it's already in the list
	Layout    string            `json:"layout"`    // docs, paper savers: "" | 2up | 4up | booklet
	SkipBlank bool              `json:"skipBlank"` // docs: leave out pages that are (almost) completely white
	ShortEdge bool              `json:"shortEdge"` // both sides, bound on the short edge (flip like a notepad)
}

// Placed is one photo put on a page by hand. Positions and sizes are fractions of the page (0..1).
type Placed struct {
	ID    string  `json:"id"`
	Page  int     `json:"page"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Rot   int     `json:"rot"`   // extra turn chosen by the user: 0, 90, 180, 270
	Whole bool    `json:"whole"` // true = show the whole photo, false = fill the frame (crops)
}

// PageRef says where one printed page comes from, so "edit this page" can find the real file and page
type PageRef struct {
	K     string `json:"k"`               // page (a page of a PDF) | image | photos | cover | blank
	Item  int    `json:"item"`            // index in the job's list of files
	Page  int    `json:"page,omitempty"`  // page number inside that PDF
	Items []int  `json:"items,omitempty"` // photos on this sheet
}

type Built struct {
	ID        string            `json:"id"`
	Dir       string            `json:"-"`
	Combined  string            `json:"-"`
	Pass1     string            `json:"-"`
	Pass2     string            `json:"-"`
	Pages     int               `json:"pages"`
	Sheets    int               `json:"sheets"`
	Duplex    bool              `json:"duplex"`     // both sides, by flipping the stack by hand
	Auto      bool              `json:"autoDuplex"` // both sides, the printer does it by itself
	Printer   string            `json:"printer"`
	Options   map[string]string `json:"-"`
	Rotate    bool              `json:"rotate"`
	ShortEdge bool              `json:"shortEdge"` // backs turned right round, for binding on the short edge
	Warnings  []string          `json:"warnings"`
	Map       []PageRef         `json:"map"`     // one entry per page of ONE copy
	PerCopy   int               `json:"perCopy"` // pages in one copy
	Spec      Spec              `json:"-"`       // what was asked for (kept for "Print again")
	Created   time.Time         `json:"-"`

	mu  sync.Mutex // one print per preview, even if Print is tapped 5 times
	job *Job
}

var pageRangeRe = regexp.MustCompile(`^[0-9z,\- ]+$`)

// expandRange turns "1-3,5,z" into the page numbers it picks (same rules as qpdf for these simple forms)
func expandRange(rng string, n int) []int {
	num := func(s string) int {
		if s == "z" {
			return n
		}
		v, _ := strconv.Atoi(s)
		if v < 1 {
			v = 1
		}
		if v > n {
			v = n
		}
		return v
	}
	var out []int
	for _, tok := range strings.Split(strings.ReplaceAll(rng, " ", ""), ",") {
		if tok == "" {
			continue
		}
		if i := strings.Index(tok, "-"); i > 0 {
			a, b := num(tok[:i]), num(tok[i+1:])
			step := 1
			if a > b {
				step = -1
			}
			for p := a; ; p += step {
				out = append(out, p)
				if p == b {
					break
				}
			}
		} else {
			out = append(out, num(tok))
		}
	}
	return out
}

func qpdf(args ...string) error {
	out, err := exec.Command("qpdf", args...).CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 3 { // 3 = harmless warnings, common with school PDFs
			return nil
		}
		return fmt.Errorf("qpdf: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func npages(path string) int {
	out, _ := exec.Command("qpdf", "--show-npages", path).Output()
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

func isMono(opts map[string]string) bool {
	for k, v := range opts {
		lk, lv := strings.ToLower(k), strings.ToLower(v)
		if (strings.Contains(lk, "color") || strings.Contains(lk, "colour") || strings.Contains(lk, "mono")) &&
			(strings.Contains(lv, "mono") || strings.Contains(lv, "gray") || strings.Contains(lv, "grey")) {
			return true
		}
	}
	return false
}
