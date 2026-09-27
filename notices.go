// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// The notifications centre: everything that needs the person, in one list, most urgent first (see
// docs/design/notices.md). Prints waiting for the paper to be turned over, the printer's problems in plain words
// with what to do, prints from unknown phones, low ink, newer drivers, phone access about to switch off, setup not
// finished. The web app shows them behind the bell, with a count; the computer's desktop gets a notification for
// the urgent ones (see remindFlip).

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Notice struct {
	ID          string `json:"id"`
	Level       string `json:"level"` // action (needs you now) | problem (the printer can't print) | warning | info
	Title       string `json:"title"`
	Text        string `json:"text"`
	Go          string `json:"go,omitempty"`          // a screen that deals with it
	Job         string `json:"job,omitempty"`         // a print waiting for the flip
	Held        int    `json:"held,omitempty"`        // a print from an unknown phone
	Action      string `json:"action,omitempty"`      // a one-tap fix
	ActionLabel string `json:"actionLabel,omitempty"` // its button
	key         string // the notice's state: put away, it stays away until this changes
	order       int
}

type heldView struct {
	ID          int
	Title, From string
}

// noticeSources: what the notices are made from (filled in from the live state, or by tests)
type noticeSources struct {
	now     time.Time
	flips   []JobView
	held    []heldView
	printer string
	status  PrinterStatus
	updates []drvUpdate
	phones  map[string]any
	setupOK bool
}

// the printer's own reasons (RFC 8011 printer-state-reasons, without -error/-warning/-report), in plain words
var printerProblems = []struct {
	reasons     []string
	title, text string
}{
	{[]string{"media-empty", "media-needed"}, "Out of paper", "Put paper in, and printing carries on by itself."},
	{[]string{"media-jam"}, "Paper jam", "Open the printer's cover and gently pull the stuck paper out, then close it."},
	{[]string{"cover-open", "door-open", "interlock-open"}, "A cover is open", "Close the printer's covers, and printing carries on."},
	{[]string{"marker-supply-empty", "toner-empty"}, "Out of ink", "Refill or replace the ink. Printer care shows which colour."},
	{[]string{"input-tray-missing"}, "The paper tray is out", "Push the paper tray back in."},
	{[]string{"offline", "connecting-to-device", "timed-out", "shutdown"}, "The printer can't be reached", "Is it switched on, and on the same WiFi as this computer? Printing carries on once it's back."},
	{[]string{"paused"}, "Printing is paused", "Printing was stopped on this computer. Printer care can start it again."},
}

var noticeOrder = map[string]int{"flip": 0, "setup": 1, "problem": 2, "held": 3, "ink": 4, "update": 5, "phones": 6}

func collectNotices(src noticeSources, local bool) []Notice {
	var ns []Notice
	add := func(kind string, n Notice) {
		n.order = noticeOrder[kind]
		if n.key == "" {
			n.key = n.ID
		}
		ns = append(ns, n)
	}
	for _, j := range src.flips {
		if j.State != "flip" {
			continue
		}
		add("flip", Notice{ID: "flip:" + j.ID, Level: "action", Job: j.ID, Title: "Turn the paper over: “" + firstNonEmpty(j.Title, "your print") + "”",
			Text: "Side one is done. Take the pages out, turn them over as shown, put them back, then print the other side."})
	}
	if local && !src.setupOK {
		add("setup", Notice{ID: "setup", Level: "action", Go: "#/setup", Title: "Finish setting up",
			Text: "A few questions and your printer is ready: which one it is, how its paper goes in, and phones."})
	}
	if src.printer != "" {
		inkSaid := false
		for _, m := range src.status.Markers {
			if m.Level >= 0 && m.Level <= 15 {
				inkSaid = true
			}
		}
		said := map[string]bool{}
		for _, r := range strings.Split(src.status.Reasons, ",") {
			r = strings.TrimSpace(r)
			for _, sfx := range []string{"-error", "-warning", "-report"} {
				r = strings.TrimSuffix(r, sfx)
			}
			if r == "" || r == "none" {
				continue
			}
			for _, p := range printerProblems {
				if !said[p.title] && contains(p.reasons, r) {
					said[p.title] = true
					add("problem", Notice{ID: "printer:" + src.printer + ":" + p.reasons[0], Level: "problem", Title: p.title, Text: p.text, Go: "#/printer"})
				}
			}
			if (r == "marker-supply-low" || r == "toner-low") && !inkSaid && !said["low"] {
				said["low"] = true
				add("ink", Notice{ID: "printer:" + src.printer + ":ink-low", Level: "warning", Title: "Ink is running low", Text: "Have a refill ready.", Go: "#/printer"})
			}
		}
		if src.status.State == "stopped" && len(said) == 0 {
			p := printerProblems[len(printerProblems)-1]
			add("problem", Notice{ID: "printer:" + src.printer + ":paused", Level: "problem", Title: p.title, Text: p.text, Go: "#/printer"})
		}
		for _, m := range src.status.Markers {
			if m.Level < 0 || m.Level > 15 {
				continue
			}
			title := fmt.Sprintf("%s is low (%d%%)", m.Name, m.Level)
			if m.Level == 0 {
				title = m.Name + " is empty"
			}
			add("ink", Notice{ID: "printer:" + src.printer + ":ink:" + m.Name, Level: "warning", Title: title, Text: "Have a refill ready.", Go: "#/printer",
				key: fmt.Sprintf("ink:%s:%d", m.Name, m.Level/5)})
		}
	}
	for _, h := range src.held {
		from := ""
		if h.From != "" {
			from = " (" + h.From + ")"
		}
		add("held", Notice{ID: fmt.Sprint("held:", h.ID), Level: "action", Held: h.ID, Go: "#/", Title: "A print is waiting: “" + h.Title + "”",
			Text: "From a phone that hasn't logged in to Sakura Print" + from + ". Print it only if you know who sent it."})
	}
	if local {
		for _, u := range src.updates {
			add("update", Notice{ID: "update:" + u.Package, Level: "info", Go: "#/addprinter", key: "update:" + u.Package + ":" + u.Available,
				Title: "A newer driver for " + firstNonEmpty(u.Name, "your printer"),
				Text:  fmt.Sprintf("%s %s → %s, from its maker. Update it in Printers & drivers.", u.Package, u.Installed, u.Available)})
		}
		if src.phones["state"] == "until" {
			if until, ok := src.phones["until"].(time.Time); ok && until.Sub(src.now) <= 15*time.Minute && until.After(src.now) {
				mins := int(until.Sub(src.now).Round(time.Minute).Minutes())
				add("phones", Notice{ID: "phones:ending", Level: "info", Action: "phones:1h", ActionLabel: "Keep on for an hour", key: "phones:" + until.Format(time.RFC3339),
					Title: "Phone access turns off soon", Text: fmt.Sprintf("At %s, in %d minutes.", until.Format("3:04 PM"), mins)})
			}
		}
	}
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].order < ns[j].order })
	return ns
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// dismissals: notices put away. Prints waiting for someone can't be put away (they need finishing or deciding).
type dismissals struct {
	mu   sync.Mutex
	gone map[string]string // id → the state it was put away in
}

func (d *dismissals) dismiss(n Notice) {
	if n.Level == "action" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gone == nil {
		d.gone = map[string]string{}
	}
	d.gone[n.ID] = n.key
}

func (d *dismissals) filter(ns []Notice) []Notice {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []Notice
	for _, n := range ns {
		if k, ok := d.gone[n.ID]; ok && k == n.key && n.Level != "action" {
			continue
		}
		out = append(out, n)
	}
	return out
}

var putAway = &dismissals{}

// driver updates are asked about once a day, in the background (they come from the makers' servers)
var updatesCache = &cached[[]drvUpdate]{maxAge: 24 * time.Hour, load: driverUpdates}

// liveSources: the notices' sources, now
func liveSources(local bool) noticeSources {
	src := noticeSources{now: time.Now()}
	stateMu.Lock()
	var all []*Job
	for _, j := range jobs {
		all = append(all, j)
	}
	stateMu.Unlock()
	for _, j := range all {
		j.mu.Lock()
		j.refresh()
		if j.State == "flip" {
			src.flips = append(src.flips, j.view())
		}
		j.mu.Unlock()
	}
	if ippSrv != nil {
		ippSrv.mu.Lock()
		for _, h := range ippSrv.waiting() {
			src.held = append(src.held, heldView{ID: h.id, Title: h.name, From: h.user})
		}
		ippSrv.mu.Unlock()
	}
	if p := ippPrinter(); p != "" {
		src.printer, src.status = p, printerStatus(p)
	}
	settingsMu.Lock()
	src.phones = phonesState()
	src.setupOK = src.printer != "" && settings.Setup[src.printer] >= setupVersion
	settingsMu.Unlock()
	if local {
		if ups, known := updatesCache.peek(); known {
			src.updates = ups
		} else {
			go updatesCache.get()
		}
	}
	return src
}

func serveNotices(api func(string, func(http.ResponseWriter, *http.Request))) {
	api("/api/notices", func(w http.ResponseWriter, r *http.Request) {
		local := isLocal(r)
		if r.Method == http.MethodPost {
			var req struct{ ID, Action string }
			if readJSON(r, &req) != nil {
				fail(w, 400, "bad request")
				return
			}
			switch req.Action {
			case "dismiss":
				for _, n := range collectNotices(liveSources(local), local) {
					if n.ID == req.ID {
						putAway.dismiss(n)
					}
				}
			case "phones:1h":
				if !local {
					fail(w, 403, "Only the computer can change phone access")
					return
				}
				settingsMu.Lock()
				settings.PhonesOff, settings.PhonesTill = true, time.Now().Add(time.Hour)
				saveSettings()
				settingsMu.Unlock()
			default:
				fail(w, 400, "unknown action")
				return
			}
		}
		ns := putAway.filter(collectNotices(liveSources(local), local))
		if ns == nil {
			ns = []Notice{}
		}
		writeJSON(w, ns)
	})
}
