// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Drivers: the plan: every way of getting a printer's driver, whether it can work on this computer, and why not.

// ---------- the plan: every route, whether it can work here, and why not ----------

type aurPkg struct {
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Votes      int     `json:"votes"`
	Maintainer string  `json:"maintainer"`
	Updated    int64   `json:"updated"`
	Licence    string  `json:"licence"`
	Popularity float64 `json:"popularity"`
}

type route struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	State    string   `json:"state"` // ready | available | skipped
	Why      string   `json:"why,omitempty"`
	Packages []string `json:"packages,omitempty"`
	PPD      string   `json:"ppd,omitempty"`
	Files    []string `json:"files,omitempty"`
	Site     string   `json:"site,omitempty"`
	AUR      []aurPkg `json:"aur,omitempty"`
	Restart  bool     `json:"restart,omitempty"`
}

type drvPlan struct {
	Device   device  `json:"device"`
	System   system  `json:"system"`
	Routes   []route `json:"routes"`
	Best     string  `json:"best"` // the first route that can work
	Scan     []route `json:"scan"` // the same for its scanner (empty: it doesn't scan, as far as anyone can tell)
	ScanBest string  `json:"scanBest"`
}

var kindNames = map[string]string{"apt": "Debian/Ubuntu", "dnf": "Fedora", "zypper": "openSUSE", "pacman": "Arch",
	"ostree": "Fedora Atomic", "pkcon": "your system", "nixos": "NixOS"}

func makePlan(d device) drvPlan {
	s := detectSystem()
	p := drvPlan{Device: d, System: s}
	_, h := makerOf(d.Make)
	makerName := firstNonEmpty(d.Make, "the maker")
	add := func(r route) { p.Routes = append(p.Routes, r) }
	skip := func(id, title, why string) { add(route{ID: id, Title: title, State: "skipped", Why: why}) }
	hints := driverHints()

	// 0. already here
	if !s.ppdDrivers() {
		skip("installed", "A driver already on this computer", "CUPS 3 doesn't use classic drivers")
	} else if ppds := installedDrivers(d); len(ppds) > 0 {
		add(route{ID: "installed", Title: "Use the driver that's already here", Detail: makerName + "'s driver is already on this computer.", State: "ready", PPD: ppds[0]})
	} else {
		skip("installed", "A driver already on this computer", "none installed fits this printer")
	}

	// 1. official, in the distribution
	official := func() route {
		t := "Official driver from " + kindNames[s.Kind]
		if !s.ppdDrivers() {
			return route{ID: "official", Title: t, State: "skipped", Why: "CUPS 3 doesn't use classic drivers"}
		}
		if !s.canInstall() {
			return route{ID: "official", Title: t, State: "skipped", Why: noInstallWhy(s)}
		}
		var cands []string
		cands = append(cands, packagesFor(s, d)...)
		if h != nil {
			cands = append(cands, h.Official[s.Kind]...)
		}
		if len(cands) == 0 && (h == nil || h.Lookup == nil) {
			cands = append(cands, hints.Any.Official[s.Kind]...)
		}
		var pkgs []string
		for _, c := range cands {
			if !slices.Contains(pkgs, c) && s.packageInstalled(c) == "" && s.packageExists(c) {
				pkgs = append(pkgs, c)
			}
		}
		if len(pkgs) == 0 {
			return route{ID: "official", Title: t, State: "skipped", Why: "no package from " + kindNames[s.Kind] + " for this printer (or it's installed and doesn't cover it)"}
		}
		return route{ID: "official", Title: t, Detail: "Installs " + strings.Join(pkgs, ", ") + ", signed by " + kindNames[s.Kind] + ".", State: "available", Packages: pkgs, Restart: s.needsRestart()}
	}()
	add(official)

	// 2. official, from the maker's own server
	func() {
		t := makerName + "'s driver, from " + makerName
		switch {
		case h == nil || h.Lookup == nil:
			skip("maker", t, "no lookup for this maker: see the maker's site below")
			return
		case !s.ppdDrivers():
			skip("maker", t, "CUPS 3 doesn't use classic drivers")
			return
		case !s.canInstall() || s.fileKind() == "":
			skip("maker", t, noInstallWhy(s))
			return
		case s.Kind == "pacman" && !h.ConvertForPacman:
			skip("maker", t, "this maker's packages aren't made for Arch")
			return
		}
		info, err := makerLookup(h, d.Model)
		if err != nil {
			skip("maker", t, "couldn't reach "+makerName+": "+err.Error())
			return
		}
		files := makerFiles(info, s.fileKind())
		if len(files) == 0 {
			skip("maker", t, makerName+" has no Linux driver for this model in its list")
			return
		}
		detail := "Downloads " + strings.Join(files, ", ") + " from " + makerName + "'s server, shows " + makerName + "'s licence, then installs it."
		if s.Kind == "pacman" {
			detail += " On Arch it's turned into a pacman package first."
		}
		add(route{ID: "maker", Title: t, Detail: detail, State: "available", Files: files, Restart: s.needsRestart()})
	}()

	// 3. official, guided download from the maker's site
	if h != nil && h.Site != "" && s.canInstall() && s.ppdDrivers() {
		add(route{ID: "guided", Title: "Download it from " + makerName + "'s website", State: "available", Site: h.Site,
			Detail: "Opens " + h.Site + " in the browser. Search for " + d.Model + ", pick Linux and download the driver: Sakura Print finds it in Downloads and installs it."})
	} else {
		skip("guided", "Download it from the maker's website", map[bool]string{true: "no website known for this maker", false: noInstallWhy(s)}[h == nil || h.Site == ""])
	}

	// 4. community: the AUR
	func() {
		t := "Community package (AUR)"
		helper := aurHelper()
		if s.Kind != "pacman" {
			skip("aur", t, "only on Arch-based systems")
			return
		}
		if helper == "" {
			skip("aur", t, "needs yay or paru")
			return
		}
		found, err := aurSearch(d.Model)
		if err != nil {
			skip("aur", t, "couldn't reach the AUR: "+err.Error())
			return
		}
		if len(found) == 0 {
			skip("aur", t, "nothing in the AUR for "+d.Model)
			return
		}
		add(route{ID: "aur", Title: t, State: "available", AUR: found,
			Detail: "Made by volunteers, not checked by anyone. Installs with " + helper + " in a terminal window, where you can read the recipe first."})
	}()

	// 5. open drivers
	func() {
		t := "Open driver (gutenprint and others)"
		if !s.ppdDrivers() || !s.canInstall() {
			skip("open", t, map[bool]string{true: "CUPS 3 doesn't use classic drivers", false: noInstallWhy(s)}[!s.ppdDrivers()])
			return
		}
		var cands []string
		if h != nil {
			cands = append(cands, h.Open[s.Kind]...)
		}
		cands = append(cands, hints.Any.Open[s.Kind]...)
		var pkgs []string
		for _, c := range cands {
			if !slices.Contains(pkgs, c) && s.packageInstalled(c) == "" && s.packageExists(c) {
				pkgs = append(pkgs, c)
			}
		}
		if len(pkgs) == 0 {
			skip("open", t, "all installed already, or none here")
			return
		}
		add(route{ID: "open", Title: t, State: "available", Packages: pkgs, Restart: s.needsRestart(),
			Detail: "Community drivers covering hundreds of printers: " + strings.Join(pkgs, ", ") + ". Not made by " + makerName + "."})
	}()

	// 6. Printer Application (the way drivers come with CUPS 3)
	func() {
		app := hints.Any.PrinterApp
		if h != nil && h.PrinterApp != "" {
			app = h.PrinterApp
		}
		t := "Printer Application (" + app + ")"
		switch {
		case app == "":
			skip("app", t, "none known")
		case !sys.have("snap"):
			skip("app", t, "needs snap")
		default:
			add(route{ID: "app", Title: t, State: "available", Packages: []string{app},
				Detail: "OpenPrinting's driver packaged as an app, the way drivers come with CUPS 3. Installs with snap."})
		}
	}()

	// 7. driverless
	if d.canDriverless() {
		add(route{ID: "driverless", Title: "Driverless (nothing to install)", State: "ready",
			Detail: "Works with nearly every printer from the last ten years. On small inkjets it can be slower and look a little softer than the maker's driver."})
	} else {
		skip("driverless", "Driverless", "this printer isn't reachable over IPP (USB printers need ipp-usb)")
	}

	// the best: CUPS 3 prefers a Printer Application; otherwise the first that can work
	for _, r := range p.Routes {
		if r.State != "skipped" {
			if !s.ppdDrivers() && r.ID == "driverless" {
				if slices.ContainsFunc(p.Routes, func(x route) bool { return x.ID == "app" && x.State == "available" }) {
					p.Best = "app"
					break
				}
			}
			p.Best = r.ID
			break
		}
	}
	p.Scan = makeScanPlan(d, s)
	for _, r := range p.Scan {
		if r.State != "skipped" {
			p.ScanBest = r.ID
			break
		}
	}
	return p
}

func noInstallWhy(s system) string {
	switch s.Kind {
	case "nixos":
		return "NixOS installs drivers from its configuration (services.printing.drivers)"
	case "":
		return "no package manager Sakura Print knows was found"
	}
	return "not on this computer"
}

// packagesFor: packages that say they drive this printer (Fedora and RHEL write it into their packages)
func packagesFor(s system, d device) []string {
	id := parseDeviceID(d.DeviceID)
	mk, md := strings.ToLower(firstNonEmpty(id["MFG"], d.Make)), strings.ToLower(firstNonEmpty(id["MDL"], d.Model))
	if mk == "" || md == "" {
		return nil
	}
	cap := fmt.Sprintf("postscriptdriver(%s;%s;)", strings.ReplaceAll(mk, " ", "_"), strings.ReplaceAll(md, " ", "_"))
	var out string
	switch {
	case (s.Kind == "dnf" || s.Kind == "ostree") && sys.have("dnf"):
		out, _ = sys.run("dnf", "repoquery", "-q", "--whatprovides", cap, "--qf", "%{name}\n")
	case sys.have("pkcon"):
		o, _ := sys.run("pkcon", "--plain", "what-provides", cap)
		for _, l := range strings.Split(o, "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && (f[0] == "Available" || f[0] == "Installed") {
				if m := regexp.MustCompile(`^(.+)-[^-]+-[^-]+\.[^.]+$`).FindStringSubmatch(f[1]); m != nil {
					out += m[1] + "\n"
				}
			}
		}
	}
	var pkgs []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.Contains(l, " ") && !slices.Contains(pkgs, l) {
			pkgs = append(pkgs, l)
		}
	}
	return pkgs
}

func aurHelper() string {
	for _, h := range []string{"yay", "paru"} {
		if sys.have(h) {
			return h
		}
	}
	return ""
}

// aurSearch asks the AUR's public search for packages mentioning the model
func aurSearch(model string) ([]aurPkg, error) {
	terms := []string{strings.ToLower(brotherModel(model))}
	if t := strings.ToLower(strings.ReplaceAll(model, " ", "-")); t != terms[0] {
		terms = append(terms, t)
	}
	var found []aurPkg
	for _, t := range terms {
		if len(t) < 3 {
			continue
		}
		data, err := sys.fetch("https://aur.archlinux.org/rpc/v5/search/"+t+"?by=name-desc", 4<<20)
		if err != nil {
			return nil, err
		}
		var r struct {
			Results []struct {
				Name, Version, Maintainer string
				NumVotes                  int
				Popularity                float64
				LastModified              int64
				License                   []string
			}
		}
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		for _, x := range r.Results {
			if slices.ContainsFunc(found, func(a aurPkg) bool { return a.Name == x.Name }) {
				continue
			}
			found = append(found, aurPkg{Name: x.Name, Version: x.Version, Votes: x.NumVotes, Maintainer: firstNonEmpty(x.Maintainer, "nobody (orphaned)"),
				Updated: x.LastModified, Licence: strings.Join(x.License, ", "), Popularity: x.Popularity})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Votes > found[j].Votes })
	if len(found) > 8 {
		found = found[:8]
	}
	return found, nil
}
