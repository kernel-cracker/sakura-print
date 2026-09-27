// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Every kind of file phones and computers have: the app itself only works with PDF, JPEG and PNG, so anything
// else is turned into one of those the moment it's added. Files are recognised by what's inside them, not by
// their name (a photo called .jpg that is really HEIC still works).
//
//	iPhone / Android / Samsung photos: HEIC, HEIF, AVIF       -> JPEG  (libheif: heif-dec, or ImageMagick, vips, ffmpeg)
//	other pictures: WebP, GIF, BMP, TIFF, ICO                   -> PNG / JPEG (ImageMagick, vips or ffmpeg)
//	multi-page TIFF (faxes, scanners)                           -> PDF
//	drawings: SVG                                               -> PNG   (rsvg-convert)
//	Word, Excel, PowerPoint, OpenDocument, RTF, text, CSV, HTML -> PDF   (LibreOffice)
//
// Converted copies go in ~/.local/share/sakuraprint/converted, named by the original's fingerprint, so the
// same file is only converted once, and nothing is ever written next to your own files.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// sniff says what a file really is, from its first bytes (and, for zip-based office files, its name)
func sniff(path, name string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	ext := strings.ToLower(filepath.Ext(name))
	has := func(p string) bool { return bytes.HasPrefix(head, []byte(p)) }
	switch {
	case has("%PDF"):
		return "pdf"
	case has("\xff\xd8\xff"):
		return "jpeg"
	case has("\x89PNG\r\n\x1a\n"):
		return "png"
	case has("GIF87a"), has("GIF89a"):
		return "gif"
	case has("BM") && n > 18 && head[15] == 0 && head[16] == 0 && head[17] == 0 &&
		(head[14] == 12 || head[14] == 40 || head[14] == 52 || head[14] == 56 || head[14] == 108 || head[14] == 124): // the header's size field
		return "bmp"
	case has("II*\x00"), has("MM\x00*"):
		return "tiff"
	case has("RIFF") && n >= 12 && string(head[8:12]) == "WEBP":
		return "webp"
	case has("\x00\x00\x01\x00") && ext == ".ico":
		return "ico"
	case n >= 12 && string(head[4:8]) == "ftyp":
		brand := string(head[8:12])
		switch brand {
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1", "avif", "avis":
			return "heif"
		}
		// some phones put the real brand in the compatible list
		if i := bytes.Index(head[:min(n, 64)], []byte("heic")); i > 0 {
			return "heif"
		}
		if i := bytes.Index(head[:min(n, 64)], []byte("avif")); i > 0 {
			return "heif"
		}
	case has("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"): // old Office (.doc .xls .ppt)
		return "office"
	case has("PK\x03\x04"):
		switch ext {
		case ".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp", ".odg", ".docm", ".xlsm", ".pptm", ".dotx", ".ott", ".pages", ".key", ".numbers":
			return "office"
		}
		if bytes.Contains(head, []byte("word/")) || bytes.Contains(head, []byte("xl/")) || bytes.Contains(head, []byte("ppt/")) ||
			bytes.Contains(head, []byte("[Content_Types].xml")) || bytes.Contains(head, []byte("mimetypeapplication/vnd.oasis.opendocument")) {
			return "office"
		}
	case has("{\\rtf"):
		return "office"
	}
	trim := bytes.TrimLeft(head, " \t\r\n\xef\xbb\xbf")
	lower := bytes.ToLower(trim)
	switch {
	case bytes.HasPrefix(lower, []byte("<svg")) || (bytes.HasPrefix(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<svg"))):
		return "svg"
	case bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")):
		return "office" // LibreOffice turns web pages into PDFs too
	}
	if n > 0 && utf8.Valid(head[:n-utf8Tail(head)]) && !bytes.ContainsRune(head, 0) {
		return "text"
	}
	return ""
}

// utf8Tail: how many bytes at the end are a cut-off UTF-8 character (the 4 KB read can end mid-character)
func utf8Tail(b []byte) int {
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c&0xC0 != 0x80 { // the start of a character: is it complete?
			if !utf8.FullRune(b[len(b)-i:]) {
				return i
			}
			return 0
		}
	}
	return 0
}

func convertedDir() string { return filepath.Join(dataDir(), "converted") }

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

func have(tool string) bool { _, err := exec.LookPath(tool); return err == nil }

// runQuiet runs a converter with a time limit; the error includes what it said
func runQuiet(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("%s took too long", name)
	}
	if err != nil {
		return fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// normalize returns a PDF, JPEG or PNG version of any supported file (the file itself if it already is one)
func normalize(path, name string) (string, error) {
	kind := sniff(path, name)
	switch kind {
	case "pdf", "jpeg", "png":
		return path, nil
	case "":
		return "", fmt.Errorf("%s isn't a kind of file Sakura Print can print", name)
	}
	h, err := fileHash(path)
	if err != nil {
		return "", err
	}
	os.MkdirAll(convertedDir(), 0o700)
	for _, ext := range []string{".jpg", ".png", ".pdf"} { // converted before: use that
		if p := filepath.Join(convertedDir(), h+ext); fileExists(p) {
			now := time.Now()
			os.Chtimes(p, now, now)
			return p, nil
		}
	}
	base := filepath.Join(convertedDir(), h)
	var out string
	switch kind {
	case "heif":
		out, err = convertImage(path, base+".jpg", "heif")
	case "gif", "webp", "ico":
		out, err = convertImage(path+"[0]", base+".png", kind) // first frame of an animation
	case "bmp":
		out, err = convertImage(path, base+".png", kind)
	case "tiff":
		out, err = convertTIFF(path, base)
	case "svg":
		out = base + ".png"
		if !have("rsvg-convert") {
			return "", errors.New("drawings (SVG) need rsvg-convert, which the installer puts in")
		}
		err = runQuiet(time.Minute, "rsvg-convert", "-w", "2480", "--keep-aspect-ratio", "-b", "white", "-o", out, path)
	case "office", "text":
		out, err = convertOffice(path, name, base)
	}
	if err != nil {
		os.Remove(out)
		return "", fmt.Errorf("couldn't open %s: %v", name, err)
	}
	if !fileExists(out) {
		return "", fmt.Errorf("couldn't open %s", name)
	}
	return out, nil
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && st.Size() > 0 }

// convertImage tries each picture tool that's installed, best first
func convertImage(src, out, kind string) (string, error) {
	plain := strings.TrimSuffix(src, "[0]")
	type way struct {
		tool string
		args []string
	}
	var ways []way
	if kind == "heif" {
		for _, t := range []string{"heif-dec", "heif-convert"} { // libheif: turns the picture the right way up by itself
			ways = append(ways, way{t, []string{"-q", "92", plain, out}})
		}
	}
	ways = append(ways,
		way{"magick", []string{src, "-auto-orient", "-background", "white", "-flatten", "-quality", "92", out}},
		way{"convert", []string{src, "-auto-orient", "-background", "white", "-flatten", "-quality", "92", out}},
		way{"vips", []string{"copy", plain, out}},
		way{"ffmpeg", []string{"-y", "-loglevel", "error", "-i", plain, "-frames:v", "1", out}})
	var last error
	for _, w := range ways {
		if !have(w.tool) {
			continue
		}
		if last = runQuiet(2*time.Minute, w.tool, w.args...); last == nil && fileExists(out) {
			return out, nil
		}
		os.Remove(out)
	}
	if last == nil {
		if kind == "heif" {
			return out, errors.New("this computer can't open iPhone HEIC photos. Send the photo from the iPhone's photo picker instead (it converts it), or on the iPhone: Settings → Camera → Formats → Most Compatible")
		}
		return out, errors.New("this kind of picture needs ImageMagick (the installer puts it in)")
	}
	if kind == "heif" { // the tools are there but can't decode it (no HEVC codec, e.g. openSUSE without Packman)
		return out, errors.New("this computer can't open iPhone HEIC photos (" + last.Error() + "). Send the photo from the iPhone's photo picker instead (it converts it), or on the iPhone: Settings → Camera → Formats → Most Compatible")
	}
	return out, last
}

// convertTIFF: one page becomes a PNG, several pages (a fax, a scanner's document) a PDF
func convertTIFF(src, base string) (string, error) {
	pages := 1
	for _, t := range []string{"magick", "identify"} {
		if !have(t) {
			continue
		}
		args := []string{"identify", "-format", "%n\n", src}
		if t == "identify" {
			args = args[1:]
		}
		if o, err := exec.Command(t, args...).Output(); err == nil {
			fmt.Sscan(strings.TrimSpace(string(o)), &pages)
		}
		break
	}
	if pages > 1 {
		out := base + ".pdf"
		for _, t := range []string{"magick", "convert"} {
			if have(t) {
				return out, runQuiet(3*time.Minute, t, src, "-compress", "jpeg", "-quality", "90", out)
			}
		}
	}
	return convertImage(src+"[0]", base+".png", "tiff")
}

// convertOffice: Word, Excel, PowerPoint, OpenDocument, RTF, text and web pages become a PDF, with LibreOffice
func convertOffice(src, name, base string) (string, error) {
	office := ""
	for _, t := range []string{"soffice", "libreoffice", "lowriter"} {
		if have(t) {
			office = t
			break
		}
	}
	if office == "" {
		return base + ".pdf", errors.New("Word, Excel, PowerPoint and text files need LibreOffice. Install it with: ./install.sh --office")
	}
	// LibreOffice picks how to read a file from its name, so give it a copy with the right ending, and its own
	// settings folder so it works even while LibreOffice is open on the computer
	work, err := os.MkdirTemp("", "sakura-office-")
	if err != nil {
		return base + ".pdf", err
	}
	defer os.RemoveAll(work)
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" || sniff(src, name) == "text" && ext != ".csv" && ext != ".txt" && ext != ".md" {
		ext = ".txt"
	}
	in := filepath.Join(work, "document"+ext)
	data, err := os.ReadFile(src)
	if err != nil {
		return base + ".pdf", err
	}
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return base + ".pdf", err
	}
	if err := runQuiet(3*time.Minute, office, "-env:UserInstallation=file://"+filepath.Join(work, "profile"), "--headless", "--norestore",
		"--convert-to", "pdf", "--outdir", work, in); err != nil {
		return base + ".pdf", err
	}
	out := base + ".pdf"
	if err := os.Rename(filepath.Join(work, "document.pdf"), out); err != nil {
		return out, errors.New("LibreOffice couldn't read it")
	}
	return out, nil
}

// cleanConverted forgets converted copies nobody has used for a week
func cleanConverted() { cleanOld(convertedDir(), 7*24*time.Hour) }
