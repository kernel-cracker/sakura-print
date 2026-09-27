// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Drivers, part 1: the computer. How it installs things (found by which tools exist, not by the distribution's
// name), running commands (anything as the administrator goes through ONE password window per install), and a
// pretend computer for the tests, so they never install anything for real.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// sysRunner is everything driver setup does to the computer
type sysRunner interface {
	run(name string, args ...string) (string, error)           // as the person
	root(steps [][]string) (string, error)                     // as the administrator: one password window for all steps
	fetch(u string, max int64) ([]byte, error)                 // HTTPS download
	have(name string) bool                                     // a program exists
	exists(path string) bool                                   // a file exists
	terminal(title string, args ...string) (*osexecCmd, error) // a terminal window the person sees
	open(u string) error                                       // the browser
}

// sys is the real computer, or a pretend one for tests (SAKURA_FAKE_SYSTEM)
var sys sysRunner = realSystem{}

func init() {
	if f := os.Getenv("SAKURA_FAKE_SYSTEM"); f != "" {
		fs, err := loadFakeSystem(f)
		if err != nil {
			panic("SAKURA_FAKE_SYSTEM: " + err.Error())
		}
		sys = fs
	}
}

// ---------- the real computer ----------

type realSystem struct{}

func (realSystem) run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C") // messages in English, so they can be read
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// root runs all the steps in one shell as the administrator, after the system's own password window (polkit)
func (realSystem) root(steps [][]string) (string, error) {
	pk, err := exec.LookPath("pkexec")
	if err != nil {
		return "", errors.New("this computer has no pkexec (polkit), so Sakura Print can't ask for the password: run the commands shown yourself")
	}
	cmd := exec.Command(pk, "/bin/sh", "-c", rootScript(steps))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) && !strings.Contains(string(out), "+ ") {
		return string(out), errors.New("the password wasn't given (the window was closed, or no password window could open on this computer)")
	}
	return string(out), err
}

func (realSystem) fetch(u string, max int64) ([]byte, error) {
	return httpFetch(u, max)
}

func (realSystem) have(name string) bool { return have(name) }

func (realSystem) exists(path string) bool { _, err := os.Stat(path); return err == nil }

// (the pretend computer answers `ppd QUEUE` with a queue's driver file, and lists filter programs under files)

func (realSystem) terminal(title string, args ...string) (*osexecCmd, error) {
	term, targs := findTerminal(title)
	if term == "" {
		return nil, errors.New("no terminal program found")
	}
	b := startBackground(term, append(targs, args...)...)
	if b == nil {
		return nil, errors.New("the terminal didn't start")
	}
	return b, nil
}

func (realSystem) open(u string) error {
	return exec.Command("xdg-open", u).Start()
}

// rootScript: the steps as one shell script that stops at the first failure, and shows each step
func rootScript(steps [][]string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, s := range steps {
		q := make([]string, len(s))
		for i, a := range s {
			q[i] = shellQuote(a)
		}
		line := strings.Join(q, " ")
		// show the step (its first line only: scripts' own text must never look like their output)
		show := strings.SplitN(strings.Join(s, " "), "\n", 2)[0]
		if len(show) > 120 {
			show = show[:120] + "…"
		}
		b.WriteString("echo " + shellQuote("+ "+show) + "\n" + line + "\n")
	}
	return b.String()
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:=+,@") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// findTerminal: the person's terminal program, and how to hand it a command
func findTerminal(title string) (string, []string) {
	if t := os.Getenv("TERMINAL"); t != "" && have(t) {
		return t, []string{"-e"}
	}
	for _, t := range []struct {
		name string
		args []string
	}{
		{"kgx", []string{"--title", title, "--"}},
		{"gnome-terminal", []string{"--title", title, "--wait", "--"}},
		{"konsole", []string{"--hide-menubar", "-e"}},
		{"xfce4-terminal", []string{"--title", title, "--disable-server", "-x"}},
		{"kitty", []string{"--title", title}},
		{"alacritty", []string{"--title", title, "-e"}},
		{"foot", []string{"--title", title}},
		{"wezterm", []string{"start", "--"}},
		{"tilix", []string{"-t", title, "-e"}},
		{"x-terminal-emulator", []string{"-e"}},
		{"xterm", []string{"-T", title, "-e"}},
	} {
		if have(t.name) {
			return t.name, t.args
		}
	}
	return "", nil
}

// ---------- downloads: HTTPS only, only to the makers' own addresses ----------

var driverClient = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !allowedDownload(req.URL) {
			return fmt.Errorf("refused a redirect to %s", req.URL.Host)
		}
		return nil
	},
}

// allowedDownload: HTTPS, to an address in the hints (a maker's download server or the AUR's search)
func allowedDownload(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return slices.Contains(driverHints().DownloadDomains, h) || h == "aur.archlinux.org"
}

func httpFetch(u string, max int64) ([]byte, error) {
	pu, err := url.Parse(u)
	if err != nil || !allowedDownload(pu) {
		return nil, fmt.Errorf("not an address Sakura Print downloads from: %s", u)
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "SakuraPrint/"+version+" (printer driver setup)")
	resp, err := driverClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", pu.Host, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("the download is much bigger than a driver should be")
	}
	return data, nil
}

// ---------- how this computer installs things ----------

// system: found by which tools exist, in this order
type system struct {
	Kind string `json:"kind"` // ostree | nixos | apt | dnf | zypper | pacman | pkcon | "" (none found)
	CUPS int    `json:"cups"` // CUPS major version, 0 = unknown
}

func detectSystem() system {
	s := system{CUPS: cupsMajor()}
	switch {
	case sys.exists("/run/ostree-booted"):
		s.Kind = "ostree"
	case sys.exists("/etc/NIXOS"):
		s.Kind = "nixos"
	case sys.have("apt-get") && sys.have("dpkg"):
		s.Kind = "apt"
	case sys.have("dnf"):
		s.Kind = "dnf"
	case sys.have("zypper"):
		s.Kind = "zypper"
	case sys.have("pacman"):
		s.Kind = "pacman"
	case sys.have("pkcon"):
		s.Kind = "pkcon"
	}
	return s
}

// cupsMajor asks the CUPS server which version it is (its web server says, like "CUPS/2.4 IPP/2.1")
func cupsMajor() int {
	if fs, ok := sys.(*fakeSystem); ok {
		return fs.CUPS
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://localhost:631/")
	if err != nil {
		return 0
	}
	resp.Body.Close()
	var major int
	if _, err := fmt.Sscanf(strings.TrimPrefix(resp.Header.Get("Server"), "CUPS/"), "%d", &major); err != nil {
		return 0
	}
	return major
}

// canInstall: Sakura Print can install packages here (NixOS installs from its configuration instead)
func (s system) canInstall() bool { return s.Kind != "" && s.Kind != "nixos" }

// ppdDrivers: classic drivers still work (CUPS 3 drops them)
func (s system) ppdDrivers() bool { return s.CUPS == 0 || s.CUPS < 3 }

// packageExists: the package manager knows this package
func (s system) packageExists(p string) bool {
	var out string
	var err error
	switch s.Kind {
	case "apt":
		out, err = sys.run("apt-cache", "show", "--no-all-versions", p)
		return err == nil && strings.Contains(out, "Package:")
	case "dnf", "ostree":
		if !sys.have("dnf") {
			return true // can't ask: try it
		}
		out, err = sys.run("dnf", "info", "-q", p)
		return err == nil && strings.Contains(out, "Name")
	case "zypper":
		out, err = sys.run("zypper", "--non-interactive", "--quiet", "info", p)
		return err == nil && strings.Contains(out, "Name") && !strings.Contains(out, "not found")
	case "pacman":
		out, err = sys.run("pacman", "-Si", p)
		return err == nil && strings.Contains(out, "Name")
	case "pkcon":
		out, err = sys.run("pkcon", "--plain", "resolve", p)
		return err == nil && (strings.Contains(out, "Available") || strings.Contains(out, "Installed"))
	}
	return false
}

// packageInstalled: the package is installed; its version, or ""
func (s system) packageInstalled(p string) string {
	switch s.Kind {
	case "apt":
		out, err := sys.run("dpkg-query", "-W", "-f=${Status}|${Version}", p)
		if err == nil && strings.HasPrefix(out, "install ok installed|") {
			return strings.TrimSpace(strings.TrimPrefix(out, "install ok installed|"))
		}
	case "dnf", "ostree", "zypper":
		out, err := sys.run("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", p)
		if err == nil && !strings.Contains(out, "not installed") {
			return strings.TrimSpace(out)
		}
	case "pacman":
		out, err := sys.run("pacman", "-Q", p)
		if f := strings.Fields(out); err == nil && len(f) == 2 {
			return f[1]
		}
	}
	return ""
}

// installPackages: the steps that install packages from the distribution
func (s system) installPackages(pkgs ...string) [][]string {
	switch s.Kind {
	case "apt":
		return [][]string{{"apt-get", "update"}, append([]string{"apt-get", "install", "-y"}, pkgs...)}
	case "dnf":
		return [][]string{append([]string{"dnf", "install", "-y"}, pkgs...)}
	case "zypper":
		return [][]string{append([]string{"zypper", "--non-interactive", "install"}, pkgs...)}
	case "pacman":
		return [][]string{append([]string{"pacman", "-S", "--needed", "--noconfirm"}, pkgs...)}
	case "ostree":
		return [][]string{append([]string{"rpm-ostree", "install", "-y", "--idempotent"}, pkgs...)}
	case "pkcon":
		return [][]string{append([]string{"pkcon", "install", "-y"}, pkgs...)}
	}
	return nil
}

// installFile: the steps that install a maker's package file (.deb or .rpm); nil if this computer can't directly
func (s system) installFile(path string) [][]string {
	deb := strings.HasSuffix(path, ".deb")
	switch {
	case s.Kind == "apt" && deb:
		return [][]string{{"apt-get", "install", "-y", path}}
	case s.Kind == "dnf" && !deb:
		return [][]string{{"dnf", "install", "-y", path}}
	case s.Kind == "zypper" && !deb:
		return [][]string{{"zypper", "--non-interactive", "install", "--allow-unsigned-rpm", path}}
	case s.Kind == "ostree" && !deb:
		return [][]string{{"rpm-ostree", "install", "-y", path}}
	case s.Kind == "pkcon":
		return [][]string{{"pkcon", "install-local", "-y", path}}
	}
	return nil
}

// fileKind: which kind of maker package this computer takes (Arch: either, it's converted)
func (s system) fileKind() string {
	switch s.Kind {
	case "apt", "pacman":
		return "deb"
	case "dnf", "zypper", "ostree":
		return "rpm"
	}
	return ""
}

// lib32: the steps and package that bring 32-bit programs' libraries (some makers' drivers are 32-bit)
func (s system) lib32() (steps [][]string, pkg string, err error) {
	pkg = driverHints().Lib32[s.Kind]
	switch s.Kind {
	case "apt":
		return [][]string{{"dpkg", "--add-architecture", "i386"}}, pkg, nil
	case "pacman":
		if !s.packageExists(pkg) {
			return nil, "", errors.New("this driver needs 32-bit libraries, which on Arch come from the multilib repository. Turn it on in /etc/pacman.conf (uncomment the [multilib] lines), run sudo pacman -Syu, then try again")
		}
	}
	return nil, pkg, nil
}

// needsRestart: installs only take effect after restarting the computer
func (s system) needsRestart() bool { return s.Kind == "ostree" }

// ---------- a pretend computer, for the tests ----------

// fakeSystem answers from a script and writes down what would have been run as the administrator. Programs it has
// no answer for, from a short list of harmless ones that only read files (unpacking a package to show its licence),
// really run.
type fakeSystem struct {
	mu        sync.Mutex
	Have      []string                     `json:"have"`
	Files     []string                     `json:"files"`
	CUPS      int                          `json:"cups"`
	Out       map[string]string            `json:"out"`       // command line → what it prints ("!" first = it fails)
	Fetch     map[string]string            `json:"fetch"`     // address → contents, or "@file" = that file's contents
	AfterRoot map[string]map[string]string `json:"afterRoot"` // answers that change once a step containing the key ran as the administrator
	RootLog   string                       `json:"rootLog"`   // where to write down administrator steps
	Opened    []string                     `json:"-"`
	Rooted    [][]string                   `json:"-"`
	dir       string
}

func loadFakeSystem(path string) (*fakeSystem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &fakeSystem{dir: filepath.Dir(path)}
	return f, json.Unmarshal(data, f)
}

var fakePassThrough = []string{"bsdtar", "dpkg-deb", "rpm2cpio", "cpio", "tar", "makepkg", "sha256sum"}

func (f *fakeSystem) run(name string, args ...string) (string, error) {
	line := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.mu.Lock()
	out, ok := f.Out[line]
	f.mu.Unlock()
	if ok {
		if strings.HasPrefix(out, "!") {
			return out[1:], errors.New("failed")
		}
		return out, nil
	}
	if slices.Contains(fakePassThrough, name) {
		return realSystem{}.run(name, args...)
	}
	return "", fmt.Errorf("pretend computer has no answer for: %s", line)
}

func (f *fakeSystem) root(steps [][]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Rooted = append(f.Rooted, steps...)
	if f.RootLog != "" {
		if fh, err := os.OpenFile(f.RootLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			for _, s := range steps {
				fmt.Fprintln(fh, strings.Join(s, " "))
			}
			fh.Close()
		}
	}
	var all strings.Builder
	for _, s := range steps {
		all.WriteString(strings.Join(s, " ") + "\n")
	}
	for trigger, outs := range f.AfterRoot {
		if strings.Contains(all.String(), trigger) {
			for k, v := range outs {
				f.Out[k] = v
			}
		}
	}
	return "ok", nil
}

func (f *fakeSystem) fetch(u string, max int64) ([]byte, error) {
	v, ok := f.Fetch[u]
	if !ok {
		return nil, errors.New("404 Not Found")
	}
	if strings.HasPrefix(v, "@") {
		p := v[1:]
		if !filepath.IsAbs(p) {
			p = filepath.Join(f.dir, p)
		}
		return os.ReadFile(p)
	}
	return []byte(v), nil
}

func (f *fakeSystem) have(name string) bool   { return slices.Contains(f.Have, name) }
func (f *fakeSystem) exists(path string) bool { return slices.Contains(f.Files, path) }

func (f *fakeSystem) terminal(title string, args ...string) (*osexecCmd, error) {
	f.root([][]string{append([]string{"(terminal)"}, args...)}) // written down like an install
	return startBackground("true"), nil
}

func (f *fakeSystem) open(u string) error {
	f.mu.Lock()
	f.Opened = append(f.Opened, u)
	f.mu.Unlock()
	return nil
}

// rootedLines: what the pretend computer was asked to do as the administrator (for tests)
func (f *fakeSystem) rootedLines() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b bytes.Buffer
	for _, s := range f.Rooted {
		b.WriteString(strings.Join(s, " ") + "\n")
	}
	return b.String()
}
