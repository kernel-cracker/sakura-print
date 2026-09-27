// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// A tiny PDF writer for pages made of pictures (photos, scans, edited pages).
// Phone JPEGs go into the PDF exactly as they are: no decoding, no re-encoding.
// That makes photo jobs build in milliseconds instead of seconds, with zero quality loss.

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"strings"
)

// placement: one picture on a page. Position and size in mm from the top-left corner.
type placement struct {
	im         *imgInfo
	x, y, w, h float64
	fill       bool // true: fill the frame (crops), false: show the whole picture
	rot        int  // clockwise turn as seen on the page: 0, 90, 180, 270
}

type picPage struct {
	w, h  float64 // mm
	items []placement
}

const ptPerMM = 72 / 25.4

// pdfImage returns the bytes and colour space to embed for a picture
func pdfImage(im *imgInfo) (data []byte, cs string, w, h int, err error) {
	if im.Mime == "image/jpeg" && (im.Model == "rgb" || im.Model == "gray") {
		cs = "/DeviceRGB"
		if im.Model == "gray" {
			cs = "/DeviceGray"
		}
		return im.Data, cs, im.RawW, im.RawH, nil
	}
	// PNG, GIF, CMYK JPEG...: decode once, flatten onto white, store as a high quality JPEG
	src, _, err := image.Decode(bytes.NewReader(im.Data))
	if err != nil {
		return nil, "", 0, 0, err
	}
	b := src.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 92}); err != nil {
		return nil, "", 0, 0, err
	}
	return buf.Bytes(), "/DeviceRGB", b.Dx(), b.Dy(), nil
}

// writePicturePDF writes pages of pictures to out
func writePicturePDF(pages []picPage, out string) error {
	type obj struct{ head, stream []byte }
	var objs []obj
	add := func(head string, stream []byte) int {
		objs = append(objs, obj{[]byte(head), stream})
		return len(objs)
	}
	add("<< /Type /Catalog /Pages 2 0 R >>", nil)
	add("", nil) // Pages: filled in at the end, once the kids are known

	imgObj := map[*imgInfo]int{}
	var kids []string
	for _, pg := range pages {
		W, H := pg.w*ptPerMM, pg.h*ptPerMM
		var cs strings.Builder
		res := map[int]bool{}
		for _, p := range pg.items {
			n, ok := imgObj[p.im]
			if !ok {
				data, space, iw, ih, err := pdfImage(p.im)
				if err != nil {
					return err
				}
				n = add(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>",
					iw, ih, space, len(data)), data)
				imgObj[p.im] = n
				// remember the real pixel size for the maths below (re-encoded pictures keep theirs)
				p.im.RawW, p.im.RawH = iw, ih
			}
			res[n] = true
			// the frame, in PDF units (origin bottom-left)
			fx, fy := p.x*ptPerMM, H-(p.y+p.h)*ptPerMM
			fw, fh := p.w*ptPerMM, p.h*ptPerMM
			r := ((p.rot % 360) + 360) % 360
			rw, rh := float64(p.im.RawW), float64(p.im.RawH)
			dw, dh := rw, rh // size as seen after turning
			if r == 90 || r == 270 {
				dw, dh = rh, rw
			}
			s := math.Min(fw/dw, fh/dh)
			if p.fill {
				s = math.Max(fw/dw, fh/dh)
			}
			RW, RH := rw*s, rh*s
			th := -float64(r) * math.Pi / 180 // clockwise on the page = negative angle in PDF
			a, b := RW*math.Cos(th), RW*math.Sin(th)
			c, d := -RH*math.Sin(th), RH*math.Cos(th)
			cx, cy := fx+fw/2, fy+fh/2
			e, f := cx-(a+c)/2, cy-(b+d)/2
			fmt.Fprintf(&cs, "q %.3f %.3f %.3f %.3f re W n %.5f %.5f %.5f %.5f %.3f %.3f cm /Im%d Do Q\n", fx, fy, fw, fh, a, b, c, d, e, f, n)
		}
		content := []byte(cs.String())
		cn := add(fmt.Sprintf("<< /Length %d >>", len(content)), content)
		var xo strings.Builder
		for n := range res {
			fmt.Fprintf(&xo, "/Im%d %d 0 R ", n, n)
		}
		pn := add(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.3f %.3f] /Resources << /XObject << %s>> >> /Contents %d 0 R >>", W, H, xo.String(), cn), nil)
		kids = append(kids, fmt.Sprintf("%d 0 R", pn))
	}
	objs[1].head = []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))

	fh, err := os.Create(out)
	if err != nil {
		return err
	}
	defer fh.Close()
	w := bufio.NewWriterSize(fh, 1<<20)
	pos := 0
	put := func(b []byte) { w.Write(b); pos += len(b) }
	put([]byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"))
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = pos
		put([]byte(fmt.Sprintf("%d 0 obj\n", i+1)))
		put(o.head)
		if o.stream != nil {
			put([]byte("\nstream\n"))
			put(o.stream)
			put([]byte("\nendstream"))
		}
		put([]byte("\nendobj\n"))
	}
	xref := pos
	put([]byte(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)))
	for _, o := range offs {
		put([]byte(fmt.Sprintf("%010d 00000 n \n", o)))
	}
	put([]byte(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)))
	return w.Flush()
}
