// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"log"
	"strings"
	"testing"
)

// a report for a bug report never carries addresses on the WiFi (this computer's, phones', the printer's)
func TestRedact(t *testing.T) {
	in := `server at http://192.168.0.100:8632 and http://10.1.2.3:8632/
printer ipp://192.168.0.156:631/ipp/print, phone fe80::1c2b:3aff:fe4d:5e6f and 2001:db8::5
tailscale 100.101.102.103, docker 172.17.0.1, localhost 127.0.0.1, version 0.21.0, CUPS 2.4.19, date 2026-09-27`
	out := redact(in)
	for _, leak := range []string{"192.168.0.100", "10.1.2.3", "192.168.0.156", "fe80::1c2b:3aff:fe4d:5e6f", "2001:db8::5", "100.101.102.103", "172.17.0.1"} {
		if strings.Contains(out, leak) {
			t.Errorf("%s is still in the report:\n%s", leak, out)
		}
	}
	for _, keep := range []string{"127.0.0.1", "0.21.0", "2.4.19", "2026-09-27", ":8632", "ipp://<address>:631/ipp/print"} {
		if !strings.Contains(out, keep) {
			t.Errorf("%q should stay (not an address on the WiFi):\n%s", keep, out)
		}
	}
}

// the server's recent log lines are kept for the Advanced page, the newest last, a few hundred at most
func TestLogRing(t *testing.T) {
	r := &logRing{max: 3}
	lg := log.New(r, "", 0)
	for _, s := range []string{"one", "two", "three", "four"} {
		lg.Print(s)
	}
	got := r.lines()
	if strings.Join(got, ",") != "two,three,four" {
		t.Errorf("got %v", got)
	}
}

// the report reads like sentences, not like Go's insides
func TestReportWording(t *testing.T) {
	d := debugInfo{Version: "1", Queues: []debugQueue{{Name: "P", Model: "M", Driver: "driver", State: "ready", Reasons: "none"}, {Name: "Q", Driver: "broken", State: "stopped", Reasons: "offline-report"}},
		Phones:   map[string]any{"access": map[string]any{"state": "off"}, "pins": 1},
		AirPrint: map[string]any{"switchedOn": true}}
	r := d.report()
	for _, want := range []string{"P: M, the maker's driver, ready", "Q: , its driver is missing, stopped (offline-report)", "access=off"} {
		if !strings.Contains(r, want) {
			t.Errorf("want %q in:\n%s", want, r)
		}
	}
	for _, bad := range []string{"driver driver", "map[", "ready none"} {
		if strings.Contains(r, bad) {
			t.Errorf("%q in the report:\n%s", bad, r)
		}
	}
}
