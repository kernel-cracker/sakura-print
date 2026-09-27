// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Printing: building the print-ready PDF, previews, sending it, and following a job through the flip. (see docs/design/server.md)

func (s *server) routesPrint() {
	work, api := s.work, s.api
	api("/api/build", func(w http.ResponseWriter, r *http.Request) {
		var spec Spec
		if readJSON(r, &spec) != nil {
			fail(w, 400, "bad request")
			return
		}
		b, err := build(spec, work)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		stateMu.Lock()
		builds[b.ID] = b
		stateMu.Unlock()
		writeJSON(w, b)
	})

	api("/api/preview/", func(w http.ResponseWriter, r *http.Request) {
		// /api/preview/{build}/{page}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/preview/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		stateMu.Lock()
		b, ok := builds[parts[0]]
		stateMu.Unlock()
		n, err := strconv.Atoi(parts[1])
		if !ok || err != nil || n < 1 || n > b.Pages {
			http.NotFound(w, r)
			return
		}
		out := filepath.Join(b.Dir, fmt.Sprintf("preview-%d", n))
		if _, err := os.Stat(out + ".jpg"); err != nil {
			if o, err := exec.Command("pdftoppm", "-jpeg", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-scale-to", "420", "-singlefile", b.Combined, out).CombinedOutput(); err != nil {
				http.Error(w, string(o), 500)
				return
			}
		}
		http.ServeFile(w, r, out+".jpg")
	})

	api("/api/print", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Build string
			Test  bool
		}
		if readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		stateMu.Lock()
		b, ok := builds[req.Build]
		stateMu.Unlock()
		if !ok {
			fail(w, 400, "that preview expired, press Preview again")
			return
		}
		if !validPrinter(b.Printer) {
			fail(w, 400, "pick a printer first")
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.job != nil { // double-tap: hand back the job that's already printing
			b.job.mu.Lock()
			v := b.job.view()
			b.job.mu.Unlock()
			writeJSON(w, v)
			return
		}
		j, err := printBuilt(b, req.Test, "")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, j.view())
	})

	api("/api/jobs/waiting", func(w http.ResponseWriter, r *http.Request) {
		// prints sent from another app's Print button that are waiting for the paper to be flipped
		stateMu.Lock()
		var all []*Job
		for _, j := range jobs {
			all = append(all, j)
		}
		stateMu.Unlock()
		sort.Slice(all, func(a, b int) bool { return all[a].Created.Before(all[b].Created) })
		out := []JobView{}
		for _, j := range all {
			j.mu.Lock()
			if j.Source == "airprint" {
				j.refresh()
				if j.State == "flip" {
					out = append(out, j.view())
				}
			}
			j.mu.Unlock()
		}
		writeJSON(w, out)
	})

	api("/api/job/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/job/")
		action := ""
		if i := strings.Index(id, "/"); i >= 0 {
			id, action = id[:i], id[i+1:]
		}
		stateMu.Lock()
		j, ok := jobs[id]
		stateMu.Unlock()
		if !ok {
			fail(w, 404, "no such job")
			return
		}
		j.mu.Lock()
		defer j.mu.Unlock()
		switch action {
		case "":
			j.refresh()
			writeJSON(w, j.view())
		case "side2":
			if err := j.side2(); err != nil {
				fail(w, 400, err.Error())
				return
			}
			writeJSON(w, j.view())
		case "cancel": // just this print
			j.cancel()
			writeJSON(w, j.view())
		default:
			fail(w, 404, "unknown action")
		}
	})

}
