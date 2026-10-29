package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The files already checked, kept between runs: a file checked again, still the same file with
// the same size and times, is not read again. Every new file NXPress writes is checked, and is
// kept too.
type checkCache struct {
	mutex sync.Mutex
	path  string
	files map[string]checkedFile
}

// fileStamp tells a file apart: its size and date, the drive and the number the system gives it
// there, and when its entry changed last. A copy keeps the size and the date, as Explorer and
// robocopy make it, but it is another file, with its own number and changed when it was made.
type fileStamp struct {
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Volume   uint64    `json:"volume"`
	Index    uint64    `json:"index"`
	Changed  time.Time `json:"changed"`
	Folder   bool      `json:"-"`
}

// same tells whether two stamps are of the same file, not changed between them. A stamp kept
// before the number and the time of change were has neither, so its file is checked again.
func (s fileStamp) same(other fileStamp) bool {
	return s.Size == other.Size && s.Modified.Equal(other.Modified) && s.Volume == other.Volume &&
		s.Index == other.Index && s.Changed.Equal(other.Changed)
}

type checkedFile struct {
	fileStamp
	Checked time.Time `json:"checked"`
	Note    string    `json:"note,omitempty"`
}

var checked = &checkCache{}

// load reads the cache of the user, once.
func (c *checkCache) load() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.files != nil {
		return
	}
	c.files = map[string]checkedFile{}
	if c.path == "" {
		folder, err := dataFolder()
		if err != nil {
			return
		}
		c.path = filepath.Join(folder, "checked.json")
	}
	if data, err := os.ReadFile(c.path); err == nil {
		// a cache that is null or broken starts again empty
		if json.Unmarshal(data, &c.files) != nil || c.files == nil {
			c.files = map[string]checkedFile{}
		}
	}
}

// find gives when a file was checked, if it did not change since.
func (c *checkCache) find(path string) (checkedFile, bool) {
	c.load()
	stamp, err := stampOf(path)
	if err != nil {
		return checkedFile{}, false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	entry, ok := c.files[path]
	if !ok || !entry.same(stamp) {
		return checkedFile{}, false
	}
	return entry, true
}

// keep records a file as checked now, and writes the cache.
func (c *checkCache) keep(path string, note string) {
	c.load()
	stamp, err := stampOf(path)
	if err != nil || stamp.Folder {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.files[path] = checkedFile{fileStamp: stamp, Checked: time.Now(), Note: note}
	// the files gone or changed are left out
	for name, entry := range c.files {
		if info, err := os.Stat(name); err != nil || info.Size() != entry.Size {
			delete(c.files, name)
		}
	}
	if c.path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(c.path), 0o755)
	if data, err := json.Marshal(c.files); err == nil {
		os.WriteFile(c.path+".new", data, 0o644)
		os.Rename(c.path+".new", c.path)
	}
}
