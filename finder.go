// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

// Finding PDFs and pictures anywhere in the home folder, not just Downloads/Desktop/Documents.
// A background index keeps searching instant; it skips hidden folders, caches, games and code dependencies.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FoundFile struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Dir   string `json:"dir"` // folder, shown like ~/School/Grade 8
	Kind  string `json:"kind"`
	MTime int64  `json:"mtime"`
	Size  int64  `json:"size"`
}

var (
	index     []FoundFile
	indexTime time.Time
	indexMu   sync.Mutex
	indexing  bool
)

// folders that never hold things you'd want to print (or are huge and slow to walk)
var skipDirs = map[string]bool{
	"node_modules": true, "__pycache__": true, "venv": true, "site-packages": true, "target": true,
	"snap": true, "Steam": true, "steamapps": true, "SteamLibrary": true, "flatpak": true, "Trash": true,
	"proc": true, "sys": true, "dev": true, "vendor": true, "bower_components": true, "dist-packages": true,
	"CMakeFiles": true, "DerivedData": true, "Cache": true, "cache": true, "GPUCache": true, "Code Cache": true,
}

const (
	indexMax      = 30000
	indexMaxDepth = 12
	indexBudget   = 25 * time.Second
)

func buildIndex() {
	indexMu.Lock()
	if indexing {
		indexMu.Unlock()
		return
	}
	indexing = true
	indexMu.Unlock()

	h := home()
	ours := dataDir()
	start := time.Now()
	var found []FoundFile
	filepath.WalkDir(h, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if time.Since(start) > indexBudget || len(found) >= indexMax {
			return filepath.SkipAll
		}
		name := d.Name()
		if d.IsDir() {
			if p != h && (strings.HasPrefix(name, ".") || skipDirs[name] || p == ours ||
				strings.Count(strings.TrimPrefix(p, h), string(os.PathSeparator)) > indexMaxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		kind := ""
		switch strings.ToLower(filepath.Ext(name)) {
		case ".pdf":
			kind = "pdf"
		case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".avif", ".webp", ".gif", ".bmp", ".tif", ".tiff":
			kind = "image"
		case ".doc", ".docx", ".odt", ".rtf", ".xls", ".xlsx", ".ods", ".csv", ".ppt", ".pptx", ".odp", ".txt":
			kind = "office" // printed through LibreOffice (convert.go)
		default:
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() == 0 {
			return nil
		}
		dir := filepath.Dir(p)
		label := "~" + strings.TrimPrefix(dir, h)
		if dir == h {
			label = "~"
		}
		found = append(found, FoundFile{Path: p, Name: name, Dir: label, Kind: kind, MTime: info.ModTime().Unix(), Size: info.Size()})
		return nil
	})
	sort.Slice(found, func(i, j int) bool { return found[i].MTime > found[j].MTime })

	indexMu.Lock()
	index, indexTime, indexing = found, time.Now(), false
	indexMu.Unlock()
}

// searchFiles answers from the index; kind: doc (PDFs + pictures), pdf, image
func searchFiles(kind, q string, limit int) (out []FoundFile, total int, ready bool) {
	indexMu.Lock()
	idx, when, busy := index, indexTime, indexing
	indexMu.Unlock()
	if !busy && time.Since(when) > time.Minute {
		go buildIndex() // freshen up in the background; answer from what we have right now
	}
	words := strings.Fields(strings.ToLower(q))
	out = []FoundFile{}
	for _, f := range idx {
		if (kind == "pdf" && f.Kind != "pdf") || (kind == "image" && f.Kind != "image") || (kind == "office" && f.Kind != "office") {
			continue
		}
		if len(words) > 0 {
			hay := strings.ToLower(f.Name + " " + f.Dir)
			ok := true
			for _, w := range words {
				if !strings.Contains(hay, w) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		total++
		if len(out) < limit {
			out = append(out, f)
		}
	}
	return out, total, !when.IsZero()
}
