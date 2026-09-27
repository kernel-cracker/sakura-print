// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Drivers, part 4: scanners, and HP's extra plugin.
//
// Scanning: a printer that also scans gets the same treatment as printing. The maker's scanner driver (Brother's
// list names it: SCANNER_DRV=brscan4, then infs/brscan4.lnk names the files), registered with the maker's own tool
// by the printer's network name so it survives a new address; or driverless scanning (sane-airscan, when the
// printer announces eSCL/WSD scanning); or the AUR on Arch.
//
// HP: a few HP models need a closed part from HP on top of hplip. hplip's own model list says which (plugin=1
// needed, plugin=2 optional); HP's hp-plugin tool shows HP's licence and fetches it, in a terminal the person sees.

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ---------- does it scan, and does scanning work already ----------

// scannersNow: the scanners the computer sees (the pretend computer answers `scanimage -L` itself)
func scannersNow(force bool) []Scanner {
	if _, fake := sys.(*fakeSystem); fake {
		out, _ := sys.run("scanimage", "-L")
		return parseScanners(out)
	}
	return listScanners(force)
}

// scanWorks: a scanner for this printer is already there
func scanWorks(d device, force bool) string {
	return scannerFor(firstNonEmpty(d.Queue, d.Name), d.Name, scannersNow(force))
}

// announcesScanning: the printer says on the network that it scans driverless (eSCL, over _uscan / _uscans)
func announcesScanning(d device) bool {
	for _, svc := range []string{"_uscan._tcp", "_uscans._tcp"} {
		out, _ := sys.run("avahi-browse", "-rtp", "--no-db-lookup", svc)
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "=") && strings.Contains(modelKey(strings.ReplaceAll(l, `\032`, " ")), modelKey(d.Model)) && modelKey(d.Model) != "" {
				return true
			}
		}
	}
	return false
}

// scannerAddress: how the maker's scanner driver should find the printer: by a name that really resolves on this
// network (survives the router giving the printer a new address), else by its address. Found on a real network:
// printers' names often only resolve with ".local" (mDNS), which old drivers only understand spelled out.
func scannerAddress(d device) string {
	var names, ips []string
	for _, u := range d.URIs {
		pu, err := url.Parse(u.URI)
		if err != nil || pu.Hostname() == "" || strings.Contains(pu.Hostname(), "._") || pu.Scheme == "usb" || pu.Scheme == "dnssd" {
			continue
		}
		if net.ParseIP(pu.Hostname()) != nil {
			ips = append(ips, pu.Hostname())
		} else {
			names = append(names, strings.TrimSuffix(pu.Hostname(), "."))
		}
	}
	// what the printer's announcement says its name and address are
	out, _ := sys.run("avahi-browse", "-rtp", "--no-db-lookup", "_ipp._tcp")
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, ";")
		if len(f) > 7 && f[0] == "=" && f[2] == "IPv4" && modelKey(d.Model) != "" && strings.Contains(modelKey(strings.ReplaceAll(f[3], `\032`, " ")), modelKey(d.Model)) {
			names, ips = append(names, strings.TrimSuffix(f[6], ".")), append(ips, f[7])
		}
	}
	for _, n := range names {
		base := strings.TrimSuffix(n, ".local")
		for _, c := range []string{base, base + ".local"} {
			if o, err := sys.run("getent", "ahostsv4", c); err == nil && strings.TrimSpace(o) != "" {
				return "nodename=" + c
			}
		}
	}
	for _, ip := range ips {
		if net.ParseIP(ip).To4() != nil {
			return "ip=" + ip
		}
	}
	return ""
}

// ---------- the scanner's routes ----------

func makeScanPlan(d device, s system) []route {
	var rs []route
	add := func(r route) { rs = append(rs, r) }
	skip := func(id, title, why string) { add(route{ID: id, Title: title, State: "skipped", Why: why}) }
	if sc := scanWorks(d, false); sc != "" {
		add(route{ID: "scan-ready", Title: "Scanning works", Detail: "Found as " + sc + ".", State: "ready"}) // the others stay listed, for updates
	}
	_, h := makerOf(d.Make)
	maker := firstNonEmpty(d.Make, "the maker")

	// the maker's own scanner driver, from its list
	func() {
		t := maker + "'s scanner driver, from " + maker
		info, err := makerLookup(h, d.Model)
		switch {
		case h == nil || h.Lookup == nil:
			skip("scan-maker", t, "no lookup for this maker")
			return
		case !s.canInstall() || s.fileKind() == "":
			skip("scan-maker", t, noInstallWhy(s))
			return
		case err != nil:
			skip("scan-maker", t, "couldn't reach "+maker+": "+err.Error())
			return
		case info["SCANNER_DRV"] == "":
			skip("scan-maker", t, maker+"'s list has no scanner driver for this model (it may not scan)")
			return
		}
		file, err := scannerFile(h, info["SCANNER_DRV"], s)
		if err != nil || file == "" {
			skip("scan-maker", t, firstNonEmpty(errString(err), "no "+info["SCANNER_DRV"]+" for this kind of computer"))
			return
		}
		add(route{ID: "scan-maker", Title: t, State: "available", Files: []string{file}, Packages: []string{info["SCANNER_DRV"]}, Restart: s.needsRestart(),
			Detail: "Downloads " + file + " from " + maker + "'s server, shows the licence, installs it and connects it to this printer."})
	}()

	// driverless scanning
	func() {
		t := "Driverless scanning (sane-airscan)"
		switch {
		case !s.canInstall():
			skip("scan-driverless", t, noInstallWhy(s))
		case s.packageInstalled("sane-airscan") != "":
			skip("scan-driverless", t, "already installed, and it doesn't see this printer's scanner")
		case !announcesScanning(d):
			skip("scan-driverless", t, "the printer doesn't announce driverless scanning")
		case !s.packageExists("sane-airscan"):
			skip("scan-driverless", t, "sane-airscan isn't available here")
		default:
			add(route{ID: "scan-driverless", Title: t, State: "available", Packages: []string{"sane-airscan"}, Restart: s.needsRestart(),
				Detail: "Scans with the printer's own network scanning (eSCL). Installs sane-airscan from " + kindNames[s.Kind] + "."})
		}
	}()

	// the AUR
	func() {
		t := "Community scanner package (AUR)"
		info, _ := makerLookup(h, d.Model)
		drv := info["SCANNER_DRV"]
		switch {
		case s.Kind != "pacman":
			skip("scan-aur", t, "only on Arch-based systems")
			return
		case aurHelper() == "":
			skip("scan-aur", t, "needs yay or paru")
			return
		case drv == "":
			skip("scan-aur", t, "no scanner driver name to look for")
			return
		}
		found, err := aurSearchName(drv)
		if err != nil || len(found) == 0 {
			skip("scan-aur", t, "nothing in the AUR for "+drv)
			return
		}
		add(route{ID: "scan-aur", Title: t, State: "available", AUR: found, Packages: []string{drv},
			Detail: "Made by volunteers, not checked by anyone. Installs in a terminal window, where you can read the recipe first."})
	}()
	return rs
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// scannerFile: the maker's scanner package for this computer, from its list (infs/<driver>.lnk: DEB64=…, RPM32=…)
func scannerFile(h *makerHint, drv string, s system) (string, error) {
	if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(drv) {
		return "", errors.New("odd scanner driver name")
	}
	u := strings.ReplaceAll(h.Lookup.List, "{MODEL}", drv+".lnk")
	data, err := sys.fetch(u, 16<<10)
	if err != nil {
		return "", err
	}
	info := parseInfs(string(data))
	bits := map[string]string{"amd64": "64", "386": "32"}[runtime.GOARCH]
	if bits == "" {
		return "", errors.New(drv + " only runs on Intel/AMD computers")
	}
	return info[strings.ToUpper(s.fileKind())+bits], nil
}

// aurSearchName: AUR packages named exactly like this, or starting with it
func aurSearchName(name string) ([]aurPkg, error) {
	all, err := aurSearch(name)
	if err != nil {
		return nil, err
	}
	var out []aurPkg
	for _, a := range all {
		if a.Name == name || strings.HasPrefix(a.Name, name+"-") || strings.HasPrefix(a.Name, name) {
			out = append(out, a)
		}
	}
	return out, nil
}

// ---------- doing it ----------

func runScanRoute(j *drvJob, d device, r route, choice string) (string, error) {
	s := detectSystem()
	switch r.ID {
	case "scan-maker":
		return runScanMaker(j, d, s, r)
	case "scan-driverless":
		j.step("Installing sane-airscan: enter the computer's password in the window that opens")
		if _, err := sys.root(s.installPackages(r.Packages...)); err != nil {
			return "", err
		}
		return scanCheck(j, d, s, "airscan:", "escl:")
	case "scan-aur":
		if choice == "" && len(r.AUR) == 1 {
			choice = r.AUR[0].Name
		}
		if !strings.HasPrefix(choice, r.Packages[0]) {
			return "", errors.New("pick one of the packages found")
		}
		helper := aurHelper()
		j.step("Installing " + choice + " with " + helper + " in the terminal window. Read the recipe it shows, then answer its questions there")
		term, err := sys.terminal("Sakura Print: installing "+choice, helper, "-S", "--needed", choice)
		if err != nil {
			return "", fmt.Errorf("%v. Run this in a terminal yourself: %s -S %s", err, helper, choice)
		}
		for i := 0; i < 600 && term.running(); i++ {
			time.Sleep(3 * time.Second)
		}
		if where := scannerAddress(d); where != "" {
			j.step("Connecting the scanner to the printer: enter the computer's password in the window that opens")
			if _, err := sys.root([][]string{registerScanner(r.Packages[0], d, where)}); err != nil {
				return "", err
			}
		}
		return scanCheck(j, d, s, scanBackend(r.Packages[0]))
	}
	return "", errors.New("unknown way")
}

func runScanMaker(j *drvJob, d device, s system, r route) (string, error) {
	_, h := makerOf(d.Make)
	file, drv := r.Files[0], r.Packages[0]
	j.step("Downloading " + file + " from " + d.Make)
	p, err := downloadMakerFile(strings.ReplaceAll(h.Lookup.File, "{FILE}", file), file)
	if err != nil {
		return "", err
	}
	pi, err := unpackPackage(p, filepath.Join(driverCache(), "unpacked", "scanner"))
	if err != nil {
		return "", fmt.Errorf("couldn't open %s: %v", file, err)
	}
	if err := agreeLicence(j, d.Make, []*pkgInfo{pi}, h.Site); err != nil {
		return "", err
	}
	var steps [][]string
	if s.Kind == "pacman" {
		j.step("Packaging " + file + " for Arch")
		b, err := pacmanPackage(pi, filepath.Join(driverCache(), "build", "scanner"), "sane")
		if err != nil {
			return "", err
		}
		steps = [][]string{{"pacman", "-U", "--noconfirm", "--needed", b}}
	} else if steps = s.installFile(p); steps == nil {
		return "", errors.New("this computer can't install " + file)
	}
	if where := scannerAddress(d); where != "" {
		steps = append(steps, registerScanner(drv, d, where))
	}
	if s.needsRestart() {
		if _, err := sys.root(steps); err != nil {
			return "", err
		}
		return "", errors.New("installed: restart the computer, then open this again")
	}
	j.step("Installing " + d.Make + "'s scanner driver: enter the computer's password in the window that opens")
	if _, err := sys.root(steps); err != nil {
		return "", err
	}
	return scanCheck(j, d, s, scanBackend(drv))
}

// registerScanner: the step that tells the maker's scanner driver where the printer is, by its network name
// (survives a new address) or its address, with the maker's own tool (Brother: brsaneconfig4 -a name= model= nodename=)
func registerScanner(drv string, d device, where string) []string {
	n := strings.TrimLeft(strings.TrimPrefix(drv, "brscan"), "-")
	if !regexp.MustCompile(`^[0-9]*$`).MatchString(n) {
		n = ""
	}
	tool := "brsaneconfig" + n
	name := strings.Map(func(r rune) rune {
		if r == ' ' {
			return '-'
		}
		return r
	}, d.Model)
	return []string{"sh", "-c", `
tool=$(command -v "$1" || echo "/opt/brother/scanner/$2/$1")
"$tool" -r "$3" >/dev/null 2>&1 || true
"$tool" -a name="$3" model="$4" "$5"`, "sh", tool, drv, name, d.Model, where}
}

// scanBackend: the scanner name prefix a maker's driver gives its scanners (Brother's brscan4 → "brother4:")
func scanBackend(drv string) string {
	if n, ok := strings.CutPrefix(drv, "brscan"); ok && regexp.MustCompile(`^[0-9]*$`).MatchString(n) {
		return "brother" + n + ":"
	}
	return ""
}

// scanCheck: the scanner shows up now, through the driver just installed (not another one that was already there)
func scanCheck(j *drvJob, d device, s system, backends ...string) (string, error) {
	j.step("Looking for the scanner")
	for i := 0; i < 3; i++ {
		var mine []Scanner
		for _, sc := range scannersNow(true) {
			for _, b := range backends {
				if b == "" || strings.HasPrefix(sc.Device, b) {
					mine = append(mine, sc)
					break
				}
			}
		}
		if len(backends) == 0 {
			mine = scannersNow(true)
		}
		if sc := scannerFor(firstNonEmpty(d.Queue, d.Name), d.Name, mine); sc != "" {
			return sc, nil
		}
		time.Sleep(2 * time.Second)
	}
	return "", errors.New("it installed, but the scanner doesn't show up yet. Check the printer is on, then try Scan; if it still isn't there, try the next way")
}

// ---------- HP's plugin ----------

var hplipModels = "/usr/share/hplip/data/models/models.dat"
var hplipState = "/var/lib/hp/hplip.state"

// hpPluginNeed: 0 no plugin, 1 needed, 2 optional, from hplip's own model list
func hpPluginNeed(model string) int {
	f, err := os.Open(hplipModels)
	if err != nil {
		return 0
	}
	defer f.Close()
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")
		return strings.Trim(s, "_")
	}
	m := norm(model)
	m = strings.TrimPrefix(strings.TrimPrefix(m, "hewlett_packard_"), "hp_")
	want := map[string]bool{"hp_" + m: true, "hp_" + strings.TrimSuffix(m, "_series"): true, "hp_" + m + "_series": true}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	match, plugin := false, 0
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(l, "["):
			if match {
				return plugin
			}
			match, plugin = want[norm(strings.Trim(l, "[]"))], 0
		case strings.HasPrefix(l, "plugin="):
			fmt.Sscanf(strings.TrimPrefix(l, "plugin="), "%d", &plugin)
		case regexp.MustCompile(`^model\d+=`).MatchString(l):
			v := norm(strings.SplitN(l, "=", 2)[1])
			v = strings.TrimSuffix(strings.TrimSuffix(v, "_printer"), "_series")
			if want["hp_"+strings.TrimPrefix(v, "hp_")] || want["hp_"+strings.TrimPrefix(v, "hp_")+"_series"] {
				match = true
			}
		}
	}
	if match {
		return plugin
	}
	return 0
}

// hpPluginInstalled: hplip writes down when its plugin is installed
func hpPluginInstalled() bool {
	data, err := os.ReadFile(hplipState)
	if err != nil {
		return false
	}
	in := false
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			in = l == "[plugin]"
		} else if in && strings.ReplaceAll(l, " ", "") == "installed=1" {
			return true
		}
	}
	return false
}

// hpPlugin offers HP's plugin after hplip, if this model needs it; returns a note for the finished screen
func hpPlugin(j *drvJob, d device) string {
	_, h := makerOf(d.Make)
	if h == nil || !h.HPPlugin {
		return ""
	}
	need := hpPluginNeed(d.Model)
	if need == 0 || hpPluginInstalled() {
		return ""
	}
	text := "This printer needs a small closed-source part from HP (the \"plugin\") to print. HP's own tool, hp-plugin, shows HP's licence and downloads it, in a terminal window."
	if need == 2 {
		text = "HP offers an extra closed-source part (the \"plugin\") that adds features for this printer. It prints without it. HP's own tool, hp-plugin, shows HP's licence and downloads it, in a terminal window."
	}
	ans, err := j.ask(&jobAsk{Kind: "plugin", Title: "HP's plugin", Text: text})
	if err != nil || !ans.OK {
		if need == 1 {
			return "HP's plugin isn't installed, and this printer needs it: open Printers & drivers again to add it."
		}
		return ""
	}
	j.step("HP's plugin: follow hp-plugin in the terminal window (it asks for the computer's password there)")
	term, err := sys.terminal("Sakura Print: HP's plugin", "hp-plugin", "-i")
	if err != nil {
		return "Couldn't open a terminal: run  hp-plugin -i  yourself to add HP's plugin."
	}
	for i := 0; i < 400 && term.running(); i++ {
		time.Sleep(3 * time.Second)
	}
	if !hpPluginInstalled() && need == 1 {
		return "HP's plugin doesn't seem to be installed: run  hp-plugin -i  in a terminal."
	}
	return ""
}
