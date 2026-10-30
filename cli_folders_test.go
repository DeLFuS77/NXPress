package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runForTest runs the command line with keys that are not there (none is needed to split for
// FAT32), and gives what it printed and its exit code.
func runForTest(t *testing.T, args ...string) (string, int) {
	var stdout, stderr bytes.Buffer
	keys := filepath.Join(t.TempDir(), "none.keys")
	code := runCommand(append([]string{"--keys", keys}, args...), &stdout, &stderr)
	return stdout.String() + stderr.String(), code
}

// gameFolder makes a folder with a game, a game in a folder inside it and a folder of parts.
func gameFolder(t *testing.T) string {
	folder := t.TempDir()
	for _, name := range []string{"b.nsp", "a.nsp", "sub/c.nsp", "parts.nsp/00", "notes.txt"} {
		path := filepath.Join(folder, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("x"), 0o644)
	}
	return folder
}

// The command line takes the files of a folder given, not the ones in the folders inside it,
// as nsz does; --recursive takes those too, and a folder of parts is one file either way.
func TestCommandLineTakesOneLevelOfAFolder(t *testing.T) {
	folder := gameFolder(t)
	var printed bytes.Buffer
	out := &reporter{out: &printed}
	files, failed := commandLineFiles(options{files: []string{folder, filepath.Join(folder, "sub", "c.nsp")}}, out)
	want := []string{filepath.Join(folder, "a.nsp"), filepath.Join(folder, "b.nsp"), filepath.Join(folder, "notes.txt"), filepath.Join(folder, "parts.nsp"), filepath.Join(folder, "sub", "c.nsp")}
	if !reflect.DeepEqual(files, want) || failed != 0 || !strings.Contains(printed.String(), "note: the folders in "+folder+" are left out (--recursive looks into them)") {
		t.Fatalf("one level: %v %d %q", files, failed, printed.String())
	}
	printed.Reset()
	files, _ = commandLineFiles(options{files: []string{folder}, recursive: true}, out)
	if len(files) != 5 || files[4] != filepath.Join(folder, "sub", "c.nsp") || printed.Len() > 0 {
		t.Fatalf("recursive: %v %q", files, printed.String())
	}

	printed.Reset()
	text, code := runForTest(t, "--fat32", folder)
	if code != 0 || !strings.Contains(text, "OK a.nsp (it fits in FAT32 as it is)") || !strings.Contains(text, "OK b.nsp") || strings.Contains(text, "c.nsp") {
		t.Fatalf("one level: %d %s", code, text)
	}
	text, code = runForTest(t, "--fat32", "--recursive", folder)
	if code != 0 || !strings.Contains(text, "OK c.nsp (it fits in FAT32 as it is)") || strings.Contains(text, "left out") {
		t.Fatalf("recursive: %d %s", code, text)
	}
}

// A folder given that cannot be read fails, with why, and the exit code is not 0.
func TestCommandLineTellsAnUnreadableFolder(t *testing.T) {
	folder := t.TempDir()
	locked := unreadableFolder(t, folder)
	text, code := runForTest(t, "--fat32", "--recursive", folder)
	if code != 1 || !strings.Contains(text, "FAILED "+filepath.Base(locked)+": ") {
		t.Fatalf("%d %s", code, text)
	}
	text, code = runForTest(t, "--fat32", locked)
	if code != 1 || !strings.Contains(text, "FAILED "+filepath.Base(locked)+": ") {
		t.Fatalf("given: %d %s", code, text)
	}
}
