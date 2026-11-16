package nup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// UnpackSelected is Unpack for some of the packages of a NUP: the ones keep tells of, by their
// number. It is the code of Unpack, with that one test added, so that a package that is not asked
// for is not read from the NUP nor written. Split uses it to unpack the game, the updates or the DLC
// the person chose, each into the folder of its package inside dest.
func UnpackSelected(ctx context.Context, path string, dest string, workers int, readers int, keep func(pkg int) bool, progress func(done int64, total int64)) (result UnpackResult, err error) {
	source, err := os.Open(path)
	if err != nil {
		return UnpackResult{}, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return UnpackResult{}, err
	}
	container, err := Read(source, info.Size())
	if err != nil {
		return UnpackResult{}, err
	}
	// every path of every file, and every folder, checked before anything is written
	paths := make([][]string, len(container.Files))
	folders := []string{}
	for i, pkg := range container.Packages {
		if pkg.Kind > KindAddon {
			return UnpackResult{}, fmt.Errorf("NUP package %d has kind %d, which NUP version 1 keeps for later", i, pkg.Kind)
		}
		// the packages not asked for are not read, not written and not counted
		if keep != nil && !keep(i) {
			continue
		}
		root := filepath.Join(dest, container.PackageFolder(i))
		folders = append(folders, root)
		err := container.walkFolders(i, func(folder string) error {
			if err := safePath(folder); err != nil {
				return err
			}
			folders = append(folders, filepath.Join(root, filepath.FromSlash(folder)))
			return nil
		})
		if err != nil {
			return UnpackResult{}, err
		}
		err = container.Walk(i, func(name string, index int) error {
			if err := safePath(name); err != nil {
				return err
			}
			// packers give each file of each package a record of its own: a record a crafted NUP
			// puts under many paths would be written again for each of them, past what the card
			// and the check of the free space tell
			if len(paths[index]) > 0 {
				return fmt.Errorf("NUP holds the file %q at more than one place", name)
			}
			paths[index] = append(paths[index], filepath.Join(root, filepath.FromSlash(name)))
			result.Files++
			result.Size += int64(container.Files[index].Size)
			return nil
		})
		if err != nil {
			return UnpackResult{}, err
		}
	}
	if err := clashes(folders, paths); err != nil {
		return UnpackResult{}, err
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return UnpackResult{}, err
	}
	// the folder made here, and only it, goes when the work fails
	defer func() {
		if err != nil {
			os.RemoveAll(dest)
		}
	}()
	for _, folder := range folders {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return UnpackResult{}, err
		}
	}

	// the pieces of data, each once with the files it is written into; the files of 0 bytes are
	// written at once
	type key struct {
		offset, stored, size uint64
		codec                byte
	}
	byKey := map[key]*extentJob{}
	empty := sha256.Sum256(nil)
	for i, f := range container.Files {
		if len(paths[i]) == 0 {
			continue
		}
		if f.Size == 0 {
			if f.Hash != empty {
				return UnpackResult{}, fmt.Errorf("%s: its SHA-256 does not match", paths[i][0])
			}
			for _, path := range paths[i] {
				out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
				if err != nil {
					return UnpackResult{}, err
				}
				if err := out.Close(); err != nil {
					return UnpackResult{}, err
				}
			}
			continue
		}
		k := key{f.ExtentOffset, f.ExtentSize, f.Size, f.Codec}
		job, ok := byKey[k]
		if !ok {
			job = &extentJob{offset: f.ExtentOffset, stored: f.ExtentSize, size: f.Size, codec: f.Codec}
			byKey[k] = job
		}
		job.files = append(job.files, i)
		job.outputs = append(job.outputs, paths[i]...)
	}
	jobs := make([]*extentJob, 0, len(byKey))
	total := int64(0)
	for _, job := range byKey {
		jobs = append(jobs, job)
		total += int64(job.stored)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].offset != jobs[j].offset {
			return jobs[i].offset < jobs[j].offset
		}
		return jobs[i].stored < jobs[j].stored
	})
	if progress != nil {
		progress(0, total)
	}
	if _, err := readAll(ctx, source, jobs, max(1, workers), max(1, readers), func(done int64) {
		if progress != nil {
			progress(done, total)
		}
	}); err != nil {
		return UnpackResult{}, err
	}
	for _, job := range jobs {
		for _, index := range job.files {
			where := paths[index][0]
			switch {
			case job.reason != "":
				return UnpackResult{}, fmt.Errorf("%s: %s", where, job.reason)
			case container.Files[index].Hash != job.sum:
				return UnpackResult{}, fmt.Errorf("%s: its SHA-256 does not match", where)
			}
		}
	}
	if progress != nil {
		progress(total, total)
	}
	return result, nil
}
