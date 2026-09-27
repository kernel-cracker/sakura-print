// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The printer: its driver options, status, maintenance commands, the calibration test page and the both-sides profile. (see docs/design/server.md)

func (s *server) routesPrinter() {
	work, api := s.work, s.api
	api("/api/options", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("printer")
		if !validPrinter(p) {
			fail(w, 400, "unknown printer")
			return
		}
		_, has := loadProfile(p)
		settingsMu.Lock()
		style, seen := settings.Styles[p], settings.Setup[p]
		settingsMu.Unlock()
		opts := ppdOptions(p)
		writeJSON(w, map[string]any{"options": opts, "calibrated": has, "style": style,
			"caps": capsFor(p), "setupDone": seen >= setupVersion,
			"roles": optionRoles(opts), "labels": choiceLabels(parsePPD(queuePPD(p)))}) // any maker's driver: options.go
	})

	api("/api/printer", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("printer")
		if !validPrinter(p) {
			fail(w, 400, "unknown printer")
			return
		}
		writeJSON(w, printerStatus(p))
	})

	api("/api/maintenance", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Printer, Action string }
		if readJSON(r, &req) != nil || !validPrinter(req.Printer) {
			fail(w, 400, "bad request")
			return
		}
		var err error
		if req.Action == "cancel" {
			err = cancelAll(req.Printer)
		} else {
			err = maintenance(req.Printer, req.Action)
		}
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	api("/api/test", func(w http.ResponseWriter, r *http.Request) {
		// builds the 4-page calibration test for a printer
		var req struct{ Printer string }
		if readJSON(r, &req) != nil || !validPrinter(req.Printer) {
			fail(w, 400, "pick a printer")
			return
		}
		id := newID()
		dir := filepath.Join(work, id)
		os.MkdirAll(dir, 0o755)
		tp, err := testPDF(dir)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		f, err := addFile(tp, "duplex test.pdf")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		b, err := build(Spec{Printer: req.Printer, Mode: "docs", Sides: "duplex", Items: []Item{{ID: f.ID}}, Copies: 1, Cover: 1}, work)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		stateMu.Lock()
		builds[b.ID] = b
		stateMu.Unlock()
		writeJSON(w, b)
	})

	api("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Printer        string
			Style          string
			FaceUp         *bool
			Back, Top      int
			UpsideDown     bool
			Answer, Forget bool
			Setup          bool   // the setup screens were finished
			Duplex         string // "auto" or "manual": can it really print both sides by itself?
		}
		if readJSON(r, &req) != nil || !validPrinter(req.Printer) {
			fail(w, 400, "bad request")
			return
		}
		if req.Setup || req.Duplex != "" {
			settingsMu.Lock()
			if req.Setup {
				settings.Setup[req.Printer] = setupVersion
			}
			switch req.Duplex {
			case "manual":
				settings.Duplex[req.Printer] = "manual"
			case "auto":
				delete(settings.Duplex, req.Printer)
			}
			saveSettings()
			settingsMu.Unlock()
		}
		if req.Style == "rear" || req.Style == "tray" {
			settingsMu.Lock()
			settings.Styles[req.Printer] = req.Style
			saveSettings()
			settingsMu.Unlock()
			if req.FaceUp == nil && !req.Answer && !req.Forget {
				writeJSON(w, map[string]string{"result": "saved"})
				return
			}
		}
		p, _ := loadProfile(req.Printer)
		result := "saved"
		switch {
		case req.Forget:
			path := profilePath()
			data, _ := os.ReadFile(path)
			var keep []string
			for _, l := range strings.Split(string(data), "\n") {
				if l != "" && !strings.HasPrefix(l, req.Printer+"|") {
					keep = append(keep, l)
				}
			}
			os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
			writeJSON(w, map[string]string{"result": "forgotten"})
			return
		case req.Answer:
			if req.UpsideDown {
				p.Rot = !p.Rot
			}
			switch {
			case req.Back == 1 || req.Back == 3 || req.Back == 0:
				result = "noflip"
			case req.Back == 2 && req.Top == 1:
				result = "perfect"
				if req.UpsideDown {
					result = "updated"
				}
			default:
				if np, ok := calibrate(p, req.Back, req.Top); ok {
					p = np
					result = "updated"
				} else {
					result = "unknown"
				}
			}
		case req.FaceUp != nil:
			p.FaceUp = *req.FaceUp
		}
		if err := saveProfile(req.Printer, p); err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"result": result, "profile": p})
	})

}
