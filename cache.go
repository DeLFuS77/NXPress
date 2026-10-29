package main

import (
	"encoding/json"
	"errors"
	"io/fs"
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
	// a change is written a while after it, with the ones that follow, and at the end of a work
	dirty  bool
	timer  *time.Timer
	saving sync.Mutex
	// the files gone are left out once, in the background, after the cache is read
	purging sync.WaitGroup
}

// checkCacheDelay is the longest a file checked waits to be written in the cache, before the end
// of the work (a variable, so the tests wait less)
var checkCacheDelay = 5 * time.Second

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

// load reads the cache of the user, once, and then leaves out in the background the files gone
// or changed.
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
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	// a cache that is null or broken starts again empty
	var read map[string]checkedFile
	if json.Unmarshal(data, &read) != nil {
		return
	}
	for path, entry := range read {
		// an entry kept before the time of change was can never be found again
		if !entry.Changed.IsZero() {
			c.files[path] = entry
		}
	}
	c.purging.Add(1)
	go func() {
		defer c.purging.Done()
		c.purge()
	}()
}

// purge leaves out the files gone or changed, only on the drives that are here: what was checked
// on a microSD, a USB drive or a share not plugged in is kept for when it is again. The files are
// looked at without holding the cache, so the work goes on meanwhile.
func (c *checkCache) purge() {
	c.mutex.Lock()
	entries := make(map[string]checkedFile, len(c.files))
	for path, entry := range c.files {
		entries[path] = entry
	}
	c.mutex.Unlock()
	drives := drivesHere{}
	stale := []string{}
	for path, entry := range entries {
		if !drives.here(path, entry.Volume) {
			continue
		}
		stamp, err := stampOf(path)
		// a file that cannot be read now may still be there
		if err == nil && !entry.same(stamp) || errors.Is(err, fs.ErrNotExist) {
			stale = append(stale, path)
		}
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, path := range stale {
		// a file kept again meanwhile stays
		if entry, ok := c.files[path]; ok && entry.Checked.Equal(entries[path].Checked) {
			delete(c.files, path)
			c.changed()
		}
	}
}

// drivesHere tells whether the drive a file was checked on is here, by the nearest of its
// folders that exists: a drive or a share not plugged in has none, and a microSD in the place of
// another is another drive. Each folder is looked at once.
type drivesHere map[string]driveHere

type driveHere struct {
	volume uint64
	found  bool
}

func (d drivesHere) here(path string, volume uint64) bool {
	// the root of a drive or a share first: one not plugged in is not waited for at each folder
	if root := filepath.VolumeName(path); root != "" && !d.find(root+string(filepath.Separator)).found {
		return false
	}
	drive := d.find(filepath.Dir(path))
	return drive.found && drive.volume == volume
}

// find gives the drive of the nearest folder that exists, from folder up.
func (d drivesHere) find(folder string) driveHere {
	looked := []string{}
	drive := driveHere{}
	for {
		if known, ok := d[folder]; ok {
			drive = known
			break
		}
		looked = append(looked, folder)
		if stamp, err := stampOf(folder); err == nil && stamp.Folder {
			drive = driveHere{volume: stamp.Volume, found: true}
			break
		}
		parent := filepath.Dir(folder)
		if parent == folder {
			break
		}
		folder = parent
	}
	for _, folder := range looked {
		d[folder] = drive
	}
	return drive
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

// keep records a file as checked now. It looks at that file alone, whatever the number of files
// in the cache, which is written a while later or at the end of the work.
func (c *checkCache) keep(path string, note string) {
	c.load()
	stamp, err := stampOf(path)
	if err != nil || stamp.Folder {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.files[path] = checkedFile{fileStamp: stamp, Checked: time.Now(), Note: note}
	c.changed()
}

// changed has the cache written a while after its first change, with the ones that follow. The
// mutex is held.
func (c *checkCache) changed() {
	c.dirty = true
	if c.timer == nil && c.path != "" {
		c.timer = time.AfterFunc(checkCacheDelay, c.save)
	}
}

// flush writes what the cache has left to write, at the end of a work.
func (c *checkCache) flush() {
	c.save()
}

// save writes the cache if it changed, as a new file that takes the place of the old one: a power
// cut or a full drive leaves the old one or the new one, never half of one.
func (c *checkCache) save() {
	c.saving.Lock()
	defer c.saving.Unlock()
	c.mutex.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if !c.dirty || c.path == "" {
		c.mutex.Unlock()
		return
	}
	data, err := json.Marshal(c.files)
	c.dirty = false
	c.mutex.Unlock()
	if err == nil {
		err = writeWhole(c.path, data)
	}
	if err != nil {
		// written again with the next change, or at the end of the work
		c.mutex.Lock()
		c.dirty = true
		c.mutex.Unlock()
	}
}

// writeWhole writes a file under a temporary name next to it, and gives it its name once it is
// on the disk.
func writeWhole(path string, data []byte) error {
	folder := filepath.Dir(path)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(folder, "."+filepath.Base(path)+".*.new")
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = commitFile(file.Name(), path)
	}
	if err != nil {
		os.Remove(file.Name())
	}
	return err
}
