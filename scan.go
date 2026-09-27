// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Scanning with SANE (scanimage): finding scanners, scanning a page.

// ---------- scanning (SANE) ----------

type Scanner struct {
	Device string `json:"device"`
	Name   string `json:"name"`
}

// Looking for scanners takes up to 10 seconds, so the list is remembered and refreshed in the
// background: nobody waits for it except the very first time (and "Look again").
var scannersCache = &cached[[]Scanner]{maxAge: 2 * time.Minute, load: readScanners}

func listScanners(force bool) []Scanner {
	if force {
		return scannersCache.refresh()
	}
	return scannersCache.get()
}

func readScanners() []Scanner {
	if _, err := exec.LookPath("scanimage"); err != nil {
		return []Scanner{}
	}
	out, _ := runTimeout(exec.Command("scanimage", "-L"), 30*time.Second)
	return parseScanners(out)
}

// parseScanners reads `scanimage -L`. The same scanner often shows up several times (its maker's driver,
// eSCL, airscan): it's listed once, under a tidy name. Webcams show up too ("v4l"), and aren't scanners.
func parseScanners(out string) []Scanner {
	found := []Scanner{}
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		// device `airscan:e0:Brother DCP-T510W' is a eSCL Brother DCP-T510W ip=...
		i, j := strings.Index(l, "`"), strings.Index(l, "'")
		if i < 0 || j <= i {
			continue
		}
		dev := l[i+1 : j]
		if strings.HasPrefix(dev, "v4l:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(l[j+1:], " is a "))
		if k := strings.Index(name, " ip="); k > 0 {
			name = name[:k]
		}
		name = tidyScannerName(name)
		key := strings.Join(modelKeys(name), " ")
		if key == "" {
			key = strings.ToLower(name)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, Scanner{Device: dev, Name: name})
	}
	return found
}

// tidyScannerName: "eSCL Brother DCP-T510W adf scanner" and "Brother DCPT510W DCP-T510W" both become "Brother DCP-T510W"
func tidyScannerName(n string) string {
	for _, p := range []string{"eSCL ", "WSD ", "escl ", "wsd "} {
		n = strings.TrimPrefix(n, p)
	}
	for _, sfx := range []string{" adf scanner", " flatbed scanner", " scanner"} {
		n = strings.TrimSuffix(n, sfx)
	}
	words := strings.Fields(n)
	key := func(w string) string { return strings.Join(modelKeys(w), "") }
	var out []string
	for _, w := range words {
		k := key(w)
		dup := -1
		for i, o := range out {
			if k != "" && key(o) == k {
				dup = i
			}
		}
		switch {
		case dup < 0:
			out = append(out, w)
		case strings.Contains(w, "-") && !strings.Contains(out[dup], "-"):
			out[dup] = w // the maker's own spelling, with its dash
		}
	}
	return strings.Join(out, " ")
}

func runTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(d):
		cmd.Process.Kill()
		return buf.String(), errors.New("timed out")
	}
}

var scanBusy sync.Mutex

func scanPage(device string, dpi int, colour bool) (*File, error) {
	scanBusy.Lock() // a scanner can only do one page at a time, others wait their turn
	defer scanBusy.Unlock()
	if device == "" {
		ss := listScanners(false)
		if len(ss) == 0 {
			return nil, errors.New("no scanner found. Is the printer on and on the same WiFi? Is sane-airscan installed?")
		}
		device = ss[0].Device
	}
	if dpi != 150 && dpi != 300 && dpi != 600 {
		dpi = 300
	}
	dir := filepath.Join(dataDir(), "scans")
	os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, "scan-"+time.Now().Format("20060102-150405")+"-"+newID()[:4]+".png")
	try := func(extra ...string) error {
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		args := append([]string{"-d", device, "--format=png", "--resolution", strconv.Itoa(dpi)}, extra...)
		cmd := exec.Command("scanimage", args...)
		cmd.Stdout = f
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Start()
		if err == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err = <-done:
			case <-time.After(3 * time.Minute):
				cmd.Process.Kill()
				err = errors.New("scanner timed out")
			}
		}
		f.Close()
		if err != nil {
			return fmt.Errorf("%v %s", err, strings.TrimSpace(stderr.String()))
		}
		if st, e := os.Stat(out); e != nil || st.Size() < 100 {
			return errors.New("scanner sent back an empty page")
		}
		return nil
	}
	mode := "Color"
	if !colour {
		mode = "Gray"
	}
	if err := try("--mode", mode); err != nil {
		// some drivers call the modes something else, try the scanner's default
		if err2 := try(); err2 != nil {
			return nil, fmt.Errorf("scan failed: %v", err)
		}
	}
	return addFile(out, "Scan "+time.Now().Format("15:04:05")+".png")
}
