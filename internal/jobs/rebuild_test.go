package jobs

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/optimize"
	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// --rebuild is an action of its own; -C with it makes its files NSZ, and it never deletes the
// original. The help tells it needs sigpatches.
func TestRebuildOptions(t *testing.T) {
	for args, want := range map[string]string{
		"--rebuild x":             "",
		"--rebuild -C x":          "",
		"--rebuild -D x":          "choose one of",
		"--rebuild --optimize x":  "choose one of",
		"--rebuild --rm-source x": "--rm-source does not go with --rebuild",
		"--rebuild --extra -C x":  "--extra does not go with --rebuild",
	} {
		o, err := ParseOptions(strings.Fields(args))
		if err == nil {
			err = o.Check()
		}
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", args, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: %v, not %q", args, err, want)
		case want == "" && o.Action() != "rebuild":
			t.Errorf("%s: action %s", args, o.Action())
		}
	}
	if !strings.Contains(Usage, "--rebuild") || !strings.Contains(Usage, "it needs sigpatches") {
		t.Error("the help does not tell --rebuild and the sigpatches")
	}
}

// A package rebuilt by RunJobs: "(rebuilt)" next to it, NSP or NSZ, the original kept, and told
// what was made right.
func TestRebuildWritesNextToIt(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, map[string][]byte{"x.xml": []byte("<x/>")})
	for _, compress := range []bool{false, true} {
		results := &testkit.Results{}
		failed := RunJobs(context.Background(), Options{Rebuild: true, Compress: compress, Level: 3, Threads: 2, Multi: 1, Files: []string{filepath.Join(folder, "game.nsp")}}, results)
		if !optimize.Built {
			if failed != 1 {
				t.Fatalf("without cgo: %d failed", failed)
			}
			return
		}
		if failed != 0 {
			t.Fatalf("%d failed: %v", failed, results.Results)
		}
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"game (rebuilt).nsp", "game (rebuilt).nsz", "game.nsp"}) {
		t.Fatalf("left: %v", left)
	}
	check, err := switchfs.VerifyFile(context.Background(), filepath.Join(folder, "game (rebuilt).nsp"), nil)
	if err != nil || len(check.Modified) > 0 || check.Partial {
		t.Fatalf("%v %+v", err, check)
	}
	if note := RebuildNote(optimize.RebuildResult{Titles: 1, NCAs: 1, Dropped: 1}); !strings.Contains(note, "1 files left out") || !strings.Contains(note, "sigpatches") {
		t.Fatal(note)
	}
}

// -V tells a CNMT that lists a content the file does not hold, as installers such as Sphaira do
// not find it (MODIFIED, not failed), and --rebuild pairs it with the NCA of its type.
func TestVerifyTellsContentsNotInTheFile(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	program := testkit.ProgramNcaWith(testkit.GameData(0x10000, 1), nil)
	path := testkit.NspListing(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), "fedcba9876543210fedcba9876543210", nil)
	if got := UnlistedContents(path); !reflect.DeepEqual(got, []string{"fedcba9876543210fedcba9876543210"}) {
		t.Fatalf("missing: %v", got)
	}
	_, _, err := verify(context.Background(), path, Options{Verify: true, NoCache: true}, testkit.Quiet{})
	if !IsModified(err) || !strings.Contains(err.Error(), "not in the file") {
		t.Fatalf("told %v", err)
	}
	clean := testkit.TitleNsp(t, filepath.Join(folder, "clean.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 2, nil)
	if got := UnlistedContents(clean); len(got) != 0 {
		t.Fatalf("a clean package: %v", got)
	}
	if !optimize.Built {
		return
	}
	results := &testkit.Results{}
	if failed := RunJobs(context.Background(), Options{Rebuild: true, Threads: 1, Multi: 1, Files: []string{path}}, results); failed != 0 {
		t.Fatalf("%v", results.Results)
	}
	if got := UnlistedContents(filepath.Join(folder, "game (rebuilt).nsp")); len(got) != 0 {
		t.Fatalf("rebuilt: %v", got)
	}
}
