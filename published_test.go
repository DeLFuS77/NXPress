package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// The page asks for the title IDs of its list and gets the newest version of the ones the list of
// versions knows; with no list, or for what is not a title ID, nothing.
func TestPublished(t *testing.T) {
	a := &App{}
	ids := []string{"0100A1B2C3D40000", "0100a1b2c3d41001", "0100E6F5D4C30000", "not an id", "0100A1B2C3D4"}
	if got := a.Published(ids); len(got) != 0 {
		t.Fatalf("with no versions: %v", got)
	}
	folder := t.TempDir()
	list := `{"0100a1b2c3d40000":{"65536":"2026-01-10","393216":"2026-10-21"},"0100a1b2c3d41001":{"65536":"2026-08-12"}}`
	if err := os.WriteFile(filepath.Join(folder, titledb.FileName), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	versions, err := titledb.Load(folder)
	if err != nil {
		t.Fatal(err)
	}
	a.versions.Store(versions)
	got := a.Published(ids)
	want := map[string]titledb.Published{"0100A1B2C3D40000": {Version: 393216, Date: "2026-10-21"}, "0100a1b2c3d41001": {Version: 65536, Date: "2026-08-12"}}
	if len(got) != len(want) {
		t.Fatalf("Published = %v, want %v", got, want)
	}
	for id, published := range want {
		if got[id] != published {
			t.Errorf("%s = %v, want %v", id, got[id], published)
		}
	}
}
