// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Files the person added: uploads, the computer's own files, scans. Each gets an id the web app uses.

// ---------- files the user added (uploads, laptop files, scans) ----------

type File struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // pdf | image
	Pages int    `json:"pages"`
	Path  string `json:"-"`
}

var (
	files   = map[string]*File{}
	filesMu sync.Mutex
	builds  = map[string]*Built{}
	jobs    = map[string]*Job{}
	stateMu sync.Mutex
	saved   = map[string]string{} // download token -> path
)

func getFile(id string) (*File, bool) {
	filesMu.Lock()
	defer filesMu.Unlock()
	f, ok := files[id]
	return f, ok
}

func addFile(path, name string) (*File, error) {
	// any kind of file: HEIC photos, WebP, Word, Excel... become a PDF, JPEG or PNG first (convert.go)
	path, err := normalize(path, name)
	if err != nil {
		return nil, err
	}
	head := make([]byte, 16)
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	n, _ := io.ReadFull(fh, head)
	fh.Close()
	head = head[:n]
	f := &File{ID: newID(), Name: name, Path: path}
	switch {
	case strings.HasPrefix(string(head), "%PDF"):
		f.Kind = "pdf"
		f.Pages = npages(path)
		if f.Pages < 1 {
			return nil, fmt.Errorf("%s can't be read (password protected or broken?)", name)
		}
	default:
		info, err := loadImage(path)
		if err != nil {
			lower := strings.ToLower(name)
			if strings.HasSuffix(lower, ".heic") || strings.HasSuffix(lower, ".heif") {
				return nil, fmt.Errorf("%s is a HEIC photo. On iPhone: Settings → Camera → Formats → Most Compatible, or share it as JPEG", name)
			}
			return nil, fmt.Errorf("%s isn't a PDF, JPEG or PNG", name)
		}
		f.Kind = "image"
		f.Pages = 1
		if info.RawW > proxySide || info.RawH > proxySide {
			go makeProxy(path) // big photo: make its small copy in the background, ready by the time the print screen asks
		}
	}
	filesMu.Lock()
	files[f.ID] = f
	filesMu.Unlock()
	return f, nil
}
