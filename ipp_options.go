// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Print from any app, part 6: the printer's own options, for phones. Paper sizes (with borderless), paper types,
// quality, colour and both sides come from the real printer's driver file (its PPD, which CUPS serves for each
// queue), so a phone's print dialog offers what the printer can really do, and the phone's choices become the
// driver's own choices. Nothing here is specific to one maker: the driver's option names and choice names are
// matched by meaning (see docs/design/ipp.md).

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- reading a driver file (PPD) ----------

type ppdChoice struct{ Name, Text string }

type ppdOpt struct {
	Key, Label, Default string
	Choices             []ppdChoice
}

type ppdInfo struct {
	Order   []string           // option keys, in the file's order
	Options map[string]*ppdOpt // by key
	Paper   map[string][2]float64
	Area    map[string][4]float64
	PPM     int
}

var (
	ppdOpenUI  = regexp.MustCompile(`^\*OpenUI\s+\*([^/:\s]+)(?:/([^:]*))?:`)
	ppdDefault = regexp.MustCompile(`^\*Default([^:\s]+):\s*(\S+)`)
	ppdEntry   = regexp.MustCompile(`^\*([^%\s/:][^\s/:]*)\s+([^/:\s]+)(?:/([^:]*))?:\s*"?([^"]*)`)
)

func parsePPD(text string) ppdInfo {
	p := ppdInfo{Options: map[string]*ppdOpt{}, Paper: map[string][2]float64{}, Area: map[string][4]float64{}}
	nums := func(s string) []float64 {
		var out []float64
		for _, f := range strings.Fields(s) {
			if v, err := strconv.ParseFloat(f, 64); err == nil {
				out = append(out, v)
			}
		}
		return out
	}
	defaults := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(l, "*%") {
			continue // commented out
		}
		if m := ppdOpenUI.FindStringSubmatch(l); m != nil {
			if p.Options[m[1]] == nil {
				p.Options[m[1]] = &ppdOpt{Key: m[1], Label: strings.TrimSpace(m[2])}
				p.Order = append(p.Order, m[1])
			}
			continue
		}
		if v, ok := strings.CutPrefix(l, "*Throughput:"); ok {
			p.PPM, _ = strconv.Atoi(strings.Trim(strings.TrimSpace(v), `"`))
			continue
		}
		if m := ppdDefault.FindStringSubmatch(l); m != nil {
			defaults[m[1]] = m[2]
			continue
		}
		m := ppdEntry.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		key, name, txt, val := m[1], m[2], strings.TrimSpace(m[3]), m[4]
		switch key {
		case "PaperDimension":
			if n := nums(val); len(n) == 2 {
				p.Paper[name] = [2]float64{n[0], n[1]}
			}
		case "ImageableArea":
			if n := nums(val); len(n) == 4 {
				p.Area[name] = [4]float64{n[0], n[1], n[2], n[3]}
			}
		default:
			if o := p.Options[key]; o != nil && !slices.ContainsFunc(o.Choices, func(c ppdChoice) bool { return c.Name == name }) {
				o.Choices = append(o.Choices, ppdChoice{Name: name, Text: firstNonEmpty(txt, name)})
			}
		}
	}
	for k, v := range defaults {
		if o := p.Options[k]; o != nil {
			o.Default = v
		}
	}
	return p
}

// ---------- what the driver offers, in IPP's words ----------

type ippMedia struct {
	PWG                      string // IPP's name for the size, like iso_a4_210x297mm
	W, H                     int    // hundredths of a millimetre
	Top, Bottom, Left, Right int    // margins, hundredths of a millimetre
	Borderless               bool
	PPD                      string // the driver's own choice
}

type driverCaps struct {
	Media          []ippMedia
	DefaultMedia   string
	DefaultPPD     string            // the driver's default paper
	MediaTypeKey   string            // the driver's paper type option
	MediaTypes     map[string]string // IPP media-type keyword → the driver's choice
	DefaultType    string
	QualityKey     string
	Quality        map[int]string // 3 draft, 4 normal, 5 high → the driver's choice
	DefaultQuality int
	Colour         bool
	ColorKey       string
	ColorValue     string
	MonoValue      string
	DuplexKey      string // only when the printer prints both sides by itself
	DuplexLong     string
	DuplexShort    string
	DuplexOff      string
	PPM            int // pages per minute, if the driver says (*Throughput)
}

// standard sizes, hundredths of a millimetre, portrait
var pwgSizes = []struct {
	name string
	w, h int
}{
	{"iso_a3_297x420mm", 29700, 42000}, {"iso_a4_210x297mm", 21000, 29700}, {"iso_a5_148x210mm", 14800, 21000},
	{"iso_a6_105x148mm", 10500, 14800}, {"iso_b5_176x250mm", 17600, 25000}, {"jis_b5_182x257mm", 18200, 25700},
	{"jis_b6_128x182mm", 12800, 18200}, {"na_letter_8.5x11in", 21590, 27940}, {"na_legal_8.5x14in", 21590, 35560},
	{"na_executive_7.25x10.5in", 18415, 26670}, {"na_govt-letter_8x10in", 20320, 25400}, {"na_foolscap_8.5x13in", 21590, 33020},
	{"na_index-4x6_4x6in", 10160, 15240}, {"na_index-5x8_5x8in", 12700, 20320}, {"na_5x7_5x7in", 12700, 17780},
	{"oe_photo-l_3.5x5in", 8890, 12700}, {"na_index-3x5_3x5in", 7620, 12700}, {"jpn_hagaki_100x148mm", 10000, 14800},
	{"jpn_oufuku_148x200mm", 14800, 20000}, {"iso_dl_110x220mm", 11000, 22000}, {"iso_c5_162x229mm", 16200, 22900},
	{"iso_c6_114x162mm", 11400, 16200}, {"na_number-10_4.125x9.5in", 10478, 24130}, {"na_monarch_3.875x7.5in", 9843, 19050},
	{"jpn_you4_105x235mm", 10500, 23500}, {"jpn_chou3_120x235mm", 12000, 23500}, {"jpn_chou4_90x205mm", 9000, 20500},
	{"om_16k_195x270mm", 19500, 27000}, {"na_invoice_5.5x8.5in", 13970, 21590},
}

func ptToHmm(pt float64) int { return int(math.Round(pt * 2540 / 72)) }

// pwgName: IPP's name for a size (within a millimetre), or a self-describing custom name
func pwgName(ppdName string, w, h int) string {
	if w > h {
		w, h = h, w
	}
	for _, s := range pwgSizes {
		if abs(s.w-w) <= 100 && abs(s.h-h) <= 100 {
			return s.name
		}
	}
	n := strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(ppdName, "-"))
	num := func(v int) string { return strconv.FormatFloat(float64(v)/100, 'f', -1, 64) } // 215.9, never 215.90
	return fmt.Sprintf("custom_%s_%sx%smm", firstNonEmpty(strings.Trim(n, "-"), "paper"), num(w), num(h))
}

// what each paper type choice means: the IPP words it could be, best first
var mediaTypeRules = []struct {
	re  *regexp.Regexp
	kws []string
}{
	{regexp.MustCompile(`(?i)transparen|film|ohp`), []string{"transparency"}},
	{regexp.MustCompile(`(?i)envelope`), []string{"envelope"}},
	{regexp.MustCompile(`(?i)label`), []string{"labels"}},
	{regexp.MustCompile(`(?i)matte?\b|matt`), []string{"photographic-matte", "photographic"}},
	{regexp.MustCompile(`(?i)gloss`), []string{"photographic-glossy", "photographic"}},
	{regexp.MustCompile(`(?i)photo`), []string{"photographic"}},
	{regexp.MustCompile(`(?i)card|thick|heavy`), []string{"cardstock"}},
	{regexp.MustCompile(`(?i)inkjet|coated`), []string{"stationery-inkjet"}},
	{regexp.MustCompile(`(?i)plain|normal|standard|stationery`), []string{"stationery"}},
}

var qualityRules = []struct {
	re    *regexp.Regexp
	level int
}{
	{regexp.MustCompile(`(?i)draft|fast|econom|quick|toner.?sav`), 3},
	{regexp.MustCompile(`(?i)best|high|fine|photo|max|super`), 5},
	{regexp.MustCompile(`(?i)normal|standard|default|medium`), 4},
}

var qualityKeys = []string{"cupsPrintQuality", "print-quality", "OutputMode", "PrintQuality", "Quality", "BRResolution", "StpQuality", "EPIJ_Qual", "CNIJPrintQuality", "HPPrintQuality"}
var colorKeys = []string{"print-color-mode", "ColorModel", "BRMonoColor", "ColorMode", "CNIJColorMode", "EPIJ_Colo", "XRColorMode"}
var mediaTypeKeys = []string{"MediaType", "media-type", "BRMediaType", "EPIJ_Medi", "CNIJMediaType", "HPMediaType", "PaperType"}

func firstOption(p ppdInfo, keys []string, fallback *regexp.Regexp) *ppdOpt {
	for _, k := range keys {
		if o := p.Options[k]; o != nil && len(o.Choices) > 0 {
			return o
		}
	}
	for _, k := range p.Order {
		if fallback != nil && fallback.MatchString(k+" "+p.Options[k].Label) && len(p.Options[k].Choices) > 0 {
			return p.Options[k]
		}
	}
	return nil
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// driverCapsFrom works out what a driver offers. colour: the printer prints colour; duplex: it prints both sides
// by itself (else Sakura Print does both sides by hand).
func driverCapsFrom(p ppdInfo, colour, duplex bool) driverCaps {
	dc := driverCaps{Colour: colour, MediaTypes: map[string]string{}, Quality: map[int]string{}, PPM: p.PPM}

	// paper
	if ps := p.Options["PageSize"]; ps != nil {
		dc.DefaultPPD = ps.Default
		for _, c := range ps.Choices {
			dim, ok := p.Paper[c.Name]
			if !ok {
				continue
			}
			w, h := ptToHmm(dim[0]), ptToHmm(dim[1])
			m := ippMedia{PWG: pwgName(c.Name, w, h), W: min(w, h), H: max(w, h), PPD: c.Name}
			if a, ok := p.Area[c.Name]; ok {
				m.Left, m.Bottom = ptToHmm(a[0]), ptToHmm(a[1])
				m.Right, m.Top = ptToHmm(dim[0]-a[2]), ptToHmm(dim[1]-a[3])
				for _, v := range []*int{&m.Left, &m.Bottom, &m.Right, &m.Top} {
					if *v < 0 {
						*v = 0
					}
				}
			}
			m.Borderless = m.Top == 0 && m.Bottom == 0 && m.Left == 0 && m.Right == 0
			if dc.find(m.PWG, m.Borderless) == nil {
				dc.Media = append(dc.Media, m)
			}
			if c.Name == ps.Default {
				dc.DefaultMedia = m.PWG
			}
		}
	}
	if dc.DefaultMedia == "" && len(dc.Media) > 0 {
		dc.DefaultMedia = dc.Media[0].PWG
	}

	// paper types: each IPP word goes to the plainest choice that means it (shortest name first); a choice that
	// loses its first word can still take its second (another glossy photo paper becomes "photographic")
	if o := firstOption(p, mediaTypeKeys, regexp.MustCompile(`(?i)media.?type|paper.?type`)); o != nil {
		dc.MediaTypeKey = o.Key
		cs := slices.Clone(o.Choices)
		sort.SliceStable(cs, func(i, j int) bool { return len(cs[i].Name) < len(cs[j].Name) })
		for _, c := range cs {
			if regexp.MustCompile(`(?i)hagaki|disc|cd|dvd`).MatchString(c.Name + " " + c.Text) {
				continue // special media phones don't offer
			}
			for _, r := range mediaTypeRules {
				if !r.re.MatchString(c.Name + " " + c.Text) {
					continue
				}
				for _, kw := range r.kws {
					if _, taken := dc.MediaTypes[kw]; !taken {
						dc.MediaTypes[kw] = c.Name
						if c.Name == o.Default {
							dc.DefaultType = kw
						}
						break
					}
				}
				break
			}
		}
	}

	// quality: for each level, the choice most like the driver's default (Brother: PlainFast next to PlainNormal)
	if o := firstOption(p, qualityKeys, regexp.MustCompile(`(?i)quality`)); o != nil {
		cands := map[int][]string{}
		for _, c := range o.Choices {
			for _, r := range qualityRules {
				if r.re.MatchString(c.Name + " " + c.Text) {
					cands[r.level] = append(cands[r.level], c.Name)
					break
				}
			}
		}
		for level, cs := range cands {
			best := cs[0]
			for _, c := range cs[1:] {
				if commonPrefix(c, o.Default) > commonPrefix(best, o.Default) {
					best = c
				}
			}
			if slices.Contains(cs, o.Default) {
				best = o.Default
				dc.DefaultQuality = level
			}
			dc.Quality[level] = best
		}
		if len(dc.Quality) > 0 {
			dc.QualityKey = o.Key
		}
	}
	if dc.DefaultQuality == 0 {
		dc.DefaultQuality = 4
	}

	// colour
	if o := firstOption(p, colorKeys, nil); o != nil {
		for _, c := range o.Choices {
			t := c.Name + " " + c.Text
			switch {
			case dc.MonoValue == "" && regexp.MustCompile(`(?i)mono|gr[ae]y|black|kgray`).MatchString(t):
				dc.MonoValue = c.Name
			case dc.ColorValue == "" && regexp.MustCompile(`(?i)colou?r|rgb|cmyk`).MatchString(t):
				dc.ColorValue = c.Name
			}
		}
		if dc.MonoValue != "" || dc.ColorValue != "" {
			dc.ColorKey = o.Key
		}
	}

	// both sides by the printer itself
	if duplex {
		var opts []PPDOption
		for _, k := range p.Order {
			o := p.Options[k]
			po := PPDOption{Key: o.Key, Default: o.Default}
			for _, c := range o.Choices {
				po.Values = append(po.Values, c.Name)
			}
			opts = append(opts, po)
		}
		if key, on, off := duplexOption(opts); key != "" {
			dc.DuplexKey, dc.DuplexLong, dc.DuplexOff = key, on, off
			for _, c := range p.Options[key].Choices {
				if regexp.MustCompile(`(?i)^(duplex)?tumble$|short`).MatchString(c.Name) || (strings.Contains(strings.ToLower(c.Name), "tumble") && !duplexOnLong.MatchString(c.Name)) {
					dc.DuplexShort = c.Name
				}
			}
		}
	}
	return dc
}

// find a paper by IPP name, with or without borders
func (dc *driverCaps) find(pwg string, borderless bool) *ippMedia {
	for i := range dc.Media {
		if dc.Media[i].PWG == pwg && dc.Media[i].Borderless == borderless {
			return &dc.Media[i]
		}
	}
	return nil
}

// options: a phone's choices as the driver's own
func (dc driverCaps) options(sp ippSpec) map[string]string {
	o := map[string]string{}
	if m := dc.find(sp.media, sp.borderless); m != nil {
		o["PageSize"] = m.PPD
	} else if m := dc.find(sp.media, !sp.borderless); m != nil {
		o["PageSize"] = m.PPD
	} else if dc.DefaultPPD != "" {
		o["PageSize"] = dc.DefaultPPD
	}
	if v, ok := dc.MediaTypes[sp.mediaType]; ok && dc.MediaTypeKey != "" {
		o[dc.MediaTypeKey] = v
	}
	if v, ok := dc.Quality[sp.quality]; ok && dc.QualityKey != "" {
		o[dc.QualityKey] = v
	}
	switch {
	case dc.ColorKey == "":
	case sp.color == "monochrome" && dc.MonoValue != "":
		o[dc.ColorKey] = dc.MonoValue
	case sp.color == "color" && dc.ColorValue != "":
		o[dc.ColorKey] = dc.ColorValue
	}
	if dc.DuplexKey != "" {
		switch sp.sides {
		case "two-sided-long-edge":
			o[dc.DuplexKey] = dc.DuplexLong
		case "two-sided-short-edge":
			o[dc.DuplexKey] = firstNonEmpty(dc.DuplexShort, dc.DuplexLong)
		case "one-sided":
			if dc.DuplexOff != "" {
				o[dc.DuplexKey] = dc.DuplexOff
			}
		}
	}
	return o
}

// attrs: the printer's paper, paper types, quality, colour and both-sides attributes
func (dc driverCaps) attrs(o *ippOut) {
	media := dc.Media
	if len(media) == 0 { // no driver file: the common sizes
		for _, p := range ippPapers {
			media = append(media, ippMedia{PWG: p.pwg, W: p.w, H: p.h, Top: 300, Bottom: 300, Left: 300, Right: 300, PPD: p.ppd})
		}
	}
	def := firstNonEmpty(dc.DefaultMedia, media[0].PWG)
	var names []any
	for _, m := range media {
		if !slices.Contains(names, any(m.PWG)) {
			names = append(names, m.PWG)
		}
	}
	o.attr(vKeyword, "media-default", def)
	o.attr(vKeyword, "media-supported", names...)
	o.attr(vKeyword, "media-ready", def)
	o.attr(vKeyword, "media-source-supported", "auto")
	types := []any{"auto"}
	for _, r := range mediaTypeRules {
		for _, kw := range r.kws {
			if _, ok := dc.MediaTypes[kw]; ok && !slices.Contains(types, any(kw)) {
				types = append(types, kw)
			}
		}
	}
	if len(types) == 1 {
		types = append(types, "stationery")
	}
	o.attr(vKeyword, "media-type-supported", types...)
	o.attr(vKeyword, "media-type-default", firstNonEmpty(dc.DefaultType, "auto"))
	o.attr(vKeyword, "media-col-supported", "media-size", "media-source", "media-type", "media-top-margin", "media-bottom-margin", "media-left-margin", "media-right-margin")
	col := func(name string, m ippMedia) {
		o.collStart(name)
		o.memberColl("media-size")
		o.member("x-dimension", vInteger, m.W)
		o.member("y-dimension", vInteger, m.H)
		o.collEnd()
		o.member("media-top-margin", vInteger, m.Top)
		o.member("media-bottom-margin", vInteger, m.Bottom)
		o.member("media-left-margin", vInteger, m.Left)
		o.member("media-right-margin", vInteger, m.Right)
		o.member("media-source", vKeyword, "auto")
		o.member("media-type", vKeyword, firstNonEmpty(dc.DefaultType, "auto"))
		o.collEnd()
	}
	defMedia := media[0]
	for _, m := range media {
		if m.PWG == def && !m.Borderless {
			defMedia = m
		}
	}
	col("media-col-default", defMedia)
	col("media-col-ready", defMedia)
	for i, m := range media {
		n := ""
		if i == 0 {
			n = "media-col-database"
		}
		col(n, m)
	}
	for i, m := range media {
		n := ""
		if i == 0 {
			n = "media-size-supported"
		}
		o.collStart(n)
		o.member("x-dimension", vInteger, m.W)
		o.member("y-dimension", vInteger, m.H)
		o.collEnd()
	}
	margins := map[string][]any{}
	for _, m := range media {
		for k, v := range map[string]int{"media-top-margin-supported": m.Top, "media-bottom-margin-supported": m.Bottom, "media-left-margin-supported": m.Left, "media-right-margin-supported": m.Right} {
			if !slices.Contains(margins[k], any(v)) {
				margins[k] = append(margins[k], v)
			}
		}
	}
	for _, k := range []string{"media-top-margin-supported", "media-bottom-margin-supported", "media-left-margin-supported", "media-right-margin-supported"} {
		o.attr(vInteger, k, margins[k]...)
	}
	var qs []any
	for _, q := range []int{3, 4, 5} {
		if _, ok := dc.Quality[q]; ok || q == 4 {
			qs = append(qs, q)
		}
	}
	o.attr(vEnum, "print-quality-default", dc.DefaultQuality)
	o.attr(vEnum, "print-quality-supported", qs...)
	if dc.Colour {
		o.attr(vKeyword, "print-color-mode-default", "color")
		o.attr(vKeyword, "print-color-mode-supported", "auto", "color", "monochrome")
	} else {
		o.attr(vKeyword, "print-color-mode-default", "monochrome")
		o.attr(vKeyword, "print-color-mode-supported", "auto", "monochrome")
	}
	// both sides, either edge: by the printer itself, or by hand with Sakura Print's flip guide
	o.attr(vKeyword, "sides-default", "one-sided")
	o.attr(vKeyword, "sides-supported", "one-sided", "two-sided-long-edge", "two-sided-short-edge")
}

// specFromRequest: what the phone asked for, in Sakura's words
func specFromRequest(req *ippRequest, dc driverCaps) ippSpec {
	sp := ippSpec{format: req.str("document-format"), sides: req.str("sides"), color: req.str("print-color-mode"), media: req.str("media"),
		name: req.str("job-name"), copies: 1, mediaType: firstNonEmpty(req.str("media-col.media-type"), req.str("media-type"))}
	if n, ok := req.num("copies"); ok && n > 0 && n < 100 {
		sp.copies = n
	}
	if q, ok := req.num("print-quality"); ok && q >= 3 && q <= 5 {
		sp.quality = q
	}
	if sp.media == "" { // media-col: the paper from its size
		x, okx := req.num("media-col.media-size.x-dimension")
		y, oky := req.num("media-col.media-size.y-dimension")
		if okx && oky {
			if x > y {
				x, y = y, x
			}
			media := dc.Media
			if len(media) == 0 {
				for _, p := range ippPapers {
					media = append(media, ippMedia{PWG: p.pwg, W: p.w, H: p.h})
				}
			}
			for _, m := range media {
				if abs(m.W-x) < 200 && abs(m.H-y) < 200 {
					sp.media = m.PWG
					break
				}
			}
			if sp.media == "" {
				sp.media = pwgName("custom", x, y)
			}
		}
	}
	zero, seen := true, 0
	for _, k := range []string{"media-top-margin", "media-bottom-margin", "media-left-margin", "media-right-margin"} {
		if v, ok := req.num("media-col." + k); ok {
			seen++
			zero = zero && v == 0
		}
	}
	sp.borderless = seen == 4 && zero
	if a := req.get("page-ranges"); a != nil { // each value is a range: two numbers
		var rs []string
		for _, v := range a.values {
			if len(v) == 8 {
				from, to := int(int32(beUint32(v))), int(int32(beUint32(v[4:])))
				if from >= 1 && to >= from {
					rs = append(rs, fmt.Sprintf("%d-%d", from, to))
				}
			}
		}
		sp.pages = strings.Join(rs, ",")
	}
	return sp
}

func firstPositive(v ...int) int {
	for _, x := range v {
		if x > 0 {
			return x
		}
	}
	return 0
}

func beUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// ---------- a queue's driver file, from CUPS ----------

var (
	ppdCacheMu sync.Mutex
	ppdCache   = map[string]struct {
		text string
		at   time.Time
	}{}
)

// queuePPD: the driver file CUPS uses for a queue (the pretend computer answers `ppd QUEUE`)
func queuePPD(queue string) string {
	if f, ok := sys.(*fakeSystem); ok {
		out, _ := f.run("ppd", queue)
		return out
	}
	ppdCacheMu.Lock()
	c, ok := ppdCache[queue]
	ppdCacheMu.Unlock()
	if ok && time.Since(c.at) < 5*time.Minute {
		return c.text
	}
	cl := http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Get("http://localhost:631/printers/" + url.PathEscape(queue) + ".ppd")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	ppdCacheMu.Lock()
	ppdCache[queue] = struct {
		text string
		at   time.Time
	}{string(b), time.Now()}
	ppdCacheMu.Unlock()
	return string(b)
}

// driverCapsFor: what the queue's driver offers
func driverCapsFor(printer string) driverCaps {
	_, _, _, auto := autoDuplex(printer)
	return driverCapsFrom(parsePPD(queuePPD(printer)), capsFor(printer).Colour, auto)
}
