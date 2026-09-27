// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrintAgain(t *testing.T) {
	for _, tool := range []string{"qpdf", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("needs " + tool)
		}
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	src, err := addFile(filepath.Join("tests", "fixtures", "twopage.pdf"), "Maths homework.pdf")
	if err != nil {
		t.Fatal(err)
	}
	// what a print of 3 copies looks like: the two pages, three times
	combined := filepath.Join(t.TempDir(), "combined.pdf")
	if err := qpdf("--empty", "--pages", src.Path, "1-z", src.Path, "1-z", src.Path, "1-z", "--", combined); err != nil {
		t.Fatal(err)
	}
	b := &Built{Combined: combined, PerCopy: 2, Printer: "Brother", Spec: Spec{Copies: 3, Sides: "duplex", Mode: "docs", Items: []Item{{ID: src.ID}}, Options: map[string]string{"PageSize": "A4"}}}
	if err := remember(b); err != nil {
		t.Fatal(err)
	}
	list := listHistory()
	if len(list) != 1 {
		t.Fatalf("want 1 kept print, got %d", len(list))
	}
	e := list[0]
	if e.Title != "Maths homework" || e.Pages != 2 || e.Copies != 3 || e.Sides != "duplex" || e.Options["PageSize"] != "A4" {
		t.Fatalf("kept the wrong details: %+v", e)
	}

	mux := http.NewServeMux()
	serveHistory(func(p string, h func(http.ResponseWriter, *http.Request)) { mux.HandleFunc(p, h) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/history/"+e.ID+"/file", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		File  File         `json:"file"`
		Entry HistoryEntry `json:"entry"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.File.Kind != "pdf" || got.File.Pages != 2 {
		t.Fatalf("the kept print should come back as ONE copy, a 2-page PDF: %+v", got.File)
	}
	if r, _ := http.Get(srv.URL + "/api/history/" + e.ID + "/thumb"); r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumbnail: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	if r, _ := http.Get(srv.URL + "/api/history/../../etc/passwd/thumb"); r.StatusCode == 200 {
		t.Fatal("only kept prints can be read")
	}

	// only the newest 30 are kept
	for i := 0; i < 31; i++ {
		if err := remember(b); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(listHistory()); n != historyKeep {
		t.Fatalf("want the newest %d, have %d", historyKeep, n)
	}
	// remove one, then clear
	id := listHistory()[0].ID
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/history/"+id, nil)
	http.DefaultClient.Do(req)
	if n := len(listHistory()); n != historyKeep-1 {
		t.Fatalf("remove one: have %d", n)
	}
	req, _ = http.NewRequest("DELETE", srv.URL+"/api/history", nil)
	http.DefaultClient.Do(req)
	if n := len(listHistory()); n != 0 {
		t.Fatalf("clear: have %d", n)
	}
}
