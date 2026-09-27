// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Drivers, part 3: finding printers, working out how to get each one's driver, and doing it (docs/design/drivers.md).
//
// Almost nothing is written in: the printer says what it is, CUPS says which installed driver fits it
// (lpinfo --device-id), the package manager says what exists. The hints in assets/drivers.json (package names,
// makers' sites, Brother's lookup) are only tried after checking them, and ~/.config/sakuraprint/drivers.json can
// replace them without a new Sakura Print.
//
// Routes, in order: installed · official (distribution) · maker (maker's server) · guided (maker's site) ·
// aur · open · app (Printer Application) · driverless.

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed assets/drivers.json
var driversJSON []byte

type makerHint struct {
	Names    []string            `json:"names"`
	Site     string              `json:"site"`
	Vendor   string              `json:"vendor"`
	Official map[string][]string `json:"official"`
	Open     map[string][]string `json:"open"`
	Lookup   *struct {
		Kind, List, File string
	} `json:"lookup"`
	ConvertForPacman bool     `json:"convertForPacman"`
	ArchDepends      []string `json:"archDepends"` // what its packages need on Arch that they don't say (their programs call these)
	PrinterApp       string   `json:"printerApp"`
	HPPlugin         bool     `json:"hpPlugin"`
}

type hintsFile struct {
	Makers          map[string]*makerHint `json:"makers"`
	Any             makerHint             `json:"any"`
	Lib32           map[string]string     `json:"lib32"`
	DownloadDomains []string              `json:"downloadDomains"`
}

var (
	hintsMu sync.Mutex
	hints   *hintsFile
	hintsAt time.Time
)

func driversOverridePath() string { return filepath.Join(filepath.Dir(configPath()), "drivers.json") }

// driverHints: the built-in hints, with the person's own file laid over them
func driverHints() *hintsFile {
	hintsMu.Lock()
	defer hintsMu.Unlock()
	if hints != nil && time.Since(hintsAt) < 10*time.Second {
		return hints
	}
	h := &hintsFile{}
	json.Unmarshal(driversJSON, h)
	if data, err := os.ReadFile(driversOverridePath()); err == nil {
		var o hintsFile
		if json.Unmarshal(data, &o) == nil {
			for k, v := range o.Makers {
				h.Makers[k] = v
			}
			if o.Any.Official != nil || o.Any.Open != nil || o.Any.PrinterApp != "" {
				h.Any = o.Any
			}
			for k, v := range o.Lib32 {
				h.Lib32[k] = v
			}
			h.DownloadDomains = append(h.DownloadDomains, o.DownloadDomains...)
		}
	}
	hints, hintsAt = h, time.Now()
	return h
}

// makerOf finds the maker's hints from the printer's make ("Brother", "Hewlett-Packard"…)
func makerOf(brand string) (string, *makerHint) {
	m := strings.ToLower(strings.TrimSpace(brand))
	h := driverHints()
	keys := make([]string, 0, len(h.Makers))
	for k := range h.Makers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, n := range h.Makers[k].Names {
			if m == n || strings.HasPrefix(m, n+" ") {
				return k, h.Makers[k]
			}
		}
	}
	return "", nil
}
