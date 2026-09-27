// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Everything that talks to CUPS (the Linux printing system) lives here.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PPDOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

type Marker struct {
	Name  string `json:"name"`
	Level int    `json:"level"` // 0-100, or -1 when the printer doesn't report it
	Color string `json:"color"`
}

type PrinterStatus struct {
	Name     string   `json:"name"`
	Model    string   `json:"model"`
	State    string   `json:"state"`
	Reasons  string   `json:"reasons"`
	Jobs     []string `json:"jobs"`
	Markers  []Marker `json:"markers"`
	Commands []string `json:"commands"`
	Profile  *Profile `json:"profile"`
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// Most CUPS commands that look at "all printers" (lpstat -a, lpoptions -p) spend about a second
// searching the network for printers every single time. So the things that almost never change
// (which printers exist, their driver options, their model and ink info) are remembered for a
// while and refreshed in the background, and only the live state is asked for each time, with
// commands that go straight to the one printer (fast).

// cached keeps a value and refreshes it in the background once it's older than maxAge.
// Only the very first call waits.
type cached[T any] struct {
	mu      sync.Mutex
	val     T
	at      time.Time
	loading bool
	maxAge  time.Duration
	load    func() T
}

func (c *cached[T]) get() T {
	c.mu.Lock()
	if c.at.IsZero() {
		c.mu.Unlock()
		return c.refresh()
	}
	if time.Since(c.at) > c.maxAge && !c.loading {
		c.loading = true
		go c.refresh()
	}
	v := c.val
	c.mu.Unlock()
	return v
}

// peek never waits: it says false if the value hasn't been read even once yet (and starts reading it)
func (c *cached[T]) peek() (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if (c.at.IsZero() || time.Since(c.at) > c.maxAge) && !c.loading {
		c.loading = true
		go c.refresh()
	}
	return c.val, !c.at.IsZero()
}

func (c *cached[T]) refresh() T {
	v := c.load()
	c.mu.Lock()
	c.val, c.at, c.loading = v, time.Now(), false
	c.mu.Unlock()
	return v
}

type printerList struct {
	names []string
	def   string
}

var printersCache = &cached[printerList]{maxAge: 30 * time.Second, load: func() printerList {
	ps, def := readPrinters()
	return printerList{ps, def}
}}

// per printer: driver options, and the slow-to-read details (model, ink, cleaning commands)
var (
	perPrinterMu sync.Mutex
	optsCache    = map[string]*cached[[]PPDOption]{}
	attrsCache   = map[string]*cached[map[string]string]{}
)

func printerCache[T any](m map[string]*cached[T], printer string, maxAge time.Duration, load func(string) T) *cached[T] {
	perPrinterMu.Lock()
	defer perPrinterMu.Unlock()
	c := m[printer]
	if c == nil {
		c = &cached[T]{maxAge: maxAge, load: func() T { return load(printer) }}
		m[printer] = c
	}
	return c
}

func listPrinters() ([]string, string) {
	l := printersCache.get()
	return l.names, l.def
}

func readPrinters() ([]string, string) {
	out, _ := run("lpstat", "-a")
	var ps []string
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			ps = append(ps, f[0])
		}
	}
	def := ""
	if d, err := run("lpstat", "-d"); err == nil {
		if i := strings.LastIndex(d, ": "); i >= 0 {
			def = strings.TrimSpace(d[i+2:])
		}
	}
	return ps, def
}

func validPrinter(p string) bool {
	if p == "" {
		return false
	}
	ps, _ := listPrinters()
	if slices.Contains(ps, p) {
		return true
	}
	// maybe it was added a moment ago
	ps, _ = rereadPrinters()
	return slices.Contains(ps, p)
}

// rereadPrinters reads the list again right now (only when a name isn't in the remembered list)
func rereadPrinters() ([]string, string) {
	l := printersCache.refresh()
	return l.names, l.def
}

func ppdOptions(printer string) []PPDOption {
	return printerCache(optsCache, printer, 5*time.Minute, readPPDOptions).get()
}

// readPPDOptions reads the printer driver's own options (paper, colour, quality, Brother extras...)
func readPPDOptions(printer string) []PPDOption {
	out, err := exec.Command("lpoptions", "-p", printer, "-l").Output()
	if err != nil {
		return nil
	}
	var opts []PPDOption
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		head, rest := line[:i], line[i+1:]
		key, label := head, head
		if j := strings.Index(head, "/"); j >= 0 {
			key, label = head[:j], head[j+1:]
		}
		o := PPDOption{Key: key, Label: label}
		for _, f := range strings.Fields(rest) {
			if strings.HasPrefix(f, "*") {
				f = f[1:]
				o.Default = f
			}
			o.Values = append(o.Values, f)
		}
		if o.Default == "" && len(o.Values) > 0 {
			o.Default = o.Values[0]
		}
		opts = append(opts, o)
	}
	return opts
}

// parseAttrs understands `lpoptions -p X` output: key=value key='quoted\ value' ...
func parseAttrs(s string) map[string]string {
	m := map[string]string{}
	var key, val strings.Builder
	inKey, esc := true, false
	var quote rune
	flush := func() {
		if key.Len() > 0 {
			m[key.String()] = val.String()
		}
		key.Reset()
		val.Reset()
		inKey = true
	}
	for _, r := range s {
		switch {
		case esc:
			if inKey {
				key.WriteRune(r)
			} else {
				val.WriteRune(r)
			}
			esc = false
		case r == '\\':
			esc = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				val.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\n' || r == '\t':
			flush()
		case r == '=' && inKey:
			inKey = false
		default:
			if inKey {
				key.WriteRune(r)
			} else {
				val.WriteRune(r)
			}
		}
	}
	flush()
	return m
}

func printerStatus(printer string) PrinterStatus {
	st := PrinterStatus{Name: printer, Jobs: []string{}, Markers: []Marker{}, Commands: []string{}}
	a := printerCache(attrsCache, printer, time.Minute, readAttrs).get()
	st.Model = a["printer-make-and-model"]
	st.State, st.Reasons = liveState(printer)
	names := splitList(a["marker-names"])
	levels := splitList(a["marker-levels"])
	colors := splitList(a["marker-colors"])
	for i, n := range names {
		mk := Marker{Name: n, Level: -1}
		if i < len(levels) {
			if v, err := strconv.Atoi(levels[i]); err == nil && v >= 0 {
				mk.Level = v
			}
		}
		if i < len(colors) {
			mk.Color = colors[i]
		}
		st.Markers = append(st.Markers, mk)
	}
	for _, c := range splitList(a["printer-commands"]) {
		if c == "Clean" || c == "PrintSelfTestPage" {
			st.Commands = append(st.Commands, c)
		}
	}
	jobs, _ := run("lpstat", "-o", printer)
	for _, l := range strings.Split(jobs, "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			st.Jobs = append(st.Jobs, f[0])
		}
	}
	if p, ok := loadProfile(printer); ok {
		st.Profile = &p
	}
	return st
}

func readAttrs(printer string) map[string]string {
	out, _ := run("lpoptions", "-p", printer)
	return parseAttrs(out)
}

// liveState asks CUPS about this one printer right now (fast, no network search)
func liveState(printer string) (state, reasons string) {
	out, err := run("lpstat", "-l", "-p", printer)
	if err != nil {
		return "unknown", ""
	}
	return parseLpstat(out)
}

// parseLpstat understands `lpstat -l -p NAME`:
//
//	printer NAME is idle.  enabled since ...
//	printer NAME now printing NAME-12.  enabled since ...
//	printer NAME disabled since ... -
//		reason the admin gave
//		Alerts: media-empty-error ...
func parseLpstat(out string) (state, reasons string) {
	first, _, _ := strings.Cut(out, "\n")
	switch {
	case strings.Contains(first, " disabled since "):
		state = "stopped"
	case strings.Contains(first, " now printing "):
		state = "printing"
	case strings.Contains(first, " is idle"):
		state = "ready"
	default:
		state = "unknown"
	}
	for _, l := range strings.Split(out, "\n") {
		if r, ok := strings.CutPrefix(strings.TrimSpace(l), "Alerts:"); ok {
			if r = strings.TrimSpace(r); r != "none" {
				reasons = strings.Join(strings.Fields(r), ",")
			}
		}
	}
	return state, reasons
}

func splitList(s string) []string {
	if s == "" || s == "none" {
		return nil
	}
	return strings.Split(s, ",")
}

// submit sends a PDF to CUPS and returns the CUPS job id
// dryPrint: SAKURA_DRY_PRINT=<folder> makes "printing" copy the file there instead (for the tests: nothing
// ever comes out of a real printer while testing)
var dryPrint = os.Getenv("SAKURA_DRY_PRINT")

func submit(printer, title, file string, opts map[string]string) (string, error) {
	if dryPrint != "" {
		os.MkdirAll(dryPrint, 0o755)
		id := fmt.Sprintf("dry-%d", time.Now().UnixNano())
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return id, os.WriteFile(filepath.Join(dryPrint, id+".pdf"), data, 0o644)
	}
	args := []string{"-d", printer, "-t", title}
	for k, v := range opts {
		if k == "" || v == "" || strings.ContainsAny(k+v, "= \n") {
			continue
		}
		args = append(args, "-o", k+"="+v)
	}
	args = append(args, file)
	out, err := run("lp", args...)
	if err != nil {
		return "", fmt.Errorf("lp failed: %s", strings.TrimSpace(out))
	}
	out = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "request id is "))
	if f := strings.Fields(out); len(f) > 0 {
		return f[0], nil
	}
	return "", errors.New("lp gave no job id")
}

// jobActiveHook: tests say whether a job is still printing (instead of asking CUPS)
var jobActiveHook func(printer, job string) bool

func jobActive(printer, job string) bool {
	if jobActiveHook != nil {
		return jobActiveHook(printer, job)
	}
	if dryPrint != "" {
		return false // "printed" at once
	}
	out, _ := run("lpstat", "-o", printer)
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, job+" ") {
			return true
		}
	}
	return false
}

// maintenance sends a CUPS command file (the driver decides what "Clean" etc. do)
func maintenance(printer, command string) error {
	st := printerStatus(printer)
	allowed := false
	for _, c := range st.Commands {
		if c == command {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("this printer doesn't support %q", command)
	}
	f, err := os.CreateTemp("", "sakura-cmd-*.cmd")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	fmt.Fprintf(f, "#CUPS-COMMAND %s\n", command)
	f.Close()
	_, err = submit(printer, "maintenance: "+command, f.Name(), nil)
	return err
}

// cancelJob stops one print (a job id from submit), leaving everyone else's alone
func cancelJob(job string) error {
	if dryPrint != "" || job == "" {
		return nil
	}
	out, err := run("cancel", job)
	if err != nil && jobActiveAnywhere(job) {
		return fmt.Errorf("cancel failed: %s", strings.TrimSpace(out))
	}
	return nil // already printed or gone
}

func jobActiveAnywhere(job string) bool {
	out, _ := run("lpstat", "-o")
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, job+" ") {
			return true
		}
	}
	return false
}

func cancelAll(printer string) error {
	if dryPrint != "" {
		return nil
	}
	out, err := run("cancel", "-a", printer)
	if err != nil {
		return fmt.Errorf("cancel failed: %s", strings.TrimSpace(out))
	}
	return nil
}
