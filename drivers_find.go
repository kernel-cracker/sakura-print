// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Drivers: finding printers (lpinfo), their print queues, and which installed driver fits (see docs/design/drivers.md).

// ---------- finding printers ----------

type devURI struct {
	URI      string `json:"uri"`
	Class    string `json:"class"`
	DeviceID string `json:"-"`
}

type device struct {
	Key         string   `json:"key"`
	Make        string   `json:"make"`
	Model       string   `json:"model"`
	Name        string   `json:"name"` // make and model, for the screen
	DeviceID    string   `json:"deviceId"`
	URIs        []devURI `json:"uris"`
	Queue       string   `json:"queue,omitempty"`       // its print queue, if it has one
	QueueDriver string   `json:"queueDriver,omitempty"` // "driver" | "driverless" | "broken" (its driver's programs are gone)
	QueueModel  string   `json:"queueModel,omitempty"`
	Maker       string   `json:"maker,omitempty"` // the maker's hints key
}

// parseDeviceID reads an IEEE 1284 device ID: "MFG:Brother;MDL:DCP-T510W;CMD:…"
func parseDeviceID(id string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(id, ";") {
		if k, v, ok := strings.Cut(part, ":"); ok {
			k = strings.ToUpper(strings.TrimSpace(k))
			switch k {
			case "MANUFACTURER":
				k = "MFG"
			case "MODEL":
				k = "MDL"
			case "COMMAND SET":
				k = "CMD"
			}
			m[k] = strings.TrimSpace(v)
		}
	}
	return m
}

var keepRe = regexp.MustCompile(`[^a-z0-9]`)

func modelKey(s string) string { return keepRe.ReplaceAllString(strings.ToLower(s), "") }

// parseDevices reads `lpinfo -l -v`: every printer CUPS can see, on the network and USB, grouped per printer
func parseDevices(out string) []device {
	type raw struct{ uri, class, mm, id string }
	var all []raw
	var cur *raw
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "Device: uri = ") {
			all = append(all, raw{uri: strings.TrimPrefix(t, "Device: uri = ")})
			cur = &all[len(all)-1]
			continue
		}
		if cur == nil {
			continue
		}
		k, v, _ := strings.Cut(t, " = ")
		switch k {
		case "class":
			cur.class = v
		case "make-and-model":
			cur.mm = v
		case "device-id":
			cur.id = v
		}
	}
	byKey := map[string]*device{}
	var order []string
	for _, r := range all {
		if !strings.Contains(r.uri, ":") || r.mm == "" || r.mm == "Unknown" || strings.HasPrefix(r.mm, "Sakura Print") {
			continue // a backend with nothing found, or Sakura Print's own "print from any app" printer
		}
		id := parseDeviceID(r.id)
		mk, md := id["MFG"], id["MDL"]
		if mk == "" || md == "" {
			f := strings.SplitN(r.mm, " ", 2)
			if mk == "" {
				mk = f[0]
			}
			if md == "" && len(f) == 2 {
				md = f[1]
			}
		}
		key := modelKey(mk + md)
		d := byKey[key]
		if d == nil {
			d = &device{Key: key, Make: mk, Model: md, Name: strings.TrimSpace(mk + " " + strings.TrimPrefix(md, mk+" "))}
			byKey[key] = d
			order = append(order, key)
		}
		if d.DeviceID == "" || (!strings.Contains(d.DeviceID, "CMD:") && strings.Contains(r.id, "CMD:")) {
			d.DeviceID = r.id
		}
		d.URIs = append(d.URIs, devURI{URI: r.uri, Class: r.class, DeviceID: r.id})
	}
	var out2 []device
	for _, k := range order {
		d := byKey[k]
		if d.DeviceID == "" || !strings.Contains(strings.ToUpper(d.DeviceID), "MDL:") {
			d.DeviceID = fmt.Sprintf("MFG:%s;MDL:%s;", d.Make, d.Model)
		}
		d.Maker, _ = makerOf(d.Make)
		out2 = append(out2, *d)
	}
	return out2
}

// bestURI: where a print queue should send to. For a driver: a name that survives the router giving the printer a
// new address (dnssd), else ipp, socket, lpd, usb. Driverless needs IPP.
func (d device) bestURI(driverless bool) string {
	rank := func(u string) int {
		for i, p := range []string{"dnssd://", "ipps://", "ipp://", "usb://", "socket://", "lpd://"} {
			if strings.HasPrefix(u, p) {
				if driverless && i > 2 {
					return -1
				}
				if driverless && i == 0 && !strings.Contains(u, "._ipp") {
					return -1
				}
				return 10 - i
			}
		}
		return -1
	}
	best, br := "", -1
	for _, u := range d.URIs {
		if r := rank(u.URI); r > br {
			best, br = u.URI, r
		}
	}
	return best
}

// canDriverless: the printer can be reached over IPP (IPP Everywhere / AirPrint; USB printers through ipp-usb)
func (d device) canDriverless() bool { return d.bestURI(true) != "" }

type queueInfo struct{ name, uri, model string }

// queues: the print queues there are, with where they send and their driver's name
func queues() []queueInfo {
	out, _ := sys.run("lpstat", "-v")
	var qs []queueInfo
	for _, l := range strings.Split(out, "\n") {
		// "device for NAME: URI"
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), "device for ")
		if !ok {
			continue
		}
		name, uri, ok := strings.Cut(rest, ": ")
		if !ok {
			continue
		}
		q := queueInfo{name: name, uri: uri}
		if o, err := sys.run("lpoptions", "-p", name); err == nil {
			if m := regexp.MustCompile(`printer-make-and-model='([^']*)'`).FindStringSubmatch(o); m != nil {
				q.model = m[1]
			} else if m := regexp.MustCompile(`printer-make-and-model=(\S+)`).FindStringSubmatch(o); m != nil {
				q.model = m[1]
			}
		}
		qs = append(qs, q)
	}
	return qs
}

func isDriverlessModel(m string) bool {
	l := strings.ToLower(m)
	return strings.Contains(l, "driverless") || strings.Contains(l, "ipp everywhere") || strings.Contains(l, "everywhere")
}

// queueDriverState: driverless, a driver, or a driver whose programs were removed (found testing for real: removing
// a driver package leaves its queue behind, and it can't print). CUPS serves each queue's driver file, which names
// the programs it needs.
func queueDriverState(q queueInfo) string {
	if isDriverlessModel(q.model) {
		return "driverless"
	}
	if missingFilters(q.name) {
		return "broken"
	}
	return "driver"
}

var filterDirs = []string{"/usr/lib/cups/filter", "/usr/libexec/cups/filter"}

func missingFilters(queue string) bool {
	var ppd string
	if f, ok := sys.(*fakeSystem); ok {
		ppd, _ = f.run("ppd", queue)
	} else {
		c := http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get("http://localhost:631/printers/" + url.PathEscape(queue) + ".ppd")
		if err != nil {
			return false
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return false
		}
		ppd = string(b)
	}
	for _, m := range regexp.MustCompile(`(?m)^\*cupsFilter2?:\s*"([^"]*)"`).FindAllStringSubmatch(ppd, -1) {
		f := strings.Fields(m[1])
		prog := f[len(f)-1]
		if prog == "-" || strings.HasPrefix(prog, "/") && sys.exists(prog) {
			continue
		}
		found := false
		for _, d := range filterDirs {
			if sys.exists(filepath.Join(d, prog)) {
				found = true
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// findDevices: the printers CUPS can see, each with its queue if it has one. Queues whose printer is switched off
// right now are listed too.
func findDevices() []device {
	out, _ := sys.run("lpinfo", "--timeout", "10", "-l", "-v")
	ds := parseDevices(out)
	qs := queues()
	used := map[string]bool{}
	for i := range ds {
		mk := modelKey(ds[i].Model)
		for _, q := range qs {
			if used[q.name] || mk == "" || !strings.Contains(modelKey(q.model), mk) {
				continue
			}
			ds[i].Queue, ds[i].QueueModel = q.name, q.model
			ds[i].QueueDriver = queueDriverState(q)
			used[q.name] = true
			break
		}
	}
	for _, q := range qs { // queues for printers not found right now (off, or elsewhere)
		if used[q.name] {
			continue
		}
		mm := strings.TrimSpace(strings.Split(q.model, ",")[0])
		mm = strings.TrimSuffix(strings.TrimSuffix(mm, " CUPS"), " - IPP Everywhere")
		f := strings.SplitN(mm, " ", 2)
		d := device{Key: "queue-" + modelKey(q.name), Name: firstNonEmpty(mm, q.name), Queue: q.name, QueueModel: q.model, QueueDriver: "driver",
			URIs: []devURI{{URI: q.uri}}}
		if len(f) == 2 {
			d.Make, d.Model = f[0], f[1]
		}
		d.DeviceID = fmt.Sprintf("MFG:%s;MDL:%s;", d.Make, d.Model)
		d.QueueDriver = queueDriverState(q)
		d.Maker, _ = makerOf(d.Make)
		ds = append(ds, d)
	}
	return ds
}

// installedDrivers: what CUPS says fits the printer among the drivers already installed (not driverless ones)
func installedDrivers(d device) []string {
	out, _ := sys.run("lpinfo", "--device-id", d.DeviceID, "-m")
	var ppds []string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 || strings.HasPrefix(f[0], "driverless:") || f[0] == "everywhere" || strings.HasPrefix(f[0], "drv:///sample.drv") {
			continue
		}
		ppds = append(ppds, f[0])
	}
	return ppds
}
