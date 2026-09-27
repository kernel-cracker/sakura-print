// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Advanced: what's going on under the hood, for fixing problems (the computer only). This copy of Sakura Print,
// the computer, CUPS and its print queues with their drivers, scanners, print from any app, phones, the tools found,
// and the server's recent log. A report of it can be copied for a bug report: it never holds secrets (PINs, their
// hashes, sessions, phones' hardware addresses are never put in) and addresses on the WiFi are blanked out.

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- the server's recent log ----------

type logRing struct {
	mu   sync.Mutex
	max  int
	buf  []string
	part []byte
}

func (r *logRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.part = append(r.part, p...)
	for {
		i := bytes.IndexByte(r.part, '\n')
		if i < 0 {
			break
		}
		r.buf = append(r.buf, string(r.part[:i]))
		r.part = r.part[i+1:]
		if len(r.buf) > r.max {
			r.buf = r.buf[len(r.buf)-r.max:]
		}
	}
	return len(p), nil
}

func (r *logRing) lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.buf...)
}

var serverLog = &logRing{max: 300}

// keepLog: the server's log goes to its usual place and into the ring for the Advanced page
func keepLog() { log.SetOutput(io.MultiWriter(os.Stderr, serverLog)) }

// ---------- blanking out addresses ----------

var (
	ipv4Re = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)
	ipv6Re = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{1,4}\b|(?i)\b(?:[0-9a-f]{1,4}:){1,7}:(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4})*)?\b`)
)

// redact blanks out addresses on the WiFi (and any other network), keeping loopback and version numbers
func redact(s string) string {
	s = ipv4Re.ReplaceAllStringFunc(s, func(m string) string {
		ip := net.ParseIP(m)
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return m
		}
		return "<address>"
	})
	return ipv6Re.ReplaceAllStringFunc(s, func(m string) string {
		ip := net.ParseIP(m)
		if ip == nil || ip.IsLoopback() {
			return m
		}
		return "<address>"
	})
}

// ---------- what's going on ----------

type debugQueue struct {
	Name, URI, Model, Driver, State, Reasons string
}

type debugInfo struct {
	Version, Build, Go, OS, Kernel, Distro string
	Uptime                                 string
	Paths                                  map[string]string
	Install                                system
	CUPSServer                             string
	Default                                string
	Queues                                 []debugQueue
	Scanners                               []Scanner
	ScannersKnown                          bool
	AirPrint                               map[string]any
	Phones                                 map[string]any
	Tools                                  map[string]bool
	Log                                    []string
}

var started = time.Now()

func collectDebug() debugInfo {
	d := debugInfo{Version: version, Build: buildInfo, Go: runtime.Version(), OS: runtime.GOOS + "/" + runtime.GOARCH,
		Uptime: time.Since(started).Round(time.Second).String(), Install: detectSystem()}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		d.Kernel = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		if m := regexp.MustCompile(`(?m)^PRETTY_NAME="?([^"\n]*)`).FindStringSubmatch(string(b)); m != nil {
			d.Distro = m[1]
		}
	}
	d.Paths = map[string]string{"settings": configPath(), "data": dataDir(), "log": "the server's own output (journalctl --user -u sakuraprint)"}
	c := http.Client{Timeout: 3 * time.Second}
	if resp, err := c.Get("http://localhost:631/"); err == nil {
		d.CUPSServer = resp.Header.Get("Server")
		resp.Body.Close()
	} else {
		d.CUPSServer = "not reachable: " + err.Error()
	}
	names, def := listPrinters()
	d.Default = def
	for _, n := range names {
		st := printerStatus(n)
		q := debugQueue{Name: n, Model: st.Model, State: st.State, Reasons: st.Reasons}
		q.Driver = queueDriverState(queueInfo{name: n, model: st.Model})
		if out, err := run("lpstat", "-v", n); err == nil {
			if _, uri, ok := strings.Cut(strings.TrimSpace(out), ": "); ok {
				q.URI = uri
			}
		}
		d.Queues = append(d.Queues, q)
	}
	d.Scanners, d.ScannersKnown = scannersCache.peek()
	settingsMu.Lock()
	d.Phones = map[string]any{"access": phonesState(), "pins": len(settings.PINs), "loggedIn": len(settings.Sessions), "pinRequired": settings.RequirePIN,
		"allowedToPrintFromAnyApp": len(settings.Devices)}
	d.AirPrint = map[string]any{"switchedOn": !settings.AirPrintOff}
	settingsMu.Unlock()
	d.AirPrint["accepting"] = airprintOn()
	d.AirPrint["bonjourTool"] = have("avahi-publish-service")
	airAd.mu.Lock()
	d.AirPrint["announcing"] = airAd.cmd.running()
	airAd.mu.Unlock()
	if ippSrv != nil {
		ippSrv.mu.Lock()
		d.AirPrint["waitingToBeAllowed"] = len(ippSrv.waiting())
		ippSrv.mu.Unlock()
	}
	d.Tools = map[string]bool{}
	for _, t := range []string{"lp", "lpstat", "lpadmin", "qpdf", "pdftoppm", "rsvg-convert", "scanimage", "avahi-publish-service", "notify-send",
		"pkexec", "heif-dec", "magick", "vips", "soffice", "bsdtar", "ipptool"} {
		d.Tools[t] = have(t)
	}
	d.Log = serverLog.lines()
	return d
}

// report: the same as text, for a bug report (addresses blanked out)
func (d debugInfo) report() string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("Sakura Print %s", d.Version)
	w("  %s; %s; %s", d.Build, d.Go, d.OS)
	w("Computer: %s, Linux %s; installs with %s; up %s", firstNonEmpty(d.Distro, "unknown Linux"), d.Kernel, firstNonEmpty(d.Install.Kind, "nothing known"), d.Uptime)
	w("CUPS: %s (major %d)", d.CUPSServer, d.Install.CUPS)
	w("Print queues (default: %s):", firstNonEmpty(d.Default, "none"))
	driverWords := map[string]string{"driver": "the maker's driver", "driverless": "driverless", "broken": "its driver is missing"}
	for _, q := range d.Queues {
		why := ""
		if q.Reasons != "" && q.Reasons != "none" {
			why = " (" + q.Reasons + ")"
		}
		w("  %s: %s, %s, %s%s, %s", q.Name, q.Model, firstNonEmpty(driverWords[q.Driver], q.Driver), q.State, why, q.URI)
	}
	if !d.ScannersKnown {
		w("Scanners: not looked for yet")
	} else {
		w("Scanners: %d", len(d.Scanners))
		for _, s := range d.Scanners {
			w("  %s (%s)", s.Name, s.Device)
		}
	}
	keys := func(m map[string]any) string {
		var ks []string
		for k, v := range m {
			if st, ok := v.(map[string]any); ok { // phone access: "off", "on", or "until 21:40"
				v = st["state"]
				if u, ok := st["until"].(time.Time); ok && st["state"] == "until" {
					v = "until " + u.Format("15:04")
				}
			}
			ks = append(ks, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(ks)
		return strings.Join(ks, ", ")
	}
	w("Print from any app: %s", keys(d.AirPrint))
	w("Phones: %s", keys(d.Phones))
	var tools []string
	for t, ok := range d.Tools {
		tools = append(tools, map[bool]string{true: t, false: t + " (missing)"}[ok])
	}
	sort.Strings(tools)
	w("Tools: %s", strings.Join(tools, ", "))
	w("Recent log:")
	for _, l := range d.Log {
		w("  %s", l)
	}
	return redact(b.String())
}

func serveDebug(api func(string, func(http.ResponseWriter, *http.Request))) {
	local := func(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if !isLocal(r) {
				fail(w, 403, "Only on the computer itself")
				return
			}
			h(w, r)
		}
	}
	api("/api/debug", local(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, collectDebug()) }))
	api("/api/debug/report", local(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, collectDebug().report())
	}))
	api("/api/debug/doctor", local(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		doctorDrivers(&b)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, redact(b.String()))
	}))
	api("/api/debug/refresh", local(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 400, "bad request")
			return
		}
		rereadPrinters()
		go listScanners(true)
		ppdCacheMu.Lock()
		ppdCache = map[string]struct {
			text string
			at   time.Time
		}{}
		ppdCacheMu.Unlock()
		log.Printf("Advanced: printer and scanner information refreshed")
		writeJSON(w, map[string]bool{"ok": true})
	}))
}
