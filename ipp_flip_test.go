// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// a double-sided print from a phone, side 1 done: the phone's print queue says what to do
func TestFlipShowsInThePhonesQueue(t *testing.T) {
	printing := false
	jobActiveHook = func(string, string) bool { return printing }
	defer func() { jobActiveHook = nil }()
	s := &ippServer{jobs: map[int]*ippJob{}}
	job := &Job{ID: "j1", State: "flip", Build: &Built{Printer: "p"}}
	ij := &ippJob{id: 1, name: "Homework", job: job, state: 5}
	s.jobs[1] = ij
	s.refresh(ij)
	if ij.state != 6 || ij.reason != "printer-stopped" || !strings.Contains(ij.message, "Turn the printed pages over") {
		t.Errorf("waiting for the flip: state %d %q %q", ij.state, ij.reason, ij.message)
	}
	state, reason, msg := s.printerState(true)
	if state != 3 || reason != "media-needed" || !strings.Contains(msg, "Homework") {
		t.Errorf("the printer asks for the paper to be turned over: %d %q %q", state, reason, msg)
	}
	job.State, job.CupsID, printing = "printing2", "p-2", true // the other side is printing
	s.refresh(ij)
	if ij.state != 5 || ij.reason != "job-printing" || ij.message != "" {
		t.Errorf("printing the other side: %d %q %q", ij.state, ij.reason, ij.message)
	}
	printing = false // and it's done
	s.refresh(ij)
	if ij.state != 9 || ij.reason != "job-completed-successfully" || ij.message != "" {
		t.Errorf("done: %d %q %q", ij.state, ij.reason, ij.message)
	}
	if state, reason, _ := s.printerState(true); state != 3 || reason != "none" {
		t.Errorf("nothing waiting: %d %q", state, reason)
	}
}

// the computer's notification has a "Print the other side" button that does it
func TestFlipNotificationButton(t *testing.T) {
	log.SetOutput(serverLog)
	defer log.SetOutput(os.Stderr)
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	// a stand-in notify-send: remembers what it was asked, and "clicks" the first button it's given
	os.WriteFile(filepath.Join(bin, "notify-send"), []byte(`#!/bin/sh
echo "$@" >> `+calls+`
case "$*" in --help*) echo "  -A, --action=[NAME=]Text"; exit 0;; esac
for a in "$@"; do case $a in side2=*) echo side2; exit 0;; esac; done
`), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	old := dryPrint
	dryPrint = t.TempDir()
	defer func() { dryPrint = old }()
	oldPoll := flipPoll
	flipPoll = 10 * time.Millisecond
	defer func() { flipPoll = oldPoll }()
	notifyActions = -1 // ask the stand-in again

	pass2 := filepath.Join(t.TempDir(), "pass2.pdf")
	os.WriteFile(pass2, []byte("%PDF-1.4 side two"), 0o644)
	job := &Job{ID: "j1", State: "flip", Build: &Built{Printer: "p", Pass2: pass2}}
	remindFlip(job, "Homework")
	job.mu.Lock()
	st := job.State
	job.mu.Unlock()
	if st != "printing2" {
		t.Fatalf("clicking the button prints the other side: state %s; notify-send was asked:\n%s", st, readFile(calls))
	}
	got := readFile(calls)
	if !strings.Contains(got, "side2=Print the other side") || !strings.Contains(got, "Homework") {
		t.Errorf("the notification: %s", got)
	}
	if entries, _ := os.ReadDir(dryPrint); len(entries) != 1 {
		t.Errorf("side 2 went to the printer: %d files", len(entries))
	}
	// and the server's log (Advanced) says what happened
	found := false
	for _, l := range serverLog.lines() {
		if strings.Contains(l, "side 2") && strings.Contains(l, "p") {
			found = true
		}
	}
	if !found {
		t.Errorf("the log should say side 2 was sent: %v", serverLog.lines())
	}
}

// "Stop printing" stops this print, not everything else waiting for the printer
func TestCancelIsJustThisJob(t *testing.T) {
	job := &Job{ID: "j1", State: "printing1", CupsID: "Brother-12", Build: &Built{Printer: "p"}}
	var cancelled []string
	cancelHook = func(id string) { cancelled = append(cancelled, id) }
	defer func() { cancelHook = nil }()
	job.mu.Lock()
	job.cancel()
	job.mu.Unlock()
	if job.State != "canceled" || len(cancelled) != 1 || cancelled[0] != "Brother-12" {
		t.Errorf("state %s, cancelled %v", job.State, cancelled)
	}
}
