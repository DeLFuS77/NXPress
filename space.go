package main

import (
	"os"
	"path/filepath"
	"sort"

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

// spaceShort weighs each new file by the most it can take: as big as its original (all its
// parts, for a file in parts), and an NSZ or XCZ decompressed as big as its content.
func spaceShort(request Request, free func(string) (int64, error)) []SpaceShort {
	switch request.Mode {
	case "compress", "decompress", "recompress", "trim", "convert", "fat32", "split", "pack":
	default:
		return nil
	}
	replaces := request.RemoveSource || request.Output == "" && (request.Mode == "recompress" || request.Mode == "trim" || request.Mode == "fat32")
	files := []spaceFile{}
	for _, path := range request.Files {
		size, ok := originalSize(path)
		if !ok {
			continue
		}
		most := size
		if request.Mode == "decompress" {
			if facts, err := switchfs.DescribeFile(path); err == nil && facts.DecompressedSize > most {
				most = facts.DecompressedSize
			}
		}
		folder := filepath.Dir(path)
		switch {
		case request.Mode == "pack" && !request.PackGames && request.Pack != "":
			folder = filepath.Dir(request.Pack)
		case request.Output != "":
			folder = request.Output
		}
		drive := volumeOf(folder)
		files = append(files, spaceFile{
			folder: folder, drive: drive, size: size, most: most,
			replaces: replaces && request.Mode != "pack" && drive == volumeOf(filepath.Dir(path)),
		})
	}
	return spaceNeeded(files, max(1, request.Multi), free)
}

// spaceFile is a new file of a work: the folder and the drive it goes to, the size of its
// original and the most it may take, and whether it replaces its original on that drive.
type spaceFile struct {
	folder, drive string
	size, most    int64
	replaces      bool
}

// spaceNeeded gives the drives where the new files may not fit at the worst moment of a work,
// whatever the order the files are done in. A file kept beside its original takes all of it for
// good. A file that replaces its original gives that room back once done, so it keeps only what
// it grew by, and that adds up file after file (decompressing with the originals deleted); while
// it is made it stands beside its original, and of those the ones made at once count.
func spaceNeeded(files []spaceFile, multi int, free func(string) (int64, error)) []SpaceShort {
	type drive struct {
		folder string
		need   int64
		beside []int64
	}
	drives := map[string]*drive{}
	order := []string{}
	for _, file := range files {
		d := drives[file.drive]
		if d == nil {
			d = &drive{folder: file.folder}
			drives[file.drive] = d
			order = append(order, file.drive)
		}
		if !file.replaces {
			d.need += file.most
			continue
		}
		grows := max(0, file.most-file.size)
		d.need += grows
		d.beside = append(d.beside, file.most-grows)
	}
	short := []SpaceShort{}
	for _, key := range order {
		d := drives[key]
		sort.Slice(d.beside, func(i, j int) bool { return d.beside[i] > d.beside[j] })
		need := d.need
		for i := 0; i < len(d.beside) && i < multi; i++ {
			need += d.beside[i]
		}
		left, err := free(d.folder)
		if err == nil && need > left {
			short = append(short, SpaceShort{Folder: d.folder, Need: need, Free: left})
		}
	}
	return short
}

// originalSize is the size of a file of a work: all its parts, for a file in parts.
func originalSize(path string) (int64, bool) {
	if _, ok := splitContainer(path); ok {
		size, _, err := partsSize(path)
		return size, err == nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, false
	}
	return info.Size(), true
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
