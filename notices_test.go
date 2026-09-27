// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func noticeIDs(ns []Notice) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func byID(ns []Notice, id string) *Notice {
	for i := range ns {
		if ns[i].ID == id {
			return &ns[i]
		}
	}
	return nil
}

// everything that needs the person, in one list, most urgent first
func TestNoticesCollect(t *testing.T) {
	now := time.Date(2026, 9, 27, 20, 0, 0, 0, time.Local)
	src := noticeSources{
		now:     now,
		flips:   []JobView{{ID: "j1", State: "flip", Title: "Homework", Source: "airprint"}},
		held:    []heldView{{ID: 4, Title: "Mystery", From: "Neighbour"}},
		printer: "Office",
		status: PrinterStatus{Name: "Office", State: "stopped", Reasons: "media-empty-error,marker-supply-low-warning",
			Markers: []Marker{{Name: "Black ink", Level: 4, Color: "#000000"}, {Name: "Cyan ink", Level: 60}}},
		updates: []drvUpdate{{Name: "Brother DCP-T510W", Package: "dcpt510wpdrv", Installed: "1.0.1-0", Available: "1.0.2-0"}},
		phones:  map[string]any{"state": "until", "until": now.Add(8 * time.Minute)},
		setupOK: true,
	}
	ns := collectNotices(src, true)
	want := []string{"flip:j1", "printer:Office:media-empty", "held:4", "printer:Office:ink:Black ink", "update:dcpt510wpdrv", "phones:ending"}
	if got := noticeIDs(ns); !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	flip := byID(ns, "flip:j1")
	if flip.Level != "action" || !strings.Contains(flip.Title, "Homework") || flip.Job != "j1" {
		t.Errorf("flip: %+v", flip)
	}
	paper := byID(ns, "printer:Office:media-empty")
	if paper.Level != "problem" || paper.Title != "Out of paper" || !strings.Contains(paper.Text, "paper") {
		t.Errorf("out of paper, in plain words with what to do: %+v", paper)
	}
	ink := byID(ns, "printer:Office:ink:Black ink")
	if ink.Level != "warning" || !strings.Contains(ink.Title, "Black ink") || !strings.Contains(ink.Title, "4%") {
		t.Errorf("low ink: %+v", ink)
	}
	if byID(ns, "printer:Office:marker-supply-low") != nil {
		t.Error("the printer's own low-ink reason is said once, by the ink level notice")
	}
	if up := byID(ns, "update:dcpt510wpdrv"); up.Go != "#/addprinter" || !strings.Contains(up.Text, "1.0.2-0") {
		t.Errorf("driver update: %+v", up)
	}
	if ph := byID(ns, "phones:ending"); !strings.Contains(ph.Text, "8 minutes") || ph.Action != "phones:1h" {
		t.Errorf("phone access ending soon, with a button to keep it on: %+v", ph)
	}
}

// phones see what concerns them; the computer sees everything
func TestNoticesForPhones(t *testing.T) {
	src := noticeSources{now: time.Now(), flips: []JobView{{ID: "j1", State: "flip", Title: "Homework"}},
		held: []heldView{{ID: 4, Title: "Mystery"}}, updates: []drvUpdate{{Package: "x", Available: "2"}},
		printer: "P", status: PrinterStatus{State: "stopped", Reasons: "media-jam-error"}, setupOK: false,
		phones: map[string]any{"state": "until", "until": time.Now().Add(5 * time.Minute)}}
	got := noticeIDs(collectNotices(src, false))
	for _, id := range []string{"flip:j1", "held:4", "printer:P:media-jam"} {
		if !slices.Contains(got, id) {
			t.Errorf("a phone should see %s: %v", id, got)
		}
	}
	for _, id := range []string{"update:x", "phones:ending", "setup"} {
		if slices.Contains(got, id) {
			t.Errorf("only the computer can act on %s: %v", id, got)
		}
	}
}

// the printer's own words, in plain words with what to do
func TestPrinterProblemsInPlainWords(t *testing.T) {
	for reason, title := range map[string]string{
		"media-empty-error": "Out of paper", "media-needed": "Out of paper", "media-jam-error": "Paper jam",
		"cover-open-error": "A cover is open", "door-open-report": "A cover is open", "marker-supply-empty-error": "Out of ink",
		"toner-empty-error": "Out of ink", "offline-report": "The printer can't be reached",
		"connecting-to-device": "The printer can't be reached", "input-tray-missing": "The paper tray is out",
	} {
		ns := collectNotices(noticeSources{now: time.Now(), printer: "P", status: PrinterStatus{State: "stopped", Reasons: reason}, setupOK: true}, true)
		if len(ns) != 1 || ns[0].Title != title || ns[0].Text == "" {
			t.Errorf("%s: %+v", reason, ns)
		}
	}
	// a printer that's fine, or only says something unimportant, says nothing
	for _, r := range []string{"none", "", "cups-waiting-for-job-completed", "com.apple.print.recoverable-warning"} {
		if ns := collectNotices(noticeSources{now: time.Now(), printer: "P", status: PrinterStatus{State: "ready", Reasons: r}, setupOK: true}, true); len(ns) != 0 {
			t.Errorf("%q: %+v", r, ns)
		}
	}
	// stopped, and nothing more said: it was paused
	ns := collectNotices(noticeSources{now: time.Now(), printer: "P", status: PrinterStatus{State: "stopped", Reasons: "paused"}, setupOK: true}, true)
	if len(ns) != 1 || ns[0].Title != "Printing is paused" {
		t.Errorf("paused: %+v", ns)
	}
}

// a notice put away stays away until something changes (the ink runs lower, another print needs flipping)
func TestNoticesDismiss(t *testing.T) {
	d := &dismissals{}
	src := noticeSources{now: time.Now(), printer: "P", status: PrinterStatus{Markers: []Marker{{Name: "Black ink", Level: 12}}}, setupOK: true}
	ns := collectNotices(src, true)
	if len(ns) != 1 {
		t.Fatalf("low ink: %v", noticeIDs(ns))
	}
	d.dismiss(ns[0])
	if got := d.filter(collectNotices(src, true)); len(got) != 0 {
		t.Errorf("put away: %v", noticeIDs(got))
	}
	src.status.Markers[0].Level = 11 // a percent lower: no nagging, it stays away
	if got := d.filter(collectNotices(src, true)); len(got) != 0 {
		t.Errorf("a small change shouldn't bring it back: %v", noticeIDs(got))
	}
	src.status.Markers[0].Level = 3 // much lower: it comes back
	if got := d.filter(collectNotices(src, true)); len(got) != 1 {
		t.Errorf("the ink ran lower: it should come back, got %v", noticeIDs(got))
	}
	flip := noticeSources{now: time.Now(), flips: []JobView{{ID: "j1", State: "flip", Title: "A"}}, setupOK: true}
	if got := d.filter(collectNotices(flip, true)); len(got) != 1 {
		t.Error("prints waiting for a flip can't be put away: the job needs finishing or cancelling")
	}
}
