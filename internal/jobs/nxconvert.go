package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/nxpack"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The .nx files and the other formats for emulators: a dump folder or a NUP packed into a .nx
// (--to-nx), a .nx turned into a dump folder (Decompress) or into a NUP (Combine, --nup), and a
// .nx checked whole (Verify). A NUP becomes a .nx through a folder of its own next to it, and a
// .nx a NUP the same way; the folder goes once the new file is checked.

// verifyNx checks a .nx whole: every file of every version by its SHA-256, every block by its
// CRC-32C.
func verifyNx(ctx context.Context, path string, out Output) (string, string, error) {
	data, err := switchfs.OpenNxData(path)
	if err != nil {
		return "", "", fmt.Errorf("cannot be read: %w", err)
	}
	defer data.Close()
	if err := data.Verify(ctx, out.Progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	versions := 0
	for _, app := range data.Applications {
		versions += len(app.Snapshots)
	}
	return "", count(versions, "version", "versions") + " checked, every file by its SHA-256", nil
}

// unpackNx writes the versions of a .nx into a dump folder named as it is, without .nx.
func unpackNx(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	name, err := target(o, strings.TrimSuffix(path, filepath.Ext(path)))
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(name); err == nil {
		return "", "", Skipped(fmt.Sprintf("%s is already there: move or rename it to unpack the .nx again", filepath.Base(name)))
	}
	written := temporary(name)
	if _, err := os.Lstat(written); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", written)
	}
	if err := nxpack.ToDump(ctx, path, written, out.Progress(path, "Unpacking")); err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(name); err == nil {
		return "", "", fmt.Errorf("%s appeared while the .nx was unpacked: the files unpacked are in %s", filepath.Base(name), written)
	}
	if err := os.Rename(written, name); err != nil {
		os.RemoveAll(written)
		return "", "", err
	}
	note := "every file checked by its SHA-256 as it was written"
	if o.RmSource {
		note = joinNotes(note, keptOriginal)
	}
	return name, note, nil
}

// packNx packs a dump folder or a NUP into a .nx next to it, checked whole before it gets its
// name.
func packNx(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	dir := path
	var name string
	if isNup(path) {
		name = strings.TrimSuffix(path, filepath.Ext(path)) + ".nx"
	} else {
		nupName, err := NupNameWith(path, o.NupTemplate)
		if err != nil {
			return "", "", fmt.Errorf("cannot be read: %w", err)
		}
		name = filepath.Join(filepath.Dir(filepath.Clean(path)), strings.TrimSuffix(nupName, filepath.Ext(nupName))+".nx")
	}
	name, err := newFile(ctx, o, path, name)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	if _, err := os.Lstat(written); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", written)
	}
	defer os.Remove(written)
	workers := max(1, threads)
	if isNup(path) {
		// the NUP into a folder of its own beside the .nx, checked as it is written
		dir = temporary(name) + "-folder"
		if _, err := os.Lstat(dir); err == nil {
			return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", dir)
		}
		defer os.RemoveAll(dir)
		if _, err := nup.Unpack(ctx, path, dir, workers, nupReaders(path, workers), out.Progress(path, "Unpacking")); err != nil {
			return "", "", err
		}
	}
	game, notes, err := nxpack.FromDump(dir)
	if err != nil {
		return "", "", err
	}
	level := nupLevel
	if o.LevelGiven {
		level = o.Level
	}
	result, err := nxpack.Write(ctx, written, []nxpack.Game{game}, nxpack.Options{Level: level, Workers: workers}, out.Progress(path, "Packaging"))
	if err != nil {
		return "", "", err
	}
	if _, _, err := verifyNx(ctx, written, out); err != nil {
		return "", "", fmt.Errorf("the new .nx cannot be read back: %w", err)
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s in %s, %s → %s, checked whole", count(result.Files, "file", "files"), count(len(game.Versions), "version", "versions"),
		size(result.Size), size(result.FileSize))
	return name, joinNotes(append([]string{note}, notes...)...), nil
}

// nupFromNx packs a .nx into a NUP next to it, through a folder of its own.
func nupFromNx(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	dir := temporary(path) + "-folder"
	if _, err := os.Lstat(dir); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", dir)
	}
	if err := nxpack.ToDump(ctx, path, dir, out.Progress(path, "Unpacking")); err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	// the folder is named as the .nx, so the NUP is named and placed by it
	named := filepath.Join(filepath.Dir(dir), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if _, err := os.Lstat(named); err == nil {
		named = dir
	} else if err := os.Rename(dir, named); err != nil {
		named = dir
	} else {
		defer os.RemoveAll(named)
	}
	name, note, err := packNup(ctx, named, o, threads, out)
	if err != nil {
		return "", "", err
	}
	return name, note, nil
}
