// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"testing"
)

// what lpoptions -l says, from a driver file
func optsOf(t *testing.T, name string) []PPDOption {
	t.Helper()
	p := readPPDFixture(t, name)
	var out []PPDOption
	for _, k := range p.Order {
		o := p.Options[k]
		po := PPDOption{Key: o.Key, Label: o.Label, Default: o.Default}
		for _, c := range o.Choices {
			po.Values = append(po.Values, c.Name)
		}
		out = append(out, po)
	}
	return out
}

// the print screen's colour, quality and paper type settings are found in any maker's driver, not just Brother's
func TestOptionRolesEveryMaker(t *testing.T) {
	canon := []PPDOption{ // shaped like Canon's cnijfilter2
		{Key: "PageSize", Label: "Page Size", Values: []string{"A4", "Letter"}, Default: "A4"},
		{Key: "CNIJMediaType", Label: "Media Type", Values: []string{"plain", "glossygold", "photopaperpro"}, Default: "plain"},
		{Key: "CNIJPrintQuality", Label: "Print Quality", Values: []string{"high", "standard", "draft"}, Default: "standard"},
		{Key: "CNIJGrayScale", Label: "Grayscale Printing", Values: []string{"False", "True"}, Default: "False"},
	}
	for _, c := range []struct {
		name                              string
		opts                              []PPDOption
		colour, on, mono, quality, medium string
	}{
		{"Brother", optsOf(t, "brother-dcpt510w.ppd"), "BRMonoColor", "Color", "Mono", "BRResolution", "BRMediaType"},
		{"HP", optsOf(t, "hp-like.ppd"), "ColorModel", "RGB", "KGray", "OutputMode", "MediaType"},
		{"Epson", optsOf(t, "epson-like.ppd"), "", "", "", "cupsPrintQuality", ""},
		{"Canon", canon, "CNIJGrayScale", "False", "True", "CNIJPrintQuality", "CNIJMediaType"},
	} {
		r := optionRoles(c.opts)
		if r.Colour != c.colour || r.ColourOn != c.on || r.MonoOn != c.mono || r.Quality != c.quality || r.MediaType != c.medium {
			t.Errorf("%s: got %+v", c.name, r)
		}
	}
	if r := optionRoles(optsOf(t, "hp-like.ppd")); len(r.Duplex) != 1 || r.Duplex[0] != "Duplex" {
		t.Errorf("HP's own both-sides option is left to the both-sides switch: %v", r.Duplex)
	}
}

// choices are shown in the driver's own words ("A4 (Borderless)"), for every maker
func TestChoiceLabels(t *testing.T) {
	l := choiceLabels(readPPDFixture(t, "brother-dcpt510w.ppd"))
	if l["PageSize"]["BrA4_B"] != "A4 (Borderless)" || l["BRMediaType"]["Glossy"] != "Other Photo Paper" || l["BRResolution"]["PlainFast"] != "Plain Fast" {
		t.Errorf("Brother: %v / %v", l["PageSize"]["BrA4_B"], l["BRMediaType"]["Glossy"])
	}
	h := choiceLabels(readPPDFixture(t, "hp-like.ppd"))
	if h["PageSize"]["Photo4x6.FB"] != "4x6in Borderless" || h["OutputMode"]["FastDraft"] != "Fast Draft" {
		t.Errorf("HP: %v", h["PageSize"])
	}
}
