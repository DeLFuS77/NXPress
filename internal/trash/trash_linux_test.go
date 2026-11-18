package trash

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// homeIn makes the home trash a folder of the test, beside the files it moves, so the trash of
// the person who runs the tests is never touched.
func homeIn(t *testing.T) (folder string, home string) {
	t.Helper()
	folder = t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(folder, "data"))
	return folder, filepath.Join(folder, "data", "Trash")
}

func write(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func info(t *testing.T, home string, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "info", name+".trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A file goes to the home trash with its .trashinfo: where it was, as a URL, and when it went;
// one of the same name goes there too, under a name of its own; and so do names in other
// alphabets and a folder, whole.
func TestMoveToTheHomeTrash(t *testing.T) {
	folder, home := homeIn(t)
	path := filepath.Join(folder, "Game [0100000000010000][v0].nsp")
	for _, text := range []string{"first", "second"} {
		write(t, path, text)
		if err := Check(path); err != nil {
			t.Fatal(err)
		}
		if err := Move(path); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("the file is still there")
		}
	}
	for name, text := range map[string]string{"Game [0100000000010000][v0].nsp": "first", "Game [0100000000010000][v0].2.nsp": "second"} {
		if data, err := os.ReadFile(filepath.Join(home, "files", name)); err != nil || string(data) != text {
			t.Errorf("%s: %q %v", name, data, err)
		}
		want := "[Trash Info]\nPath=" + escape(path) + "\nDeletionDate="
		if got := info(t, home, name); !strings.HasPrefix(got, want) || !strings.Contains(got, "%5B0100000000010000%5D") {
			t.Errorf("%s: %q", name, got)
		}
	}
	date := strings.TrimSpace(strings.SplitN(info(t, home, "Game [0100000000010000][v0].nsp"), "DeletionDate=", 2)[1])
	if when, err := time.ParseInLocation("2006-01-02T15:04:05", date, time.Local); err != nil || time.Since(when) > time.Minute {
		t.Errorf("DeletionDate %q", date)
	}

	unicode := filepath.Join(folder, "ゲーム ñ.nsz")
	write(t, unicode, "unicode")
	if err := Move(unicode); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info(t, home, "ゲーム ñ.nsz"), "/%E3%82%B2%E3%83%BC%E3%83%A0%20%C3%B1.nsz\n") {
		t.Errorf("%q", info(t, home, "ゲーム ñ.nsz"))
	}

	dump := filepath.Join(folder, "Game dump")
	write(t, filepath.Join(dump, "base", "exefs", "main"), "code")
	if err := Move(dump); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(home, "files", "Game dump", "base", "exefs", "main")); err != nil || string(data) != "code" {
		t.Errorf("the folder is not whole in the trash: %q %v", data, err)
	}
	for _, inside := range []string{home, filepath.Join(home, "files"), filepath.Join(home, "info")} {
		if stat, err := os.Stat(inside); err != nil || stat.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v", inside, stat.Mode(), err)
		}
	}
}

// The trash of another drive: $topdir/.Trash-$uid, made when it is not there, whose .trashinfo
// files tell where each item was from the top of the drive; $topdir/.Trash/$uid when the
// administrator made .Trash with the sticky bit.
func TestTheTrashOfAnotherDrive(t *testing.T) {
	topdir := t.TempDir()
	uid := os.Getuid()
	dir := topdirTrash(topdir, uid)
	if dir != filepath.Join(topdir, ".Trash-"+strconv.Itoa(uid)) {
		t.Fatalf("%q", dir)
	}
	path := filepath.Join(topdir, "Switch", "Game.nsz")
	write(t, path, "game")
	if err := (place{dir: dir, topdir: topdir}).put(path, time.Date(2026, 11, 18, 21, 5, 9, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	if got := info(t, dir, "Game.nsz"); got != "[Trash Info]\nPath=Switch/Game.nsz\nDeletionDate=2026-11-18T21:05:09\n" {
		t.Errorf("%q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", "Game.nsz")); err != nil {
		t.Error(err)
	}

	shared := t.TempDir()
	if err := os.Mkdir(filepath.Join(shared, ".Trash"), 0o777); err != nil {
		t.Fatal(err)
	}
	// without the sticky bit it is not used
	if dir := topdirTrash(shared, uid); dir != filepath.Join(shared, ".Trash-"+strconv.Itoa(uid)) {
		t.Errorf("without the sticky bit: %q", dir)
	}
	if err := os.Chmod(filepath.Join(shared, ".Trash"), 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	if dir := topdirTrash(shared, uid); dir != filepath.Join(shared, ".Trash", strconv.Itoa(uid)) {
		t.Errorf("with the sticky bit: %q", dir)
	}
}

// A drive whose trash cannot be made has none: the file stays where it is.
func TestADriveWithoutATrash(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes in any folder")
	}
	topdir := t.TempDir()
	path := filepath.Join(topdir, "Game.nsz")
	write(t, path, "game")
	if err := os.Chmod(topdir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(topdir, 0o700) })
	if dir := topdirTrash(topdir, os.Getuid()); dir != "" {
		t.Fatalf("a trash in a folder that cannot be written: %q", dir)
	}
	// a link in place of the trash of the user is not followed
	other := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(other, ".Trash-"+strconv.Itoa(os.Getuid()))); err != nil {
		t.Fatal(err)
	}
	if dir := topdirTrash(other, os.Getuid()); dir != "" {
		t.Errorf("a link taken for the trash: %q", dir)
	}
}

func TestEscapeAndNumbered(t *testing.T) {
	if got := escape("/games/a b/ñ%#?.nsp"); got != "/games/a%20b/%C3%B1%25%23%3F.nsp" {
		t.Errorf("escape: %q", got)
	}
	for name, want := range map[string]string{"Game.nsp": "Game.3.nsp", "Game dump": "Game dump.3", ".hidden": ".hidden.3"} {
		if got := numbered(name, 3); got != want {
			t.Errorf("%s: %q", name, got)
		}
	}
}
