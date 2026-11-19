package jobs

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/DeLFuS77/NXPress/internal/trash"

	"github.com/dtrunk90/switch-library-manager-web/par2"
)

// The originals a work is done with, once their new files are checked, go to the recycle bin of
// the system with --trash (in the window, unless Settings › General says to delete them for good),
// and are deleted for good without it, as nsz deletes them. A drive without a recycle bin keeps
// them; the window asks first whether to delete them for good there (TrashDelete).

// moveToTrash sends a file or a folder to the recycle bin; the tests give one of their own.
var moveToTrash = trash.Move

// checkTrash tells whether a file or a folder can go to the recycle bin; the tests give one of
// their own too.
var checkTrash = trash.Check

// The notes of what became of an original with --trash.
const (
	trashedOne   = "the original is in the recycle bin"
	trashedAll   = "the originals are in the recycle bin"
	trashedSome  = "%d originals are in the recycle bin"
	keptNoBin    = "the original is kept: "
	deletedNoBin = "the original is deleted for good: "
	dumpTrashed  = "the dump folder is in the recycle bin: the NUP holds all its files, checked"
	// a dump folder goes to the recycle bin whole, or not at all
	dumpKeptInPart   = "the dump folder is kept: some of its files are not in the NUP as they are, and it goes to the recycle bin only whole"
	dumpKeptNoBin    = "the dump folder is kept: "
	dumpDeletedNoBin = "the dump folder is deleted for good: "
	trashFailed      = "it could not go to the recycle bin: "
)

// fate is what became of an original: in the recycle bin, kept where it was because its drive has
// none (with why), or deleted for good, as asked or because its drive has none (with why).
type fate struct {
	trashed bool
	kept    bool
	reason  string
}

// note tells it of the original, "" when it was deleted for good as asked.
func (f fate) note() string {
	switch {
	case f.trashed:
		return trashedOne
	case f.kept:
		return keptNoBin + f.reason
	case f.reason != "":
		return deletedNoBin + f.reason
	}
	return ""
}

// discard takes away the items of an original (a file, a folder of parts, or its parts one by
// one): to the recycle bin with --trash, else with remove, for good. Where there is no recycle
// bin they stay, or go for good with TrashDelete. An error tells they could not be taken away,
// and the items not taken are where they were.
func discard(o Options, items []string, remove func(string) error) (fate, error) {
	if !o.Trash {
		for _, item := range items {
			if err := remove(item); err != nil {
				return fate{}, err
			}
		}
		return fate{}, nil
	}
	for i, item := range items {
		err := moveToTrash(item)
		if err == nil {
			continue
		}
		unavailable, ok := trash.Why(err)
		// the parts of one file are on one drive: only the first can find it has no recycle bin
		if !ok || i > 0 {
			return fate{}, err
		}
		if !o.TrashDelete {
			return fate{kept: true, reason: unavailable.Reason}, nil
		}
		for _, item := range items {
			if err := remove(item); err != nil {
				return fate{}, err
			}
		}
		return fate{reason: unavailable.Reason}, nil
	}
	return fate{trashed: true}, nil
}

// removeRecovery takes away the recovery data of an original that went: to the recycle bin with
// it, so it is there when the original is put back, or for good.
func removeRecovery(path string, gone fate) {
	sidecar := par2.Path(path)
	if gone.kept || !exists(sidecar) {
		return
	}
	if gone.trashed && moveToTrash(sidecar) == nil {
		return
	}
	os.Remove(sidecar)
}

// NoTrash is a drive the originals of a work are on that cannot go to its recycle bin, with how
// many of them and how big they are: it has none, or they do not fit in it (TooBig).
type NoTrash struct {
	Drive  string `json:"drive"`
	Files  int    `json:"files"`
	Size   int64  `json:"size"`
	TooBig bool   `json:"tooBig"`
}

// NoTrashOf tells, before a work starts, the originals it sends to the recycle bin that cannot
// go there, by drive: the window asks whether to delete them for good or keep them. A work that
// takes the originals away in place (Recompress, Trim and FAT32 without another folder) or keeps
// them (the NUP files Decompress and Split unpack) has none to tell.
func NoTrashOf(o Options) []NoTrash {
	if !o.RmSource || !o.Trash {
		return nil
	}
	action := o.Action()
	extensions, ok := Takes[action]
	if !ok {
		return nil
	}
	switch action {
	case "compress", "decompress", "convert", "tocard", "recompress", "trim", "split", "fat32", "pack", "nup":
	default:
		return nil
	}
	files, _ := Inputs(o.Files, extensions)
	output, _ := outputFolder(o)
	byDrive := map[string]*NoTrash{}
	for _, path := range files {
		switch {
		case isNup(path) && (action == "decompress" || action == "split"):
			continue
		case IsDumpFolder(path) && !o.RmDumps:
			continue
		case (action == "recompress" || action == "trim" || action == "fat32") && (output == "" || SameFile(output, filepath.Dir(path))):
			continue
		}
		items := []string{path}
		if _, parts := SplitContainer(path); parts && !isDir(path) {
			items = partNames(path)
		}
		unavailable, ok := trash.Why(checkTrash(items[0]))
		if !ok {
			continue
		}
		drive := byDrive[unavailable.Drive]
		if drive == nil {
			drive = &NoTrash{Drive: unavailable.Drive}
			byDrive[unavailable.Drive] = drive
		}
		drive.Files++
		drive.TooBig = drive.TooBig || unavailable.Reason == trash.TooBig
		for _, item := range items {
			drive.Size += sizeOfItem(item)
		}
	}
	drives := []NoTrash{}
	for _, drive := range byDrive {
		drives = append(drives, *drive)
	}
	sort.Slice(drives, func(i, j int) bool { return drives[i].Drive < drives[j].Drive })
	return drives
}

// sizeOfItem is the size of a file, or of the files of a folder.
func sizeOfItem(path string) int64 {
	if IsDumpFolder(path) {
		return DumpSize(path)
	}
	if size, _, err := PartsSize(path); err == nil && isDir(path) {
		return size
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
