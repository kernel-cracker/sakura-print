// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Drivers: updates, the API (the computer only) and `sakuraprint doctor drivers`.

// ---------- updates: newer versions of makers' packages ----------

type drvUpdate struct {
	Device    string `json:"device"`
	Name      string `json:"name"`
	Package   string `json:"package"`
	Installed string `json:"installed"`
	Available string `json:"available"`
	Route     string `json:"route"` // maker | scan-maker
}

func driverUpdates() []drvUpdate {
	s := detectSystem()
	var ups []drvUpdate
	for _, d := range findDevices() {
		_, h := makerOf(d.Make)
		if d.QueueDriver != "driver" || h == nil || h.Lookup == nil {
			continue
		}
		info, err := makerLookup(h, d.Model)
		if err != nil || info == nil {
			continue
		}
		files := makerFiles(info, s.fileKind())
		route := map[string]string{}
		for _, f := range files {
			route[f] = "maker"
		}
		if drv := info["SCANNER_DRV"]; drv != "" && scanWorks(d, false) != "" {
			if f, err := scannerFile(h, drv, s); err == nil && f != "" {
				files, route[f] = append(files, f), "scan-maker"
			}
		}
		for _, f := range files {
			name, ver := pkgFileVersion(f)
			inst := s.packageInstalled(name)
			if s.Kind == "pacman" && inst == "" {
				if inst = s.packageInstalled("sakura-" + name); inst != "" {
					inst = strings.ReplaceAll(strings.TrimSuffix(inst, "-1"), "_", "-")
				} else {
					inst = s.packageInstalled(strings.ToLower(d.Make) + "-" + strings.ToLower(brotherModel(d.Model))) // the AUR's name
				}
			}
			if inst != "" && ver != "" && newerVersion(ver, inst) {
				ups = append(ups, drvUpdate{Device: d.Key, Name: d.Name, Package: name, Installed: inst, Available: ver, Route: route[f]})
			}
		}
	}
	return ups
}

// ---------- the API (the computer only) ----------

func serveDrivers(api func(string, func(http.ResponseWriter, *http.Request))) {
	localOnly := func(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if !isLocal(r) {
				fail(w, 403, "Printers can only be set up on the computer itself")
				return
			}
			h(w, r)
		}
	}
	var devMu sync.Mutex
	var lastDevs []device
	deviceByKey := func(key string) (device, bool) {
		devMu.Lock()
		defer devMu.Unlock()
		for _, d := range lastDevs {
			if d.Key == key {
				return d, true
			}
		}
		return device{}, false
	}
	api("/api/drivers", localOnly(func(w http.ResponseWriter, r *http.Request) {
		ds := findDevices()
		devMu.Lock()
		lastDevs = ds
		devMu.Unlock()
		if ds == nil {
			ds = []device{}
		}
		writeJSON(w, map[string]any{"system": detectSystem(), "devices": ds})
	}))
	api("/api/drivers/plan", localOnly(func(w http.ResponseWriter, r *http.Request) {
		d, ok := deviceByKey(r.URL.Query().Get("key"))
		if !ok {
			fail(w, 404, "that printer isn't in the list any more: look again")
			return
		}
		writeJSON(w, makePlan(d))
	}))
	api("/api/drivers/run", localOnly(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Key, Route, Choice string }
		if r.Method != http.MethodPost || readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		d, ok := deviceByKey(req.Key)
		if !ok {
			fail(w, 404, "that printer isn't in the list any more: look again")
			return
		}
		p := makePlan(d)
		all := append(p.Routes, p.Scan...)
		i := slices.IndexFunc(all, func(x route) bool {
			return x.ID == req.Route && x.State == "available" || x.ID == req.Route && x.State == "ready" && !strings.HasPrefix(x.ID, "scan-")
		})
		if i < 0 {
			fail(w, 400, "that way doesn't work for this printer")
			return
		}
		writeJSON(w, startDriverJob(d, all[i], req.Choice).view())
	}))
	api("/api/drivers/job/", localOnly(func(w http.ResponseWriter, r *http.Request) {
		id, what, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/drivers/job/"), "/")
		drvJobsMu.Lock()
		j := drvJobs[id]
		drvJobsMu.Unlock()
		if j == nil {
			fail(w, 404, "no such job")
			return
		}
		if r.Method == http.MethodPost {
			var ans jobAnswer
			if what == "file" { // the driver file, picked in the browser instead of found in Downloads
				r.Body = http.MaxBytesReader(w, r.Body, 300<<20)
				f, hdr, err := r.FormFile("file")
				if err != nil {
					fail(w, 400, "no file")
					return
				}
				defer f.Close()
				name := filepath.Base(hdr.Filename)
				os.MkdirAll(filepath.Join(driverCache(), "picked"), 0o700)
				p := filepath.Join(driverCache(), "picked", name)
				out, err := os.Create(p)
				if err != nil {
					fail(w, 500, err.Error())
					return
				}
				io.Copy(out, f)
				out.Close()
				ans = jobAnswer{OK: true, File: p}
			} else if readJSON(r, &ans) != nil {
				fail(w, 400, "bad request")
				return
			}
			select {
			case j.answers <- ans:
			default:
			}
			time.Sleep(150 * time.Millisecond)
		}
		writeJSON(w, j.view())
	}))
	api("/api/drivers/updates", localOnly(func(w http.ResponseWriter, r *http.Request) {
		ups := driverUpdates()
		if ups == nil {
			ups = []drvUpdate{}
		}
		writeJSON(w, ups)
	}))
}

// ---------- sakuraprint doctor drivers ----------

func doctorDrivers(w io.Writer) {
	s := detectSystem()
	fmt.Fprintf(w, "Installs with: %s\n", firstNonEmpty(kindNames[s.Kind]+" ("+s.Kind+")", "nothing Sakura Print knows"))
	fmt.Fprintf(w, "CUPS: %s\n", map[bool]string{true: fmt.Sprint(s.CUPS), false: "unknown"}[s.CUPS > 0])
	if _, err := os.Stat(driversOverridePath()); err == nil {
		fmt.Fprintf(w, "Your own hints: %s\n", driversOverridePath())
	}
	ds := findDevices()
	if len(ds) == 0 {
		fmt.Fprintln(w, "\nNo printers found (are they on, and on the same WiFi or plugged in?)")
		return
	}
	for _, d := range ds {
		fmt.Fprintf(w, "\n%s  (%s)\n", d.Name, d.DeviceID)
		if d.Queue != "" {
			fmt.Fprintf(w, "  print queue: %s, %s (%s)\n", d.Queue, d.QueueDriver, d.QueueModel)
		}
		p := makePlan(d)
		for _, r := range p.Routes {
			mark := map[string]string{"ready": "✔", "available": "•", "skipped": "·"}[r.State]
			line := fmt.Sprintf("  %s %-10s %s", mark, r.ID, r.Title)
			if r.Why != "" {
				line += ": " + r.Why
			} else if r.Detail != "" {
				line += ": " + r.Detail
			}
			if r.ID == p.Best {
				line += "   ← best"
			}
			fmt.Fprintln(w, line)
		}
		if len(p.Scan) > 0 {
			fmt.Fprintln(w, "  scanner:")
			for _, r := range p.Scan {
				mark := map[string]string{"ready": "✔", "available": "•", "skipped": "·"}[r.State]
				line := fmt.Sprintf("    %s %-15s %s", mark, r.ID, r.Title)
				if r.Why != "" {
					line += ": " + r.Why
				} else if r.Detail != "" {
					line += ": " + r.Detail
				}
				if r.ID == p.ScanBest && r.State != "ready" {
					line += "   ← best"
				}
				fmt.Fprintln(w, line)
			}
		}
	}
}
