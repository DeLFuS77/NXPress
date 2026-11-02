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

	"github.com/DeLFuS77/NXPress/internal/cli"
	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The plan of Combine shows a game as combined already when its pack, made before, comes among
// the files given under the name it would get again, as the work then tells it.
func TestPackPlanTellsAPackAmongTheFiles(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	testkit.Game(t, folder)
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: []string{folder}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	plan := (&App{}).PackPlan([]string{folder}, nil, "", "")
	if len(plan) != 1 || plan[0].Packed == "" {
		t.Fatalf("plan %+v", plan)
	}
}

// The packs by game, before they are made: one game for each base game of the folder, none
// packed yet. It needs games of the user, so it runs only where they are.
func TestPackPlanOfRealGames(t *testing.T) {
	folder := os.Getenv("NXPRESS_TEST_GAMES")
	if folder == "" {
		t.Skip("NXPRESS_TEST_GAMES is not set")
	}
	if _, err := jobs.LoadKeys(""); err != nil {
		t.Skip("no prod.keys")
	}
	files, _ := filepath.Glob(filepath.Join(folder, "*.ns*"))
	plan := (&App{}).PackPlan(files, nil, t.TempDir(), "")
	if len(plan) == 0 {
		t.Fatal("no game")
	}
	for _, game := range plan {
		t.Logf("%s %s %d files packed=%q", game.ID, game.Name, len(game.Files), game.Packed)
		if game.Packed != "" {
			t.Errorf("%s is told packed in an empty folder", game.Name)
		}
	}
}

// packFolderGame writes a game in a folder of its own, as collections keep them: the game, and
// its update and DLC in folders inside. A second game takes the title IDs after the first.
func packFolderGame(t *testing.T, folder string, second bool) (string, string, string) {
	t.Helper()
	base, update, dlc := uint64(testkit.BaseID), uint64(testkit.UpdateID), uint64(testkit.DlcID)
	if second {
		base, update, dlc = base+0x10000, update+0x10000, dlc+0x10000
	}
	return testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), base, 0, switchfs.ContentMetaType_Application, 1, nil),
		testkit.TitleNsp(t, filepath.Join(folder, "Updates", "update.nsp"), update, 65536, switchfs.ContentMetaType_Patch, 2, nil),
		testkit.TitleNsp(t, filepath.Join(folder, "DLC", "dlc.nsp"), dlc, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
}

// packsIn are the packs a folder holds: the files of more than one title.
func packsIn(t *testing.T, folder string) []string {
	t.Helper()
	packs := []string{}
	for _, name := range testkit.List(folder) {
		if titles, err := jobs.ReadTitles(filepath.Join(folder, name)); err == nil && len(titles) > 1 {
			packs = append(packs, name)
		}
	}
	return packs
}

// A folder of games, each in a folder of its own, combined by game: the SuperNSP of each game
// goes next to its folder, not inside it (#45). Given again, each game is combined already.
func TestPackGamesOfFoldersGoNextToThem(t *testing.T) {
	testkit.Isolate(t)
	collection := filepath.Join(t.TempDir(), "test")
	packFolderGame(t, filepath.Join(collection, "Alpha"), false)
	packFolderGame(t, filepath.Join(collection, "Beta"), true)
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: []string{collection}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	packs := packsIn(t, collection)
	if len(packs) != 2 || !strings.Contains(packs[0], "[0100000000010000]") || !strings.Contains(packs[1], "[0100000000020000]") {
		t.Fatalf("next to the folders: %v", testkit.List(collection))
	}
	for _, game := range []string{"Alpha", "Beta"} {
		if inside := testkit.List(filepath.Join(collection, game)); !reflect.DeepEqual(inside, []string{"DLC", "Updates", "game.nsp"}) {
			t.Errorf("%s holds %v", game, inside)
		}
	}
	out := &testkit.Results{}
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: []string{collection}}, out); failed != 0 {
		t.Fatalf("again: %d failed", failed)
	}
	for path, result := range out.Results {
		if !jobs.IsSkipped(result.Err) || !strings.HasPrefix(result.Err.Error(), "already combined in ") {
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
	testkit.Isolate(t)
	collection := filepath.Join(t.TempDir(), "test")
	base, update, dlc := packFolderGame(t, filepath.Join(collection, "Alpha"), false)
	files := []string{base, update, dlc}
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: files, Origins: []string{collection}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if packs := packsIn(t, collection); len(packs) != 1 {
		t.Fatalf("next to the folder: %v", testkit.List(collection))
	}
	if plan := (&App{}).PackPlan(files, []string{collection}, "", ""); len(plan) != 1 || plan[0].Packed == "" {
		t.Errorf("the plan %+v", plan)
	}
}

// Files given one by one keep their pack next to them, in the folder of the game; and so do
// the games of a folder that holds the files of several games together, with no folder each.
func TestPackOfFilesStaysNextToThem(t *testing.T) {
	testkit.Isolate(t)
	root := t.TempDir()
	game := filepath.Join(root, "Alpha")
	base, update, dlc := packFolderGame(t, game, false)
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: []string{base, update, dlc}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if packs := packsIn(t, game); len(packs) != 1 || len(packsIn(t, root)) != 0 {
		t.Fatalf("next to the files: %v, above: %v", testkit.List(game), testkit.List(root))
	}

	together := filepath.Join(root, "together")
	testkit.Game(t, together)
	testkit.TitleNsp(t, filepath.Join(together, "other.nsp"), testkit.BaseID+0x10000, 0, switchfs.ContentMetaType_Application, 4, nil)
	testkit.TitleNsp(t, filepath.Join(together, "other update.nsp"), testkit.UpdateID+0x10000, 65536, switchfs.ContentMetaType_Patch, 5, nil)
	if failed := jobs.PackGames(context.Background(), jobs.Options{PackGames: true, Files: []string{together}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("together: %d failed", failed)
	}
	if packs := packsIn(t, together); len(packs) != 2 {
		t.Fatalf("together: %v, above: %v", testkit.List(together), testkit.List(root))
	}
}

// On the command line, which takes the files of the folders given in their place, the folders
// still count: one file with everything of a folder goes next to it, and so does the SuperNSP
// of each game of a collection.
func TestPackOfAFolderOnTheCommandLineGoesNextToIt(t *testing.T) {
	testkit.Isolate(t)
	keys := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(keys, []byte("header_key = "+testkit.HeaderKey+"\nkey_area_key_application_00 = "+testkit.AreaKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := cli.RunCommand(append([]string{"--keys", keys}, args...), &stdout, &stderr); code != 0 {
			t.Fatalf("%v: %d %s%s", args, code, stdout.String(), stderr.String())
		}
	}
	collection := filepath.Join(t.TempDir(), "test")
	game := filepath.Join(collection, "Alpha")
	packFolderGame(t, game, false)
	command("--pack-one", "--recursive", game)
	packs := packsIn(t, collection)
	if len(packs) != 1 || len(packsIn(t, game)) != 0 {
		t.Fatalf("next to the folder: %v; inside: %v", testkit.List(collection), testkit.List(game))
	}
	if got := packTitles(t, filepath.Join(collection, packs[0])); !reflect.DeepEqual(got, testkit.WholeGame) {
		t.Fatalf("the pack holds %v", got)
	}

	other := filepath.Join(t.TempDir(), "test")
	packFolderGame(t, filepath.Join(other, "Alpha"), false)
	packFolderGame(t, filepath.Join(other, "Beta"), true)
	command("--pack-games", "--recursive", other)
	if packs := packsIn(t, other); len(packs) != 2 {
		t.Fatalf("by game: %v", testkit.List(other))
	}
}

// packTitles are the titles a pack holds, as "<title ID> v<version>", in order.
func packTitles(t *testing.T, path string) []string {
	t.Helper()
	titles, err := jobs.ReadTitles(path)
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
