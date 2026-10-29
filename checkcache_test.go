package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// a cache of the files checked of its own, in a folder of the test
func testCheckCache(t *testing.T, content string) (*checkCache, string) {
	t.Helper()
	folder := t.TempDir()
	cache := &checkCache{path: filepath.Join(folder, "checked.json")}
	if content != "" {
		if err := os.WriteFile(cache.path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cache, folder
}

// a game of its own size in the folder
func testGame(t *testing.T, folder string, name string, size int) string {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// a cache that is null or broken starts again empty, and keeps the next file checked
func TestCheckCacheNullOrBroken(t *testing.T) {
	for _, content := range []string{"null", "{broken", `{"a.nsp": 5}`, `[]`} {
		cache, folder := testCheckCache(t, content)
		game := testGame(t, folder, "a.nsp", 100)
		if _, ok := cache.find(game); ok {
			t.Errorf("%s: a file never checked is found", content)
		}
		cache.keep(game, "")
		if _, ok := cache.find(game); !ok {
			t.Errorf("%s: the file checked is not kept", content)
		}
	}
}

// a game copied again with the same size and date, as Explorer and robocopy leave it, is not the
// file checked; the file checked is, also once the cache is read again
func TestCheckCacheCopySameDate(t *testing.T) {
	cache, folder := testCheckCache(t, "")
	game := testGame(t, folder, "a.nsp", 100)
	info, err := os.Stat(game)
	if err != nil {
		t.Fatal(err)
	}
	cache.keep(game, "")
	if _, ok := cache.find(game); !ok {
		t.Fatal("the file checked is not found")
	}
	if _, ok := (&checkCache{path: cache.path}).find(game); !ok {
		t.Fatal("the file checked is not found once the cache is read again")
	}
	sameSizeAndDate := func(name string) {
		t.Helper()
		if err := os.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		if now, err := os.Stat(name); err != nil || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) {
			t.Fatalf("the copy has another size or date: %v", err)
		}
	}
	// the clock of the file system moves on between the check and the copy
	time.Sleep(50 * time.Millisecond)
	// written again in place, as a copy over the old file
	if err := os.WriteFile(game, bytes.Repeat([]byte{1}, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	sameSizeAndDate(game)
	if _, ok := cache.find(game); ok {
		t.Error("a game written again with its date set back passes as checked")
	}
	// a new copy put in its place
	cache.keep(game, "")
	copied := testGame(t, folder, "a.nsp.copy", 100)
	sameSizeAndDate(copied)
	if err := os.Rename(copied, game); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.find(game); ok {
		t.Error("a copy put in the place of the game passes as checked")
	}
}

// an entry kept before the number and the time of change of a file were is checked again once,
// without an error
func TestCheckCacheOldEntry(t *testing.T) {
	folder := t.TempDir()
	game := testGame(t, folder, "a.nsp", 100)
	info, err := os.Stat(game)
	if err != nil {
		t.Fatal(err)
	}
	old, err := json.Marshal(map[string]any{game: map[string]any{"size": 100, "modified": info.ModTime(), "checked": time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	cache, _ := testCheckCache(t, string(old))
	if _, ok := cache.find(game); ok {
		t.Error("an old entry passes as checked")
	}
	cache.keep(game, "")
	if _, ok := cache.find(game); !ok {
		t.Error("the game checked again is not kept")
	}
}
