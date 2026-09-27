// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Security for phones on a WiFi shared with strangers (an apartment block, a hostel...):
//   - PINs are stored hashed, and logging in hands out a random session instead of a PIN-derived cookie
//   - wrong PINs lock out only the phone that typed them, so a neighbour can't lock Mom out
//   - other websites open in the computer's browser can't use the app behind your back

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// ---------- PINs and sessions ----------

const pinRounds = 200_000

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPIN(pin, saltHex string) string {
	salt, _ := hex.DecodeString(saltHex)
	k, _ := pbkdf2.Key(sha256.New, pin, salt, pinRounds, 32)
	return hex.EncodeToString(k)
}

func newPinEntry(label, pin string) PinEntry {
	salt := randHex(16)
	return PinEntry{ID: newID(), Label: label, Salt: salt, Hash: hashPIN(pin, salt)}
}

func (p PinEntry) matches(pin string) bool {
	return subtle.ConstantTimeCompare([]byte(hashPIN(pin, p.Salt)), []byte(p.Hash)) == 1
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

const sessionLife = 365 * 24 * time.Hour
const maxSessions = 100

// newSession remembers a login and returns the cookie value. Only its hash is saved. Call with settingsMu held.
func newSession(pinID string) string {
	tok := randHex(32)
	settings.Sessions = append(settings.Sessions, Session{Hash: sha(tok), PinID: pinID, Created: time.Now()})
	if n := len(settings.Sessions); n > maxSessions {
		settings.Sessions = settings.Sessions[n-maxSessions:]
	}
	saveSettings()
	return tok
}

// validSession checks a cookie value. Call with settingsMu held.
func validSession(tok string) bool {
	_, ok := sessionPin(tok)
	return ok
}

// sessionPin checks a cookie value and says which PIN it logged in with. Call with settingsMu held.
func sessionPin(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	h := []byte(sha(tok))
	for _, s := range settings.Sessions {
		if subtle.ConstantTimeCompare(h, []byte(s.Hash)) == 1 && time.Since(s.Created) < sessionLife {
			return s.PinID, true
		}
	}
	return "", false
}

// migrateSecurity turns old plain-text PINs into hashed ones and drops expired sessions. Call with settingsMu held.
func migrateSecurity() (changed bool) {
	if settings.PIN != "" {
		if pinRe.MatchString(settings.PIN) {
			settings.PINs = append(settings.PINs, PinEntry{Label: "Default", PIN: settings.PIN})
		}
		settings.PIN = ""
		changed = true
	}
	for i, p := range settings.PINs {
		if p.PIN != "" {
			e := newPinEntry(p.Label, p.PIN)
			if p.ID != "" {
				e.ID = p.ID
			}
			settings.PINs[i] = e
			changed = true
		} else if p.ID == "" {
			settings.PINs[i].ID = newID()
			changed = true
		}
	}
	devices := len(settings.Devices)
	forgetDevices()
	changed = changed || len(settings.Devices) != devices
	before := len(settings.Sessions)
	settings.Sessions = slices.DeleteFunc(settings.Sessions, func(s Session) bool { return time.Since(s.Created) > sessionLife })
	return changed || len(settings.Sessions) != before
}

// pinsForScreen is what the computer's own screen gets: names only, a hashed PIN can't be shown
func pinsForScreen() []map[string]string {
	out := []map[string]string{}
	for _, p := range settings.PINs {
		out = append(out, map[string]string{"id": p.ID, "label": p.Label})
	}
	return out
}

// ---------- wrong-PIN limits ----------

// Each phone (by address) gets 5 tries, then waits 10 minutes. On top of that, more than 30 wrong PINs
// in an hour from everyone together pauses new logins for a while, so someone with lots of addresses
// can't just keep guessing. Phones that are already logged in are never affected.
type loginLimiter struct {
	mu     sync.Mutex
	phones map[string]*phoneTries
	recent []time.Time // every wrong PIN in the last hour
}

type phoneTries struct {
	wrong  int
	until  time.Time
	lastAt time.Time
}

const (
	triesPerPhone = 5
	phoneWait     = 10 * time.Minute
	triesPerHour  = 30
)

var logins = &loginLimiter{phones: map[string]*phoneTries{}}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if ip.To4() == nil { // a phone can pick any address in its IPv6 network, so count the whole network
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}
	return ip.String()
}

// wait says how long this phone has to wait before trying, 0 = go ahead
func (l *loginLimiter) wait(key string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.forget(now)
	if p := l.phones[key]; p != nil && now.Before(p.until) {
		return p.until.Sub(now)
	}
	if len(l.recent) >= triesPerHour {
		return l.recent[0].Add(time.Hour).Sub(now)
	}
	return 0
}

func (l *loginLimiter) failed(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.phones[key]
	if p == nil {
		p = &phoneTries{}
		l.phones[key] = p
	}
	p.wrong++
	p.lastAt = now
	if p.wrong >= triesPerPhone {
		p.wrong, p.until = 0, now.Add(phoneWait)
		log.Printf("5 wrong PINs from %s, that phone has to wait 10 minutes", key)
	}
	l.recent = append(l.recent, now)
	if len(l.recent) == triesPerHour {
		log.Printf("%d wrong PINs in the last hour, new phone logins paused for a while", triesPerHour)
	}
}

func (l *loginLimiter) succeeded(key string) {
	l.mu.Lock()
	delete(l.phones, key)
	l.mu.Unlock()
}

func (l *loginLimiter) forget(now time.Time) {
	i := 0
	for i < len(l.recent) && now.Sub(l.recent[i]) > time.Hour {
		i++
	}
	l.recent = l.recent[i:]
	for k, p := range l.phones {
		if now.After(p.until) && now.Sub(p.lastAt) > time.Hour {
			delete(l.phones, k)
		}
	}
}

// ---------- who is asking ----------

// isLocal: the request comes from this computer, through a name that means this computer.
// Checking the name too stops a web page in the computer's browser from sneaking in
// through a look-alike address (DNS rebinding).
func isLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	name := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		name = h
	}
	name = strings.Trim(strings.ToLower(name), "[]")
	if name == "localhost" {
		return true
	}
	nip := net.ParseIP(name)
	return nip != nil && nip.IsLoopback()
}

// ---------- the phone access switch ----------

// phonesAllowed: phones may connect (always on, or switched on for a while and the time isn't up)
func phonesAllowed() bool {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return !settings.PhonesOff || time.Now().Before(settings.PhonesTill)
}

// phonesState is the switch as the computer's screen shows it: on | off | until, and until when
func phonesState() map[string]any {
	on := !settings.PhonesOff
	if !on && time.Now().Before(settings.PhonesTill) {
		return map[string]any{"state": "until", "until": settings.PhonesTill}
	}
	if on {
		return map[string]any{"state": "on"}
	}
	return map[string]any{"state": "off"}
}

const phonesOffPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sakura Print</title><style>body{font-family:system-ui,sans-serif;background:#fbe9ee;color:#2c2a2e;display:grid;place-items:center;min-height:100vh;margin:0;padding:24px;box-sizing:border-box}
div{background:#fff;border-radius:22px;padding:28px;max-width:360px;text-align:center;box-shadow:0 10px 40px rgba(60,20,40,.12)}h1{font-size:22px;color:#9c4a61;margin:0 0 10px}p{line-height:1.5;margin:0}</style></head>
<body><div><h1>Phone access is off</h1><p>To print from this phone, open <b>Sakura Print</b> on the computer and turn on phone access (on its home screen, or in Settings).</p></div></body></html>`

// protect wraps the whole app: other websites can't send requests on your behalf,
// and nobody can put the app inside their own page.
func protect(next http.Handler) http.Handler {
	h := http.NewCrossOriginProtection().Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !isLocal(r) && !phonesAllowed() { // phone access switched off: phones get a friendly page, nothing else
			if strings.HasPrefix(r.URL.Path, "/api/") {
				fail(w, 403, "phones off")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(403)
			io.WriteString(w, phonesOffPage)
			return
		}
		h.ServeHTTP(w, r)
	})
}
