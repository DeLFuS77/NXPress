package jobs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/system"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// NUP files are made of dump folders, in Combine (--nup), and turned back into folders, in
// Decompress (-D): the dump folders of OunceCake and of the packer of the format, a game with its
// updates and its DLC as plain ExeFS and RomFS trees.

// dumpFolders, among the extensions taken, takes the dump folders a NUP is made of.
const dumpFolders = "dump"

// nupLevel is the level of zstd of the files of a NUP: 3, as the packer of the format, unless -l
// asks for another.
const nupLevel = 3

// IsDumpFolder tells whether a path is a dump folder a NUP is made of.
func IsDumpFolder(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir() && nup.IsDump(path)
}

// keptOriginal is why --rm-source leaves a dump folder or a NUP where it is.
const keptOriginal = "the original is kept: --rm-source does not delete dump folders or NUP files"

// dumpDeleted is what --rm-dumps did with a dump folder all of whose files its NUP holds.
const dumpDeleted = "the dump folder is deleted: the NUP holds all its files, checked"

// dumpSummary is what a dump folder holds, as the window and the name of its NUP tell it.
type dumpSummary struct {
	sources []nup.Source
	name    string
	main    nup.Source
	latest  uint32
}

func readDump(dir string) (dumpSummary, error) {
	sources, _, err := nup.Discover(dir)
	if err != nil {
		return dumpSummary{}, err
	}
	summary := dumpSummary{sources: sources, main: sources[0]}
	for _, source := range sources {
		if source.Kind == nup.KindUpdate && (summary.main.Kind != nup.KindUpdate || source.Version > summary.latest) {
			summary.latest = source.Version
		}
	}
	if summary.main.Kind == nup.KindUpdate {
		summary.latest = sources[len(sources)-1].Version
		for _, source := range sources {
			if source.Kind == nup.KindUpdate {
				summary.latest = max(summary.latest, source.Version)
			}
		}
	}
	summary.name = nup.ControlName(summary.main.Control)
	if summary.name == "" {
		// the name of the folder, without the title ID and the version it may tell already
		summary.name = strings.TrimSpace(folderTags.ReplaceAllString(filepath.Base(filepath.Clean(dir)), ""))
	}
	if summary.name == "" {
		summary.name = filepath.Base(filepath.Clean(dir))
	}
	return summary, nil
}

// folderTags are the title ID and the version the name of a folder may have: [0100...][v1.2.3]
var folderTags = regexp.MustCompile(`\s*\[(?:[0-9A-Fa-f]{16}|[vV][0-9.]+)\]`)

var (
	// any tag of the name of a folder: [0100...], [v1.2.3], [ROM]
	dumpTags = regexp.MustCompile(`\s*\[[^\]]*\]`)
	// a version at the end of the name: _v1.1.1, v65536
	dumpVersion = regexp.MustCompile(`(?i)[\s_-]*v\d+(?:\.\d+)*$`)
	// the word dumps are often named with
	dumpRom = regexp.MustCompile(`(?i)\bROM\b`)
	// words written together: MarioTennisFever
	dumpWords = regexp.MustCompile(`(\p{Ll})(\p{Lu})`)
)

// dumpGameName is the name a NUP carries for a dump folder without control data, so emulators show
// a name and not the label of the dump: the name of the folder without its tags, the version at
// its end and the word ROM, its underscores as spaces and its words set apart (MarioTennisFever_v1.1.1
// is Mario Tennis Fever). A folder with control data gives none: the NUP takes the name of the game.
func dumpGameName(dir string) string {
	if summary, err := readDump(dir); err != nil || nup.ControlName(summary.main.Control) != "" {
		return ""
	}
	return dumpNameOf(filepath.Base(filepath.Clean(dir)))
}

// dumpNameOf is the name of a game the name of its dump folder gives (see dumpGameName).
func dumpNameOf(folder string) string {
	name := dumpTags.ReplaceAllString(folder, "")
	name = dumpVersion.ReplaceAllString(strings.TrimSpace(strings.ReplaceAll(name, "_", " ")), "")
	name = dumpWords.ReplaceAllString(dumpRom.ReplaceAllString(name, ""), "$1 $2")
	return strings.Join(strings.Fields(name), " ")
}

// NupName is the name of the NUP of a dump folder: the name of the game, from its control data
// when the folder has it, else the name of the folder, then its title ID and newest version.
func NupName(dir string) (string, error) {
	summary, err := readDump(dir)
	if err != nil {
		return "", err
	}
	id := nup.Package{TitleID: summary.main.TitleID, Kind: summary.main.Kind}.TitleIDText()
	name := cleanName(fmt.Sprintf("%s [%s][v%d]", summary.name, id, summary.latest))
	return name + ".nup", nil
}

// dumpTitle is the main title of a dump folder, as ReadTitle gives the one of a game file, with
// the size of its files.
func dumpTitle(dir string) (GameTitle, error) {
	summary, err := readDump(dir)
	if err != nil {
		return GameTitle{}, err
	}
	main := summary.main
	kind := map[byte]string{nup.KindBase: "BASE", nup.KindUpdate: "UPD", nup.KindAddon: "DLC"}[main.Kind]
	result := GameTitle{Name: summary.name, TitleID: nup.Package{TitleID: main.TitleID, Kind: main.Kind}.TitleIDText(),
		Version: int(main.Version), Type: kind, Titles: len(summary.sources), Latest: int(summary.latest), Container: "DUMP"}
	if main.Kind == nup.KindBase && summary.latest == 0 {
		result.Latest = int(main.Version)
	}
	for _, source := range summary.sources {
		switch source.Kind {
		case nup.KindBase:
			result.Bases++
		case nup.KindUpdate:
			result.Updates++
		case nup.KindAddon:
			result.Dlcs++
		}
	}
	if main.Kind != nup.KindBase {
		if game, err := switchfs.GameTitleID(result.TitleID, kind); err == nil {
			result.GameID = fmt.Sprintf("%016X", game)
		}
	}
	for _, source := range summary.sources {
		result.Pieces = append(result.Pieces, DumpPiece{Type: map[byte]string{nup.KindBase: "BASE", nup.KindUpdate: "UPD", nup.KindAddon: "DLC"}[source.Kind],
			TitleID: nup.Package{TitleID: source.TitleID, Kind: source.Kind}.TitleIDText(), Version: int(source.Version)})
	}
	result.NupName, _ = NupName(dir)
	result.Size = DumpSize(dir)
	return result, nil
}

// DumpSize is the size of the files of a dump folder, the listings of FTP clients left out.
func DumpSize(dir string) int64 {
	total := int64(0)
	filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() != ".listing" {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// NupUncompressed is the size of the files of a NUP uncompressed, each written once per path,
// as unpacking writes them: 0 when it cannot be read.
func NupUncompressed(path string) int64 {
	container, err := nup.Open(path)
	if err != nil {
		return 0
	}
	return int64(container.Totals().Size)
}

// packNup makes the NUP of a dump folder, next to it or in the output folder: written under a
// hidden name, read back whole and checked by the hashes of its files, then given its name.
func packNup(ctx context.Context, dir string, o Options, threads int, out Output) (string, string, error) {
	name, err := NupName(dir)
	if err != nil {
		return "", "", fmt.Errorf("cannot be read: %w", err)
	}
	name, err = newFile(ctx, o, dir, filepath.Join(filepath.Dir(filepath.Clean(dir)), name))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	// the hidden file of a work that stopped is not this one's to remove
	if _, err := os.Lstat(written); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", written)
	}
	defer os.Remove(written)
	level := nupLevel
	if o.LevelGiven {
		level = o.Level
	}
	// every worker reads at once on an SSD, where many small files open faster so; one at a time on
	// a hard disk, and on a drive that does not tell what it is
	workers, readers := max(1, threads), 1
	if penalty, known := system.SeekPenalty(dir); known && !penalty {
		readers = workers
	}
	// a file of the folder changed after this is not the one packed: --rm-dumps leaves it
	started := time.Now()
	packing := nup.PackOptions{Level: level, Workers: workers, Readers: readers, Name: dumpGameName(dir)}
	if !o.NupZstdBig {
		packing.RawOver = nup.RawBig
	}
	result, err := nup.Pack(ctx, dir, written, packing, out.Progress(dir, "Packaging"))
	if err != nil {
		return "", "", err
	}
	checkers := nupWorkers(o)
	_, check, err := nup.Verify(ctx, written, checkers, nupReaders(written, checkers), out.Progress(dir, "Verifying"))
	if err != nil {
		return "", "", fmt.Errorf("the new NUP cannot be read back: %w", err)
	}
	if len(check.Damaged) > 0 {
		return "", "", fmt.Errorf("the new NUP does not read back whole: %d files are not what was packed", len(check.Damaged))
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s in %s, %s → %s", count(result.Files, "file", "files"), count(len(result.Packages), "package", "packages"),
		size(result.Size), size(result.FileSize))
	details := []string{}
	if result.Shared > 0 {
		details = append(details, fmt.Sprintf("%d with the data of another file, kept once", result.Shared))
	}
	if result.Zstd > 0 {
		details = append(details, fmt.Sprintf("%d with zstd", result.Zstd))
	}
	if result.RawBig > 0 {
		details = append(details, fmt.Sprintf("%d over 256 MiB stored as they are, for Eglantine and Lichii", result.RawBig))
	}
	if len(details) > 0 {
		note += ": " + strings.Join(details, ", ")
	}
	notes := append([]string{note}, result.Notes...)
	switch {
	case o.RmSource && o.RmDumps:
		notes = append(notes, removeDump(dir, name, result.Packages, started))
	case o.RmSource:
		notes = append(notes, keptOriginal)
	}
	return name, joinNotes(notes...), nil
}

// removeDump deletes, with --rm-dumps, the files of a dump folder its NUP holds, the NUP read back
// whole and checked already: each file of a package whose copy in the folder is a plain file of
// the same size, not changed since the packing began; then the folders left empty, the dump
// folder too. A file the NUP does not hold stays, and so does one changed since: the note tells
// how many stay.
func removeDump(dir string, nupPath string, sources []nup.Source, since time.Time) string {
	container, err := nup.Open(nupPath)
	if err != nil {
		return "the dump folder is kept: the NUP cannot be read again: " + err.Error()
	}
	if len(container.Packages) != len(sources) {
		return "the dump folder is kept: the NUP does not hold the packages of the folder"
	}
	var held []string
	for i, source := range sources {
		if pkg := container.Packages[i]; pkg.Kind != source.Kind || pkg.Version != source.Version {
			return "the dump folder is kept: the NUP does not hold the packages of the folder"
		}
		root := filepath.Clean(source.Dir)
		err := container.Walk(i, func(path string, file int) error {
			local := filepath.Join(root, filepath.FromSlash(path))
			if rel, err := filepath.Rel(root, local); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil
			}
			info, err := os.Lstat(local)
			if err == nil && info.Mode().IsRegular() && uint64(info.Size()) == container.Files[file].Size && !info.ModTime().After(since) {
				held = append(held, local)
			}
			return nil
		})
		if err != nil {
			return "the dump folder is kept: " + err.Error()
		}
	}
	if len(held) == 0 {
		return "the dump folder is kept: none of its files is in the NUP as it is now"
	}
	for _, path := range held {
		os.Remove(path)
	}
	// the folders left empty, the deepest first: a folder that still holds something stays
	var folders []string
	filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			folders = append(folders, path)
		}
		return nil
	})
	for i := len(folders) - 1; i >= 0; i-- {
		os.Remove(folders[i])
	}
	left := 0
	filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			left++
		}
		return nil
	})
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return dumpDeleted
	}
	if left == 1 {
		return "1 file stays in the dump folder, which the NUP does not hold as it is: the rest is deleted"
	}
	return fmt.Sprintf("%d files stay in the dump folder, which the NUP does not hold as they are: the rest is deleted", left)
}

// unpackNup turns a NUP into a folder of the same name next to it, or in the output folder: each
// file checked by its SHA-256 as it is written, in a hidden folder given its name once all of
// them are. A folder there already is never written into or replaced.
func unpackNup(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	name, err := target(o, strings.TrimSuffix(path, filepath.Ext(path)))
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(name); err == nil {
		return "", "", Skipped(fmt.Sprintf("%s is already there: move or rename it to unpack the NUP again", filepath.Base(name)))
	}
	written := temporary(name)
	if _, err := os.Lstat(written); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", written)
	}
	workers := max(1, threads)
	result, err := nup.Unpack(ctx, path, written, workers, nupReaders(path, workers), out.Progress(path, "Unpacking"))
	if err != nil {
		return "", "", err
	}
	// a folder that took the name meanwhile is never replaced (on Linux and macOS a rename would
	// replace an empty one without a word): the files stay in the hidden folder, said where
	if _, err := os.Lstat(name); err == nil {
		return "", "", fmt.Errorf("%s appeared while the NUP was unpacked: the files unpacked are in %s", filepath.Base(name), written)
	}
	if err := os.Rename(written, name); err != nil {
		// the hidden folder is the one this work made and filled
		os.RemoveAll(written)
		return "", "", err
	}
	notes := []string{fmt.Sprintf("%s, %s, checked by their SHA-256 as they were written", count(result.Files, "file", "files"), size(result.Size))}
	if o.RmSource {
		notes = append(notes, keptOriginal)
	}
	return name, joinNotes(notes...), nil
}
