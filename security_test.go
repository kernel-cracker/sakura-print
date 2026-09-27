// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withConfig(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if content != "" {
		os.MkdirAll(filepath.Join(dir, "sakuraprint"), 0o700)
		os.WriteFile(configPath(), []byte(content), 0o600)
	}
}

func TestOldPlainPINsGetHashed(t *testing.T) {
	withConfig(t, `{"requirePin":true,"pin":"1111","pins":[{"label":"Mom","pin":"123456"}],"secret":"abc"}`)
	if !loadSettings() {
		t.Fatal("expected a migration")
	}
	saveSettings()
	raw, _ := os.ReadFile(configPath())
	for _, leak := range []string{`"123456"`, `"1111"`, `"secret"`} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("settings file still contains %s:\n%s", leak, raw)
		}
	}
	loadSettings()
	if len(settings.PINs) != 2 {
		t.Fatalf("want 2 PINs, got %+v", settings.PINs)
	}
	for _, p := range settings.PINs {
		if p.ID == "" || p.Salt == "" || p.Hash == "" || p.PIN != "" {
			t.Fatalf("bad entry %+v", p)
		}
	}
	if !settings.PINs[0].matches("123456") || settings.PINs[0].matches("123457") || !settings.PINs[1].matches("1111") {
		t.Fatal("hashed PINs don't match correctly")
	}
}

func TestFreshInstallHasNoPIN(t *testing.T) {
	withConfig(t, "")
	loadSettings()
	if len(settings.PINs) != 0 || !settings.RequirePIN {
		t.Fatalf("fresh install should require a PIN and have none yet: %+v", settings)
	}
}

func TestSessions(t *testing.T) {
	withConfig(t, "")
	loadSettings()
	tok := newSession("p1")
	if !validSession(tok) || validSession(tok+"x") || validSession("") {
		t.Fatal("session check wrong")
	}
	raw, _ := os.ReadFile(configPath())
	if strings.Contains(string(raw), tok) {
		t.Fatal("the raw session token was saved")
	}
	for i := 0; i < maxSessions+5; i++ {
		newSession("p2")
	}
	if len(settings.Sessions) != maxSessions || validSession(tok) {
		t.Fatalf("oldest sessions should be dropped, have %d", len(settings.Sessions))
	}
}

func TestLimiterOnlyLocksTheGuessingPhone(t *testing.T) {
	l := &loginLimiter{phones: map[string]*phoneTries{}}
	now := time.Now()
	for i := 0; i < triesPerPhone; i++ {
		if l.wait("10.0.0.66", now) != 0 {
			t.Fatalf("locked too early at try %d", i)
		}
		l.failed("10.0.0.66", now)
	}
	if l.wait("10.0.0.66", now) == 0 {
		t.Fatal("neighbour should be locked out")
	}
	if l.wait("10.0.0.5", now) != 0 {
		t.Fatal("Mom's phone must not be locked by the neighbour")
	}
	if l.wait("10.0.0.66", now.Add(phoneWait+time.Second)) != 0 {
		t.Fatal("lock should end")
	}
}

func TestLimiterCapsManyAddresses(t *testing.T) {
	l := &loginLimiter{phones: map[string]*phoneTries{}}
	now := time.Now()
	for i := 0; i < triesPerHour; i++ {
		l.failed(net.IPv4(10, 1, byte(i/250), byte(i%250)).String(), now)
	}
	if l.wait("10.9.9.9", now) == 0 {
		t.Fatal("30 wrong PINs an hour from many addresses should pause new logins")
	}
	if l.wait("10.9.9.9", now.Add(time.Hour+time.Second)) != 0 {
		t.Fatal("the pause should end after an hour")
	}
}

func TestIPv6PhonesCountedPerNetwork(t *testing.T) {
	a := clientKey(&http.Request{RemoteAddr: "[2001:db8:1:2::aaaa]:5000"})
	b := clientKey(&http.Request{RemoteAddr: "[2001:db8:1:2::bbbb]:5000"})
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestIsLocal(t *testing.T) {
	cases := []struct {
		remote, host string
		want         bool
	}{
		{"127.0.0.1:1234", "localhost:8632", true},
		{"127.0.0.1:1234", "127.0.0.1:8632", true},
		{"[::1]:1234", "[::1]:8632", true},
		{"127.0.0.1:1234", "evil.example:8632", false}, // DNS rebinding
		{"192.168.0.50:1234", "localhost:8632", false},
	}
	for _, c := range cases {
		r := &http.Request{RemoteAddr: c.remote, Host: c.host}
		if got := isLocal(r); got != c.want {
			t.Errorf("isLocal(%s, Host %s) = %v", c.remote, c.host, got)
		}
	}
}

func TestProtect(t *testing.T) {
	h := protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))

	// another website in the computer's browser can't post to the app
	r := httptest.NewRequest("POST", "http://localhost:8632/api/settings", strings.NewReader(`{"requirePin":false}`))
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-site POST: %d", w.Code)
	}
	// the app itself can
	r = httptest.NewRequest("POST", "http://localhost:8632/api/settings", strings.NewReader(`{}`))
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("same-origin POST: %d", w.Code)
	}
}

func BenchmarkPINCheck(b *testing.B) {
	p := newPinEntry("x", "123456")
	for b.Loop() {
		p.matches("123456")
	}
}

func TestPhoneSwitch(t *testing.T) {
	withConfig(t, "")
	loadSettings()
	h := protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "app") }))
	get := func(remote, host, path string) (int, string) {
		r := httptest.NewRequest("GET", "http://"+host+path, nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	phone, laptop := "192.168.0.50:4000", "127.0.0.1:4000"
	if c, _ := get(phone, "192.168.0.100:8632", "/"); c != 200 {
		t.Fatalf("phones are allowed by default: %d", c)
	}
	settings.PhonesOff = true
	if c, body := get(phone, "192.168.0.100:8632", "/"); c != 403 || !strings.Contains(body, "Phone access is off") {
		t.Fatalf("switched off: phones get the friendly page, got %d", c)
	}
	if c, body := get(phone, "192.168.0.100:8632", "/api/info"); c != 403 || !strings.Contains(body, "phones off") {
		t.Fatalf("switched off: no API for phones, got %d %s", c, body)
	}
	if c, _ := get(laptop, "localhost:8632", "/api/info"); c != 200 {
		t.Fatalf("the computer itself always works: %d", c)
	}
	if c, _ := get(laptop, "evil.example:8632", "/"); c != 403 {
		t.Fatalf("a look-alike address on the computer counts as a phone: %d", c)
	}
	settings.PhonesTill = time.Now().Add(time.Hour)
	if c, _ := get(phone, "192.168.0.100:8632", "/"); c != 200 {
		t.Fatalf("on for an hour: phones allowed, got %d", c)
	}
	if st := phonesState(); st["state"] != "until" {
		t.Fatalf("state should say until: %v", st)
	}
	settings.PhonesTill = time.Now().Add(-time.Second)
	if c, _ := get(phone, "192.168.0.100:8632", "/"); c != 403 {
		t.Fatalf("the hour is up: off again, got %d", c)
	}
	settings.PhonesOff, settings.PhonesTill = false, time.Time{}
}

// updating from before the first-time setup existed: a printer already set up means setup isn't forced again
func TestWelcomeOnUpdate(t *testing.T) {
	withConfig(t, `{"setup":{"DCPT510W-Brother":2},"styles":{"DCPT510W-Brother":"rear"}}`)
	loadSettings()
	if !settings.Welcome {
		t.Error("an install that already set up its printer shouldn't be sent through setup again")
	}
	withConfig(t, `{"setup":{}}`)
	loadSettings()
	if settings.Welcome {
		t.Error("a fresh install gets the first-time setup")
	}
	withConfig(t, `{"setup":{"P":2},"welcome":false}`)
	loadSettings()
	if !settings.Welcome {
		t.Error("the flag didn't exist before: its absence and false read the same, so a set-up printer decides")
	}
}
