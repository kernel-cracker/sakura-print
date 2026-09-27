// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSniff(t *testing.T) {
	dir := filepath.Join("tests", "fixtures", "formats")
	want := map[string]string{
		"photo.heic": "heif", "photo.avif": "heif", "renamed-heic.jpg": "heif", "photo.webp": "webp", "photo-animated.gif": "gif",
		"photo.bmp": "bmp", "photo.tiff": "tiff", "fax-2pages.tiff": "tiff", "drawing.svg": "svg", "letter.docx": "office",
		"letter.odt": "office", "sheet.xlsx": "office", "slides.pptx": "office", "letter.rtf": "office", "page.html": "office",
		"notes.txt": "text", "prices.csv": "text",
	}
	for name, k := range want {
		if got := sniff(filepath.Join(dir, name), name); got != k {
			t.Errorf("%s: want %s, got %q", name, k, got)
		}
	}
	tmp := t.TempDir()
	bmw := filepath.Join(tmp, "car.txt")
	os.WriteFile(bmw, []byte("BMW service booked for Monday, then the tyres.\n"), 0o644)
	if got := sniff(bmw, "car.txt"); got != "text" {
		t.Errorf("text starting with BM is still text, got %q", got)
	}
	junk := filepath.Join(tmp, "junk.bin")
	os.WriteFile(junk, []byte{0, 1, 2, 3, 0xff, 0xfe, 0, 9, 0, 0, 7}, 0o644)
	if _, err := normalize(junk, "junk.bin"); err == nil {
		t.Error("an unknown kind of file should be refused, with a clear message")
	}
}

// colours reads the left and right halves of a converted picture (the samples are red on the left, blue on the right)
func colours(t *testing.T, path string) (left, right [3]uint32) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	b := im.Bounds()
	at := func(x, y int) [3]uint32 { r, g, bl, _ := im.At(x, y).RGBA(); return [3]uint32{r >> 8, g >> 8, bl >> 8} }
	return at(b.Min.X+b.Dx()/4, b.Min.Y+b.Dy()/2), at(b.Min.X+3*b.Dx()/4, b.Min.Y+b.Dy()/2)
}

func TestConvertEveryFormat(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join("tests", "fixtures", "formats")
	before, _ := os.ReadDir(dir)
	red := func(c [3]uint32) bool { return c[0] > 180 && c[1] < 110 && c[2] < 110 }
	blue := func(c [3]uint32) bool { return c[2] > 170 && c[0] < 110 }
	pictures := []string{"photo.heic", "photo.avif", "renamed-heic.jpg", "photo.webp", "photo-animated.gif", "photo.bmp", "photo.tiff", "drawing.svg"}
	for _, name := range pictures {
		f, err := addFile(filepath.Join(dir, name), name)
		if err != nil {
			if strings.Contains(err.Error(), "need") { // the converter isn't installed on this machine
				t.Logf("skipped %s: %v", name, err)
				continue
			}
			t.Errorf("%s: %v", name, err)
			continue
		}
		if f.Kind != "image" || f.Name != name {
			t.Errorf("%s: want a picture keeping its name, got %+v", name, f)
			continue
		}
		l, r := colours(t, f.Path)
		if !red(l) || !blue(r) {
			t.Errorf("%s: the picture came out wrong (left %v, right %v)", name, l, r)
		}
	}
	if !have("soffice") && !have("libreoffice") {
		t.Log("LibreOffice not installed: skipping documents")
	} else {
		for _, name := range []string{"letter.docx", "letter.odt", "sheet.xlsx", "slides.pptx", "letter.rtf", "page.html", "notes.txt", "prices.csv"} {
			f, err := addFile(filepath.Join(dir, name), name)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			if f.Kind != "pdf" || f.Pages < 1 {
				t.Errorf("%s: want a PDF, got %+v", name, f)
			}
			if name == "notes.txt" && have("pdftotext") {
				out, _ := exec.Command("pdftotext", f.Path, "-").Output()
				for _, w := range []string{"Shopping list", "ಅಕ್ಕಿ", "चावल"} {
					if !strings.Contains(string(out), w) {
						t.Errorf("notes.txt: %q missing from the PDF (Kannada and Hindi must survive)", w)
					}
				}
			}
		}
	}
	if have("magick") || have("convert") {
		f, err := addFile(filepath.Join(dir, "fax-2pages.tiff"), "fax-2pages.tiff")
		if err != nil || f.Kind != "pdf" || f.Pages != 2 {
			t.Errorf("a two-page TIFF should become a two-page PDF: %+v %v", f, err)
		}
	}
	// converted once: adding it again reuses the copy
	a, _ := addFile(filepath.Join(dir, "photo.webp"), "photo.webp")
	b, _ := addFile(filepath.Join(dir, "photo.webp"), "photo.webp")
	if a == nil || b == nil || a.Path != b.Path || !strings.HasPrefix(a.Path, convertedDir()) {
		t.Error("the same file should be converted once, into the cache folder")
	}
	after, _ := os.ReadDir(dir)
	if len(after) != len(before) {
		t.Error("nothing may ever be written next to the original files")
	}
}
