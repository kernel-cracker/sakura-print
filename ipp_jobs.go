// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Print from any app, part 3: taking a job: who may print, holding prints from unknown phones, sending it through Sakura's pipeline.

// ---------- taking a job ----------

// ippHandle answers one IPP request (POST /ipp/print)
func (s *ippServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, s.printerName()+": add it as a printer on your phone (it shows up by itself on the WiFi).\n")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
	req, err := readIPP(r.Body)
	o := &ippOut{}
	if err != nil {
		o.head([2]byte{1, 1}, stBadRequest, 0)
		o.group(tagOperation)
		o.attr(vCharset, "attributes-charset", "utf-8")
		o.attr(vLanguage, "attributes-natural-language", "en")
		o.group(tagEnd)
		s.reply(w, o)
		return
	}
	status := uint16(stOK)
	var body func()
	switch {
	case req.version[0] != 1 && req.version[0] != 2:
		status, req.version = stVersionUnsupported, [2]byte{2, 0}
	case req.requestID == 0, len(req.attrs) < 2, req.attrs[0].name != "attributes-charset", req.attrs[1].name != "attributes-natural-language":
		status = stBadRequest // every request starts with these two, in this order
	case req.str("printer-uri") == "" && req.str("job-uri") == "":
		status = stBadRequest // and says which printer (or job) it's for
	}
	if status != stOK {
		req.op = 0 // answer with just the error
	}
	switch req.op {
	case opGetPrinterAttribs:
		body = func() { s.printerAttrs(o, r) }
	case opValidateJob:
		status = s.check(req)
	case opPrintJob:
		if status = s.check(req); status == stOK {
			var j *ippJob
			j, status = s.takeJob(r, req, true)
			if j != nil {
				body = func() { s.mu.Lock(); s.jobAttrs(o, r, j); s.mu.Unlock() }
			}
		}
	case opCreateJob:
		if status = s.check(req); status == stOK {
			var j *ippJob
			j, status = s.takeJob(r, req, false)
			if j != nil {
				body = func() { s.mu.Lock(); s.jobAttrs(o, r, j); s.mu.Unlock() }
			}
		}
	case opSendDocument:
		id, _ := req.num("job-id")
		mine := s.owner(r)
		s.mu.Lock()
		j := s.jobs[id]
		s.mu.Unlock()
		if j == nil {
			status = stNotFound
			break
		}
		if !mine(j) { // only the phone that made the job can put pages in it
			status = stNotAuthorized
			break
		}
		if req.str("document-format") != "" {
			j.spec.format = req.str("document-format")
		}
		if status = s.receive(j, req.data); status == stOK {
			body = func() { s.mu.Lock(); s.jobAttrs(o, r, j); s.mu.Unlock() }
		}
	case opGetJobAttributes:
		id, _ := req.num("job-id")
		if id == 0 { // job-uri form
			if u := req.str("job-uri"); u != "" {
				id, _ = strconv.Atoi(u[strings.LastIndex(u, "/")+1:])
			}
		}
		s.mu.Lock()
		j := s.jobs[id]
		s.mu.Unlock()
		if j == nil {
			status = stNotFound
		} else {
			body = func() { s.mu.Lock(); s.jobAttrs(o, r, j); s.mu.Unlock() }
		}
	case opGetJobs:
		which := req.str("which-jobs")
		var ids map[int]bool
		if a := req.get("job-ids"); a != nil {
			ids = map[int]bool{}
			for _, v := range a.values {
				if len(v) == 4 {
					ids[int(int32(binary.BigEndian.Uint32(v)))] = true
				}
			}
		}
		switch which {
		case "", "not-completed", "completed", "all":
		default:
			status = stAttrsNotSupported
		}
		if status != stOK {
			break
		}
		body = func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			var list []*ippJob
			for _, j := range s.jobs {
				s.refresh(j)
				done := j.state >= 7
				if (ids != nil && !ids[j.id]) || (ids == nil && ((which == "completed" && !done) || ((which == "" || which == "not-completed") && done))) {
					continue
				}
				list = append(list, j)
			}
			sort.Slice(list, func(a, b int) bool { return list[a].id > list[b].id })
			for _, j := range list {
				s.jobAttrs(o, r, j)
			}
		}
	case opCancelJob:
		id, _ := req.num("job-id")
		mine := s.owner(r)
		s.mu.Lock()
		j := s.jobs[id]
		switch {
		case j == nil:
			status = stNotFound
		case !mine(j):
			status = stNotAuthorized
		default:
			s.cancel(j)
		}
		s.mu.Unlock()
	case opCloseJob: // no more documents coming: each job here only ever has one
		id, _ := req.num("job-id")
		s.mu.Lock()
		if s.jobs[id] == nil {
			status = stNotFound
		}
		s.mu.Unlock()
	case opCancelMyJobs: // "mine": sent from this phone
		mine := s.owner(r)
		s.mu.Lock()
		for _, j := range s.jobs {
			if j.state < 7 && mine(j) {
				s.cancel(j)
			}
		}
		s.mu.Unlock()
	case opIdentifyPrinter:
	case 0: // the request itself was bad
	default:
		status = stOpUnsupported
	}
	o.head(req.version, status, req.requestID)
	o.group(tagOperation)
	o.attr(vCharset, "attributes-charset", "utf-8")
	o.attr(vLanguage, "attributes-natural-language", "en")
	if status != stOK {
		o.attr(vText, "status-message", ippMessage(status))
	}
	if body != nil {
		body()
	}
	o.group(tagEnd)
	s.reply(w, o)
}

// supplies: the ink levels the real printer reports, so the phone's print screen can show them
func (s *ippServer) supplies(o *ippOut, printer string) {
	var names []string
	var levels []int
	var colours []string
	if printer != "" {
		a := printerCache(attrsCache, printer, time.Minute, readAttrs).get()
		names, colours = splitList(a["marker-names"]), splitList(a["marker-colors"])
		for _, l := range splitList(a["marker-levels"]) {
			v, err := strconv.Atoi(l)
			if err != nil || v < 0 {
				v = -2 // unknown
			}
			levels = append(levels, v)
		}
	}
	if len(names) == 0 { // nothing reported: say so, rather than nothing at all
		names, levels, colours = []string{"Ink"}, []int{-2}, []string{"none"}
	}
	var supply, desc []any
	for i, n := range names {
		level, colour := -2, "none"
		if i < len(levels) {
			level = levels[i]
		}
		if i < len(colours) {
			colour = colourant(colours[i])
		}
		supply = append(supply, fmt.Sprintf("index=%d;type=ink;unit=percent;maxcapacity=100;level=%d;colorantname=%s;", i+1, level, colour))
		desc = append(desc, n)
	}
	o.attr(vOctets, "printer-supply", supply...)
	o.attr(vText, "printer-supply-description", desc...)
}

// pagesWithin keeps a page range to the pages the document has ("" = all)
func pagesWithin(rng string, n int) string {
	if rng == "" || n < 1 {
		return ""
	}
	var keep []string
	for _, r := range strings.Split(rng, ",") {
		a, b, _ := strings.Cut(r, "-")
		from, _ := strconv.Atoi(a)
		to, _ := strconv.Atoi(b)
		if from > n {
			continue
		}
		keep = append(keep, fmt.Sprintf("%d-%d", from, min(to, n)))
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, ",")
}

// colourant names an ink colour CUPS gives as "#00FFFF"
func colourant(hex string) string {
	switch strings.ToUpper(hex) {
	case "#000000":
		return "black"
	case "#00FFFF":
		return "cyan"
	case "#FF00FF":
		return "magenta"
	case "#FFFF00":
		return "yellow"
	case "":
		return "none"
	}
	return "unknown"
}

// cancel stops a job (s.mu held)
func (s *ippServer) cancel(j *ippJob) {
	j.state, j.reason = 7, "job-canceled-by-user"
	if j.job != nil { // just this print: other people's prints stay in the queue
		j.job.mu.Lock()
		cancelJob(j.job.CupsID)
		j.job.State = "canceled"
		j.job.mu.Unlock()
	}
}

func ippMessage(st uint16) string {
	switch st {
	case stNotAccepting:
		return "Printing from other apps is switched off in Sakura Print"
	case stDocFormatUnsupported:
		return "That kind of document can't be printed"
	case stNotFound:
		return "No such job"
	case stBadRequest:
		return "Bad request"
	case stNotAuthorized:
		return "That print was sent from another phone"
	case stNotPossible:
		return "Too many prints are waiting to be allowed in Sakura Print"
	case stVersionUnsupported:
		return "Only IPP 1.1 and 2.0 are spoken here"
	}
	return "Something went wrong"
}

func (s *ippServer) reply(w http.ResponseWriter, o *ippOut) {
	w.Header().Set("Content-Type", "application/ipp")
	w.Header().Set("Content-Length", strconv.Itoa(o.Len()))
	w.Write(o.Bytes())
}

// check: can a job be taken right now, in this format?
func (s *ippServer) check(req *ippRequest) uint16 {
	if !airprintOn() {
		return stNotAccepting
	}
	switch f := req.str("document-format"); f {
	case "", "application/pdf", "image/jpeg", "image/urf", "image/pwg-raster", "application/octet-stream":
		return stOK
	default:
		return stDocFormatUnsupported
	}
}

// takeJob: a new job. For Print-Job the pages come along; for Create-Job they follow in Send-Document.
func (s *ippServer) takeJob(r *http.Request, req *ippRequest, withData bool) (*ippJob, uint16) {
	trusted, _ := mayPrint(r)
	mac, peer := macOf(r), peerIP(r).String()
	sp := specFromRequest(req, driverCapsFor(ippPrinter()))
	if sp.name == "" {
		sp.name = "From " + firstNonEmpty(req.str("requesting-user-name"), "a phone")
	}
	s.mu.Lock()
	for id, old := range s.jobs {
		if old.held && old.state < 7 && time.Since(old.created) > heldLife { // nobody allowed it
			old.state, old.reason = 7, "job-canceled-by-operator"
		}
		if time.Since(old.created) > 24*time.Hour { // forget finished jobs after a day
			delete(s.jobs, id)
		}
	}
	if !trusted && len(s.waiting()) >= heldMax {
		s.mu.Unlock()
		return nil, stNotPossible
	}
	s.nextID++
	j := &ippJob{id: s.nextID, name: sp.name, user: firstNonEmpty(req.str("requesting-user-name"), "phone"), state: 3, reason: "none", created: time.Now(), spec: sp,
		held: !trusted, mac: mac, peer: peer}
	s.jobs[j.id] = j
	s.mu.Unlock()
	if withData {
		if st := s.receive(j, req.data); st != stOK {
			return nil, st
		}
	}
	return j, stOK
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// receive saves the pages, turns them into something printable and sends them through Sakura's pipeline
func (s *ippServer) receive(j *ippJob, data io.Reader) uint16 {
	dir := filepath.Join(s.work, "airprint-"+newID())
	os.MkdirAll(dir, 0o755)
	raw := filepath.Join(dir, "document")
	f, err := os.Create(raw)
	if err != nil {
		return stInternal
	}
	n, err := io.Copy(f, data)
	f.Close()
	if err != nil || n == 0 {
		return stBadRequest
	}
	path, name := raw, j.name
	format := j.spec.format
	if format == "" || format == "application/octet-stream" { // work it out from the first bytes
		if fh, err := os.Open(raw); err == nil {
			hdr := make([]byte, 8)
			io.ReadFull(fh, hdr)
			fh.Close()
			switch {
			case bytes.HasPrefix(hdr, []byte("UNIRAST")):
				format = "image/urf"
			case bytes.HasPrefix(hdr, []byte("RaS2")):
				format = "image/pwg-raster"
			case bytes.HasPrefix(hdr, []byte{0xFF, 0xD8, 0xFF}):
				format = "image/jpeg"
			}
		}
	}
	ext := ".pdf"
	switch format {
	case "image/urf", "image/pwg-raster":
		pdf := filepath.Join(dir, "pages.pdf")
		if err := rasterToPDF(raw, pdf, dir, format); err != nil {
			log.Printf("AirPrint: %v", err)
			s.fail(j, err)
			return stDocFormatUnsupported
		}
		path = pdf
	case "image/jpeg":
		ext = ".jpg"
	}
	file, err := addFile(path, name+ext)
	if err != nil {
		s.fail(j, err)
		return stDocFormatUnsupported
	}
	s.mu.Lock()
	j.file = file
	held := j.held
	if held {
		j.state, j.reason = 4, "job-hold-until-specified"
	}
	s.mu.Unlock()
	if held {
		log.Printf("Print from any app: %q from an unknown phone is waiting to be allowed", j.name)
		go remindHeld(j.name)
		return stOK
	}
	return s.start(j)
}

// start prints a job whose pages are here
func (s *ippServer) start(j *ippJob) uint16 {
	file := j.file
	printer := ippPrinter()
	sp := Spec{Printer: printer, Mode: "docs", Items: []Item{{ID: file.ID, Pages: pagesWithin(j.spec.pages, file.Pages)}}, Copies: j.spec.copies, Cover: 1, FitPage: true,
		Sides: "single", Options: jobOptions(printer, j.spec)}
	if strings.HasPrefix(j.spec.sides, "two-sided") {
		sp.Sides = "duplex"
		sp.ShortEdge = j.spec.sides == "two-sided-short-edge"
	}
	b, err := build(sp, s.work)
	if err != nil {
		s.fail(j, err)
		return stInternal
	}
	stateMu.Lock()
	builds[b.ID] = b
	stateMu.Unlock()
	b.mu.Lock()
	job, err := printBuilt(b, false, "airprint")
	b.mu.Unlock()
	if err != nil {
		s.fail(j, err)
		return stInternal
	}
	job.mu.Lock()
	job.Title = j.name
	job.mu.Unlock()
	s.mu.Lock()
	j.job, j.state, j.reason = job, 5, "job-printing"
	s.mu.Unlock()
	if b.Duplex {
		go remindFlip(job, j.name)
	}
	return stOK
}

// owner says which jobs the phone making this request sent (the computer may touch any)
func (s *ippServer) owner(r *http.Request) func(*ippJob) bool {
	if isLocal(r) {
		return func(*ippJob) bool { return true }
	}
	mac, peer := macOf(r), peerIP(r).String()
	return func(j *ippJob) bool {
		if j.mac != "" {
			return mac == j.mac
		}
		return peer == j.peer
	}
}

// waiting: prints from unknown phones waiting to be allowed, oldest first (s.mu held)
func (s *ippServer) waiting() []*ippJob {
	var list []*ippJob
	for _, j := range s.jobs {
		if j.held && j.state < 7 {
			list = append(list, j)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].id < list[b].id })
	return list
}

// decide: "allow" (once), "trust" (and this phone from now on) or "refuse" a waiting print
func (s *ippServer) decide(id int, action string) error {
	s.mu.Lock()
	j := s.jobs[id]
	if j == nil || !j.held || j.state >= 7 {
		s.mu.Unlock()
		return errors.New("that print isn't waiting any more")
	}
	if action == "refuse" {
		j.state, j.reason = 7, "job-canceled-by-operator"
		s.mu.Unlock()
		return nil
	}
	if j.file == nil { // Create-Job came, the pages haven't yet: they'll print as soon as they arrive
		j.held = false
		s.mu.Unlock()
	} else {
		j.held = false
		s.mu.Unlock()
		if st := s.start(j); st != stOK {
			return errors.New("couldn't print it")
		}
	}
	if action == "trust" && j.mac != "" {
		settingsMu.Lock()
		rememberDevice(j.mac, "", firstNonEmpty(strings.TrimSpace(j.user), "Phone"))
		saveSettings()
		settingsMu.Unlock()
	}
	return nil
}

// remindHeld tells the computer a print from an unknown phone is waiting
func remindHeld(title string) {
	if have("notify-send") {
		runQuiet(10*time.Second, "notify-send", "-a", "Sakura Print", "-i", "printer", "A print is waiting for you",
			fmt.Sprintf("%q came from a phone that hasn't logged in to Sakura Print. Open Sakura Print to see it and allow it.", title))
	}
}

func (s *ippServer) fail(j *ippJob, err error) {
	s.mu.Lock()
	j.state, j.reason = 8, "document-format-error"
	s.mu.Unlock()
	log.Printf("AirPrint job %q: %v", j.name, err)
}

// jobOptions turns what the phone asked for into the real printer's own settings
// jobOptions: the phone's choices as the printer driver's own (paper, paper type, quality, colour, both sides)
func jobOptions(printer string, sp ippSpec) map[string]string {
	return driverCapsFor(printer).options(sp)
}

// remindFlip: a double-sided AirPrint job needs the paper turned over. The phone's print screen can't say that,
// so the computer shows a notification, and Sakura Print's home screen shows the flip guide.
var (
	flipPoll      = 2 * time.Second // how often a print from another app is checked for side 1 being done
	notifyActions = -1              // does notify-send offer buttons: -1 not asked yet, 0 no, 1 yes
)

// remindFlip: when side 1 of a double-sided print from another app is done, the computer says so, with a button
// that prints the other side (the phone's print queue says it too: see printerState)
func remindFlip(job *Job, title string) {
	for i := 0; i < int(20*time.Minute/flipPoll); i++ {
		time.Sleep(flipPoll)
		job.mu.Lock()
		job.refresh()
		st := job.State
		job.mu.Unlock()
		if st == "flip" {
			flipNotify(job, title)
			return
		}
		if st != "printing1" {
			return
		}
	}
}

func flipNotify(job *Job, title string) {
	if !have("notify-send") {
		return
	}
	body := fmt.Sprintf("“%s”: side one is done. Take the printed pages, turn them over as Sakura Print shows, put them back, then print the other side.", title)
	if notifyActions < 0 {
		out, _ := exec.Command("notify-send", "--help").CombinedOutput()
		notifyActions = map[bool]int{true: 1, false: 0}[strings.Contains(string(out), "--action")]
	}
	if notifyActions == 0 { // an older notify-send: no buttons, just the message
		runQuiet(10*time.Second, "notify-send", "-a", "Sakura Print", "-i", "printer", "-u", "critical", "Turn the paper over", body)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "notify-send", "-a", "Sakura Print", "-i", "printer", "-u", "critical",
		"-A", "side2=Print the other side", "-A", "guide=Show me how", "Turn the paper over", body).Output()
	switch strings.TrimSpace(string(out)) {
	case "side2":
		job.mu.Lock()
		err := job.side2()
		job.mu.Unlock()
		msg := "Printing the other side of “" + title + "”."
		if err != nil {
			msg = "Couldn't print the other side: " + err.Error()
		}
		runQuiet(10*time.Second, "notify-send", "-a", "Sakura Print", "-i", "printer", "Sakura Print", msg)
	case "guide": // the app, with the flip guide on its home screen
		if serverPort > 0 {
			openWindow(serverPort)
		}
	}
}
