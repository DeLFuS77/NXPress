package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// SpaceShort is a drive where the new files of a work may not fit: the folder they go to, the
// most they may take there at once, and what is free.
type SpaceShort struct {
	Folder string `json:"folder"`
	Need   int64  `json:"need"`
	Free   int64  `json:"free"`
}

// SpaceCheck tells the drives where the new files of a request may not fit, before it starts.
func (a *App) SpaceCheck(request Request) []SpaceShort {
	return spaceShort(request, freeSpace)
}

// spaceShort weighs each new file by the most it can take: as big as its original, and an NSZ
// or XCZ decompressed as big as its content. A file that replaces its original on the same
// drive gives its room back once it is done, so of those only the ones made at once count.
func spaceShort(request Request, free func(string) (int64, error)) []SpaceShort {
	switch request.Mode {
	case "compress", "decompress", "recompress", "trim", "convert", "fat32", "split", "pack":
	default:
		return nil
	}
	replaces := request.RemoveSource || request.Output == "" && (request.Mode == "recompress" || request.Mode == "trim" || request.Mode == "fat32")
	type drive struct {
		folder string
		kept   int64
		freed  []int64
	}
	drives := map[string]*drive{}
	order := []string{}
	for _, path := range request.Files {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		need := info.Size()
		if request.Mode == "decompress" {
			if facts, err := switchfs.DescribeFile(path); err == nil && facts.DecompressedSize > need {
				need = facts.DecompressedSize
			}
		}
		folder := filepath.Dir(path)
		switch {
		case request.Mode == "pack" && !request.PackGames && request.Pack != "":
			folder = filepath.Dir(request.Pack)
		case request.Output != "":
			folder = request.Output
		}
		key := volumeOf(folder)
		if drives[key] == nil {
			drives[key] = &drive{folder: folder}
			order = append(order, key)
		}
		if replaces && request.Mode != "pack" && key == volumeOf(filepath.Dir(path)) {
			drives[key].freed = append(drives[key].freed, need)
		} else {
			drives[key].kept += need
		}
	}
	short := []SpaceShort{}
	for _, key := range order {
		d := drives[key]
		sort.Slice(d.freed, func(i, j int) bool { return d.freed[i] > d.freed[j] })
		need := d.kept
		for i := 0; i < len(d.freed) && i < max(1, request.Multi); i++ {
			need += d.freed[i]
		}
		left, err := free(d.folder)
		if err == nil && need > left {
			short = append(short, SpaceShort{Folder: d.folder, Need: need, Free: left})
		}
	}
	return short
}

// volumeOf names the drive of a folder: its letter or share on Windows, the folder elsewhere.
func volumeOf(folder string) string {
	if volume := filepath.VolumeName(folder); volume != "" {
		return strings.ToLower(volume)
	}
	return folder
}

// existing is the folder itself, or the nearest folder above it that exists: an output folder is
// made when the work starts.
func existing(folder string) string {
	for {
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			return folder
		}
		parent := filepath.Dir(folder)
		if parent == folder {
			return folder
		}
		folder = parent
	}
}
