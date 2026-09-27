// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNeighbourTables(t *testing.T) {
	arp := `IP address       HW type     Flags       HW address            Mask     Device
192.168.0.1      0x1         0x2         bc:0f:9a:cf:f5:5f     *        wlo1
192.168.0.57     0x1         0x2         AA:BB:CC:DD:EE:01     *        wlo1
192.168.0.58     0x1         0x0         00:00:00:00:00:00     *        wlo1
`
	for ip, want := range map[string]string{"192.168.0.57": "aa:bb:cc:dd:ee:01", "192.168.0.1": "bc:0f:9a:cf:f5:5f", "192.168.0.58": "", "192.168.0.9": ""} {
		if got := arpLookup(strings.NewReader(arp), ip); got != want {
			t.Errorf("arp %s: got %q want %q", ip, got, want)
		}
	}
	for out, want := range map[string]string{
		"fe80::1c2b:3aff:fe4d:5e6f dev wlo1 lladdr 1e:2b:3a:4d:5e:6f STALE\n": "1e:2b:3a:4d:5e:6f",
		"2001:db8::5 dev wlo1 lladdr aa:bb:cc:dd:ee:02 router REACHABLE\n":    "aa:bb:cc:dd:ee:02",
		"192.168.0.77 dev wlo1 INCOMPLETE\n":                                  "",
		"192.168.0.78 dev wlo1 lladdr aa:bb:cc:dd:ee:03 FAILED\n":             "",
		"": "",
	} {
		if got := neighLookup(out); got != want {
			t.Errorf("neigh %q: got %q want %q", out, got, want)
		}
	}
}

func TestWhoMayPrint(t *testing.T) {
	testMACs = true
	defer func() { testMACs = false }()
	settingsMu.Lock()
	saved := settings
	settings = Settings{RequirePIN: true, PINs: []PinEntry{{ID: "mom", Label: "Mom"}}}
	settingsMu.Unlock()
	defer func() { settingsMu.Lock(); settings = saved; settingsMu.Unlock() }()

	phone := func(mac string) (bool, string) {
		r := httptest.NewRequest("POST", "http://192.168.0.100:8632/ipp/print", nil)
		r.RemoteAddr = "192.168.0.57:50000"
		if mac != "" {
			r.Header.Set("X-Test-Mac", mac)
		}
		return mayPrint(r)
	}
	if ok, _ := phone("aa:bb:cc:dd:ee:01"); ok {
		t.Fatal("a phone that never logged in must wait")
	}
	settingsMu.Lock()
	rememberDevice("aa:bb:cc:dd:ee:01", "mom", "Mom")
	settingsMu.Unlock()
	if ok, _ := phone("aa:bb:cc:dd:ee:01"); !ok {
		t.Fatal("after logging in with a PIN, it prints")
	}
	if ok, _ := phone("aa:bb:cc:dd:ee:02"); ok {
		t.Fatal("another phone still waits")
	}
	if ok, _ := phone(""); ok {
		t.Fatal("a phone whose address can't be found waits")
	}
	local := httptest.NewRequest("POST", "http://localhost:8632/ipp/print", nil)
	local.RemoteAddr = "127.0.0.1:50000"
	if ok, _ := mayPrint(local); !ok {
		t.Fatal("the computer itself always prints")
	}
	rebound := httptest.NewRequest("POST", "http://evil.example:8632/ipp/print", nil)
	rebound.RemoteAddr = "127.0.0.1:50000"
	if ok, _ := mayPrint(rebound); ok {
		t.Fatal("a web page reaching in through a look-alike name (DNS rebinding) must not count as the computer")
	}

	settingsMu.Lock() // removing Mom's PIN forgets her phone
	settings.PINs = nil
	forgetDevices()
	n := len(settings.Devices)
	settingsMu.Unlock()
	if ok, _ := phone("aa:bb:cc:dd:ee:01"); ok || n != 0 {
		t.Fatal("a removed PIN's phone must wait again")
	}

	settingsMu.Lock() // allowed by hand: no PIN, stays; but not forever
	rememberDevice("aa:bb:cc:dd:ee:03", "", "Guest")
	forgetDevices()
	settingsMu.Unlock()
	if ok, _ := phone("aa:bb:cc:dd:ee:03"); !ok {
		t.Fatal("a phone allowed by hand prints")
	}
	settingsMu.Lock()
	settings.Devices[0].Seen = time.Now().Add(-deviceLife - time.Hour)
	settingsMu.Unlock()
	if ok, _ := phone("aa:bb:cc:dd:ee:03"); ok {
		t.Fatal("a phone not seen for months waits again")
	}

	settingsMu.Lock() // PINs off: the whole app is open, so is printing
	settings.RequirePIN = false
	settingsMu.Unlock()
	if ok, _ := phone(""); !ok {
		t.Fatal("with PINs off, any phone prints")
	}
}
