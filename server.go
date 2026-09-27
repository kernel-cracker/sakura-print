// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The web server: starting up, cleaning old files, the routes (api_*.go), the embedded web app.

// ---------- the server ----------

var loginMu sync.Mutex

// setupVersion goes up when the setup screens learn something new, so printers set up before see them once more
const setupVersion = 2

var serverPort int // the port the server listens on (for opening the app from a notification)

func serve(addr string, port int) error {
	serverPort = port
	keepLog()
	log.Printf("Sakura Print %s starting (%s)", version, buildInfo)
	if loadSettings() {
		saveSettings()
	}
	root := dataDir()
	work := filepath.Join(root, "work")
	uploads := filepath.Join(root, "uploads")
	os.RemoveAll(work) // old builds from last time
	os.RemoveAll(filepath.Join(root, "thumbs"))
	os.MkdirAll(work, 0o755)
	os.MkdirAll(uploads, 0o755)
	cleanOld(uploads, 48*time.Hour)
	cleanOld(filepath.Join(root, "scans"), 7*24*time.Hour)
	cleanEdits()
	cleanConverted()
	go pruneHistory()
	cleanOld(filepath.Join(root, "proxies"), 3*24*time.Hour)

	go func() {
		for range time.Tick(30 * time.Minute) {
			stateMu.Lock()
			for id, b := range builds {
				if time.Since(b.Created) > 6*time.Hour {
					os.RemoveAll(b.Dir)
					delete(builds, id)
				}
			}
			for id, j := range jobs {
				if time.Since(j.Build.Created) > 6*time.Hour {
					delete(jobs, id)
				}
			}
			stateMu.Unlock()
			cleanOld(uploads, 48*time.Hour)
			cleanOld(filepath.Join(root, "proxies"), 3*24*time.Hour)
			cleanEdits()
		}
	}()

	go func() { // ask CUPS the slow questions now, so the first phone to open the app doesn't wait
		ps, _ := listPrinters()
		for _, p := range ps {
			ppdOptions(p)
			printerStatus(p)
		}
		listScanners(false)
	}()
	go buildIndex() // find PDFs and pictures all over the home folder, in the background
	go func() {
		for range time.Tick(10 * time.Minute) {
			buildIndex()
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/", staticFiles())

	api := func(path string, h func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if !authed(r) {
				fail(w, 401, "pin")
				return
			}
			h(w, r)
		})
	}

	s := &server{root: root, work: work, uploads: uploads, port: port, mux: mux, api: api}
	s.routesSession()
	s.routesInfo()
	s.routesPrinter()
	s.routesFiles()
	s.routesPrint()
	s.routesScan()
	serveEdits(api)                // the editor's autosave: edits.go
	serveHistory(api)              // Print again: history.go
	serveDrivers(api)              // Printers & drivers: drivers_api.go
	serveNotices(api)              // the notifications centre: notices.go
	serveDebug(api)                // Advanced: debug.go
	serveIPP(mux, api, work, port) // print from any app (AirPrint / Android): ipp.go
	srv := &http.Server{Addr: net.JoinHostPort(addr, strconv.Itoa(port)), Handler: protect(mux), ReadHeaderTimeout: 20 * time.Second}
	log.Printf("Sakura Print %s on http://localhost:%d", version, port)
	for _, u := range lanURLs(port) {
		log.Printf("  from a phone on the same WiFi: %s", u)
	}
	return srv.ListenAndServe()
}

// staticFiles serves the app compressed, and lets phones keep it: an unchanged file costs one tiny "still the same" reply
type asset struct {
	raw, gz []byte
	etag    string
	ctype   string
}

func staticFiles() http.Handler {
	web, _ := fs.Sub(webFiles, "web")
	assets := map[string]*asset{}
	// the app's code lives in web/js/ as numbered files (easier to read); phones get them as one app.js
	var app bytes.Buffer
	js, _ := fs.Glob(web, "js/*.js")
	sort.Strings(js)
	for _, p := range js {
		b, _ := fs.ReadFile(web, p)
		fmt.Fprintf(&app, "// ===== %s =====\n", p)
		app.Write(b)
		app.WriteString("\n")
	}
	fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, "js/") {
			return nil
		}
		raw, _ := fs.ReadFile(web, p)
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(raw)
		zw.Close()
		sum := sha256.Sum256(raw)
		ct := mime.TypeByExtension(filepath.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		assets["/"+p] = &asset{raw: raw, gz: buf.Bytes(), etag: `"` + hex.EncodeToString(sum[:8]) + `"`, ctype: ct}
		return nil
	})
	if app.Len() > 0 {
		raw := app.Bytes()
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(raw)
		zw.Close()
		sum := sha256.Sum256(raw)
		assets["/app.js"] = &asset{raw: raw, gz: buf.Bytes(), etag: `"` + hex.EncodeToString(sum[:8]) + `"`, ctype: "text/javascript; charset=utf-8"}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		a, ok := assets[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", a.etag)
		w.Header().Set("Cache-Control", "no-cache") // always check, but a match costs almost nothing
		w.Header().Set("Vary", "Accept-Encoding")
		if r.Header.Get("If-None-Match") == a.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", a.ctype)
		body := a.raw
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(a.gz) < len(a.raw) {
			w.Header().Set("Content-Encoding", "gzip")
			body = a.gz
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	})
}

func register(p string) map[string]string {
	tok := newID()
	stateMu.Lock()
	saved[tok] = p
	stateMu.Unlock()
	return map[string]string{"name": filepath.Base(p), "url": "/api/download/" + tok}
}

func saveUpload(fh *multipart.FileHeader, dir string) (*File, error) {
	name := filepath.Base(strings.ReplaceAll(fh.Filename, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	sub := filepath.Join(dir, newID())
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return nil, err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	p := filepath.Join(sub, name)
	dst, err := os.Create(p)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return nil, err
	}
	dst.Close()
	return addFile(p, name)
}

func cleanOld(dir string, age time.Duration) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}
