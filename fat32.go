package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A file bigger than FAT32 holds (4 GiB) goes to a microSD card in parts: a folder with the
// name of the file and the archive bit, holding 00, 01... The console reads such a folder as
// one file. Parts of 4 GiB less 64 KiB, as the tools that dump games write them.
const fat32Limit = 1<<32 - 1

// fat32Part is the size of each part (a variable, so the tests split small files)
var fat32Part int64 = 0xFFFF0000

// splitContainer tells whether path is a file in parts: a folder of a game file holding 00,
// or the first part of one split as name.ns0 (or .nz0, .xc0, .xz0). It gives the name of the
// file it makes whole.
func splitContainer(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		if _, ok := gameExtension(path); !ok {
			return "", false
		}
		if _, err := os.Stat(filepath.Join(path, "00")); err != nil {
			return "", false
		}
		return path, true
	}
	whole := map[string]string{".ns0": ".nsp", ".nz0": ".nsz", ".xc0": ".xci", ".xz0": ".xcz"}
	extension := strings.ToLower(filepath.Ext(path))
	if to, ok := whole[extension]; ok {
		return strings.TrimSuffix(path, filepath.Ext(path)) + to, true
	}
	return "", false
}

// splitPart tells whether path is a part after the first of a file split as name.ns1...
func splitPart(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	if len(extension) < 4 {
		return false
	}
	switch extension[:3] {
	case ".ns", ".nz", ".xc", ".xz":
		n, err := strconv.Atoi(extension[3:])
		return err == nil && n > 0
	}
	return false
}

func gameExtension(path string) (string, bool) {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".nsp", ".nsz", ".xci", ".xcz":
		return extension, true
	}
	return "", false
}

// fat32 splits a file bigger than FAT32 holds into a folder of parts, or makes a file in parts
// whole again. Without an output folder the file is replaced, once its new copy is checked.
func fat32(ctx context.Context, path string, o options, out output) (string, string, error) {
	if whole, ok := splitContainer(path); ok {
		return joinParts(ctx, path, whole, o, out)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	if info.Size() <= fat32Limit {
		return "", "it fits in FAT32 as it is", nil
	}
	return splitParts(ctx, path, info.Size(), o, out)
}

func splitParts(ctx context.Context, path string, size int64, o options, out output) (string, string, error) {
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	os.RemoveAll(written)
	defer os.RemoveAll(written)
	if err := os.Mkdir(written, 0o755); err != nil {
		return "", "", err
	}
	input, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer input.Close()
	sum := sha256.New()
	progress := out.progress(path, "Splitting")
	var done int64
	parts := 0
	for done < size {
		length := min(fat32Part, size-done)
		part, err := os.Create(filepath.Join(written, fmt.Sprintf("%02d", parts)))
		if err != nil {
			return "", "", err
		}
		err = copyWithProgress(ctx, io.MultiWriter(part, sum), io.NewSectionReader(input, done, length), length, func(n int64, _ int64) { progress(done+n, size) })
		if closeErr := part.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return "", "", err
		}
		done += length
		parts++
	}
	// the parts, read again as one file, must give the original
	if err := checkSum(ctx, filepath.Join(written, "00"), size, sum.Sum(nil), out.progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	// Windows deletes no file still open
	input.Close()
	// the parts are on the disk before the original goes
	if err := syncPath(written); err != nil {
		return "", "", err
	}
	if inPlace {
		// the folder takes the name of the file: the file goes first
		if err := os.Remove(path); err != nil {
			return "", "", err
		}
	} else if o.overwrite {
		os.RemoveAll(name)
	}
	if err := commitFile(written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%d parts in a folder", parts)
	if archiveBit(name) != nil {
		note += "; set its archive bit on the microSD (Hekate: Tools, Archive bit)"
	}
	if inPlace {
		return name, note, nil
	}
	// the parts, read again as one file, gave the original byte for byte
	return removeSource(o, path, switchfs.FileCheck{}, name, note)
}

func joinParts(ctx context.Context, path string, whole string, o options, out output) (string, string, error) {
	first := path
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		first = filepath.Join(path, "00")
	}
	reader, err := switchfs.OpenFile(first)
	if err != nil {
		return "", "", err
	}
	defer reader.Close()
	size, parts, err := partsSize(path)
	if err != nil {
		return "", "", err
	}
	name, err := target(o, whole)
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err != nil {
		return "", "", err
	}
	// in place, into the folder of the parts however it is written, the parts give way to the
	// whole file; a folder of parts gives it its own name
	inPlace := o.output == "" || sameFile(filepath.Dir(name), filepath.Dir(path))
	if !sameFile(name, path) {
		if err := free(o, name); err != nil {
			return "", "", err
		}
	}
	written := temporary(name)
	defer os.Remove(written)
	output, err := os.Create(written)
	if err != nil {
		return "", "", err
	}
	sum := sha256.New()
	progress := out.progress(path, "Joining")
	err = copyWithProgress(ctx, io.MultiWriter(output, sum), io.NewSectionReader(reader, 0, size), size, progress)
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", "", err
	}
	if err := checkSum(ctx, written, size, sum.Sum(nil), out.progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	reader.Close()
	// the whole file is on the disk before the parts go
	if err := syncPath(written); err != nil {
		return "", "", err
	}
	// the parts give way to the whole file, in place or when asked to
	if inPlace || o.rmSource {
		if err := removeParts(path); err != nil {
			return "", "", err
		}
	}
	if o.overwrite {
		os.Remove(name)
	}
	if err := commitFile(written, name); err != nil {
		return "", "", err
	}
	return name, fmt.Sprintf("%d parts joined", parts), nil
}

// partsSize is the size of a file in parts, and how many parts it has.
func partsSize(path string) (int64, int, error) {
	var names []string
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		for n := 0; ; n++ {
			part := filepath.Join(path, fmt.Sprintf("%02d", n))
			if _, err := os.Stat(part); err != nil {
				break
			}
			names = append(names, part)
		}
	} else {
		base := strings.TrimSuffix(path, "0")
		for n := 0; ; n++ {
			part := base + strconv.Itoa(n)
			if _, err := os.Stat(part); err != nil {
				break
			}
			names = append(names, part)
		}
	}
	var size int64
	for _, name := range names {
		info, err := os.Stat(name)
		if err != nil {
			return 0, 0, err
		}
		size += info.Size()
	}
	if len(names) == 0 {
		return 0, 0, errors.New("no parts found")
	}
	return size, len(names), nil
}

// removeParts deletes a file in parts: its folder, or its parts name.ns0, name.ns1...
func removeParts(path string) error {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return os.RemoveAll(path)
	}
	base := strings.TrimSuffix(path, "0")
	for n := 0; ; n++ {
		part := base + strconv.Itoa(n)
		if _, err := os.Stat(part); err != nil {
			return nil
		}
		if err := os.Remove(part); err != nil {
			return err
		}
	}
}

// copyWithProgress copies size bytes, telling the progress when asked.
func copyWithProgress(ctx context.Context, output io.Writer, input io.Reader, size int64, progress func(done int64, total int64)) error {
	buffer := make([]byte, 4<<20)
	var done int64
	for done < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(input, buffer[:min(int64(len(buffer)), size-done)])
		if n > 0 {
			if _, werr := output.Write(buffer[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, size)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// checkSum reads a file (or the first of its parts) again and compares its SHA-256.
func checkSum(ctx context.Context, path string, size int64, want []byte, progress func(done int64, total int64)) error {
	reader, err := switchfs.OpenFile(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	sum := sha256.New()
	if err := copyWithProgress(ctx, sum, io.NewSectionReader(reader, 0, size), size, progress); err != nil {
		return err
	}
	if !bytes.Equal(sum.Sum(nil), want) {
		return errors.New("the new copy on the disk differs from the original")
	}
	return nil
}
