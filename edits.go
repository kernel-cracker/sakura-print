// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Autosave for the photo editor. While you edit, the phone sends its edit "recipe" (crop, adjustments, filter,
// everything drawn, and the undo history) here every time something changes. Opening the same photo again
// picks up where you left off, and Undo still goes all the way back.
//
// Recipes are kept by a fingerprint of the photo's content (SHA-256), not by the app's temporary file ids, so
// they survive the server restarting and the same photo being added again. A copy of the original is kept
// next to its recipe, and the finished (flattened) picture is linked back to it: opening an edited picture
// again opens the original with all its edits still movable, instead of the flattened copy.
//
//	~/.local/share/sakuraprint/edits/<hash>.json       {"name", "state", "updated"}  or  {"link": "<hash>"}
//	~/.local/share/sakuraprint/edits/originals/<hash>   the untouched original photo
//	~/.local/share/sakuraprint/edits/pdf-<hash>.json   {"pages": {"3": "<recipe hash>"}}
//
// Pages inside a PDF: "Edit this page" turns the page into a picture, and the finished picture goes back into a
// new copy of the PDF. Opening that page again makes a new picture from the new PDF, with a new fingerprint, so
// each PDF also keeps a small map of which of its pages have a recipe. Every new copy of a PDF inherits its
// parent's map, so a page edited earlier keeps its original even after other pages are edited.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	editsKeep    = 30 * 24 * time.Hour
	maxEditState = 16 << 20 // an undo history with lots of drawing can be a few MB
)

var (
	hashRe   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	editsMu  sync.Mutex
	hashes   = map[string]string{} // file id -> content hash, worked out once
	hashesMu sync.Mutex
)

func editsDir() string { return filepath.Join(dataDir(), "edits") }

// contentHash fingerprints a file's bytes (cached per file id, files never change after they're added)
func contentHash(f *File) (string, error) {
	hashesMu.Lock()
	h, ok := hashes[f.ID]
	hashesMu.Unlock()
	if ok {
		return h, nil
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	s := sha256.New()
	if _, err := io.Copy(s, fh); err != nil {
		return "", err
	}
	h = hex.EncodeToString(s.Sum(nil))
	hashesMu.Lock()
	hashes[f.ID] = h
	hashesMu.Unlock()
	return h, nil
}

type editRecord struct {
	Link    string          `json:"link,omitempty"` // this picture is the finished version of that one
	Name    string          `json:"name,omitempty"`
	State   json.RawMessage `json:"state,omitempty"`
	Updated time.Time       `json:"updated"`
}

func readEdit(hash string) (*editRecord, error) {
	if !hashRe.MatchString(hash) {
		return nil, errors.New("bad key")
	}
	data, err := os.ReadFile(filepath.Join(editsDir(), hash+".json"))
	if err != nil {
		return nil, err
	}
	var r editRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func writeEdit(hash string, r editRecord) error {
	if !hashRe.MatchString(hash) {
		return errors.New("bad key")
	}
	r.Updated = time.Now()
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	os.MkdirAll(editsDir(), 0o700)
	p := filepath.Join(editsDir(), hash+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// keepOriginal stores a copy of the photo the recipe applies to, once
func keepOriginal(f *File, hash string) error {
	dir := filepath.Join(editsDir(), "originals")
	p := filepath.Join(dir, hash)
	if _, err := os.Stat(p); err == nil {
		now := time.Now()
		os.Chtimes(p, now, now) // still in use: don't let the cleanup take it
		return nil
	}
	os.MkdirAll(dir, 0o700)
	src, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := p + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	dst.Close()
	return os.Rename(tmp, p)
}

// ---------- pages of PDFs ----------

type pageRef struct {
	PDF  string // file id of the PDF
	Page int
}

var (
	pageOf   = map[string]pageRef{} // page picture file id -> where it came from
	pageOfMu sync.Mutex
)

// notePage remembers that a picture is page n of a PDF (called when "Edit this page" makes it)
func notePage(img, pdf *File, n int) {
	pageOfMu.Lock()
	pageOf[img.ID] = pageRef{pdf.ID, n}
	pageOfMu.Unlock()
}

type pdfPages struct {
	Pages   map[string]string `json:"pages"` // page number -> recipe hash
	Updated time.Time         `json:"updated"`
}

func pdfPagesPath(hash string) string { return filepath.Join(editsDir(), "pdf-"+hash+".json") }

func readPDFPages(hash string) map[string]string {
	if !hashRe.MatchString(hash) {
		return map[string]string{}
	}
	var pp pdfPages
	if data, err := os.ReadFile(pdfPagesPath(hash)); err == nil && json.Unmarshal(data, &pp) == nil && pp.Pages != nil {
		return pp.Pages
	}
	return map[string]string{}
}

func writePDFPages(hash string, pages map[string]string) error {
	if !hashRe.MatchString(hash) {
		return errors.New("bad key")
	}
	if len(pages) == 0 {
		os.Remove(pdfPagesPath(hash))
		return nil
	}
	data, _ := json.Marshal(pdfPages{Pages: pages, Updated: time.Now()})
	os.MkdirAll(editsDir(), 0o700)
	tmp := pdfPagesPath(hash) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, pdfPagesPath(hash))
}

// carryPages is called when an edited page goes back into a PDF: the new copy gets the old copy's page map,
// with this page now pointing at the recipe the edited picture was made from (or at nothing, if it has none)
func carryPages(oldPDF, newPDF *File, page int, result *File) {
	oh, err1 := contentHash(oldPDF)
	nh, err2 := contentHash(newPDF)
	if err1 != nil || err2 != nil || oh == nh {
		return
	}
	editsMu.Lock()
	defer editsMu.Unlock()
	pages := readPDFPages(oh)
	key := strconv.Itoa(page)
	delete(pages, key)
	if rh, err := contentHash(result); err == nil {
		if r, err := readEdit(rh); err == nil && r.Link != "" {
			pages[key] = r.Link
		}
	}
	writePDFPages(nh, pages)
}

// original registers the kept original a recipe applies to, so the editor can load it
func original(recipe, name string) (*File, *editRecord, error) {
	r, err := readEdit(recipe)
	if err != nil {
		return nil, nil, err
	}
	orig := filepath.Join(editsDir(), "originals", recipe)
	if _, err := os.Stat(orig); err != nil {
		return nil, nil, err // the original is gone (cleaned up): edit the picture as it is
	}
	if r.Name != "" {
		name = r.Name
	}
	src, err := addFile(orig, name)
	if err != nil {
		return nil, nil, err
	}
	hashesMu.Lock()
	hashes[src.ID] = recipe
	hashesMu.Unlock()
	return src, r, nil
}

// loadEdit finds the recipe for a picture: its own; for a finished picture, its original's; for a page made
// from a PDF, the recipe that page of that PDF was made from. It returns the file the recipe applies to,
// registered so the editor can load it.
func loadEdit(f *File) (*File, *editRecord, error) {
	h, err := contentHash(f)
	if err != nil {
		return nil, nil, err
	}
	editsMu.Lock()
	defer editsMu.Unlock()
	r, err := readEdit(h)
	switch {
	case err == nil && r.Link != "":
		return original(r.Link, f.Name)
	case err == nil:
		return f, r, nil
	}
	pageOfMu.Lock()
	ref, ok := pageOf[f.ID]
	pageOfMu.Unlock()
	if pdf, found := getFile(ref.PDF); ok && found {
		if ph, err := contentHash(pdf); err == nil {
			if recipe := readPDFPages(ph)[strconv.Itoa(ref.Page)]; recipe != "" {
				return original(recipe, f.Name)
			}
		}
	}
	return nil, nil, err
}

func serveEdits(api func(string, func(http.ResponseWriter, *http.Request))) {
	api("/api/edit/", func(w http.ResponseWriter, r *http.Request) {
		f, ok := getFile(strings.TrimPrefix(r.URL.Path, "/api/edit/"))
		if !ok || f.Kind != "image" {
			fail(w, 404, "no such picture")
			return
		}
		switch r.Method {
		case http.MethodGet:
			src, rec, err := loadEdit(f)
			if err != nil || len(rec.State) == 0 {
				writeJSON(w, map[string]any{"state": nil})
				return
			}
			writeJSON(w, map[string]any{"source": src, "state": rec.State, "updated": rec.Updated})
		case http.MethodPost:
			var req struct {
				State  json.RawMessage `json:"state"`
				Result string          `json:"result"` // the finished picture made from this recipe
			}
			defer r.Body.Close()
			err := json.NewDecoder(io.LimitReader(r.Body, maxEditState)).Decode(&req)
			if err != nil || len(req.State) == 0 || string(req.State) == "null" { // never overwrite a recipe with nothing
				fail(w, 400, "bad request")
				return
			}
			h, err := contentHash(f)
			if err == nil {
				err = keepOriginal(f, h)
			}
			if err != nil {
				fail(w, 500, "couldn't keep the edit: "+err.Error())
				return
			}
			editsMu.Lock()
			err = writeEdit(h, editRecord{Name: f.Name, State: req.State})
			if err == nil && req.Result != "" {
				if rf, ok := getFile(req.Result); ok {
					if rh, e := contentHash(rf); e == nil && rh != h {
						err = writeEdit(rh, editRecord{Link: h})
					}
				}
			}
			editsMu.Unlock()
			if err != nil {
				fail(w, 500, "couldn't keep the edit: "+err.Error())
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		case http.MethodDelete: // start over: forget the recipe for this picture
			if h, err := contentHash(f); err == nil {
				editsMu.Lock()
				os.Remove(filepath.Join(editsDir(), h+".json"))
				editsMu.Unlock()
			}
			writeJSON(w, map[string]bool{"ok": true})
		default:
			fail(w, 405, "GET, POST or DELETE")
		}
	})
}

// cleanEdits forgets recipes and originals nobody has touched for a month
func cleanEdits() {
	cleanOld(filepath.Join(editsDir(), "originals"), editsKeep)
	entries, _ := os.ReadDir(editsDir())
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && time.Since(info.ModTime()) > editsKeep {
			os.Remove(filepath.Join(editsDir(), e.Name()))
		}
	}
}
