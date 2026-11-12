package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The folder of the image holds the app and the link to Applications, and nothing else: no
// script, no installer.
func TestDmgFolder(t *testing.T) {
	work := t.TempDir()
	folder, err := dmgFolder(work, func(to string) error {
		if err := os.MkdirAll(filepath.Join(to, "Contents", "MacOS"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, "Contents", "MacOS", "nxpress"), []byte("the program"), 0o755)
	})
	if err != nil {
		// Windows makes links only for an administrator or in developer mode, and writes / as \
		if strings.Contains(err.Error(), "symlink") && os.Getenv("CI") == "" {
			t.Skip("no links here:", err)
		}
		t.Fatal(err)
	}
	if filepath.Base(folder) != volumeName {
		t.Errorf("the folder is %s", folder)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"Applications", "NXPress.app"}) {
		t.Errorf("the image holds %v", names)
	}
	if target, err := os.Readlink(filepath.Join(folder, "Applications")); err != nil || filepath.ToSlash(target) != "/Applications" {
		t.Errorf("the link to Applications: %q %v", target, err)
	}
	if _, err := os.Stat(filepath.Join(folder, "NXPress.app", "Contents", "MacOS", "nxpress")); err != nil {
		t.Error(err)
	}
}

// The image is compressed, named NXPress, and made of that folder only.
func TestDmgArgs(t *testing.T) {
	got := strings.Join(dmgArgs("/tmp/w/NXPress", "dist/nxpress-macos-intel.dmg"), " ")
	want := "create -volname NXPress -srcfolder /tmp/w/NXPress -fs HFS+ -format UDZO -ov dist/nxpress-macos-intel.dmg"
	if got != want {
		t.Errorf("%s", got)
	}
}
