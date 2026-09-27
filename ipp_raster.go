// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
)

// Print from any app, part 4: pages sent as pictures (Apple URF and PWG raster) turned into a PDF.

// ---------- Apple raster (URF / "UNIRAST") to PDF ----------

// rasterToPDF: phones may send each page as a picture (Apple or PWG raster). Each page becomes a JPEG,
// and the JPEGs become a PDF (with the tiny picture-PDF writer).
func rasterToPDF(src, out, dir, format string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	decode := decodeURF
	if format == "image/pwg-raster" {
		decode = decodePWG
	}
	pages, err := decode(bufio.NewReader(f))
	if err != nil {
		return err
	}
	var pp []picPage
	for i, pg := range pages {
		p := filepath.Join(dir, fmt.Sprintf("page-%03d.jpg", i+1))
		fh, err := os.Create(p)
		if err != nil {
			return err
		}
		err = jpeg.Encode(fh, pg.img, &jpeg.Options{Quality: 92})
		fh.Close()
		if err != nil {
			return err
		}
		im, err := loadImage(p)
		if err != nil {
			return err
		}
		w := float64(pg.img.Bounds().Dx()) / float64(pg.dpi) * 25.4
		h := float64(pg.img.Bounds().Dy()) / float64(pg.dpi) * 25.4
		pp = append(pp, picPage{w: w, h: h, items: []placement{{im: im, x: 0, y: 0, w: w, h: h}}})
	}
	if len(pp) == 0 {
		return errors.New("no pages in the raster")
	}
	return writePicturePDF(pp, out)
}

type urfPage struct {
	img image.Image
	dpi int
}

// decodeURF reads Apple raster: "UNIRAST\0", a page count, then for each page a 32-byte header (bits per pixel,
// colour space, duplex, quality, ..., width, height, dpi) and the packed pixels (see readRasterPage).
func decodeURF(r *bufio.Reader) ([]urfPage, error) {
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic[:7]) != "UNIRAST" {
		return nil, errors.New("not an Apple raster document")
	}
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	var pages []urfPage
	for p := uint32(0); count == 0 || p < count; p++ { // some senders say 0 pages: read until the end
		hdr := make([]byte, 32)
		if _, err := io.ReadFull(r, hdr); err != nil {
			if err == io.EOF && len(pages) > 0 {
				break
			}
			return pages, err
		}
		bpp, w, h, dpi := int(hdr[0]), int(binary.BigEndian.Uint32(hdr[12:])), int(binary.BigEndian.Uint32(hdr[16:])), int(binary.BigEndian.Uint32(hdr[20:]))
		pg, err := readRasterPage(r, bpp, w, h, dpi)
		if err != nil {
			return nil, err
		}
		pages = append(pages, pg)
	}
	return pages, nil
}

// decodePWG reads PWG raster (what Android, Chromebooks and Linux send to "IPP Everywhere" printers): "RaS2",
// then for each page a 1796-byte header and the pixels, packed exactly like Apple raster
func decodePWG(r *bufio.Reader) ([]urfPage, error) {
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "RaS2" {
		return nil, errors.New("not a PWG raster document")
	}
	var pages []urfPage
	for {
		hdr := make([]byte, 1796)
		if _, err := io.ReadFull(r, hdr); err != nil {
			if err == io.EOF && len(pages) > 0 {
				return pages, nil
			}
			if err == io.EOF {
				err = errors.New("no pages in the raster")
			}
			return nil, err
		}
		u := func(at int) int { return int(binary.BigEndian.Uint32(hdr[at:])) }
		dpi, w, h, bitsPerColor, bpp, space := u(276), u(372), u(376), u(384), u(388), u(400)
		if bitsPerColor != 8 || (space != 18 && space != 19) { // only what we ask for: sgray_8 and srgb_8
			return nil, fmt.Errorf("unsupported raster: colour space %d, %d bits", space, bitsPerColor)
		}
		pg, err := readRasterPage(r, bpp, w, h, dpi)
		if err != nil {
			return nil, err
		}
		pages = append(pages, pg)
	}
}

// readRasterPage reads one page of packed pixels (Apple and PWG raster pack them the same way): each line starts
// with how many times it repeats, then runs of repeated pixels (count 0-127: next pixel count+1 times) or literal
// pixels (count 129-255: 257-count pixels follow), and 128 means "the rest of the line is white".
func readRasterPage(r *bufio.Reader, bpp, w, h, dpi int) (urfPage, error) {
	if bpp != 8 && bpp != 24 {
		return urfPage{}, fmt.Errorf("unsupported raster: %d bits per pixel", bpp)
	}
	if w <= 0 || h <= 0 || w > 20000 || h > 30000 || dpi <= 0 {
		return urfPage{}, errors.New("broken raster page")
	}
	ch := bpp / 8
	var img image.Image
	var set func(x, y int, px []byte)
	if ch == 1 {
		g := image.NewGray(image.Rect(0, 0, w, h))
		img, set = g, func(x, y int, px []byte) { g.Pix[y*g.Stride+x] = px[0] }
	} else {
		c := image.NewRGBA(image.Rect(0, 0, w, h))
		img, set = c, func(x, y int, px []byte) {
			i := y*c.Stride + x*4
			c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], 255
		}
	}
	white := bytes.Repeat([]byte{255}, ch)
	line := make([][]byte, w)
	px := make([]byte, ch)
	for y := 0; y < h; {
		rep, err := r.ReadByte()
		if err != nil {
			return urfPage{}, err
		}
		for x := 0; x < w; {
			c, err := r.ReadByte()
			if err != nil {
				return urfPage{}, err
			}
			switch {
			case c == 128:
				for ; x < w; x++ {
					line[x] = white
				}
			case c < 128:
				if _, err := io.ReadFull(r, px); err != nil {
					return urfPage{}, err
				}
				v := append([]byte(nil), px...)
				for k := 0; k <= int(c) && x < w; k++ {
					line[x] = v
					x++
				}
			default:
				for k := 0; k < 257-int(c) && x < w; k++ {
					v := make([]byte, ch)
					if _, err := io.ReadFull(r, v); err != nil {
						return urfPage{}, err
					}
					line[x] = v
					x++
				}
			}
		}
		for k := 0; k <= int(rep) && y < h; k++ {
			for x := 0; x < w; x++ {
				set(x, y, line[x])
			}
			y++
		}
	}
	return urfPage{img: img, dpi: dpi}, nil
}
