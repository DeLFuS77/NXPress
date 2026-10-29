package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The files already checked, kept between runs: a file checked again with the same size and
// time of change is not read again. Every new file NXPress writes is checked, and is kept too.
type checkCache struct {
	mutex sync.Mutex
	path  string
	files map[string]checkedFile
}

type checkedFile struct {
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Checked  time.Time `json:"checked"`
	Note     string    `json:"note,omitempty"`
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
	info, err := os.Stat(path)
	if err != nil {
		return checkedFile{}, false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	entry, ok := c.files[path]
	if !ok || entry.Size != info.Size() || !entry.Modified.Equal(info.ModTime()) {
		return checkedFile{}, false
	}
	return entry, true
}

// keep records a file as checked now, and writes the cache.
func (c *checkCache) keep(path string, note string) {
	c.load()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.files[path] = checkedFile{Size: info.Size(), Modified: info.ModTime(), Checked: time.Now(), Note: note}
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
