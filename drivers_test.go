// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// what `lpinfo -l -v` said for the real Brother DCP-T510W (plus Sakura Print's own printer, and empty backends)
const lpinfoBrother = `Device: uri = socket
        class = network
        info = AppSocket/HP JetDirect
        make-and-model = Unknown
        device-id =
        location =
Device: uri = dnssd://Brother%20DCP-T510W._ipp._tcp.local/?uuid=e3248000-80ce-11db-8000-105bad6f7645
        class = network
        info = Brother DCP-T510W
        make-and-model = Brother DCP-T510W
        device-id = MFG:Brother;MDL:DCP-T510W;CMD:HBP,PJL;
        location =
Device: uri = lpd://BRW105BAD6F7645/BINARY_P1
        class = network
        info = Brother DCP-T510W
        make-and-model = Brother DCP-T510W
        device-id = MFG:Brother;CMD:HBP,PJL;MDL:DCP-T510W;CLS:PRINTER;CID:Brother Generic Jpeg Type2;
        location =
Device: uri = ipp://Brother%20DCP-T510W._ipp._tcp.local/
        class = network
        info = Brother DCP-T510W
        make-and-model = Brother DCP-T510W
        device-id = MFG:Brother;MDL:DCP-T510W;CMD:PWGRaster,AppleRaster,URF,JPEG,PWG;
        location =
Device: uri = dnssd://Sakura%20Print%20(Brother%20DCP-T510W)._ipp._tcp.local/
        class = network
        info = Sakura Print (Brother DCP-T510W)
        make-and-model = Sakura Print (Brother DCP-T510W)
        device-id =
        location =
`

const infsBrother = `[DCP-T510W]
PRN_CUP_RPM=
PRN_CUP_DEB=
PRN_LPD_RPM=
PRN_LPD_DEB=
PRN_DRV_RPM=dcpt510wpdrv-1.0.1-1.i386.rpm
PRN_DRV_DEB=dcpt510wpdrv-1.0.1-0.i386.deb
REQUIRE32LIB=yes
PRINTERNAME=DCPT510W
SCANNER_DRV=brscan4
`

func TestParseDevices(t *testing.T) {
	ds := parseDevices(lpinfoBrother)
	if len(ds) != 1 {
		t.Fatalf("want one printer (not the empty backends, not Sakura Print's own), got %d: %+v", len(ds), ds)
	}
	d := ds[0]
	if d.Make != "Brother" || d.Model != "DCP-T510W" || d.Maker != "brother" || len(d.URIs) != 3 {
		t.Fatalf("got %+v", d)
	}
	if u := d.bestURI(false); !strings.HasPrefix(u, "dnssd://") {
		t.Errorf("a driver queue should use the name that survives a new address, got %s", u)
	}
	if u := d.bestURI(true); !strings.Contains(u, "._ipp") {
		t.Errorf("driverless needs IPP, got %s", u)
	}
	if !d.canDriverless() {
		t.Error("it speaks IPP Everywhere")
	}
	usb := device{URIs: []devURI{{URI: "usb://Brother/DCP-T510W?serial=1"}}}
	if usb.canDriverless() || usb.bestURI(false) == "" {
		t.Error("a USB-only printer: a driver queue yes, driverless no")
	}
}

func TestLocalPaper(t *testing.T) {
	dir := t.TempDir()
	paperFile, zoneTab = filepath.Join(dir, "none"), filepath.Join(dir, "zone.tab")
	os.WriteFile(zoneTab, []byte("# country\tcoords\tzone\nIN\t+2232+08822\tAsia/Kolkata\nUS\t+404251-0740023\tAmerica/New_York\nDE\t+5230+01322\tEurope/Berlin\n"), 0o644)
	for _, c := range []struct{ paper, tz, lang, want string }{
		{"", "Asia/Kolkata", "en_US.UTF-8", "A4"}, // the real test computer: India, with en_US
		{"", "America/New_York", "en_IN.UTF-8", "Letter"},
		{"", "Europe/Berlin", "", "A4"},
		{"en_US.UTF-8", "Asia/Kolkata", "", "Letter"},    // an explicit paper setting wins
		{"", "Nowhere/Unknown", "fr_CA.UTF-8", "Letter"}, // unknown zone: the language's country
		{"", "", "C", "A4"},
	} {
		t.Setenv("LC_PAPER", c.paper)
		t.Setenv("TZ", c.tz)
		t.Setenv("LANG", c.lang)
		t.Setenv("LC_ALL", "")
		localTime = filepath.Join(dir, "no-localtime")
		if got := localPaper(); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	os.WriteFile(paperFile, []byte("# Simply write the paper name.\n"), 0o644) // only a comment (like the real computer's): ignored
	t.Setenv("TZ", "America/New_York")
	if localPaper() != "Letter" {
		t.Error("a paper file with only a comment says nothing")
	}
	os.WriteFile(paperFile, []byte("# a comment\nletter\n"), 0o644)
	t.Setenv("TZ", "Asia/Kolkata")
	if localPaper() != "Letter" {
		t.Error("the system's paper file wins over the time zone")
	}
}

func TestMakersAndLists(t *testing.T) {
	for in, want := range map[string]string{"Brother": "brother", "Hewlett-Packard": "hp", "HP": "hp", "EPSON": "epson", "Konica Minolta": "konica", "Acme": ""} {
		if got, _ := makerOf(in); got != want {
			t.Errorf("makerOf(%q) = %q, want %q", in, got, want)
		}
	}
	if brotherModel("DCP-T510W") != "DCPT510W" || brotherModel("MFC-L2710DW series") != "MFCL2710DWSERIES" {
		t.Error("Brother's list names models without dashes, in capitals")
	}
	info := parseInfs(infsBrother)
	if f := makerFiles(info, "deb"); !slices.Equal(f, []string{"dcpt510wpdrv-1.0.1-0.i386.deb"}) {
		t.Errorf("deb: %v", f)
	}
	split := parseInfs("[HL-2270DW]\nPRN_LPD_DEB=hl2270dwlpr-2.1.0-1.i386.deb\nPRN_CUP_DEB=cupswrapperHL2270DW-2.0.4-2.i386.deb\n")
	if f := makerFiles(split, "deb"); !slices.Equal(f, []string{"hl2270dwlpr-2.1.0-1.i386.deb", "cupswrapperHL2270DW-2.0.4-2.i386.deb"}) {
		t.Errorf("older two-part models: printing part first, got %v", f)
	}
	if n, v := pkgFileVersion("dcpt510wpdrv-1.0.1-0.i386.deb"); n != "dcpt510wpdrv" || v != "1.0.1-0" {
		t.Errorf("got %s %s", n, v)
	}
	for _, c := range [][2]string{{"1.0.10-0", "1.0.9-0"}, {"1.0.1-1", "1.0.1-0"}, {"2.0", "1.9.9"}} {
		if !newerVersion(c[0], c[1]) || newerVersion(c[1], c[0]) {
			t.Errorf("%s should be newer than %s", c[0], c[1])
		}
	}
	if newerVersion("1.0.1-0", "1.0.1-0") {
		t.Error("the same version isn't newer")
	}
}

func TestRootScriptQuoting(t *testing.T) {
	script := rootScript([][]string{{"printf", "%s|", "a b", "it's", "$HOME", "`x`", ""}})
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if !strings.HasSuffix(string(out), "a b|it's|$HOME|`x`||") {
		t.Fatalf("arguments must reach the program exactly as given, got %q", out)
	}
}

// the queue script, run with stand-in CUPS commands that write down what they were asked
func TestQueueScript(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	stub := func(name, body string) {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho \""+name+" $*\" >> "+log+"\n"+body+"\n"), 0o755)
	}
	run := func(queues, drivers string, args ...string) (string, string, error) {
		os.Remove(log)
		stub("lpstat", `for q in `+queues+`; do [ "$2" = "$q" ] && { echo "device for $q: x"; exit 0; }; done; exit 1`)
		stub("lpadmin", "")
		stub("lpinfo", "printf '"+drivers+"'")
		step := queueScript(args[0], args[1], args[2], args[3], args[4], args[5])
		cmd := exec.Command(step[0], step[1:]...)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), readFile(log), err
	}
	id := "MFG:Brother;MDL:DCP-T510W;"
	drivers := `driverless:ipp://x Brother DCP-T510W, driverless\nBrother/brother_dcpt510w_printer_en.ppd Brother DCP-T510W CUPS\n`

	out, calls, err := run("", drivers, id, "Brother_DCP-T510W", "dnssd://b", "", "", "")
	if err != nil || !strings.Contains(calls, "lpadmin -p Brother_DCP-T510W -E -v dnssd://b -m Brother/brother_dcpt510w_printer_en.ppd") || !strings.Contains(out, "SAKURA-QUEUE=Brother_DCP-T510W") {
		t.Fatalf("picks the maker's driver, not driverless: %v %s %s", err, calls, out)
	}
	out, calls, _ = run("DCPT510W", drivers, id, "Brother_DCP-T510W", "dnssd://b", "", "", "DCPT510W")
	if !strings.Contains(calls, "lpadmin -p DCPT510W -E") || !strings.Contains(out, "SAKURA-QUEUE=DCPT510W") || strings.Contains(calls, "-x") {
		t.Fatalf("the queue Brother's script made is reused: %s %s", calls, out)
	}
	_, calls, _ = run("DCPT510W-Brother DCPT510W", drivers, id, "DCPT510W-Brother", "dnssd://b", "", "", "DCPT510W")
	if !strings.Contains(calls, "lpadmin -x DCPT510W") || !strings.Contains(calls, "lpadmin -p DCPT510W-Brother -E") {
		t.Fatalf("the printer already had a queue: that one gets the driver, the extra one goes: %s", calls)
	}
	out, _, err = run("", `driverless:ipp://x Brother, driverless\n`, id, "P", "ipp://b", "", "", "")
	if err == nil || !strings.Contains(out, "doesn't cover this printer") {
		t.Fatalf("no real driver fits: it must stop, not set up the wrong one: %v %s", err, out)
	}
	_, calls, _ = run("", "", id, "P", "ipp://b", "everywhere", "", "")
	if !strings.Contains(calls, "lpadmin -p P -E -v ipp://b -m everywhere") {
		t.Fatalf("driverless: %s", calls)
	}
	// the queue's settings come through a new driver, where it offers them (found for real: paper went to Letter)
	stub("lpoptions", `echo "PageSize/Media Size: *Letter A4 Legal"; echo "BRMonoColor/Color: Color *Mono"`)
	os.Remove(log)
	step := queueScript(id, "Q", "ipp://b", "everywhere", "", "", "PageSize=A4", "BRMonoColor=Mono", "Missing=Yes", "PageSize=Tabloid")
	cmd := exec.Command(step[0], step[1:]...)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	calls = readFile(log)
	if !strings.Contains(calls, "lpadmin -p Q -o PageSize=A4") || !strings.Contains(calls, "lpadmin -p Q -o BRMonoColor=Mono") ||
		strings.Contains(calls, "Missing") || strings.Contains(calls, "Tabloid") {
		t.Fatalf("keep what the driver offers, skip what it doesn't:\n%s", calls)
	}
	_, calls, _ = run("", "", id, "P", "ipp://b", "", "/home/me/Downloads/x.ppd", "")
	if !strings.Contains(calls, "lpadmin -p P -E -v ipp://b -P /home/me/Downloads/x.ppd") {
		t.Fatalf("a driver file: %s", calls)
	}
}

// makeDeb builds a small .deb like a maker's: files, a licence, install scripts
func makeDeb(t *testing.T, path, pkg, maintainer string, extra map[string]string) {
	t.Helper()
	tgz := func(files map[string]string) []byte {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		for name, body := range files {
			mode := int64(0o644)
			if strings.Contains(name, "inst") || strings.Contains(name, "rm") || strings.HasPrefix(name, "./opt") && strings.HasSuffix(name, "filter") {
				mode = 0o755
			}
			tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), ModTime: time.Unix(1700000000, 0), Typeflag: tar.TypeReg})
			tw.Write([]byte(body))
		}
		tw.Close()
		gz.Close()
		return b.Bytes()
	}
	control := tgz(map[string]string{
		"./control":  "Package: " + pkg + "\nVersion: 1.0.1-0\nMaintainer: " + maintainer + "\nArchitecture: i386\nDescription: Test Inkjet Printer Driver\n A test.\n",
		"./postinst": "#!/bin/sh\necho configured\n",
		"./prerm":    "#!/bin/sh\necho removing\n",
	})
	files := map[string]string{
		"./opt/brother/Printers/test/LICENSE_ENG.txt":   "Brother License Agreement\nYou may use this.\n",
		"./opt/brother/Printers/test/LICENSE_JPN.txt":   "使用許諾\n",
		"./opt/brother/Printers/test/lpd/filter_test":   "#!/bin/sh\ncat\n",
		"./opt/brother/Printers/test/cupswrapper/x.ppd": "*PPD-Adobe: \"4.3\"\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	data := tgz(files)
	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	member := func(name string, body []byte) {
		fmt.Fprintf(&ar, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, 1700000000, 0, 0, "100644", len(body))
		ar.Write(body)
		if len(body)%2 == 1 {
			ar.WriteByte('\n')
		}
	}
	member("debian-binary", []byte("2.0\n"))
	member("control.tar.gz", control)
	member("data.tar.gz", data)
	if err := os.WriteFile(path, ar.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnpackPackage(t *testing.T) {
	if !have("bsdtar") && !have("ar") {
		t.Skip("no bsdtar or ar")
	}
	dir := t.TempDir()
	deb := filepath.Join(dir, "testpdrv-1.0.1-0.i386.deb")
	makeDeb(t, deb, "testpdrv", "Brother Industries, Ltd.", nil)
	p, err := unpackPackage(deb, filepath.Join(dir, "u"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "testpdrv" || p.Version != "1.0.1-0" || p.Vendor != "Brother Industries, Ltd." || p.Scripts["postinst"] == "" {
		t.Fatalf("got %+v", p)
	}
	if len(p.Licences) != 1 || !strings.Contains(licenceText(p), "Brother License Agreement") {
		t.Fatalf("the English licence, not the Japanese copy too: %v", p.Licences)
	}
	_, brother := makerOf("Brother")
	_, canon := makerOf("Canon")
	if !vendorIs(p, brother) || vendorIs(p, canon) {
		t.Error("the package names Brother as its maker")
	}
}

func TestPacmanPackage(t *testing.T) {
	if !have("makepkg") || !have("bsdtar") || !have("fakeroot") {
		t.Skip("needs Arch's makepkg")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	deb := filepath.Join(dir, "testpdrv-1.0.1-0.i386.deb")
	makeDeb(t, deb, "testpdrv", "Brother Industries, Ltd.", nil)
	p, err := unpackPackage(deb, filepath.Join(dir, "u"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := pacmanPackage(p, dir, "lib32-glibc")
	if err != nil {
		t.Fatal(err)
	}
	list, _ := exec.Command("bsdtar", "-tf", built).Output()
	info, _ := exec.Command("bsdtar", "-xOf", built, ".PKGINFO").Output()
	hooks, _ := exec.Command("bsdtar", "-xOf", built, ".INSTALL").Output()
	for _, want := range []string{"opt/brother/Printers/test/lpd/filter_test", "usr/share/sakuraprint/driver-scripts/sakura-testpdrv/postinst"} {
		if !strings.Contains(string(list), want) {
			t.Errorf("the package should hold %s:\n%s", want, list)
		}
	}
	if !strings.Contains(string(info), "pkgname = sakura-testpdrv") || !strings.Contains(string(info), "pkgver = 1.0.1_0-1") || !strings.Contains(string(info), "depend = lib32-glibc") {
		t.Errorf("package info:\n%s", info)
	}
	if !strings.Contains(string(hooks), "sh /usr/share/sakuraprint/driver-scripts/sakura-testpdrv/postinst configure") ||
		!strings.Contains(string(hooks), "sakura-testpdrv/prerm remove") || !strings.Contains(string(hooks), "try-restart cups") {
		t.Errorf("pacman must run the maker's scripts like dpkg does:\n%s", hooks)
	}
	// and the real Brother driver, when there's a copy to try (SAKURA_TEST_BROTHER_DEB=path)
	if real := os.Getenv("SAKURA_TEST_BROTHER_DEB"); real != "" {
		rp, err := unpackPackage(real, filepath.Join(dir, "real"))
		if err != nil {
			t.Fatal(err)
		}
		rb, err := pacmanPackage(rp, filepath.Join(dir, "realbuild"), "lib32-glibc")
		if err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command("bsdtar", "-tf", rb).Output()
		if !strings.Contains(string(out), "cupswrapper/brother_dcpt510w_printer_en.ppd") {
			t.Errorf("real Brother package: %s", out)
		}
		t.Logf("real Brother driver packaged as %s", filepath.Base(rb))
	}
}

// fakeSys puts a pretend computer in place for one test
func fakeSys(t *testing.T, f *fakeSystem) *fakeSystem {
	t.Helper()
	old := sys
	if f.Out == nil {
		f.Out = map[string]string{}
	}
	sys = f
	t.Cleanup(func() { sys = old })
	infsMu.Lock()
	infsCache = map[string]struct {
		info map[string]string
		at   time.Time
	}{}
	infsMu.Unlock()
	withConfig(t, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	settingsMu.Lock()
	settings = Settings{}
	settingsMu.Unlock()
	return f
}

func routeOf(p drvPlan, id string) route {
	for _, r := range p.Routes {
		if r.ID == id {
			return r
		}
	}
	return route{}
}

func TestPlans(t *testing.T) {
	brother := parseDevices(lpinfoBrother)[0]

	// Debian with no driver yet: Brother's own server first
	fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg"}, CUPS: 2,
		Out: map[string]string{
			"apt-cache show --no-all-versions printer-driver-brlaser":      "Package: printer-driver-brlaser\n",
			"dpkg-query -W -f=${Status}|${Version} printer-driver-brlaser": "!",
		},
		Fetch: map[string]string{"https://download.brother.com/pub/com/linux/linux/infs/DCPT510W": infsBrother}})
	p := makePlan(brother)
	if p.Best != "maker" || routeOf(p, "installed").State != "skipped" || routeOf(p, "official").State != "skipped" {
		t.Fatalf("Debian, Brother: want Brother's server first, got %s: %+v", p.Best, p.Routes)
	}
	if r := routeOf(p, "maker"); !slices.Equal(r.Files, []string{"dcpt510wpdrv-1.0.1-0.i386.deb"}) {
		t.Errorf("the .deb for Debian: %+v", r)
	}
	if routeOf(p, "aur").State != "skipped" || routeOf(p, "open").State != "available" || routeOf(p, "driverless").State != "ready" {
		t.Errorf("AUR only on Arch; brlaser offered; driverless always there: %+v", p.Routes)
	}

	// Fedora with an HP: the package that says it drives this printer
	hp := device{Key: "hp", Make: "HP", Model: "DeskJet 2130 series", Name: "HP DeskJet 2130 series", DeviceID: "MFG:HP;MDL:DeskJet 2130 series;",
		URIs: []devURI{{URI: "ipp://hp.local/ipp/print"}}}
	fakeSys(t, &fakeSystem{Have: []string{"dnf"}, CUPS: 2, Out: map[string]string{
		"dnf repoquery -q --whatprovides postscriptdriver(hp;deskjet_2130_series;) --qf %{name}\n": "hplip\n",
		"dnf info -q hplip":                       "Name : hplip\n",
		"rpm -q --qf %{VERSION}-%{RELEASE} hplip": "!package hplip is not installed",
	}})
	p = makePlan(hp)
	if p.Best != "official" || !slices.Equal(routeOf(p, "official").Packages, []string{"hplip"}) || routeOf(p, "maker").State != "skipped" {
		t.Fatalf("Fedora, HP: hplip from Fedora, got %s %+v", p.Best, p.Routes)
	}

	// NixOS: nothing to install from here; driverless still works
	fakeSys(t, &fakeSystem{Files: []string{"/etc/NIXOS"}, Have: []string{"pacman"}, CUPS: 2})
	p = makePlan(brother)
	if p.Best != "driverless" || !strings.Contains(routeOf(p, "official").Why, "configuration") {
		t.Fatalf("NixOS: %s %+v", p.Best, p.Routes)
	}

	// CUPS 3: classic drivers are gone; a Printer Application first
	fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg", "snap"}, CUPS: 3})
	p = makePlan(brother)
	if p.Best != "app" || routeOf(p, "maker").State != "skipped" {
		t.Fatalf("CUPS 3: %s %+v", p.Best, p.Routes)
	}

	// Arch with yay: Brother's package (converted) first, the AUR offered with who made it
	fakeSys(t, &fakeSystem{Have: []string{"pacman", "yay"}, CUPS: 2, Fetch: map[string]string{
		"https://download.brother.com/pub/com/linux/linux/infs/DCPT510W": infsBrother,
		"https://aur.archlinux.org/rpc/v5/search/dcpt510w?by=name-desc":  `{"results":[{"Name":"brother-dcpt510w","Version":"1.0.1-0","Maintainer":"shahril","NumVotes":2,"License":["custom"]}]}`,
		"https://aur.archlinux.org/rpc/v5/search/dcp-t510w?by=name-desc": `{"results":[]}`,
	}})
	p = makePlan(brother)
	if p.Best != "maker" || !strings.Contains(routeOf(p, "maker").Detail, "pacman package") {
		t.Fatalf("Arch: %s %+v", p.Best, p.Routes)
	}
	if a := routeOf(p, "aur"); a.State != "available" || len(a.AUR) != 1 || a.AUR[0].Maintainer != "shahril" {
		t.Errorf("AUR: %+v", a)
	}
}

// the whole Brother route on a pretend Debian: download, licence, one password window with every step
func TestBrotherOnDebian(t *testing.T) {
	if !have("bsdtar") && !have("ar") {
		t.Skip("no bsdtar or ar")
	}
	dir := t.TempDir()
	deb := filepath.Join(dir, "dcpt510wpdrv-1.0.1-0.i386.deb")
	makeDeb(t, deb, "dcpt510wpdrv", "Brother Industries, Ltd.", nil)
	f := fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg"}, CUPS: 2, Out: map[string]string{"lpstat -v": ""},
		Fetch: map[string]string{
			"https://download.brother.com/pub/com/linux/linux/infs/DCPT510W":                          infsBrother,
			"https://download.brother.com/pub/com/linux/linux/packages/dcpt510wpdrv-1.0.1-0.i386.deb": "@" + deb,
		}})
	brother := parseDevices(lpinfoBrother)[0]
	p := makePlan(brother)
	j := startDriverJob(brother, routeOf(p, "maker"), "")
	var v drvJobView
	for i := 0; i < 100; i++ {
		if v = j.view(); v.State != "working" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if v.State != "asking" || v.Ask == nil || v.Ask.Kind != "licence" || !strings.Contains(v.Ask.Text, "Brother License Agreement") {
		t.Fatalf("Brother's licence first: %+v", v)
	}
	if strings.TrimSpace(f.rootedLines()) != "" {
		t.Fatal("nothing may be installed before the licence is agreed to")
	}
	j.answers <- jobAnswer{OK: true}
	for i := 0; i < 100; i++ {
		if v = j.view(); v.State == "done" || v.State == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if v.State != "done" {
		t.Fatalf("finished: %+v", v)
	}
	got := f.rootedLines()
	for _, want := range []string{"dpkg --add-architecture i386", "apt-get install -y libc6:i386", "apt-get install -y " + filepath.Join(driverCache(), "dcpt510wpdrv-1.0.1-0.i386.deb"), "sh -c"} {
		if !strings.Contains(got, want) {
			t.Errorf("one password window should do %q; it did:\n%s", want, got)
		}
	}
	if strings.Index(got, "libc6:i386") > strings.Index(got, "dcpt510wpdrv") {
		t.Error("the 32-bit libraries come before the driver that needs them")
	}
	settingsMu.Lock()
	lic, hash := settings.DriverLicences, settings.DriverHashes["dcpt510wpdrv-1.0.1-0.i386.deb"]
	settingsMu.Unlock()
	if len(lic) != 1 || lic[0].Maker != "Brother" || hash == "" {
		t.Errorf("the agreement and the file's fingerprint are written down: %+v %q", lic, hash)
	}

	// the same file name coming back different is refused
	makeDeb(t, deb, "dcpt510wpdrv", "Brother Industries, Ltd.", map[string]string{"./opt/brother/extra": "changed"})
	if _, err := downloadMakerFile("https://download.brother.com/pub/com/linux/linux/packages/dcpt510wpdrv-1.0.1-0.i386.deb", "dcpt510wpdrv-1.0.1-0.i386.deb"); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("a changed file must be refused: %v", err)
	}
}

func TestDownloadsOnlyFromMakers(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://download.brother.com/pub/x":          true,
		"http://download.brother.com/pub/x":           false,
		"https://evil.example/x":                      false,
		"https://aur.archlinux.org/rpc/v5/x":          true,
		"https://download.brother.com.evil.example/x": false,
	} {
		if _, err := httpFetch(u, 1); ok != (err == nil || !strings.Contains(err.Error(), "not an address")) {
			t.Errorf("%s: allowed=%v, err=%v", u, ok, err)
		}
	}
}

// the guided route: the maker's site opens, the driver lands in Downloads, it's checked and installed
func TestGuidedDownload(t *testing.T) {
	if !have("bsdtar") && !have("ar") {
		t.Skip("no bsdtar or ar")
	}
	canon := device{Key: "canon", Make: "Canon", Model: "TS3350", Name: "Canon TS3350", DeviceID: "MFG:Canon;MDL:TS3350;", URIs: []devURI{{URI: "ipp://canon.local/ipp/print"}}}
	for _, c := range []struct {
		maintainer string
		ok         bool
	}{{"Canon Inc.", true}, {"Totally Legit Drivers", false}} {
		f := fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg"}, CUPS: 2, Out: map[string]string{"lpstat -v": ""}})
		h := t.TempDir()
		t.Setenv("HOME", h) // never the real Downloads folder
		os.MkdirAll(filepath.Join(h, "Downloads"), 0o755)
		p := makePlan(canon)
		r := routeOf(p, "guided")
		if r.State != "available" || r.Site != "https://global.canon/en/support/" {
			t.Fatalf("guided: %+v", r)
		}
		j := startDriverJob(canon, r, "")
		time.Sleep(300 * time.Millisecond)
		if v := j.view(); v.Ask == nil || v.Ask.Kind != "download" || len(f.Opened) != 1 {
			t.Fatalf("the maker's site opens and it waits for the download: %+v %v", v, f.Opened)
		}
		makeDeb(t, filepath.Join(h, "Downloads", "cnijfilter2_6.60-1_amd64.deb"), "cnijfilter2", c.maintainer, nil)
		var v drvJobView
		for i := 0; i < 100; i++ {
			if v = j.view(); v.State == "failed" || (v.Ask != nil && v.Ask.Kind == "licence") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !c.ok {
			if v.State != "failed" || !strings.Contains(v.Error, "not Canon") || f.rootedLines() != "" {
				t.Fatalf("a package that isn't Canon's must be refused: %+v", v)
			}
			continue
		}
		if v.Ask == nil || v.Ask.Kind != "licence" {
			t.Fatalf("found in Downloads, licence next: %+v", v)
		}
		j.answers <- jobAnswer{OK: true}
		for i := 0; i < 100; i++ {
			if v = j.view(); v.State == "done" || v.State == "failed" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if v.State != "done" || !strings.Contains(f.rootedLines(), "apt-get install -y "+filepath.Join(h, "Downloads", "cnijfilter2_6.60-1_amd64.deb")) {
			t.Fatalf("installed with apt: %+v\n%s", v, f.rootedLines())
		}
	}
}

func TestHPPlugin(t *testing.T) {
	hplipModels = "tests/fixtures/drivers/models.dat"
	for model, want := range map[string]int{
		"HP LaserJet Professional P1102":   1, // by its section name
		"LaserJet Professional P1102":      1,
		"HP LaserJet Professional P 1102w": 1,
		"HP LaserJet 1022nw":               2, // optional
		"HP Deskjet Ink Advantage K109a":   0, // by its model1= line, and it needs none
		"HP DeskJet 2130 series":           0, // not in the list: no plugin
	} {
		if got := hpPluginNeed(model); got != want {
			t.Errorf("%s: plugin=%d, want %d", model, got, want)
		}
	}
	st := filepath.Join(t.TempDir(), "hplip.state")
	hplipState = st
	os.WriteFile(st, []byte("[plugin]\ninstalled = 0\n"), 0o644)
	if hpPluginInstalled() {
		t.Error("installed = 0")
	}
	os.WriteFile(st, []byte("[upgrade]\ninstalled = 1\n[plugin]\nversion = 3.26.4\ninstalled = 1\n"), 0o644)
	if !hpPluginInstalled() {
		t.Error("installed = 1 under [plugin]")
	}
}

func TestScannerPlans(t *testing.T) {
	brother := parseDevices(lpinfoBrother)[0]
	uscan := `=;wlo1;IPv4;Brother\032DCP-T510W;_uscan._tcp;local;BRW105BAD6F7645.local;192.168.0.156;80;"rs=eSCL" "ty=Brother DCP-T510W"`
	fetch := map[string]string{
		"https://download.brother.com/pub/com/linux/linux/infs/DCPT510W":    infsBrother,
		"https://download.brother.com/pub/com/linux/linux/infs/brscan4.lnk": "RPM32=brscan4-0.4.11-2.i386.rpm\nDEB32=brscan4-0.4.11-1.i386.deb\nRPM64=brscan4-0.4.11-2.x86_64.rpm\nDEB64=brscan4-0.4.11-1.amd64.deb\n",
	}

	// scanning works already: nothing else offered
	fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg"}, CUPS: 2, Fetch: fetch,
		Out: map[string]string{"scanimage -L": "device `brother4:net1;dev0' is a Brother DCP-T510W USB scanner\n"}})
	p := makePlan(brother)
	// scanning works: that's what's shown and best; the other ways stay listed (for driver updates), each with
	// its own state, exactly
	var got []string
	for _, r := range p.Scan {
		got = append(got, r.ID+":"+r.State)
	}
	if want := []string{"scan-ready:ready", "scan-maker:available", "scan-driverless:skipped", "scan-aur:skipped"}; !slices.Equal(got, want) || p.ScanBest != "scan-ready" {
		t.Fatalf("scanning works: got %v best %s, want %v best scan-ready", got, p.ScanBest, want)
	}

	// Fedora, nothing yet: Brother's scanner driver first, driverless scanning next
	fakeSys(t, &fakeSystem{Have: []string{"dnf"}, CUPS: 2, Fetch: fetch, Out: map[string]string{
		"avahi-browse -rtp --no-db-lookup _uscan._tcp":   uscan,
		"dnf info -q sane-airscan":                       "Name : sane-airscan\n",
		"rpm -q --qf %{VERSION}-%{RELEASE} sane-airscan": "!package sane-airscan is not installed",
	}})
	p = makePlan(brother)
	if p.ScanBest != "scan-maker" || !slices.Equal(routeOf(drvPlan{Routes: p.Scan}, "scan-maker").Files, []string{"brscan4-0.4.11-2.x86_64.rpm"}) {
		t.Fatalf("Fedora: Brother's 64-bit .rpm scanner driver: %+v", p.Scan)
	}
	if routeOf(drvPlan{Routes: p.Scan}, "scan-driverless").State != "available" {
		t.Errorf("the printer announces eSCL: sane-airscan offered: %+v", p.Scan)
	}

	// registering the scanner by a name that really resolves here, with Brother's tool (found on a real network:
	// the bare name didn't resolve, the ".local" one did)
	fakeSys(t, &fakeSystem{Have: []string{"apt-get", "dpkg"}, Out: map[string]string{
		"getent ahostsv4 BRW105BAD6F7645":       "!",
		"getent ahostsv4 BRW105BAD6F7645.local": "192.168.0.156   STREAM BRW105BAD6F7645.local\n",
	}})
	where := scannerAddress(brother)
	if where != "nodename=BRW105BAD6F7645.local" {
		t.Errorf("the name that resolves: %s", where)
	}
	step := registerScanner("brscan4", brother, where)
	if got := strings.Join(step[len(step)-5:], " "); got != "brsaneconfig4 brscan4 DCP-T510W DCP-T510W nodename=BRW105BAD6F7645.local" {
		t.Errorf("register: %s", got)
	}
	fakeSys(t, &fakeSystem{Out: map[string]string{
		"avahi-browse -rtp --no-db-lookup _ipp._tcp": `=;wlo1;IPv4;Brother\032DCP-T510W;_ipp._tcp;local;BRW105BAD6F7645.local;192.168.0.156;631;"ty=Brother DCP-T510W"`,
	}})
	if where := scannerAddress(brother); where != "ip=192.168.0.156" {
		t.Errorf("no name resolves: its address, from its announcement: %s", where)
	}
	if where := scannerAddress(device{URIs: []devURI{{URI: "socket://192.168.0.9:9100"}}}); where != "ip=192.168.0.9" {
		t.Errorf("or an address: %s", where)
	}
}

// Arch keeps files only under /usr: a maker's package with /usr/lib64 (like Brother's scanner driver) is moved
func TestPacmanPackageMovesLib64(t *testing.T) {
	if !have("makepkg") || !have("bsdtar") || !have("fakeroot") {
		t.Skip("needs Arch's makepkg")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	deb := filepath.Join(dir, "brscantest-0.4.11-1.amd64.deb")
	makeDeb(t, deb, "brscantest", "Brother Industries, Ltd.", map[string]string{"./usr/lib64/sane/libsane-brother4.so.1.0.7": "x", "./usr/bin/brsaneconfig4": "y"})
	p, _ := unpackPackage(deb, filepath.Join(dir, "u"))
	built, err := pacmanPackage(p, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	list, _ := exec.Command("bsdtar", "-tf", built).Output()
	if strings.Contains(string(list), "usr/lib64") || !strings.Contains(string(list), "usr/lib/sane/libsane-brother4.so.1.0.7") {
		t.Errorf("files must be under /usr/lib, not /usr/lib64:\n%s", list)
	}
	if real := os.Getenv("SAKURA_TEST_BROTHER_SCAN_DEB"); real != "" {
		rp, err := unpackPackage(real, filepath.Join(dir, "real"))
		if err != nil {
			t.Fatal(err)
		}
		rb, err := pacmanPackage(rp, filepath.Join(dir, "realbuild"), "")
		if err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command("bsdtar", "-tf", rb).Output()
		if strings.Contains(string(out), "usr/lib64") || !strings.Contains(string(out), "usr/lib/sane/libsane-brother4.so") {
			t.Errorf("real brscan4:\n%s", out)
		}
		t.Logf("real Brother scanner driver packaged as %s", filepath.Base(rb))
	}
}

// found testing on a real computer: the script's own text, echoed before it runs, must not be taken for its answer
func TestQueueNameFromRealOutput(t *testing.T) {
	step := queueScript("MFG:Brother;MDL:DCP-T510W;", "DCPT510W-Brother", "dnssd://x", "", "", "DCPT510W")
	script := rootScript([][]string{{"true"}, step})
	if strings.Count(script, "SAKURA-QUEUE=") != 1 {
		t.Fatalf("the echoed step must not contain the marker:\n%s", script)
	}
	bin := t.TempDir()
	for _, n := range []string{"lpstat", "lpadmin"} {
		os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	}
	os.WriteFile(filepath.Join(bin, "lpinfo"), []byte("#!/bin/sh\necho 'Brother/brother_dcpt510w_printer_en.ppd Brother DCP-T510W CUPS'\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "lpadmin"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	all := regexp.MustCompile(`(?m)^SAKURA-QUEUE=(\S+)$`).FindAllStringSubmatch(string(out), -1)
	if len(all) != 1 || all[0][1] != "DCPT510W-Brother" {
		t.Fatalf("want exactly the real answer, got %v in:\n%s", all, out)
	}
}

// found testing for real: a queue whose driver package was removed is "broken", not "set up with its driver"
func TestBrokenQueue(t *testing.T) {
	ppd := "*PPD-Adobe: \"4.3\"\n*cupsFilter: \"application/vnd.cups-pdf 0 brother_lpdwrapper_dcpt510w\"\n"
	fakeSys(t, &fakeSystem{Out: map[string]string{"ppd DCPT510W-Brother": ppd, "ppd Other": "*cupsFilter2: \"application/pdf application/vnd.cups-pdf 0 -\"\n"}})
	if s := queueDriverState(queueInfo{name: "DCPT510W-Brother", model: "Brother DCP-T510W CUPS"}); s != "broken" {
		t.Errorf("filter gone: %s", s)
	}
	fakeSys(t, &fakeSystem{Out: map[string]string{"ppd DCPT510W-Brother": ppd}, Files: []string{"/usr/lib/cups/filter/brother_lpdwrapper_dcpt510w"}})
	if s := queueDriverState(queueInfo{name: "DCPT510W-Brother", model: "Brother DCP-T510W CUPS"}); s != "driver" {
		t.Errorf("filter there: %s", s)
	}
	if s := queueDriverState(queueInfo{name: "X", model: "Brother DCP-T510W, driverless, cups-filters 2.0.1"}); s != "driverless" {
		t.Errorf("driverless: %s", s)
	}
}

// found testing for real: "the scanner shows up" must mean through the driver just installed, not the driverless one
func TestScanCheckWantsTheNewDriver(t *testing.T) {
	brother := parseDevices(lpinfoBrother)[0]
	j := &drvJob{answers: make(chan jobAnswer, 1)}
	fakeSys(t, &fakeSystem{Out: map[string]string{"scanimage -L": "device `airscan:e0:Brother DCP-T510W' is a eSCL Brother DCP-T510W ip=192.168.0.156\n"}})
	if _, err := scanCheck(j, brother, system{}, scanBackend("brscan4")); err == nil {
		t.Error("only the driverless scanner: Brother's driver didn't work")
	}
	if _, err := scanCheck(j, brother, system{}, "airscan:", "escl:"); err != nil {
		t.Error("driverless scanning route: the eSCL scanner counts")
	}
	fakeSys(t, &fakeSystem{Out: map[string]string{"scanimage -L": "device `brother4:net1;dev0' is a Brother DCP-T510W DCP-T510W\n"}})
	if sc, err := scanCheck(j, brother, system{}, scanBackend("brscan4")); err != nil || sc == "" {
		t.Errorf("Brother's scanner: %v", err)
	}
}
