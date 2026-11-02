// Package space tells whether the new files of a work fit on the drives they go to.
package space

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/DeLFuS77/NXPress/internal/jobs"
)

// Short is a drive where the new files of a work may not fit: the folder they go to, the
// most they may take there at once, and what is free.
type Short struct {
	Folder string `json:"folder"`
	Need   int64  `json:"need"`
	Free   int64  `json:"free"`
}

// File is a new file of a work: the folder and the drive it goes to, the size of its
// original and the most it may take, and whether it replaces its original on that drive.
type File struct {
	Folder, Drive string
	Size, Most    int64
	Replaces      bool
}

// Needed gives the drives where the new files may not fit at the worst moment of a work,
// whatever the order the files are done in. A file kept beside its original takes all of it for
// good. A file that replaces its original gives that room back once done, so it keeps only what
// it grew by, and that adds up file after file (decompressing with the originals deleted); while
// it is made it stands beside its original, and of those the ones made at once count.
func Needed(files []File, multi int, free func(string) (int64, error)) []Short {
	type drive struct {
		folder string
		need   int64
		beside []int64
	}
	drives := map[string]*drive{}
	order := []string{}
	for _, file := range files {
		d := drives[file.Drive]
		if d == nil {
			d = &drive{folder: file.Folder}
			drives[file.Drive] = d
			order = append(order, file.Drive)
		}
		if !file.Replaces {
			d.need += file.Most
			continue
		}
		grows := max(0, file.Most-file.Size)
		d.need += grows
		d.beside = append(d.beside, file.Most-grows)
	}
	short := []Short{}
	for _, key := range order {
		d := drives[key]
		sort.Slice(d.beside, func(i, j int) bool { return d.beside[i] > d.beside[j] })
		need := d.need
		for i := 0; i < len(d.beside) && i < multi; i++ {
			need += d.beside[i]
		}
		left, err := free(d.folder)
		if err == nil && need > left {
			short = append(short, Short{Folder: d.folder, Need: need, Free: left})
		}
	}
	return short
}

// OriginalSize is the size of a file of a work: all its parts, for a file in parts.
func OriginalSize(path string) (int64, bool) {
	if _, ok := jobs.SplitContainer(path); ok {
		size, _, err := jobs.PartsSize(path)
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
