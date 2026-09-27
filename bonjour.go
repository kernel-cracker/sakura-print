// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"log"
	"os"
	osexec "os/exec"
	"strconv"
	"sync"
	"syscall"
)

// Print from any app, part 5: telling phones the printer is there (Bonjour / mDNS through avahi-publish-service).

// ---------- telling phones it's there (Bonjour / mDNS, via avahi) ----------

// advertiser keeps an avahi-publish-service running while printing from other apps is on
type advertiser struct {
	mu   sync.Mutex
	cmd  *osexecCmd
	name string
}

var airAd = &advertiser{}

func (a *advertiser) sync(port int, uuid string) {
	want := airprintOn() && os.Getenv("SAKURA_NO_BONJOUR") == ""
	name := ""
	if want && ippSrv != nil {
		name = ippSrv.printerName()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmd != nil && (!want || name != a.name || !a.cmd.running()) { // switched off, renamed, or avahi restarted
		a.cmd.stop()
		a.cmd = nil
	}
	if !want || a.cmd != nil || !have("avahi-publish-service") {
		return
	}
	caps := Caps{}
	if p := ippPrinter(); p != "" {
		caps = capsFor(p)
	}
	colour := "F"
	if caps.Colour {
		colour = "T"
	}
	txt := []string{"txtvers=1", "qtotal=1", "rp=ipp/print", "ty=" + name, "product=(" + name + ")", "note=",
		"adminurl=http://" + localName() + ".local:" + strconv.Itoa(port) + "/", "priority=10",
		"pdl=application/pdf,image/jpeg,image/urf,image/pwg-raster", "URF=V1.4,W8,SRGB24,CP1,RS300,DM1", "Color=" + colour, "Duplex=T", "Copies=T",
		"PaperMax=legal-A4", "kind=document,photo", "Transparent=T", "Binary=T", "UUID=" + uuid, "printer-type=0x809046"}
	args := append([]string{"--subtype=_universal._sub._ipp._tcp", name, "_ipp._tcp", strconv.Itoa(port)}, txt...)
	a.cmd = startBackground("avahi-publish-service", args...)
	a.name = name
	log.Printf("AirPrint: announced %q on the WiFi", name)
}

// ---------- a helper program running in the background (avahi-publish-service) ----------

type osexecCmd struct {
	c    *osexec.Cmd
	done chan struct{}
}

func startBackground(name string, args ...string) *osexecCmd {
	c := osexec.Command(name, args...)
	// if Sakura Print stops (even killed), this stops too: phones mustn't keep seeing a printer that isn't there
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := c.Start(); err != nil {
		log.Printf("%s: %v", name, err)
		return nil
	}
	b := &osexecCmd{c: c, done: make(chan struct{})}
	go func() { c.Wait(); close(b.done) }()
	return b
}

func (b *osexecCmd) running() bool {
	if b == nil {
		return false
	}
	select {
	case <-b.done:
		return false
	default:
		return true
	}
}

func (b *osexecCmd) stop() {
	if b.running() {
		b.c.Process.Kill()
		<-b.done
	}
}
