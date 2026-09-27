// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Where things are kept, and the settings file (~/.config/sakuraprint/settings.json).

// ---------- paths ----------

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/tmp"
}

func dataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(home(), ".local", "share")
	}
	return filepath.Join(base, "sakuraprint")
}

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home(), ".config")
	}
	return filepath.Join(base, "sakuraprint", "settings.json")
}

func userDir(kind, fallback string) string {
	if out, err := exec.Command("xdg-user-dir", kind).Output(); err == nil {
		d := strings.TrimSpace(string(out))
		if d != "" && d != home() {
			return d
		}
	}
	return filepath.Join(home(), fallback)
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- settings ----------

type PinEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Salt  string `json:"salt"`
	Hash  string `json:"hash"`
	PIN   string `json:"pin,omitempty"` // PINs used to be saved as they are, only read to hash them
}

// Session is one logged-in phone. Only a hash of its cookie is kept.
type Session struct {
	Hash    string    `json:"hash"`
	PinID   string    `json:"pin"`
	Created time.Time `json:"created"`
}

type Settings struct {
	Theme          int               `json:"theme"` // palette index for the app colours
	RequirePIN     bool              `json:"requirePin"`
	PIN            string            `json:"pin,omitempty"` // old single PIN, moved into PINs
	PINs           []PinEntry        `json:"pins"`          // several PINs can work at once (Mom, Dad...)
	Sessions       []Session         `json:"sessions"`
	Printer        string            `json:"printer"`                  // favourite printer, "" = system default
	Styles         map[string]string `json:"styles"`                   // printer -> "rear" (feeder at the back) or "tray" (front drawer)
	Duplex         map[string]string `json:"duplex"`                   // printer -> "manual" if it can't really print both sides by itself
	Setup          map[string]int    `json:"setup"`                    // printer -> which version of the setup screens it has seen
	PhonesOff      bool              `json:"phonesOff"`                // phone access switched off (the computer always works)
	PhonesTill     time.Time         `json:"phonesTill,omitzero"`      // phone access on until then, then off again
	AirPrintOff    bool              `json:"airprintOff"`              // "print from any app" (AirPrint / Android printing) switched off
	Welcome        bool              `json:"welcome"`                  // first-time setup finished on the computer
	Devices        []Device          `json:"devices,omitempty"`        // phones that may print from any app: devices.go
	DriverHashes   map[string]string `json:"driverHashes,omitempty"`   // makers' driver files: fingerprint when first downloaded
	DriverLicences []LicenceRecord   `json:"driverLicences,omitempty"` // makers' licences agreed to, and when
	PrinterUUID    string            `json:"printerUuid"`              // so phones know it\'s the same printer after a restart
}

var pinRe = regexp.MustCompile(`^[0-9]{4,8}$`)

var (
	settings   Settings
	settingsMu sync.Mutex
)

// loadSettings reads the settings file and upgrades it in memory. It says if anything changed, and
// only the server saves, so the `phone` command never overwrites what the running server knows.
func loadSettings() (changed bool) {
	settings = Settings{Theme: 0, RequirePIN: true}
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, &settings)
	}
	changed = migrateSecurity()
	if settings.Styles == nil {
		settings.Styles = map[string]string{}
	}
	if settings.Duplex == nil {
		settings.Duplex = map[string]string{}
	}
	if settings.Setup == nil {
		settings.Setup = map[string]int{}
	}
	// from before the first-time setup existed: a printer already set up means it isn't forced again (it's in
	// Settings for anyone who wants the tour)
	if !settings.Welcome && len(settings.Setup) > 0 {
		settings.Welcome, changed = true, true
	}
	if settings.Theme < 0 || settings.Theme >= len(palettes) {
		settings.Theme = 0
	}
	return changed
}

func saveSettings() {
	os.MkdirAll(filepath.Dir(configPath()), 0o700)
	data, _ := json.MarshalIndent(settings, "", "  ")
	tmp := configPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		os.Rename(tmp, configPath())
	}
}

func authed(r *http.Request) bool {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if isLocal(r) || !settings.RequirePIN {
		return true
	}
	c, err := r.Cookie("sakura_session")
	if err != nil {
		return false
	}
	pinID, ok := sessionPin(c.Value)
	if ok {
		go noteDevice(r, pinID) // this phone may now print from any app too
	}
	return ok
}
