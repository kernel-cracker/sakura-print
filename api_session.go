// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"net/http"
	"slices"
	"time"
)

// Logging in from a phone with a PIN. (see docs/design/server.md)

func (s *server) routesSession() {
	mux := s.mux
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ PIN string }
		if r.Method != http.MethodPost || readJSON(r, &req) != nil {
			fail(w, 400, "bad request")
			return
		}
		key := clientKey(r)
		loginMu.Lock() // one guess at a time, so a burst of guesses can't slip past the limit together
		if wait := logins.wait(key, time.Now()); wait > 0 {
			loginMu.Unlock()
			fail(w, 429, fmt.Sprintf("Too many wrong PINs. Try again in %d minutes.", int(wait.Minutes())+1))
			return
		}
		settingsMu.Lock()
		pins := slices.Clone(settings.PINs)
		settingsMu.Unlock()
		if len(pins) == 0 {
			loginMu.Unlock()
			fail(w, 403, "No phone PIN is set up yet. On the computer, open Sakura Print, Settings, Phone access, and add one.")
			return
		}
		id := ""
		for _, p := range pins {
			if p.matches(req.PIN) && id == "" {
				id = p.ID
			}
		}
		if id == "" {
			logins.failed(key, time.Now())
			loginMu.Unlock()
			time.Sleep(time.Second)
			fail(w, 403, "wrong PIN")
			return
		}
		logins.succeeded(key)
		loginMu.Unlock()
		settingsMu.Lock()
		tok := newSession(id)
		settingsMu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sakura_session", Value: tok, Path: "/", MaxAge: int(sessionLife.Seconds()),
			HttpOnly: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, map[string]bool{"ok": true})
	})

}
