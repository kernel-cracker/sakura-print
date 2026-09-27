// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func editServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	mux := http.NewServeMux()
	serveEdits(func(path string, h func(http.ResponseWriter, *http.Request)) { mux.HandleFunc(path, h) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// testPicture adds a small PNG to the app, like an upload would
func testPicture(t *testing.T, shade uint8) *File {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(0, 0, color.RGBA{shade, 1, 2, 255})
	p := filepath.Join(t.TempDir(), "photo.png")
	fh, _ := os.Create(p)
	png.Encode(fh, img)
	fh.Close()
	f, err := addFile(p, "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func call(t *testing.T, srv *httptest.Server, method, id string, body any) map[string]any {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, srv.URL+"/api/edit/"+id, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	m["_status"] = float64(resp.StatusCode)
	return m
}

func TestEditAutosaveRoundTrip(t *testing.T) {
	srv := editServer(t)
	f := testPicture(t, 100)
	if got := call(t, srv, "GET", f.ID, nil); got["state"] != nil {
		t.Fatalf("a new picture has no autosave: %v", got)
	}
	state := map[string]any{"hist": []string{"a", "b"}, "at": 1}
	if got := call(t, srv, "POST", f.ID, map[string]any{"state": state}); got["ok"] != true {
		t.Fatalf("autosave failed: %v", got)
	}
	got := call(t, srv, "GET", f.ID, nil)
	st, _ := got["state"].(map[string]any)
	if st == nil || st["at"] != float64(1) {
		t.Fatalf("autosave didn't come back: %v", got)
	}
	if src, _ := got["source"].(map[string]any); src == nil || src["id"] != f.ID {
		t.Fatalf("the recipe should apply to the same picture: %v", got["source"])
	}
	// the same photo added again (new id, same bytes) finds the same autosave
	again, err := addFile(f.Path, "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := call(t, srv, "GET", again.ID, nil)["state"].(map[string]any); st == nil {
		t.Fatal("the same photo added again should find its autosave")
	}
	// start over
	call(t, srv, "DELETE", f.ID, nil)
	if got := call(t, srv, "GET", f.ID, nil); got["state"] != nil {
		t.Fatalf("after starting over there's no autosave: %v", got)
	}
}

func TestFinishedPictureOpensTheOriginal(t *testing.T) {
	srv := editServer(t)
	orig := testPicture(t, 50)
	done := testPicture(t, 200) // the flattened result, different bytes
	call(t, srv, "POST", orig.ID, map[string]any{"state": map[string]any{"at": 3}, "result": done.ID})
	got := call(t, srv, "GET", done.ID, nil)
	src, _ := got["source"].(map[string]any)
	if src == nil || src["id"] == done.ID {
		t.Fatalf("opening the finished picture should open the original: %v", got)
	}
	// and that original has the same content as the one first edited
	of, _ := getFile(src["id"].(string))
	a, _ := os.ReadFile(orig.Path)
	b, _ := os.ReadFile(of.Path)
	if !bytes.Equal(a, b) || of.Kind != "image" {
		t.Fatal("the kept original isn't the original photo")
	}
	if st, _ := got["state"].(map[string]any); st == nil || st["at"] != float64(3) {
		t.Fatalf("the recipe should come with it: %v", got["state"])
	}
	// the original is kept even after the upload is cleaned away
	os.Remove(orig.Path)
	if got := call(t, srv, "GET", done.ID, nil); got["source"] == nil {
		t.Fatal("the kept copy should still work after the upload is gone")
	}
}

func TestEditRequestsAreChecked(t *testing.T) {
	srv := editServer(t)
	f := testPicture(t, 10)
	if got := call(t, srv, "GET", "nope", nil); got["_status"] != float64(404) {
		t.Fatalf("unknown picture: %v", got)
	}
	if got := call(t, srv, "POST", f.ID, map[string]any{"state": nil}); got["_status"] != float64(400) {
		t.Fatalf("empty state must be refused: %v", got)
	}
	big := strings.Repeat("x", maxEditState+10)
	if got := call(t, srv, "POST", f.ID, map[string]any{"state": big}); got["_status"] != float64(400) {
		t.Fatalf("oversized state must be refused: %v", got["_status"])
	}
	if _, err := readEdit("../../etc/passwd"); err == nil {
		t.Fatal("keys must be hashes, nothing else")
	}
}

// fakePDF registers a "PDF" file for the page-map tests (only its bytes matter here: they're its fingerprint)
func fakePDF(t *testing.T, content string) *File {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.pdf")
	os.WriteFile(p, []byte("%PDF-fake "+content), 0o644)
	f := &File{ID: newID(), Name: "doc.pdf", Kind: "pdf", Pages: 4, Path: p}
	filesMu.Lock()
	files[f.ID] = f
	filesMu.Unlock()
	return f
}

func TestPDFPagesKeepTheirOriginal(t *testing.T) {
	srv := editServer(t)
	x := fakePDF(t, "first")
	page3 := testPicture(t, 30) // "Edit this page" on page 3 of x
	notePage(page3, x, 3)
	done3 := testPicture(t, 31) // the finished page 3
	call(t, srv, "POST", page3.ID, map[string]any{"state": map[string]any{"at": 7}, "result": done3.ID})
	y := fakePDF(t, "second") // x with the finished page 3 put back
	carryPages(x, y, 3, done3)

	// open page 3 of y: a fresh picture of the finished page, with different bytes
	again := testPicture(t, 32)
	notePage(again, y, 3)
	got := call(t, srv, "GET", again.ID, nil)
	src, _ := got["source"].(map[string]any)
	if src == nil {
		t.Fatalf("page 3 of the new PDF should open its original: %v", got)
	}
	of, _ := getFile(src["id"].(string))
	a, _ := os.ReadFile(page3.Path)
	b, _ := os.ReadFile(of.Path)
	if !bytes.Equal(a, b) {
		t.Fatal("that should be the original page 3")
	}
	if st, _ := got["state"].(map[string]any); st == nil || st["at"] != float64(7) {
		t.Fatalf("with its recipe: %v", got["state"])
	}

	// now edit page 1 of y; in the next copy z, page 3 must still open its original
	page1 := testPicture(t, 40)
	notePage(page1, y, 1)
	done1 := testPicture(t, 41)
	call(t, srv, "POST", page1.ID, map[string]any{"state": map[string]any{"at": 2}, "result": done1.ID})
	z := fakePDF(t, "third")
	carryPages(y, z, 1, done1)
	for _, c := range []struct {
		page int
		at   float64
	}{{3, 7}, {1, 2}} {
		pic := testPicture(t, uint8(50+c.page))
		notePage(pic, z, c.page)
		st, _ := call(t, srv, "GET", pic.ID, nil)["state"].(map[string]any)
		if st == nil || st["at"] != c.at {
			t.Fatalf("page %d of the third copy: got %v", c.page, st)
		}
	}
	// a page never edited opens as it is
	plain := testPicture(t, 60)
	notePage(plain, z, 2)
	if got := call(t, srv, "GET", plain.ID, nil); got["state"] != nil {
		t.Fatalf("page 2 was never edited: %v", got)
	}
}
