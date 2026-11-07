package cache

import (
	"testing"
)

// The version of the check a file was checked by is kept with it, and read again: a file of a
// kind whose check grew is read again when an older version checked it.
func TestCheckCacheKeepsTheVersionOfTheCheck(t *testing.T) {
	cache, folder := testCheckCache(t, "")
	nup := testGame(t, folder, "a.nup", 100)
	nsp := testGame(t, folder, "b.nsp", 50)
	cache.KeepChecked(nup, "3 files checked by their SHA-256", 1)
	cache.Keep(nsp, "")
	if entry, ok := cache.Find(nup); !ok || entry.Checker != 1 || entry.Note != "3 files checked by their SHA-256" {
		t.Fatalf("the NUP: %+v %v", entry, ok)
	}
	if entry, ok := cache.Find(nsp); !ok || entry.Checker != 0 {
		t.Fatalf("the NSP: %+v %v", entry, ok)
	}
	cache.Flush()
	if files := testCacheFile(t, cache.path); files[nup].Checker != 1 || files[nsp].Checker != 0 {
		t.Errorf("the cache written holds %v", files)
	}
}
