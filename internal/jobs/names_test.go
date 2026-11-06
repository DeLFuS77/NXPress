package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// A dump folder of a game of the Switch 2 without control data takes the name the list of the
// games of the Switch 2 gives it, in its NUP and in the name of the NUP; without the list, the
// one of its folder.
func TestPackNupNameFromTheList(t *testing.T) {
	folder := t.TempDir()
	list := `{"titles":{"0400D4201C6EA000":{"name":"Made-up Racer 2"}}}`
	if err := os.WriteFile(filepath.Join(folder, titledb.NamesFile), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := titledb.LoadNames(folder)
	if err != nil {
		t.Fatal(err)
	}
	UseNames(names)
	t.Cleanup(func() { UseNames(nil) })

	dir := writeDump(t, "MR2 ROM")
	title, err := dumpTitle(dir)
	if err != nil || title.Name != "Made-up Racer 2" || title.NupName != "Made-up Racer 2 [0400D4201C6EA000][v0].nup" {
		t.Fatalf("%+v %v", title, err)
	}
	name, _, err := packNup(t.Context(), dir, Options{}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(name), "/Made-up Racer 2 [0400D4201C6EA000][v0].nup") {
		t.Errorf("named %s", name)
	}
	container, err := nup.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := container.Packages[0].Name(); got != "Made-up Racer 2" {
		t.Errorf("the NUP carries %q", got)
	}

	// without the list, the name of the folder
	UseNames(nil)
	if title, err := dumpTitle(writeDump(t, "MR2 ROM")); err != nil || title.Name != "MR2 ROM" {
		t.Errorf("without the list: %+v %v", title, err)
	}
	if got := dumpGameName(writeDump(t, "MR2 ROM")); got != "MR2" {
		t.Errorf("without the list the NUP carries %q", got)
	}
}
