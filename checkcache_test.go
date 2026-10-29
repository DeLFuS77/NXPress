package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// a cache of the files checked of its own, in a folder of the test
func testCheckCache(t *testing.T, content string) (*checkCache, string) {
	t.Helper()
	folder := t.TempDir()
	cache := testCache(t, filepath.Join(folder, "checked.json"))
	if content != "" {
		if err := os.WriteFile(cache.path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cache, folder
}

// testCache is a cache read from path, done with before the folder of the test goes
func testCache(t *testing.T, path string) *checkCache {
	cache := &checkCache{path: path}
	t.Cleanup(func() {
		cache.purging.Wait()
		cache.flush()
	})
	return cache
}

// the files a cache written at path holds
func testCacheFile(t *testing.T, path string) map[string]checkedFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]checkedFile{}
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	return files
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
	cache.flush()
	if _, ok := testCache(t, cache.path).find(game); !ok {
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

// keeping a checked file looks at that file alone: it takes no longer with many files in the
// cache, and nothing is written before a while later or the end of the work
func TestCheckCacheKeepDoesNotGrow(t *testing.T) {
	timeKeep := func(entries int) time.Duration {
		cache, folder := testCheckCache(t, "")
		game := testGame(t, folder, "a.nsp", 100)
		cache.load()
		cache.mutex.Lock()
		for i := range entries {
			cache.files[filepath.Join(folder, "gone", strconv.Itoa(i)+".nsp")] = checkedFile{Checked: time.Now()}
		}
		cache.mutex.Unlock()
		// the fastest of three rounds of 50 files kept
		fastest := time.Hour
		for range 3 {
			start := time.Now()
			for range 50 {
				cache.keep(game, "")
			}
			fastest = min(fastest, time.Since(start))
		}
		if _, err := os.Stat(cache.path); err == nil {
			t.Errorf("with %d files the cache is written at each file kept", entries)
		}
		cache.flush()
		if files := testCacheFile(t, cache.path); len(files) != entries+1 {
			t.Errorf("with %d files the cache written at the end holds %d", entries, len(files))
		}
		return fastest
	}
	few, many := timeKeep(10), timeKeep(5000)
	t.Logf("keeping 50 files takes %v with 10 files in the cache, %v with 5000", few, many)
	if many > 10*few+5*time.Millisecond {
		t.Errorf("keeping 50 files takes %v with 5000 files in the cache, %v with 10", many, few)
	}
}

// the cache is written a while after a file is kept, or at the end of the work, each time as a
// new file that takes the place of the old one
func TestCheckCacheWritten(t *testing.T) {
	delay := checkCacheDelay
	checkCacheDelay = 20 * time.Millisecond
	defer func() { checkCacheDelay = delay }()
	cache, folder := testCheckCache(t, "")
	first := testGame(t, folder, "a.nsp", 100)
	cache.keep(first, "")
	for start := time.Now(); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(cache.path); err == nil {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the cache is not written a while after a file is kept")
		}
	}
	checkCacheDelay = time.Hour
	second := testGame(t, folder, "b.nsp", 50)
	cache.keep(second, "note")
	cache.flush()
	files := testCacheFile(t, cache.path)
	if _, ok := files[first]; !ok || files[second].Note != "note" || len(files) != 2 {
		t.Errorf("the cache written holds %v", files)
	}
	// nothing is left beside it
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, " ") != "a.nsp b.nsp checked.json" {
		t.Errorf("the folder of the cache holds %v", names)
	}
}

// once read, the cache leaves out the files gone or changed on the drives that are here, and
// keeps the ones of a drive not plugged in, or of another microSD where theirs was
func TestCheckCachePurge(t *testing.T) {
	cache, folder := testCheckCache(t, "")
	kept := testGame(t, folder, "kept.nsp", 100)
	gone := testGame(t, folder, "gone.nsp", 100)
	changed := testGame(t, folder, "changed.nsp", 100)
	old := filepath.Join(folder, "old")
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	inOld := testGame(t, old, "a.nsp", 100)
	for _, path := range []string{kept, gone, changed, inOld} {
		cache.keep(path, "")
	}
	unplugged := filepath.Join(notPlugged(t), "Games", "a.nsp")
	elsewhere := filepath.Join(folder, "card", "a.nsp")
	away := checkedFile{fileStamp: fileStamp{Size: 100, Modified: time.Now(), Volume: 0xfeedfacecafe, Index: 7, Changed: time.Now()}, Checked: time.Now()}
	cache.mutex.Lock()
	cache.files[unplugged] = away
	cache.files[elsewhere] = away
	cache.changed()
	cache.mutex.Unlock()
	cache.flush()
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, make([]byte, 200), 0o644); err != nil {
		t.Fatal(err)
	}
	again := testCache(t, cache.path)
	again.load()
	again.purging.Wait()
	again.flush()
	for name, files := range map[string]map[string]checkedFile{"read": again.files, "written": testCacheFile(t, cache.path)} {
		for _, path := range []string{kept, unplugged, elsewhere} {
			if _, ok := files[path]; !ok {
				t.Errorf("%s: %s is left out", name, path)
			}
		}
		for _, path := range []string{gone, changed, inOld} {
			if _, ok := files[path]; ok {
				t.Errorf("%s: %s is kept", name, path)
			}
		}
	}
}
