// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Print from any app: Sakura Print pretends to be a printer on the WiFi (IPP, the language AirPrint on iPhones
// and the built-in printing on Android both speak), so the normal Share → Print in Photos, WhatsApp, Safari,
// Mail... works without opening Sakura Print. Jobs still go through Sakura's pipeline: the right page order for
// the real printer, double-sided by hand (the app shows the flip guide), and "Print again".
//
// Only the parts of IPP phones actually use are here: describe the printer, take a job, report on it, cancel it.
// iPhones may send pages as Apple raster ("URF", image/urf), Android and Linux as PWG raster, instead of PDF;
// those are turned into a PDF.

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---------- wiring it in ----------

func serveIPP(mux *http.ServeMux, api func(string, func(http.ResponseWriter, *http.Request)), work string, port int) {
	settingsMu.Lock()
	if settings.PrinterUUID == "" {
		b := []byte(randHex(16))
		settings.PrinterUUID = fmt.Sprintf("%s-%s-%s-%s-%s", b[0:8], b[8:12], b[12:16], b[16:20], b[20:32])
		saveSettings()
	}
	uuid := settings.PrinterUUID
	settingsMu.Unlock()
	ippSrv = &ippServer{jobs: map[int]*ippJob{}, work: work, port: port, up: time.Now(), uuid: uuid}
	mux.HandleFunc("/ipp/print", ippSrv.handle)
	mux.HandleFunc("/ipp/print/", ippSrv.handle)
	// prints from phones that haven't logged in, waiting to be allowed: any logged-in phone or the computer decides
	api("/api/airprint/waiting", func(w http.ResponseWriter, r *http.Request) {
		ippSrv.mu.Lock()
		out := []map[string]any{}
		for _, j := range ippSrv.waiting() {
			v := map[string]any{"id": j.id, "title": j.name, "from": j.user, "when": j.created}
			if j.file != nil {
				v["file"], v["pages"] = j.file.ID, j.file.Pages
			}
			out = append(out, v)
		}
		ippSrv.mu.Unlock()
		writeJSON(w, out)
	})
	api("/api/airprint/waiting/", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Action string }
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/airprint/waiting/"))
		if r.Method != http.MethodPost || err != nil || readJSON(r, &req) != nil || (req.Action != "allow" && req.Action != "trust" && req.Action != "refuse") {
			fail(w, 400, "bad request")
			return
		}
		if err := ippSrv.decide(id, req.Action); err != nil {
			fail(w, 409, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	go func() { // keep the announcement in step with the switches (phone access, printing from other apps)
		for {
			airAd.sync(port, uuid)
			time.Sleep(5 * time.Second)
		}
	}()
}

func localName() string { h, _ := os.Hostname(); return h }
