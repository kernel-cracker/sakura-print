// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Both-sides profiles: how each printer's paper has to be turned over (shared with the old pdftool CLI).

// ---------- duplex profiles, shared with the pdftool CLI (~/.config/pdftool/printers) ----------

type Profile struct {
	FaceUp bool   `json:"faceUp"`
	First  string `json:"first"` // even|odd
	R1     bool   `json:"r1"`
	R2     bool   `json:"r2"`
	Rot    bool   `json:"rot"`
}

func profilePath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home(), ".config")
	}
	return filepath.Join(base, "pdftool", "printers")
}

func defaultProfile() Profile { return Profile{FaceUp: true, First: "even", R1: false, R2: true} }

func loadProfile(printer string) (Profile, bool) {
	data, err := os.ReadFile(profilePath())
	if err != nil {
		return defaultProfile(), false
	}
	for _, l := range strings.Split(string(data), "\n") {
		f := strings.Split(l, "|")
		if len(f) >= 2 && f[0] == printer {
			p := defaultProfile()
			p.FaceUp = f[1] == "1"
			if len(f) >= 5 {
				if f[2] == "odd" || f[2] == "even" {
					p.First = f[2]
				}
				p.R1, p.R2 = f[3] == "1", f[4] == "1"
			}
			if len(f) >= 6 {
				p.Rot = f[5] == "1"
			}
			return p, true
		}
	}
	return defaultProfile(), false
}

func saveProfile(printer string, p Profile) error {
	path := profilePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	var keep []string
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if l != "" && !strings.HasPrefix(l, printer+"|") {
				keep = append(keep, l)
			}
		}
	}
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	keep = append(keep, strings.Join([]string{printer, b(p.FaceUp), p.First, b(p.R1), b(p.R2), b(p.Rot)}, "|"))
	return os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}

// sim models a 4-page, 2-sheet manual duplex test: returns (page behind page 1, page on top)
func sim(faceUp bool, feedLast bool, first string, r1, r2 bool) (int, int) {
	l1, l2 := []int{2, 4}, []int{1, 3}
	if first == "odd" {
		l1, l2 = []int{1, 3}, []int{2, 4}
	}
	if r1 {
		l1 = []int{l1[1], l1[0]}
	}
	if r2 {
		l2 = []int{l2[1], l2[0]}
	}
	f2 := []int{l1[0], l1[1]}
	if feedLast {
		f2 = []int{l1[1], l1[0]}
	}
	b1 := 0
	for i := 0; i < 2; i++ {
		if f2[i] == 1 {
			b1 = l2[i]
		}
		if l2[i] == 1 {
			b1 = f2[i]
		}
	}
	top := f2[1]
	if faceUp {
		top = l2[1]
	}
	return b1, top
}

// calibrate finds the page order that gives a perfect stack, given what the test print looked like
func calibrate(cur Profile, back, top int) (Profile, bool) {
	for _, fu := range []bool{cur.FaceUp, !cur.FaceUp} {
		for _, fl := range []bool{true, false} {
			b, t := sim(fu, fl, cur.First, cur.R1, cur.R2)
			if b != back || t != top {
				continue
			}
			for _, fp := range []string{"even", "odd"} {
				for _, r1 := range []bool{false, true} {
					for _, r2 := range []bool{false, true} {
						if b2, t2 := sim(fu, fl, fp, r1, r2); b2 == 2 && t2 == 1 {
							return Profile{FaceUp: fu, First: fp, R1: r1, R2: r2, Rot: cur.Rot}, true
						}
					}
				}
			}
		}
	}
	return cur, false
}
