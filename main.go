// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Sakura Print: a friendly print & scan app for your home printer.
// Vibe coded with Claude. One Go binary, the whole web app is embedded inside.

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const version = "0.21.2"

// buildInfo: how this copy was built (the installer builds everyone's own, tuned to their processor)
var buildInfo = "built by hand"

//go:embed web
var webFiles embed.FS

//go:embed assets/cover.svg
var coverTemplate string

// ---------- desktop window ----------

func serverUp(port int) bool {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/info", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func openWindow(port int) error {
	if !serverUp(port) {
		self, _ := os.Executable()
		cmd := exec.Command(self, "serve", "-port", strconv.Itoa(port))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		logf, _ := os.OpenFile(filepath.Join(os.TempDir(), "sakuraprint.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		cmd.Stdout, cmd.Stderr = logf, logf
		if err := cmd.Start(); err != nil {
			return err
		}
		for i := 0; i < 50 && !serverUp(port); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	url := fmt.Sprintf("http://localhost:%d", port)
	// app-style window (no tabs or address bar) if a Chromium-family browser is around
	for _, b := range []string{"chromium", "google-chrome-stable", "google-chrome", "brave", "brave-browser", "microsoft-edge-stable", "vivaldi-stable"} {
		if p, err := exec.LookPath(b); err == nil {
			return exec.Command(p, "--app="+url, "--window-size=1100,820", "--class=SakuraPrint").Start()
		}
	}
	if p, err := exec.LookPath("firefox"); err == nil {
		return exec.Command(p, "--new-window", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

func main() {
	log.SetFlags(0)
	cmd := "window"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	port := fl.Int("port", 8632, "port for the web app")
	addr := fl.String("addr", "0.0.0.0", "address to listen on (127.0.0.1 = this computer only)")
	fl.Parse(args)

	switch cmd {
	case "serve":
		if err := serve(*addr, *port); err != nil {
			log.Fatal(err)
		}
	case "window":
		if err := openWindow(*port); err != nil {
			log.Fatal(err)
		}
	case "url", "phone":
		loadSettings()
		urls := lanURLs(*port)
		if len(urls) == 0 {
			fmt.Println("No WiFi/network connection found right now, so phones can't reach it yet.")
		} else {
			fmt.Println("Open one of these on a phone connected to the same WiFi:")
		}
		for _, u := range urls {
			fmt.Println("  " + u)
		}
		if len(settings.PINs) == 0 {
			fmt.Println("No phone PIN yet: open Sakura Print on this computer, Settings, Phone access, and add one.")
		} else {
			var names []string
			for _, p := range settings.PINs {
				names = append(names, p.Label)
			}
			fmt.Println("PINs set up for: " + strings.Join(names, ", "))
		}
	case "doctor": // sakuraprint doctor drivers: what each way of getting a driver finds here, and why not
		loadSettings()
		doctorDrivers(os.Stdout)
	case "version", "--version", "-v":
		fmt.Println("sakuraprint", version)
		fmt.Println(buildInfo)
	default:
		fmt.Println(`sakuraprint: a friendly print & scan app for your home printer

usage: sakuraprint           open the app in its own window (starts the server if needed)
       sakuraprint serve     run the server (the installer sets this up to start automatically)
       sakuraprint phone     show the address + PIN to open it on a phone
       sakuraprint doctor drivers   what each way of getting a printer driver finds on this computer
       sakuraprint version

options: -port 8632   -addr 0.0.0.0`)
	}
}

// server: what the route groups (api_*.go) share
type server struct {
	root, work, uploads string
	port                int
	mux                 *http.ServeMux
	api                 func(string, func(http.ResponseWriter, *http.Request))
}
