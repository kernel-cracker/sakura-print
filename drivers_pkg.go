// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Drivers, part 2: makers' package files. Brother's per-model driver list, unpacking a .deb/.rpm to read its
// licence and who made it, remembering each download's fingerprint, and (on Arch) turning a maker's .deb into a
// pacman package that runs the maker's own install scripts, like dpkg would.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ---------- Brother's per-model list ----------

// brotherModel: how Brother's list names a model ("DCP-T510W" → "DCPT510W")
func brotherModel(model string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, model))
}

// parseInfs reads Brother's list: "[DCP-T510W]" then KEY=value lines
func parseInfs(text string) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if k, v, ok := strings.Cut(l, "="); ok && !strings.HasPrefix(l, "#") {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// makerFiles: the package files a maker's list names for this kind of computer, in install order (the printing
// part first, then the CUPS part, for models that come in two)
func makerFiles(info map[string]string, kind string) []string {
	K := strings.ToUpper(kind)
	if f := info["PRN_DRV_"+K]; f != "" {
		return []string{f}
	}
	var out []string
	for _, k := range []string{"PRN_LPD_" + K, "PRN_CUP_" + K} {
		if f := info[k]; f != "" {
			out = append(out, f)
		}
	}
	return out
}

var pkgFileRe = regexp.MustCompile(`^(.+?)[-_](\d[\w.+~]*[-_][\w.+~]+?)[._](i386|i686|x86_64|amd64|noarch|all)\.(deb|rpm)$`)

// pkgFileVersion: "dcpt510wpdrv-1.0.1-0.i386.deb" → "dcpt510wpdrv", "1.0.1-0"
func pkgFileVersion(file string) (name, ver string) {
	if m := pkgFileRe.FindStringSubmatch(file); m != nil {
		return m[1], m[2]
	}
	return strings.TrimSuffix(strings.TrimSuffix(file, ".deb"), ".rpm"), ""
}

// newerVersion: is a newer than b? Numbers compared as numbers ("1.0.10" > "1.0.9")
func newerVersion(a, b string) bool {
	split := func(s string) []string {
		return regexp.MustCompile(`\d+|[A-Za-z]+`).FindAllString(s, -1)
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] == y[i] {
			continue
		}
		var nx, ny int
		_, ex := fmt.Sscanf(x[i], "%d", &nx)
		_, ey := fmt.Sscanf(y[i], "%d", &ny)
		if ex == nil && ey == nil {
			return nx > ny
		}
		return x[i] > y[i]
	}
	return len(x) > len(y)
}

var (
	infsMu    sync.Mutex
	infsCache = map[string]struct {
		info map[string]string
		at   time.Time
	}{}
)

// makerLookup fetches (and keeps for an hour) a maker's list for a model; nil if the maker has none for it
func makerLookup(h *makerHint, model string) (map[string]string, error) {
	if h == nil || h.Lookup == nil || h.Lookup.Kind != "brother-infs" {
		return nil, nil
	}
	u := strings.ReplaceAll(h.Lookup.List, "{MODEL}", brotherModel(model))
	infsMu.Lock()
	c, ok := infsCache[u]
	infsMu.Unlock()
	if ok && time.Since(c.at) < time.Hour {
		return c.info, nil
	}
	data, err := sys.fetch(u, 64<<10)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, nil // the maker has nothing for this model
		}
		return nil, err
	}
	info := parseInfs(string(data))
	if len(info) == 0 {
		return nil, nil
	}
	infsMu.Lock()
	infsCache[u] = struct {
		info map[string]string
		at   time.Time
	}{info, time.Now()}
	infsMu.Unlock()
	return info, nil
}

// ---------- downloads, remembered by fingerprint ----------

func driverCache() string { return filepath.Join(dataDir(), "drivers") }

// downloadMakerFile fetches a maker's package into the cache. The first time a file name is seen, its fingerprint
// is written down; the same name coming back different later is refused (makers publish no checksums).
func downloadMakerFile(u, file string) (string, error) {
	if strings.ContainsAny(file, "/\\") || file == "" || strings.HasPrefix(file, ".") {
		return "", errors.New("odd file name in the maker's list")
	}
	data, err := sys.fetch(u, 300<<20)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	settingsMu.Lock()
	known := settings.DriverHashes[file]
	if known == "" {
		if settings.DriverHashes == nil {
			settings.DriverHashes = map[string]string{}
		}
		settings.DriverHashes[file] = hash
		saveSettings()
	}
	settingsMu.Unlock()
	if known != "" && known != hash {
		return "", fmt.Errorf("%s isn't the same file as before (its fingerprint changed), so it wasn't used. If the maker really replaced it, remove it from driverHashes in the settings file", file)
	}
	os.MkdirAll(driverCache(), 0o700)
	p := filepath.Join(driverCache(), file)
	return p, os.WriteFile(p, data, 0o600)
}

// ---------- unpacking a package: its files, licence and maker ----------

type pkgInfo struct {
	Name, Version, Vendor, Description string
	Dir                                string            // its files, unpacked
	Scripts                            map[string]string // .deb install scripts: postinst, prerm, postrm
	Licences                           map[string]string // licence file → text
}

// unpackPackage unpacks a .deb or .rpm (without installing) to read it
func unpackPackage(file, dir string) (*pkgInfo, error) {
	os.RemoveAll(dir)
	data, ctl := filepath.Join(dir, "data"), filepath.Join(dir, "control")
	os.MkdirAll(data, 0o700)
	os.MkdirAll(ctl, 0o700)
	p := &pkgInfo{Dir: data, Scripts: map[string]string{}, Licences: map[string]string{}}
	switch {
	case strings.HasSuffix(file, ".deb"):
		// a .deb is an "ar" archive holding control.tar.* and data.tar.*
		outer := filepath.Join(dir, "outer")
		os.MkdirAll(outer, 0o700)
		if err := unpackWith(outer, file); err != nil {
			return nil, err
		}
		members, _ := os.ReadDir(outer)
		for _, m := range members {
			switch {
			case strings.HasPrefix(m.Name(), "control.tar"):
				if err := unpackWith(ctl, filepath.Join(outer, m.Name())); err != nil {
					return nil, err
				}
			case strings.HasPrefix(m.Name(), "data.tar"):
				if err := unpackWith(data, filepath.Join(outer, m.Name())); err != nil {
					return nil, err
				}
			}
		}
		fields := parseControl(readFile(filepath.Join(ctl, "control")))
		p.Name, p.Version, p.Vendor, p.Description = fields["Package"], fields["Version"], fields["Maintainer"], fields["Description"]
		for _, s := range []string{"postinst", "prerm", "postrm", "preinst"} {
			if b := readFile(filepath.Join(ctl, s)); b != "" {
				p.Scripts[s] = b
			}
		}
	case strings.HasSuffix(file, ".rpm"):
		if err := unpackWith(data, file); err != nil {
			return nil, err
		}
		if have("rpm") {
			if out, err := exec.Command("rpm", "-qp", "--nosignature", "--qf", "%{NAME}\n%{VERSION}-%{RELEASE}\n%{VENDOR}|%{PACKAGER}\n%{SUMMARY}", file).Output(); err == nil {
				l := strings.SplitN(string(out), "\n", 4)
				if len(l) == 4 {
					p.Name, p.Version, p.Vendor, p.Description = l[0], l[1], strings.Trim(strings.ReplaceAll(l[2], "(none)", ""), "|"), l[3]
				}
			}
		}
		if p.Name == "" {
			p.Name, p.Version = pkgFileVersion(filepath.Base(file))
		}
	default:
		return nil, errors.New("not a .deb or .rpm package")
	}
	p.Licences = findLicences(data)
	return p, nil
}

// unpackWith unpacks an archive (ar, tar.*, rpm) with whichever tool this computer has
func unpackWith(dir, file string) error {
	var cmds [][]string
	if have("bsdtar") { // libarchive: reads ar, rpm and every tar
		cmds = append(cmds, []string{"bsdtar", "-xf", file, "-C", dir})
	}
	switch {
	case strings.HasSuffix(file, ".deb") && have("ar"):
		cmds = append(cmds, []string{"sh", "-c", `cd "$1" && ar x "$2"`, "sh", dir, file})
	case strings.HasSuffix(file, ".rpm") && have("rpm2cpio") && have("cpio"):
		cmds = append(cmds, []string{"sh", "-c", `cd "$1" && rpm2cpio "$2" | cpio -idm --quiet`, "sh", dir, file})
	case strings.Contains(filepath.Base(file), ".tar") && have("tar"):
		cmds = append(cmds, []string{"tar", "-xf", file, "-C", dir})
	}
	if len(cmds) == 0 {
		return errors.New("no tool here can unpack it (install libarchive's bsdtar)")
	}
	var last error
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("%s: %s", c[0], strings.TrimSpace(string(out)))
	}
	return last
}

func readFile(p string) string { b, _ := os.ReadFile(p); return string(b) }

// parseControl reads a .deb's control file (continuation lines belong to the field before)
func parseControl(text string) map[string]string {
	m := map[string]string{}
	last := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, " ") && last != "" {
			m[last] += "\n" + strings.TrimSpace(l)
			continue
		}
		if k, v, ok := strings.Cut(l, ":"); ok {
			last = strings.TrimSpace(k)
			m[last] = strings.TrimSpace(v)
		}
	}
	return m
}

var licenceName = regexp.MustCompile(`(?i)(licen[cs]e|copying|eula|agreement)[^/]*$`)

// findLicences: the licence files a package brings (English first when there are several languages)
func findLicences(dir string) map[string]string {
	found := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !licenceName.MatchString(d.Name()) {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Size() < 512<<10 {
			rel, _ := filepath.Rel(dir, p)
			found["/"+rel] = strings.ToValidUTF8(readFile(p), "?")
		}
		return nil
	})
	// several languages of the same licence: keep the English one
	var eng []string
	for k := range found {
		if regexp.MustCompile(`(?i)(eng|_en|-en|\.en)[._-]?`).MatchString(filepath.Base(k)) {
			eng = append(eng, k)
		}
	}
	if len(eng) > 0 {
		for k := range found {
			if regexp.MustCompile(`(?i)(jpn|_ja|-ja|chn|_zh|kor|_ko)`).MatchString(filepath.Base(k)) {
				delete(found, k)
			}
		}
	}
	return found
}

// licenceText: all of a package's licences, one after another, for the licence screen
func licenceText(ps ...*pkgInfo) string {
	var b strings.Builder
	for _, p := range ps {
		keys := make([]string, 0, len(p.Licences))
		for k := range p.Licences {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "── %s (%s) ──\n\n%s\n\n", p.Name, filepath.Base(k), strings.TrimSpace(p.Licences[k]))
		}
	}
	return strings.TrimSpace(b.String())
}

// vendorIs: does the package say it's from this maker?
func vendorIs(p *pkgInfo, h *makerHint) bool {
	if p.Vendor == "" || h == nil {
		return false
	}
	v := strings.ToLower(p.Vendor)
	if h.Vendor != "" && strings.Contains(v, strings.ToLower(h.Vendor)) {
		return true
	}
	for _, n := range h.Names {
		if len(n) > 2 && strings.Contains(v, n) {
			return true
		}
	}
	return false
}

// ---------- Arch: a maker's .deb as a pacman package ----------

var pkgNameRe = regexp.MustCompile(`[^a-z0-9@._+-]`)

// pacmanPackage turns an unpacked .deb into a pacman package, built as the person (makepkg won't run as the
// administrator). The maker's install scripts run from pacman's hooks exactly like dpkg runs them.
func pacmanPackage(p *pkgInfo, work string, deps ...string) (string, error) {
	if !have("makepkg") {
		return "", errors.New("building the package needs makepkg (sudo pacman -S --needed base-devel)")
	}
	if runtime.GOARCH != "amd64" {
		return "", errors.New("this maker's driver only runs on Intel/AMD computers")
	}
	name := "sakura-" + pkgNameRe.ReplaceAllString(strings.ToLower(p.Name), "")
	ver := strings.NewReplacer("-", "_", ":", "_").Replace(p.Version)
	if ver == "" {
		ver = "0"
	}
	build := filepath.Join(work, "pkgbuild")
	os.RemoveAll(build)
	os.MkdirAll(build, 0o700)
	// the maker's scripts go into the package, and pacman's hooks call them like dpkg does
	scripts := "/usr/share/sakuraprint/driver-scripts/" + name
	var hooks strings.Builder
	call := func(hook, script, arg, extra string) {
		fmt.Fprintf(&hooks, "%s() {\n", hook)
		if _, ok := p.Scripts[script]; ok {
			fmt.Fprintf(&hooks, "  sh %s/%s %s || true\n", scripts, script, arg)
		}
		fmt.Fprintf(&hooks, "%s  true\n}\n\n", extra)
	}
	call("post_install", "postinst", "configure", "  systemctl try-restart cups.service 2>/dev/null || true\n")
	fmt.Fprintf(&hooks, "post_upgrade() {\n  true\n}\n\n") // like the AUR: a new version keeps the print queue as it is
	call("pre_remove", "prerm", "remove", "")
	call("post_remove", "postrm", "remove", "")
	if err := os.WriteFile(filepath.Join(build, "sakura.install"), []byte(hooks.String()), 0o644); err != nil {
		return "", err
	}
	desc := strings.ReplaceAll(strings.SplitN(p.Description+"\n", "\n", 2)[0], `"`, "'")
	depends := "'cups'"
	for _, d := range deps {
		if d != "" && regexp.MustCompile(`^[a-z0-9@._+-]+$`).MatchString(d) && !strings.Contains(depends, "'"+d+"'") {
			depends += " '" + d + "'"
		}
	}
	pkgbuild := fmt.Sprintf(`# Made by Sakura Print from the maker's own package %s %s
pkgname=%s
pkgver=%s
pkgrel=1
pkgdesc="%s (the maker's driver, packaged by Sakura Print)"
arch=('x86_64')
license=('LicenseRef-maker')
depends=(%s)
options=('!strip' '!debug' 'emptydirs')
install=sakura.install

package() {
  cp -a "$startdir/files/." "$pkgdir/"
  install -d "$pkgdir%s"
  cp -a "$startdir/scripts/." "$pkgdir%s/" 2>/dev/null || true
}
`, p.Name, p.Version, name, ver, desc, depends, scripts, scripts)
	if err := os.WriteFile(filepath.Join(build, "PKGBUILD"), []byte(pkgbuild), 0o644); err != nil {
		return "", err
	}
	if err := exec.Command("cp", "-a", p.Dir, filepath.Join(build, "files")).Run(); err != nil {
		return "", err
	}
	// Arch keeps everything under /usr/bin and /usr/lib (/lib64, /sbin… are links there, and pacman refuses
	// packages with files under a link): move such files where Arch keeps them
	files := filepath.Join(build, "files")
	for _, mv := range [][2]string{{"lib64", "usr/lib"}, {"usr/lib64", "usr/lib"}, {"lib", "usr/lib"}, {"bin", "usr/bin"}, {"sbin", "usr/bin"}, {"usr/sbin", "usr/bin"}} {
		from, to := filepath.Join(files, mv[0]), filepath.Join(files, mv[1])
		if fi, err := os.Lstat(from); err != nil || !fi.IsDir() {
			continue
		}
		os.MkdirAll(to, 0o755)
		if out, err := exec.Command("cp", "-a", from+"/.", to+"/").CombinedOutput(); err != nil {
			return "", fmt.Errorf("moving %s: %s", mv[0], out)
		}
		os.RemoveAll(from)
	}
	os.MkdirAll(filepath.Join(build, "scripts"), 0o755)
	for s, text := range p.Scripts {
		os.WriteFile(filepath.Join(build, "scripts", s), []byte(text), 0o755)
	}
	cmd := exec.Command("makepkg", "--force", "--nodeps", "--noconfirm")
	cmd.Dir = build
	cmd.Env = append(os.Environ(), "PKGEXT=.pkg.tar.zst", "LC_ALL=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return "", fmt.Errorf("building the package failed: %s", lines[len(lines)-1])
	}
	built, _ := filepath.Glob(filepath.Join(build, name+"-*.pkg.tar.zst"))
	if len(built) == 0 {
		return "", errors.New("building the package made nothing")
	}
	return built[0], nil
}
