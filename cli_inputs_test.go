package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// The same file given under two names is done once: through a link to it, and on Windows by
// its name in other capitals.
func TestInputsTakeTheSameFileOnce(t *testing.T) {
	folder := t.TempDir()
	game := filepath.Join(folder, "game.nsp")
	os.WriteFile(game, []byte("x"), 0o644)
	other := filepath.Join(folder, "other.nsp")
	os.WriteFile(other, []byte("y"), 0o644)
	given := []string{game, folder}
	link := filepath.Join(folder, "link.nsp")
	if err := os.Link(game, link); err == nil {
		given = append(given, link)
	}
	if runtime.GOOS == "windows" {
		given = append(given, filepath.Join(folder, "GAME.NSP"), filepath.Join(folder, "Other.Nsp"))
	}
	files, missing := inputs(given, takes["verify"])
	if !reflect.DeepEqual(files, []string{game, other}) || len(missing) > 0 {
		t.Fatalf("files %v, missing %v", files, missing)
	}
}

// unreadableFolder makes a folder in folder that cannot be read: one whose name ends in a dot on
// Windows, which only the long form of its path reaches, and one without permissions elsewhere.
func unreadableFolder(t *testing.T, folder string) string {
	if runtime.GOOS == "windows" {
		path := filepath.Join(folder, "locked.")
		if err := os.Mkdir(`\\?\`+path, 0o755); err != nil {
			t.Skip(err)
		}
		os.WriteFile(`\\?\`+path+`\game.nsp`, []byte("x"), 0o644)
		return path
	}
	path := filepath.Join(folder, "locked")
	os.Mkdir(path, 0o755)
	os.WriteFile(filepath.Join(path, "game.nsp"), []byte("x"), 0o644)
	os.Chmod(path, 0)
	t.Cleanup(func() { os.Chmod(path, 0o755) })
	if _, err := os.ReadDir(path); err == nil {
		t.Skip("the folder can be read without permissions (root)")
	}
	return path
}

// A folder that cannot be read is told among the missing paths, not left out without a word.
func TestInputsTellUnreadableFolders(t *testing.T) {
	folder := t.TempDir()
	game := filepath.Join(folder, "a.nsp")
	os.WriteFile(game, []byte("x"), 0o644)
	locked := unreadableFolder(t, folder)
	files, missing := inputs([]string{folder}, takes["verify"])
	if !reflect.DeepEqual(files, []string{game}) || !reflect.DeepEqual(missing, []string{locked}) {
		t.Fatalf("files %v, missing %v", files, missing)
	}
	found, unread, nested := folderFiles(folder, false)
	if !reflect.DeepEqual(found, []string{game}) || len(unread) != 0 || !nested {
		t.Fatalf("one level: %v %v %v", found, unread, nested)
	}
	if _, unread, _ := folderFiles(folder, true); len(unread) != 1 || unread[0].path != locked || unread[0].err == nil {
		t.Fatalf("unread: %v", unread)
	}
}
