// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scanning with the printer's scanner (SANE), and saving scans. (see docs/design/server.md)

func (s *server) routesScan() {
	work, api := s.work, s.api
	api("/api/scanners", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listScanners(r.URL.Query().Get("refresh") == "1"))
	})

	api("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Device string
			DPI    int
			Colour bool
		}
		if readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		f, err := scanPage(req.Device, req.DPI, req.Colour)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, f)
	})

	api("/api/scan/save", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs    []string
			Format string // pdf | images
		}
		if readJSON(r, &req) != nil || len(req.IDs) == 0 {
			fail(w, 400, "nothing to save")
			return
		}
		dest := filepath.Join(userDir("DOCUMENTS", "Documents"), "Scans")
		os.MkdirAll(dest, 0o755)
		stamp := time.Now().Format("2006-01-02 15-04-05")
		var out []map[string]string
		if req.Format == "images" {
			for i, id := range req.IDs {
				f, ok := getFile(id)
				if !ok {
					continue
				}
				ext := strings.ToLower(filepath.Ext(f.Path)) // camera pages are JPEGs, scanner pages PNGs: keep what it is
				if ext != ".jpg" && ext != ".jpeg" {
					ext = ".png"
				}
				p := filepath.Join(dest, fmt.Sprintf("Scan %s (%d)%s", stamp, i+1, ext))
				data, err := os.ReadFile(f.Path)
				if err != nil || os.WriteFile(p, data, 0o644) != nil {
					continue
				}
				out = append(out, register(p))
			}
		} else {
			var paths []string
			for _, id := range req.IDs {
				if f, ok := getFile(id); ok {
					paths = append(paths, f.Path)
				}
			}
			tmp := filepath.Join(work, newID())
			os.MkdirAll(tmp, 0o755)
			p := filepath.Join(dest, "Scan "+stamp+".pdf")
			if _, err := photoPDF(paths, "A4", 1, false, tmp, p, false); err != nil {
				fail(w, 500, err.Error())
				return
			}
			out = append(out, register(p))
		}
		writeJSON(w, map[string]any{"saved": out, "folder": strings.Replace(dest, home(), "~", 1)})
	})

}
