// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Print again: every print is remembered (one copy of exactly what went to the printer, the settings, a small
// preview), so yesterday's worksheet is one tap away. The last 30, for 30 days; test pages aren't kept.
//
//	~/.local/share/sakuraprint/history/<id>/job.pdf    one copy, covers and all
//	~/.local/share/sakuraprint/history/<id>/thumb.jpg  its first page, small
//	~/.local/share/sakuraprint/history/<id>/meta.json  what it was and how it was printed

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	historyKeep = 30
	historyAge  = 30 * 24 * time.Hour
)

type HistoryEntry struct {
	ID      string            `json:"id"`
	When    time.Time         `json:"when"`
	Title   string            `json:"title"`
	Pages   int               `json:"pages"` // in one copy
	Copies  int               `json:"copies"`
	Sides   string            `json:"sides"` // single | duplex
	Mode    string            `json:"mode"`  // docs | photos | copy | text
	Printer string            `json:"printer"`
	Options map[string]string `json:"options"`
}

var (
	historyMu sync.Mutex
	idRe      = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func historyDir() string { return filepath.Join(dataDir(), "history") }

// remember keeps a print for "Print again" (called once a print has been sent)
func remember(b *Built) error {
	sp := b.Spec
	if b.PerCopy < 1 || b.Combined == "" {
		return errors.New("nothing to keep")
	}
	e := HistoryEntry{ID: newID(), When: time.Now(), Pages: b.PerCopy, Copies: max(1, sp.Copies), Sides: sp.Sides, Mode: sp.Mode,
		Printer: b.Printer, Options: sp.Options}
	if e.Sides != "duplex" {
		e.Sides = "single"
	}
	var names []string
	for _, it := range sp.Items {
		if f, ok := getFile(it.ID); ok {
			names = append(names, strings.TrimSuffix(f.Name, filepath.Ext(f.Name)))
		}
	}
	switch {
	case len(names) == 0:
		e.Title = "Print"
	case len(names) == 1:
		e.Title = names[0]
	default:
		e.Title = fmt.Sprintf("%s and %d more", names[0], len(names)-1)
	}
	dir := filepath.Join(historyDir(), e.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// one copy: the first PerCopy pages (the rest are the same pages again)
	if err := qpdf("--empty", "--pages", b.Combined, "1-"+strconv.Itoa(b.PerCopy), "--", filepath.Join(dir, "job.pdf")); err != nil {
		os.RemoveAll(dir)
		return err
	}
	exec.Command("pdftoppm", "-jpeg", "-f", "1", "-l", "1", "-scale-to", "240", "-singlefile", filepath.Join(dir, "job.pdf"), filepath.Join(dir, "thumb")).Run()
	data, _ := json.MarshalIndent(e, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o600); err != nil {
		return err
	}
	pruneHistory()
	return nil
}

func listHistory() []HistoryEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	entries, _ := os.ReadDir(historyDir())
	var out []HistoryEntry
	for _, d := range entries {
		var e HistoryEntry
		data, err := os.ReadFile(filepath.Join(historyDir(), d.Name(), "meta.json"))
		if err != nil || json.Unmarshal(data, &e) != nil || e.ID != d.Name() {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	return out
}

// pruneHistory keeps the newest 30, none older than 30 days
func pruneHistory() {
	all := listHistory()
	historyMu.Lock()
	defer historyMu.Unlock()
	for i, e := range all {
		if i >= historyKeep || time.Since(e.When) > historyAge {
			os.RemoveAll(filepath.Join(historyDir(), e.ID))
		}
	}
}

func historyEntry(id string) (HistoryEntry, string, bool) {
	if !idRe.MatchString(id) {
		return HistoryEntry{}, "", false
	}
	for _, e := range listHistory() {
		if e.ID == id {
			return e, filepath.Join(historyDir(), id), true
		}
	}
	return HistoryEntry{}, "", false
}

func serveHistory(api func(string, func(http.ResponseWriter, *http.Request))) {
	api("/api/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete { // Clear
			historyMu.Lock()
			os.RemoveAll(historyDir())
			historyMu.Unlock()
		}
		list := listHistory()
		if list == nil {
			list = []HistoryEntry{}
		}
		writeJSON(w, list)
	})
	api("/api/history/", func(w http.ResponseWriter, r *http.Request) {
		id, what, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/history/"), "/")
		e, dir, ok := historyEntry(id)
		if !ok {
			fail(w, 404, "that print isn't kept any more")
			return
		}
		switch {
		case what == "thumb":
			w.Header().Set("Cache-Control", "private, max-age=86400")
			http.ServeFile(w, r, filepath.Join(dir, "thumb.jpg"))
		case what == "file" && r.Method == http.MethodPost:
			// the kept PDF as a file in the app, with how it was printed, so it can be printed again
			f, err := addFile(filepath.Join(dir, "job.pdf"), e.Title+".pdf")
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			writeJSON(w, map[string]any{"file": f, "entry": e})
		case what == "" && r.Method == http.MethodDelete:
			historyMu.Lock()
			os.RemoveAll(dir)
			historyMu.Unlock()
			writeJSON(w, map[string]bool{"ok": true})
		default:
			fail(w, 400, "bad request")
		}
	})
}
