package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/DeLFuS77/NXPress/internal/trash"

	"github.com/dtrunk90/switch-library-manager-web/par2"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// fakeBin stands in for the recycle bin of the system: what goes there is moved into a folder
// of the test, and the paths in noBin (and in the folders of noBin) are on a drive that has
// none. It gives the folder.
func fakeBin(t *testing.T, noBin ...string) string {
	t.Helper()
	bin := t.TempDir()
	unavailable := func(path string) error {
		for _, none := range noBin {
			if path == none || strings.HasPrefix(path, none+string(filepath.Separator)) {
				return &trash.Unavailable{Reason: trash.NoBin, Drive: none}
			}
		}
		return nil
	}
	move, check := moveToTrash, checkTrash
	moveToTrash = func(path string) error {
		if err := unavailable(path); err != nil {
			return err
		}
		return os.Rename(path, filepath.Join(bin, filepath.Base(path)))
	}
	checkTrash = unavailable
	t.Cleanup(func() { moveToTrash, checkTrash = move, check })
	return bin
}

// allNotes keeps the notes of every file done.
type allNotes struct {
	testkit.Quiet
	mutex sync.Mutex
	notes []string
}

func (n *allNotes) Done(path string, output string, note string, err error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.notes = append(n.notes, note)
}

func writeFile(t *testing.T, path string, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// With --trash the original goes to the recycle bin, its recovery data with it, and the note
// says so; without it, it is deleted for good as nsz deletes it, and the note says nothing more.
func TestRemoveSourceToTheRecycleBin(t *testing.T) {
	bin := fakeBin(t)
	folder := t.TempDir()
	original := writeFile(t, filepath.Join(folder, "Game [0100000000010000][v0].nsp"), "game")
	writeFile(t, par2.Path(original), "recovery")
	_, note, err := removeSource(Options{RmSource: true, Trash: true}, original, switchfs.FileCheck{}, filepath.Join(folder, "Game.nsz"), "1.0 GB -> 500.0 MB")
	if err != nil {
		t.Fatal(err)
	}
	if note != "1.0 GB -> 500.0 MB; "+trashedOne {
		t.Errorf("note %q", note)
	}
	if left := testkit.List(folder); len(left) != 0 {
		t.Errorf("left: %v", left)
	}
	if in := testkit.List(bin); !reflect.DeepEqual(in, []string{filepath.Base(original), filepath.Base(par2.Path(original))}) {
		t.Errorf("in the recycle bin: %v", in)
	}

	writeFile(t, original, "game")
	_, note, err = removeSource(Options{RmSource: true}, original, switchfs.FileCheck{}, "Game.nsz", "converted")
	if err != nil || note != "converted" {
		t.Fatalf("%q %v", note, err)
	}
	if testkit.Exists(original) || len(testkit.List(bin)) != 2 {
		t.Error("without --trash the original did not go for good")
	}
}

// Where there is no recycle bin the original stays, with its recovery data, and the note says
// why; with TrashDelete, as the window asks for, it goes for good, and the note says why too.
func TestRemoveSourceWithoutARecycleBin(t *testing.T) {
	share := t.TempDir()
	bin := fakeBin(t, share)
	original := writeFile(t, filepath.Join(share, "Game.xci"), "game")
	writeFile(t, par2.Path(original), "recovery")
	_, note, err := removeSource(Options{RmSource: true, Trash: true}, original, switchfs.FileCheck{}, "Game.xcz", "")
	if err != nil {
		t.Fatal(err)
	}
	if note != keptNoBin+trash.NoBin || !testkit.Exists(original) || !testkit.Exists(par2.Path(original)) {
		t.Errorf("kept: %q, %v", note, testkit.List(share))
	}
	_, note, err = removeSource(Options{RmSource: true, Trash: true, TrashDelete: true}, original, switchfs.FileCheck{}, "Game.xcz", "")
	if err != nil {
		t.Fatal(err)
	}
	if note != deletedNoBin+trash.NoBin || testkit.Exists(original) || testkit.Exists(par2.Path(original)) {
		t.Errorf("deleted: %q, %v", note, testkit.List(share))
	}
	if in := testkit.List(bin); len(in) != 0 {
		t.Errorf("in the recycle bin: %v", in)
	}
}

// A recycle bin that refuses the original for another reason fails the file: the original is
// where it was, and the error says it could not go to the recycle bin.
func TestRemoveSourceWhenTheRecycleBinFails(t *testing.T) {
	fakeBin(t)
	moveToTrash = func(string) error { return errors.New("access is denied") }
	original := writeFile(t, filepath.Join(t.TempDir(), "Game.nsp"), "game")
	_, _, err := removeSource(Options{RmSource: true, Trash: true}, original, switchfs.FileCheck{}, "Game.nsz", "")
	if err == nil || !strings.Contains(err.Error(), "could not go to the recycle bin: access is denied") || !testkit.Exists(original) {
		t.Fatalf("%v", err)
	}
}

// Combine with --trash: each original goes to the recycle bin, each row says so, and the folders
// left empty go, which hold nothing to put back.
func TestPackGamesToTheRecycleBin(t *testing.T) {
	testkit.Isolate(t)
	bin := fakeBin(t)
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	files := gameInFolders(t, game)
	out := &allNotes{}
	o := Options{PackGames: true, RmSource: true, Trash: true, Files: files, Origins: []string{game}}
	if failed := PackGames(context.Background(), o, out); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if left := testkit.List(root); len(left) != 1 || filepath.Ext(left[0]) != ".nsp" {
		t.Fatalf("left: %v", left)
	}
	if in := testkit.List(bin); len(in) != 3 {
		t.Errorf("in the recycle bin: %v", in)
	}
	told := 0
	for _, note := range out.notes {
		if strings.HasSuffix(note, trashedOne) {
			told++
		}
	}
	if told != 3 {
		t.Errorf("the rows tell: %v", out.notes)
	}
}

// The parts of a file joined into another folder go to the recycle bin with --trash: a folder
// of parts as one item; joined in place, they give way to the whole file as before.
func TestJoinedPartsToTheRecycleBin(t *testing.T) {
	bin := fakeBin(t)
	folder := t.TempDir()
	parts := filepath.Join(folder, "Game.nsp")
	writeFile(t, filepath.Join(parts, "00"), "first half ")
	writeFile(t, filepath.Join(parts, "01"), "second half")
	output := t.TempDir()
	o := Options{Fat32: true, RmSource: true, Trash: true, Output: output}
	name, note, err := joinParts(t.Context(), parts, parts, o, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(name); err != nil || string(data) != "first half second half" {
		t.Fatalf("%q %v", data, err)
	}
	if !strings.HasSuffix(note, trashedOne) || testkit.Exists(parts) || !reflect.DeepEqual(testkit.List(bin), []string{"Game.nsp"}) {
		t.Errorf("note %q, in the recycle bin %v", note, testkit.List(bin))
	}
}

// A dump folder goes to the recycle bin whole once its NUP is checked, when the NUP holds every
// file of it; with a file the NUP does not hold it stays whole, and the note says why.
func TestPackNupDumpToTheRecycleBin(t *testing.T) {
	bin := fakeBin(t)
	o := Options{RmSource: true, RmDumps: true, Trash: true}
	dir := writeDump(t, "Quiet Meadow")
	_, note, err := packNup(t.Context(), dir, o, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(note, dumpTrashed) || testkit.Exists(dir) {
		t.Errorf("note %q", note)
	}
	if _, err := os.Stat(filepath.Join(bin, "Quiet Meadow", "base", "romfs", "Data", "game.bin")); err != nil {
		t.Errorf("the dump folder is not whole in the recycle bin: %v", err)
	}

	dir = writeDump(t, "Amber Coast")
	writeFile(t, filepath.Join(dir, "readme.txt"), "notes of the dump")
	_, note, err = packNup(t.Context(), dir, o, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(note, dumpKeptInPart) || !testkit.Exists(filepath.Join(dir, "base", "exefs", "main")) {
		t.Errorf("note %q", note)
	}
}

// A dump folder on a drive without a recycle bin stays, or goes for good with TrashDelete.
func TestPackNupDumpWithoutARecycleBin(t *testing.T) {
	dir := writeDump(t, "Silver Pines")
	fakeBin(t, dir)
	_, note, err := packNup(t.Context(), dir, Options{RmSource: true, RmDumps: true, Trash: true}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(note, dumpKeptNoBin+trash.NoBin) || !testkit.Exists(dir) {
		t.Errorf("kept: %q", note)
	}
	dir = writeDump(t, "Silver Pines")
	fakeBin(t, dir)
	_, note, err = packNup(t.Context(), dir, Options{RmSource: true, RmDumps: true, Trash: true, TrashDelete: true, Overwrite: true}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(note, dumpDeletedNoBin+trash.NoBin) || testkit.Exists(dir) {
		t.Errorf("deleted: %q", note)
	}
}

// Before a work, the originals that cannot go to the recycle bin are told by drive, with how
// many and how big; the ones a work keeps or replaces in place are not.
func TestNoTrashOf(t *testing.T) {
	share := t.TempDir()
	fakeBin(t, share)
	local := t.TempDir()
	files := []string{
		writeFile(t, filepath.Join(share, "a.nsp"), "12345"),
		writeFile(t, filepath.Join(share, "b.nsp"), "123"),
		writeFile(t, filepath.Join(local, "c.nsp"), "1"),
	}
	got := NoTrashOf(Options{Compress: true, RmSource: true, Trash: true, Files: files})
	if want := []NoTrash{{Drive: share, Files: 2, Size: 8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("%+v", got)
	}
	for _, o := range []Options{
		{Compress: true, RmSource: true, Files: files},
		{Compress: true, Trash: true, Files: files},
		// in place: the file is replaced, not taken away
		{Trim: true, RmSource: true, Trash: true, Files: files},
		{Verify: true, RmSource: true, Trash: true, Files: files},
	} {
		if got := NoTrashOf(o); len(got) != 0 {
			t.Errorf("%+v: %+v", o, got)
		}
	}
	if got := NoTrashOf(Options{Trim: true, RmSource: true, Trash: true, Files: files, Output: local}); len(got) != 1 || got[0].Files != 2 {
		t.Errorf("trimmed into another folder: %+v", got)
	}
}

// --trash on the command line, told when it does nothing.
func TestTrashOption(t *testing.T) {
	o, err := ParseOptions([]string{"-C", "--rm-source", "--trash", "game.nsp"})
	if err != nil || !o.Trash || !o.RmSource || o.TrashDelete {
		t.Fatalf("%+v %v", o, err)
	}
	o, _ = ParseOptions([]string{"-C", "--trash", "game.nsp"})
	if err := o.Check(); err != nil || !reflect.DeepEqual(o.Notes, []string{"--trash has no effect without --rm-source"}) {
		t.Errorf("%v %v", o.Notes, err)
	}
	if !strings.Contains(Usage, "--trash             with --rm-source, send the originals to the recycle bin instead of") {
		t.Error("--help does not tell --trash")
	}
}
