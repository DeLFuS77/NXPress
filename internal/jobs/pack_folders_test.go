package jobs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// gameInFolders writes a game in game, its update and DLC in folders of their own, as a dump
// collection keeps them, and gives its three files.
func gameInFolders(t *testing.T, game string) []string {
	t.Helper()
	return []string{
		testkit.TitleNsp(t, filepath.Join(game, "base.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil),
		testkit.TitleNsp(t, filepath.Join(game, "Updates", "update.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil),
		testkit.TitleNsp(t, filepath.Join(game, "DLC", "dlc.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil),
	}
}

// A game given as a folder, with its update and DLC in folders of their own: once its pack is
// made with --rm-source, the folders left empty go too, the one given among them (#126).
func TestPackGamesRemovesTheFoldersLeftEmpty(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	files := gameInFolders(t, game)
	o := Options{PackGames: true, RmSource: true, Files: files, Origins: []string{game}}
	if failed := PackGames(context.Background(), o, testkit.Quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	left := testkit.List(root)
	if len(left) != 1 || filepath.Ext(left[0]) != ".nsp" {
		t.Fatalf("left: %v", left)
	}
}

// A folder with anything else in it stays, with what it holds, and so does the folder above it;
// the empty ones below go.
func TestPackGamesKeepsAFolderWithOtherFiles(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	files := gameInFolders(t, game)
	if err := os.WriteFile(filepath.Join(game, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "DLC", "desktop.ini"), []byte("[x]"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := Options{PackGames: true, RmSource: true, Files: files, Origins: []string{game}}
	if failed := PackGames(context.Background(), o, testkit.Quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if left := testkit.List(game); !reflect.DeepEqual(left, []string{"DLC", "notes.txt"}) {
		t.Fatalf("left in the game: %v", left)
	}
	if left := testkit.List(filepath.Join(game, "DLC")); !reflect.DeepEqual(left, []string{"desktop.ini"}) {
		t.Fatalf("left in DLC: %v", left)
	}
}

// Files given one by one leave their folders as they are, empty or not; without --rm-source
// nothing goes either.
func TestPackLeavesTheFoldersOfFilesGivenOneByOne(t *testing.T) {
	testkit.Isolate(t)
	for _, o := range []Options{{PackGames: true, RmSource: true}, {PackGames: true}} {
		root := t.TempDir()
		game := filepath.Join(root, "Game")
		files := gameInFolders(t, game)
		o.Files = files
		if !o.RmSource {
			o.Origins = []string{game}
		}
		if failed := PackGames(context.Background(), o, testkit.Quiet{}); failed != 0 {
			t.Fatalf("%d failed", failed)
		}
		for _, folder := range []string{game, filepath.Join(game, "Updates"), filepath.Join(game, "DLC")} {
			if !testkit.Exists(folder) {
				t.Fatalf("rm-source %v: %s went", o.RmSource, folder)
			}
		}
	}
}

// inside is true only for a path in the folder, never the folder itself, the one above it or a
// folder whose name only starts the same.
func TestInside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Game")
	for path, want := range map[string]bool{
		filepath.Join(root, "DLC"):         true,
		filepath.Join(root, "DLC", "x"):    true,
		root:                               false,
		filepath.Dir(root):                 false,
		root + " 2":                        false,
		filepath.Join(root+" 2", "DLC"):    false,
		filepath.Join(root, "..", "Other"): false,
	} {
		if got := inside(path, root); got != want {
			t.Errorf("inside(%s) = %v", path, got)
		}
	}
}
