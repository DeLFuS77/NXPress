package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the titles of the synthetic game, its update and its DLC together
var safetyWholeGame = []string{"0100000000010000 v0", "0100000000010800 v65536", "0100000000011001 v0"}

// A pack written under the name of one of its files, with -w and --rm-source: that file is
// the pack now, and it is not deleted as an original.
func TestPackUnderTheNameOfOneOfItsFilesKeepsIt(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	base, update, dlc := safetyGame(t, folder)
	pack(context.Background(), options{pack: base, overwrite: true, rmSource: true, files: []string{base, update, dlc}}, quiet{})
	if !safetyExists(base) {
		t.Fatalf("the pack was deleted as one of its originals; left: %v", safetyList(folder))
	}
	if got := safetyTitles(t, base); !reflect.DeepEqual(got, safetyWholeGame) {
		t.Fatalf("the pack holds %v", got)
	}
}

// Packing a folder by game again, with the pack made before among its files, -w and
// --rm-source: the game is still whole in one file.
func TestPackGamesAgainKeepsTheGame(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	safetyGame(t, folder)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{folder}}, quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	packGames(context.Background(), options{packGames: true, overwrite: true, rmSource: true, files: []string{folder}}, quiet{})
	for _, name := range safetyList(folder) {
		if titles, err := readTitles(filepath.Join(folder, name)); err == nil && len(titles) == 3 {
			return
		}
	}
	t.Fatalf("no file holds the whole game anymore: %v", safetyList(folder))
}

// Two files a template gives the same name: the second is not renamed onto the first, with -w
// or with several files at once.
func TestRenameNeverGivesTwoFilesOneName(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	safetyTitleNsp(t, filepath.Join(folder, "Game [one].nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	safetyTitleNsp(t, filepath.Join(folder, "Game [two].nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
	out := &safetyResults{}
	if failed := runJobs(context.Background(), options{template: "{name}", overwrite: true, files: []string{folder}, multi: 1, threads: 1}, out); failed != 1 {
		t.Errorf("%d failed, want 1", failed)
	}
	if left := safetyList(folder); len(left) != 2 {
		t.Fatalf("one game took the place of the other: %v", left)
	}

	for round := 0; round < 5; round++ {
		folder := t.TempDir()
		for i := 0; i < 8; i++ {
			safetyTitleNsp(t, filepath.Join(folder, fmt.Sprintf("Game [%d].nsp", i)), safetyBaseID+uint64(i)<<16, 0, switchfs.ContentMetaType_Application, i, nil)
		}
		runJobs(context.Background(), options{template: "{name}", files: []string{folder}, multi: 8, threads: 1}, quiet{})
		if left := safetyList(folder); len(left) != 8 {
			t.Fatalf("round %d: %d files left of 8: %v", round, len(left), left)
		}
	}
}

// A SuperNSP named as its game, split with -w: its first part would take its name, so nothing
// is written and it stays whole.
func TestSplitNeverTakesTheNameOfTheFileItSplits(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	base := safetyTitleNsp(t, filepath.Join(folder, "b.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	dlc := safetyTitleNsp(t, filepath.Join(folder, "d.nsp"), safetyDlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	super := filepath.Join(folder, "Game [0100000000010000][v0].nsp")
	if _, _, _, err := switchfs.MergeNewestAndCheck(context.Background(), []string{base, dlc}, super, true, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(base)
	os.Remove(dlc)
	if _, _, err := split(context.Background(), super, options{overwrite: true, rmSource: true}, quiet{}); err == nil {
		t.Error("split into its own name")
	}
	if got := safetyTitles(t, super); len(got) != 2 {
		t.Fatalf("the SuperNSP holds %v; left: %v", got, safetyList(folder))
	}
	if left := safetyList(folder); len(left) != 1 {
		t.Fatalf("parts were written: %v", left)
	}
}

// The output folder given as the folder of the file, written another way ("." from it): the
// file is worked on in place, and not deleted as its own original.
func TestOutputFolderWrittenAnotherWayIsInPlace(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	nsp := safetyTitleNsp(t, filepath.Join(folder, "game.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	nsz := filepath.Join(folder, "game.nsz")
	if _, err := switchfs.CompressGame(context.Background(), nsp, nsz, switchfs.CompressOptions{Level: "3", Workers: 1}); err != nil {
		t.Fatal(err)
	}
	os.Remove(nsp)
	t.Chdir(folder)
	o := options{output: ".", overwrite: true, rmSource: true, level: 3, block: true}
	if _, _, err := recompress(context.Background(), nsz, o, 1, quiet{}); err != nil {
		t.Fatal(err)
	}
	if !safetyExists(nsz) {
		t.Fatalf("the recompressed file was deleted as its own original: %v", safetyList(folder))
	}

	defer func(size int64) { fat32Part = size }(fat32Part)
	fat32Part = 4
	path := filepath.Join(folder, "big.nsp")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	name, _, err := splitParts(context.Background(), path, 10, o, quiet{})
	if err != nil || name != path {
		t.Fatalf("split in place: %q %v", name, err)
	}
	if _, parts, err := partsSize(path); parts != 3 || err != nil {
		t.Fatalf("the parts: %d %v; left: %v", parts, err, safetyList(folder))
	}
}

// Two files of the same name in two folders, into one output folder with -w and --rm-source:
// the second does not write over the first, and its original stays.
func TestSameNameFromTwoFoldersIntoOneOutput(t *testing.T) {
	safetyIsolate(t)
	root := t.TempDir()
	a := safetyTitleNsp(t, filepath.Join(root, "A", "Game.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	b := safetyTitleNsp(t, filepath.Join(root, "B", "Game.nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
	output := filepath.Join(root, "out")
	out := &safetyResults{}
	failed := runJobs(context.Background(), options{compress: true, level: 3, output: output, overwrite: true, rmSource: true, files: []string{a, b}, multi: 1, threads: 1}, out)
	if failed != 1 || !reflect.DeepEqual(out.failed(), []string{b}) {
		t.Fatalf("%d failed: %v", failed, out.results)
	}
	if safetyExists(a) || !safetyExists(b) {
		t.Fatalf("A is there %v, B is there %v", safetyExists(a), safetyExists(b))
	}
	if got := safetyTitles(t, filepath.Join(output, "Game.nsz")); !reflect.DeepEqual(got, []string{"0100000000010000 v0"}) {
		t.Fatalf("the new file holds %v", got)
	}
}

// The same with several files at once: they never share the hidden file they are written
// under, and one of them fails before writing anything.
func TestSameNameAtOnceIntoOneOutput(t *testing.T) {
	safetyIsolate(t)
	for round := 0; round < 3; round++ {
		root := t.TempDir()
		a := safetyTitleNsp(t, filepath.Join(root, "A", "Game.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
		b := safetyTitleNsp(t, filepath.Join(root, "B", "Game.nsp"), 0x0100000000020000, 0, switchfs.ContentMetaType_Application, 2, nil)
		output := filepath.Join(root, "out")
		out := &safetyResults{}
		if failed := runJobs(context.Background(), options{compress: true, level: 3, output: output, overwrite: round%2 == 0, files: []string{a, b}, multi: 2, threads: 1}, out); failed != 1 {
			t.Fatalf("round %d: %d failed: %v", round, failed, out.results)
		}
		if left := safetyList(output); len(left) != 1 || left[0] != "Game.nsz" {
			t.Fatalf("round %d: the output folder holds %v", round, left)
		}
		if _, err := switchfs.VerifyFile(context.Background(), filepath.Join(output, "Game.nsz"), nil); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}
