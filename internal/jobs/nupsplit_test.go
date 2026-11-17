package jobs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// Split takes out of a NUP only the parts chosen: here the game and its DLC, without the update
// nor the icon; a NUP with no choice gives all of it, with its icon (#138).
func TestSplitNupTakesThePartsChosen(t *testing.T) {
	packages := []testnup.Package{
		{TitleId: 0x0400245021D2C000, Kind: testnup.Base,
			Meta:  []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Book")}, {Type: testnup.Icon, Payload: testnup.Jpeg}},
			Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte(strings.Repeat("a", 5000)), Zstd: true}}},
		{TitleId: 0x0400245021D2C800, Kind: testnup.Update, Version: 65536,
			Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte(strings.Repeat("b", 5000)), Zstd: true}}},
		{TitleId: 0x0400245021D2D001, Kind: testnup.Addon,
			Files: []testnup.File{{Path: "romfs/c.bin", Data: []byte("dlc")}}},
	}
	dir := t.TempDir()
	some := filepath.Join(dir, "some.nup")
	all := filepath.Join(dir, "all.nup")
	os.WriteFile(some, testnup.Build(packages), 0o644)
	os.WriteFile(all, testnup.Build(packages), 0o644)
	out := &recording{}
	failed := RunJobs(t.Context(), Options{Split: true, Files: []string{some, all}, Multi: 1,
		SplitParts: map[string]SplitParts{some: {Base: true, Dlc: true}}}, out)
	if failed != 0 || len(out.skipped) != 0 {
		t.Fatalf("failed %d %+v", failed, out)
	}
	listed := func(folder string) []string {
		names := []string{}
		filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(folder, path)
				names = append(names, filepath.ToSlash(rel))
			}
			return nil
		})
		sort.Strings(names)
		return names
	}
	someFiles, allFiles := listed(filepath.Join(dir, "some")), listed(filepath.Join(dir, "all"))
	has := func(files []string, part string) bool {
		for _, f := range files {
			if strings.Contains(f, part) {
				return true
			}
		}
		return false
	}
	if !has(someFiles, "a.bin") || !has(someFiles, "c.bin") || has(someFiles, nupIconFile) || len(someFiles) != 2 {
		t.Errorf("some: %v", someFiles)
	}
	if len(allFiles) != 4 || !has(allFiles, nupIconFile) {
		t.Errorf("all: %v", allFiles)
	}

	// nothing chosen: skipped, and nothing written
	none := filepath.Join(dir, "none.nup")
	os.WriteFile(none, testnup.Build(packages), 0o644)
	out = &recording{}
	RunJobs(t.Context(), Options{Split: true, Files: []string{none}, Multi: 1, SplitParts: map[string]SplitParts{none: {}}}, out)
	if len(out.skipped) != 1 || !strings.Contains(out.skipped[0], "nothing to take out") {
		t.Errorf("%+v", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "none")); err == nil {
		t.Error("a folder was written with nothing chosen")
	}
}
