package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/testkit"
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
	files, missing := jobs.Inputs(given, jobs.Takes["verify"])
	if !reflect.DeepEqual(files, []string{game, other}) || len(missing) > 0 {
		t.Fatalf("files %v, missing %v", files, missing)
	}
}

// A folder that cannot be read is told among the missing paths, not left out without a word.
func TestInputsTellUnreadableFolders(t *testing.T) {
	folder := t.TempDir()
	game := filepath.Join(folder, "a.nsp")
	os.WriteFile(game, []byte("x"), 0o644)
	locked := testkit.UnreadableFolder(t, folder)
	files, missing := jobs.Inputs([]string{folder}, jobs.Takes["verify"])
	if !reflect.DeepEqual(files, []string{game}) || !reflect.DeepEqual(missing, []string{locked}) {
		t.Fatalf("files %v, missing %v", files, missing)
	}
	found, unread, nested := jobs.FolderFiles(folder, false)
	if !reflect.DeepEqual(found, []string{game}) || len(unread) != 0 || !nested {
		t.Fatalf("one level: %v %v %v", found, unread, nested)
	}
	if _, unread, _ := jobs.FolderFiles(folder, true); len(unread) != 1 || unread[0].Path != locked || unread[0].Err == nil {
		t.Fatalf("unread: %v", unread)
	}
}
