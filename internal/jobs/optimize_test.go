package jobs

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/optimize"
	"github.com/DeLFuS77/NXPress/internal/testkit"
)

// --optimize is an action of its own; -C with it makes its files NSZ, and what would lose the
// originals or make a file installers do not read is refused.
func TestOptimizeOptions(t *testing.T) {
	for args, want := range map[string]string{
		"--optimize x":              "",
		"--optimize -C x":           "",
		"--opt x":                   "",
		"--optimize -D x":           "choose one of",
		"--optimize --rm-source x":  "--rm-source does not go with --optimize",
		"--optimize --watch x":      "--watch does not go with",
		"--optimize --extra -C x":   "--extra does not go with --optimize",
		"--optimize --pack-games x": "choose one of",
	} {
		o, err := ParseOptions(strings.Fields(args))
		if err == nil {
			err = o.Check()
		}
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", args, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: told %v, not %q", args, err, want)
		case want == "" && o.Action() != "optimize":
			t.Errorf("%s: the action is %s", args, o.Action())
		}
	}
	o, _ := ParseOptions([]string{"--optimize", "--pack-name", "{name}", "x"})
	if o.Check(); len(o.Notes) != 0 || optimizeTemplate(o) != "{name}" {
		t.Errorf("--pack-name with --optimize: %v, %q", o.Notes, optimizeTemplate(o))
	}
	if !strings.Contains(Usage, "--optimize") || !strings.Contains(Usage, "sigpatches") || !strings.Contains(Usage, "NXDock or DBI") || !strings.Contains(Usage, "official updates") {
		t.Error("the help does not tell --optimize, the sigpatches and the official updates")
	}
	// an update left on the console is read over the optimized game, which then closes with an error
	if !strings.Contains(strings.Join(strings.Fields(Usage), " "), "after deleting the update of the game from the console") {
		t.Error("the help does not tell to delete the update of the game from the console before installing")
	}
}

// What cannot be optimized is told and left as it is: a DLC, a game without its update, and a
// game and an update that do not read as one (the synthetic update is no patch); nothing new is
// left in the folder.
func TestOptimizeGamesLeavesWhatItCannot(t *testing.T) {
	testkit.Isolate(t)
	folder := filepath.Join(t.TempDir(), "games")
	base, update, dlc := testkit.Game(t, folder)
	results := &testkit.Results{}
	failed := OptimizeGames(context.Background(), Options{Optimize: true, Threads: 2, Files: []string{folder}}, results)
	if !optimize.Built {
		if failed != 3 {
			t.Fatalf("without cgo: %d failed", failed)
		}
		return
	}
	if failed != 0 {
		t.Fatalf("%d failed: %v", failed, results.Results)
	}
	for path, want := range map[string]string{dlc: "a DLC is not optimized", base: "not optimized:", update: "not optimized:"} {
		got := results.Results[path].Err
		if got == nil || !IsSkipped(got) || !strings.Contains(got.Error(), want) {
			t.Errorf("%s: %v", filepath.Base(path), got)
		}
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"base.nsp", "dlc.nsp", "update.nsp"}) {
		t.Fatalf("left: %v", left)
	}

	results = &testkit.Results{}
	OptimizeGames(context.Background(), Options{Optimize: true, Files: []string{base}}, results)
	if got := results.Results[base].Err; got == nil || !strings.Contains(got.Error(), "its update is not among the files given") {
		t.Fatalf("a game alone: %v", got)
	}
}

// The name of an optimized game: the game at the version of its newest update, one title.
func TestOptimizeName(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, _ := testkit.Game(t, folder)
	name, err := optimizeName(Options{}, []string{base, update}, ".nsp", folder)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(name); !strings.Contains(got, "[0100000000010000][v65536] (optimized).nsp") {
		t.Fatalf("named %s", got)
	}
}
