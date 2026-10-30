package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitFileKeepsTheBytes(t *testing.T) {
	folder := t.TempDir()
	temporary := filepath.Join(folder, ".game.nsz.nxpress")
	target := filepath.Join(folder, "game.nsz")
	data := bytes.Repeat([]byte("switch"), 1000)
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := commitFile(temporary, target); err != nil {
		t.Fatal(err)
	}
	if read, err := os.ReadFile(target); err != nil || !bytes.Equal(read, data) {
		t.Fatalf("the file must keep its bytes: %v", err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("the temporary name must be gone")
	}
}

// the parts of a FAT32 split, in their folder, are written to the disk one by one
func TestSyncPathOfAFolder(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "game.nsp")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"00", "01"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncPath(folder); err != nil {
		t.Fatal(err)
	}
	if err := syncPath(filepath.Join(folder, "02")); err == nil {
		t.Fatal("a file that is not there must fail")
	}
}
