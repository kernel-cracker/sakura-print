// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"errors"
	"log"
	"sync"
	"time"
)

// Print jobs: sent to CUPS, followed until done, with the manual flip in the middle for double-sided printing.

// ---------- print jobs (with the manual flip in the middle) ----------

type Job struct {
	mu      sync.Mutex
	ID      string    `json:"id"`
	Build   *Built    `json:"-"`
	State   string    `json:"state"` // printing1 | flip | printing2 | done | error
	CupsID  string    `json:"-"`
	Error   string    `json:"error,omitempty"`
	Sheets  int       `json:"sheets"`
	Rotate  bool      `json:"rotate"`
	Test    bool      `json:"test"`
	Source  string    `json:"source,omitempty"` // "airprint": sent from another app's Print button
	Title   string    `json:"title,omitempty"`
	Created time.Time `json:"-"`
}

// printBuilt sends a built job to the printer (side one, for double-sided by hand). Call with b.mu held.
// source: "" from the app, "airprint" from another app's Print button (the app shows those when they need a flip)
func printBuilt(b *Built, test bool, source string) (*Job, error) {
	if err := b.passes(); err != nil {
		return nil, err
	}
	cid, err := submit(b.Printer, "Sakura Print", b.Pass1, b.Options)
	if err != nil {
		return nil, err
	}
	j := &Job{ID: newID(), Build: b, State: "printing1", CupsID: cid, Sheets: b.Sheets, Rotate: b.Rotate, Test: test, Source: source, Created: time.Now()}
	if !test && !b.Spec.Preview && !b.Spec.Again {
		go remember(b) // for "Print again" (in the background: printing doesn't wait for it)
	}
	b.job = j
	stateMu.Lock()
	jobs[j.ID] = j
	stateMu.Unlock()
	log.Printf("Print %s: %d sheet(s) sent to %s (%s)%s", j.ID, j.Sheets, b.Printer, cid, map[bool]string{true: ", from another app", false: ""}[source == "airprint"])
	return j, nil
}

// view returns a safe copy of the job for sending to the app
type JobView struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
	Sheets int    `json:"sheets"`
	Rotate bool   `json:"rotate"`
	Test   bool   `json:"test"`
	Source string `json:"source,omitempty"`
	Title  string `json:"title,omitempty"`
}

func (j *Job) view() JobView {
	return JobView{ID: j.ID, State: j.State, Error: j.Error, Sheets: j.Sheets, Rotate: j.Rotate, Test: j.Test, Source: j.Source, Title: j.Title}
}

// refresh must be called with j.mu held
func (j *Job) refresh() {
	if (j.State == "printing1" || j.State == "printing2") && !jobActive(j.Build.Printer, j.CupsID) {
		if j.State == "printing1" && j.Build.Duplex {
			j.State = "flip"
		} else {
			j.State = "done"
		}
	}
}

// side2 sends the other side, after the paper was turned over. Call with j.mu held. Tapping twice is harmless.
func (j *Job) side2() error {
	j.refresh()
	if j.State == "printing2" || j.State == "done" {
		return nil
	}
	if j.State != "flip" {
		return errors.New("side 1 isn't finished yet")
	}
	cid, err := submit(j.Build.Printer, "Sakura Print (side 2)", j.Build.Pass2, j.Build.Options)
	if err != nil {
		return err
	}
	j.CupsID, j.State = cid, "printing2"
	log.Printf("Print %s: side 2 sent to %s (%s)", j.ID, j.Build.Printer, cid)
	return nil
}

var cancelHook func(cupsID string) // tests watch cancelling here instead of CUPS

// cancel stops this print, and only this one (others waiting for the printer carry on). Call with j.mu held.
func (j *Job) cancel() {
	if cancelHook != nil {
		cancelHook(j.CupsID)
	} else {
		cancelJob(j.CupsID)
	}
	j.State = "canceled"
	log.Printf("Print %s: stopped", j.ID)
}
