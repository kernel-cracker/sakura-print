// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// What a printer can do: colour, both sides by itself or with help, a scanner.

// ---------- what a printer can do ----------

// Caps is what a printer can do, going by its driver and the scanners on the network
type Caps struct {
	Colour     bool   `json:"colour"`
	AutoDuplex bool   `json:"autoDuplex"`          // prints both sides by itself
	CanAuto    bool   `json:"canAuto"`             // the driver says it can, even if the user said it can't
	DuplexKey  string `json:"duplexKey,omitempty"` // the driver option for it, hidden from "More printer settings"
	Scanner    string `json:"scanner"`             // a scanner that looks like this printer, "" = none
	ScanKnown  bool   `json:"scanKnown"`           // false while still looking for scanners
}

// driver option names printers use for printing both sides by themselves
var duplexKeys = []string{"Duplex", "sides", "BRDuplex", "EFDuplex", "KMDuplex", "JCLDuplex", "OKDuplex", "CNDuplex", "XRDuplex"}

var (
	duplexOnLong = regexp.MustCompile(`(?i)notumble|long-?edge`)
	duplexOn     = regexp.MustCompile(`(?i)tumble|two-?sided|double|duplex|^on$|^true$`)
	duplexOff    = regexp.MustCompile(`(?i)^(none|off|false|one-?sided|single|simplex)$`)
	notInstalled = regexp.MustCompile(`(?i)^(false|none|notinstalled|not-installed)$`)
	colourValue  = regexp.MustCompile(`(?i)colou?r|rgb|cmyk`)
)

// duplexOption finds the driver option that makes the printer print both sides by itself:
// on = the normal "flip on the long edge" value, off = one side only
func duplexOption(opts []PPDOption) (key, on, off string) {
	for _, o := range opts { // a driver can list duplex even when the duplex unit isn't fitted
		if strings.Contains(strings.ToLower(o.Key), "duplexer") || strings.Contains(strings.ToLower(o.Key), "duplexunit") {
			if notInstalled.MatchString(o.Default) {
				return "", "", ""
			}
		}
	}
	for _, o := range opts {
		if !slices.Contains(duplexKeys, o.Key) {
			continue
		}
		on, off = "", ""
		for _, v := range o.Values {
			switch {
			case duplexOff.MatchString(v):
				off = v
			case duplexOnLong.MatchString(v) && (on == "" || !duplexOnLong.MatchString(on)):
				on = v
			case duplexOn.MatchString(v) && on == "":
				on = v
			}
		}
		if on != "" {
			return o.Key, on, off
		}
	}
	return "", "", ""
}

// autoDuplex says if this printer should print both sides by itself (the driver can, and the user
// didn't say it can't), and which option does it
// duplexShortValue: the duplex choice that binds on the short edge ("DuplexTumble")
func duplexShortValue(opts []PPDOption, key string) string {
	for _, o := range opts {
		if o.Key != key {
			continue
		}
		for _, v := range o.Values {
			if strings.Contains(strings.ToLower(v), "tumble") && !duplexOnLong.MatchString(v) || strings.Contains(strings.ToLower(v), "short") {
				return v
			}
		}
	}
	return ""
}

func autoDuplex(printer string) (key, on, off string, ok bool) {
	key, on, off = duplexOption(ppdOptions(printer))
	if key == "" {
		return "", "", "", false
	}
	settingsMu.Lock()
	manual := settings.Duplex[printer] == "manual"
	settingsMu.Unlock()
	return key, on, off, !manual
}

// modelKeys turns "Brother DCP-T510W" or "DCPT510W-Brother" into the part that names the model: "dcpt510w"
func modelKeys(s string) []string {
	var keys []string
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ' ' || r == '_' || r == ',' || r == '(' || r == ')' }) {
		k := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, f)
		if len(k) >= 4 && strings.ContainsAny(k, "0123456789") {
			keys = append(keys, k)
		}
	}
	return keys
}

// scannerFor finds a scanner on the network that looks like this printer
func scannerFor(printer, model string, scanners []Scanner) string {
	keys := append(modelKeys(model), modelKeys(strings.ReplaceAll(printer, "-", " "))...)
	for _, sc := range scanners {
		name := strings.Join(modelKeys(sc.Name+" "+sc.Device), " ")
		for _, k := range keys {
			if strings.Contains(name, k) {
				return sc.Name
			}
		}
	}
	return ""
}

func capsFor(printer string) Caps {
	opts := ppdOptions(printer)
	c := Caps{}
	for _, o := range opts {
		if o.Key == "BRMonoColor" || o.Key == "ColorModel" || o.Key == "print-color-mode" {
			c.Colour = slices.ContainsFunc(o.Values, colourValue.MatchString)
		}
	}
	key, _, _, auto := autoDuplex(printer)
	c.DuplexKey, c.CanAuto, c.AutoDuplex = key, key != "", auto
	scs, known := scannersCache.peek()
	c.ScanKnown = known
	if known {
		a := printerCache(attrsCache, printer, time.Minute, readAttrs).get()
		c.Scanner = scannerFor(printer, a["printer-make-and-model"], scs)
	}
	return c
}
