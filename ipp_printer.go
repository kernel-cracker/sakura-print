// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Print from any app, part 2: the printer Sakura Print pretends to be: its attributes, and answering each operation.

// ---------- the printer Sakura pretends to be ----------

type ippPaper struct {
	pwg  string // IPP media name
	w, h int    // hundredths of a millimetre
	ppd  string // the matching CUPS PageSize, roughly
}

var ippPapers = []ippPaper{
	{"iso_a4_210x297mm", 21000, 29700, "A4"},
	{"na_letter_8.5x11in", 21590, 27940, "Letter"},
	{"na_legal_8.5x14in", 21590, 35560, "Legal"},
	{"iso_a5_148x210mm", 14800, 21000, "A5"},
	{"na_index-4x6_4x6in", 10160, 15240, "PostC4x6"},
	{"na_5x7_5x7in", 12700, 17780, "Photo2L"},
	{"oe_photo-l_3.5x5in", 8900, 12700, "PhotoL"},
}

type ippJob struct {
	id      int
	name    string
	user    string
	state   int // 3 pending, 5 processing, 7 canceled, 8 aborted, 9 completed
	reason  string
	created time.Time
	job     *Job    // Sakura's own job, once printing
	spec    ippSpec // how the phone asked for it (Create-Job, until Send-Document brings the pages)
	held    bool    // from a phone that hasn't logged in with a PIN: waits until someone allows it
	mac     string  // the sending phone's hardware address ("" = unknown), to allow it for good
	peer    string  // and its IP address
	file    *File   // the pages, once they're here
	message string  // job-state-message: what the person has to do (turn the paper over)
}

const (
	heldMax  = 10            // prints from unknown phones waiting at once
	heldLife = 2 * time.Hour // then they're thrown away
)

type ippSpec struct {
	format, sides, color, media, name string
	pages                             string // "" = all, or like "1-3,5"
	copies                            int
	mediaType                         string // IPP paper type (stationery, photographic-glossy…)
	quality                           int    // 3 draft, 4 normal, 5 high (0: the driver's default)
	borderless                        bool
}

type ippServer struct {
	mu     sync.Mutex
	nextID int
	jobs   map[int]*ippJob
	work   string
	port   int
	up     time.Time
	uuid   string
}

var ippSrv *ippServer

// ippPrinter: the real printer AirPrint jobs go to (the favourite, or the computer's default)
func ippPrinter() string {
	ps, def := listPrinters()
	settingsMu.Lock()
	fav := settings.Printer
	settingsMu.Unlock()
	if fav != "" && validPrinter(fav) {
		return fav
	}
	if def != "" {
		return def
	}
	if len(ps) > 0 {
		return ps[0]
	}
	return ""
}

// airprintOn: printing from other apps is switched on, and phones may connect right now
func airprintOn() bool {
	settingsMu.Lock()
	off := settings.AirPrintOff
	settingsMu.Unlock()
	return !off && phonesAllowed() && ippPrinter() != ""
}

func (s *ippServer) printerName() string {
	p := ippPrinter()
	model := ""
	if p != "" {
		model = printerCache(attrsCache, p, time.Minute, readAttrs).get()["printer-make-and-model"]
	}
	model = strings.TrimSpace(regexp.MustCompile(`(?i)\s*(CUPS|- IPP Everywhere|, driverless.*|, using .*)$`).ReplaceAllString(model, ""))
	if model == "" {
		model = p
	}
	if model == "" {
		return "Sakura Print"
	}
	return "Sakura Print (" + model + ")"
}

func (s *ippServer) uri(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = "localhost:" + strconv.Itoa(s.port)
	}
	return "ipp://" + host + "/ipp/print"
}

// printerAttrs describes the printer (what phones ask before showing the print screen)
func (s *ippServer) printerAttrs(o *ippOut, r *http.Request) {
	printer := ippPrinter()
	caps := Caps{}
	if printer != "" {
		caps = capsFor(printer)
	}
	o.group(tagPrinter)
	o.attr(vURI, "printer-uri-supported", s.uri(r))
	o.attr(vKeyword, "uri-security-supported", "none")
	o.attr(vKeyword, "uri-authentication-supported", "none")
	o.attr(vName, "printer-name", s.printerName())
	o.attr(vText, "printer-info", s.printerName())
	o.attr(vText, "printer-location", "")
	o.attr(vText, "printer-make-and-model", s.printerName())
	o.attr(vURI, "printer-more-info", "http://"+r.Host+"/")
	o.attr(vURI, "printer-supply-info-uri", "http://"+r.Host+"/#/printer")
	o.attr(vURI, "printer-uuid", "urn:uuid:"+s.uuid)
	o.attr(vText, "printer-device-id", "MFG:Sakura Print;MDL:"+s.printerName()+";CMD:PDF,URF,JPEG;")
	s.mu.Lock()
	state, reason, message := s.printerState(airprintOn())
	s.mu.Unlock()
	o.attr(vEnum, "printer-state", state)
	o.attr(vKeyword, "printer-state-reasons", reason)
	o.attr(vText, "printer-state-message", message)
	o.attr(vBoolean, "printer-is-accepting-jobs", airprintOn())
	o.attr(vInteger, "printer-up-time", int(time.Since(s.up).Seconds())+1)
	o.attr(vInteger, "queued-job-count", s.activeJobs())
	o.attr(vKeyword, "ipp-versions-supported", "1.1", "2.0")
	o.attr(vKeyword, "ipp-features-supported", "airprint-1.7", "ipp-everywhere")
	o.attr(vEnum, "operations-supported", opPrintJob, opValidateJob, opCreateJob, opSendDocument, opCancelJob, opGetJobAttributes, opGetJobs,
		opGetPrinterAttribs, opCancelMyJobs, opCloseJob, opIdentifyPrinter)
	o.attr(vBoolean, "job-ids-supported", true)
	o.attr(vBoolean, "preferred-attributes-supported", false)
	o.attr(vInteger, "multiple-operation-time-out", 60)
	o.attr(vKeyword, "multiple-operation-time-out-action", "abort-job")
	o.attr(vKeyword, "overrides-supported", "none")
	o.attr(vBoolean, "page-ranges-supported", true)
	o.attr(vKeyword, "print-content-optimize-default", "auto")
	o.attr(vKeyword, "print-content-optimize-supported", "auto")
	o.attr(vKeyword, "print-rendering-intent-default", "auto")
	o.attr(vKeyword, "print-rendering-intent-supported", "auto")
	o.attr(vDateTime, "printer-config-change-date-time", s.up)
	o.attr(vInteger, "printer-config-change-time", 1)
	o.attr(vDateTime, "printer-state-change-date-time", s.up)
	o.attr(vInteger, "printer-state-change-time", 1)
	o.attr(vDateTime, "printer-current-time", time.Now())
	o.attr(vUnknown, "printer-geo-location", "")
	o.attr(vText, "printer-organization", "")
	o.attr(vText, "printer-organizational-unit", "")
	o.attr(vKeyword, "printer-get-attributes-supported", "document-format")
	o.attr(vURI, "printer-icons", "http://"+r.Host+"/icon-192.png", "http://"+r.Host+"/icon-512.png")
	s.supplies(o, printer)
	o.attr(vBoolean, "multiple-document-jobs-supported", false)
	o.attr(vCharset, "charset-configured", "utf-8")
	o.attr(vCharset, "charset-supported", "utf-8")
	o.attr(vLanguage, "natural-language-configured", "en")
	o.attr(vLanguage, "generated-natural-language-supported", "en")
	o.attr(vMimeType, "document-format-default", "application/pdf")
	o.attr(vMimeType, "document-format-supported", "application/pdf", "image/jpeg", "image/urf", "image/pwg-raster", "application/octet-stream")
	o.attr(vKeyword, "compression-supported", "none")
	o.attr(vKeyword, "pdl-override-supported", "attempted")
	o.attr(vBoolean, "color-supported", caps.Colour)
	o.attr(vInteger, "copies-default", 1)
	o.raw(vRange, "copies-supported", rangeOf(1, 99))
	o.attr(vEnum, "finishings-default", 3)
	o.attr(vEnum, "finishings-supported", 3)
	o.attr(vEnum, "orientation-requested-default", 3)
	o.attr(vEnum, "orientation-requested-supported", 3, 4, 5, 6)
	o.attr(vKeyword, "output-bin-default", "face-up")
	o.attr(vKeyword, "output-bin-supported", "face-up")
	o.raw(vResolution, "printer-resolution-default", resolution(300, 300))
	o.raw(vResolution, "printer-resolution-supported", resolution(300, 300))
	o.raw(vResolution, "pwg-raster-document-resolution-supported", resolution(300, 300))
	o.attr(vKeyword, "pwg-raster-document-type-supported", "sgray_8", "srgb_8")
	o.attr(vKeyword, "pwg-raster-document-sheet-back", "normal")
	urf := []string{"V1.4", "W8", "SRGB24", "CP1", "RS300", "DM1"}
	o.attr(vKeyword, "urf-supported", toAny(urf)...)
	o.attr(vKeyword, "printer-kind", "document", "photo")
	dc := driverCapsFor(printer)
	o.attr(vInteger, "pages-per-minute", firstPositive(dc.PPM, 10)) // the driver's rated speed, if it says
	if dc.Colour {
		o.attr(vInteger, "pages-per-minute-color", firstPositive(dc.PPM/2, 5))
	}
	dc.attrs(o) // paper sizes, paper types, quality, colour, both sides: from the printer's driver
	o.attr(vKeyword, "job-creation-attributes-supported", "copies", "media", "media-col", "page-ranges", "print-color-mode", "print-quality", "sides", "orientation-requested", "media-type")
	o.attr(vKeyword, "which-jobs-supported", "completed", "not-completed")
	o.attr(vKeyword, "identify-actions-default", "display")
	o.attr(vKeyword, "identify-actions-supported", "display")
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func (s *ippServer) activeJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, j := range s.jobs {
		s.refresh(j)
		if j.state < 7 {
			n++
		}
	}
	return n
}

// refresh follows Sakura's own job. Call with s.mu held.
func (s *ippServer) refresh(j *ippJob) {
	if j.job == nil {
		return
	}
	j.job.mu.Lock()
	j.job.refresh()
	st, e := j.job.State, j.job.Error
	j.job.mu.Unlock()
	switch st {
	case "done":
		j.state, j.reason = 9, "job-completed-successfully"
	case "error":
		j.state, j.reason = 8, "aborted-by-system"
		if e != "" {
			j.reason = "job-canceled-at-device"
		}
	case "canceled":
		j.state, j.reason = 7, "job-canceled-by-user"
	case "flip": // stopped until the paper is turned over: the phone's print queue shows what to do
		j.state, j.reason = 6, "printer-stopped"
		j.message = flipMessage
		return
	default:
		j.state, j.reason = 5, "job-printing"
	}
	j.message = ""
}

const flipMessage = "Turn the printed pages over and put them back as Sakura Print shows, then tap “Print the other side” in Sakura Print."

// printerState: what the printer says about itself; while a print waits for the paper to be turned over, it asks
// for it (media-needed), naming the print. Call with s.mu held.
func (s *ippServer) printerState(on bool) (state int, reason, message string) {
	if !on {
		return 5, "moving-to-paused", "Printing from other apps is switched off in Sakura Print"
	}
	for _, j := range s.jobs {
		if j.job == nil {
			continue
		}
		s.refresh(j)
		if j.message != "" {
			return 3, "media-needed", "“" + j.name + "”: " + flipMessage
		}
	}
	return 3, "none", ""
}

func (s *ippServer) jobAttrs(o *ippOut, r *http.Request, j *ippJob) {
	s.refresh(j)
	o.group(tagJob)
	o.attr(vInteger, "job-id", j.id)
	o.attr(vURI, "job-uri", s.uri(r)+"/"+strconv.Itoa(j.id))
	o.attr(vURI, "job-printer-uri", s.uri(r))
	o.attr(vName, "job-name", j.name)
	o.attr(vName, "job-originating-user-name", j.user)
	o.attr(vEnum, "job-state", j.state)
	o.attr(vKeyword, "job-state-reasons", j.reason)
	o.attr(vText, "job-state-message", j.message)
	o.attr(vInteger, "time-at-creation", int(j.created.Sub(s.up).Seconds())+1)
}
