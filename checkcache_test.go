package main

import (
	"os"
	"path/filepath"
	"testing"
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
