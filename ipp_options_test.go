// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func readPPDFixture(t *testing.T, name string) ppdInfo {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("tests/fixtures/drivers", name))
	if err != nil {
		t.Fatal(err)
	}
	return parsePPD(string(b))
}

func mediaNames(dc driverCaps) []string {
	var out []string
	for _, m := range dc.Media {
		n := m.PWG
		if m.Borderless {
			n += " (borderless)"
		}
		out = append(out, n)
	}
	return out
}

// the phone offers what the printer's driver offers: paper sizes (and borderless), paper types, quality, colour
func TestDriverCapsBrother(t *testing.T) {
	dc := driverCapsFrom(readPPDFixture(t, "brother-dcpt510w.ppd"), true, false)
	names := mediaNames(dc)
	for _, want := range []string{"iso_a4_210x297mm", "iso_a4_210x297mm (borderless)", "na_letter_8.5x11in", "na_legal_8.5x14in",
		"iso_a5_148x210mm", "iso_a6_105x148mm", "na_index-4x6_4x6in", "na_index-4x6_4x6in (borderless)", "oe_photo-l_3.5x5in (borderless)",
		"na_5x7_5x7in", "jpn_hagaki_100x148mm", "iso_dl_110x220mm", "na_number-10_4.125x9.5in"} {
		if !slices.Contains(names, want) {
			t.Errorf("missing %s in %v", want, names)
		}
	}
	if dc.DefaultMedia != "na_letter_8.5x11in" { // Brother's own file says Letter (a queue's copy can say A4)
		t.Errorf("default paper, from the driver file: %s", dc.DefaultMedia)
	}
	a4 := dc.find("iso_a4_210x297mm", false)
	a4b := dc.find("iso_a4_210x297mm", true)
	if a4 == nil || a4.PPD != "A4" || a4.Top != 318 || a4b == nil || a4b.PPD != "BrA4_B" || a4b.Top != 0 {
		t.Errorf("A4 and borderless A4 map to the driver's own choices with their margins: %+v %+v", a4, a4b)
	}
	for kw, ppd := range map[string]string{"stationery": "Plain", "stationery-inkjet": "Inkjet", "photographic-glossy": "Glossy",
		"photographic-matte": "BrotherBP60Matte", "photographic": "BrotherGlossyR"} {
		if got := dc.MediaTypes[kw]; got != ppd {
			t.Errorf("paper type %s → %q, want %q (all: %v)", kw, got, ppd, dc.MediaTypes)
		}
	}
	if dc.MediaTypeKey != "BRMediaType" {
		t.Errorf("paper type option: %s", dc.MediaTypeKey)
	}
	if dc.QualityKey != "BRResolution" || dc.Quality[3] != "PlainFast" || dc.Quality[4] != "PlainNormal" || dc.Quality[5] != "High" {
		t.Errorf("quality: %s %v", dc.QualityKey, dc.Quality)
	}
	if dc.ColorKey != "BRMonoColor" || dc.ColorValue != "Color" || dc.MonoValue != "Mono" {
		t.Errorf("colour: %s %s %s", dc.ColorKey, dc.ColorValue, dc.MonoValue)
	}
}

func TestDriverCapsOtherMakers(t *testing.T) {
	hp := driverCapsFrom(readPPDFixture(t, "hp-like.ppd"), true, true)
	if hp.DefaultMedia != "na_letter_8.5x11in" || hp.find("na_index-4x6_4x6in", true) == nil || hp.find("na_index-4x6_4x6in", true).PPD != "Photo4x6.FB" {
		t.Errorf("HP papers: %v default %s", mediaNames(hp), hp.DefaultMedia)
	}
	if hp.MediaTypes["photographic-glossy"] != "Glossy" || hp.MediaTypes["transparency"] != "Transparency" || hp.MediaTypes["stationery"] != "Plain" {
		t.Errorf("HP paper types: %v", hp.MediaTypes)
	}
	if hp.QualityKey != "OutputMode" || hp.Quality[3] != "FastDraft" || hp.Quality[5] != "Best" {
		t.Errorf("HP quality: %v", hp.Quality)
	}
	if hp.ColorKey != "ColorModel" || hp.ColorValue != "RGB" || hp.MonoValue != "KGray" {
		t.Errorf("HP colour: %s %s %s", hp.ColorKey, hp.ColorValue, hp.MonoValue)
	}
	if hp.DuplexKey != "Duplex" || hp.DuplexLong != "DuplexNoTumble" || hp.DuplexShort != "DuplexTumble" {
		t.Errorf("HP prints both sides itself, on either edge: %s %s %s", hp.DuplexKey, hp.DuplexLong, hp.DuplexShort)
	}
	ep := driverCapsFrom(readPPDFixture(t, "epson-like.ppd"), true, false)
	if ep.find("iso_a4_210x297mm", false) == nil || ep.find("iso_a6_105x148mm", false) == nil || ep.find("jpn_hagaki_100x148mm", false) == nil {
		t.Errorf("Epson papers (fractional sizes): %v", mediaNames(ep))
	}
	if ep.QualityKey != "cupsPrintQuality" || ep.Quality[3] != "Draft" || ep.Quality[5] != "High" || len(ep.MediaTypes) != 0 {
		t.Errorf("Epson: quality %v, no paper types %v", ep.Quality, ep.MediaTypes)
	}
	// every paper name follows the PWG naming rule (the IPP 2.0 conformance suite's own pattern: no trailing zeros)
	pwgRule := regexp.MustCompile(`^((custom|na|asme|roc|oe|roll)_[a-z0-9][-a-z0-9]*_([1-9][0-9]*(\.[0-9]*[1-9])?|0\.[0-9]*[1-9])x([1-9][0-9]*(\.[0-9]*[1-9])?|0\.[0-9]*[1-9])in|(custom|iso|jis|jpn|prc|om|roll)_[a-z0-9][-a-z0-9]*_([1-9][0-9]*(\.[0-9]*[1-9])?|0\.[0-9]*[1-9])x([1-9][0-9]*(\.[0-9]*[1-9])?|0\.[0-9]*[1-9])mm)$`)
	for _, f := range []string{"brother-dcpt510w.ppd", "hp-like.ppd", "epson-like.ppd"} {
		for _, m := range driverCapsFrom(readPPDFixture(t, f), true, false).Media {
			if !pwgRule.MatchString(m.PWG) {
				t.Errorf("%s: %q breaks the PWG naming rule", f, m.PWG)
			}
		}
	}
	// a paper that has no standard name keeps a self-describing one
	odd := driverCapsFrom(parsePPD("*OpenUI *PageSize/Size: PickOne\n*DefaultPageSize: Odd\n*PageSize Odd/Odd: \"\"\n*PaperDimension Odd/Odd: \"300 400\"\n*ImageableArea Odd/Odd: \"0 0 300 400\"\n"), false, false)
	if len(odd.Media) != 1 || !strings.HasPrefix(odd.Media[0].PWG, "custom_odd_") || !odd.Media[0].Borderless {
		t.Errorf("custom size: %+v", odd.Media)
	}
}

// what the phone chose becomes the driver's own choices
func TestIPPJobOptionsFromDriver(t *testing.T) {
	dc := driverCapsFrom(readPPDFixture(t, "brother-dcpt510w.ppd"), true, false)
	o := dc.options(ippSpec{media: "iso_a4_210x297mm", borderless: true, mediaType: "photographic-glossy", quality: 5, color: "monochrome"})
	want := map[string]string{"PageSize": "BrA4_B", "BRMediaType": "Glossy", "BRResolution": "High", "BRMonoColor": "Mono"}
	for k, v := range want {
		if o[k] != v {
			t.Errorf("%s = %q, want %q (all %v)", k, o[k], v, o)
		}
	}
	o = dc.options(ippSpec{media: "na_index-4x6_4x6in", color: "color"})
	if o["PageSize"] != "BrPostC4x6_S" || o["BRMonoColor"] != "Color" || o["BRMediaType"] != "" || o["BRResolution"] != "" {
		t.Errorf("4x6 with borders, colour, the rest left to the driver's defaults: %v", o)
	}
	if o := dc.options(ippSpec{media: "iso_a3_297x420mm"}); o["PageSize"] != "Letter" {
		t.Errorf("a size the printer doesn't have: its default, got %v", o)
	}
	hp := driverCapsFrom(readPPDFixture(t, "hp-like.ppd"), true, true)
	if o := hp.options(ippSpec{sides: "two-sided-short-edge"}); o["Duplex"] != "DuplexTumble" {
		t.Errorf("both sides on the short edge, by the printer itself: %v", o)
	}
}

// the phone's print dialog reads these
func TestIPPAttributesFromDriver(t *testing.T) {
	dc := driverCapsFrom(readPPDFixture(t, "brother-dcpt510w.ppd"), true, false)
	o := &ippOut{}
	o.head([2]byte{2, 0}, 0, 1)
	dc.attrs(o)
	o.group(tagEnd)
	req, err := readIPP(bytes.NewReader(o.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	vals := func(name string) []string {
		var out []string
		for _, a := range req.attrs {
			if a.name == name {
				for _, v := range a.values {
					out = append(out, string(v))
				}
			}
		}
		return out
	}
	if m := vals("media-supported"); !slices.Contains(m, "iso_a4_210x297mm") || !slices.Contains(m, "oe_photo-l_3.5x5in") {
		t.Errorf("media-supported: %v", m)
	}
	if mt := vals("media-type-supported"); !slices.Contains(mt, "photographic-glossy") || !slices.Contains(mt, "stationery") {
		t.Errorf("media-type-supported: %v", mt)
	}
	if s := vals("sides-supported"); !slices.Equal(s, []string{"one-sided", "two-sided-long-edge", "two-sided-short-edge"}) {
		t.Errorf("sides-supported: %v", s)
	}
	if vals("media-default")[0] != dc.DefaultMedia {
		t.Errorf("media-default: %v", vals("media-default"))
	}
	// every paper is in the database, and the borderless ones have margins of 0 (read back entry by entry)
	ints := func(name string) []int {
		var out []int
		if a := req.get(name); a != nil {
			for _, v := range a.values {
				out = append(out, int(beUint32(v)))
			}
		}
		return out
	}
	widths, tops := ints("media-col-database.media-size.x-dimension"), ints("media-col-database.media-top-margin")
	if len(widths) != len(dc.Media) || len(tops) != len(dc.Media) {
		t.Fatalf("the database lists %d papers (%d margins), the driver has %d", len(widths), len(tops), len(dc.Media))
	}
	for i, m := range dc.Media {
		if widths[i] != m.W || tops[i] != m.Top || (tops[i] == 0) != m.Borderless {
			t.Errorf("paper %d (%s): width %d top %d, want %d %d borderless %v", i, m.PWG, widths[i], tops[i], m.W, m.Top, m.Borderless)
		}
	}
	// and the margins offered include both
	var top []int
	for _, a := range req.attrs {
		if a.name == "media-top-margin-supported" {
			for _, v := range a.values {
				top = append(top, int(beUint32(v)))
			}
		}
	}
	if !slices.Contains(top, 0) || !slices.Contains(top, 318) {
		t.Errorf("top margins offered: %v", top)
	}
}

// the phone's choices, read from its request
func TestIPPSpecFromRequest(t *testing.T) {
	o := &ippOut{}
	o.head([2]byte{2, 0}, opPrintJob, 1)
	o.group(tagOperation)
	o.attr(vCharset, "attributes-charset", "utf-8")
	o.attr(vLanguage, "attributes-natural-language", "en")
	o.group(tagJob)
	o.attr(vKeyword, "sides", "two-sided-short-edge")
	o.attr(vEnum, "print-quality", 5)
	o.collStart("media-col")
	o.memberColl("media-size")
	o.member("x-dimension", vInteger, 21000)
	o.member("y-dimension", vInteger, 29700)
	o.collEnd()
	o.member("media-type", vKeyword, "photographic-glossy")
	for _, m := range []string{"media-top-margin", "media-bottom-margin", "media-left-margin", "media-right-margin"} {
		o.member(m, vInteger, 0)
	}
	o.collEnd()
	o.group(tagEnd)
	req, err := readIPP(bytes.NewReader(o.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	sp := specFromRequest(req, driverCapsFrom(readPPDFixture(t, "brother-dcpt510w.ppd"), true, false))
	if sp.media != "iso_a4_210x297mm" || !sp.borderless || sp.mediaType != "photographic-glossy" || sp.quality != 5 || sp.sides != "two-sided-short-edge" {
		t.Fatalf("got %+v", sp)
	}
}

// both sides by hand, bound on the short edge: the backs are turned right round
func TestShortEdgeByHand(t *testing.T) {
	if !have("qpdf") || !have("pdfinfo") {
		t.Skip("needs qpdf and pdfinfo")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	four, err := testPDF(dir) // 4 pages
	if err != nil {
		t.Fatal(err)
	}
	b := &Built{Dir: dir, Combined: four, Duplex: true, ShortEdge: true, Printer: "test"}
	if err := b.passes(); err != nil {
		t.Fatal(err)
	}
	rot := func(pdf string) string {
		out, _ := exec.Command("pdfinfo", "-f", "1", "-l", "9", pdf).Output()
		var r []string
		for _, l := range strings.Split(string(out), "\n") {
			if strings.Contains(l, " rot:") {
				r = append(r, strings.TrimSpace(l[strings.LastIndex(l, ":")+1:]))
			}
		}
		return strings.Join(r, ",")
	}
	r1, r2 := rot(b.Pass1), rot(b.Pass2)
	// one pass is the fronts (not turned), the other the backs (turned 180°)
	if !(r1 == "0,0" && r2 == "180,180" || r1 == "180,180" && r2 == "0,0") {
		t.Errorf("fronts upright, backs turned: pass 1 %s, pass 2 %s", r1, r2)
	}
}
