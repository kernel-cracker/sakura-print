// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Small pictures of files for the screen.

// ---------- thumbnails ----------

func thumbnail(f *File) (string, error) {
	cache := filepath.Join(dataDir(), "thumbs", f.ID+".jpg")
	if _, err := os.Stat(cache); err == nil {
		return cache, nil
	}
	os.MkdirAll(filepath.Dir(cache), 0o755)
	if f.Kind == "pdf" {
		base := strings.TrimSuffix(cache, ".jpg")
		if o, err := exec.Command("pdftoppm", "-jpeg", "-f", "1", "-l", "1", "-scale-to", "360", "-singlefile", f.Path, base).CombinedOutput(); err != nil {
			return "", fmt.Errorf("pdftoppm: %s", o)
		}
		return cache, nil
	}
	info, _ := loadImage(f.Path)
	srcPath := f.Path
	if info != nil && (info.RawW > proxySide || info.RawH > proxySide) {
		if pp, err := makeProxy(f.Path); err == nil {
			srcPath = pp
		}
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(in)
	in.Close()
	if err != nil {
		return "", err
	}
	small := fastShrink(img, 360)
	if info != nil && info.Rot != 0 {
		small = rotate(small, info.Rot)
	}
	out, err := os.Create(cache)
	if err != nil {
		return "", err
	}
	defer out.Close()
	return cache, jpeg.Encode(out, small, &jpeg.Options{Quality: 80})
}

func rotate(src image.Image, deg int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var dst *image.RGBA
	switch deg {
	case 90, 270:
		dst = image.NewRGBA(image.Rect(0, 0, h, w))
	default:
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			switch deg {
			case 90:
				dst.Set(h-1-y, x, c)
			case 180:
				dst.Set(w-1-x, h-1-y, c)
			case 270:
				dst.Set(y, w-1-x, c)
			default:
				dst.Set(x, y, c)
			}
		}
	}
	return dst
}
