// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Which of a printer driver's options does what, for the print screen, whatever the maker: colour, quality, paper
// type, and the driver's own both-sides option (left to Sakura Print's both-sides switch). The same matching as for
// phones (ipp_options.go: the known option names of many makers, then the option's label), so no maker is special.
// And every choice's own words from the driver ("A4 (Borderless)", "Other Photo Paper") for the screen.

import (
	"regexp"
	"slices"
	"strings"
)

type optRoles struct {
	Colour    string   `json:"colour,omitempty"`   // the colour option
	ColourOn  string   `json:"colourOn,omitempty"` // its choice for colour
	MonoOn    string   `json:"monoOn,omitempty"`   // and for black & white
	Quality   string   `json:"quality,omitempty"`
	MediaType string   `json:"mediaType,omitempty"`
	Duplex    []string `json:"duplex"` // the driver's own both-sides options
}

var (
	colourLabel  = regexp.MustCompile(`(?i)colou?r|gr[ae]y|mono`)
	qualityLabel = regexp.MustCompile(`(?i)quality`)
	mediaLabel   = regexp.MustCompile(`(?i)media.?type|paper.?type`)
	colourChoice = regexp.MustCompile(`(?i)colou?r|rgb|cmyk`)
	monoChoice   = regexp.MustCompile(`(?i)mono|gr[ae]y|black|kgray`)
)

func optionRoles(opts []PPDOption) optRoles {
	r := optRoles{Duplex: []string{}}
	find := func(keys []string, label *regexp.Regexp) *PPDOption {
		for _, k := range keys {
			for i := range opts {
				if opts[i].Key == k && len(opts[i].Values) > 0 {
					return &opts[i]
				}
			}
		}
		for i := range opts {
			if label.MatchString(opts[i].Label) && len(opts[i].Values) > 0 && !slices.Contains(duplexKeys, opts[i].Key) {
				return &opts[i]
			}
		}
		return nil
	}
	if o := find(colorKeys, colourLabel); o != nil {
		boolean := len(o.Values) == 2 && slices.ContainsFunc(o.Values, func(v string) bool { return strings.EqualFold(v, "true") })
		for _, v := range o.Values {
			switch {
			case boolean && regexp.MustCompile(`(?i)gr[ae]y|mono|black`).MatchString(o.Label): // "Grayscale printing: True/False"
				if strings.EqualFold(v, "true") {
					r.MonoOn = v
				} else {
					r.ColourOn = v
				}
			case r.MonoOn == "" && monoChoice.MatchString(v):
				r.MonoOn = v
			case r.ColourOn == "" && colourChoice.MatchString(v):
				r.ColourOn = v
			}
		}
		if r.MonoOn != "" || r.ColourOn != "" {
			r.Colour = o.Key
		}
	}
	if o := find(qualityKeys, qualityLabel); o != nil && o.Key != r.Colour {
		r.Quality = o.Key
	}
	if o := find(mediaTypeKeys, mediaLabel); o != nil {
		r.MediaType = o.Key
	}
	for _, o := range opts {
		if slices.Contains(duplexKeys, o.Key) {
			r.Duplex = append(r.Duplex, o.Key)
		}
	}
	return r
}

// choiceLabels: option → choice → the driver's own words for it
func choiceLabels(p ppdInfo) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, k := range p.Order {
		m := map[string]string{}
		for _, c := range p.Options[k].Choices {
			if c.Text != "" && c.Text != c.Name {
				m[c.Name] = c.Text
			}
		}
		if len(m) > 0 {
			out[k] = m
		}
	}
	return out
}
