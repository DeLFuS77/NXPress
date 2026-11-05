package nup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// The unpacking of a NUP into a folder, in the layout the packer of the format reads back: the
// game in base, each update in updates/<version>, each DLC in dlc/<title ID>/<index>. Every
// file is checked by its SHA-256 as it is written.

// PackageFolder is the folder of a package in an unpacked NUP, relative to it.
func (c *Container) PackageFolder(pkg int) string {
	p := c.Packages[pkg]
	switch p.Kind {
	case KindBase:
		return "base"
	case KindUpdate:
		return filepath.Join("updates", strconv.FormatUint(uint64(p.Version), 10))
	case KindAddon:
		index := 0
		for i := 0; i < pkg; i++ {
			if c.Packages[i].Kind == KindAddon && c.Packages[i].TitleID == p.TitleID {
				index++
			}
		}
		return filepath.Join("dlc", fmt.Sprintf("%016x", p.TitleID), strconv.Itoa(index))
	}
	return filepath.Join(fmt.Sprintf("kind%d", p.Kind), fmt.Sprintf("%016x", p.TitleID))
}

// UnpackResult is what an unpacked NUP holds.
type UnpackResult struct {
	Files int
	Size  int64
}

// Unpack writes the packages of a NUP into dest, a folder that must not be there yet; a file
// whose data is not what the table says fails the work, and the folder it made is removed. The
// NUP is read as Verify reads it, by readers that go through it in order (one for a hard disk,
// several for an SSD), each piece of data once: workers decompress it, hash it and write it into
// every file that has it. progress is told the bytes read of the ones to read.
func Unpack(ctx context.Context, path string, dest string, workers int, readers int, progress func(done int64, total int64)) (result UnpackResult, err error) {
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

// walkFolders calls fn for every folder of a package, by its path inside it, empty ones too.
func (c *Container) walkFolders(pkg int, fn func(path string) error) error {
	type frame struct {
		dir    uint32
		prefix string
		depth  int
	}
	seen := make([]bool, len(c.Dirs))
	stack := []frame{{dir: c.Packages[pkg].RootDir}}
	seen[c.Packages[pkg].RootDir] = true
	dirCount := uint32(len(c.Dirs))
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		dir := c.Dirs[top.dir]
		for slot := dir.FirstChild; slot < dir.FirstChild+dir.ChildCount; slot++ {
			node := c.Children[slot]
			if node >= dirCount || seen[node] {
				continue
			}
			if top.depth+1 >= maxDepth {
				return errors.New("NUP folders go too deep")
			}
			seen[node] = true
			path := top.prefix + c.Dirs[node].Name
			if err := fn(path); err != nil {
				return err
			}
			stack = append(stack, frame{dir: node, prefix: path + "/", depth: top.depth + 1})
		}
	}
	return nil
}

// clashes tells two files, or a file and a folder, that would be written to the same place: two
// updates of the same version, or names that differ only in case where the system does not tell
// them apart (Windows and macOS). Found before anything is written, not midway through gigabytes.
func clashes(folders []string, paths [][]string) error {
	key := func(path string) string {
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			return strings.ToLower(path)
		}
		return path
	}
	taken := map[string]bool{}
	for _, folder := range folders {
		taken[key(folder)] = true
	}
	for _, outputs := range paths {
		for _, path := range outputs {
			if taken[key(path)] {
				return fmt.Errorf("NUP holds two files, or a file and a folder, that would be written to the same place: %q", path)
			}
			taken[key(path)] = true
		}
	}
	return nil
}

// windowsNames are the names Windows keeps for its devices, which no file can take.
var windowsNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// safePath tells whether a path of a NUP can be written as it is under the folder of a package:
// names are labels in the format, never paths, so one that climbs out of its folder, or that the
// system cannot take, fails the work instead of going anywhere else.
func safePath(path string) error {
	for _, name := range strings.Split(path, "/") {
		bad := name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\\\x00")
		for _, r := range name {
			if r < 0x20 {
				bad = true
			}
		}
		if runtime.GOOS == "windows" {
			base, _, _ := strings.Cut(strings.ToUpper(name), ".")
			bad = bad || strings.ContainsAny(name, `<>:"|?*`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") ||
				windowsNames[strings.TrimRight(base, " ")]
		}
		if bad {
			return fmt.Errorf("NUP holds a file or folder whose name cannot be written here: %q", path)
		}
	}
	return nil
}
