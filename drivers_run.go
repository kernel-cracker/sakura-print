// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Drivers: doing it: each route as a job the screen follows, the licence screens, and setting up the print queue.

// ---------- doing it ----------

type jobAsk struct {
	Kind    string   `json:"kind"` // licence | terminal | download | confirm | app
	Title   string   `json:"title"`
	Text    string   `json:"text,omitempty"`
	Site    string   `json:"site,omitempty"`
	Choices []aurPkg `json:"choices,omitempty"`
}

type jobAnswer struct {
	OK     bool   `json:"ok"`
	Choice string `json:"choice"`
	File   string `json:"-"`
}

type drvJob struct {
	mu sync.Mutex
	drvJobView
	answers chan jobAnswer
}

// drvJobView: what the screen sees of an install
type drvJobView struct {
	ID      string   `json:"id"`
	Device  string   `json:"device"`
	Route   string   `json:"route"`
	State   string   `json:"state"` // working | asking | done | failed
	Step    string   `json:"step"`
	Log     []string `json:"log"`
	Ask     *jobAsk  `json:"ask,omitempty"`
	Error   string   `json:"error,omitempty"`
	Queue   string   `json:"queue,omitempty"` // the print queue, or for a scanner route the scanner found
	Restart bool     `json:"restart,omitempty"`
	Scanner bool     `json:"scanner,omitempty"` // a scanner route
	Note    string   `json:"note,omitempty"`    // something to say on the finished screen
}

var (
	drvJobsMu sync.Mutex
	drvJobs   = map[string]*drvJob{}
	drvBusy   sync.Mutex // one install at a time
)

func (j *drvJob) step(s string) {
	log.Printf("Printers & drivers: %s", s)
	j.mu.Lock()
	j.Step = s
	j.Log = append(j.Log, s)
	j.mu.Unlock()
}

func (j *drvJob) view() drvJobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := j.drvJobView
	v.Log = append([]string(nil), j.Log...)
	return v
}

// ask waits for the person (up to an hour)
func (j *drvJob) ask(a *jobAsk) (jobAnswer, error) {
	j.mu.Lock()
	j.State, j.Ask = "asking", a
	j.mu.Unlock()
	defer func() { j.mu.Lock(); j.State, j.Ask = "working", nil; j.mu.Unlock() }()
	select {
	case ans := <-j.answers:
		return ans, nil
	case <-time.After(time.Hour):
		return jobAnswer{}, errors.New("no answer for an hour, stopped")
	}
}

var errCancelled = errors.New("cancelled")

func startDriverJob(d device, r route, choice string) *drvJob {
	j := &drvJob{drvJobView: drvJobView{ID: newID(), Device: d.Key, Route: r.ID, State: "working", Scanner: strings.HasPrefix(r.ID, "scan-")}, answers: make(chan jobAnswer, 1)}
	drvJobsMu.Lock()
	drvJobs[j.ID] = j
	drvJobsMu.Unlock()
	go func() {
		drvBusy.Lock()
		defer drvBusy.Unlock()
		queue, err := runRoute(j, d, r, choice)
		note := ""
		if err == nil && (r.ID == "official" || r.ID == "open" || r.ID == "installed" || r.ID == "aur") {
			note = hpPlugin(j, d) // HP's plugin, for the models hplip says need it
		}
		j.mu.Lock()
		j.Note = note
		defer j.mu.Unlock()
		switch {
		case errors.Is(err, errCancelled):
			j.State, j.Error = "failed", "Stopped. Nothing more was installed."
		case err != nil:
			j.State, j.Error = "failed", err.Error()
		default:
			j.State, j.Queue, j.Restart = "done", queue, r.Restart
			j.Log = append(j.Log, "Ready: "+queue)
		}
	}()
	return j
}

func runRoute(j *drvJob, d device, r route, choice string) (string, error) {
	if strings.HasPrefix(r.ID, "scan-") {
		return runScanRoute(j, d, r, choice)
	}
	s := detectSystem()
	switch r.ID {
	case "installed":
		j.step("Setting up the printer with the driver that's here")
		return makeQueue(j, d, r.PPD, nil, "")
	case "official", "open":
		if r.Restart {
			j.step("Installing " + strings.Join(r.Packages, ", ") + " (active after the computer restarts)")
			if _, err := sys.root(s.installPackages(r.Packages...)); err != nil {
				return "", err
			}
			return "", errors.New("installed: restart the computer, then open this again to finish setting up the printer")
		}
		j.step("Installing " + strings.Join(r.Packages, ", ") + ": enter the computer's password in the window that opens")
		return makeQueue(j, d, "", s.installPackages(r.Packages...), "")
	case "maker":
		return runMaker(j, d, s)
	case "guided":
		return runGuided(j, d, s, r)
	case "aur":
		return runAUR(j, d, r, choice)
	case "app":
		return runApp(j, d, r)
	case "driverless":
		j.step("Setting up the printer without a driver")
		return makeQueue(j, d, "everywhere", nil, "")
	}
	return "", errors.New("unknown way")
}

// runMaker: the maker's own packages from its server (Brother), with the maker's licence
func runMaker(j *drvJob, d device, s system) (string, error) {
	_, h := makerOf(d.Make)
	info, err := makerLookup(h, d.Model)
	if err != nil || info == nil {
		return "", fmt.Errorf("couldn't get %s's list for %s", d.Make, d.Model)
	}
	files := makerFiles(info, s.fileKind())
	var pkgs []*pkgInfo
	var paths []string
	for i, f := range files {
		j.step("Downloading " + f + " from " + d.Make)
		p, err := downloadMakerFile(strings.ReplaceAll(h.Lookup.File, "{FILE}", f), f)
		if err != nil {
			return "", err
		}
		pi, err := unpackPackage(p, filepath.Join(driverCache(), "unpacked", fmt.Sprint(i)))
		if err != nil {
			return "", fmt.Errorf("couldn't open %s: %v", f, err)
		}
		pkgs, paths = append(pkgs, pi), append(paths, p)
	}
	if err := agreeLicence(j, d.Make, pkgs, h.Site); err != nil {
		return "", err
	}
	var steps [][]string
	lib32 := ""
	if strings.EqualFold(info["REQUIRE32LIB"], "yes") {
		pre, pkg, err := s.lib32()
		if err != nil {
			return "", err
		}
		steps, lib32 = append(steps, pre...), pkg
	}
	if s.Kind == "pacman" {
		var built []string
		for i, pi := range pkgs {
			j.step("Packaging " + filepath.Base(paths[i]) + " for Arch")
			b, err := pacmanPackage(pi, filepath.Join(driverCache(), "build", fmt.Sprint(i)), append([]string{lib32}, h.ArchDepends...)...)
			if err != nil {
				return "", err
			}
			built = append(built, b)
		}
		steps = append(steps, append([]string{"pacman", "-U", "--noconfirm", "--needed"}, built...))
	} else {
		if lib32 != "" {
			steps = append(steps, s.installPackages(lib32)...)
		}
		for _, p := range paths {
			st := s.installFile(p)
			if st == nil {
				return "", errors.New("this computer can't install " + filepath.Base(p))
			}
			steps = append(steps, st...)
		}
	}
	if s.needsRestart() {
		j.step("Installing " + d.Make + "'s driver (active after the computer restarts)")
		if _, err := sys.root(steps); err != nil {
			return "", err
		}
		return "", errors.New("installed: restart the computer, then open this again to finish setting up the printer")
	}
	j.step("Installing " + d.Make + "'s driver: enter the computer's password in the window that opens")
	// the maker's script may make its own queue for the printer (Brother's names it like DCPT510W): that one is used
	return makeQueue(j, d, "", steps, brotherModel(d.Model))
}

// agreeLicence shows the packages' licences and waits for "I agree"; the agreement is written down
func agreeLicence(j *drvJob, maker string, pkgs []*pkgInfo, site string) error {
	text := licenceText(pkgs...)
	if text == "" {
		text = "This package came without a licence file. " + maker + "'s terms for its drivers are on its website: " + site
	}
	ans, err := j.ask(&jobAsk{Kind: "licence", Title: maker + "'s licence", Text: text})
	if err != nil {
		return err
	}
	if !ans.OK {
		return errCancelled
	}
	settingsMu.Lock()
	for _, p := range pkgs {
		sum := sha(licenceText(p))
		settings.DriverLicences = append(settings.DriverLicences, LicenceRecord{Maker: maker, Package: p.Name + " " + p.Version, Licence: sum[:16], When: time.Now()})
	}
	saveSettings()
	settingsMu.Unlock()
	return nil
}

// LicenceRecord: a licence the person agreed to, for a maker's package
type LicenceRecord struct {
	Maker   string    `json:"maker"`
	Package string    `json:"package"`
	Licence string    `json:"licence"` // fingerprint of the licence text agreed to
	When    time.Time `json:"when"`
}

// runGuided: the maker's site opens, the person downloads the driver, it's found in Downloads and installed
func runGuided(j *drvJob, d device, s system, r route) (string, error) {
	_, h := makerOf(d.Make)
	start := time.Now()
	dl := userDir("DOWNLOAD", "Downloads")
	sys.open(r.Site)
	j.step("Waiting for the driver from " + d.Make + "'s website")
	found := make(chan string, 1)
	stop := make(chan struct{})
	go func() { // watch Downloads for a new driver file
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Second):
			}
			entries, _ := os.ReadDir(dl)
			for _, e := range entries {
				n := strings.ToLower(e.Name())
				info, err := e.Info()
				if err != nil || info.ModTime().Before(start) || strings.HasSuffix(n, ".part") || strings.HasSuffix(n, ".crdownload") {
					continue
				}
				if strings.HasSuffix(n, ".deb") || strings.HasSuffix(n, ".rpm") || strings.HasSuffix(n, ".ppd") || strings.HasSuffix(n, ".ppd.gz") {
					found <- filepath.Join(dl, e.Name())
					return
				}
			}
		}
	}()
	var file string
	j.mu.Lock()
	j.State, j.Ask = "asking", &jobAsk{Kind: "download", Title: "Download the driver from " + d.Make, Site: r.Site,
		Text: "On " + d.Make + "'s website, search for " + d.Model + ", choose Linux and download the driver (" + map[string]string{"deb": ".deb", "rpm": ".rpm"}[s.fileKind()] + " file, or a .ppd). Sakura Print picks it up from your Downloads folder by itself."}
	j.mu.Unlock()
	select {
	case file = <-found:
	case ans := <-j.answers:
		if !ans.OK {
			close(stop)
			return "", errCancelled
		}
		file = ans.File
	case <-time.After(time.Hour):
		close(stop)
		return "", errors.New("no driver arrived in Downloads within an hour")
	}
	close(stop)
	j.mu.Lock()
	j.State, j.Ask = "working", nil
	j.mu.Unlock()
	j.step("Found " + filepath.Base(file))
	lower := strings.ToLower(file)
	if strings.HasSuffix(lower, ".ppd") || strings.HasSuffix(lower, ".ppd.gz") {
		ans, err := j.ask(&jobAsk{Kind: "confirm", Title: "Use this driver file?", Text: filepath.Base(file) + " is a printer description file (PPD). Use it only if it came from " + d.Make + "'s website."})
		if err != nil || !ans.OK {
			return "", errCancelled
		}
		j.step("Setting up the printer with " + filepath.Base(file) + ": enter the computer's password in the window that opens")
		return makeQueue(j, d, "", nil, "", file)
	}
	pi, err := unpackPackage(file, filepath.Join(driverCache(), "unpacked", "guided"))
	if err != nil {
		return "", err
	}
	if !vendorIs(pi, h) {
		who := firstNonEmpty(pi.Vendor, "nobody")
		return "", fmt.Errorf("%s says it's made by %s, not %s, so it wasn't installed", filepath.Base(file), who, d.Make)
	}
	if err := agreeLicence(j, d.Make, []*pkgInfo{pi}, r.Site); err != nil {
		return "", err
	}
	var steps [][]string
	if s.Kind == "pacman" {
		if h == nil || !h.ConvertForPacman || !strings.HasSuffix(lower, ".deb") {
			return "", errors.New("on Arch, only makers whose packages Sakura Print knows how to convert can be installed this way: try the AUR")
		}
		b, err := pacmanPackage(pi, filepath.Join(driverCache(), "build", "guided"), h.ArchDepends...)
		if err != nil {
			return "", err
		}
		steps = [][]string{{"pacman", "-U", "--noconfirm", "--needed", b}}
	} else if steps = s.installFile(file); steps == nil {
		return "", errors.New("this computer can't install " + filepath.Base(file) + ": download the other kind (.deb for Debian/Ubuntu, .rpm for Fedora/openSUSE)")
	}
	j.step("Installing " + filepath.Base(file) + ": enter the computer's password in the window that opens")
	return makeQueue(j, d, "", steps, "")
}

// runAUR: the chosen package, installed by yay/paru in a terminal the person sees
func runAUR(j *drvJob, d device, r route, choice string) (string, error) {
	if choice == "" && len(r.AUR) == 1 {
		choice = r.AUR[0].Name
	}
	if !slices.ContainsFunc(r.AUR, func(a aurPkg) bool { return a.Name == choice }) {
		return "", errors.New("pick one of the packages found")
	}
	helper := aurHelper()
	j.step("Installing " + choice + " with " + helper + " in the terminal window. Read the recipe it shows, then answer its questions there")
	term, err := sys.terminal("Sakura Print: installing "+choice, helper, "-S", "--needed", choice)
	if err != nil {
		return "", fmt.Errorf("%v. Run this in a terminal yourself: %s -S %s, then open this again", err, helper, choice)
	}
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		if len(installedDrivers(d)) > 0 {
			break
		}
		if !term.running() {
			time.Sleep(2 * time.Second)
			if len(installedDrivers(d)) == 0 {
				return "", errors.New("the terminal closed without a driver for this printer being installed")
			}
		}
	}
	ppds := installedDrivers(d)
	if len(ppds) == 0 {
		return "", errors.New("no driver for this printer after 30 minutes")
	}
	j.step("Setting up the printer: enter the computer's password in the window that opens")
	return makeQueue(j, d, ppds[0], nil, brotherModel(d.Model))
}

// runApp: a Printer Application from the Snap Store; it shows up as a driverless printer
func runApp(j *drvJob, d device, r route) (string, error) {
	app := r.Packages[0]
	j.step("Installing " + app + ": enter the computer's password in the window that opens")
	if _, err := sys.root([][]string{{"snap", "install", app}}); err != nil {
		return "", err
	}
	for {
		j.step("Waiting for " + app + " to offer the printer")
		for i := 0; i < 20; i++ {
			for _, nd := range findDevices() {
				for _, u := range nd.URIs {
					if strings.Contains(u.URI, "localhost") && modelKey(nd.Model) != "" && strings.Contains(modelKey(nd.Name), modelKey(d.Model)) {
						j.step("Setting up the printer through " + app)
						return makeQueue(j, nd, "everywhere", nil, "")
					}
				}
			}
			time.Sleep(3 * time.Second)
		}
		ans, err := j.ask(&jobAsk{Kind: "app", Title: "Add the printer in " + app,
			Text: app + " is installed. Open it from your app menu (or its page in the browser), add your " + d.Name + " there, then tap Check again."})
		if err != nil || !ans.OK {
			return "", errCancelled
		}
	}
}

// makeQueue installs (if steps are given) and sets up the print queue, all behind one password window. ppd: a
// driver CUPS knows ("everywhere" for driverless), or "" to take whichever installed driver fits the printer; a
// driver file can be given instead. makerQueue: a queue the maker's install script may have made for this printer,
// which is reused.
func makeQueue(j *drvJob, d device, ppd string, steps [][]string, makerQueue string, ppdFile ...string) (string, error) {
	driverless := ppd == "everywhere"
	uri := d.bestURI(driverless)
	if uri == "" {
		return "", errors.New("couldn't find the printer on the network or USB: is it switched on?")
	}
	name := d.Queue
	if name == "" {
		name = strings.Trim(regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(d.Name, "_"), "_")
		if name == "" {
			name = "Printer"
		}
	}
	before := map[string]bool{}
	for _, q := range queues() {
		before[q.name] = true
	}
	queueStep := queueScript(d.DeviceID, name, uri, ppd, firstNonEmpty(ppdFile...), makerQueue, keepSettings(d.Queue)...)
	out, err := sys.root(append(steps, queueStep))
	if err != nil {
		if regexp.MustCompile(`(?m)^SAKURA: the installed driver doesn't cover`).MatchString(out) {
			return "", errors.New("it installed, but the driver it brought doesn't cover this printer: try the next way")
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		return "", fmt.Errorf("%v: %s", err, lines[len(lines)-1])
	}
	if all := regexp.MustCompile(`(?m)^SAKURA-QUEUE=(\S+)$`).FindAllStringSubmatch(out, -1); len(all) > 0 {
		name = all[len(all)-1][1] // the script's real answer, the last line saying it
	}
	rereadPrinters()
	settingsMu.Lock()
	if settings.Printer == "" || !before[settings.Printer] || settings.Printer == d.Queue {
		settings.Printer = name
	}
	delete(settings.Setup, name) // the setup questions again, for the new driver
	saveSettings()
	settingsMu.Unlock()
	return name, nil
}

// queueScript: the step that sets up the print queue. It runs as the administrator after the install, in the same
// shell, and asks CUPS which installed driver fits the printer, so it can only pick one that really does. A queue
// the maker's install script made for the printer is reused (or removed, if the printer already had one).
func queueScript(deviceID, name, uri, ppd, file, makerQueue string, keep ...string) []string {
	return append([]string{"sh", "-c", `
id="$1"; name="$2"; uri="$3"; ppd="$4"; file="$5"; maker="$6"
if [ -n "$maker" ] && [ "$maker" != "$name" ] && lpstat -v "$maker" >/dev/null 2>&1; then
  if [ -z "$(lpstat -v "$name" 2>/dev/null)" ]; then name="$maker"; else lpadmin -x "$maker"; fi
fi
if [ -n "$file" ]; then
  lpadmin -p "$name" -E -v "$uri" -P "$file"
else
  if [ -z "$ppd" ]; then
    ppd=$(lpinfo --device-id "$id" -m | grep -v -e '^driverless:' -e '^everywhere ' -e '^drv:///sample.drv' | head -n 1 | cut -d ' ' -f 1)
    [ -n "$ppd" ] || { echo "SAKURA: the installed driver doesn't cover this printer" >&2; exit 3; }
  fi
  lpadmin -p "$name" -E -v "$uri" -m "$ppd"
fi
shift 6
# settings to keep (the queue's defaults before, and the paper size used here), where the driver offers them:
# a new driver starts from its own defaults (found for real: Brother's is Letter)
for kv in "$@"; do
  opt=${kv%%=*}; val=${kv#*=}
  if lpoptions -p "$name" -l 2>/dev/null | grep "^$opt/" | cut -d: -f2- | tr ' ' '\n' | sed 's/^\*//' | grep -qxF "$val"; then
    lpadmin -p "$name" -o "$opt=$val" || true
  fi
done
echo "SAKURA-QUEUE=$name"`, "sh", deviceID, name, uri, ppd, file, makerQueue}, keep...)
}

// keepSettings: the queue's defaults now (to keep through a new driver), with the paper size used here first
func keepSettings(queue string) []string {
	keep := []string{"PageSize=" + localPaper()}
	if queue == "" {
		return keep
	}
	out, _ := sys.run("lpoptions", "-p", queue, "-l")
	for _, l := range strings.Split(out, "\n") {
		opt, rest, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		opt, _, _ = strings.Cut(opt, "/")
		for _, v := range strings.Fields(rest) {
			if strings.HasPrefix(v, "*") && regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(opt) {
				keep = append(keep, opt+"="+strings.TrimPrefix(v, "*"))
			}
		}
	}
	return keep
}

var (
	paperFile = "/etc/papersize"
	zoneTab   = "/usr/share/zoneinfo/zone.tab"
	localTime = "/etc/localtime"
)

// localPaper: Letter where that's the standard, A4 everywhere else. The language setting is a poor guide (lots of
// people outside the US use en_US: found on the real test computer, in India), so first an explicit paper setting,
// then the system's paper file, then the country of the time zone, and only then the language's country.
func localPaper() string {
	letter := func(country string) string {
		switch country {
		case "US", "CA", "MX", "PH", "CL", "CO", "VE", "PR", "GT", "CR", "SV", "PA", "DO", "NI", "BZ":
			return "Letter"
		}
		return "A4"
	}
	countryOf := func(loc string) string {
		if m := regexp.MustCompile(`_([A-Z]{2})`).FindStringSubmatch(loc); m != nil {
			return m[1]
		}
		return ""
	}
	if c := countryOf(os.Getenv("LC_PAPER")); c != "" {
		return letter(c)
	}
	if b, err := os.ReadFile(paperFile); err == nil {
		for _, l := range strings.Split(strings.ToLower(string(b)), "\n") {
			if f := strings.Fields(l); len(f) > 0 && !strings.HasPrefix(f[0], "#") {
				if f[0] == "letter" || f[0] == "legal" {
					return "Letter"
				}
				return "A4"
			}
		}
	}
	zone := os.Getenv("TZ")
	if zone == "" {
		if l, err := os.Readlink(localTime); err == nil {
			if _, z, ok := strings.Cut(l, "zoneinfo/"); ok {
				zone = z
			}
		}
	}
	if zone = strings.TrimPrefix(zone, ":"); zone != "" {
		if b, err := os.ReadFile(zoneTab); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if f := strings.Fields(l); len(f) >= 3 && f[2] == zone {
					return letter(f[0])
				}
			}
		}
	}
	return letter(countryOf(firstNonEmpty(os.Getenv("LC_ALL"), os.Getenv("LANG"))))
}
