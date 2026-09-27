// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// What the app needs to start (/api/info) and the settings screen (/api/settings). (see docs/design/server.md)

func (s *server) routesInfo() {
	port, api := s.port, s.api
	api("/api/info", func(w http.ResponseWriter, r *http.Request) {
		ps, def := listPrinters()
		settingsMu.Lock()
		s := settings
		settingsMu.Unlock()
		if s.Printer != "" && validPrinter(s.Printer) {
			def = s.Printer
		}
		_, scanErr := exec.LookPath("scanimage")
		scs, known := scannersCache.peek()
		scanOK := scanErr == nil && (!known || len(scs) > 0) // hide Scan and Copy once we know there's no scanner
		info := map[string]any{
			"version": version, "printers": ps, "default": def, "palettes": palettes, "theme": s.Theme,
			"local": isLocal(r), "urls": lanURLs(port), "canScan": scanOK,
		}
		if isLocal(r) {
			settingsMu.Lock()
			info["pins"] = pinsForScreen()
			info["phones"] = phonesState()
			settingsMu.Unlock()
			info["requirePin"] = s.RequirePIN
			info["welcome"] = s.Welcome
			info["airprint"] = !s.AirPrintOff
		}
		writeJSON(w, info)
	})

	api("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req struct {
				Theme       *int
				RequirePIN  *bool
				AddPIN      *struct{ Label, PIN string }
				Phones      string // on | off | 1h | 3h
				AirPrint    *bool  // print from any app on/off
				Welcome     *bool  // first-time setup finished
				RemovePIN   string // the PIN's id
				RemovePhone string // a phone that may print from any app, by the id Settings shows
				Printer     *string
			}
			if readJSON(r, &req) != nil {
				fail(w, 400, "bad request")
				return
			}
			settingsMu.Lock()
			if req.Theme != nil && *req.Theme >= 0 && *req.Theme < len(palettes) {
				settings.Theme = *req.Theme
			}
			if isLocal(r) { // only the laptop itself can change security stuff
				if req.RequirePIN != nil {
					settings.RequirePIN = *req.RequirePIN
				}
				if req.AirPrint != nil {
					settings.AirPrintOff = !*req.AirPrint
				}
				if req.Welcome != nil {
					settings.Welcome = *req.Welcome
				}
				switch req.Phones {
				case "on":
					settings.PhonesOff, settings.PhonesTill = false, time.Time{}
				case "off":
					settings.PhonesOff, settings.PhonesTill = true, time.Time{}
				case "1h", "3h":
					h := map[string]time.Duration{"1h": time.Hour, "3h": 3 * time.Hour}[req.Phones]
					settings.PhonesOff, settings.PhonesTill = true, time.Now().Add(h)
				}
				if a := req.AddPIN; a != nil {
					label := strings.TrimSpace(a.Label)
					if len([]rune(label)) > 20 {
						label = string([]rune(label)[:20])
					}
					msg := ""
					switch {
					case !pinRe.MatchString(a.PIN):
						msg = "a PIN is 4 to 8 numbers"
					case len(settings.PINs) >= 10:
						msg = "that's already 10 PINs, remove one first"
					case slices.ContainsFunc(settings.PINs, func(p PinEntry) bool { return p.matches(a.PIN) }):
						msg = "that PIN is already used"
					}
					if msg != "" {
						settingsMu.Unlock()
						fail(w, 400, msg)
						return
					}
					if label == "" {
						label = fmt.Sprintf("PIN %d", len(settings.PINs)+1)
					}
					settings.PINs = append(settings.PINs, newPinEntry(label, a.PIN))
					log.Printf("Settings: PIN added for %s", label) // the name only, never the PIN
				}
				if id := req.RemovePIN; id != "" {
					for _, p := range settings.PINs {
						if p.ID == id {
							log.Printf("Settings: PIN for %s removed (its phones are logged out)", p.Label)
						}
					}
					settings.PINs = slices.DeleteFunc(settings.PINs, func(p PinEntry) bool { return p.ID == id })
					// phones that logged in with it are logged out
					settings.Sessions = slices.DeleteFunc(settings.Sessions, func(x Session) bool { return x.PinID == id })
					forgetDevices() // its phones can't print from other apps any more either
				}
				if req.RemovePhone != "" {
					removeDevice(req.RemovePhone)
					log.Printf("Settings: a phone can no longer print from other apps without asking")
				}
				if req.Phones != "" {
					log.Printf("Settings: phone access %s", map[string]string{"on": "always on", "off": "off", "1h": "on for 1 hour", "3h": "on for 3 hours"}[req.Phones])
				}
				if req.AirPrint != nil {
					log.Printf("Settings: print from any app %s", map[bool]string{true: "on", false: "off"}[*req.AirPrint])
				}
				if req.RequirePIN != nil {
					log.Printf("Settings: PINs %s", map[bool]string{true: "asked for on phones", false: "switched off: any phone on the WiFi can use it"}[*req.RequirePIN])
				}
			}
			if req.Printer != nil {
				settings.Printer = *req.Printer
			}
			saveSettings()
			settingsMu.Unlock()
		}
		settingsMu.Lock()
		s := map[string]any{"theme": settings.Theme, "printer": settings.Printer}
		if isLocal(r) {
			s["pins"], s["requirePin"] = pinsForScreen(), settings.RequirePIN
			s["phones"] = phonesState()
			s["airprint"] = !settings.AirPrintOff
			s["devices"] = devicesForScreen()
			s["bonjour"] = have("avahi-publish-service")
			s["welcome"] = settings.Welcome
		}
		settingsMu.Unlock()
		writeJSON(w, s)
	})

}
