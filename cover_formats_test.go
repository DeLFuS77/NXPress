package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An AVIF or ICO chosen for an icon is given to the window to read, as an address of data.
func TestCoverImageGivesTheWindowWhatNXPressDoesNotRead(t *testing.T) {
	for _, name := range []string{"a.avif", "b.ICO"} {
		path := filepath.Join(t.TempDir(), name)
		os.WriteFile(path, []byte("not read here"), 0o644)
		cover, err := coverImage(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(cover.Convert, "data:image/") || cover.Path != "" || cover.Name != name {
			t.Errorf("%s: %+v", name, cover)
		}
	}
}
