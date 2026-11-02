package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A pack chosen under the name of one of its files, with -w and --rm-source: nothing is
// written over that file, nor deleted. A pack made before of the others, given again, is
// written again.
func TestPackUnderTheNameOfOneOfItsFilesKeepsIt(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := testkit.Game(t, folder)
	if err := pack(context.Background(), Options{Pack: base, Overwrite: true, RmSource: true, Files: []string{base, update, dlc}}, testkit.Quiet{}); err == nil {
		t.Error("the pack took the name of the game")
	}
	if got := safetyTitles(t, base); !reflect.DeepEqual(got, testkit.WholeGame[:1]) || !testkit.Exists(update) || !testkit.Exists(dlc) {
		t.Fatalf("the game holds %v; left: %v", got, testkit.List(folder))
	}

	packed := filepath.Join(folder, "pack.nsp")
	if err := pack(context.Background(), Options{Pack: packed, Files: []string{base, update, dlc}}, testkit.Quiet{}); err != nil {
		t.Fatal(err)
	}
	if err := pack(context.Background(), Options{Pack: packed, Overwrite: true, RmSource: true, Files: []string{base, update, dlc, packed}}, testkit.Quiet{}); err != nil {
		t.Fatal(err)
	}
	if got := safetyTitles(t, packed); !reflect.DeepEqual(got, testkit.WholeGame) {
		t.Fatalf("the pack holds %v; left: %v", got, testkit.List(folder))
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"pack.nsp"}) {
		t.Fatalf("left: %v", left)
	}
}

// Packing a folder by game again, with the pack made before among its files (the files given
// one by one, it went next to them), -w and --rm-source: the game is still whole in one file,
// now next to the folder (#45).
func TestPackGamesAgainKeepsTheGame(t *testing.T) {
	testkit.Isolate(t)
	folder := filepath.Join(t.TempDir(), "games")
	base, update, dlc := testkit.Game(t, folder)
	if failed := PackGames(context.Background(), Options{PackGames: true, Files: []string{base, update, dlc}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	PackGames(context.Background(), Options{PackGames: true, Overwrite: true, RmSource: true, Files: []string{folder}}, testkit.Quiet{})
	for _, place := range []string{folder, filepath.Dir(folder)} {
		for _, name := range testkit.List(place) {
			if titles, err := ReadTitles(filepath.Join(place, name)); err == nil && len(titles) == 3 {
				return
			}
		}
	}
	t.Fatalf("no file holds the whole game anymore: %v, above: %v", testkit.List(folder), testkit.List(filepath.Dir(folder)))
}

// Two files a template gives the same name: the second is not renamed onto the first, with -w
// or with several files at once.
func TestRenameNeverGivesTwoFilesOneName(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	testkit.TitleNsp(t, filepath.Join(folder, "Game [one].nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	testkit.TitleNsp(t, filepath.Join(folder, "Game [two].nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
	out := &testkit.Results{}
	if failed := RunJobs(context.Background(), Options{Template: "{name}", Overwrite: true, Files: []string{folder}, Multi: 1, Threads: 1}, out); failed != 1 {
		t.Errorf("%d failed, want 1", failed)
	}
	if left := testkit.List(folder); len(left) != 2 {
		t.Fatalf("one game took the place of the other: %v", left)
	}

	for round := 0; round < 5; round++ {
		folder := t.TempDir()
		for i := 0; i < 8; i++ {
			testkit.TitleNsp(t, filepath.Join(folder, fmt.Sprintf("Game [%d].nsp", i)), testkit.BaseID+uint64(i)<<16, 0, switchfs.ContentMetaType_Application, i, nil)
		}
		RunJobs(context.Background(), Options{Template: "{name}", Files: []string{folder}, Multi: 8, Threads: 1}, testkit.Quiet{})
		if left := testkit.List(folder); len(left) != 8 {
			t.Fatalf("round %d: %d files left of 8: %v", round, len(left), left)
		}
	}
}

// A SuperNSP named as its game, split with -w: its first part would take its name, so nothing
// is written and it stays whole.
func TestSplitNeverTakesTheNameOfTheFileItSplits(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base := testkit.TitleNsp(t, filepath.Join(folder, "b.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	dlc := testkit.TitleNsp(t, filepath.Join(folder, "d.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	super := filepath.Join(folder, "Game [0100000000010000][v0].nsp")
	if _, _, _, err := switchfs.MergeNewestAndCheck(context.Background(), []string{base, dlc}, super, true, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(base)
	os.Remove(dlc)
	if _, _, err := split(context.Background(), super, Options{Overwrite: true, RmSource: true}, testkit.Quiet{}); err == nil {
		t.Error("split into its own name")
	}
	if got := safetyTitles(t, super); len(got) != 2 {
		t.Fatalf("the SuperNSP holds %v; left: %v", got, testkit.List(folder))
	}
	if left := testkit.List(folder); len(left) != 1 {
		t.Fatalf("parts were written: %v", left)
	}
}

// The output folder given as the folder of the file, written another way ("." from it): the
// file is worked on in place, and not deleted as its own original.
func TestOutputFolderWrittenAnotherWayIsInPlace(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	nsp := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	nsz := filepath.Join(folder, "game.nsz")
	if _, err := switchfs.CompressGame(context.Background(), nsp, nsz, switchfs.CompressOptions{Level: "3", Workers: 1}); err != nil {
		t.Fatal(err)
	}
	os.Remove(nsp)
	t.Chdir(folder)
	o := Options{Output: ".", Overwrite: true, RmSource: true, Level: 3, Block: true}
	if _, _, err := recompress(context.Background(), nsz, o, 1, testkit.Quiet{}); err != nil {
		t.Fatal(err)
	}
	if !testkit.Exists(nsz) {
		t.Fatalf("the recompressed file was deleted as its own original: %v", testkit.List(folder))
	}

	defer func(size int64) { fat32Part = size }(fat32Part)
	fat32Part = 4
	path := filepath.Join(folder, "big.nsp")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	name, _, err := splitParts(context.Background(), path, 10, o, testkit.Quiet{})
	if err != nil || name != path {
		t.Fatalf("split in place: %q %v", name, err)
	}
	if _, parts, err := PartsSize(path); parts != 3 || err != nil {
		t.Fatalf("the parts: %d %v; left: %v", parts, err, testkit.List(folder))
	}
}

// Two files of the same name in two folders, into one output folder with -w and --rm-source:
// the second does not write over the first, and its original stays.
func TestSameNameFromTwoFoldersIntoOneOutput(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	a := testkit.TitleNsp(t, filepath.Join(root, "A", "Game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	b := testkit.TitleNsp(t, filepath.Join(root, "B", "Game.nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
	output := filepath.Join(root, "out")
	out := &testkit.Results{}
	failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Output: output, Overwrite: true, RmSource: true, Files: []string{a, b}, Multi: 1, Threads: 1}, out)
	if failed != 1 || !reflect.DeepEqual(out.Failed(IsSkipped), []string{b}) {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	if testkit.Exists(a) || !testkit.Exists(b) {
		t.Fatalf("A is there %v, B is there %v", testkit.Exists(a), testkit.Exists(b))
	}
	if got := safetyTitles(t, filepath.Join(output, "Game.nsz")); !reflect.DeepEqual(got, []string{"0100000000010000 v0"}) {
		t.Fatalf("the new file holds %v", got)
	}
}

// The same with several files at once: they never share the hidden file they are written
// under, and one of them fails before writing anything.
func TestSameNameAtOnceIntoOneOutput(t *testing.T) {
	testkit.Isolate(t)
	for round := 0; round < 3; round++ {
		root := t.TempDir()
		a := testkit.TitleNsp(t, filepath.Join(root, "A", "Game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
		b := testkit.TitleNsp(t, filepath.Join(root, "B", "Game.nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
		output := filepath.Join(root, "out")
		out := &testkit.Results{}
		if failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Output: output, Overwrite: round%2 == 0, Files: []string{a, b}, Multi: 2, Threads: 1}, out); failed != 1 {
			t.Fatalf("round %d: %d failed: %v", round, failed, out.Results)
		}
		if left := testkit.List(output); len(left) != 1 || left[0] != "Game.nsz" {
			t.Fatalf("round %d: the output folder holds %v", round, left)
		}
		if _, err := switchfs.VerifyFile(context.Background(), filepath.Join(output, "Game.nsz"), nil); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}

// safetyTitles are the titles a game file holds, as "<title ID> v<version>", in order.
func safetyTitles(t *testing.T, path string) []string {
	t.Helper()
	titles, err := ReadTitles(path)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
	found := []string{}
	for _, title := range titles {
		found = append(found, fmt.Sprintf("%s v%d", strings.ToUpper(title.TitleId), title.Version))
	}
	sort.Strings(found)
	return found
}
