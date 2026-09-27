// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Fast picture handling: small cached copies of big photos for the screen, and a quick shrink.

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
)

const proxySide = 1400 // plenty for a preview on any screen

var proxyLocks sync.Map // one lock per picture, so two requests don't both make the same copy

func proxyPath(path string) string {
	st, _ := os.Stat(path)
	var mt int64
	if st != nil {
		mt = st.ModTime().UnixNano()
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d", path, mt)))
	return filepath.Join(dataDir(), "proxies", hex.EncodeToString(h[:10])+".jpg")
}

// makeProxy writes a small JPEG copy of a big picture (same orientation as the file; the EXIF turn is kept separately)
func makeProxy(path string) (string, error) {
	out := proxyPath(path)
	l, _ := proxyLocks.LoadOrStore(out, &sync.Mutex{})
	mu := l.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", err
	}
	os.MkdirAll(filepath.Dir(out), 0o755)
	tmp := out + ".tmp"
	w, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	err = jpeg.Encode(w, fastShrink(src, proxySide), &jpeg.Options{Quality: 82})
	w.Close()
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return out, os.Rename(tmp, out)
}

// pictureFor loads a picture for a PDF: the real thing for printing, a small copy for previews
func pictureFor(path string, preview bool) (*imgInfo, error) {
	orig, err := loadImage(path)
	if err != nil || !preview || (orig.RawW <= proxySide && orig.RawH <= proxySide) {
		return orig, err
	}
	pp, err := makeProxy(path)
	if err != nil {
		return orig, nil // no small copy? the original still works, just slower
	}
	data, err := os.ReadFile(pp)
	if err != nil {
		return orig, nil
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return orig, nil
	}
	im := &imgInfo{RawW: cfg.Width, RawH: cfg.Height, W: cfg.Width, H: cfg.Height, Rot: orig.Rot, Mime: "image/jpeg", Model: "rgb", Data: data}
	if im.Rot == 90 || im.Rot == 270 {
		im.W, im.H = im.H, im.W
	}
	im.Portr = im.H >= im.W
	return im, nil
}

// fastShrink scales a picture down quickly (reads the JPEG's own brightness/colour planes directly)
func fastShrink(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	scale := float64(max) / float64(w)
	if h > w {
		scale = float64(max) / float64(h)
	}
	nw, nh := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	yc, isYC := src.(*image.YCbCr)
	for y := 0; y < nh; y++ {
		sy0 := b.Min.Y + int(float64(y)/scale)
		sy1 := b.Min.Y + int(float64(y+1)/scale) - 1
		if sy1 < sy0 {
			sy1 = sy0
		}
		if sy1 >= b.Max.Y {
			sy1 = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			sx0 := b.Min.X + int(float64(x)/scale)
			sx1 := b.Min.X + int(float64(x+1)/scale) - 1
			if sx1 < sx0 {
				sx1 = sx0
			}
			if sx1 >= b.Max.X {
				sx1 = b.Max.X - 1
			}
			var r, g, bl int
			for _, pt := range [4][2]int{{sx0, sy0}, {sx1, sy0}, {sx0, sy1}, {sx1, sy1}} { // 4 samples, averaged: smooth, not blocky
				if isYC {
					yi, ci := yc.YOffset(pt[0], pt[1]), yc.COffset(pt[0], pt[1])
					cr, cg, cb := color.YCbCrToRGB(yc.Y[yi], yc.Cb[ci], yc.Cr[ci])
					r, g, bl = r+int(cr), g+int(cg), bl+int(cb)
				} else {
					cr, cg, cb, _ := src.At(pt[0], pt[1]).RGBA()
					r, g, bl = r+int(cr>>8), g+int(cg>>8), bl+int(cb>>8)
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/4), uint8(g/4), uint8(bl/4), 255
		}
	}
	return dst
}
