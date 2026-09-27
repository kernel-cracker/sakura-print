// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Which phones may print from any app. AirPrint has no way to ask for a PIN, so a phone earns it by logging in
// to Sakura Print with a PIN once: from then on the computer knows that phone by its WiFi hardware address (MAC,
// read from the computer's neighbour table, the same for its IPv4 and IPv6 addresses and when the router gives it
// a new address). Prints from phones it doesn't know wait until someone allows them.
//
// A MAC can be copied by someone set on it (it isn't secret), so this keeps out neighbours, not a determined
// attacker; the worst they could do is print pages.

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const deviceLife = 90 * 24 * time.Hour // a phone not seen for this long has to log in again

// Device is a phone that may print from any app
type Device struct {
	MAC   string    `json:"mac"`
	PinID string    `json:"pinId,omitempty"` // the PIN it logged in with ("" = allowed by hand)
	Label string    `json:"label"`
	Seen  time.Time `json:"seen"`
}

var macRe = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

// peerIP is the address a request came from
func peerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// testMACs: the browser tests pretend to be phones with a header, since they all come from this computer.
// Only when the test runner starts the server with SAKURA_TEST_FAKE_MAC set; never in normal use.
var testMACs = os.Getenv("SAKURA_TEST_FAKE_MAC") != ""

// macOf finds the hardware address of the phone a request came from ("" = not on this WiFi, or unknown)
func macOf(r *http.Request) string {
	if testMACs {
		if m := strings.ToLower(r.Header.Get("X-Test-Mac")); macRe.MatchString(m) {
			return m
		}
		return ""
	}
	ip := peerIP(r)
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	if ip.To4() != nil {
		if f, err := os.Open("/proc/net/arp"); err == nil {
			m := arpLookup(f, ip.String())
			f.Close()
			if m != "" {
				return m
			}
		}
	}
	out, err := exec.Command("ip", "neigh", "show", "to", ip.String()).Output()
	if err != nil {
		return ""
	}
	return neighLookup(string(out))
}

// arpLookup reads /proc/net/arp: "IP  HW-type  Flags  HW-address  Mask  Device"
func arpLookup(f io.Reader, ip string) string {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c := strings.Fields(sc.Text())
		if len(c) >= 4 && c[0] == ip && c[2] != "0x0" {
			if m := strings.ToLower(c[3]); macRe.MatchString(m) && m != "00:00:00:00:00:00" {
				return m
			}
		}
	}
	return ""
}

// neighLookup reads `ip neigh`: "fe80::1 dev wlo1 lladdr aa:bb:cc:dd:ee:ff REACHABLE"
func neighLookup(out string) string {
	for _, l := range strings.Split(out, "\n") {
		c := strings.Fields(l)
		for i := 0; i+1 < len(c); i++ {
			if c[i] == "lladdr" && !strings.Contains(l, "FAILED") && !strings.Contains(l, "INCOMPLETE") {
				if m := strings.ToLower(c[i+1]); macRe.MatchString(m) {
					return m
				}
			}
		}
	}
	return ""
}

// noted: when each phone was last written down, so browsing the app doesn't rewrite the settings every tap
var (
	notedMu sync.Mutex
	noted   = map[string]time.Time{}
)

// noteDevice remembers a logged-in phone (called for requests with a valid login)
func noteDevice(r *http.Request, pinID string) {
	mac := macOf(r)
	if mac == "" {
		return
	}
	notedMu.Lock()
	if time.Since(noted[mac+pinID]) < 10*time.Minute {
		notedMu.Unlock()
		return
	}
	noted[mac+pinID] = time.Now()
	notedMu.Unlock()
	settingsMu.Lock()
	defer settingsMu.Unlock()
	label := "Phone"
	for _, p := range settings.PINs {
		if p.ID == pinID {
			label = p.Label
		}
	}
	rememberDevice(mac, pinID, label)
	saveSettings()
}

// rememberDevice adds or refreshes a phone. Call with settingsMu held.
func rememberDevice(mac, pinID, label string) {
	for i, d := range settings.Devices {
		if d.MAC == mac {
			settings.Devices[i].Seen = time.Now()
			if pinID != "" {
				settings.Devices[i].PinID, settings.Devices[i].Label = pinID, label
			}
			return
		}
	}
	settings.Devices = append(settings.Devices, Device{MAC: mac, PinID: pinID, Label: label, Seen: time.Now()})
	if n := len(settings.Devices); n > 50 {
		settings.Devices = settings.Devices[n-50:]
	}
}

// mayPrint: can a print from this request start without asking? The computer itself, any phone when PINs are
// off (then the whole app is open anyway), and phones that logged in with a PIN (or were allowed by hand).
func mayPrint(r *http.Request) (bool, string) {
	if isLocal(r) {
		return true, ""
	}
	settingsMu.Lock()
	open := !settings.RequirePIN
	settingsMu.Unlock()
	if open {
		return true, ""
	}
	mac := macOf(r)
	if mac == "" {
		return false, ""
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	for _, d := range settings.Devices {
		if d.MAC == mac && time.Since(d.Seen) < deviceLife && (d.PinID == "" || slices.ContainsFunc(settings.PINs, func(p PinEntry) bool { return p.ID == d.PinID })) {
			return true, mac
		}
	}
	return false, mac
}

// forgetDevices drops phones that logged in with a removed PIN, and ones not seen for too long. settingsMu held.
func forgetDevices() {
	settings.Devices = slices.DeleteFunc(settings.Devices, func(d Device) bool {
		return time.Since(d.Seen) > deviceLife || (d.PinID != "" && !slices.ContainsFunc(settings.PINs, func(p PinEntry) bool { return p.ID == d.PinID }))
	})
}

// devicesForScreen: the list in Settings (no addresses: they mean nothing to anyone). settingsMu held.
func devicesForScreen() []map[string]any {
	out := []map[string]any{}
	for _, d := range settings.Devices {
		out = append(out, map[string]any{"id": sha(d.MAC)[:12], "label": d.Label, "seen": d.Seen, "byHand": d.PinID == ""})
	}
	return out
}

// removeDevice by the id devicesForScreen gave it. settingsMu held.
func removeDevice(id string) {
	settings.Devices = slices.DeleteFunc(settings.Devices, func(d Device) bool { return sha(d.MAC)[:12] == id })
}
