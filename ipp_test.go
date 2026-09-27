// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"image"
	"slices"
	"testing"
)

// encodeURF writes Apple raster the way phones do, using every kind of run, so the decoder is checked against it
func encodeURF(pages []image.Image, gray bool) []byte {
	var b bytes.Buffer
	b.WriteString("UNIRAST\x00")
	binary.Write(&b, binary.BigEndian, uint32(len(pages)))
	ch := 3
	if gray {
		ch = 1
	}
	for _, im := range pages {
		hdr := make([]byte, 32)
		hdr[0], hdr[1] = byte(ch*8), 1
		binary.BigEndian.PutUint32(hdr[12:], uint32(im.Bounds().Dx()))
		binary.BigEndian.PutUint32(hdr[16:], uint32(im.Bounds().Dy()))
		binary.BigEndian.PutUint32(hdr[20:], 300)
		b.Write(hdr)
		packPixels(&b, im, ch)
	}
	return b.Bytes()
}

// encodePWG writes PWG raster, as Android does: "RaS2", then a 1796-byte header and the packed pixels per page
func encodePWG(pages []image.Image, gray bool) []byte {
	var b bytes.Buffer
	b.WriteString("RaS2")
	ch, space := 3, 19
	if gray {
		ch, space = 1, 18
	}
	for _, im := range pages {
		hdr := make([]byte, 1796)
		copy(hdr, "PwgRaster")
		put := func(at, v int) { binary.BigEndian.PutUint32(hdr[at:], uint32(v)) }
		put(276, 300)
		put(280, 300)
		put(372, im.Bounds().Dx())
		put(376, im.Bounds().Dy())
		put(384, 8)
		put(388, ch*8)
		put(392, ch*im.Bounds().Dx())
		put(400, space)
		b.Write(hdr)
		packPixels(&b, im, ch)
	}
	return b.Bytes()
}

// packPixels packs a page the way both raster formats do, using every kind of run
func packPixels(b *bytes.Buffer, im image.Image, ch int) {
	gray := ch == 1
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	px := func(x, y int) []byte {
		r, g, bl, _ := im.At(x, y).RGBA()
		if gray {
			return []byte{byte(r >> 8)}
		}
		return []byte{byte(r >> 8), byte(g >> 8), byte(bl >> 8)}
	}
	row := func(y int) []byte {
		var out []byte
		for x := 0; x < w; x++ {
			out = append(out, px(x, y)...)
		}
		return out
	}
	for y := 0; y < h; {
		rep := 0 // identical following lines
		for y+rep+1 < h && rep < 255 && bytes.Equal(row(y), row(y+rep+1)) {
			rep++
		}
		b.WriteByte(byte(rep))
		allWhite := func(from int) bool {
			for x := from; x < w; x++ {
				for _, v := range px(x, y) {
					if v != 255 {
						return false
					}
				}
			}
			return true
		}
		for x := 0; x < w; {
			if x > 0 && allWhite(x) { // "the rest of the line is white"
				b.WriteByte(128)
				break
			}
			n := 1 // a run of the same pixel?
			for x+n < w && n < 128 && bytes.Equal(px(x, y), px(x+n, y)) {
				n++
			}
			if n > 1 {
				b.WriteByte(byte(n - 1))
				b.Write(px(x, y))
				x += n
				continue
			}
			n = 1 // literal pixels until the next run
			for x+n < w && n < 128 && !bytes.Equal(px(x+n-1, y), px(x+n, y)) {
				n++
			}
			b.WriteByte(byte(257 - n))
			for k := 0; k < n; k++ {
				b.Write(px(x+k, y))
			}
			x += n
		}
		y += rep + 1
	}
}

func testImage(w, h int, seed int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*im.Stride + x*4
			switch {
			case x > w*3/4: // white right edge: the "rest is white" code
				im.Pix[i], im.Pix[i+1], im.Pix[i+2] = 255, 255, 255
			case y < h/4: // long runs, and identical lines
				im.Pix[i], im.Pix[i+1], im.Pix[i+2] = 200, 30, 40
			default: // noise: literal pixels
				v := byte((x*7 + y*13 + seed*31) % 251)
				im.Pix[i], im.Pix[i+1], im.Pix[i+2] = v, 255-v, v/2
			}
			im.Pix[i+3] = 255
		}
	}
	return im
}

func TestDecodeRaster(t *testing.T) {
	type codec struct {
		name   string
		encode func([]image.Image, bool) []byte
		decode func(*bufio.Reader) ([]urfPage, error)
	}
	for _, c := range []codec{{"URF", encodeURF, decodeURF}, {"PWG", encodePWG, decodePWG}} {
		t.Run(c.name, func(t *testing.T) { testRaster(t, c.encode, c.decode) })
	}
	if _, err := decodePWG(bufio.NewReader(bytes.NewReader([]byte("RaS2")))); err == nil {
		t.Error("a PWG raster with no pages must be refused")
	}
}

func testRaster(t *testing.T, encode func([]image.Image, bool) []byte, decodeURF func(*bufio.Reader) ([]urfPage, error)) {
	for _, gray := range []bool{false, true} {
		a, b := testImage(173, 61, 1), testImage(90, 140, 2)
		data := encode([]image.Image{a, b}, gray)
		pages, err := decodeURF(bufio.NewReader(bytes.NewReader(data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 2 || pages[0].dpi != 300 {
			t.Fatalf("want 2 pages at 300 dpi, got %d", len(pages))
		}
		for n, want := range []*image.RGBA{a, b} {
			got := pages[n].img
			if got.Bounds() != want.Bounds() {
				t.Fatalf("page %d size %v", n+1, got.Bounds())
			}
			for y := 0; y < want.Bounds().Dy(); y++ {
				for x := 0; x < want.Bounds().Dx(); x++ {
					wr, wg, wb, _ := want.At(x, y).RGBA()
					gr, gg, gb, _ := got.At(x, y).RGBA()
					if gray {
						wg, wb, gg, gb = wr, wr, gr, gr
					}
					if wr>>8 != gr>>8 || wg>>8 != gg>>8 || wb>>8 != gb>>8 {
						t.Fatalf("gray=%v page %d pixel %d,%d: want %d,%d,%d got %d,%d,%d", gray, n+1, x, y, wr>>8, wg>>8, wb>>8, gr>>8, gg>>8, gb>>8)
					}
				}
			}
		}
	}
	if _, err := decodeURF(bufio.NewReader(bytes.NewReader([]byte("%PDF-1.4 not raster")))); err == nil {
		t.Error("a non-raster file must be refused")
	}
	trunc := encode([]image.Image{testImage(50, 50, 3)}, false)
	if _, err := decodeURF(bufio.NewReader(bytes.NewReader(trunc[:len(trunc)/2]))); err == nil {
		t.Error("a cut-off raster must be refused, not half-printed")
	}
}

func TestIPPRoundTrip(t *testing.T) {
	o := &ippOut{}
	o.head([2]byte{2, 0}, opPrintJob, 7)
	o.group(tagOperation)
	o.attr(vCharset, "attributes-charset", "utf-8")
	o.attr(vMimeType, "document-format", "image/urf")
	o.group(tagJob)
	o.attr(vInteger, "copies", 3)
	o.attr(vKeyword, "sides", "two-sided-long-edge")
	o.collStart("media-col")
	o.memberColl("media-size")
	o.member("x-dimension", vInteger, 21000)
	o.member("y-dimension", vInteger, 29700)
	o.collEnd()
	o.member("media-type", vKeyword, "stationery")
	o.collEnd()
	o.attr(vKeyword, "print-color-mode", "monochrome")
	o.group(tagEnd)
	o.WriteString("DOCUMENT BYTES")
	req, err := readIPP(bytes.NewReader(o.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if req.op != opPrintJob || req.requestID != 7 || req.str("document-format") != "image/urf" || req.str("sides") != "two-sided-long-edge" || req.str("print-color-mode") != "monochrome" {
		t.Fatalf("attributes lost: %+v", req.attrs)
	}
	if n, _ := req.num("copies"); n != 3 {
		t.Fatalf("copies %d", n)
	}
	x, okx := req.num("media-col.media-size.x-dimension")
	y, oky := req.num("media-col.media-size.y-dimension")
	if !okx || !oky || x != 21000 || y != 29700 {
		t.Fatalf("paper size inside media-col lost: %v %v %d %d (%+v)", okx, oky, x, y, req.attrs)
	}
	if req.str("media-col.media-type") != "stationery" {
		t.Fatalf("media-type: %q", req.str("media-col.media-type"))
	}
	rest := new(bytes.Buffer)
	rest.ReadFrom(req.data)
	if rest.String() != "DOCUMENT BYTES" {
		t.Fatalf("document: %q", rest.String())
	}
}

// an attribute holding several collections (like a list of paper sizes): every one is read, under its own name,
// and each member keeps one value per collection, in order
func TestIPPManyCollections(t *testing.T) {
	o := &ippOut{}
	o.head([2]byte{2, 0}, opPrintJob, 9)
	o.group(tagOperation)
	o.attr(vCharset, "attributes-charset", "utf-8")
	o.attr(vLanguage, "attributes-natural-language", "en")
	o.group(tagPrinter)
	for i, size := range [][3]int{{21000, 29700, 300}, {21000, 29700, 0}, {10160, 15240, 0}} {
		name := ""
		if i == 0 {
			name = "media-col-database"
		}
		o.collStart(name)
		o.memberColl("media-size")
		o.member("x-dimension", vInteger, size[0])
		o.member("y-dimension", vInteger, size[1])
		o.collEnd()
		o.member("media-top-margin", vInteger, size[2])
		o.collEnd()
	}
	o.attr(vKeyword, "after", "still-here")
	o.group(tagEnd)
	req, err := readIPP(bytes.NewReader(o.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	ints := func(name string) []int {
		a := req.get(name)
		if a == nil {
			return nil
		}
		var out []int
		for _, v := range a.values {
			out = append(out, int(binary.BigEndian.Uint32(v)))
		}
		return out
	}
	if got := ints("media-col-database.media-size.x-dimension"); !slices.Equal(got, []int{21000, 21000, 10160}) {
		t.Errorf("widths of all three: %v", got)
	}
	if got := ints("media-col-database.media-top-margin"); !slices.Equal(got, []int{300, 0, 0}) {
		t.Errorf("top margins of all three: %v", got)
	}
	for _, a := range req.attrs {
		if a.name == "media-size" || a.name == "x-dimension" || a.name == "media-top-margin" {
			t.Errorf("a collection's member leaked out as its own attribute: %q", a.name)
		}
	}
	if req.str("after") != "still-here" {
		t.Error("the attribute after the collections must still be read")
	}
}
