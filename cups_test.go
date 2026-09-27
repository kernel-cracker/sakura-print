// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestParseLpstat(t *testing.T) {
	cases := []struct{ out, state, reasons string }{
		{"printer DCPT510W-Brother is idle.  enabled since Thu 24 Sep 2026 09:59:13 PM IST\n\tForm mounted:\n\tAlerts: none\n", "ready", ""},
		{"printer DCPT510W-Brother now printing DCPT510W-Brother-212.  enabled since Thu 24 Sep 2026\n\tAlerts: media-low-warning\n", "printing", "media-low-warning"},
		{"printer DCPT510W-Brother disabled since Thu 24 Sep 2026 -\n\tPaused\n\tAlerts: paused media-empty-error\n", "stopped", "paused,media-empty-error"},
		{"lpstat: Invalid destination name\n", "unknown", ""},
	}
	for _, c := range cases {
		s, r := parseLpstat(c.out)
		if s != c.state || r != c.reasons {
			t.Errorf("%q: got %q %q, want %q %q", c.out, s, r, c.state, c.reasons)
		}
	}
}

func TestCachedOnlyFirstCallWaits(t *testing.T) {
	var loads atomic.Int32
	c := &cached[int]{maxAge: 50 * time.Millisecond, load: func() int {
		time.Sleep(30 * time.Millisecond)
		return int(loads.Add(1))
	}}
	if v := c.get(); v != 1 {
		t.Fatalf("first get = %d", v)
	}
	time.Sleep(60 * time.Millisecond) // now stale
	start := time.Now()
	if v := c.get(); v != 1 {
		t.Fatalf("stale get should return the old value right away, got %d", v)
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Fatal("a stale get waited for the reload")
	}
	for i := 0; i < 5; i++ {
		c.get() // only one background reload at a time
	}
	time.Sleep(60 * time.Millisecond)
	if v := c.get(); v != 2 || loads.Load() != 2 {
		t.Fatalf("want exactly one background reload, value %d loads %d", v, loads.Load())
	}
}

func TestDuplexOption(t *testing.T) {
	o := func(key, def string, vals ...string) PPDOption {
		return PPDOption{Key: key, Values: vals, Default: def}
	}
	cases := []struct {
		name         string
		opts         []PPDOption
		key, on, off string
	}{
		{"brother inkjet without duplex", []PPDOption{o("PageSize", "A4", "A4", "Letter"), o("BRMonoColor", "Mono", "Color", "Mono")}, "", "", ""},
		{"classic PPD", []PPDOption{o("Duplex", "None", "None", "DuplexNoTumble", "DuplexTumble")}, "Duplex", "DuplexNoTumble", "None"},
		{"tumble listed first", []PPDOption{o("Duplex", "None", "DuplexTumble", "DuplexNoTumble", "None")}, "Duplex", "DuplexNoTumble", "None"},
		{"driverless / IPP Everywhere", []PPDOption{o("sides", "one-sided", "one-sided", "two-sided-long-edge", "two-sided-short-edge")}, "sides", "two-sided-long-edge", "one-sided"},
		{"only one-sided offered", []PPDOption{o("sides", "one-sided", "one-sided")}, "", "", ""},
		{"duplex unit not fitted", []PPDOption{o("OptionDuplexer", "False", "False", "True"), o("Duplex", "None", "None", "DuplexNoTumble")}, "", "", ""},
		{"duplex unit fitted", []PPDOption{o("OptionDuplexer", "True", "False", "True"), o("Duplex", "None", "None", "DuplexNoTumble")}, "Duplex", "DuplexNoTumble", "None"},
	}
	for _, c := range cases {
		k, on, off := duplexOption(c.opts)
		if k != c.key || on != c.on || off != c.off {
			t.Errorf("%s: got %q %q %q, want %q %q %q", c.name, k, on, off, c.key, c.on, c.off)
		}
	}
}

func TestScannerFor(t *testing.T) {
	scs := []Scanner{{Device: "airscan:e0:Brother DCP-T510W", Name: "eSCL Brother DCP-T510W"}, {Device: "escl:http://10.0.0.9", Name: "HP Envy 6000"}}
	if got := scannerFor("DCPT510W-Brother", "Brother DCP-T510W, using brlaser", scs); got != "eSCL Brother DCP-T510W" {
		t.Errorf("Brother: got %q", got)
	}
	if got := scannerFor("DCPT510W-Brother", "", scs); got != "eSCL Brother DCP-T510W" {
		t.Errorf("Brother by queue name only: got %q", got)
	}
	if got := scannerFor("HL-L2350DW", "Brother HL-L2350DW series", scs); got != "" {
		t.Errorf("print-only laser must not match someone else's scanner, got %q", got)
	}
	if got := scannerFor("anything", "Generic PDF", nil); got != "" {
		t.Errorf("no scanners: got %q", got)
	}
}

func TestParseScanners(t *testing.T) {
	out := "device `brother4:net1;dev0' is a Brother DCPT510W DCP-T510W\n" +
		"device `v4l:/dev/video0' is a Noname HP Wide Vision HD Camera: HP Wi virtual device\n" +
		"device `escl:http://192.168.0.156:80' is a Brother DCP-T510W adf scanner\n" +
		"device `airscan:e0:Brother DCP-T510W' is a eSCL Brother DCP-T510W ip=192.168.0.156\n" +
		"device `airscan:e1:HP Envy 6000' is a eSCL HP Envy 6000 ip=192.168.0.20\n"
	got := parseScanners(out)
	if len(got) != 2 {
		t.Fatalf("want the Brother once and the HP, no webcam: %+v", got)
	}
	if got[0].Device != "brother4:net1;dev0" || got[0].Name != "Brother DCP-T510W" {
		t.Errorf("Brother: %+v", got[0])
	}
	if got[1].Name != "HP Envy 6000" {
		t.Errorf("HP: %+v", got[1])
	}
	if len(parseScanners("")) != 0 {
		t.Error("no scanners")
	}
}
