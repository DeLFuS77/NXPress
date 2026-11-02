package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// packFolderGame writes a game in a folder of its own, as collections keep them: the game, and
// its update and DLC in folders inside. A second game takes the title IDs after the first.
func packFolderGame(t *testing.T, folder string, second bool) (string, string, string) {
	t.Helper()
	base, update, dlc := uint64(safetyBaseID), uint64(safetyUpdateID), uint64(safetyDlcID)
	if second {
		base, update, dlc = base+0x10000, update+0x10000, dlc+0x10000
	}
	return safetyTitleNsp(t, filepath.Join(folder, "game.nsp"), base, 0, switchfs.ContentMetaType_Application, 1, nil),
		safetyTitleNsp(t, filepath.Join(folder, "Updates", "update.nsp"), update, 65536, switchfs.ContentMetaType_Patch, 2, nil),
		safetyTitleNsp(t, filepath.Join(folder, "DLC", "dlc.nsp"), dlc, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
}

// packsIn are the packs a folder holds: the files of more than one title.
func packsIn(t *testing.T, folder string) []string {
	t.Helper()
	packs := []string{}
	for _, name := range safetyList(folder) {
		if titles, err := readTitles(filepath.Join(folder, name)); err == nil && len(titles) > 1 {
			packs = append(packs, name)
		}
	}
	return packs
}

// A folder of games, each in a folder of its own, combined by game: the SuperNSP of each game
// goes next to its folder, not inside it (#45). Given again, each game is combined already.
func TestPackGamesOfFoldersGoNextToThem(t *testing.T) {
	safetyIsolate(t)
	collection := filepath.Join(t.TempDir(), "test")
	packFolderGame(t, filepath.Join(collection, "Alpha"), false)
	packFolderGame(t, filepath.Join(collection, "Beta"), true)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{collection}}, quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	packs := packsIn(t, collection)
	if len(packs) != 2 || !strings.Contains(packs[0], "[0100000000010000]") || !strings.Contains(packs[1], "[0100000000020000]") {
		t.Fatalf("next to the folders: %v", safetyList(collection))
	}
	for _, game := range []string{"Alpha", "Beta"} {
		if inside := safetyList(filepath.Join(collection, game)); !reflect.DeepEqual(inside, []string{"DLC", "Updates", "game.nsp"}) {
			t.Errorf("%s holds %v", game, inside)
		}
	}
	out := &safetyResults{}
	if failed := packGames(context.Background(), options{packGames: true, files: []string{collection}}, out); failed != 0 {
		t.Fatalf("again: %d failed", failed)
	}
	for path, result := range out.results {
		if !isSkipped(result.err) || !strings.HasPrefix(result.err.Error(), "already combined in ") {
			t.Errorf("again, %s: %+v", filepath.Base(path), result)
		}
	}
	if plan := (&App{}).PackPlan([]string{collection}, nil, "", ""); len(plan) != 2 || plan[0].Packed == "" || plan[1].Packed == "" {
		t.Errorf("the plan %+v", plan)
	}
}

// In the window the list holds the files of the folders dropped or chosen, which come as its
// origins: the pack goes next to the folder there too, and the plan finds it there.
func TestPackGamesOfTheWindowGoNextToTheFolder(t *testing.T) {
	safetyIsolate(t)
	collection := filepath.Join(t.TempDir(), "test")
	base, update, dlc := packFolderGame(t, filepath.Join(collection, "Alpha"), false)
	files := []string{base, update, dlc}
	if failed := packGames(context.Background(), options{packGames: true, files: files, origins: []string{collection}}, quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if packs := packsIn(t, collection); len(packs) != 1 {
		t.Fatalf("next to the folder: %v", safetyList(collection))
	}
	if plan := (&App{}).PackPlan(files, []string{collection}, "", ""); len(plan) != 1 || plan[0].Packed == "" {
		t.Errorf("the plan %+v", plan)
	}
}

// Files given one by one keep their pack next to them, in the folder of the game; and so do
// the games of a folder that holds the files of several games together, with no folder each.
func TestPackOfFilesStaysNextToThem(t *testing.T) {
	safetyIsolate(t)
	root := t.TempDir()
	game := filepath.Join(root, "Alpha")
	base, update, dlc := packFolderGame(t, game, false)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{base, update, dlc}}, quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if packs := packsIn(t, game); len(packs) != 1 || len(packsIn(t, root)) != 0 {
		t.Fatalf("next to the files: %v, above: %v", safetyList(game), safetyList(root))
	}

	together := filepath.Join(root, "together")
	safetyGame(t, together)
	safetyTitleNsp(t, filepath.Join(together, "other.nsp"), safetyBaseID+0x10000, 0, switchfs.ContentMetaType_Application, 4, nil)
	safetyTitleNsp(t, filepath.Join(together, "other update.nsp"), safetyUpdateID+0x10000, 65536, switchfs.ContentMetaType_Patch, 5, nil)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{together}}, quiet{}); failed != 0 {
		t.Fatalf("together: %d failed", failed)
	}
	if packs := packsIn(t, together); len(packs) != 2 {
		t.Fatalf("together: %v, above: %v", safetyList(together), safetyList(root))
	}
}

// On the command line, which takes the files of the folders given in their place, the folders
// still count: one file with everything of a folder goes next to it, and so does the SuperNSP
// of each game of a collection.
func TestPackOfAFolderOnTheCommandLineGoesNextToIt(t *testing.T) {
	safetyIsolate(t)
	keys := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(keys, []byte("header_key = "+safetyHeaderKey+"\nkey_area_key_application_00 = "+safetyAreaKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runCommand(append([]string{"--keys", keys}, args...), &stdout, &stderr); code != 0 {
			t.Fatalf("%v: %d %s%s", args, code, stdout.String(), stderr.String())
		}
	}
	collection := filepath.Join(t.TempDir(), "test")
	game := filepath.Join(collection, "Alpha")
	packFolderGame(t, game, false)
	command("--pack-one", "--recursive", game)
	packs := packsIn(t, collection)
	if len(packs) != 1 || len(packsIn(t, game)) != 0 {
		t.Fatalf("next to the folder: %v; inside: %v", safetyList(collection), safetyList(game))
	}
	if got := packTitles(t, filepath.Join(collection, packs[0])); !reflect.DeepEqual(got, safetyWholeGame) {
		t.Fatalf("the pack holds %v", got)
	}

	other := filepath.Join(t.TempDir(), "test")
	packFolderGame(t, filepath.Join(other, "Alpha"), false)
	packFolderGame(t, filepath.Join(other, "Beta"), true)
	command("--pack-games", "--recursive", other)
	if packs := packsIn(t, other); len(packs) != 2 {
		t.Fatalf("by game: %v", safetyList(other))
	}
}

// packTitles are the titles a pack holds, as "<title ID> v<version>", in order.
func packTitles(t *testing.T, path string) []string {
	t.Helper()
	titles, err := readTitles(path)
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
