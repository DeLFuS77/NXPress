package jobs

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// safetyTicket is a ticket of a test, filled with one byte.
func safetyTicket(fill byte) []byte {
	data := make([]byte, 0x2C0)
	for i := range data {
		data[i] = fill
	}
	return data
}

// A SuperNSP with two versions of an update, packed with a DLC and --rm-source: the pack keeps
// the ticket of the update it keeps, so it holds everything and the originals go.
func TestPackOfTwoVersionsKeepsTheTicketOfTheNewest(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	ticket := "0100000000010800000000000000000a.tik"
	base := testkit.TitleNsp(t, filepath.Join(folder, "b.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	v1 := testkit.TitleNsp(t, filepath.Join(folder, "u1.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, map[string][]byte{ticket: safetyTicket(1)})
	v2 := testkit.TitleNsp(t, filepath.Join(folder, "u2.nsp"), testkit.UpdateID, 131072, switchfs.ContentMetaType_Patch, 4, map[string][]byte{ticket: safetyTicket(2)})
	super := filepath.Join(folder, "super.nsp")
	if _, _, err := switchfs.MergeAndCheck(context.Background(), []string{base, v2, v1}, super, true, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(base)
	os.Remove(v1)
	os.Remove(v2)
	dlc := testkit.TitleNsp(t, filepath.Join(folder, "d.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	packed := filepath.Join(folder, "pack.nsp")
	if err := pack(context.Background(), Options{Pack: packed, RmSource: true, Files: []string{super, dlc}}, testkit.Quiet{}); err != nil {
		t.Fatal(err)
	}
	contents, err := switchfs.ReadPfs0File(packed)
	if err != nil {
		t.Fatal(err)
	}
	hasTicket := false
	for _, file := range contents.Files {
		hasTicket = hasTicket || file.Name == ticket
	}
	if !hasTicket {
		t.Fatalf("the pack lost the ticket of the update it keeps")
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"pack.nsp"}) {
		t.Fatalf("left: %v", left)
	}
}

// A pack without the ticket an original had is told as lacking it, whatever made it so: its
// originals then stay.
func TestPackLacksTellsAMissingTicket(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	ticket := "0100000000010800000000000000000a.tik"
	original := testkit.TitleNsp(t, filepath.Join(folder, "u.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, map[string][]byte{ticket: safetyTicket(1)})
	without := testkit.TitleNsp(t, filepath.Join(folder, "pack.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil)
	lacks, err := packLacks(without, []string{original})
	if err != nil || !reflect.DeepEqual(lacks, []string{"the ticket of 0100000000010800"}) {
		t.Fatalf("the pack lacks %v, %v", lacks, err)
	}
}

// A pack that holds everything of its titles deletes its originals with --rm-source.
func TestPackWithEverythingDeletesItsOriginals(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base := testkit.TitleNsp(t, filepath.Join(folder, "b.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	update := testkit.TitleNsp(t, filepath.Join(folder, "u.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, map[string][]byte{"0100000000010800000000000000000a.tik": safetyTicket(1)})
	packed := filepath.Join(folder, "pack.nsp")
	if err := pack(context.Background(), Options{Pack: packed, RmSource: true, Files: []string{base, update}}, testkit.Quiet{}); err != nil {
		t.Fatal(err)
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"pack.nsp"}) {
		t.Fatalf("left: %v", left)
	}
	if lacks, err := packLacks(packed, nil); err != nil || len(lacks) != 0 {
		t.Fatalf("the pack lacks %v, %v", lacks, err)
	}
}

// safetyWatcher looks at the folders when a pack starts to be written, for its hidden file.
type safetyWatcher struct {
	testkit.Quiet
	folders []string
	seen    map[string][]string
}

func (w *safetyWatcher) Progress(path string, step string) func(int64, int64) {
	return func(int64, int64) {
		if w.seen != nil {
			return
		}
		w.seen = map[string][]string{}
		for _, folder := range w.folders {
			w.seen[folder] = testkit.List(folder)
		}
	}
}

// The file chosen for a pack is where it goes, whatever the output folder: written there under
// a hidden name, never among its files, and the name is looked at before anything is written.
func TestPackGoesWhereItIsChosen(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	games := filepath.Join(root, "games")
	base, update, dlc := testkit.Game(t, games)
	chosen := filepath.Join(root, "sd", "Game.nsp")
	out := &safetyWatcher{folders: []string{games, filepath.Dir(chosen)}}
	if err := pack(context.Background(), Options{Pack: chosen, Output: filepath.Join(root, "other"), Files: []string{base, update, dlc}}, out); err != nil {
		t.Fatal(err)
	}
	if len(out.seen[games]) != 3 || len(out.seen[filepath.Dir(chosen)]) != 1 || !strings.HasPrefix(out.seen[filepath.Dir(chosen)][0], ".nxpress-pack-") {
		t.Fatalf("while written: %v", out.seen)
	}
	if got := safetyTitles(t, chosen); !reflect.DeepEqual(got, testkit.WholeGame) || testkit.Exists(filepath.Join(root, "other")) {
		t.Fatalf("the pack holds %v; other folder there %v", got, testkit.Exists(filepath.Join(root, "other")))
	}

	// the name taken: nothing is written
	if err := pack(context.Background(), Options{Pack: chosen, Files: []string{base, update, dlc}}, &safetyWatcher{}); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Fatalf("onto a pack there: %v", err)
	}
	// the file chosen given among the files is not one of them
	err := pack(context.Background(), Options{Pack: base, Overwrite: true, RmSource: true, Files: []string{base, dlc}}, testkit.Quiet{})
	if err == nil || !testkit.Exists(base) || !testkit.Exists(dlc) {
		t.Fatalf("a pack of itself: %v; left: %v", err, testkit.List(games))
	}
	if got := safetyTitles(t, base); len(got) != 1 {
		t.Fatalf("the game holds %v", got)
	}
}

// Packing a folder by game again, with the pack made before among its files: without -w the
// game is told combined already, and nothing is written.
func TestPackGamesAgainIsCombinedAlready(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	testkit.Game(t, folder)
	if failed := PackGames(context.Background(), Options{PackGames: true, Files: []string{folder}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	before := testkit.List(folder)
	out := &testkit.Results{}
	if failed := PackGames(context.Background(), Options{PackGames: true, RmSource: true, Files: []string{folder}}, out); failed != 0 {
		t.Fatalf("second run: %d failed: %v", failed, out.Results)
	}
	for path, result := range out.Results {
		if !IsSkipped(result.Err) || !strings.HasPrefix(result.Err.Error(), "already combined in ") {
			t.Errorf("%s: %+v", filepath.Base(path), result)
		}
	}
	if after := testkit.List(folder); !reflect.DeepEqual(after, before) {
		t.Fatalf("before %v, after %v", before, after)
	}
}

// One file with everything and no file chosen: the pack goes next to the game itself, wherever
// its other files are, named by its template, or into the output folder; a file that cannot be
// read leaves every file as it is.
func TestPackOneGoesNextToTheFiles(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	games := filepath.Join(root, "games")
	base, update, dlc := testkit.Game(t, games)
	moved := filepath.Join(root, "dlc", "dlc.nsp")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dlc, moved); err != nil {
		t.Fatal(err)
	}
	out := &testkit.Results{}
	if failed := RunJobs(context.Background(), Options{PackOne: true, RmSource: true, Files: []string{moved, update, base}}, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	left := testkit.List(games)
	if len(left) != 1 || testkit.Exists(moved) {
		t.Fatalf("games %v, the DLC there %v", left, testkit.Exists(moved))
	}
	if got := safetyTitles(t, filepath.Join(games, left[0])); !reflect.DeepEqual(got, testkit.WholeGame) {
		t.Fatalf("the pack holds %v", got)
	}

	// into the output folder, named by the template, the files left where they are
	again := filepath.Join(root, "again")
	base, update, _ = testkit.Game(t, again)
	output := filepath.Join(root, "output")
	if failed := RunJobs(context.Background(), Options{PackOne: true, PackName: "{name} pack", Output: output, Files: []string{base, update}}, &testkit.Results{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if left := testkit.List(output); len(left) != 1 || !strings.HasSuffix(left[0], "pack.nsp") || len(testkit.List(again)) != 3 {
		t.Fatalf("output %v, the files %v", left, testkit.List(again))
	}

	// a file that cannot be read: no pack lacking it
	broken := filepath.Join(root, "broken")
	testkit.Game(t, broken)
	if err := os.WriteFile(filepath.Join(broken, "zz.nsp"), []byte("not a game"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := testkit.List(broken)
	if failed := RunJobs(context.Background(), Options{PackOne: true, RmSource: true, Files: []string{broken}}, &testkit.Results{}); failed != 4 {
		t.Errorf("%d failed, want 4", failed)
	}
	if after := testkit.List(broken); !reflect.DeepEqual(after, before) {
		t.Fatalf("before %v, after %v", before, after)
	}
}

// The files of a game card are the ones of its secure partition.
func TestPackageFilesOfAGameCard(t *testing.T) {
	hfs0 := func(names []string, sizes []int) []byte {
		table := []byte{}
		at := []int{}
		for _, name := range names {
			at = append(at, len(table))
			table = append(table, append([]byte(name), 0)...)
		}
		head := make([]byte, 0x10+0x40*len(names))
		copy(head, "HFS0")
		binary.LittleEndian.PutUint32(head[4:], uint32(len(names)))
		binary.LittleEndian.PutUint32(head[8:], uint32(len(table)))
		offset := 0
		for i := range names {
			entry := head[0x10+0x40*i:]
			binary.LittleEndian.PutUint64(entry[0:], uint64(offset))
			binary.LittleEndian.PutUint64(entry[8:], uint64(sizes[i]))
			binary.LittleEndian.PutUint32(entry[16:], uint32(at[i]))
			offset += sizes[i]
		}
		return append(head, table...)
	}
	secure := hfs0([]string{"0123456789abcdef0123456789abcdef.nca", "0100000000010800000000000000000a.tik"}, []int{0x10, 0x10})
	secure = append(secure, make([]byte, 0x20)...)
	root := hfs0([]string{"update", "secure"}, []int{0, len(secure)})
	card := make([]byte, 0x200)
	copy(card[0x100:], "HEAD")
	binary.LittleEndian.PutUint64(card[0x130:], 0x200)
	card = append(append(card, root...), secure...)
	path := filepath.Join(t.TempDir(), "card.xci")
	os.WriteFile(path, card, 0o644)
	names, err := packageFiles(path)
	if err != nil || !reflect.DeepEqual(names, []string{"0123456789abcdef0123456789abcdef.nca", "0100000000010800000000000000000a.tik"}) {
		t.Fatalf("%v %v", names, err)
	}
}
