// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Files: uploads, the computer's files, thumbnails, single pages as pictures, replacing a page, downloads. (see docs/design/server.md)

func (s *server) routesFiles() {
	work, uploads, api := s.work, s.uploads, s.api
	api("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, "POST only")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			fail(w, 400, "upload too big or broken (500 MB max)")
			return
		}
		added, errs := []*File{}, []string{}
		for _, fh := range r.MultipartForm.File["files"] {
			f, err := saveUpload(fh, uploads)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			added = append(added, f)
		}
		writeJSON(w, map[string]any{"files": added, "errors": errs})
	})

	api("/api/local", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		files, total, ready := searchFiles(q.Get("kind"), q.Get("q"), 300)
		indexMu.Lock()
		busy := indexing
		indexMu.Unlock()
		writeJSON(w, map[string]any{"files": files, "total": total, "ready": ready, "busy": busy})
	})

	api("/api/local/add", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Paths []string }
		if readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		added, errs := []*File{}, []string{}
		for _, p := range req.Paths {
			clean, err := filepath.EvalSymlinks(filepath.Clean(p)) // follow links, so a link can't point outside home
			if err != nil {
				errs = append(errs, filepath.Base(p)+": not found")
				continue
			}
			realHome, _ := filepath.EvalSymlinks(home())
			st, err := os.Stat(clean)
			if err != nil || !st.Mode().IsRegular() || !strings.HasPrefix(clean, realHome+"/") {
				errs = append(errs, filepath.Base(p)+": not allowed")
				continue
			}
			f, err := addFile(clean, filepath.Base(clean))
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			added = append(added, f)
		}
		writeJSON(w, map[string]any{"files": added, "errors": errs})
	})

	api("/api/thumb/", func(w http.ResponseWriter, r *http.Request) {
		f, ok := getFile(strings.TrimPrefix(r.URL.Path, "/api/thumb/"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		p, err := thumbnail(f)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Cache-Control", "max-age=3600")
		http.ServeFile(w, r, p)
	})

	api("/api/pageimage", func(w http.ResponseWriter, r *http.Request) {
		// one page of a PDF as a picture, so the editor can work on it
		var req struct {
			File string
			Page int
		}
		if readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		f, ok := getFile(req.File)
		if !ok || f.Kind != "pdf" || req.Page < 1 || req.Page > f.Pages {
			fail(w, 400, "that page isn't there anymore")
			return
		}
		dir := filepath.Join(work, newID())
		os.MkdirAll(dir, 0o755)
		base := filepath.Join(dir, "page")
		if o, err := exec.Command("pdftoppm", "-png", "-r", "200", "-f", strconv.Itoa(req.Page), "-l", strconv.Itoa(req.Page), "-singlefile", f.Path, base).CombinedOutput(); err != nil {
			fail(w, 500, "couldn't open that page: "+strings.TrimSpace(string(o)))
			return
		}
		img, err := addFile(base+".png", fmt.Sprintf("%s page %d.png", strings.TrimSuffix(f.Name, filepath.Ext(f.Name)), req.Page))
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		notePage(img, f, req.Page) // so the editor can find this page's earlier edits
		writeJSON(w, img)
	})

	api("/api/replacepage", func(w http.ResponseWriter, r *http.Request) {
		// put an edited picture of a page back into the PDF (as a new file, the original is never changed)
		var req struct {
			File, Image string
			Page        int
		}
		if readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		f, ok1 := getFile(req.File)
		im, ok2 := getFile(req.Image)
		if !ok1 || !ok2 || f.Kind != "pdf" || im.Kind != "image" {
			fail(w, 400, "that page isn't there anymore")
			return
		}
		dir := filepath.Join(uploads, newID())
		os.MkdirAll(dir, 0o755)
		name := strings.TrimSuffix(f.Name, " (edited).pdf")
		name = strings.TrimSuffix(name, filepath.Ext(name)) + " (edited).pdf"
		out := filepath.Join(dir, name)
		if err := replacePage(f.Path, req.Page, im.Path, dir, out); err != nil {
			fail(w, 500, err.Error())
			return
		}
		nf, err := addFile(out, name)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		carryPages(f, nf, req.Page, im) // the new copy remembers which of its pages can be re-edited from the original
		writeJSON(w, nf)
	})

	api("/api/file/", func(w http.ResponseWriter, r *http.Request) {
		// the full-size picture, for the editor (pictures only, never PDFs or other files)
		f, ok := getFile(strings.TrimPrefix(r.URL.Path, "/api/file/"))
		if !ok || f.Kind != "image" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.ServeFile(w, r, f.Path)
	})

	api("/api/pdf/", func(w http.ResponseWriter, r *http.Request) {
		stateMu.Lock()
		b, ok := builds[strings.TrimPrefix(r.URL.Path, "/api/pdf/")]
		stateMu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", `inline; filename="sakura-print.pdf"`)
		http.ServeFile(w, r, b.Combined)
	})

	api("/api/download/", func(w http.ResponseWriter, r *http.Request) {
		stateMu.Lock()
		p, ok := saved[strings.TrimPrefix(r.URL.Path, "/api/download/")]
		stateMu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filepath.Base(p), `"`, "")))
		http.ServeFile(w, r, p)
	})

}
